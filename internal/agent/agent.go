// Package agent runs the ACP agent as a child process and turns what it
// says into core events. It is part of the shell: it starts goroutines,
// reads the clock and talks to the SDK.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	acp "github.com/caelis-labs/acp-go-sdk"
	"github.com/caelis-labs/acp-go-sdk/transport/stdio"

	"github.com/kirahowe/relay-tui/internal/core"
	"github.com/kirahowe/relay-tui/internal/wire"
)

// ShutdownGrace is how long the agent gets to exit after its stdin closes
// before its process tree is killed.
const ShutdownGrace = 3 * time.Second

// Agent is one agent process. Against a plain agent it has one session.
// Against relay's hub (initialize carries _meta["relay/hub"]) it has as
// many as the client attaches to, and every event it emits for one is
// wrapped in core.Routed with its session id.
type Agent struct {
	proc    *stdio.ClientProcess
	emit    func(core.Event)
	dir     string
	session acp.SessionId // set before Connected is emitted; unused in hub mode
	hub     bool          // set before HubConnected is emitted

	mu sync.Mutex

	// Calls to relay's extension methods run one at a time, in the order
	// they were asked, so an older answer cannot overwrite a newer one.
	extQueue   []func()
	extRunning bool
}

// Start launches command in dir and begins the handshake in the
// background. emit receives every event, from any goroutine, in order for
// each source; it may block (it does: Bubble Tea's Send). Against a
// plain agent Connected follows the session; against relay's hub
// HubConnected follows initialize's answer. AgentExited follows a failed
// handshake or the process ending.
func Start(command []string, dir string, stderr io.Writer, emit func(core.Event)) (*Agent, error) {
	if len(command) == 0 {
		return nil, errors.New("no agent command")
	}
	a := &Agent{emit: emit, dir: dir}
	if stderr == nil {
		stderr = io.Discard
	}
	// relay-tui's notes about the agent go where its stderr goes, never to
	// the screen.
	log := &lockedWriter{w: stderr}
	process, err := stdio.Start(context.Background(), stdio.Command{
		Executable: command[0], Args: command[1:], Dir: dir, Stderr: log,
	})
	if err != nil {
		return nil, err
	}
	// As stdio.StartClient, but the SDK reads the agent's output only
	// through the screen.
	conn, err := acp.NewClientSideConnectionWithOptions(client{a}, process.Input(),
		newScreened(process.Output(), log), acp.ConnectionOptions{Logger: slog.New(slog.NewTextHandler(log, nil))})
	if err != nil {
		_ = process.Close()
		return nil, err
	}
	proc := &stdio.ClientProcess{Connection: conn, Process: process}
	a.proc = proc
	var once sync.Once
	exited := func(err error) {
		once.Do(func() {
			msg := ""
			if err != nil {
				msg = err.Error()
			}
			emit(core.AgentExited{At: time.Now(), Err: msg})
		})
	}
	go func() {
		if err := a.handshake(dir); err != nil {
			exited(fmt.Errorf("handshake: %w", err))
			_ = proc.Close()
		}
	}()
	go func() { exited(proc.Wait(context.Background())) }()
	return a, nil
}

func (a *Agent) handshake(dir string) error {
	ctx := context.Background()
	init, err := a.proc.Connection.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion:    acp.ProtocolVersion(acp.ProtocolVersionNumber),
		ClientCapabilities: wire.Capabilities(),
		ClientInfo:         &acp.Implementation{Name: "relay-tui", Version: "0"},
	})
	if err != nil {
		return err
	}
	if wire.IsHub(init) {
		a.hub = true
		a.emit(wire.HubConnected(init, time.Now()))
		raw, err := a.call(wire.MethodRoles, struct{}{})
		a.emit(wire.Roster(raw, err, time.Now()))
		return nil
	}
	sess, err := a.proc.Connection.NewSession(ctx, acp.NewSessionRequest{Cwd: dir, McpServers: []acp.McpServer{}})
	if err != nil {
		return err
	}
	a.session = sess.SessionId
	a.emit(wire.Connected(init, sess, time.Now()))
	return nil
}

