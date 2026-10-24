package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kirahowe/relay-tui/internal/core"
	"github.com/kirahowe/relay-tui/internal/render"
)

// Hub mode: the agent is relay's hub, and the program is the hub's whole
// screen. This file is the shell for it. As in the single-session mode,
// Update turns a message into core events, runs core.ReduceHub, and turns
// the effects into commands; everything drawn comes from render.HubView.

func (m Model) updateHub(msg tea.Msg) (tea.Model, tea.Cmd) {
	now := m.now()
	var evs []core.Event
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.input.MaxHeight = max(m.h/2-2, 1)
		m.sized = true
	case tea.KeyboardEnhancementsMsg:
		m.caps.Disambiguation = msg.SupportsKeyDisambiguation()
	case tea.KeyPressMsg:
		var cmd tea.Cmd
		evs, cmd = m.hubKey(msg, now)
		// Every key is the operator's, even one only the input box sees.
		evs = append([]core.Event{core.KeyPressed{At: now}}, evs...)
		cmds = append(cmds, cmd)
	case tea.MouseWheelMsg:
		evs = m.hubWheel(msg, now)
	case tea.PasteMsg:
		evs = []core.Event{core.KeyPressed{At: now}, pasted(core.HubSurface(m.hub), now, msg.Content)}
	case EventMsg:
		evs = []core.Event{msg.Event}
		if _, ok := msg.Event.(core.AgentExited); ok && !m.quitting {
			m.died = true
		}
	case tickMsg:
		m.ticking = false
		evs = []core.Event{core.Tick{At: time.Time(msg)}}
	}
	for _, e := range evs {
		if e == nil {
			continue
		}
		var fx []core.Effect
		m.hub, fx = core.ReduceHub(m.hub, e)
		for _, f := range fx {
			m.hubEffect(f)
		}
	}
	m.swapDraft()
	// The text area is the inside of the box drawn, whatever the layout is
	// now: the pane beside it, the sidebar, a narrow screen.
	m.input.SetWidth(render.HubInputWidth(m.hub, m.width()))
	if c := m.hub.Focused(); m.hub.TooSmall || (c != nil && len(c.State.Asks) > 0) {
		// Whether the window shows the request whole decides whether the
		// keys that approve do anything.
		if f := m.hubFrame(now); f.TooSmall != m.hub.TooSmall {
			m.hub, _ = core.ReduceHub(m.hub, core.Viewport{At: now, TooSmall: f.TooSmall})
		}
	}
	cmds = append(cmds, m.hubTick(now), m.stop())
	return m, tea.Batch(cmds...)
}

// swapDraft keeps what was typed for the role whose draft the input
// held, and brings back what was typed for the one in focus now.
func (m *Model) swapDraft() {
	after := m.hub.Focus
	if m.inputRole == after {
		return
	}
	if m.drafts == nil {
		m.drafts = map[string]string{}
	}
	if m.inputRole != "" {
		m.drafts[m.inputRole] = m.input.Value()
	}
	m.input.SetValue(m.drafts[after])
	delete(m.drafts, after)
	m.inputRole = after
}

// hubEffect carries out one effect of ReduceHub.
func (m *Model) hubEffect(f core.Effect) {
	switch f := f.(type) {
	case core.ForSession:
		m.sessionEffect(f.Session, f.Effect)
	case core.Attach:
		m.conn.Attach(f.Role, f.Session)
	case core.EndConversation:
		m.conn.End(f.Session)
	case core.SendSeen:
		m.conn.Seen(f.Session, f.Turn)
	case core.RestoreDraft:
		// The input holds inputRole's draft until swapDraft runs.
		if f.Role == m.inputRole {
			m.input.SetValue(placed(f, m.input.Value()))
			return
		}
		if m.drafts == nil {
			m.drafts = map[string]string{}
		}
		m.drafts[f.Role] = placed(f, m.drafts[f.Role])
	default:
		m.effect(f, m.now())
	}
}

// focusedState is the conversation in focus, or the zero State.
func (m Model) focusedState() core.State {
	if c := m.hub.Focused(); c != nil && !m.hub.List {
		return c.State
	}
	return core.State{}
}

// hubKey turns a key press into events.
func (m *Model) hubKey(k tea.KeyPressMsg, now time.Time) ([]core.Event, tea.Cmd) {
	return m.routeKey(core.HubSurface(m.hub), k, now)
}

