package core

import (
	"slices"
	"strings"
	"time"
)

// Hub mode: relay's hub answers one ACP connection with the whole roster
// of roles and a live conversation per role. Hub is the top-level state
// then. It holds the roster as relay last sent it, one State per
// conversation attached, and which role is in focus. ReduceHub is pure,
// as Reduce is: it applies an event to the hub, hands events for one
// conversation to Reduce, and returns effects for the shell.
//
// Nothing here is true of its own accord. The roster is relay's, and
// replaces itself whole whenever relay sends it. A conversation's
// entries are what its updates said, in the order they came, whether
// they were replayed or live.

// TurnState is where a conversation's turn stands, as the roster says.
type TurnState string

const (
	TurnIdle    TurnState = "idle"
	TurnRunning TurnState = "running"
	// TurnQueued means a prompt waits for the role's scheduled work to
	// finish: a role runs one turn at a time.
	TurnQueued TurnState = "queued"
)

// Role is one row of the roster.
type Role struct {
	ID, Title string
	Labels    Labels
	// Working says a scheduled wake of the role runs.
	Working bool
	// Holds is how many of the role's calls wait for the operator.
	Holds int
	// Conv is the role's live conversation, or nil.
	Conv *RosterConv
}

// RosterConv is the roster's account of a live conversation.
type RosterConv struct {
	Session                string
	OpenedAt, LastActivity time.Time
	Turn                   TurnState
	Turns                  int
	// Unread says a turn ended after the last one the operator saw.
	Unread bool
	// Approvals is how many permission requests are asked and not
	// answered.
	Approvals int
}

// Conv is a conversation this client has attached to, or is attaching
// to. Session is empty while a new one is being opened and relay has not
// yet said its id.
type Conv struct {
	Session, Role string
	// Attached says the attach has been answered.
	Attached bool
	// InRoster says the roster has listed the session. A listed session
	// the roster stops listing has ended.
	InRoster bool
	// Loaded says relay has finished the replay (_relay/replayed), or a
	// turn of ours has ended, so what is on screen is the latest.
	Loaded bool
	// Truncated says relay dropped the oldest history from the replay; the
	// view says so at the top of the conversation, in its own words.
	Truncated bool
	// Closed says what was left open by the replay has been closed, once
	// the roster reported the conversation idle.
	Closed bool
	// SeenSent is the highest turn this client has told relay it saw.
	SeenSent int
	State    State
}

// Hub is the whole client state in hub mode: the app, which holds the
// board, beside the roster and the conversations.
type Hub struct {
	// App is the connection's own state (see App): the board, there with
	// or without a conversation, and the pane that shows it.
	App App
	// Open is the role named by --open, focused at the start.
	Open string
	// Connected says initialize has answered; Agent, Title and Version
	// come from agentInfo.
	Connected             bool
	Agent, Title, Version string
	Embedded              bool
	// KeepsKnown says relay told us whether it keeps conversations when this
	// client goes (_meta["relay/keeps"]); Keeps is the answer. False is
	// embedded mode: they end when this client closes.
	KeepsKnown, Keeps bool
	Roles             []Role
	RosterLoaded      bool
	Convs             []Conv
	// Focus is the role whose conversation is on screen ("" for none).
	// Cursor is the role the list has selected. List says the list has
	// the keys: on a narrow screen it is the screen. The keys are in one
	// of three columns: the list, the conversation in focus, or the pane
	// (App.PaneFocus). With no role in focus there is no conversation
	// column, so the keys are in the list or the pane.
	Focus, Cursor string
	List          bool
	// EndAsk says the operator is being asked to confirm ending Focus's
	// conversation. Ending says the request is out.
	EndAsk, Ending bool
	// EndSession is the session the question is about. It is asked for
	// that conversation only: if the role's conversation is another by the
	// time the answer comes, the question is dropped.
	EndSession string
	// Ended are the sessions this client ended, so a roster that still
	// lists one is not taken for a new conversation.
	Ended []string
	// Notice is a note for the status line, cleared by the next key.
	Notice string
	Help   bool
	// TooSmall: see State.TooSmall. Blanks hold the paste state of roles
	// with no conversation yet, so what is pasted there is kept.
	TooSmall bool
	Blanks   []Blank
	// Restore is a saved view still on its way back (see view.go).
	Restore *Restore
	// Exit is why the connection went away, once Gone.
	Exit string
	Gone bool
}

// maxEnded bounds the sessions remembered as ended or failed.
const maxEnded = 64

// noteEnded is Ended with a session added, oldest dropped past maxEnded.
func (h Hub) noteEnded(session string) []string {
	out := append(slices.Clone(h.Ended), session)
	return out[max(len(out)-maxEnded, 0):]
}

// Blank is the state of pastes made to a role that has no conversation:
// they wait, as placeholders in its draft, for the first message to start
// one.
type Blank struct {
	Role  string
	State State
}

// NewHub is the state at start: open names the role to focus, or "".
func NewHub(open string) Hub {
	return Hub{Open: open, List: open == "", App: App{Moving: true}}
}

