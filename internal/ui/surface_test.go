package ui

import (
	"fmt"
	"maps"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"pgregory.net/rapid"

	"github.com/kirahowe/relay-tui/internal/core"
)

// The surface, through the real model: one function in the core
// (core.HubSurface, core.SingleSurface) says what has the keys, the
// renderer draws that as the active surface, and the key router gives
// the key to it alone.

// call is something the model asked the agent to do, and when.
type call struct {
	what string // answer, card-op, card-trust, trust, end, prompt
	// option is the answer's option, "" for cancelled.
	option string
	at     time.Time
}

// callConn records the calls that act, at the test's clock.
type callConn struct {
	nopConn
	calls *[]call
	clock *time.Time
}

func (c callConn) add(what, option string) {
	*c.calls = append(*c.calls, call{what: what, option: option, at: *c.clock})
}
func (c callConn) Answer(a core.AnswerPermission)           { c.add("answer", a.OptionID) }
func (c callConn) CardOp(_ int64, op core.CardOp)           { c.add("card-op", op.Op) }
func (c callConn) CardTrust(int64, int64, string)           { c.add("card-trust", "") }
func (c callConn) Trust(string, string, string, string)     { c.add("trust", "") }
func (c callConn) End(string)                               { c.add("end", "") }
func (c callConn) Prompt(string, string, []core.Attachment) { c.add("prompt", "") }
func (c callConn) Cancel(string)                            { c.add("cancel", "") }
func (c callConn) Attach(string, string)                    {}
func (c callConn) Items(string)                             {}
func (c callConn) Seen(string, int)                         {}
func (c callConn) Resolve(string, string)                   {}
func (c callConn) Cards()                                   {}
func (c callConn) Shutdown()                                {}

var surfaceAsk = core.Permission{Name: "send_mail", Options: []core.PermOption{
	{ID: "allow", Name: "Allow", Kind: core.AllowOnce}, {ID: "reject", Name: "Reject", Kind: core.RejectOnce}}}

func askEvent(at time.Time, id uint64) core.PermissionAsked {
	p := surfaceAsk
	p.ID, p.CallID = id, fmt.Sprintf("c%d", id)
	return core.PermissionAsked{At: at, Permission: p}
}

// surfaceHub is a hub model typing on Dev at the test's clock, with the
// board loaded (a checklist and a note) and Mail's conversation attached.
func surfaceHub(calls *[]call, clock *time.Time) Model {
	now := *clock
	roles := []core.Role{
		{ID: "dev", Title: "Dev", Conv: &core.RosterConv{Session: "s1", Turn: core.TurnIdle}},
		{ID: "mail", Title: "Mail", Conv: &core.RosterConv{Session: "s2", Turn: core.TurnIdle}},
	}
	m := New(callConn{calls: calls, clock: clock}, "").Open("dev")
	m.clock = func() time.Time { return *clock }
	return update(m, tea.WindowSizeMsg{Width: 170, Height: 40},
		ev(core.HubConnected{At: now}), ev(core.RosterUpdate{At: now, Roles: roles}),
		ev(core.Attached{At: now, Role: "dev", Session: "s1"}), ev(core.Attached{At: now, Role: "mail", Session: "s2"}),
		ev(core.CardsLoaded{At: now, Rev: 2, Cards: navCards()}))
}

// surfaceSingle is a single-agent model typing, with the board loaded.
func surfaceSingle(calls *[]call, clock *time.Time) Model {
	m := New(callConn{calls: calls, clock: clock}, "")
	m.clock = func() time.Time { return *clock }
	return update(m, tea.WindowSizeMsg{Width: 140, Height: 30}, ev(core.Connected{At: *clock, Embedded: true}),
		ev(core.CardsLoaded{At: *clock, Rev: 2, Cards: navCards()}))
}

func key(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "pgup":
		return tea.KeyPressMsg{Code: tea.KeyPgUp}
	case "ctrl+c", "ctrl+t", "ctrl+o", "ctrl+y":
		return tea.KeyPressMsg{Code: rune(s[5]), Mod: tea.ModCtrl}
	case "alt+1", "alt+2":
		return tea.KeyPressMsg{Code: rune(s[4]), Mod: tea.ModAlt}
	}
	r := []rune(s)[0]
	return tea.KeyPressMsg{Code: r, Text: s}
}

// surfaceKeys are the keys that mean something somewhere: every column
// move, every pane key, the request's keys, and plain letters.
var surfaceKeys = []string{"enter", "esc", "backspace", "tab", "up", "down", "pgup", "ctrl+c", "ctrl+t", "ctrl+o", "ctrl+y",
	"alt+1", "alt+2", "h", "j", "k", "l", "i", "e", "E", "a", "x", "X", "y", "Y", "n", "t", "b", "v", "q", "?", " ", "1", "2", "z"}