// routeKey turns a key press into events for the surface that has the
// keys, in either mode, classifying it against that surface's keys and
// no other. The core decides the surface (core.HubSurface,
// core.SingleSurface), and drops whatever event it does not take, so a
// key can only act where it is drawn to.
func (m *Model) routeKey(surf core.Surface, k tea.KeyPressMsg, now time.Time) ([]core.Event, tea.Cmd) {
	m.keysTo = surf
	one := func(e core.Event) ([]core.Event, tea.Cmd) { return []core.Event{e}, nil }
	a, index := ClassifyMoving(k)
	switch surf.Kind {
	case core.SurfaceHelp:
		// Any key closes the list of keys, and does only that: the list
		// covers what the key would act on.
		return one(core.ToggleHelp{At: now})
	case core.SurfaceEnd:
		if answered, yes := ClassifyEnd(k); answered {
			return one(core.ConfirmEnd{At: now, Yes: yes})
		}
		return nil, nil
	case core.SurfaceTranscript:
		return one(m.transcriptKey(k, m.input.Value() == "", now))
	case core.SurfaceRequest:
		// Only the request's own keys answer it. Past those, in hub mode,
		// the operator can still go to the list or another role and
		// scroll; the request stays until answered, and the sidebar says
		// so.
		if e := askKey(ClassifyAsk(k), now); e != nil {
			return one(e)
		}
		if m.kind == kindHub {
			switch a {
			case MoveJump:
				return one(core.FocusIndex{At: now, Index: index})
			case MoveLeft:
				return one(core.Move{At: now, Delta: -1})
			case MovePageUp, MovePageDown:
				return one(m.hubScroll(k, now))
			}
		}
		return nil, nil
	case core.SurfaceInput:
		return m.typingKey(k, now)
	case core.SurfaceEditor:
		return one(m.editorKey(k, now))
	case core.SurfaceConfirm:
		return one(m.confirmKey(k, now))
	case core.SurfacePane:
		return m.paneMoving(k, a, index, now)
	case core.SurfaceList:
		return one(m.listKey(k, a, index, now))
	}
	if m.kind == kindHub {
		return one(m.hubConvKey(k, a, index, now))
	}
	return one(m.singleConvKey(k, a, now))
}

// hubConvKey is the event for a key pressed while moving in the
// conversation column, in hub mode.
func (m *Model) hubConvKey(k tea.KeyPressMsg, a MoveAction, index int, now time.Time) core.Event {
	switch a {
	case MoveLeft:
		return core.Move{At: now, Delta: -1}
	case MoveRight:
		return core.Move{At: now, Delta: 1}
	case MoveUp:
		return m.scroll(core.TranscriptUp, now)
	case MoveDown:
		return m.scroll(core.TranscriptDown, now)
	case MovePageUp:
		return m.scroll(core.TranscriptPageUp, now)
	case MovePageDown:
		return m.scroll(core.TranscriptPageDown, now)
	case MoveTop:
		return m.scroll(core.TranscriptTop, now)
	case MoveBottom:
		return m.scroll(core.TranscriptBottom, now)
	case MoveEnter, MoveType:
		return core.Type{At: now}
	case MoveJump:
		return core.FocusIndex{At: now, Index: index}
	case MoveEnd:
		return core.AskEnd{At: now}
	case MoveTools:
		return core.ShowTools{At: now}
	case MoveThinking:
		return core.ShowThinking{At: now}
	case MoveHelp:
		return core.ToggleHelp{At: now}
	case MoveInterrupt:
		return core.Interrupt{At: now, InputEmpty: true}
	case MoveEOF:
		if m.input.Value() == "" {
			return core.EOF{At: now}
		}
	case MoveNone:
		if printable(k) {
			return core.Unbound{At: now}
		}
	}
	return nil
}

// singleConvKey is the event for a key pressed while moving in the
// conversation against a single agent. The conversation is printed into
// the terminal's scrollback, so the terminal scrolls it, and ctrl+t or
// ctrl+o opens the transcript.
func (m *Model) singleConvKey(k tea.KeyPressMsg, a MoveAction, now time.Time) core.Event {
	switch a {
	case MoveLeft:
		return core.Move{At: now, Delta: -1} // the edge: nothing
	case MoveRight:
		return core.Move{At: now, Delta: 1}
	case MoveEnter, MoveType:
		return core.Type{At: now}
	case MoveTools:
		return core.ShowTools{At: now}
	case MoveThinking:
		return core.ShowThinking{At: now}
	case MoveHelp:
		return core.ToggleHelp{At: now}
	case MoveInterrupt:
		return core.Interrupt{At: now, InputEmpty: true}
	case MoveEOF:
		if m.input.Value() == "" {
			return core.EOF{At: now}
		}
	default:
		if printable(k) {
			return core.Unbound{At: now}
		}
	}
	return nil
}

