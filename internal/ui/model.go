// Package ui is the Bubble Tea program: a thin shell around the core. Its
// Update turns a message into at most one core event, runs core.Reduce,
// and turns the effects into commands. keys.go and print.go are pure.
package ui

import (
	"image/color"
	"reflect"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"

	"github.com/kirahowe/relay-tui/internal/core"
	"github.com/kirahowe/relay-tui/internal/render"
)

// Conn is the agent, as the UI uses it. None of its methods may block
// except Shutdown.
type Conn interface {
	// The methods that take a session name the conversation they are
	// about; against a plain agent it is "", its only one.
	Prompt(session, text string, atts []core.Attachment)
	Cancel(session string)
	Answer(core.AnswerPermission)
	// Items, Resolve and Trust call relay's extension methods; their
	// answers come back as events.
	Items(session string)
	Resolve(session, ref string)
	Trust(session, path, version, sha string)
	// Cards, CardOp and CardTrust call relay's board methods, which are
	// the connection's and about no conversation; their answers, and
	// relay's notifications of changed cards, come back as events.
	Cards()
	CardOp(card int64, op core.CardOp)
	CardTrust(card, rev int64, sha string)
	// Attach, End and Seen are relay hub methods. Attach opens a role's
	// conversation (loading Session when it is known); End ends one; Seen
	// says the operator has read it through a turn. Answers come back as
	// events.
	Attach(role, session string)
	End(session string)
	Seen(session string, turn int)
	Shutdown()
}

// EventMsg carries a core event from the agent into the program.
type EventMsg struct{ Event core.Event }

type (
	tickMsg    time.Time
	readyMsg   struct{}
	printedMsg struct{}
	// backMsg says the inline frame is on screen again after the
	// transcript view.
	backMsg struct{}
)

// tickEvery is how often the status line redraws while the agent works.
const tickEvery = 100 * time.Millisecond

// readyDelay is how long after the first size message the first frame is
// taken to be on screen. Bubble Tea #1740: a print before the first flush
// moves the cursor down a whole screen.
const readyDelay = 50 * time.Millisecond

type Model struct {
	// one is the state against a single agent; hub, against relay's hub.
	one   core.Single
	input textarea.Model
	caps  Caps
	// termBg and termFg are the terminal's own colours, once it says;
	// render blends its tints from them.
	termBg, termFg color.Color
	w, h           int
	conn           Conn
	sized          bool
	ticking        bool
	// printing is set while lines are on their way to scrollback; more
	// wait until they land, so they arrive in order.
	printing bool
	// pinned is set once a first print has moved the frame to the bottom
	// row of the screen.
	pinned bool
	// quitting is set once the shell begins shutting down, and stopping
	// once everything has printed and the program is told to stop; Died
	// says the agent exited before that.
	quitting, stopping bool
	died               bool
	// alt is set while the transcript view is on the alternate screen,
	// and back until the inline frame is drawn again after it. Nothing
	// is printed meanwhile: Bubble Tea would write the lines into the
	// alternate screen, and they would never reach scrollback.
	alt, back bool
	// hubMode is set once the agent has said it is relay's hub. The
	// screen is then the hub's, drawn whole on the alternate screen,
	// and core is not used. open is the role --open named, and drafts
	// the input each role not in focus had when it was left.
	hubMode bool
	hub     core.Hub
	open    string
	drafts  map[string]string
	// inputRole is the role whose draft the input box holds.
	inputRole string
	// onView is told the saved view whenever it changes; seen is the
	// last one it was told.
	onView func(core.View)
	seen   core.View
}

// New returns the model for a conversation with conn. title names the
// session in the banner; empty means use the agent's own title or name.
func New(conn Conn, title string) Model {
	ta := textarea.New()
	ta.SetStyles(textarea.Styles{})
	ta.SetVirtualCursor(false)
	ta.SetPromptFunc(2, func(p textarea.PromptInfo) string {
		if p.LineNumber == 0 {
			return "› "
		}
		return "  "
	})
	ta.ShowLineNumbers = false
	ta.Placeholder = ""
	ta.DynamicHeight = true
	ta.MinHeight = 1
	ta.MaxHeight = 6              // until the first size message
	ta.MaxContentHeight = 1 << 20 // the box scrolls past MaxHeight, without a cap
	ta.KeyMap.InsertNewline.SetEnabled(false)
	ta.Focus()
	return Model{input: ta, conn: conn, one: core.Single{State: core.State{Title: title}}}
}

