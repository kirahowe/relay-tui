package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"pgregory.net/rapid"

	"github.com/kirahowe/relay-tui/internal/core"
)

// The surface, through the real model: one function in the core
// (core.HubSurface, core.SingleSurface) says what has the keys, the
// renderer draws that as the active surface, and the key router gives
// the key to it alone.

// call is something the model asked the agent to do, and when.
type call struct {
	what string // card-op, end, prompt, cancel
	// option is the card op, "" for the others.
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
func (c callConn) CardOp(_ int64, op core.CardOp) { c.add("card-op", op.Op) }
func (c callConn) End(string)                     { c.add("end", "") }
func (c callConn) Prompt(string, string)          { c.add("prompt", "") }
func (c callConn) Cancel(string)                  { c.add("cancel", "") }
func (c callConn) Attach(string, string)          {}
func (c callConn) Items(string)                   {}
func (c callConn) Seen(string, int)               {}
func (c callConn) Resolve(string, string)         {}
func (c callConn) Cards()                         {}
func (c callConn) Links()                         {}
func (c callConn) Link(_ string, keep bool) {
	c.add("link", map[bool]string{true: "keep", false: "drop"}[keep])
}
func (c callConn) Shutdown() {}

// Links in the outside text the surface runs draw, each broken as relay
// sends it: one relay lists as live, with its address, and one it does
// not.
const (
	surfaceLive      = "https://artsfund.example/grants/2027"
	surfaceLiveShown = "hxxps://artsfund[.]example/grants/2027"
	surfaceBroken    = "hxxps://bit[.]example/3xQ9"
)

// surfaceOwn is what relay lists beside a thing that shows the live one.
var surfaceOwn = []core.Link{{Address: surfaceLive, Shown: surfaceLiveShown}}

// surfaceReadyItem is a message ready for the operator that quotes both
// links. Nobody can answer it, so no reply is ever typed in these runs.
func surfaceReadyItem(at time.Time) core.ReadyItem {
	return core.ReadyItem{ID: "message:4", Kind: core.ReadyMessage, At: at.Format(time.RFC3339), Title: "Two grants",
		Lines: []string{"Arts fund: " + surfaceLiveShown, "A forum post: " + surfaceBroken}, From: "from the web", Outsider: true, Message: 4,
		Links: surfaceOwn}
}

// surfaceHub is a hub model typing on Dev at the test's clock, with the
// board loaded (a checklist and a note), relay's live links, one message
// ready, and Mail's conversation attached.
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
		ev(core.LinksLoaded{At: now, Rev: 2, Live: []string{surfaceLive}}),
		ev(core.CardsLoaded{At: now, Rev: 2, Cards: outsideNavCards()}),
		ev(core.ReadyLoaded{At: now, Rev: 2, Items: []core.ReadyItem{surfaceReadyItem(now)}}))
}

// surfaceSingle is a single-agent model typing, with the board loaded.
func surfaceSingle(calls *[]call, clock *time.Time) Model {
	m := New(callConn{calls: calls, clock: clock}, "")
	m.clock = func() time.Time { return *clock }
	return update(m, tea.WindowSizeMsg{Width: 140, Height: 30}, ev(core.Connected{At: *clock}),
		ev(core.LinksLoaded{At: *clock, Rev: 2, Live: []string{surfaceLive}}),
		ev(core.CardsLoaded{At: *clock, Rev: 2, Cards: outsideNavCards()}))
}

// outsideNavCards is navCards with outside text in the first: an item
// that quotes a live link and a broken one.
func outsideNavCards() []core.Card {
	cards := navCards()
	cards[0].Labels = core.Labels{Contaminated: true}
	cards[0].Items[1].CardText = core.CardText{Text: "go " + surfaceLiveShown + " or " + surfaceBroken, Outsider: true, Rev: 1}
	cards[0].Links = surfaceOwn
	return cards
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
	case "ctrl+c", "ctrl+t", "ctrl+o", "ctrl+y", "ctrl+j":
		return tea.KeyPressMsg{Code: rune(s[5]), Mod: tea.ModCtrl}
	case "alt+1", "alt+2":
		return tea.KeyPressMsg{Code: rune(s[4]), Mod: tea.ModAlt}
	}
	r := []rune(s)[0]
	return tea.KeyPressMsg{Code: r, Text: s}
}

// surfaceKeys are the keys that mean something somewhere: every column
// move, every pane key, and plain letters.
var surfaceKeys = []string{"enter", "esc", "backspace", "tab", "up", "down", "pgup", "ctrl+c", "ctrl+t", "ctrl+o", "ctrl+y", "ctrl+j",
	"alt+1", "alt+2", "h", "j", "k", "l", "i", "e", "E", "a", "x", "X", "y", "Y", "n", "t", "b", "v", "q", "?", " ", "1", "2", "z",
	"L", "d", "R", "o"}