// Prompt sends text and, when the agent answers, emits PromptDone or
// PromptFailed. It does not block.
func (a *Agent) Prompt(session, text string) {
	id := a.sid(session)
	go func() {
		resp, err := a.proc.Connection.Prompt(context.Background(), acp.PromptRequest{
			SessionId: id,
			Prompt:    wire.PromptBlocks(text),
		})
		a.emit(a.route(id, wire.Response(resp, err, time.Now())))
	}()
}

// sid is the session a call is about: the one named, or, in single-session
// mode, where the name is "", the agent's only one.
func (a *Agent) sid(session string) acp.SessionId {
	if session == "" {
		return a.session
	}
	return acp.SessionId(session)
}

// route marks an event as belonging to a session, in hub mode. A plain
// agent has one session, so its events go unwrapped.
func (a *Agent) route(id acp.SessionId, e core.Event) core.Event {
	if !a.hub {
		return e
	}
	return core.Routed{Session: string(id), Event: e}
}

// Cancel asks the agent to stop the current turn. It does not block.
func (a *Agent) Cancel(session string) {
	id := a.sid(session)
	go func() {
		_ = a.proc.Connection.Cancel(context.Background(), acp.CancelNotification{SessionId: id})
	}()
}

// Shutdown closes the agent's stdin and waits for it to exit, killing its
// process tree after ShutdownGrace.
func (a *Agent) Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), ShutdownGrace)
	defer cancel()
	_ = a.proc.Shutdown(ctx)
}

// client answers the agent's calls.
type client struct{ a *Agent }

// SessionUpdate emits synchronously: the SDK runs a turn's notification
// handlers before that turn's Prompt returns, so every update reaches the
// core before its PromptDone.
func (c client) SessionUpdate(_ context.Context, n acp.SessionNotification) error {
	now := time.Now()
	if l, ok := wire.Labels(n, now); ok {
		c.a.emit(c.a.route(n.SessionId, l))
	}
	c.a.emit(c.a.route(n.SessionId, wire.Update(n, now)))
	return nil
}

// RequestPermission refuses the request, as a plain cancelled. relay-tui
// advertises no approvals and shows no dialog: an agent that still asks
// (relay before it dropped its holds) is answered at once, and nothing
// reaches the screen.
func (c client) RequestPermission(context.Context, acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	return wire.Cancelled(), nil
}

// methodSettled is relay's notice that another client answered a
// permission request. This client asks none, so it has nothing to close.
const methodSettled = "_relay/permission/settled"

// extTimeout is how long a call to relay's extension methods may take.
const extTimeout = 15 * time.Second

// enqueue runs f after the calls queued before it.
func (a *Agent) enqueue(f func()) {
	a.mu.Lock()
	a.extQueue = append(a.extQueue, f)
	start := !a.extRunning
	a.extRunning = true
	a.mu.Unlock()
	if start {
		go a.extLoop()
	}
}

func (a *Agent) extLoop() {
	for {
		a.mu.Lock()
		if len(a.extQueue) == 0 {
			a.extRunning = false
			a.mu.Unlock()
			return
		}
		f := a.extQueue[0]
		a.extQueue = a.extQueue[1:]
		a.mu.Unlock()
		f()
	}
}

// call sends an extension method to the agent and waits for the answer.
func (a *Agent) call(method string, params any) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), extTimeout)
	defer cancel()
	return a.proc.Connection.CallExtension(ctx, method, params)
}

// Items asks relay for the workspace's items and emits ItemsLoaded. A
// plain agent answers method-not-found. It does not block.
func (a *Agent) Items(session string) {
	id := a.sid(session)
	a.enqueue(func() {
		raw, err := a.call(wire.MethodItems, wire.ItemsParams(id))
		a.emit(a.route(id, wire.Items(raw, err, time.Now())))
	})
}

// Resolve asks relay about a reference and emits CitationResolved. It
// does not block.
func (a *Agent) Resolve(session, ref string) {
	id := a.sid(session)
	a.enqueue(func() {
		raw, err := a.call(wire.MethodResolve, wire.ResolveParams(id, ref))
		a.emit(a.route(id, wire.Resolution(ref, raw, err, time.Now())))
	})
}