// editorText is the board's editor, or "" with none open.
func editorText(m Model) (string, bool) {
	if in := m.app().Board.Input; in != nil {
		return in.Text, true
	}
	return "", false
}

// keptAll says every draft in before is still there in after, whole
// (text put back ahead of it is allowed).
func keptAll(before, after map[string]string, except string) bool {
	for role, text := range before {
		if role != except && !strings.Contains(after[role], text) {
			return false
		}
	}
	return true
}

// surfaceRun drives a model with random keys, pastes, requests arriving
// in the conversation on screen and off it, and the clock moving, and
// checks the four rules after every step:
//
//	(a) the surface that receives a key is the one the renderer draws as
//	    active;
//	(b) no key received by a typing surface answers a request, sends a
//	    card op (but the editor's own enter), trusts or confirms anything;
//	(c) no request is answered, other than cancelled, within ArmDelay of
//	    becoming the surface;
//	(d) no draft or editor text is lost but by sending it, or by the
//	    keys that clear it on purpose (ctrl+c in an idle input, esc or
//	    ctrl+c in the editor).
func surfaceRun(rt *rapid.T, hub bool) {
	clock := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
	var calls []call
	var m Model
	surface := func(m Model) core.Surface { return core.SingleSurface(m.one) }
	drawn := func(m Model) core.SurfaceKind { return m.frame(clock).Active }
	if hub {
		m = surfaceHub(&calls, &clock)
		surface = func(m Model) core.Surface { return core.HubSurface(m.hub) }
		drawn = func(m Model) core.SurfaceKind { return m.hubFrame(clock).Active }
	} else {
		m = surfaceSingle(&calls, &clock)
	}
	nextAsk := uint64(1)
	// offer is the request that is the surface, and since when.
	offer, since := "", time.Time{}
	track := func() {
		o := ""
		if surface(m).Kind == core.SurfaceRequest {
			if hub {
				c := m.hub.Focused()
				o = fmt.Sprintf("%s/%d", c.Session, c.State.Asks[0].ID)
			} else {
				o = fmt.Sprint(m.one.State.Asks[0].ID)
			}
		}
		if o != offer {
			offer, since = o, clock
		}
	}
	step := 0
	press := func(name string) {
		k := key(name)
		surf := surface(m)
		surfaceSeen[surf.Kind]++
		before := drafts(m)
		edText, edOpen := editorText(m)
		n := len(calls)
		// (a)
		if surf.Under != core.SurfaceTranscript { // the transcript is the alternate screen
			if got := drawn(m); got != surf.Kind {
				rt.Fatalf("step %d: the surface is %+v but the frame drew %v as active", step, surf, got)
			}
		}
		idle := m.paneState().Phase != core.Busy
		m = update(m, k)
		if m.keysTo != surf {
			rt.Fatalf("step %d: key %q went to %+v, the surface was %+v", step, name, m.keysTo, surf)
		}
		for _, c := range calls[n:] {
			// (b)
			if surf.Typing() {
				ok := c.what == "prompt" && surf.Kind == core.SurfaceInput && name == "enter" ||
					c.what == "card-op" && surf.Kind == core.SurfaceEditor && name == "enter" ||
					c.what == "cancel" && name == "ctrl+c" ||
					c.what == "answer" && c.option == "" && name == "ctrl+c" // stopping a turn cancels its requests
				if !ok {
					rt.Fatalf("step %d: key %q while typing (%v) made a %s %q", step, name, surf.Kind, c.what, c.option)
				}
			}
			// (c)
			if c.what == "answer" && c.option != "" && c.at.Sub(since) < core.ArmDelay {
				rt.Fatalf("step %d: key %q answered %q %v after the request became the surface", step, name, c.option, c.at.Sub(since))
			}
		}
		// (d)
		after := drafts(m)
		if surf.Kind == core.SurfaceInput {
			role := m.inputRole
			if !keptAll(before, after, role) {
				rt.Fatalf("step %d: key %q typing in %q changed another draft: %q to %q", step, name, role, before, after)
			}
			if before[role] != "" && after[role] == "" && name != "enter" && name != "backspace" && !(name == "ctrl+c" && idle) {
				rt.Fatalf("step %d: key %q lost the draft %q", step, name, before[role])
			}
		} else if !keptAll(before, after, "") {
			rt.Fatalf("step %d: key %q on %v changed the drafts from %q to %q", step, name, surf.Kind, before, after)
		}
		now, open := editorText(m)
		switch {
		case surf.Kind == core.SurfaceEditor && !open:
			if name != "enter" && name != "esc" && name != "ctrl+c" {
				rt.Fatalf("step %d: key %q closed the editor and lost %q", step, name, edText)
			}
		case surf.Kind != core.SurfaceEditor && edOpen && (!open || now != edText):
			rt.Fatalf("step %d: key %q on %v changed the editor from %q to %q", step, name, surf.Kind, edText, now)
		}
		track()
	}
	// event applies something that is not a key: nothing typed goes,
	// nothing is answered, and what is being typed in keeps the keys.
	event := func(msg tea.Msg) {
		was, before := surface(m), drafts(m)
		edText, edOpen := editorText(m)
		n := len(calls)
		m = update(m, msg)
		if was.Typing() && surface(m) != was {
			rt.Fatalf("step %d: %T took the keys from %+v to %+v", step, msg, was, surface(m))
		}
		if after := drafts(m); !keptAll(before, after, m.inputRole) || (was.Kind != core.SurfaceInput && !maps.Equal(before, after)) {
			rt.Fatalf("step %d: %T changed the drafts from %q to %q", step, msg, before, after)
		}
		if now, open := editorText(m); edOpen && (!open || now != edText) {
			rt.Fatalf("step %d: %T changed the editor from %q to %q", step, msg, edText, now)
		}
		for _, c := range calls[n:] {
			if c.what == "answer" && c.option != "" {
				rt.Fatalf("step %d: %T answered a request", step, msg)
			}
		}
		track()
	}
	// Ways to the surfaces a random key rarely reaches.
	macros := [][]string{
		{"esc", "l"},                     // the pane
		{"esc", "l", "e"},                // a card's title in the editor
		{"esc", "l", "enter", "j", "e"},  // an item's text in the editor
		{"esc", "l", "X"},                // the archive question
		{"esc", "l", "enter", "j", "x"},  // a tick
		{"esc", "h"},                     // the list
		{"esc", "?"},                     // the help
		{"esc", "X"},                     // the end question
		{"i", "w", "o", "r", "d", "s"},   // typing
		{"esc", "l", "e", "a", "b", "c"}, // typing in the editor
	}
	for range rapid.IntRange(1, 60).Draw(rt, "steps") {
		step++
		clock = clock.Add(time.Duration(rapid.IntRange(0, 900).Draw(rt, "ms")) * time.Millisecond)
		switch rapid.IntRange(0, 11).Draw(rt, "what") {
		case 0: // a request in the conversation on screen
			e := askEvent(clock, nextAsk)
			nextAsk++
			if hub {
				event(ev(core.Routed{Session: "s1", Event: e}))
			} else {
				event(ev(e))
			}
		case 1: // a request off screen in hub mode; a second one in single
			e := askEvent(clock, nextAsk)
			nextAsk++
			if hub {
				event(ev(core.Routed{Session: "s2", Event: e}))
			} else {
				event(ev(e))
			}
		case 2: // the board changes: a card gets a new rev, or one is archived
			cards := navCards()
			c := cards[rapid.IntRange(0, 1).Draw(rt, "card")]
			c.Rev = int64(10 + step)
			if rapid.Bool().Draw(rt, "archived") {
				c.Status = core.CardArchived
			}
			event(ev(core.CardsChanged{At: clock, Rev: c.Rev, Cards: []core.Card{c}}))
		case 3:
			event(tea.PasteMsg{Content: "pasted words"})
		case 4, 5:
			for _, k := range rapid.SampledFrom(macros).Draw(rt, "macro") {
				press(k)
			}
		default:
			press(rapid.SampledFrom(surfaceKeys).Draw(rt, "key"))
		}
	}
}