// Open makes the model focus the role at the start, when the agent turns
// out to be relay's hub.
func (m Model) Open(role string) Model {
	m.open = role
	return m
}

// Restore gives the model the view saved when it last closed against
// this agent, and a note to show about it ("" for none). The core puts
// it back as what it refers to arrives.
func (m Model) Restore(v core.View, note string) Model {
	m.one = core.RestoreSingle(m.one, v)
	if note != "" {
		m.one.State.Message = core.Message{Text: note, At: time.Now()}
	}
	return m
}

// OnView makes the model tell f the saved view whenever it changes. f
// must not block.
func (m Model) OnView(f func(core.View)) Model {
	m.onView = f
	m.seen = m.SavedView()
	return m
}

// SavedView is the view to save: where the operator is now.
func (m Model) SavedView() core.View {
	if m.hubMode {
		drafts := make(map[string]string, len(m.drafts)+1)
		for role, text := range m.drafts {
			drafts[role] = text
		}
		if m.inputRole != "" {
			drafts[m.inputRole] = m.input.Value()
		}
		return core.SaveHub(m.hub, drafts)
	}
	return core.SaveSingle(m.one, m.input.Value())
}

// tellView tells onView the saved view if it has changed.
func (m *Model) tellView() {
	if m.onView == nil {
		return
	}
	if v := m.SavedView(); !reflect.DeepEqual(v, m.seen) {
		m.seen = v
		m.onView(v)
	}
}

// Keeps reports, in hub mode, whether relay said it keeps the
// conversations when this client goes (known is false when it did not
// say, or when the agent is not the hub).
func (m Model) Keeps() (known, keeps bool) {
	if !m.hubMode {
		return false, false
	}
	return m.hub.KeepsKnown, m.hub.Keeps
}

// Died reports whether the agent exited on its own, and why.
func (m Model) Died() (bool, string) {
	if m.hubMode {
		return m.died, m.hub.Exit
	}
	return m.died, m.one.State.Exit
}

// Init does nothing: the first size message starts the ticks.
// Init asks the terminal for its colours. A terminal that never answers
// keeps the untinted look.
func (m Model) Init() tea.Cmd {
	return tea.Batch(tea.RequestBackgroundColor, tea.RequestForegroundColor)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		m.termBg = msg.Color
		render.SetTerminalColors(m.termBg, m.termFg)
		return m, nil
	case tea.ForegroundColorMsg:
		m.termFg = msg.Color
		render.SetTerminalColors(m.termBg, m.termFg)
		return m, nil
	}
	if em, ok := msg.(EventMsg); ok {
		if _, ok := em.Event.(core.HubConnected); ok && !m.hubMode {
			m.hubMode, m.hub = true, core.NewHub(m.open)
			// What was typed and pasted so far is the hub's now, for the
			// first role in focus.
			m.hub = core.HubFromSingle(m.hub, m.one, m.input.Value())
			m.input.Reset()
			if msg := m.one.State.Message; msg.Text != "" && time.Since(msg.At) < core.MessageFor {
				m.hub.Notice = msg.Text
			}
			m.input.SetWidth(render.HubInputWidth(m.hub, m.width()))
		}
	}
	if m.hubMode {
		next, cmd := m.updateHub(msg)
		nm := next.(Model)
		nm.tellView()
		return nm, cmd
	}
	now := time.Now()
	var ev core.Event
	var evs []core.Event
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.input.MaxHeight = max(m.h/2-2, 1) // grow to about half the screen
		m.input.SetWidth(render.InputWidth(render.MainWidth(m.width())))
		if !m.sized {
			m.sized = true
			cmds = append(cmds, tea.Tick(readyDelay, func(time.Time) tea.Msg { return readyMsg{} }))
		}
	case tea.KeyboardEnhancementsMsg:
		m.caps.Disambiguation = msg.SupportsKeyDisambiguation()
	case tea.KeyPressMsg:
		// Every key is the operator's, even one only the input box sees.
		var cmd tea.Cmd
		evs, cmd = m.key(msg, now)
		evs = append([]core.Event{core.KeyPressed{At: now}}, evs...)
		cmds = append(cmds, cmd)
	case tea.MouseWheelMsg:
		if m.one.State.Transcript.Open {
			ev = m.wheel(msg, now)
		}
	case tea.PasteMsg:
		evs = []core.Event{core.KeyPressed{At: now}, pasted(m.one.App, now, msg.Content)}
	case EventMsg:
		ev = msg.Event
		if _, ok := ev.(core.AgentExited); ok && !m.quitting {
			m.died = true
		}
	case tickMsg:
		m.ticking = false
		ev = core.Tick{At: time.Time(msg)}
	case readyMsg:
		ev = core.TerminalReady{At: now}
	case printedMsg:
		m.printing = false
	case backMsg:
		m.back = false
	}
	if ev != nil {
		evs = append(evs, ev)
	}
	for _, e := range evs {
		if e == nil {
			continue
		}
		var fx []core.Effect
		m.one, fx = core.ReduceSingle(m.one, e)
		for _, f := range fx {
			cmds = append(cmds, m.effect(f, now))
		}
	}
	if len(m.one.State.Asks) > 0 || m.one.State.TooSmall {
		if f := m.frame(now); f.TooSmall != m.one.State.TooSmall {
			m.one, _ = core.ReduceSingle(m.one, core.Viewport{At: now, TooSmall: f.TooSmall})
		}
	}
	cmds = append(cmds, m.screen(), m.print(now), m.tick(now), m.stop())
	m.tellView()
	return m, tea.Batch(cmds...)
}

