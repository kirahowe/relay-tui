package ui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kirahowe/relay-tui/internal/core"
)

// The saved view against the real model: what the operator types before
// a restore has come back, and what is saved meanwhile.

func update(m Model, msgs ...tea.Msg) Model {
	for _, msg := range msgs {
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

func ev(e core.Event) tea.Msg { return EventMsg{Event: e} }

func letter(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }

var restoreRoles = []core.Role{{ID: "dev", Title: "Dev"}, {ID: "mail", Title: "Mail"}}

// A letter the input box takes for itself is a key like any other: the
// layout saved with the pane's keys does not come back over it, so the
// next letter cannot reach the board.
func TestTypingBeforeTheRestoreKeepsTheKeys(t *testing.T) {
	now := time.Now()
	saved := core.View{PaneFocus: true}
	m := update(New(nopConn{}, "").Restore(saved, ""), letter('n'), ev(core.Connected{At: now}))
	if m.one.App.PaneFocus {
		t.Fatalf("single: the pane took the keys after a letter was typed")
	}
	if got := m.input.Value(); got != "n" {
		t.Fatalf("single: input %q", got)
	}
	saved = core.View{Hub: &core.HubView{Focus: "mail", Cursor: "mail"}, PaneFocus: true}
	// In hub mode the list has the keys until the roster comes, and takes
	// no letters; the letter still counts.
	m = update(New(nopConn{}, "").Restore(saved, ""), ev(core.HubConnected{At: now}), letter('n'),
		ev(core.RosterUpdate{At: now, Roles: restoreRoles}))
	if m.hub.App.PaneFocus || m.hub.Focus != "" || !m.hub.List {
		t.Fatalf("hub: pane %v focus %q list %v", m.hub.App.PaneFocus, m.hub.Focus, m.hub.List)
	}
}
