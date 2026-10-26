package fakehub

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	acp "github.com/caelis-labs/acp-go-sdk"
)

type testClient struct {
	mu       sync.Mutex
	replayed chan json.RawMessage
	notes    chan json.RawMessage // _relay/ready/changed, when a test wants them
}

func (c *testClient) RequestPermission(context.Context, acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	return acp.RequestPermissionResponse{}, nil
}
func (c *testClient) SessionUpdate(context.Context, acp.SessionNotification) error { return nil }
func (c *testClient) HandleExtensionMethod(_ context.Context, method string, params json.RawMessage) (any, error) {
	if method == "_relay/replayed" {
		c.replayed <- params
	}
	if method == "_relay/ready/changed" && c.notes != nil {
		c.notes <- params
	}
	return nil, nil
}

// connect plays a client against the fake hub over pipes.
func connect(t *testing.T, cfg Config) (*acp.ClientSideConnection, *testClient) {
	t.Helper()
	return connectHub(t, New(cfg, io.Discard))
}

func connectHub(t *testing.T, h *Hub) (*acp.ClientSideConnection, *testClient) {
	t.Helper()
	cr, aw := io.Pipe()
	ar, cw := io.Pipe()
	agentConn := acp.NewAgentSideConnection(h, aw, ar)
	go func() { _ = Serve(context.Background(), h, agentConn, "") }()
	tc := &testClient{replayed: make(chan json.RawMessage, 4)}
	conn := acp.NewClientSideConnection(tc, cw, cr)
	t.Cleanup(func() { _ = conn.Close(); _ = agentConn.Close() })
	return conn, tc
}

func roleMeta(id string) map[string]json.RawMessage {
	b, _ := json.Marshal(id)
	return map[string]json.RawMessage{"relay/role": b}
}

func TestInitializeSaysWhetherRelayKeepsConversations(t *testing.T) {
	for _, embedded := range []bool{false, true} {
		conn, _ := connect(t, Config{Embedded: embedded})
		init, err := conn.Initialize(context.Background(), acp.InitializeRequest{ProtocolVersion: 1})
		if err != nil {
			t.Fatal(err)
		}
		want := "true"
		if embedded {
			want = "false"
		}
		if string(init.Meta["relay/keeps"]) != want || string(init.Meta["relay/hub"]) != "1" {
			t.Errorf("embedded=%v: meta %v", embedded, init.Meta)
		}
	}
}

