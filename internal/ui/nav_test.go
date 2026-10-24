package ui

import (
	"reflect"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"pgregory.net/rapid"

	"github.com/kirahowe/relay-tui/internal/core"
)

// Typing and moving, through the real model: which keys reach the input,
// the board and a request.

// recConn records the calls that act: a card op, a card trust, an
// answer to a request, a prompt.
type recConn struct {
	nopConn
	calls *[]string
}

func (c recConn) CardOp(int64, core.CardOp)      { *c.calls = append(*c.calls, "card-op") }
func (c recConn) CardTrust(int64, int64, string) { *c.calls = append(*c.calls, "card-trust") }
func (c recConn) Answer(a core.AnswerPermission) {
	if a.Cancelled {
		*c.calls = append(*c.calls, "cancel-answer") // esc on a request, at any time
		return
	}
	*c.calls = append(*c.calls, "answer")
}
func (c recConn) Prompt(string, string, []core.Attachment) { *c.calls = append(*c.calls, "prompt") }

func navCards() []core.Card {
	text := func(s string) core.CardText { return core.CardText{Text: s, Rev: 1} }
	return []core.Card{
		{ID: 1, Rev: 1, Kind: core.CardChecklist, Status: "active", Title: text("Trip"), Total: 2,
			Items: []core.CardElem{{ID: 1, CardText: text("pack")}, {ID: 2, CardText: text("go")}}},
		{ID: 2, Rev: 2, Kind: core.CardNote, Status: "active", Title: text("Note"), Note: &core.CardText{Text: "hello", Rev: 2}},
	}
}

// navHub is a hub model on Dev, typing, with the board loaded and a
// request waiting in Mail's conversation, off screen.
func navHub(t *testing.T, calls *[]string) Model {
	t.Helper()
	now := time.Now()
	roles := []core.Role{
		{ID: "dev", Title: "Dev", Conv: &core.RosterConv{Session: "s1", Turn: core.TurnIdle}},
		{ID: "mail", Title: "Mail", Conv: &core.RosterConv{Session: "s2", Turn: core.TurnIdle}},
		{ID: "news", Title: "News"},
	}
	ask := core.PermissionAsked{At: now, Permission: core.Permission{ID: 1, CallID: "c", Name: "send",
		Options: []core.PermOption{{ID: "a", Name: "Allow", Kind: core.AllowOnce}, {ID: "r", Name: "Reject", Kind: core.RejectOnce}}}}
	m := update(New(recConn{calls: calls}, "").Open("dev"), tea.WindowSizeMsg{Width: 170, Height: 40},
		ev(core.HubConnected{At: now}), ev(core.RosterUpdate{At: now, Roles: roles}),
		ev(core.Attached{At: now, Role: "dev", Session: "s1"}), ev(core.Attached{At: now, Role: "mail", Session: "s2"}),
		ev(core.Routed{Session: "s2", Event: ask}), ev(core.CardsLoaded{At: now, Rev: 2, Cards: navCards()}))
	if m.hub.App.Moving || m.hub.Focus != "dev" {
		t.Fatalf("not typing on dev: moving %v focus %q", m.hub.App.Moving, m.hub.Focus)
	}
	return m
}