// stop ends the program once it is quitting and nothing is left to
// print: lines the transcript view held back would be lost otherwise.
func (m *Model) stop() tea.Cmd {
	if !m.quitting || m.stopping || m.alt || m.back || m.printing {
		return nil
	}
	m.stopping = true
	conn := m.conn
	return func() tea.Msg {
		conn.Shutdown()
		return tea.Quit()
	}
}

// screen moves to the alternate screen once the transcript view is open
// and no print is in flight, and back when it closes. Leaving, it waits
// readyDelay for the inline frame to be drawn before anything prints.
func (m *Model) screen() tea.Cmd {
	switch open := m.one.State.Transcript.Open && len(m.one.State.Asks) == 0; {
	case open && !m.alt && !m.printing:
		m.alt = true
	case !open && m.alt:
		m.alt, m.back = false, true
		return tea.Tick(readyDelay, func(time.Time) tea.Msg { return backMsg{} })
	}
	return nil
}

// wheel scrolls the transcript view three lines at a time.
func (m *Model) wheel(w tea.MouseWheelMsg, now time.Time) core.Event {
	a := core.TranscriptPageUp
	switch w.Button {
	case tea.MouseWheelUp:
	case tea.MouseWheelDown:
		a = core.TranscriptPageDown
	default:
		return nil
	}
	return core.TranscriptKey{At: now, Action: a, Max: m.transcriptFrame(now).Max, Page: 3}
}

// key turns a key press into events, or passes it to the input box.
func (m *Model) key(k tea.KeyPressMsg, now time.Time) ([]core.Event, tea.Cmd) {
	one := func(e core.Event) ([]core.Event, tea.Cmd) { return []core.Event{e}, nil }
	if len(m.one.State.Asks) > 0 && m.one.App.Moving {
		// While moving the request has every key; while typing none, and
		// esc moves to answer it.
		return one(askKey(ClassifyAsk(k), now))
	}
	empty := m.input.Value() == ""
	if m.one.State.Help {
		// Any key closes the list of keys, and does only that.
		return one(core.ToggleHelp{At: now})
	}
	if m.one.State.Transcript.Open {
		return one(m.transcriptKey(k, empty, now))
	}
	if !m.one.App.Moving {
		return m.typingKey(k, now)
	}
	a, index := ClassifyMoving(k)
	if m.one.App.PaneFocus {
		return m.paneMoving(k, a, index, m.one.State, now)
	}
	// The conversation, moving. It is printed into the terminal's
	// scrollback, so the terminal scrolls it, and ctrl+t or ctrl+o opens
	// the transcript.
	switch a {
	case MoveLeft:
		return one(core.Move{At: now, Delta: -1}) // the edge: nothing
	case MoveRight:
		return one(core.Move{At: now, Delta: 1})
	case MoveEnter, MoveType:
		return one(core.Type{At: now})
	case MoveTools:
		return one(core.ShowTools{At: now})
	case MoveThinking:
		return one(core.ShowThinking{At: now})
	case MoveHelp:
		return one(core.ToggleHelp{At: now})
	case MoveInterrupt:
		return one(core.Interrupt{At: now, InputEmpty: true})
	case MoveEOF:
		if empty {
			return one(core.EOF{At: now})
		}
	default:
		if printable(k) {
			return one(core.Unbound{At: now})
		}
	}
	return nil, nil
}

