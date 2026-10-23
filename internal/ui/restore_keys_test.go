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

// A letter typed before the saved draft is back does not push the saved
// draft out of the next save.
func TestSavedDraftSurvivesTypingBeforeTheRestore(t *testing.T) {
	saved := core.View{Drafts: []core.Draft{{Text: "old draft"}}}
	m := update(New(nopConn{}, "").Restore(saved, ""), letter('n'))
	if d := m.SavedView().Drafts; len(d) != 1 || d[0].Text != "old draft\nn" {
		t.Fatalf("single: saved %+v", d)
	}
	saved = core.View{Drafts: []core.Draft{{Role: "mail", Text: "old draft"}}}
	m = update(New(nopConn{}, "").Open("mail").Restore(saved, ""), ev(core.HubConnected{At: time.Now()}), letter('n'))
	if d := m.SavedView().Drafts; len(d) != 1 || d[0].Role != "mail" || d[0].Text != "old draft\nn" {
		t.Fatalf("hub: saved %+v", d)
	}
}

// The model starts as if against a single agent; a saved hub layout
// with nothing else in it still comes back once the agent is the hub.
func TestSavedHubFocusComesBackAlone(t *testing.T) {
	now := time.Now()
	saved := core.View{Hub: &core.HubView{Focus: "mail", Cursor: "mail"}}
	m := update(New(nopConn{}, "").Restore(saved, ""), ev(core.HubConnected{At: now}), ev(core.RosterUpdate{At: now, Roles: restoreRoles}))
	if m.hub.Focus != "mail" || m.hub.List {
		t.Fatalf("focus %q list %v", m.hub.Focus, m.hub.List)
	}
}

// pastesOf is the pastes the hub holds for a role.
func pastesOf(m Model, role string) []core.Paste {
	if c := m.hub.ConvOf(role); c != nil {
		return c.State.Pastes
	}
	for _, b := range m.hub.Blanks {
		if b.Role == role {
			return b.State.Pastes
		}
	}
	return nil
}

func typeText(m Model, s string) Model {
	for _, r := range s {
		m = update(m, letter(r))
	}
	return m
}

// What is typed and pasted before the client knows the agent is the hub
// is the focused role's draft once it does, pastes and their trust
// included, after the saved draft and ahead of what is typed since.
func TestTypingBeforeTheHubIsKnownIsKept(t *testing.T) {
	now := time.Now()
	roles := []core.Role{{ID: "dev", Title: "Dev"}, {ID: "mail", Title: "Mail"}}
	saved := core.View{Drafts: []core.Draft{{Role: "mail", Text: "old draft"}}}
	m := update(New(nopConn{}, "").Open("mail").Restore(saved, ""), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = typeText(m, "new ")
	m = update(m, tea.PasteMsg{Content: "my own words"}, tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl})
	m = update(m, ev(core.HubConnected{At: now}))
	m = typeText(m, " more")
	m = update(m, ev(core.RosterUpdate{At: now, Roles: roles}))
	want := "old draft\nnew [pasted 1 line · trusted #1] more"
	if m.hub.Focus != "mail" || m.input.Value() != want {
		t.Fatalf("focus %q input %q, want %q", m.hub.Focus, m.input.Value(), want)
	}
	ps := pastesOf(m, "mail")
	if len(ps) != 1 || ps[0].Text != "my own words" || !ps[0].Trusted {
		t.Fatalf("pastes %+v", ps)
	}

	// With no role named, the draft waits for the first role in focus,
	// and a save meanwhile keeps it.
	m = update(New(nopConn{}, ""), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = typeText(m, "for whoever ")
	m = update(m, tea.PasteMsg{Content: "an email"})
	m = update(m, ev(core.HubConnected{At: now}), ev(core.RosterUpdate{At: now, Roles: roles}))
	v := m.SavedView()
	if len(v.Drafts) != 1 || v.Drafts[0].Role != "" || len(v.Drafts[0].Pastes) != 1 {
		t.Fatalf("saved %+v", v.Drafts)
	}
	m = update(m, letter('2'))
	if m.input.Value() != "for whoever [pasted 1 line · untrusted #1]" || len(pastesOf(m, "mail")) != 1 {
		t.Fatalf("mail: input %q pastes %+v", m.input.Value(), pastesOf(m, "mail"))
	}
	// And so does a reopen with that save.
	m = update(New(nopConn{}, "").Restore(v, ""), tea.WindowSizeMsg{Width: 120, Height: 30},
		ev(core.HubConnected{At: now}), ev(core.RosterUpdate{At: now, Roles: roles}), letter('1'))
	if m.input.Value() != "for whoever [pasted 1 line · untrusted #1]" || len(pastesOf(m, "dev")) != 1 || pastesOf(m, "dev")[0].Trusted {
		t.Fatalf("reopened: input %q pastes %+v", m.input.Value(), pastesOf(m, "dev"))
	}
}
