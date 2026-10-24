package ui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"pgregory.net/rapid"

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

// A key typed while connecting waits until the screen it lands on is
// known, so it does not cancel the restore: it acts on the restored
// screen. Against a plain agent that is once the session exists; against
// the hub, once the first roster has come. A key pressed after that,
// before the board has come, still cancels the card's part.
func TestTypingWhileConnectingLandsOnTheRestoredScreen(t *testing.T) {
	now := time.Now()
	saved := core.View{PaneFocus: true}
	m := update(New(nopConn{}, "").Restore(saved, ""), letter('n'), ev(core.Connected{At: now}))
	if !m.one.App.PaneFocus || m.input.Value() != "" || m.one.State.Message.Text != core.UnboundNote {
		t.Fatalf("single: pane %v input %q message %q", m.one.App.PaneFocus, m.input.Value(), m.one.State.Message.Text)
	}
	saved = core.View{Hub: &core.HubView{Focus: "mail", Cursor: "mail"}, PaneFocus: true, Card: 1, Detail: true}
	m = update(New(nopConn{}, "").Restore(saved, ""), ev(core.HubConnected{At: now}), letter('n'))
	if m.Known() || len(m.held) != 1 {
		t.Fatalf("hub: the letter was handled before the roster: known %v held %d", m.Known(), len(m.held))
	}
	m = update(m, ev(core.RosterUpdate{At: now, Roles: restoreRoles}))
	if !m.hub.App.PaneFocus || m.hub.Focus != "mail" || m.hub.Notice != core.UnboundNote {
		t.Fatalf("hub: pane %v focus %q notice %q", m.hub.App.PaneFocus, m.hub.Focus, m.hub.Notice)
	}
	// The letter came after the roster but before the board: the card is
	// not put back over it.
	m = update(m, ev(core.CardsLoaded{At: now, Rev: 2, Cards: navCards()}))
	if m.hub.App.Board.Detail {
		t.Fatal("hub: the card came back over a key")
	}
	// A roster that fails releases what was held onto the default
	// screen, with a note.
	m = update(New(nopConn{}, "").Open("mail").Restore(saved, ""), ev(core.HubConnected{At: now}), letter('n'))
	m = update(m, ev(core.RosterUpdate{At: now, Err: "boom"}))
	if !m.Known() || m.input.Value() != "n" || !strings.Contains(m.hub.Notice, "no roster: boom") {
		t.Fatalf("failed roster: known %v input %q notice %q", m.Known(), m.input.Value(), m.hub.Notice)
	}
}

// A letter typed while connecting does not push the saved draft out of
// the next save, and nothing is saved before the screen is known.
func TestSavedDraftSurvivesTypingBeforeTheRestore(t *testing.T) {
	saved := core.View{Drafts: []core.Draft{{Text: "old draft"}}}
	m := update(New(nopConn{}, "").Restore(saved, ""), letter('n'))
	// Nothing is handled before the agent answers: the view is as loaded.
	if v := m.SavedView(); m.Known() || !reflect.DeepEqual(v, saved) {
		t.Fatalf("before initialize: known %v saved %+v", m.Known(), v)
	}
	// Replayed onto the restored screen, the letter goes on where the
	// saved draft ends, as one typed just then would.
	m = update(m, ev(core.Connected{At: time.Now()}))
	if d := m.SavedView().Drafts; len(d) != 1 || d[0].Text != "old draftn" {
		t.Fatalf("single: saved %+v", d)
	}
	saved = core.View{Drafts: []core.Draft{{Role: "mail", Text: "old draft"}}}
	m = update(New(nopConn{}, "").Open("mail").Restore(saved, ""), ev(core.HubConnected{At: time.Now()}),
		ev(core.RosterUpdate{At: time.Now(), Roles: restoreRoles}), letter('n'))
	if d := m.SavedView().Drafts; len(d) != 1 || d[0].Role != "mail" || d[0].Text != "old draftn" {
		t.Fatalf("hub: saved %+v", d)
	}
}

// A saved hub layout with nothing else in it comes back once the agent
// is the hub.
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