// pasted is the event for a paste: into the input while typing, and a
// word on how to type otherwise, so nothing pasted is lost unseen.
func pasted(a core.App, now time.Time, text string) core.Event {
	if a.Moving {
		return core.Unbound{At: now}
	}
	return core.Pasted{At: now, Text: text} // the core drops it when the input is not in play
}

// transcriptKey is the event for a key pressed in the transcript view.
// Ctrl-C stops a running turn and, otherwise, leaves the view; Ctrl-D
// quits only from an empty input.
func (m *Model) transcriptKey(k tea.KeyPressMsg, empty bool, now time.Time) core.Event {
	a, move := ClassifyTranscript(k, empty)
	switch a {
	case Move:
		tf := m.transcriptFrame(now)
		return core.TranscriptKey{At: now, Action: move, Max: tf.Max, Page: tf.Page}
	case ShowTools:
		return core.ShowTools{At: now}
	case ShowThinking:
		return core.ShowThinking{At: now}
	case Help:
		return core.ToggleHelp{At: now}
	case Interrupt:
		if m.one.State.Phase == core.Busy {
			return core.Interrupt{At: now, InputEmpty: true}
		}
		return core.TranscriptKey{At: now, Action: core.TranscriptClose}
	case EOF:
		return core.EOF{At: now}
	}
	return nil
}

// paneEvent is the event for one of the pane's own keys, pressed while
// it has the keys. The input box never sees it.
func paneEvent(a core.App, st core.State, k tea.KeyPressMsg, now time.Time, detailMax func() int) core.PaneKey {
	if b := a.Board; !b.Items {
		if b.Input != nil {
			a, text := ClassifyBoardInput(k)
			return core.PaneKey{At: now, Action: a, Text: text}
		}
		return core.PaneKey{At: now, Action: ClassifyBoard(k)}
	}
	e := core.PaneKey{At: now, Action: ClassifyPane(k)}
	if st.Pane.Detail {
		e.Max = detailMax()
	}
	return e
}

// askKey is the event for a key pressed while a permission request waits.
func askKey(a AskAction, now time.Time) core.Event {
	switch a {
	case AskUp:
		return core.PermissionMove{At: now, Delta: -1}
	case AskDown:
		return core.PermissionMove{At: now, Delta: 1}
	case AskConfirm:
		return core.PermissionConfirm{At: now}
	case AskAllow:
		return core.PermissionQuick{At: now, Kind: core.AllowOnce}
	case AskReject:
		return core.PermissionQuick{At: now, Kind: core.RejectOnce}
	case AskCancel:
		return core.PermissionCancel{At: now}
	case AskShowAll:
		return core.PermissionShowAll{At: now}
	case AskInterrupt:
		return core.Interrupt{At: now, InputEmpty: true}
	case AskEOF:
		return core.EOF{At: now}
	}
	return nil
}

// effect carries out one effect of Reduce.
func (m *Model) effect(f core.Effect, now time.Time) tea.Cmd {
	if m.sessionEffect("", f) {
		return nil
	}
	switch f := f.(type) {
	case core.RequestCards:
		m.conn.Cards()
	case core.SendCardOp:
		m.conn.CardOp(f.Card, f.Op)
	case core.SendCardTrust:
		m.conn.CardTrust(f.Card, f.Rev, f.SHA256)
	case core.AnswerPermission:
		m.conn.Answer(f)
	case core.InsertInput:
		m.input.InsertString(f.Text)
	case core.RestoreInput:
		m.input.SetValue(ahead(f.Text, m.input.Value()))
	case core.RestoreDraft:
		m.input.SetValue(placed(f, m.input.Value()))
	case core.ReplaceInput:
		m.input.SetValue(strings.Replace(m.input.Value(), f.Old, f.New, 1))
	case core.Quit:
		m.quitting = true // stop ends the program
	}
	return nil
}