// surfaceID names what has the keys, in either mode.
func (m Model) surfaceID() string {
	if m.kind == kindHub {
		return core.HubSurfaceID(m.hub)
	}
	return core.SingleSurfaceID(m.one)
}

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

// onScreen says the screen shows what id names: the end or archive
// question, or the question about a link, with its yes. Spaces and frame
// lines are ignored, so a wrapped line still counts.
func onScreen(m Model, id string) bool {
	var content string
	if m.kind == kindHub {
		content = m.hubFrame(m.now()).Content
	} else {
		content = m.frame(m.now()).Content
	}
	flat := strings.Map(func(r rune) rune {
		if strings.ContainsRune(" \n│╭╮╰╯─", r) {
			return -1
		}
		return r
	}, ansi.Strip(content))
	has := func(words ...string) bool {
		for _, w := range words {
			if !strings.Contains(flat, strings.ReplaceAll(w, " ", "")) {
				return false
			}
		}
		return true
	}
	switch {
	case strings.HasPrefix(id, "archive/"):
		return has("Archive", "y archive")
	case strings.HasPrefix(id, "end/"):
		return has("End the conversation with", "y to end it")
	case strings.HasPrefix(id, "link/") && strings.Contains(id, "/true/"):
		// The question wraps, and beside another column its rows are not
		// next to each other in the screen's text: its first words and
		// its last are looked for apart.
		return has("relay cannot say", "Keep", "it?", "y keep")
	case strings.HasPrefix(id, "link/"):
		return has("Drop this link?", "shows it again.", "y drop")
	}
	return true
}