// Typing, no key reaches the board or answers a request: not one on the
// board, not one in a conversation off screen. The keys that leave
// typing (esc) or move to another role are left out; everything else is
// fair game.
func TestTypingNeverReachesTheBoardOrARequest(t *testing.T) {
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

// A request is answered only while moving. While typing it takes no
// key: y and enter go to the input. Esc makes it answerable, and its
// delay starts then, so a y right behind the esc answers nothing; once
// armed, y allows. Past its own keys, h and the digits still move, and
// l cannot take the keys to the pane.
func TestARequestIsAnsweredOnlyWhileMoving(t *testing.T) {
	var all []string
	answers := func() []string {
		var out []string
		for _, c := range all {
			if c == "answer" {
				out = append(out, c)
			}
		}
		return out
	}
	m := update(navHub(t, &all), tea.KeyPressMsg{Code: '2', Mod: tea.ModAlt}) // Mail, whose request waits
	time.Sleep(core.ArmDelay + 50*time.Millisecond)
	m = update(m, letter('y'), letter('n'), tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(answers()) != 0 || m.hub.App.Moving {
		t.Fatalf("typing answered: %v (moving %v)", answers(), m.hub.App.Moving)
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEscape}, letter('y'))
	if len(answers()) != 0 || !m.hub.App.Moving {
		t.Fatalf("a y right behind esc answered: %v", answers())
	}
	m = update(m, letter('l'), letter('x'), letter('i'))
	if m.hub.App.PaneFocus || !m.hub.App.Moving || len(answers()) != 0 {
		t.Fatalf("l x i: pane %v moving %v calls %v", m.hub.App.PaneFocus, m.hub.App.Moving, answers())
	}
	time.Sleep(core.ArmDelay + 50*time.Millisecond)
	if m = update(m, letter('y')); len(answers()) != 1 || answers()[0] != "answer" {
		t.Fatalf("armed y: %v", answers())
	}
	// h past a request reaches the list.
	all = nil
	m = update(navHub(t, &all), tea.KeyPressMsg{Code: '2', Mod: tea.ModAlt}, tea.KeyPressMsg{Code: tea.KeyEscape}, letter('h'))
	if !m.hub.List || len(answers()) != 0 {
		t.Fatalf("h: list %v calls %v", m.hub.List, answers())
	}
}

// Against a single agent the same: typing, the request takes no key.
func TestSingleRequestWaitsWhileTyping(t *testing.T) {
	now := time.Now()
	var all []string
	answers := func() []string {
		var out []string
		for _, c := range all {
			if c == "answer" {
				out = append(out, c)
			}
		}
		return out
	}
	ask := core.PermissionAsked{At: now, Permission: core.Permission{ID: 1, Options: []core.PermOption{{ID: "a", Kind: core.AllowOnce}}}}
	m := update(New(recConn{calls: &all}, ""), tea.WindowSizeMsg{Width: 80, Height: 24}, ev(core.Connected{At: now}),
		ev(core.Submit{At: now, Text: "go"}), ev(ask))
	time.Sleep(core.ArmDelay + 50*time.Millisecond)
	m = update(m, letter('y'))
	if len(answers()) != 0 || m.input.Value() != "y" {
		t.Fatalf("typing: calls %v input %q", answers(), m.input.Value())
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEscape}, letter('y'))
	if len(answers()) != 0 {
		t.Fatalf("a y right behind esc answered: %v", answers())
	}
	time.Sleep(core.ArmDelay + 50*time.Millisecond)
	if m = update(m, letter('y')); len(answers()) != 1 || m.input.Value() != "y" {
		t.Fatalf("armed y: %v input %q", answers(), m.input.Value())
	}
}

// Typing, no key sequence answers a request, on screen or off, however
// long after it came; moving, no key answers one within its delay of its
// becoming answerable. The keys that stop the turn or quit are left out:
// stopping a turn cancels its requests on purpose.
func TestNoKeyAnswersARequestWhileTypingOrUnarmed(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		var calls []string
		m := update(navHub(t, &calls), tea.KeyPressMsg{Code: '2', Mod: tea.ModAlt})
		// The request has waited past its delay while the operator typed.
		m.hub.Convs = slices.Clone(m.hub.Convs)
		for i := range m.hub.Convs {
			if a := m.hub.Convs[i].State.Asks; len(a) > 0 {
				a = slices.Clone(a)
				a[0].Since = a[0].Since.Add(-time.Hour)
				m.hub.Convs[i].State.Asks = a
			}
		}
		answerable := time.Time{}
		for range rapid.IntRange(1, 40).Draw(rt, "keys") {
			k := genKey(rt)
			if s := k.String(); s == "ctrl+c" || s == "ctrl+d" {
				continue
			}
			was := core.HubSurface(m.hub)
			before := answered(calls)
			m = update(m, k)
			if was.Typing() && answered(calls) > before {
				rt.Fatalf("key %q while typing answered: %v", k.String(), calls)
			}
			if answered(calls) > before && time.Since(answerable) < core.ArmDelay {
				rt.Fatalf("key %q answered within the delay of the request becoming the surface: %v", k.String(), calls)
			}
			if was.Kind != core.SurfaceRequest && core.HubSurface(m.hub).Kind == core.SurfaceRequest {
				answerable = time.Now()
			}
		}
	})
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

// answered is how many answers to requests calls holds.
func answered(calls []string) int {
	n := 0
	for _, c := range calls {
		if c == "answer" {
			n++
		}
	}
	return n
}