// placed is a restored draft put in an input that holds typed: ahead of
// it, or after it.
func placed(f core.RestoreDraft, typed string) string {
	if f.After && typed != "" {
		return ahead(typed, f.Text)
	}
	if f.After {
		return f.Text
	}
	return ahead(f.Text, typed)
}

// ahead is text put back ahead of what has been typed since.
func ahead(text, typed string) string {
	if typed != "" {
		text += "\n" + typed
	}
	return text
}

// sessionEffect carries out an effect that is about one conversation, and
// says whether it was one. session is "" against a plain agent.
func (m *Model) sessionEffect(session string, f core.Effect) bool {
	switch f := f.(type) {
	case core.SendPrompt:
		m.conn.Prompt(session, f.Text, f.Attachments)
	case core.SendCancel:
		m.conn.Cancel(session)
	case core.RequestItems:
		m.conn.Items(session)
	case core.RequestResolve:
		m.conn.Resolve(session, f.Ref)
	case core.SendTrust:
		m.conn.Trust(session, f.Path, f.Version, f.SHA256)
	default:
		return false
	}
	return true
}

// print puts the lines that have scrolled off the top of the frame into
// scrollback. The frame fills all but the last row of the screen, so
// each line is its own tea.Println (see PrintChunks). The core moves its
// cursor at once: the frame no longer shows those lines either way.
// Nothing is printed before the first frame is on screen. The first
// print starts with a blank line: on a cleared screen the frame starts at
// the top row and ends one row short of the bottom, and a print scrolls
// it down there.
func (m *Model) print(now time.Time) tea.Cmd {
	if !m.one.State.Ready || m.printing || m.alt || m.back || m.one.State.Transcript.Open {
		return nil
	}
	f := m.frame(now)
	lines := f.Print
	if len(lines) > 0 {
		m.one, _ = core.ReduceSingle(m.one, core.Printed{At: now, Upto: f.PrintTo})
		if m.one.State.Printed != f.PrintTo {
			lines = nil // the core refused it; printing would duplicate lines
		}
	}
	if !m.pinned {
		m.pinned = true
		lines = append([]string{""}, lines...)
	}
	if len(lines) == 0 {
		return nil
	}
	m.printing = true
	var cmds []tea.Cmd
	for _, chunk := range PrintChunks(lines, m.h-strings.Count(f.Content, "\n")-1) {
		cmds = append(cmds, tea.Println(chunk))
	}
	cmds = append(cmds, func() tea.Msg { return printedMsg{} })
	return tea.Sequence(cmds...)
}

// tick keeps the status line moving while the agent works, and stops when
// it is idle, so an idle screen is never redrawn.
func (m *Model) tick(now time.Time) tea.Cmd {
	if m.ticking || (m.one.State.Phase != core.Busy && m.one.State.Phase != core.Starting && !m.trustArming(now)) {
		return nil
	}
	m.ticking = true
	return tea.Tick(tickEvery, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// trustArming says a trust confirmation is waiting out its delay, so the
// pane must redraw when it arms.
func (m *Model) trustArming(now time.Time) bool {
	since, ok := core.TrustSince(m.one.App, m.one.State)
	return ok && now.Sub(since) < core.ArmDelay+tickEvery
}

// width is the width everything is drawn at: one column narrower than
// the screen, so a wide grapheme measured differently cannot wrap (Bubble
// Tea #1802), and printed boxes line up with the input box.
func (m Model) width() int { return max(m.w-1, 1) }

func (m Model) frame(now time.Time) render.Frame {
	return render.View(m.one, m.input.View(), NewlineHint(m.caps), now, m.width(), max(m.h, 2))
}

func (m Model) transcriptFrame(now time.Time) render.TranscriptFrame {
	return render.TranscriptView(m.one.State, now, m.width(), max(m.h, 2))
}

func (m Model) View() tea.View {
	if m.hubMode {
		return m.hubView(time.Now())
	}
	if m.alt {
		v := tea.NewView(m.transcriptFrame(time.Now()).Content)
		v.AltScreen = true
		v.MouseMode = tea.MouseModeCellMotion
		return v
	}
	f := m.frame(time.Now())
	v := tea.NewView(f.Content)
	if c := m.input.Cursor(); c != nil && !m.one.App.PaneFocus && !m.one.State.Help && !m.one.App.Moving {
		c.Position.X += f.InputX
		c.Position.Y += f.InputY
		v.Cursor = c
	}
	return v
}