// PaneShown says the workspace pane has something to show: the
// conversation in focus, or the board, which needs no conversation.
func (h Hub) PaneShown() bool { return h.Focused() != nil || !h.App.Board.Unavailable }

// RoleByID is the role with this id, or nil.
func (h Hub) RoleByID(id string) *Role {
	for i := range h.Roles {
		if h.Roles[i].ID == id {
			return &h.Roles[i]
		}
	}
	return nil
}

// ConvOf is the conversation attached for a role, or nil. A conversation
// whose role is not yet known counts when the roster lists its session
// under the role.
func (h Hub) ConvOf(role string) *Conv {
	for i := range h.Convs {
		if h.Convs[i].Role == role {
			return &h.Convs[i]
		}
	}
	if r := h.RoleByID(role); r != nil && r.Conv != nil {
		for i := range h.Convs {
			if h.Convs[i].Role == "" && h.Convs[i].Session == r.Conv.Session {
				return &h.Convs[i]
			}
		}
	}
	return nil
}

// Focused is the conversation on screen, or nil.
func (h Hub) Focused() *Conv {
	if h.Focus == "" {
		return nil
	}
	return h.ConvOf(h.Focus)
}

// ConvBySession is the conversation with this session id, or nil.
func (h Hub) ConvBySession(session string) *Conv {
	if session == "" {
		return nil
	}
	for i := range h.Convs {
		if h.Convs[i].Session == session {
			return &h.Convs[i]
		}
	}
	return nil
}

// Approvals is how many requests wait in a role's conversation: what
// relay counts, or, if more, what this client holds.
func (h Hub) Approvals(r Role) int {
	n := 0
	if r.Conv != nil {
		n = r.Conv.Approvals
	}
	if c := h.ConvOf(r.ID); c != nil {
		n = max(n, len(c.State.Asks))
	}
	return n
}

// Events from the hub's connection, translated by package wire.
type (
	// HubConnected means initialize answered with _meta["relay/hub"].
	HubConnected struct {
		At                    time.Time
		Agent, Title, Version string
		Embedded              bool
		KeepsKnown, Keeps     bool
	}
	// RosterUpdate is the whole roster, from _relay/roles or
	// _relay/roles/changed. Err is set when the roster could not be had.
	RosterUpdate struct {
		At    time.Time
		Roles []Role
		Err   string
	}
	// Routed is an event for the conversation with this session id.
	Routed struct {
		Session string
		Event   Event
	}
	// Attached is the answer to an attach. Session is the conversation's
	// id; Role the role it was asked for.
	Attached struct {
		At            time.Time
		Role, Session string
		Err           string
	}
	// PermissionSettled is _relay/permission/settled: another device
	// answered the request for CallID.
	PermissionSettled struct {
		At                       time.Time
		Session, CallID, Outcome string
	}
	// Replayed is _relay/replayed: the replay of a conversation is over.
	Replayed struct {
		At        time.Time
		Session   string
		Truncated bool // relay dropped the oldest history; see Conv.Truncated
	}
	// ConversationEnded is the answer to ending a conversation.
	ConversationEnded struct {
		At      time.Time
		Session string
		Err     string
	}
)

// Events from the user in hub mode. Events of the single-session kind
// (Submit, Interrupt, PaneKey and so on) apply to the conversation in
// focus.
type (
	// FocusIndex brings a role's conversation on screen, in the mode the
	// operator was in: 1 (moving) or alt+1 (typing) is Index 0.
	FocusIndex struct {
		At    time.Time
		Index int
	}
	ListMove struct {
		At    time.Time
		Delta int
	}
	// ListOpen is enter on the list: the role under the cursor, typing.
	ListOpen          struct{ At time.Time }
	StartConversation struct{ At time.Time }
	AskEnd            struct{ At time.Time } // X on the conversation
	ConfirmEnd        struct {
		At  time.Time
		Yes bool
	}
)

func (e HubConnected) at() time.Time      { return e.At }
func (e RosterUpdate) at() time.Time      { return e.At }
func (e Routed) at() time.Time            { return e.Event.at() }
func (e Attached) at() time.Time          { return e.At }
func (e PermissionSettled) at() time.Time { return e.At }
func (e Replayed) at() time.Time          { return e.At }
func (e ConversationEnded) at() time.Time { return e.At }
func (e FocusIndex) at() time.Time        { return e.At }
func (e ListMove) at() time.Time          { return e.At }
func (e ListOpen) at() time.Time          { return e.At }
func (e StartConversation) at() time.Time { return e.At }
func (e AskEnd) at() time.Time            { return e.At }
func (e ConfirmEnd) at() time.Time        { return e.At }

// Effects for the hub's shell.
type (
	// ForSession is an effect of Reduce for the conversation with this
	// session id.
	ForSession struct {
		Session string
		Effect  Effect
	}
	// Attach opens a role's conversation: session/load of Session when
	// relay listed one, else session/new for Role.
	Attach struct{ Role, Session string }
	// EndConversation asks relay to end the conversation.
	EndConversation struct{ Session string }
	// SendSeen tells relay the operator has seen the conversation
	// through Turn.
	SendSeen struct {
		Session string
		Turn    int
	}
)