// editorKey is the event for a key pressed while the board's one-line
// editor is open: it types, deletes, sends or cancels, and nothing else.
// Ctrl-C stops a running turn, and otherwise cancels the editor.
func (m *Model) editorKey(k tea.KeyPressMsg, now time.Time) core.Event {
	if k.String() == "ctrl+c" {
		return m.paneCtrlC(now)
	}
	a, text := ClassifyBoardInput(k)
	return core.PaneKey{At: now, Action: a, Text: text}
}

// confirmKey is the event for a key pressed while the pane asks
// something (archive a card, trust a card or a doc): every key goes to
// the pane, so the next one answers or cancels the question and does
// nothing else.
func (m *Model) confirmKey(k tea.KeyPressMsg, now time.Time) core.Event {
	if k.String() == "ctrl+c" {
		return m.paneCtrlC(now)
	}
	return paneEvent(m.app(), m.paneState(), k, now, func() int { return m.paneDetailMax(now) })
}

// paneCtrlC is Ctrl-C while the pane's editor or question is open: it
// stops a running turn, else cancels what is open.
func (m *Model) paneCtrlC(now time.Time) core.Event {
	if m.paneState().Phase == core.Busy {
		return core.Interrupt{At: now, InputEmpty: true}
	}
	return core.PaneKey{At: now, Action: core.PaneEsc}
}

// paneState is the conversation the pane is drawn with: the one on
// screen, or none.
func (m Model) paneState() core.State {
	if m.kind == kindHub {
		return m.focusedState()
	}
	return m.one.State
}

// listKey is the event for a key pressed while moving in the list of
// roles.
func (m *Model) listKey(k tea.KeyPressMsg, a MoveAction, index int, now time.Time) core.Event {
	switch a {
	case MoveUp:
		return core.ListMove{At: now, Delta: -1}
	case MoveDown:
		return core.ListMove{At: now, Delta: 1}
	case MoveEnter:
		return core.ListOpen{At: now}
	case MoveRight:
		return core.Move{At: now, Delta: 1}
	case MoveLeft:
		return core.Move{At: now, Delta: -1} // the edge: nothing
	case MoveEsc:
		if m.hub.Focus != "" {
			return core.Move{At: now, Delta: 1} // back to the conversation
		}
	case MoveType:
		return core.Type{At: now}
	case MoveJump:
		return core.FocusIndex{At: now, Index: index}
	case MoveHelp:
		return core.ToggleHelp{At: now}
	case MoveInterrupt:
		return core.Interrupt{At: now, InputEmpty: true}
	case MoveEOF:
		return core.EOF{At: now}
	case MoveNone:
		if printable(k) {
			return core.Unbound{At: now}
		}
	}
	return nil
}

// hubScroll is the event for a page key, which scrolls the conversation
// whatever has the keys in it, or nil for any other key.
func (m *Model) hubScroll(k tea.KeyPressMsg, now time.Time) core.Event {
	switch k.String() {
	case "pgup":
		return m.scroll(core.TranscriptPageUp, now)
	case "pgdown":
		return m.scroll(core.TranscriptPageDown, now)
	}
	return nil
}