// Every replay ends with _relay/replayed, a fresh conversation's too, and
// it says whether history was cut.
func TestEveryNewSessionEndsWithTheReplayedMarker(t *testing.T) {
	cfg := Config{Roles: []RoleConfig{
		{ID: "fresh", Title: "Fresh"},
		{ID: "old", Title: "Old", Live: &Live{Truncated: true, History: []Said{{User: "q"}, {Agent: "a"}}}},
	}}
	conn, tc := connect(t, cfg)
	ctx := context.Background()
	for id, want := range map[string]bool{"fresh": false, "old": true} {
		resp, err := conn.NewSession(ctx, acp.NewSessionRequest{Cwd: "/", McpServers: []acp.McpServer{}, Meta: roleMeta(id)})
		if err != nil {
			t.Fatal(err)
		}
		select {
		case p := <-tc.replayed:
			var got struct {
				SessionID string `json:"sessionId"`
				Truncated bool   `json:"truncated"`
			}
			if err := json.Unmarshal(p, &got); err != nil || got.SessionID != string(resp.SessionId) || got.Truncated != want {
				t.Errorf("%s: %s", id, p)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("%s: no _relay/replayed", id)
		}
	}
}

// A second prompt while a turn runs is refused, as relay's hub does.
func TestSecondPromptWhileATurnRunsIsRefused(t *testing.T) {
	h := New(Config{Roles: []RoleConfig{{ID: "dev", Title: "Dev"}}}, io.Discard)
	busy := make(chan string, 4)
	h.busy = busy
	conn, _ := connectHub(t, h)
	ctx := context.Background()
	resp, err := conn.NewSession(ctx, acp.NewSessionRequest{Cwd: "/", McpServers: []acp.McpServer{}, Meta: roleMeta("dev")})
	if err != nil {
		t.Fatal(err)
	}
	prompt := func(text string) error {
		_, err := conn.Prompt(ctx, acp.PromptRequest{SessionId: resp.SessionId, Prompt: []acp.ContentBlock{acp.TextBlock(text)}})
		return err
	}
	done := make(chan error, 1)
	go func() { done <- prompt("slow") }()
	<-busy // the first prompt has the conversation
	err = prompt("again")
	var re *acp.RequestError
	if !asRequestError(err, &re) || re.Code != -32000 {
		t.Fatalf("second prompt: %v", err)
	}
	_ = conn.Cancel(ctx, acp.CancelNotification{SessionId: resp.SessionId})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := prompt("after"); err != nil {
		t.Fatalf("a prompt after the answer is refused: %v", err)
	}
}

func asRequestError(err error, target **acp.RequestError) bool {
	re, ok := err.(*acp.RequestError)
	if ok {
		*target = re
	}
	return ok
}

// The roster learns that a turn ended a moment after the prompt's answer,
// as relay's does, so clients meet that ordering.
func TestRosterTurnEndsAfterThePromptAnswer(t *testing.T) {
	h := New(Config{Roles: []RoleConfig{{ID: "dev", Title: "Dev"}}}, io.Discard)
	busy := make(chan string, 4)
	h.busy = busy
	conn, _ := connectHub(t, h)
	ctx := context.Background()
	resp, err := conn.NewSession(ctx, acp.NewSessionRequest{Cwd: "/", McpServers: []acp.McpServer{}, Meta: roleMeta("dev")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Prompt(ctx, acp.PromptRequest{SessionId: resp.SessionId, Prompt: []acp.ContentBlock{acp.TextBlock("hi")}}); err != nil {
		t.Fatal(err)
	}
	turn := func() string {
		raw, err := conn.CallExtension(ctx, "_relay/roles", map[string]any{})
		if err != nil {
			t.Fatal(err)
		}
		var r struct {
			Roles []struct {
				Conversation struct {
					Turn string `json:"turn"`
				} `json:"conversation"`
			} `json:"roles"`
		}
		_ = json.Unmarshal(raw, &r)
		return r.Roles[0].Conversation.Turn
	}
	if got := turn(); got != "running" {
		t.Fatalf("right after the answer the roster says %q", got)
	}
	time.Sleep(200 * time.Millisecond)
	if got := turn(); got != "idle" {
		t.Fatalf("later it says %q", got)
	}
}

func TestReadyViewAnswersRepliesAndNotifies(t *testing.T) {
	item := json.RawMessage(`{"id":"message:4","kind":"message","message":4,"replyTo":4,"title":"T","replies":[]}`)
	h := New(Config{Ready: []json.RawMessage{item}}, io.Discard)
	conn, tc := connectHub(t, h)
	notes := make(chan json.RawMessage, 2)
	tc.notes = notes
	ctx := context.Background()
	raw, err := conn.CallExtension(ctx, "_relay/ready", map[string]any{})
	if err != nil || !json.Valid(raw) {
		t.Fatalf("%s %v", raw, err)
	}
	if _, err := conn.CallExtension(ctx, "_relay/ready", map[string]any{"sessionId": "s"}); err == nil {
		t.Errorf("a sessionId was taken")
	}
	if _, err := conn.CallExtension(ctx, "_relay/reply", map[string]any{"inReplyTo": 9, "text": "x"}); err == nil {
		t.Errorf("a reply to nothing was taken")
	}
	if _, err := conn.CallExtension(ctx, "_relay/reply", map[string]any{"inReplyTo": 4, "text": "Thanks"}); err != nil {
		t.Fatal(err)
	}
	select {
	case n := <-notes:
		if !strings.Contains(string(n), `"Thanks"`) {
			t.Errorf("%s", n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no notification")
	}
	// A hub with no view says so, as a plain agent does.
	conn2, _ := connect(t, Config{})
	if _, err := conn2.CallExtension(ctx, "_relay/ready", map[string]any{}); err == nil {
		t.Errorf("a hub without the view answered")
	}
}

func TestReadyViewUndoesAMoveOnceAndNotifies(t *testing.T) {
	item := json.RawMessage(`{"id":"move:3","kind":"move","move":3,"undo":3,"undone":false,"title":"s.pdf","lines":[]}`)
	h := New(Config{Ready: []json.RawMessage{item}}, io.Discard)
	conn, tc := connectHub(t, h)
	notes := make(chan json.RawMessage, 2)
	tc.notes = notes
	ctx := context.Background()
	if _, err := conn.CallExtension(ctx, "_relay/ready", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.CallExtension(ctx, "_relay/undo", map[string]any{"move": 3, "call": 1}); err == nil {
		t.Errorf("a parameter it does not take was taken")
	}
	raw, err := conn.CallExtension(ctx, "_relay/undo", map[string]any{"move": 3})
	if err != nil || !strings.Contains(string(raw), "Undid move 3") {
		t.Fatalf("%s %v", raw, err)
	}
	select {
	case n := <-notes:
		if !strings.Contains(string(n), `"undone":true`) {
			t.Errorf("%s", n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no notification")
	}
	if _, err := conn.CallExtension(ctx, "_relay/undo", map[string]any{"move": 3}); err == nil || !strings.Contains(err.Error(), "nothing left to undo") {
		t.Errorf("a second undo: %v", err)
	}
}