func (ForSession) effect()      {}
func (Attach) effect()          {}
func (EndConversation) effect() {}
func (SendSeen) effect()        {}

// ReduceHub applies one event. It never mutates h's slices in place. An
// operator's event that the surface does not take (see HubSurface) is
// dropped whole: whatever the shell made of a key, it acts only where
// the keys are.
func ReduceHub(h Hub, e Event) (Hub, []Effect) {
	surf := HubSurface(h)
	if !surf.takes(e) {
		return h, nil
	}
	h.Roles = slices.Clone(h.Roles)
	h.Convs = slices.Clone(h.Convs)
	h.Ended = slices.Clone(h.Ended)
	h.Blanks = slices.Clone(h.Blanks)
	h.Restore = pending(h.Restore, e)
	r := &hubReducer{h: h}
	before, offered, view := h.onScreen(), h.offered(), h.view()
	if _, ok := e.(Unbound); ok && surf.Kind == SurfaceEditor {
		r.h.Notice = EditorPasteNote // only a paste reaches the editor as Unbound
	} else {
		r.apply(e)
	}
	r.restoreHub(e.at())
	r.settleColumn()
	r.adoptBlanks()
	r.settle(before, e.at())
	r.arm(offered, e.at())
	r.keepCardTrust(view)
	r.markSeen(e.at())
	return r.h, r.fx
}

// blank is the paste state for a role with no conversation, made when
// asked.
func (r *hubReducer) blank(role string) *Blank {
	for i := range r.h.Blanks {
		if r.h.Blanks[i].Role == role {
			return &r.h.Blanks[i]
		}
	}
	r.h.Blanks = append(r.h.Blanks, Blank{Role: role})
	return &r.h.Blanks[len(r.h.Blanks)-1]
}

// adoptBlanks gives each role that now has a conversation the pastes that
// waited for it, however the conversation came to be: a message, the
// roster, --open or a load.
func (r *hubReducer) adoptBlanks() {
	for i := len(r.h.Blanks) - 1; i >= 0; i-- {
		b := r.h.Blanks[i]
		j := r.convIndex(b.Role)
		if j < 0 {
			continue
		}
		adoptWaiting(&r.h.Convs[j].State, b.State)
		r.h.Blanks = slices.Delete(r.h.Blanks, i, i+1)
	}
}

// onScreen is the session of the conversation the operator can see the
// whole of: in focus and not behind the list, the help or the end
// question. It is "" when there is none.
func (h Hub) onScreen() string {
	c := h.Focused()
	if c == nil || h.List || h.EndAsk || h.Help {
		return ""
	}
	return c.Session
}

// settle acts on what a transition did to what is on screen, whatever
// caused it: a request that has just come into view starts its delay
// afresh, so a key typed for something else cannot answer it, and a trust
// confirmation in a conversation that has left the screen is dropped.
// Doing it here, on the change, means no way of covering and uncovering a
// conversation can skip it.
func (r *hubReducer) settle(before string, at time.Time) {
	h := &r.h
	if h.EndAsk {
		if c := h.Focused(); c == nil || c.Session != h.EndSession {
			h.EndAsk = false
			h.Notice = "the conversation changed, so it was not ended"
		}
	}
	after := h.onScreen()
	if after == before {
		return
	}
	if i := r.sessionIndex(before); i >= 0 {
		r.toConv(i, Blurred{At: at}, false)
	}
	if i := r.sessionIndex(after); i >= 0 && len(h.Convs[i].State.Asks) > 0 {
		r.toConv(i, Focused{At: at}, false)
	}
}

// offered is the session whose request on offer is the surface, or "".
func (h Hub) offered() string {
	if HubSurface(h).Kind != SurfaceRequest {
		return ""
	}
	return h.onScreen()
}

// arm starts a request's delay afresh when it becomes the surface: by
// esc out of typing, by moving to its role or its column, by what
// covered it closing, or by its arriving while its conversation has the
// keys. A key meant for something else cannot answer it.
func (r *hubReducer) arm(was string, at time.Time) {
	now := r.h.offered()
	if now == was || now == "" {
		return
	}
	if i := r.sessionIndex(now); i >= 0 {
		r.toConv(i, Focused{At: at}, false)
	}
}

// hubView is what decides what the screen shows, apart from the pane.
type hubView struct {
	focus                    string
	list, pane, help, endAsk bool
}

func (h Hub) view() hubView {
	return hubView{focus: h.Focus, list: h.List, pane: h.App.PaneFocus, help: h.Help, endAsk: h.EndAsk}
}