// typingKey turns a key pressed while typing into events, or gives it to
// the input box. It serves both modes: against a single agent there is
// no role to jump to and no conversation to start.
func (m *Model) typingKey(k tea.KeyPressMsg, now time.Time) ([]core.Event, tea.Cmd) {
	one := func(e core.Event) ([]core.Event, tea.Cmd) { return []core.Event{e}, nil }
	empty := m.input.Value() == ""
	a, index := Classify(k, empty)
	switch a {
	case Send:
		text := m.input.Value()
		if isBlank(text) {
			if m.kind == kindHub && m.hub.Focus != "" && m.hub.Focused() == nil {
				return one(core.StartConversation{At: now})
			}
			return nil, nil
		}
		m.input.Reset()
		return one(core.Submit{At: now, Text: text})
	case Newline:
		m.input.InsertString("\n")
		return nil, nil
	case Interrupt:
		idle := m.one.State.Phase == core.Idle
		if m.kind == kindHub {
			idle = m.hub.Focused() == nil || m.focusedState().Phase == core.Idle
		}
		if idle && !empty {
			m.input.Reset() // like a shell: Ctrl-C clears the line
		}
		return one(core.Interrupt{At: now, InputEmpty: empty})
	case EOF:
		return one(core.EOF{At: now})
	case ShowTools:
		return one(core.ShowTools{At: now})
	case ShowThinking:
		return one(core.ShowThinking{At: now})
	case Help:
		return one(core.ToggleHelp{At: now})
	case TrustPaste:
		return one(core.TrustPaste{At: now, Draft: m.input.Value()})
	case StopTyping:
		return one(core.StopTyping{At: now})
	case Jump:
		if m.kind == kindHub {
			return one(core.FocusIndex{At: now, Index: index})
		}
		return nil, nil
	}
	if m.kind == kindHub {
		if e := m.hubScroll(k, now); e != nil {
			return one(e)
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(k)
	return nil, cmd
}

// paneMoving turns a key pressed while moving in the pane into events:
// the keys that move between columns and roles, then the pane's own. Its
// editor and its questions are surfaces of their own (editorKey,
// confirmKey).
func (m *Model) paneMoving(k tea.KeyPressMsg, a MoveAction, index int, now time.Time) ([]core.Event, tea.Cmd) {
	one := func(e core.Event) ([]core.Event, tea.Cmd) { return []core.Event{e}, nil }
	app := m.app()
	if a.Global() && !((a == MoveLeft || a == MoveRight) && core.PaneSideKeys(app)) {
		switch a {
		case MoveLeft:
			return one(core.Move{At: now, Delta: -1})
		case MoveRight:
			return one(core.Move{At: now, Delta: 1})
		case MoveType:
			return one(core.Type{At: now})
		case MoveJump:
			if m.kind == kindHub {
				return one(core.FocusIndex{At: now, Index: index})
			}
			return nil, nil
		case MoveHelp:
			return one(core.ToggleHelp{At: now})
		case MoveInterrupt:
			return one(core.Interrupt{At: now, InputEmpty: true})
		case MoveEOF:
			if m.input.Value() == "" {
				return one(core.EOF{At: now})
			}
			return nil, nil
		}
	}
	e := paneEvent(app, m.paneState(), k, now, func() int { return m.paneDetailMax(now) })
	if e.Action == core.PaneOther && printable(k) {
		return []core.Event{e, core.Unbound{At: now}}, nil
	}
	return one(e)
}

// app is the connection's state, in either mode.
func (m Model) app() core.App {
	if m.kind == kindHub {
		return m.hub.App
	}
	return m.one.App
}

// paneDetailMax is how far the pane's detail view scrolls, as drawn.
func (m Model) paneDetailMax(now time.Time) int {
	if m.kind == kindHub {
		return m.hubFrame(now).DetailMax
	}
	return m.frame(now).DetailMax
}

func isBlank(s string) bool {
	for _, r := range s {
		if r != ' ' && r != '\t' && r != '\n' && r != '\r' {
			return false
		}
	}
	return true
}

// scroll is the event that moves the conversation as drawn.
func (m *Model) scroll(a core.TranscriptAction, now time.Time) core.Event {
	f := m.hubFrame(now)
	page := f.Page
	return core.TranscriptKey{At: now, Action: a, Max: f.Max, Page: page}
}

// hubWheel scrolls the conversation three lines at a time.
func (m *Model) hubWheel(w tea.MouseWheelMsg, now time.Time) []core.Event {
	a := core.TranscriptPageUp
	switch w.Button {
	case tea.MouseWheelUp:
	case tea.MouseWheelDown:
		a = core.TranscriptPageDown
	default:
		return nil
	}
	return []core.Event{core.TranscriptKey{At: now, Action: a, Max: m.hubFrame(now).Max, Page: 3}}
}

// hubTick keeps the status line moving while a turn runs in the
// conversation on screen, and is quiet otherwise.
func (m *Model) hubTick(now time.Time) tea.Cmd {
	if m.ticking || m.hub.Gone {
		return nil
	}
	busy := false
	st := m.focusedState()
	if c := m.hub.Focused(); c != nil && !m.hub.List {
		busy = st.Phase == core.Busy || st.Phase == core.Starting
		if r := m.hub.RoleByID(m.hub.Focus); r != nil && r.Conv != nil && r.Conv.Turn == core.TurnRunning {
			busy = true
		}
	}
	// A card's trust arms with no conversation open, too.
	if since, ok := core.TrustSince(m.hub.App, st); ok && now.Sub(since) < core.ArmDelay+tickEvery {
		busy = true
	}
	if !busy {
		return nil
	}
	m.ticking = true
	return tea.Tick(tickEvery, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) hubFrame(now time.Time) render.HubFrame {
	return render.HubView(m.hub, m.input.View(), NewlineHint(m.caps), now, m.width(), max(m.h, 2))
}

func (m Model) hubView(now time.Time) tea.View {
	f := m.hubFrame(now)
	v := tea.NewView(f.Content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	if f.Cursor && core.HubSurface(m.hub).Kind == core.SurfaceInput {
		if c := m.input.Cursor(); c != nil {
			c.Position.X += f.InputX
			c.Position.Y += f.InputY
			v.Cursor = c
		}
	}
	return v
}
