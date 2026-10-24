package ui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kirahowe/relay-tui/internal/core"
	"github.com/kirahowe/relay-tui/internal/render"
)

// Until initialize has answered, the shell does not know whether the
// agent is relay's hub or a plain ACP agent, and the two have different
// states. So it builds neither: it holds the operator's input (keys and
// pastes, in order, with their times) and handles none of it. Once the
// agent says what it is, the shell builds that kind's state, gives it
// the saved view for that kind, and replays what it held through the
// normal path, each key at the time it was pressed, so every guard a
// live key meets applies to it. If the agent goes away first, nothing
// was handled, so nothing typed can be half moved from one kind's state
// to the other's, and the saved view is left as it was.

// maxHeld is how many keys and pastes are held while connecting. Past
// it more are dropped, and the status line says how many once the agent
// has answered.
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
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			// The one key handled now: an agent that never answers must not
			// trap the operator. Nothing is saved.
			m.quitting = true
			return m, m.stop()
		}
		m.hold(msg, now)
	case tea.PasteMsg:
		m.hold(msg, now)
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.size = &msg
	case tea.KeyboardEnhancementsMsg:
		m.caps0 = &msg
	case EventMsg:
		switch e := msg.Event.(type) {
		case core.HubConnected:
			return m.known(kindHub, msg)
		case core.Initialized, core.Connected:
			return m.known(kindSingle, msg)
		case core.AgentExited:
			m.died, m.exitWhy, m.quitting = true, e.Err, true
			return m, m.stop()
		}
	}
	return m, nil
}

func (m *Model) hold(msg tea.Msg, at time.Time) {
	if len(m.held) >= maxHeld {
		m.dropped++
		return
	}
	m.held = append(m.held, heldMsg{msg, at})
}

// known builds the state for the agent's kind, puts the saved view to
// it, and handles first the terminal's word, then e, the event that
// said the kind, and then what was held, in order.
func (m Model) known(kind agentKind, e EventMsg) (tea.Model, tea.Cmd) {
	now := m.now()
	m.kind = kind
	note := m.savedNote
	if m.dropped > 0 {
		note = fmt.Sprintf("%d keys typed while connecting were dropped", m.dropped)
	}
	if kind == kindHub {
		m.hub = core.RestoreHub(core.NewHub(m.open), m.saved)
		m.hub.Notice = note
	} else {
		m.one = core.RestoreSingle(m.one, m.saved)
		if note != "" {
			m.one.State.Message = core.Message{Text: note, At: now}
		}
	}
	m.seen = m.SavedView()
	var cmds []tea.Cmd
	step := func(msg tea.Msg) {
		next, cmd := m.Update(msg)
		m = next.(Model)
		cmds = append(cmds, cmd)
	}
	if m.size != nil {
		step(*m.size)
	}
	if m.caps0 != nil {
		step(*m.caps0)
	}
	step(e)
	held := m.held
	m.held, m.size, m.caps0 = nil, nil, nil
	for _, h := range held {
		m.at = h.at
		step(h.msg)
	}
	m.at = time.Time{}
	return m, tea.Batch(cmds...)
}

// connectingLine is the screen while the agent has not answered: a quiet
// word, and how many keys wait.
func connectingLine(held, width int) string {
	s := "  connecting…"
	if held > 0 {
		s += fmt.Sprintf(" · %d typed, kept for when it answers", held)
	}
	return render.Faint(s, width)
}