var surfaceSeen = map[core.SurfaceKind]int{}

func TestSurfaceRulesHub(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) { surfaceRun(rt, true) })
	t.Log(surfaceSeen)
}
func TestSurfaceRulesSingle(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) { surfaceRun(rt, false) })
	t.Log(surfaceSeen)
}

// Codex round 2, finding 1: typing in a card's editor with the board's
// keys, a request from the conversation on screen arrives. It takes no
// keys and closes nothing: a y typed long after goes into the editor,
// and enter sends the edit, not an answer.
func TestARequestNeverTakesTheEditorsKeys(t *testing.T) {
	for _, hub := range []bool{true, false} {
		t.Run(fmt.Sprint("hub=", hub), func(t *testing.T) {
			clock := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
			var calls []call
			var m Model
			if hub {
				m = surfaceHub(&calls, &clock)
			} else {
				m = surfaceSingle(&calls, &clock)
			}
			// esc, l to the board, enter on the first card, e on its title.
			m = update(m, key("esc"), key("l"), key("enter"), key("e"), key("backspace"), key("backspace"), key("backspace"), key("backspace"))
			if hub && m.hub.Focus != "dev" {
				t.Fatalf("focus %q", m.hub.Focus)
			}
			m = update(m, key("a"), key("b"))
			if text, open := editorText(m); !open || text != "ab" {
				t.Fatalf("editor %q open %v", text, open)
			}
			if hub {
				m = update(m, ev(core.Routed{Session: "s1", Event: askEvent(clock, 1)}))
			} else {
				m = update(m, ev(askEvent(clock, 1)))
			}
			clock = clock.Add(2 * time.Second)
			m = update(m, key("y"))
			surf := core.SingleSurface(m.one)
			if hub {
				surf = core.HubSurface(m.hub)
			}
			if text, _ := editorText(m); text != "aby" || surf.Kind != core.SurfaceEditor {
				t.Fatalf("editor %q surface %+v", text, surf)
			}
			m = update(m, key("enter"))
			if len(calls) != 1 || calls[0].what != "card-op" {
				t.Fatalf("calls %+v", calls)
			}
			// esc leaves the editor closed and the pane with the keys; the
			// request waits for h, and arms then.
			m = update(m, key("esc"), key("h"))
			clock = clock.Add(core.ArmDelay / 2)
			m = update(m, key("y"))
			if len(calls) != 1 {
				t.Fatalf("answered within the delay: %+v", calls)
			}
			clock = clock.Add(core.ArmDelay)
			m = update(m, key("y"))
			if len(calls) != 2 || calls[1].what != "answer" || calls[1].option != "allow" {
				t.Fatalf("calls %+v", calls)
			}
		})
	}
}