// keepCardTrust drops a trust of a card that has not been sent when the
// screen changed under it: another role came into focus, the keys went
// to another column, or the help or the end question covered it. The operator asks again on
// purpose, so a y typed for something else cannot answer it. A doc's
// trust goes the same way, with its conversation (Blurred).
func (r *hubReducer) keepCardTrust(was hubView) {
	b := &r.h.App.Board
	if r.h.view() != was && b.Trust != nil && !b.Trust.Sent {
		b.Trust = nil
		b.Message, b.Failure = "Trust cancelled.", ""
	}
}

type hubReducer struct {
	h  Hub
	fx []Effect
}

func (r *hubReducer) emit(f Effect) { r.fx = append(r.fx, f) }

// convIndex is where the conversation for a role is in h.Convs, or -1.
func (r *hubReducer) convIndex(role string) int {
	c := r.h.ConvOf(role)
	if c == nil {
		return -1
	}
	return slices.IndexFunc(r.h.Convs, func(x Conv) bool { return x.Session == c.Session && x.Role == c.Role })
}

func (r *hubReducer) sessionIndex(session string) int {
	if session == "" {
		return -1
	}
	return slices.IndexFunc(r.h.Convs, func(x Conv) bool { return x.Session == session })
}

func newConv(session, role string) Conv {
	return Conv{Session: session, Role: role, State: State{Shared: true, Transcript: Transcript{Open: true}}}
}

func (r *hubReducer) apply(e Event) {
	h := &r.h
	switch e.(type) {
	case Tick, TerminalReady, Printed, Initialized, HubConnected, RosterUpdate, Routed, Attached, PermissionSettled, Replayed,
		ConversationEnded, CardsLoaded, CardsChanged, CardOpDone, CardTrustDone, AgentExited, Viewport:
	default:
		h.Notice = "" // a key was pressed
	}
	switch e := e.(type) {
	case HubConnected:
		h.Connected = true
		h.Agent, h.Title, h.Version, h.Embedded = e.Agent, e.Title, e.Version, e.Embedded
		h.KeepsKnown, h.Keeps = e.KeepsKnown, e.Keeps
		// The board is the connection's: it is asked for now, before any
		// conversation opens.
		h.App.Live = true
		r.withApp(func(rd *reducer) { rd.askCards() })
		if h.Open != "" {
			r.focus(h.Open, true, e.At)
			h.App.Moving = false // opened to talk to it
		}
	case RosterUpdate:
		r.roster(e)
	case Routed:
		r.routed(e)
	case CardsLoaded, CardsChanged, CardOpDone, CardTrustDone:
		r.toApp(e, false)
	case PaneKey:
		if h.List {
			return // the pane does not have the keys
		}
		r.toApp(e, true)
	case Attached:
		r.attached(e)
	case Replayed:
		if i := r.sessionIndex(e.Session); i >= 0 {
			h.Convs[i].Loaded = true
			h.Convs[i].Truncated = e.Truncated
			// A call still open when the replay ends is running if the
			// turn is (it was started on another device and its update will
			// arrive live). It stops only when the roster says no turn runs;
			// before the roster has come, the roster going idle closes it.
			if r.turnIdle(h.Convs[i]) {
				r.toConv(i, ReplayEnd{At: e.At}, false)
			}
		}
	case PermissionSettled:
		if i := r.sessionIndex(e.Session); i >= 0 {
			r.toConv(i, Settled{At: e.At, CallID: e.CallID, Outcome: e.Outcome}, false)
		}
	case ConversationEnded:
		r.ended(e)
	case AgentExited:
		if h.Gone {
			return
		}
		h.Gone, h.Exit = true, e.Err
		h.App.Live = false
		for i := range h.Convs {
			r.toConv(i, e, false)
		}
		r.emit(Quit{})
	case Tick, TerminalReady, Printed, KeyPressed, Initialized:
	case FocusIndex:
		if e.Index >= 0 && e.Index < len(h.Roles) {
			r.focus(h.Roles[e.Index].ID, false, e.At)
		}
	case Move:
		r.column(e.Delta, e.At)
	case Type:
		r.startTyping(e.At)
	case StopTyping:
		h.App.Moving = true
	case Unbound:
		h.Notice = UnboundNote
	case ListMove:
		if h.List {
			r.moveCursor(e.Delta)
		}
	case ListOpen:
		if h.List && h.RoleByID(h.Cursor) != nil {
			r.focus(h.Cursor, false, e.At)
			h.App.Moving = false
		}
	case StartConversation:
		if h.Focus != "" && h.Focused() == nil && !h.List {
			r.ensureAttached(h.Focus, true)
		}
	case AskEnd:
		if c := h.Focused(); c != nil && c.Attached && c.Session != "" && !h.Ending && !h.List {
			h.EndAsk, h.EndSession = true, c.Session
		}
	case ConfirmEnd:
		if !h.EndAsk {
			return
		}
		h.EndAsk = false
		if c := h.Focused(); e.Yes && c != nil && c.Session != "" && c.Session == h.EndSession {
			h.Ending = true
			r.emit(EndConversation{Session: c.Session})
		}
	case ToggleHelp:
		h.Help = !h.Help
	case Viewport:
		h.TooSmall = e.TooSmall
	case PermissionConfirm, PermissionQuick:
		if !h.TooSmall {
			r.toFocused(e)
		}
	case Pasted, TrustPaste:
		if h.Focus == "" {
			return // not at the input
		}
		if h.Focused() != nil {
			r.toFocused(e)
			return
		}
		// No conversation yet: the paste waits in the role's draft.
		b := r.blank(h.Focus)
		var fx []Effect
		h.App, b.State, fx = reduce(h.App, b.State, e, true) // the app counts the paste
		for _, f := range fx {
			switch f.(type) {
			case InsertInput, ReplaceInput:
				r.emit(f)
			}
		}
		if m := b.State.Message; m.Text != "" && m.At.Equal(e.at()) {
			h.Notice = m.Text
		}
	case Submit:
		if strings.TrimSpace(e.Text) == "" || h.Focus == "" || h.List {
			return
		}
		if h.Focused() == nil {
			r.ensureAttached(h.Focus, true)
		}
		r.adoptBlanks() // the pastes must be in before the message resolves
		r.toFocused(e)
	case Interrupt:
		r.interrupt(e)
	case EOF:
		r.emit(Quit{}) // detach only; see interrupt
	default:
		r.toFocused(e)
	}
}