// surfaceRun drives a model with random keys, pastes and board changes,
// and the clock moving, and checks the rules after every step:
//
//	(a) the surface that receives a key is the one the renderer draws as
//	    active;
//	(b) no key received by a typing surface ends a conversation, archives
//	    a card, keeps or drops a link, or sends a card op (but the
//	    editor's own enter);
//	(c) no question (end, archive, a link kept or dropped) is answered
//	    yes within ArmDelay of being drawn in full (declining and
//	    cancelling work at once);
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
	track := func() {}
	// present is, for the thing a key could authorize, since when the
	// screen has shown it in full without a break, read off the screen
	// itself, not off what the renderer says it drew.
	present := map[string]time.Time{}
	frames := func() {
		id := m.surfaceID()
		for other := range present {
			if other != id {
				delete(present, other)
			}
		}
		if !onScreen(m, id) {
			delete(present, id)
		} else if _, ok := present[id]; !ok {
			present[id] = clock
		}
	}
	frames()
	step := 0
	press := func(name string) {
		k := key(name)
		surf := surface(m)
		target := m.surfaceID()
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
					c.what == "cancel" && name == "ctrl+c"
				if !ok {
					rt.Fatalf("step %d: key %q while typing (%v) made a %s %q", step, name, surf.Kind, c.what, c.option)
				}
			}
			// (c) what it authorized was drawn in full for the whole delay.
			if authorized(c) {
				if t0, ok := present[target]; !ok || c.at.Sub(t0) < core.ArmDelay {
					rt.Fatalf("step %d: key %q made %+v on %q, drawn in full since %v (%v)", step, name, c, target, t0, ok)
				}
				// A link is kept or dropped only from its own question.
				if c.what == "link" && surf.Kind != core.SurfaceLinkAsk && surf.Kind != core.SurfaceReadyLinkAsk {
					rt.Fatalf("step %d: key %q made %+v from %v, not the question", step, name, c, surf.Kind)
				}
				surfaceAnswered[surf.Kind]++
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
		frames()
	}
	// event applies something that is not a key: nothing typed goes,
	// nothing is answered, and what is being typed in keeps the keys.
	event := func(msg tea.Msg) {
		was, before := surface(m), drafts(m)
		edText, edOpen := editorText(m)
		// A conversation that goes (its role leaves the roster) gives back
		// what it still had queued: that text returns to its role's draft.
		queued := map[string]bool{}
		if hub {
			for _, c := range m.hub.Convs {
				if len(c.State.Queue) > 0 {
					queued[c.Role] = true
				}
			}
		}
		m = update(m, msg)
		if was.Typing() && surface(m) != was {
			rt.Fatalf("step %d: %T took the keys from %+v to %+v", step, msg, was, surface(m))
		}
		after := drafts(m)
		grew := func() bool { // only a given-back queue may add to a draft
			for role, text := range after {
				if text != before[role] && !queued[role] {
					return false
				}
			}
			return true
		}
		if !keptAll(before, after, m.inputRole) || (was.Kind != core.SurfaceInput && !grew()) {
			rt.Fatalf("step %d: %T changed the drafts from %q to %q", step, msg, before, after)
		}
		if now, open := editorText(m); edOpen && (!open || now != edText) {
			rt.Fatalf("step %d: %T changed the editor from %q to %q", step, msg, edText, now)
		}
		track()
		frames()
	}
	// Ways to the surfaces a random key rarely reaches.
	macros := [][]string{
		{"esc", "l"},                     // the pane
		{"esc", "l", "e"},                // a card's title in the editor
		{"esc", "l", "enter", "j", "e"},  // an item's text in the editor
		{"esc", "l", "X"},                // the archive question
		{"esc", "l", "X", "y"},           // archived
		{"esc", "l", "enter", "j", "x"},  // a tick
		{"esc", "l", "L", "enter"},       // the pane's question: keep a link
		{"esc", "l", "L", "enter", "y"},  // kept
		{"esc", "l", "L", "k", "d"},      // the pane's question: drop a link
		{"esc", "l", "L", "k", "d", "y"}, // dropped
		{"esc", "R", "L", "enter"},       // the ready view's question: keep a link
		{"esc", "R", "L", "enter", "y"},  // kept
		{"esc", "R", "L", "k", "d", "y"}, // dropped
		{"y"},                            // a yes to whatever is asked
		{"esc", "h"},                     // the list
		{"esc", "?"},                     // the help
		{"esc", "X"},                     // the end question
		{"i", "w", "o", "r", "d", "s"},   // typing
		{"esc", "l", "e", "a", "b", "c"}, // typing in the editor
		{"i", "a", "ctrl+j", "b", "ctrl+j", "c", "ctrl+j", "d", "ctrl+j", "e"}, // a tall draft
	}
	for range rapid.IntRange(1, 60).Draw(rt, "steps") {
		step++
		clock = clock.Add(time.Duration(rapid.IntRange(0, 900).Draw(rt, "ms")) * time.Millisecond)
		switch rapid.IntRange(0, 11).Draw(rt, "what") {
		case 0, 1: // the roster changes under the keys
			roles := []core.Role{{ID: "dev", Title: "Dev", Conv: &core.RosterConv{Session: "s1", Turn: core.TurnIdle}}}
			if hub {
				if rapid.Bool().Draw(rt, "mail stays") {
					roles = append(roles, core.Role{ID: "mail", Title: "Mail", Conv: &core.RosterConv{Session: "s2", Turn: core.TurnIdle}})
				}
				event(ev(core.RosterUpdate{At: clock, Roles: roles}))
			}
		case 2: // the board changes: a card gets a new rev, or one is archived
			cards := outsideNavCards()
			c := cards[rapid.IntRange(0, 1).Draw(rt, "card")]
			c.Rev = int64(10 + step)
			if rapid.Bool().Draw(rt, "archived") {
				c.Status = core.CardArchived
			}
			event(ev(core.CardsChanged{At: clock, Rev: c.Rev, Cards: []core.Card{c}}))
		case 3:
			if rapid.Bool().Draw(rt, "resize") {
				sz := rapid.SampledFrom([][2]int{{80, 24}, {100, 24}, {120, 30}, {170, 40}}).Draw(rt, "size")
				event(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
				break
			}
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

// surfaceAnswered counts the questions the runs answered yes, by the
// surface that asked.
var surfaceAnswered = map[core.SurfaceKind]int{}

func TestSurfaceRulesHub(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) { surfaceRun(rt, true) })
	t.Log(surfaceSeen, surfaceAnswered)
}
func TestSurfaceRulesSingle(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) { surfaceRun(rt, false) })
	t.Log(surfaceSeen, surfaceAnswered)
}

// Typing in a card's editor, the card is archived from outside. The
// editor stays open with its text and says so; the next letters go on
// into it, not to the board, so no h moves and no y archives; enter sends
// nothing; esc discards it.
func TestAnArchivedCardLeavesItsEditorTyping(t *testing.T) {
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
			m = update(m, key("esc"), key("l"), key("e"), key("x"))
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

func flatText(s string) string {
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(" \t\n│╭╮╰╯─▸", r) {
			return -1
		}
		return r
	}, s)
}

func flatScreen(content string) string { return flatText(ansi.Strip(content)) }