// Cards asks relay for the board and emits CardsLoaded. The board is the
// connection's, so the answer is about no conversation. It does not
// block.
func (a *Agent) Cards() {
	a.enqueue(func() {
		raw, err := a.call(wire.MethodCards, wire.CardsParams())
		a.emit(wire.Cards(raw, err, time.Now()))
	})
}

// CardOp sends relay an op on a card and emits CardOpDone. It does not
// block.
func (a *Agent) CardOp(card int64, op core.CardOp) {
	a.enqueue(func() {
		raw, err := a.call(wire.MethodCardOp, wire.CardOpParams(card, op))
		a.emit(wire.CardOpOutcome(card, raw, err, time.Now()))
	})
}

// Ready asks relay for what is ready for the operator and emits
// ReadyLoaded. It is the connection's, about no conversation. It does not
// block.
func (a *Agent) Ready() {
	a.enqueue(func() {
		raw, err := a.call(wire.MethodReady, wire.ReadyParams())
		a.emit(wire.Ready(raw, err, time.Now()))
	})
}

// Reply sends relay the operator's answer to a message and emits
// ReplyDone. It does not block.
func (a *Agent) Reply(message int64, text string) {
	a.enqueue(func() {
		_, err := a.call(wire.MethodReply, wire.ReplyParams(message, text))
		a.emit(wire.ReplyOutcome(message, err, time.Now()))
	})
}

// HandleExtensionMethod takes relay's notifications about the board, the
// roster and replays. It ignores _relay/permission/settled, which a relay
// that still holds calls sends: this client was never asked. The client
// answers no extension request.
func (c client) HandleExtensionMethod(_ context.Context, method string, params json.RawMessage) (any, error) {
	switch method {
	case wire.MethodCardsChanged:
		if e, ok := wire.CardsChanged(params, time.Now()); ok {
			c.a.emit(e)
		}
		return nil, nil
	case wire.MethodReadyChanged:
		if e, ok := wire.ReadyChanged(params, time.Now()); ok {
			c.a.emit(e)
		}
		return nil, nil
	case wire.MethodRolesChanged:
		if e, ok := wire.RosterChanged(params, time.Now()); ok && c.a.hub {
			c.a.emit(e)
		}
		return nil, nil
	case wire.MethodReplayed:
		if e, ok := wire.Replayed(params, time.Now()); ok && c.a.hub {
			c.a.emit(e)
		}
		return nil, nil
	case methodSettled:
		return nil, nil
	}
	return nil, acp.NewMethodNotFound(method)
}

// Attach opens a role's conversation and emits Attached when relay
// answers. With a session it loads that live conversation, and the
// replay follows as updates; without one it asks for a new session for
// the role, and relay attaches to the role's conversation if it has one.
// It does not block.
func (a *Agent) Attach(role, session string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), extTimeout)
		defer cancel()
		var err error
		if session != "" {
			_, err = a.proc.Connection.LoadSession(ctx, acp.LoadSessionRequest{
				SessionId: acp.SessionId(session), Cwd: a.dir, McpServers: []acp.McpServer{}})
		} else {
			var resp acp.NewSessionResponse
			resp, err = a.proc.Connection.NewSession(ctx, acp.NewSessionRequest{
				Cwd: a.dir, McpServers: []acp.McpServer{}, Meta: wire.RoleMeta(role)})
			session = string(resp.SessionId)
		}
		a.emit(wire.Attached(role, session, err, time.Now()))
	}()
}

// endTimeout is how long ending a conversation may take: relay cancels a
// running turn first.
const endTimeout = 60 * time.Second

// End asks relay to end a conversation and emits ConversationEnded. It
// does not block, and does not wait behind the other extension calls.
func (a *Agent) End(session string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), endTimeout)
		defer cancel()
		_, err := a.proc.Connection.CallExtension(ctx, wire.MethodEnd, wire.SessionParams(session))
		a.emit(wire.Ended(session, err, time.Now()))
	}()
}

// Seen tells relay the operator has seen a conversation through a turn.
// The answer is not used. It does not block.
func (a *Agent) Seen(session string, turn int) {
	go func() {
		_, _ = a.call(wire.MethodSeen, wire.SeenParams(session, turn))
	}()
}