// col is the column with the keys. With no role in focus there is no
// conversation column, so the keys are in the list unless the pane has
// them.
func (h Hub) col() Column {
	switch {
	case h.List:
		return ColumnList
	case h.App.PaneFocus:
		return ColumnPane
	case h.Focus == "":
		return ColumnList
	}
	return ColumnConversation
}

// has says a column has something in it: the conversation only with a
// role in focus, the pane only with something to show.
func (h Hub) has(col Column) bool {
	switch col {
	case ColumnList:
		return true
	case ColumnConversation:
		return h.Focus != ""
	case ColumnPane:
		return h.PaneShown()
	}
	return false
}

// column is h or l: the keys go to the next column with something in
// it, delta's way, and stay put at the edge. The pane is entered and
// left as a toggle, which drops what was unsent there and refuses while
// a request has the keys.
func (r *hubReducer) column(delta int, at time.Time) {
	h := &r.h
	h.App.Moving = true
	if delta == 0 {
		return
	}
	step := Column(1)
	if delta < 0 {
		step = -1
	}
	to := h.col() + step
	for to >= ColumnList && to <= ColumnPane && !h.has(to) {
		to += step
	}
	if to < ColumnList || to > ColumnPane {
		return
	}
	r.toColumn(to, at)
}

// toColumn gives the keys to column to.
func (r *hubReducer) toColumn(to Column, at time.Time) {
	h := &r.h
	if h.App.PaneFocus && to != ColumnPane {
		r.toApp(PaneKey{At: at, Action: PaneToggle}, true)
		if h.App.PaneFocus {
			return // the pane is waiting on relay, and keeps the keys
		}
	}
	switch to {
	case ColumnList:
		if !h.List {
			h.List = true
			if h.Focus != "" {
				h.Cursor = h.Focus
			}
			r.clampCursor()
		}
	case ColumnConversation:
		h.List = false
	case ColumnPane:
		h.List = false
		if !h.App.PaneFocus {
			r.toApp(PaneKey{At: at, Action: PaneToggle}, true)
		}
	}
}

// startTyping is i, or enter on the conversation: the keys go to the
// conversation in focus, to type. With no role in focus there is
// nothing to type into, and the status line says so.
func (r *hubReducer) startTyping(at time.Time) {
	h := &r.h
	if h.Focus == "" {
		h.Notice = "no role in focus: pick one first"
		return
	}
	r.toColumn(ColumnConversation, at)
	if h.col() == ColumnConversation {
		h.App.Moving = false
	}
}

// settleColumn keeps the keys somewhere they can be: with no role in
// focus there is no conversation to have them, so the list does; and
// anywhere but the conversation is moving.
func (r *hubReducer) settleColumn() {
	h := &r.h
	if h.Focus == "" && !h.List && !h.App.PaneFocus {
		h.List = true
		r.clampCursor()
	}
	if h.List {
		h.App.PaneFocus = false // one column has the keys
	}
	if h.List || h.App.PaneFocus || h.Focus == "" {
		h.App.Moving = true
	}
}

// toFocused applies a single-session event to the conversation in focus.
func (r *hubReducer) toFocused(e Event) {
	if r.h.List {
		return
	}
	if i := r.convIndex(r.h.Focus); i >= 0 {
		r.toConv(i, e, true)
	}
}

// toConv reduces one conversation, with the app around it, and maps its
// effects. user says the event came from the operator, so Quit means
// quit.
func (r *hubReducer) toConv(i int, e Event, user bool) {
	shown := r.h.Focus != "" && i == r.convIndex(r.h.Focus)
	c := &r.h.Convs[i]
	var fx []Effect
	r.h.App, c.State, fx = reduce(r.h.App, c.State, e, shown)
	for _, f := range fx {
		r.mapEffect(*c, f, user)
	}
}

