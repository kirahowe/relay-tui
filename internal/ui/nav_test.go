package ui

import (
	"reflect"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"pgregory.net/rapid"

	"github.com/kirahowe/relay-tui/internal/core"
)

// Typing and moving, through the real model: which keys reach the input,
// the board and a request.

// recConn records the calls that act: a card op, a prompt.
type recConn struct {
	nopConn
	calls *[]string
}

func (c recConn) CardOp(int64, core.CardOp) { *c.calls = append(*c.calls, "card-op") }
func (c recConn) Prompt(string, string)     { *c.calls = append(*c.calls, "prompt") }

func navCards() []core.Card {
	text := func(s string) core.CardText { return core.CardText{Text: s, Rev: 1} }
	return []core.Card{
		{ID: 1, Rev: 1, Kind: core.CardChecklist, Status: "active", Title: text("Trip"), Total: 2,
			Items: []core.CardElem{{ID: 1, CardText: text("pack")}, {ID: 2, CardText: text("go")}}},
		{ID: 2, Rev: 2, Kind: core.CardNote, Status: "active", Title: text("Note"), Note: &core.CardText{Text: "hello", Rev: 2}},
	}
}

// navHub is a hub model on Dev, typing, with the board loaded and Mail's
// conversation off screen.
func navHub(t *testing.T, calls *[]string) Model {
	t.Helper()
	now := time.Now()
	roles := []core.Role{
		{ID: "dev", Title: "Dev", Conv: &core.RosterConv{Session: "s1", Turn: core.TurnIdle}},
		{ID: "mail", Title: "Mail", Conv: &core.RosterConv{Session: "s2", Turn: core.TurnIdle}},
		{ID: "news", Title: "News"},
	}
	m := update(New(recConn{calls: calls}, "").Open("dev"), tea.WindowSizeMsg{Width: 170, Height: 40},
		ev(core.HubConnected{At: now}), ev(core.RosterUpdate{At: now, Roles: roles}),
		ev(core.Attached{At: now, Role: "dev", Session: "s1"}), ev(core.Attached{At: now, Role: "mail", Session: "s2"}),
		ev(core.CardsLoaded{At: now, Rev: 2, Cards: navCards()}))
	if m.hub.App.Moving || m.hub.Focus != "dev" {
		t.Fatalf("not typing on dev: moving %v focus %q", m.hub.App.Moving, m.hub.Focus)
	}
	return m
}

// Typing, no key reaches the board: not one. The keys that leave typing
// (esc) or move to another role are left out; everything else is fair
// game.
func TestTypingNeverReachesTheBoard(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		var calls []string
		m := navHub(t, &calls)
		for range rapid.IntRange(1, 30).Draw(rt, "keys") {
			k := genKey(rt)
			if a, _ := Classify(k, m.input.Value() == ""); a == StopTyping || a == Jump || a == Interrupt || a == EOF {
				continue
			}
			m = update(m, k)
			if m.hub.App.Moving || m.hub.App.PaneFocus || m.hub.List {
				rt.Fatalf("key %q left typing", k.String())
			}
			for _, c := range calls {
				if c != "prompt" {
					rt.Fatalf("key %q while typing made a %s", k.String(), c)
				}
			}
		}
	})
}

// drafts is every role's unsent text: the input's for the role it
// holds, and the drafts kept for the others.
func drafts(m Model) map[string]string {
	out := map[string]string{}
	for _, d := range m.hub.Drafts {
		if d.Text != "" {
			out[d.Role] = d.Text
		}
	}
	if v := m.input.Value(); v != "" {
		out[m.inputRole] = v
	} else {
		delete(out, m.inputRole)
	}
	return out
}

// Moving, no key types: every draft is as it was, whatever the keys do
// to the columns, the roles, the board or the help. And esc from typing
// keeps the draft. Against a single agent, the same.
func TestMovingNeverTypesAndEscKeepsTheDraft(t *testing.T) {
	check := func(rt *rapid.T, m Model, moving func(Model) bool) {
		for _, r := range "keep me" {
			m = update(m, letter(r))
		}
		m = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
		if !moving(m) || m.input.Value() != "keep me" {
			rt.Fatalf("esc: moving %v input %q", moving(m), m.input.Value())
		}
		for range rapid.IntRange(1, 40).Draw(rt, "keys") {
			k := genKey(rt)
			before, wasMoving := drafts(m), moving(m)
			m = update(m, k)
			if wasMoving || moving(m) { // moving, or esc out of typing
				if after := drafts(m); !reflect.DeepEqual(after, before) {
					rt.Fatalf("key %q (moving %v) changed the drafts from %q to %q", k.String(), wasMoving, before, after)
				}
			}
		}
	}
	rapid.Check(t, func(rt *rapid.T) {
		var calls []string
		check(rt, navHub(t, &calls), func(m Model) bool { return m.hub.App.Moving })
	})
	rapid.Check(t, func(rt *rapid.T) {
		now := time.Now()
		m := update(New(nopConn{}, ""), tea.WindowSizeMsg{Width: 140, Height: 30}, ev(core.Connected{At: now}),
			ev(core.CardsLoaded{At: now, Rev: 2, Cards: navCards()}))
		check(rt, m, func(m Model) bool { return m.one.App.Moving })
	})
}

