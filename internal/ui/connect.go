package ui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kirahowe/relay-tui/internal/core"
	"github.com/kirahowe/relay-tui/internal/render"
)

// Input waits until the screen it lands on is known. Until the agent
// has said what it is, the shell does not know whether it is relay's hub
// or a plain ACP agent, and the two have different states; and until the
// saved view has been put back, it does not know which screen a key
// would act on. So it holds the operator's input (keys and pastes, in
// order, with their times) and handles none of it, with "connecting…" on
// the alternate screen, which leaves nothing behind.
//
// Against a plain agent the screen is known once its session exists
// (Connected): the shell builds the single state, puts back the saved
// view, and replays. Against relay's hub it builds the hub's state on
// HubConnected, and replays once the first roster has come and the saved
// layout has been put back (or found nothing to put back); a roster that
// fails releases the input onto the default screen, with a note. The
// card may still come back later, when the board does: a replayed key
// cancels only that part, as a live one does. Replayed keys go through
// the normal path, each at the time it was pressed, so every guard a
// live key meets applies to them. If the agent goes away first, nothing
// was handled, so nothing typed can be half moved between states, and
// the saved view is left as it was.

// maxHeld is how many keys and pastes are held while connecting. Past
// it more are dropped, and the status line says how many once they are
// replayed.
const maxHeld = 4096

// heldMsg is a key or a paste held while connecting, and when it came.
type heldMsg struct {
	msg tea.Msg
	at  time.Time
}

// connecting handles a message before the agent has said what it is.
func (m Model) connecting(msg tea.Msg) (tea.Model, tea.Cmd) {
	now := m.now()
	switch msg := msg.(type) {
	case tea.KeyPressMsg, tea.PasteMsg:
		return m.holdOrQuit(msg, now)
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.size = &msg
	case tea.KeyboardEnhancementsMsg:
		m.caps0 = &msg
	case EventMsg:
		switch e := msg.Event.(type) {
		case core.HubConnected:
			return m.known(kindHub, msg)
		case core.Connected:
			return m.known(kindSingle, msg)
		case core.AgentExited:
			m.died, m.exitWhy, m.quitting = true, e.Err, true
			return m, m.stop()
		}
	}
	return m, nil
}

// holdOrQuit holds a key or a paste. Ctrl-C is the one key handled at
// once: it quits, saving nothing, so an agent that never answers cannot
// trap the operator.
func (m Model) holdOrQuit(msg tea.Msg, at time.Time) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok && k.String() == "ctrl+c" {
		m.quitting = true
		return m, m.stop()
	}
	if len(m.held) >= maxHeld {
		m.dropped++
	} else {
		m.held = append(m.held, heldMsg{msg, at})
	}
	return m, nil
}

// known builds the state for the agent's kind, puts the saved view to
// it, and handles the terminal's word and then e, the event that said
// the kind. Against a plain agent the screen is then known, and what was
// held is replayed; against the hub it waits for the roster (see
// settled).
func (m Model) known(kind agentKind, e EventMsg) (tea.Model, tea.Cmd) {
	m.kind, m.holding = kind, true
	if kind == kindHub {
		m.hub = core.RestoreHub(core.NewHub(m.open), m.saved)
		m.hub.Notice = m.savedNote
	} else {
		m.one = core.RestoreSingle(m.one, m.saved)
		if m.savedNote != "" {
			m.one.State.Message = core.Message{Text: m.savedNote, At: m.now()}
		}
	}
	var cmds []tea.Cmd
	for _, msg := range []tea.Msg{m.size, m.caps0, e} {
		switch msg := msg.(type) {
		case *tea.WindowSizeMsg:
			if msg == nil {
				continue
			}
			next, cmd := m.update(*msg)
			m, cmds = next.(Model), append(cmds, cmd)
		case *tea.KeyboardEnhancementsMsg:
			if msg == nil {
				continue
			}
			next, cmd := m.update(*msg)
			m, cmds = next.(Model), append(cmds, cmd)
		default:
			next, cmd := m.update(msg)
			m, cmds = next.(Model), append(cmds, cmd)
		}
	}
	m.size, m.caps0 = nil, nil
	next, cmd := m.settled(e)
	return next, tea.Batch(append(cmds, cmd)...)
}

// settled replays what was held once the screen it lands on is known: at
// once against a plain agent, and against the hub once the first roster
// has been answered (msg is the message just handled).
func (m Model) settled(msg tea.Msg) (tea.Model, tea.Cmd) {
	if !m.holding {
		return m, nil
	}
	failed := ""
	if m.kind == kindHub {
		em, _ := msg.(EventMsg)
		ru, isRoster := em.Event.(core.RosterUpdate)
		switch {
		case m.hub.RosterLoaded:
		case isRoster && ru.Err != "":
			failed = m.hub.Notice // "no roster: …"
		default:
			return m, nil
		}
	}
	m.holding = false
	m.seen = m.SavedView()
	var cmds []tea.Cmd
	held := m.held
	m.held = nil
	m.replaying = true
	for _, h := range held {
		m.at = h.at
		next, cmd := m.Update(h.msg)
		m, cmds = next.(Model), append(cmds, cmd)
	}
	m.at, m.replaying = time.Time{}, false
	note := failed
	if failed != "" && len(held) > 0 {
		note = failed + "; what you typed went to the list"
	}
	if m.dropped > 0 {
		note = fmt.Sprintf("%d keys typed while connecting were dropped", m.dropped)
	}
	if note != "" {
		if m.kind == kindHub {
			m.hub.Notice = note
		} else {
			m.one.State.Message = core.Message{Text: note, At: m.now()}
		}
	}
	m.tellView()
	return m, tea.Batch(cmds...)
}

// connectingLine is the screen while input is held: a quiet word, and
// how many keys wait.
func connectingLine(held, width int) string {
	s := "  connecting…"
	if held > 0 {
		s += fmt.Sprintf(" · %d typed, kept for when it answers", held)
	}
	return render.Faint(s, width)
}