// toApp applies an event about the connection's own state: the board,
// or a key for the pane. The pane shows the conversation in focus beside
// the board, so the event is reduced with that conversation when there
// is one, and with none otherwise: the board works with no conversation
// open.
func (r *hubReducer) toApp(e Event, user bool) {
	if i := r.convIndex(r.h.Focus); r.h.Focus != "" && i >= 0 {
		r.toConv(i, e, user)
		return
	}
	r.withApp(func(rd *reducer) { rd.apply(e) })
}

// withApp runs f on the app with no conversation, and maps its effects.
func (r *hubReducer) withApp(f func(*reducer)) {
	rd := &reducer{a: r.h.App, shown: true}
	rd.a.Board.Cards = slices.Clone(rd.a.Board.Cards)
	f(rd)
	r.h.App = rd.a
	for _, x := range rd.fx {
		r.mapEffect(Conv{}, x, false)
	}
}

func (r *hubReducer) mapEffect(c Conv, f Effect, user bool) {
	switch f := f.(type) {
	case AnswerPermission:
		r.emit(f)
	case RequestCards, SendCardOp, SendCardTrust:
		// The board's calls are the connection's, about no conversation.
		r.emit(f)
	case InsertInput, RestoreInput, ReplaceInput:
		if c.Role == r.h.Focus && c.Role != "" {
			r.emit(f)
		} else if ri, ok := f.(RestoreInput); ok {
			// Not the conversation on screen, so there is no input to put
			// it in. It stays where the operator will find it.
			i := slices.IndexFunc(r.h.Convs, func(x Conv) bool { return x.Session == c.Session && x.Role == c.Role })
			r.h.Convs[i].State.Entries = append(slices.Clone(r.h.Convs[i].State.Entries),
				Entry{Kind: Note, Text: "not sent: " + oneLineText(ri.Text), Final: true})
		}
	case Quit:
		if user {
			r.emit(f)
		}
	default:
		r.emit(ForSession{Session: c.Session, Effect: f})
	}
}

// oneLineText is s on one line, for a note.
func oneLineText(s string) string { return strings.Join(strings.Fields(s), " ") }

func (r *hubReducer) cancelAsks(i int) {
	c := &r.h.Convs[i]
	var fx []Effect
	rd := &reducer{s: c.State, a: r.h.App}
	rd.cancelAsks()
	c.State, fx = rd.s, rd.fx
	for _, f := range fx {
		r.mapEffect(*c, f, false)
	}
}

// interrupt is Ctrl-C: it stops what is running in the conversation in
// focus, here or on another device, and otherwise quits.
func (r *hubReducer) interrupt(e Interrupt) {
	h := &r.h
	c := h.Focused()
	if c == nil && !h.List && !e.InputEmpty {
		return // Ctrl-C clears the line; there is nothing else to stop
	}
	if c == nil || h.List {
		r.emit(Quit{}) // detach: a request left unanswered is relay's to hold
		return
	}
	if role := h.RoleByID(h.Focus); c.State.Phase == Idle && e.InputEmpty && role != nil && role.Conv != nil &&
		role.Conv.Session == c.Session && role.Conv.Turn != TurnIdle && role.Conv.Turn != "" {
		// A turn another device started, or a prompt waiting its turn.
		r.emit(ForSession{Session: c.Session, Effect: SendCancel{}})
		return
	}
	st := c.State
	if (st.Phase == Busy && !st.Turn.Cancelling) || (st.Phase == Idle && !e.InputEmpty) {
		// A turn of ours is stopped, on purpose and in view (its own
		// requests are cancelled with it), or the line is cleared.
		r.toFocused(e)
		return
	}
	// Quitting only detaches. No request in any conversation is answered:
	// agentloop takes "cancelled" as a denial, and relay holds an
	// unanswered request for the operator and tells them.
	r.emit(Quit{})
}

func (r *hubReducer) moveCursor(delta int) {
	h := &r.h
	n := len(h.Roles)
	if n == 0 {
		return
	}
	from := slices.IndexFunc(h.Roles, func(x Role) bool { return x.ID == h.Cursor })
	if from < 0 {
		from = 0
		delta = 0
	}
	h.Cursor = h.Roles[((from+delta)%n+n)%n].ID
}

func (r *hubReducer) clampCursor() {
	h := &r.h
	if h.RoleByID(h.Cursor) == nil {
		h.Cursor = ""
		if len(h.Roles) > 0 {
			h.Cursor = h.Roles[0].ID
		}
	}
}

// focus brings a role's conversation on screen, attaching to it when
// relay lists one. force opens one when there is none.
func (r *hubReducer) focus(role string, force bool, at time.Time) {
	h := &r.h
	// The operator came to the role for its conversation: the keys go to
	// it, not to the pane or the list they left. The board keeps its
	// place; what was unsent in the pane goes, as leaving it with h does.
	if h.App.PaneFocus {
		r.toApp(PaneKey{At: at, Action: PaneToggle}, true)
		h.App.PaneFocus = false // even past a trust in flight, whose answer still lands
	}
	h.Focus, h.Cursor, h.List = role, role, false
	h.EndAsk, h.Ending = false, false
	r.ensureAttached(role, force)
	if i := r.convIndex(role); i >= 0 {
		r.toConv(i, Focused{At: at}, false)
	}
}