// A letter that does nothing where the operator is says how to type,
// and goes nowhere.
func TestAnUnboundLetterSaysHowToType(t *testing.T) {
	var calls []string
	m := update(navHub(t, &calls), tea.KeyPressMsg{Code: tea.KeyEscape}, letter('w'))
	if m.hub.Notice != core.UnboundNote || m.input.Value() != "" {
		t.Fatalf("notice %q input %q", m.hub.Notice, m.input.Value())
	}
	// On the board too, where w is not one of its keys.
	m = update(m, letter('l'), letter('w'))
	if !m.hub.App.PaneFocus || m.hub.Notice != core.UnboundNote {
		t.Fatalf("pane %v notice %q", m.hub.App.PaneFocus, m.hub.Notice)
	}
	// A paste while moving goes nowhere either, and says so.
	m = update(m, tea.PasteMsg{Content: "pasted"})
	if m.hub.Notice != core.UnboundNote || m.input.Value() != "" {
		t.Fatalf("paste: notice %q input %q", m.hub.Notice, m.input.Value())
	}
}

// 1 jumps while moving and types while typing; alt+1 jumps in both,
// and neither changes the mode. Each role keeps its own draft.
func TestDigitsAndAltDigits(t *testing.T) {
	var calls []string
	m := update(navHub(t, &calls), letter('3'))
	if m.hub.Focus != "dev" || m.input.Value() != "3" {
		t.Fatalf("typing 3: focus %q input %q", m.hub.Focus, m.input.Value())
	}
	m = update(m, tea.KeyPressMsg{Code: '3', Mod: tea.ModAlt})
	if m.hub.Focus != "news" || m.hub.App.Moving || m.input.Value() != "" {
		t.Fatalf("alt+3 typing: focus %q moving %v input %q", m.hub.Focus, m.hub.App.Moving, m.input.Value())
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEscape}, letter('1'))
	if m.hub.Focus != "dev" || !m.hub.App.Moving || m.input.Value() != "3" {
		t.Fatalf("1 moving: focus %q moving %v input %q", m.hub.Focus, m.hub.App.Moving, m.input.Value())
	}
	m = update(m, tea.KeyPressMsg{Code: '3', Mod: tea.ModAlt})
	if m.hub.Focus != "news" || !m.hub.App.Moving {
		t.Fatalf("alt+3 moving: focus %q moving %v", m.hub.Focus, m.hub.App.Moving)
	}
}

// Archiving asks first, in the pane: X shows the question, and a y right
// behind it answers nothing (the key may have been meant for what was
// there before). Once the question has been up a moment, y archives; any
// other key keeps the card and says so.
func TestArchiveAsksFirst(t *testing.T) {
	var calls []string
	m := update(navHub(t, &calls), tea.KeyPressMsg{Code: tea.KeyEscape}, letter('l'), letter('X'))
	if !m.hub.App.PaneFocus || m.hub.App.Board.Confirm == 0 {
		t.Fatalf("no question: pane %v board %+v", m.hub.App.PaneFocus, m.hub.App.Board)
	}
	if got := core.HubSurface(m.hub).Kind; got != core.SurfaceArchive {
		t.Fatalf("surface %v", got)
	}
	if m = update(m, letter('y')); len(calls) != 0 {
		t.Fatalf("a y right behind X archived: %v", calls)
	}
	// The question stays up, and says why.
	if m.hub.App.Board.Confirm == 0 || m.hub.Notice != core.TooSoonNote {
		t.Fatalf("the question went, or did not say why: %+v %q", m.hub.App.Board, m.hub.Notice)
	}
	time.Sleep(core.ArmDelay + 50*time.Millisecond)
	if m = update(m, letter('y')); len(calls) != 1 || calls[0] != "card-op" {
		t.Fatalf("an armed y: %v", calls)
	}
	// Any other key keeps the card.
	calls = nil
	m = update(m, letter('X'))
	time.Sleep(core.ArmDelay + 50*time.Millisecond)
	if m = update(m, letter('n')); len(calls) != 0 || m.hub.App.Board.Confirm != 0 || m.hub.App.Board.Message != "Archive cancelled." {
		t.Fatalf("n: calls %v board %+v", calls, m.hub.App.Board)
	}
}

// The key that closes the list of keys only closes it: an x there does
// not tick the item the list covered, on any screen.
func TestClosingHelpDoesNothingElse(t *testing.T) {
	for _, w := range []int{60, 170} {
		var calls []string
		m := update(navHub(t, &calls), tea.WindowSizeMsg{Width: w, Height: 20},
			tea.KeyPressMsg{Code: tea.KeyEscape}, letter('l'), tea.KeyPressMsg{Code: tea.KeyEnter}, letter('j'))
		if !m.hub.App.PaneFocus || !m.hub.App.Board.Detail {
			t.Fatalf("%d: not on an open card", w)
		}
		m = update(m, letter('?'), letter('x'))
		if m.hub.Help || len(calls) != 0 {
			t.Fatalf("%d: help %v, calls %v", w, m.hub.Help, calls)
		}
		if m = update(m, letter('x')); len(calls) != 1 {
			t.Fatalf("%d: x after the help closed: %v", w, calls)
		}
	}
	// Against a single agent too.
	now := time.Now()
	var calls []string
	m := update(New(recConn{calls: &calls}, ""), tea.WindowSizeMsg{Width: 60, Height: 20}, ev(core.Connected{At: now}),
		ev(core.CardsLoaded{At: now, Rev: 2, Cards: navCards()}),
		tea.KeyPressMsg{Code: tea.KeyEscape}, letter('l'), tea.KeyPressMsg{Code: tea.KeyEnter}, letter('j'), letter('?'), letter('x'))
	if m.one.State.Help || len(calls) != 0 {
		t.Fatalf("single: help %v calls %v", m.one.State.Help, calls)
	}
}