// What is typed and pasted before the screen is known is held, and
// replayed once the roster has come, through the normal path: with
// --open it lands in that role's input, pastes and their trust
// included, on after the saved draft that came back with the roster.
func TestTypingBeforeTheHubIsKnownIsReplayed(t *testing.T) {
	now := time.Now()
	roles := []core.Role{{ID: "dev", Title: "Dev"}, {ID: "mail", Title: "Mail"}}
	saved := core.View{Drafts: []core.Draft{{Role: "mail", Text: "old draft"}}}
	m := update(New(nopConn{}, "").Open("mail").Restore(saved, ""), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = typeText(m, "new ")
	m = update(m, tea.PasteMsg{Content: "my own words"}, tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl})
	if m.input.Value() != "" || len(m.held) != 6 {
		t.Fatalf("handled before the agent answered: input %q held %d", m.input.Value(), len(m.held))
	}
	m = update(m, ev(core.HubConnected{At: now}))
	m = typeText(m, " more")
	m = update(m, ev(core.RosterUpdate{At: now, Roles: roles}))
	want := "old draftnew [pasted 1 line · trusted #1] more" // typed on where the saved draft ends
	if m.hub.Focus != "mail" || m.input.Value() != want {
		t.Fatalf("focus %q input %q, want %q", m.hub.Focus, m.input.Value(), want)
	}
	ps := pastesOf(m, "mail")
	if len(ps) != 1 || ps[0].Text != "my own words" || !ps[0].Trusted {
		t.Fatalf("pastes %+v", ps)
	}

	// With no role named the hub starts on the list, moving, so the
	// replayed letters are the list's keys there, as if typed then: none
	// is typed into anything, and each that does nothing says how to
	// type. Nothing is saved for no role.
	m = update(New(nopConn{}, ""), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = typeText(m, "for whoever ")
	m = update(m, tea.PasteMsg{Content: "an email"})
	m = update(m, ev(core.HubConnected{At: now}), ev(core.RosterUpdate{At: now, Roles: roles}))
	if len(drafts(m)) != 0 || m.hub.Notice != core.UnboundNote || m.hub.Focus != "" {
		t.Fatalf("drafts %q notice %q focus %q", drafts(m), m.hub.Notice, m.hub.Focus)
	}
	for _, d := range m.SavedView().Drafts {
		if d.Role == "" {
			t.Fatalf("a draft saved for no role: %+v", d)
		}
	}
}

// Codex round 2, finding 3: enter before the agent has said it is the
// hub. With --open the held text and enter replay into that role's
// conversation, which sends it once its attach is answered.
func TestEnterBeforeTheHubIsKnownIsSent(t *testing.T) {
	now := time.Now()
	var calls []string
	m := update(New(recConn{calls: &calls}, "").Open("dev"), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = typeText(m, "hi")
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m = update(m, ev(core.HubConnected{At: now}), ev(core.RosterUpdate{At: now, Roles: restoreRoles}))
	if c := m.hub.ConvOf("dev"); c == nil || len(c.State.Queue) != 1 || c.State.Queue[0] != "hi" {
		t.Fatalf("not queued: %+v", m.hub.Convs)
	}
	m = update(m, ev(core.Attached{At: now, Role: "dev", Session: "s1"}))
	if len(calls) != 1 || calls[0] != "prompt" || m.input.Value() != "" {
		t.Fatalf("calls %v input %q", calls, m.input.Value())
	}
}

// Codex round 2, finding 4: a paste with --open goes to the role's
// conversation while its attach is on its way; when relay's replay makes
// the conversation first, the attach's answer folds the waiting one into
// it whole: the paste's text and the paste counter go too, so the
// placeholder still sends its text and the next paste takes a new
// number.
func TestAPasteBeforeTheAttachSurvivesAReplayThatCameFirst(t *testing.T) {
	now := time.Now()
	m := update(New(nopConn{}, "").Open("dev"), tea.WindowSizeMsg{Width: 120, Height: 30},
		tea.PasteMsg{Content: "an email"}, ev(core.HubConnected{At: now, Embedded: true}), ev(core.RosterUpdate{At: now, Roles: restoreRoles}))
	tok := m.input.Value()
	m = update(m, ev(core.Routed{Session: "s1", Event: core.TextChunk{At: now, Text: "earlier"}}),
		ev(core.Attached{At: now, Role: "dev", Session: "s1"}))
	if len(m.hub.Convs) != 1 {
		t.Fatalf("convs %+v", m.hub.Convs)
	}
	m = update(m, tea.PasteMsg{Content: "another"})
	ps := pastesOf(m, "dev")
	if len(ps) != 2 || ps[0].Text != "an email" || ps[1].ID != 2 || m.input.Value() != tok+"[pasted 1 line · untrusted #2]" {
		t.Fatalf("pastes %+v input %q", ps, m.input.Value())
	}
}

// A replayed key meets every guard a live one does: held keys replay at
// the times they were pressed, before any request can exist, so a y or
// an enter typed while connecting answers nothing, now or later.
func TestAReplayedKeyAnswersNothing(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		clock := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
		var calls []call
		m := New(callConn{calls: &calls, clock: &clock}, "").Open("dev")
		m.clock = func() time.Time { return clock }
		m = update(m, tea.WindowSizeMsg{Width: 140, Height: 30})
		for range rapid.IntRange(0, 20).Draw(rt, "held") {
			m = update(m, key(rapid.SampledFrom([]string{"y", "Y", "enter", "esc", "j", "tab", "a"}).Draw(rt, "key")))
		}
		clock = clock.Add(time.Duration(rapid.IntRange(0, 2000).Draw(rt, "ms")) * time.Millisecond)
		ask := core.Routed{Session: "s1", Event: askEvent(clock, 1)}
		m = update(m, ev(core.HubConnected{At: clock}), ev(core.RosterUpdate{At: clock, Roles: restoreRoles}),
			ev(core.Attached{At: clock, Role: "dev", Session: "s1"}), ev(ask))
		for _, c := range calls {
			if c.what == "answer" {
				rt.Fatalf("answered: %+v", calls)
			}
		}
	})
}

// While connecting, the screen says so, on the alternate screen so it
// leaves nothing behind, and how much is held; past
// maxHeld keys more are dropped, and the status line says how many once
// the agent answers.
func TestHeldInputIsBounded(t *testing.T) {
	m := update(New(nopConn{}, ""), tea.WindowSizeMsg{Width: 80, Height: 20})
	if v := m.View(); !strings.Contains(v.Content, "connecting…") || !v.AltScreen {
		t.Fatalf("screen %q alt %v", v.Content, v.AltScreen)
	}
	for range maxHeld + 3 {
		m = update(m, letter('a'))
	}
	if len(m.held) != maxHeld || !strings.Contains(m.View().Content, fmt.Sprint(maxHeld, " typed")) {
		t.Fatalf("held %d, screen %q", len(m.held), m.View().Content)
	}
	m = update(m, ev(core.Connected{At: time.Now()}))
	if got := m.input.Value(); len(got) != maxHeld || m.one.State.Message.Text != "3 keys typed while connecting were dropped" {
		t.Fatalf("input %d long, message %q", len(got), m.one.State.Message.Text)
	}
}