// A tall draft on a short screen gives up rows to the archive question:
// the question and the key that answers it are drawn in full before a y
// can archive.
func TestArchiveQuestionIsDrawnInFullOnAShortScreen(t *testing.T) {
	surfaces := map[string]func(*[]call, *time.Time) Model{
		"single": func(c *[]call, t *time.Time) Model { return surfaceSingle(c, t) },
		"hub":    func(c *[]call, t *time.Time) Model { return surfaceHub(c, t) },
	}
	for name, build := range surfaces {
		// The widths where the pane covers the conversation too, which is
		// where the question once fell off a short screen.
		for _, size := range [][2]int{{80, 24}, {100, 24}} {
			width, height := size[0], size[1]
			t.Run(fmt.Sprint(name, "/", width, "x", height), func(t *testing.T) {
				clock := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
				var calls []call
				m := build(&calls, &clock)
				m = update(m, key("i"))
				for _, k := range []string{"a", "ctrl+j", "b", "ctrl+j", "c", "ctrl+j", "d", "ctrl+j", "e"} {
					m = update(m, key(k))
				}
				m = update(m, tea.WindowSizeMsg{Width: width, Height: height})
				m = update(m, key("esc"), key("l"), key("X"))
				id := m.surfaceID()
				if !strings.HasPrefix(id, "archive/") || !onScreen(m, id) {
					t.Fatalf("surface %q, drawn in full: %v", id, onScreen(m, id))
				}
				// A y at once is refused, and the note that says so does
				// not push the question off the screen.
				m = update(m, key("y"))
				if len(calls) != 0 || m.surfaceID() != id || !onScreen(m, id) {
					t.Fatalf("after a y too soon: calls %v, surface %q, drawn in full: %v", calls, m.surfaceID(), onScreen(m, id))
				}
			})
		}
	}
}

// The question about a link is drawn in full before a y can answer it,
// on a short or a narrow screen too, in the pane and in the ready view:
// the notes above the input, and then the input, give up their rows to
// it.
func TestLinkQuestionIsDrawnInFullOnAShortScreen(t *testing.T) {
	type build func(*[]call, *time.Time) Model
	cases := map[string]struct {
		build build
		keys  []string
	}{
		"single, keep":     {surfaceSingle, []string{"esc", "l", "L", "enter"}},
		"single, drop":     {surfaceSingle, []string{"esc", "l", "L", "k", "d"}},
		"hub pane, keep":   {surfaceHub, []string{"esc", "l", "L", "enter"}},
		"hub pane, drop":   {surfaceHub, []string{"esc", "l", "L", "k", "d"}},
		"ready view, keep": {surfaceHub, []string{"esc", "R", "L", "enter"}},
		"ready view, drop": {surfaceHub, []string{"esc", "R", "L", "k", "d"}},
	}
	for name, c := range cases {
		for _, size := range [][2]int{{80, 24}, {100, 24}, {170, 40}} {
			t.Run(fmt.Sprint(name, "/", size[0], "x", size[1]), func(t *testing.T) {
				clock := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
				var calls []call
				m := c.build(&calls, &clock)
				m = update(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
				for _, k := range c.keys {
					m = update(m, key(k))
				}
				id := m.surfaceID()
				if !strings.HasPrefix(id, "link/") || !onScreen(m, id) {
					t.Fatalf("surface %q, drawn in full: %v", id, onScreen(m, id))
				}
				// A y at once is refused, and the note that says so does
				// not push the question off the screen.
				m = update(m, key("y"))
				if len(calls) != 0 || m.surfaceID() != id || !onScreen(m, id) {
					t.Fatalf("after a y too soon: calls %v, surface %q, drawn in full: %v", calls, m.surfaceID(), onScreen(m, id))
				}
				clock = clock.Add(core.ArmDelay)
				m = update(m, key("y"))
				if len(calls) != 1 || calls[0].what != "link" {
					t.Fatalf("a y once armed made %v", calls)
				}
			})
		}
	}
}

// A message queued for a role that then leaves the roster is given back:
// its text returns to that role's draft, once, and the draft of the other
// role is untouched.
func TestAQueuedMessageIsGivenBackWhenItsRoleLeaves(t *testing.T) {
	clock := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
	var calls []call
	m := surfaceHub(&calls, &clock)
	m = update(m, key("i"))
	m = typeText(m, "first")
	m = update(m, key("enter")) // sent: the conversation is busy now
	m = typeText(m, "words")
	m = update(m, key("enter")) // queued behind it
	if got := drafts(m)["dev"]; got != "" {
		t.Fatalf("after enter the draft is %q", got)
	}
	queued := false
	for _, c := range m.hub.Convs {
		queued = queued || len(c.State.Queue) > 0
	}
	if !queued {
		t.Fatal("nothing was queued")
	}
	only := func(ids ...string) core.RosterUpdate {
		var rs []core.Role
		for _, id := range ids {
			rs = append(rs, core.Role{ID: id, Title: id, Conv: &core.RosterConv{Session: "s-" + id, Turn: core.TurnIdle}})
		}
		return core.RosterUpdate{At: clock, Roles: rs}
	}
	m = update(m, ev(only("mail")), ev(only("dev", "mail")))
	if got := drafts(m)["dev"]; got != "words" {
		t.Fatalf("the draft is %q", got)
	}
	if got := drafts(m)["mail"]; got != "" {
		t.Fatalf("mail's draft is %q", got)
	}
}