// ensureAttached makes sure a conversation is attached or attaching for
// the role: a live one in the roster, or, with force, a new one.
func (r *hubReducer) ensureAttached(role string, force bool) {
	h := &r.h
	if h.ConvOf(role) != nil {
		return
	}
	if rl := h.RoleByID(role); rl != nil && rl.Conv != nil && !slices.Contains(h.Ended, rl.Conv.Session) {
		c := newConv(rl.Conv.Session, role)
		c.InRoster = true // the roster is where the session came from
		h.Convs = append(h.Convs, c)
		r.emit(Attach{Role: role, Session: rl.Conv.Session})
		return
	}
	if force {
		h.Convs = append(h.Convs, newConv("", role))
		r.emit(Attach{Role: role})
	}
}

// routed hands an event to the conversation it names, which a replay may
// meet before its attach has been answered.
func (r *hubReducer) routed(e Routed) {
	i := r.sessionIndex(e.Session)
	if i < 0 {
		if e.Session == "" || slices.Contains(r.h.Ended, e.Session) {
			return
		}
		r.h.Convs = append(r.h.Convs, newConv(e.Session, ""))
		i = len(r.h.Convs) - 1
	}
	// More of the conversation has come in, so whatever the roster said
	// about it closing is out of date until the roster says it again.
	r.h.Convs[i].Closed = false
	switch e.Event.(type) {
	case PromptDone, PromptFailed:
		// A turn this client ran was seen live; a fresh conversation has no
		// replay to wait for.
		r.h.Convs[i].Loaded = true
	}
	r.toConv(i, e.Event, false)
}

func (r *hubReducer) attached(e Attached) {
	h := &r.h
	if e.Err != "" {
		i := slices.IndexFunc(h.Convs, func(x Conv) bool {
			return x.Role == e.Role && !x.Attached && (x.Session == "" || x.Session == e.Session)
		})
		if i >= 0 {
			s := h.Convs[i].State
			h.Convs = slices.Delete(h.Convs, i, i+1)
			// The role has no conversation again: the pastes its input or
			// queue still name wait in its blank, as before it had one.
			if len(s.Pastes) > 0 || s.NextPaste > 0 {
				b := r.blank(e.Role)
				adoptWaiting(&b.State, State{Pastes: s.Pastes, NextPaste: s.NextPaste})
			}
			if len(s.Queue) > 0 && h.Focus == e.Role {
				r.emit(RestoreInput{Text: strings.Join(s.Queue, "\n\n")})
			}
		}
		if e.Session != "" {
			// Not live any more, whatever a roster still in flight says.
			h.Ended = h.noteEnded(e.Session)
		}
		h.Notice = "could not open the conversation: " + oneLineText(e.Err)
		return
	}
	if slices.Contains(h.Ended, e.Session) {
		return
	}
	byRole := slices.IndexFunc(h.Convs, func(x Conv) bool { return x.Role == e.Role && x.Session == "" })
	bySession := r.sessionIndex(e.Session)
	switch {
	case bySession >= 0 && byRole >= 0:
		// The replay got here first and made a conversation of its own;
		// the one that was waiting gives it what the operator put in it.
		adoptWaiting(&h.Convs[bySession].State, h.Convs[byRole].State)
		h.Convs = slices.Delete(h.Convs, byRole, byRole+1)
		bySession = r.sessionIndex(e.Session)
	case bySession < 0 && byRole >= 0:
		h.Convs[byRole].Session = e.Session
		bySession = byRole
	case bySession < 0:
		h.Convs = append(h.Convs, newConv(e.Session, e.Role))
		bySession = len(h.Convs) - 1
	}
	c := &h.Convs[bySession]
	c.Role, c.Attached = e.Role, true
	if rl := h.RoleByID(e.Role); rl != nil && rl.Conv != nil && rl.Conv.Session == e.Session {
		c.InRoster = true // the roster already lists it
	}
	r.toConv(bySession, ConvAttached{At: e.At, Embedded: h.Embedded}, false)
	if e.Role == h.Focus {
		r.toConv(bySession, Focused{At: e.At}, false)
	}
}

// adoptWaiting folds into dst what the operator gave src, a
// conversation that waited for its attach: its queue, ahead of dst's,
// and its pastes with their counter, so the placeholders in the input
// and the queue still send their text and no number is used twice. A
// paste of dst's with a number of src's gives way to src's, whose
// placeholder is the one in the input.
func adoptWaiting(dst *State, src State) {
	dst.Queue = append(slices.Clone(src.Queue), dst.Queue...)
	pastes := slices.Clone(src.Pastes)
	for _, p := range dst.Pastes {
		if !slices.ContainsFunc(src.Pastes, func(q Paste) bool { return q.ID == p.ID }) {
			pastes = append(pastes, p)
		}
	}
	dst.Pastes, dst.NextPaste = pastes, max(dst.NextPaste, src.NextPaste)
}