// Codex round 2, finding 5: against a plain agent, typing with a
// request waiting, ctrl+t and ctrl+o do not open a transcript that
// could not be drawn: the letters keep typing, a paste lands in the
// input, and esc moves to the request.
func TestTheTranscriptDoesNotOpenUnderARequest(t *testing.T) {
	clock := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
	var calls []call
	m := surfaceSingle(&calls, &clock)
	m = update(m, ev(askEvent(clock, 1)))
	m = update(m, key("ctrl+t"), key("ctrl+o"))
	if m.one.State.Transcript.Open || core.SingleSurface(m.one).Kind != core.SurfaceInput {
		t.Fatalf("transcript %v surface %+v", m.one.State.Transcript.Open, core.SingleSurface(m.one))
	}
	m = typeText(m, "abc")
	m = update(m, tea.PasteMsg{Content: "pasted words"})
	if v := m.input.Value(); !strings.HasPrefix(v, "abc[pasted 1 line") {
		t.Fatalf("input %q", v)
	}
	m = update(m, key("esc"))
	if s := core.SingleSurface(m.one); s.Kind != core.SurfaceRequest || !strings.HasPrefix(m.input.Value(), "abc") {
		t.Fatalf("surface %+v input %q", s, m.input.Value())
	}
}

// Codex round 3, finding 2: typing in a card's editor, the card is
// archived from outside. The editor stays open with its text and says
// so; the next letters go on into it, not to the board, so no h moves
// and no y answers the request waiting; enter sends nothing; esc
// discards it.
func TestAnArchivedCardLeavesItsEditorTyping(t *testing.T) {
	for _, hub := range []bool{true, false} {
		t.Run(fmt.Sprint("hub=", hub), func(t *testing.T) {
			clock := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
			var calls []call
			var m Model
			ask := tea.Msg(ev(askEvent(clock, 1)))
			if hub {
				m = surfaceHub(&calls, &clock)
				ask = ev(core.Routed{Session: "s1", Event: askEvent(clock, 1)})
			} else {
				m = surfaceSingle(&calls, &clock)
			}
			m = update(m, key("esc"), key("l"), key("e"), key("x"), ask)
			gone := navCards()[0]
			gone.Rev, gone.Status = 9, core.CardArchived
			m = update(m, ev(core.CardsChanged{At: clock, Rev: 9, Cards: []core.Card{gone}}))
			clock = clock.Add(time.Hour)
			m = typeText(m, "happy")
			m = update(m, key("y"))
			text, open := editorText(m)
			if !open || !strings.HasSuffix(text, "xhappyy") || m.app().Board.Message != "card 1 left the board · enter does nothing · esc to discard" {
				t.Fatalf("editor %q open %v message %q", text, open, m.app().Board.Message)
			}
			m = update(m, key("enter"))
			if len(calls) != 0 {
				t.Fatalf("calls %+v", calls)
			}
			if m = update(m, key("esc")); func() bool { _, open := editorText(m); return open }() {
				t.Fatal("esc did not discard it")
			}
		})
	}
}