func (r *hubReducer) ended(e ConversationEnded) {
	h := &r.h
	h.Ending = false
	if e.Err != "" {
		h.Notice = "could not end the conversation: " + oneLineText(e.Err)
		return
	}
	if i := r.sessionIndex(e.Session); i >= 0 {
		h.Convs = slices.Delete(h.Convs, i, i+1)
	}
	h.Ended = h.noteEnded(e.Session)
	h.EndAsk = false
	h.Notice = "conversation ended"
}

// turnIdle says the roster is loaded and lists no running or queued turn
// for the conversation.
func (r *hubReducer) turnIdle(c Conv) bool {
	if !r.h.RosterLoaded {
		return false
	}
	for _, rl := range r.h.Roles {
		if rl.Conv != nil && rl.Conv.Session == c.Session {
			return rl.Conv.Turn != TurnRunning && rl.Conv.Turn != TurnQueued
		}
	}
	return true
}

// roster replaces the roster and settles the conversations against it.
func (r *hubReducer) roster(e RosterUpdate) {
	h := &r.h
	if e.Err != "" {
		h.Notice = "no roster: " + oneLineText(e.Err)
		return
	}
	old := h.Roles
	h.Roles = slices.Clone(e.Roles)
	h.RosterLoaded = true
	// A session the roster no longer lists cannot come back from a stale
	// roster, so it need not be remembered.
	h.Ended = slices.DeleteFunc(slices.Clone(h.Ended), func(s string) bool {
		return !slices.ContainsFunc(e.Roles, func(r Role) bool { return r.Conv != nil && r.Conv.Session == s })
	})
	r.clampCursor()
	r.reconcile(old)
	if h.Focus != "" && h.RoleByID(h.Focus) == nil {
		// The pane keeps the keys if it had them; otherwise the list takes
		// them (settleColumn).
		h.Notice = "no role " + oneLineText(h.Focus)
		h.Focus = ""
	}
}

// reconcile settles the conversations against the roster. old is the
// roster before this update, or nil.
func (r *hubReducer) reconcile(old []Role) {
	h := &r.h
	for i := len(h.Convs) - 1; i >= 0; i-- {
		c := &h.Convs[i]
		if c.Role == "" { // adopt a replay's conversation once a role lists it
			for _, rl := range h.Roles {
				if rl.Conv != nil && rl.Conv.Session == c.Session {
					c.Role = rl.ID
				}
			}
		}
		rl := h.RoleByID(c.Role)
		if rl != nil && rl.Conv != nil && rl.Conv.Session == c.Session && c.Session != "" {
			c.InRoster = true
			continue
		}
		if c.InRoster {
			// Listed before, listed no more: it ended, perhaps on another
			// device.
			if c.Role == h.Focus {
				h.Notice = "the conversation ended"
			}
			h.Convs = slices.Delete(h.Convs, i, i+1)
		}
	}
	// What a conversation left open can be closed once relay says no turn
	// runs: after one ran, or after the attach.
	for i := range h.Convs {
		c := &h.Convs[i]
		rl := h.RoleByID(c.Role)
		if rl == nil || rl.Conv == nil || rl.Conv.Session != c.Session || !c.Attached {
			continue
		}
		was := TurnIdle
		for _, o := range old {
			if o.ID == rl.ID && o.Conv != nil && o.Conv.Session == c.Session {
				was = o.Conv.Turn
			}
		}
		if rl.Conv.Turn == TurnIdle && (!c.Closed || was != TurnIdle) {
			c.Closed = true
			r.toConv(i, RemoteIdle{}, false)
		}
	}
	// A conversation opened elsewhere, in the role on screen.
	if h.Focus != "" && !h.List {
		r.ensureAttached(h.Focus, false)
	}
}

// markSeen tells relay the operator has seen the conversation in focus
// through its latest turn, once that turn is on screen: the conversation
// is attached, shown, scrolled to its end, and no turn runs.
func (r *hubReducer) markSeen(time.Time) {
	h := &r.h
	if h.List || h.Focus == "" {
		return
	}
	i := r.convIndex(h.Focus)
	rl := h.RoleByID(h.Focus)
	if i < 0 || rl == nil || rl.Conv == nil {
		return
	}
	c := &h.Convs[i]
	rc := rl.Conv
	if !c.Attached || !c.Loaded || h.Help || h.EndAsk || h.App.PaneFocus ||
		c.Session != rc.Session || c.State.Phase == Exited || c.State.Transcript.Scroll != 0 ||
		!rc.Unread || rc.Turn != TurnIdle || rc.Turns <= c.SeenSent {
		return
	}
	c.SeenSent = rc.Turns
	r.emit(SendSeen{Session: c.Session, Turn: rc.Turns})
}
