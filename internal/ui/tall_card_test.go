package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"pgregory.net/rapid"

	"github.com/kirahowe/relay-tui/internal/a2ui"
	"github.com/kirahowe/relay-tui/internal/core"
)

// An open card taller than the pane (relay#110): its frame stays on the
// screen, and the keys bring every line of its body into view.

// tb is what the helpers need of a test, plain or rapid.
type tb interface {
	Helper()
	Fatal(...any)
	Fatalf(string, ...any)
}

// opConn records every card op in full.
type opConn struct {
	nopConn
	ops *[]core.CardOp
}

func (c opConn) CardOp(_ int64, op core.CardOp) { *c.ops = append(*c.ops, op) }

// tallHub is a hub model with cards on the board, width by height, with
// the pane's keys and the first card open.
func tallHub(t tb, ops *[]core.CardOp, width, height int, cards ...core.Card) Model {
	t.Helper()
	now := time.Now()
	roles := []core.Role{{ID: "dev", Title: "Dev", Conv: &core.RosterConv{Session: "s1", Turn: core.TurnIdle}}}
	m := update(New(opConn{ops: ops}, "").Open("dev"), tea.WindowSizeMsg{Width: width, Height: height},
		ev(core.HubConnected{At: now}), ev(core.RosterUpdate{At: now, Roles: roles}),
		ev(core.Attached{At: now, Role: "dev", Session: "s1"}),
		ev(core.CardsLoaded{At: now, Rev: 9, Cards: cards}),
		tea.KeyPressMsg{Code: tea.KeyEscape}, letter('l'), tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.hub.App.PaneFocus || !m.hub.App.Board.Detail {
		t.Fatalf("no open card: pane %v detail %v", m.hub.App.PaneFocus, m.hub.App.Board.Detail)
	}
	return m
}

// tallSingle is tallHub against a single agent.
func tallSingle(t tb, ops *[]core.CardOp, width, height int, cards ...core.Card) Model {
	t.Helper()
	now := time.Now()
	m := update(New(opConn{ops: ops}, ""), tea.WindowSizeMsg{Width: width, Height: height}, ev(core.Connected{At: now}),
		ev(core.CardsLoaded{At: now, Rev: 9, Cards: cards}),
		tea.KeyPressMsg{Code: tea.KeyEscape}, letter('l'), tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.one.App.PaneFocus || !m.one.App.Board.Detail {
		t.Fatalf("no open card: pane %v detail %v", m.one.App.PaneFocus, m.one.App.Board.Detail)
	}
	return m
}

// screen is what the model draws, as plain text.
func screen(m Model) string {
	if m.kind == kindHub {
		return ansi.Strip(m.hubFrame(m.now()).Content)
	}
	return ansi.Strip(m.frame(m.now()).Content)
}

// tallNote is a note of n numbered lines.
func tallNote(id int64, n int) core.Card {
	var lines []string
	for i := 1; i <= n; i++ {
		lines = append(lines, fmt.Sprintf("line %03d", i))
	}
	return core.Card{ID: id, Rev: 1, Kind: core.CardNote, Status: core.CardActive, Owner: "dev",
		Title: core.CardText{Text: "Long note", Rev: 1}, Note: &core.CardText{Text: strings.Join(lines, "\n"), Rev: 1}}
}

// tallSurface is a surface card from outside, owned by steward: for each
// gap, that many lines of text and then a box, and after the last box
// tail lines of text. Box i is labelled "box NN" and bound to /bNN.
func tallSurface(t tb, id int64, gaps []int, tail int) core.Card {
	t.Helper()
	comps := map[string]a2ui.Component{}
	var kids []any
	data := any(map[string]any{})
	text := 0
	addText := func(n int) {
		for range n {
			text++
			cid := fmt.Sprintf("t%03d", text)
			comps[cid] = a2ui.Component{ID: cid, Type: "Text", Props: map[string]any{"text": fmt.Sprintf("text %03d", text)}}
			kids = append(kids, cid)
		}
	}
	for i, g := range gaps {
		addText(g)
		cid, path := fmt.Sprintf("box%02d", i), fmt.Sprintf("/b%02d", i)
		var err error
		if data, err = a2ui.Set(data, path, false); err != nil {
			t.Fatal(err)
		}
		comps[cid] = a2ui.Component{ID: cid, Type: "CheckBox", Props: map[string]any{"label": fmt.Sprintf("box %02d", i), "value": map[string]any{"@path": path}}}
		kids = append(kids, cid)
	}
	addText(tail)
	if kids == nil {
		kids = []any{}
	}
	comps["root"] = a2ui.Component{ID: "root", Type: "Column", Props: map[string]any{"children": kids}}
	s := a2ui.Surface{ID: fmt.Sprintf("card_%d", id), CatalogID: a2ui.RelayCatalogID, Components: comps, Data: data}
	if err := a2ui.Resolved(s); err != nil {
		t.Fatal(err)
	}
	return core.Card{ID: id, Rev: 1, Kind: core.CardSurface, Status: core.CardActive, Owner: "steward",
		Title: core.CardText{Text: "Trip prep", Outsider: true, Rev: 1}, Text: "a surface",
		Surface: &core.RoleSurface{Outsider: true, A2UI: s}}
}

// The criterion's test: a note taller than the pane can be read to its
// last line with the keys. Before relay#110 down stayed on the title and
// the window never moved.
func TestATallNoteCanBeReadToItsLastLine(t *testing.T) {
	for _, mode := range []string{"hub", "narrow hub", "single", "narrow single"} {
		t.Run(mode, func(t *testing.T) { readTallNote(t, mode) })
	}
}

func readTallNote(t *testing.T, mode string) {
	var ops []core.CardOp
	width := 170
	if strings.HasPrefix(mode, "narrow") {
		width = 80 // the pane takes the conversation's place
	}
	var m Model
	if strings.HasSuffix(mode, "hub") {
		m = tallHub(t, &ops, width, 24, tallNote(1, 60))
	} else {
		m = tallSingle(t, &ops, width, 24, tallNote(1, 60))
	}
	if strings.Contains(screen(m), "line 060") {
		t.Fatal("the test needs a note taller than the pane")
	}
	for range 100 {
		m = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
		if !strings.Contains(screen(m), "Long note") {
			t.Fatalf("the title left the screen:\n%s", screen(m))
		}
	}
	if !strings.Contains(screen(m), "line 060") {
		t.Fatalf("the last line is out of reach:\n%s", screen(m))
	}
	// Page keys come back to the top.
	for range 20 {
		m = update(m, tea.KeyPressMsg{Code: tea.KeyPgUp})
	}
	if !strings.Contains(screen(m), "line 001") {
		t.Fatalf("page up does not come back to the first line:\n%s", screen(m))
	}
	if len(ops) != 0 {
		t.Fatalf("reading sent ops: %v", ops)
	}
}

// A card shown as its text form, of a kind this client does not know.
func TestATallTextFormCanBeReadToItsLastLine(t *testing.T) {
	c := tallNote(1, 60)
	c.Kind, c.Text, c.Note = "poll", c.Note.Text, nil
	var ops []core.CardOp
	m := tallHub(t, &ops, 170, 24, c)
	for range 100 {
		m = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if !strings.Contains(screen(m), "line 060") {
		t.Fatalf("the last line is out of reach:\n%s", screen(m))
	}
}

// A surface with no inputs, and one with boxes far apart and text after
// the last: down reads every line, the frame's owner and its "from
// outside" never leave the screen, and tab stops on every box with the box
// on the screen.
func TestATallSurfaceIsReadInItsFrame(t *testing.T) {
	for _, tc := range []struct {
		gaps []int
		tail int
	}{{nil, 50}, {[]int{2, 30, 30}, 30}} {
		var ops []core.CardOp
		m := tallHub(t, &ops, 170, 24, tallSurface(t, 1, tc.gaps, tc.tail))
		frame := func(s string) {
			t.Helper()
			if !strings.Contains(s, "steward") || !strings.Contains(s, "from outside") || !strings.Contains(s, "Trip prep") {
				t.Fatalf("%v: the frame left the screen:\n%s", tc.gaps, s)
			}
		}
		for range 200 {
			m = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
			frame(screen(m))
			if m.hub.App.Board.Line != 0 {
				t.Fatalf("%v: down focused line %d", tc.gaps, m.hub.App.Board.Line)
			}
		}
		if s := screen(m); !strings.Contains(s, fmt.Sprintf("text %03d", sum(tc.gaps)+tc.tail)) {
			t.Fatalf("%v: the last line is out of reach:\n%s", tc.gaps, s)
		}
		for i := range tc.gaps {
			m = update(m, tea.KeyPressMsg{Code: tea.KeyTab})
			s := screen(m)
			frame(s)
			if m.hub.App.Board.Line != i+1 || !strings.Contains(s, fmt.Sprintf("box %02d", i)) {
				t.Fatalf("%v: tab %d is on line %d:\n%s", tc.gaps, i+1, m.hub.App.Board.Line, s)
			}
		}
		if m = update(m, tea.KeyPressMsg{Code: tea.KeyTab}); m.hub.App.Board.Line != 0 {
			t.Fatalf("%v: tab past the last box is on line %d, not the title", tc.gaps, m.hub.App.Board.Line)
		}
		if len(ops) != 0 {
			t.Fatalf("moving sent ops: %v", ops)
		}
	}
}

func sum(xs []int) int {
	n := 0
	for _, x := range xs {
		n += x
	}
	return n
}

// For any surface of boxes and text, any pane height and any keys: the
// frame is on the screen; only tab and shift+tab move the focus; and x
// ticks the focused box only when it is on the screen, sending its path,
// and otherwise brings it into view and sends nothing.
func TestMovingThroughATallSurfaceKeepsWhatXTicksOnTheScreen(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		gaps := rapid.SliceOfN(rapid.IntRange(0, 12), 0, 8).Draw(rt, "gaps")
		tail := rapid.IntRange(0, 20).Draw(rt, "tail")
		height := rapid.IntRange(24, 40).Draw(rt, "height")
		var ops []core.CardOp
		m := tallHub(rt, &ops, 170, height, tallSurface(rt, 1, gaps, tail))
		keys := map[string]tea.KeyPressMsg{"up": {Code: tea.KeyUp}, "down": {Code: tea.KeyDown},
			"pgup": {Code: tea.KeyPgUp}, "pgdown": {Code: tea.KeyPgDown}, "tab": {Code: tea.KeyTab},
			"shift+tab": {Code: tea.KeyTab, Mod: tea.ModShift}, "x": letter('x')}
		for step := range 40 {
			k := rapid.SampledFrom([]string{"up", "down", "down", "pgup", "pgdown", "tab", "shift+tab", "x"}).Draw(rt, "key")
			c, _ := core.BoardCard(m.hub.App.Board)
			line := core.BoardLine(m.hub.App.Board, c)
			label := fmt.Sprintf("box %02d", line-1)
			seen := line > 0 && strings.Contains(screen(m), label)
			n := len(ops)
			m = update(m, keys[k])
			s := screen(m)
			if !strings.Contains(s, "steward") || !strings.Contains(s, "from outside") {
				rt.Fatalf("step %d %s: the frame left the screen:\n%s", step, k, s)
			}
			now := core.BoardLine(m.hub.App.Board, c)
			if k != "tab" && k != "shift+tab" && now != line {
				rt.Fatalf("step %d %s: the focus moved from %d to %d", step, k, line, now)
			}
			if (k == "tab" || k == "shift+tab") && now > 0 && !strings.Contains(s, fmt.Sprintf("box %02d", now-1)) {
				rt.Fatalf("step %d %s: the focused box %02d is not drawn:\n%s", step, k, now-1, s)
			}
			if k != "x" || line == 0 {
				continue
			}
			switch {
			case seen && (len(ops) != n+1 || ops[n].Path == nil || *ops[n].Path != fmt.Sprintf("/b%02d", line-1)):
				rt.Fatalf("step %d: x on %s, on the screen, sent %+v", step, label, ops[n:])
			case !seen && len(ops) != n:
				rt.Fatalf("step %d: x on %s, off the screen, sent %+v:\n%s", step, label, ops[n:], s)
			case !seen && !strings.Contains(s, label):
				rt.Fatalf("step %d: x on %s, off the screen, did not bring it back:\n%s", step, label, s)
			}
			if len(ops) > n {
				// relay's answer is not part of this test: the next x
				// waits for one, so the op is taken as refused.
				m = update(m, ev(core.CardOpDone{At: time.Now(), CardID: 1, Err: "refused"}))
			}
		}
	})
}

// Tab to a box, scroll it out of sight, press x: the box comes back into
// view and nothing is sent. The next x ticks it.
func TestXOnABoxScrolledAwayBringsItBack(t *testing.T) {
	var ops []core.CardOp
	m := tallHub(t, &ops, 170, 24, tallSurface(t, 1, []int{2}, 60))
	m = update(m, tea.KeyPressMsg{Code: tea.KeyTab}, tea.KeyPressMsg{Code: tea.KeyPgDown}, tea.KeyPressMsg{Code: tea.KeyPgDown})
	if strings.Contains(screen(m), "box 00") {
		t.Fatalf("the box is still on the screen:\n%s", screen(m))
	}
	m = update(m, letter('x'))
	if len(ops) != 0 || !strings.Contains(screen(m), "box 00") {
		t.Fatalf("x off the screen: ops %v\n%s", ops, screen(m))
	}
	if m = update(m, letter('x')); len(ops) != 1 || *ops[0].Path != "/b00" {
		t.Fatalf("x on the screen: ops %+v", ops)
	}
}

// Review of relay#110's rebuild: an x on a box scrolled away put up a
// notice, which took the rows the box needed on a short pane, so every x
// after it found the box off the screen again. At the least terminal the
// second x ticks it.
func TestXBringsABoxBackOnAShortPane(t *testing.T) {
	var ops []core.CardOp
	m := tallSingle(t, &ops, 80, 24, tallSurface(t, 1, []int{0}, 60))
	m = update(m, tea.KeyPressMsg{Code: tea.KeyTab}, tea.KeyPressMsg{Code: tea.KeyPgDown}, tea.KeyPressMsg{Code: tea.KeyPgDown})
	if strings.Contains(screen(m), "box 00") {
		t.Fatal("the test needs the box off the screen")
	}
	m = update(m, letter('x'), letter('x'))
	if len(ops) != 1 {
		t.Fatalf("two x sent %d ops:\n%s", len(ops), screen(m))
	}
}

// Review of relay#110's rebuild: tab and shift+tab had stopped moving
// through a card's list of links (L).
func TestTabMovesThroughTheLinks(t *testing.T) {
	c := tallNote(1, 1)
	c.Note.Text, c.Note.Outsider = "see hxxps://two[.]example/ and hxxps://three[.]example/", true
	c.Links = []core.Link{{Address: "https://two.example/", Shown: "hxxps://two[.]example/"}, {Address: "https://three.example/", Shown: "hxxps://three[.]example/"}}
	var ops []core.CardOp
	m := tallSingle(t, &ops, 100, 30, c)
	m = update(m, tea.KeyPressMsg{Code: 'L', Text: "L", Mod: tea.ModShift})
	if m.app().Board.Links == nil {
		t.Fatalf("no list of links:\n%s", screen(m))
	}
	if m = update(m, tea.KeyPressMsg{Code: tea.KeyTab}); m.app().Board.Links.Sel != 1 {
		t.Fatalf("tab: link %d", m.app().Board.Links.Sel)
	}
	if m = update(m, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}); m.app().Board.Links.Sel != 0 {
		t.Fatalf("shift+tab: link %d", m.app().Board.Links.Sel)
	}
}

// Review round five of relay#110's rebuild: in the narrow tracker layout
// a row's cells come under its name, and a row counted as on the screen
// once its first row was. With a long name, x marked a cell below the
// window. A line counts only when all of it is on the screen.
func TestACellBelowTheWindowIsNotMarked(t *testing.T) {
	c := core.Card{ID: 1, Rev: 1, Kind: core.CardTracker, Status: core.CardActive, Title: core.CardText{Text: "Tracker", Rev: 1},
		Rows: []core.CardElem{{ID: 1, CardText: core.CardText{Text: strings.Repeat("long row ", 120) + "TAIL", Rev: 1}}}}
	for i := range 8 {
		c.Columns = append(c.Columns, core.CardElem{ID: i + 1, CardText: core.CardText{Text: fmt.Sprintf("Week%02d", i+1), Rev: 1}})
	}
	var ops []core.CardOp
	m := tallSingle(t, &ops, 80, 24, c)
	m = update(m, tea.KeyPressMsg{Code: tea.KeyDown}, tea.KeyPressMsg{Code: tea.KeyRight})
	before := screen(m)
	if strings.Contains(before, "Week02") {
		t.Fatalf("the test needs the cells off the screen:\n%s", before)
	}
	if m = update(m, letter('x')); len(ops) != 0 {
		t.Fatalf("x marked a cell off the screen: %+v\n%s", ops, before)
	}
}

// On a terminal smaller than relay-tui draws in, it says so and takes no
// key but ctrl+c: nothing it would draw can be acted on. Review round four
// of relay#110 found the hub cutting a pane at 170 by 2 after the pane
// had drawn a box, which x then ticked.
func TestATerminalTooSmallTakesNoKeys(t *testing.T) {
	for _, mode := range []string{"hub", "single"} {
		var ops []core.CardOp
		build := tallHub
		if mode == "single" {
			build = tallSingle
		}
		m := build(t, &ops, 170, 30, tallSurface(t, 1, []int{0}, 0))
		m = update(m, tea.KeyPressMsg{Code: tea.KeyTab})
		for _, size := range [][2]int{{170, 2}, {79, 30}, {170, 23}} {
			small := update(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			if got := small.View().Content; !strings.Contains(got, "Terminal too small") || strings.Contains(got, "box 00") {
				t.Fatalf("%s at %v: %q", mode, size, got)
			}
			small = update(small, letter('x'), letter('X'), letter('y'), tea.KeyPressMsg{Code: tea.KeyEsc})
			if len(ops) != 0 || !small.app().Board.Detail || small.app().Board.Confirm != 0 {
				t.Fatalf("%s at %v: a key acted: ops %+v board %+v", mode, size, ops, small.app().Board)
			}
		}
	}
}

// tallButtons is a surface card of gap lines of text, a button that tells
// the role, and tail more lines of text.
func tallButtons(t tb, id int64, gap, tail int) core.Card {
	t.Helper()
	return tallButtonLabelled(t, id, gap, tail, "Plan it")
}

// tallButtonLabelled is tallButtons with the button's label given.
func tallButtonLabelled(t tb, id int64, gap, tail int, label string) core.Card {
	t.Helper()
	comps := map[string]a2ui.Component{}
	var kids []any
	addText := func(n int) {
		for range n {
			cid := fmt.Sprintf("t%03d", len(comps))
			comps[cid] = a2ui.Component{ID: cid, Type: "Text", Props: map[string]any{"text": fmt.Sprintf("text %03d", len(comps))}}
			kids = append(kids, cid)
		}
	}
	addText(gap)
	comps["label"] = a2ui.Component{ID: "label", Type: "Text", Props: map[string]any{"text": label}}
	comps["go"] = a2ui.Component{ID: "go", Type: "Button", Props: map[string]any{"child": "label",
		"action": map[string]any{"event": map[string]any{"name": "plan"}}}}
	kids = append(kids, "go")
	addText(tail)
	comps["root"] = a2ui.Component{ID: "root", Type: "Column", Props: map[string]any{"children": kids}}
	s := a2ui.Surface{ID: fmt.Sprintf("card_%d", id), CatalogID: a2ui.RelayCatalogID, Components: comps, Data: map[string]any{}}
	if err := a2ui.Resolved(s); err != nil {
		t.Fatal(err)
	}
	return core.Card{ID: id, Rev: 1, Kind: core.CardSurface, Status: core.CardActive, Owner: "steward",
		Title: core.CardText{Text: "Trip prep", Outsider: true, Rev: 1}, Text: "a surface",
		Surface: &core.RoleSurface{Outsider: true, A2UI: s}}
}

// A button is focused with tab and pressed with x, like a box, and only
// while all of it is on the screen: with the button scrolled out of sight
// the key brings it back and sends nothing, and a press that is too soon
// after the focus came, or after the button came back, sends nothing
// either.
func TestAButtonInATallCardIsPressedOnlyWhileOnTheScreen(t *testing.T) {
	var ops []core.CardOp
	m := tallHub(t, &ops, 170, 24, tallButtons(t, 1, 2, 60))
	m = update(m, tea.KeyPressMsg{Code: tea.KeyTab}, tea.KeyPressMsg{Code: tea.KeyPgDown}, tea.KeyPressMsg{Code: tea.KeyPgDown})
	if strings.Contains(screen(m), "Plan it") {
		t.Fatalf("the button is still on the screen:\n%s", screen(m))
	}
	// Armed, but off the screen: it comes back and nothing is sent.
	time.Sleep(core.ArmDelay + 100*time.Millisecond)
	m = update(m, letter('x'))
	if len(ops) != 0 || !strings.Contains(screen(m), "Plan it") {
		t.Fatalf("x off the screen: ops %v\n%s", ops, screen(m))
	}
	// It has only just come back: the label has not been on the screen
	// for ArmDelay, so a doubled or held x presses nothing.
	shownAt := time.Now()
	m = update(m, letter('x'))
	if len(ops) != 0 {
		t.Fatalf("pressed %v after the button came on screen: %+v", time.Since(shownAt), ops)
	}
	time.Sleep(core.ArmDelay + 100*time.Millisecond)
	if m = update(m, letter('x')); len(ops) != 1 || ops[0].Op != "press" || *ops[0].Component != "go" || *ops[0].Seen != 1 {
		t.Fatalf("x on the screen: ops %+v", ops)
	}
	// The focus on a button for less than ArmDelay presses nothing.
	ops = nil
	m = tallHub(t, &ops, 170, 24, tallButtons(t, 1, 2, 60))
	m = update(m, tea.KeyPressMsg{Code: tea.KeyTab}, letter('x'))
	if len(ops) != 0 {
		t.Fatalf("a press right after the focus came: %+v", ops)
	}
}

// Scrolling the focused button off the screen and back with the arrow
// keys, with no x between, arms it afresh as well: x at once presses
// nothing, and one ArmDelay later does.
func TestAButtonScrolledAwayAndBackIsArmedAfresh(t *testing.T) {
	var ops []core.CardOp
	m := tallHub(t, &ops, 170, 24, tallButtons(t, 1, 2, 60))
	m = update(m, tea.KeyPressMsg{Code: tea.KeyTab}, tea.KeyPressMsg{Code: tea.KeyPgDown}, tea.KeyPressMsg{Code: tea.KeyPgDown})
	if strings.Contains(screen(m), "Plan it") {
		t.Fatal("the button is still on the screen")
	}
	time.Sleep(core.ArmDelay + 100*time.Millisecond)
	for range 6 {
		m = update(m, tea.KeyPressMsg{Code: tea.KeyUp}, tea.KeyPressMsg{Code: tea.KeyPgUp})
	}
	if !strings.Contains(screen(m), "Plan it") {
		t.Fatalf("not scrolled back:\n%s", screen(m))
	}
	m = update(m, letter('x'))
	if len(ops) != 0 {
		t.Fatalf("pressed right after the button scrolled back: %+v", ops)
	}
	time.Sleep(core.ArmDelay + 100*time.Millisecond)
	if update(m, letter('x')); len(ops) != 1 {
		t.Fatalf("ops %+v", ops)
	}
}

// A resize is no key, but it can take the button off the screen and bring
// it back: that arms the press afresh, as scrolling does.
func TestAButtonThatAResizeBroughtBackIsArmedAfresh(t *testing.T) {
	var ops []core.CardOp
	m := tallHub(t, &ops, 170, 40, tallButtons(t, 1, 22, 60))
	m = update(m, tea.KeyPressMsg{Code: tea.KeyTab})
	time.Sleep(core.ArmDelay + 100*time.Millisecond)
	m = update(m, tea.KeyPressMsg{Code: tea.KeyUp}) // the window stops following; the button is still shown
	m = update(m, tea.WindowSizeMsg{Width: 170, Height: 24})
	if strings.Contains(screen(m), "Plan it") {
		t.Fatalf("the button is still on the screen at 24 rows:\n%s", screen(m))
	}
	m = update(m, tea.WindowSizeMsg{Width: 170, Height: 40})
	if !strings.Contains(screen(m), "Plan it") {
		t.Fatalf("the button is not back on the screen:\n%s", screen(m))
	}
	back := time.Now()
	m = update(m, letter('x'))
	if len(ops) != 0 {
		t.Fatalf("pressed %v after the button came back on screen: %+v", time.Since(back), ops)
	}
}

// A button too tall for the window is not on screen when the focus comes
// to it. It counts as revealed when it first becomes whole, and a press
// is armed from then, not from the focus.
func TestAButtonTooTallToFitIsArmedFromWhenItFirstFits(t *testing.T) {
	var ops []core.CardOp
	label := "Plan it " + strings.Repeat("word ", 150) + "END"
	m := tallHub(t, &ops, 170, 24, tallButtonLabelled(t, 1, 2, 0, label))
	m = update(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if strings.Contains(screen(m), "END") {
		t.Fatalf("the button is whole at 24 rows:\n%s", screen(m))
	}
	time.Sleep(core.ArmDelay + 100*time.Millisecond)
	m = update(m, tea.WindowSizeMsg{Width: 170, Height: 60})
	if !strings.Contains(screen(m), "END") {
		t.Fatalf("the button is not whole at 60 rows:\n%s", screen(m))
	}
	whole := time.Now()
	m = update(m, letter('x'))
	if len(ops) != 0 {
		t.Fatalf("pressed %v after the button was first whole: %+v", time.Since(whole), ops)
	}
	time.Sleep(core.ArmDelay + 100*time.Millisecond)
	if update(m, letter('x')); len(ops) != 1 {
		t.Fatalf("ops %+v", ops)
	}
}

// While the terminal is too small View draws only a message, so none of
// the card is on the screen. Shrinking and growing back brings the button
// back on screen: a press needs ArmDelay from then, in either mode.
func TestAButtonIsNotShownWhileTheTerminalIsTooSmall(t *testing.T) {
	for _, mode := range []string{"hub", "single"} {
		t.Run(mode, func(t *testing.T) {
			var ops []core.CardOp
			card := tallButtons(t, 1, 2, 0)
			m := tallHub(t, &ops, 170, 40, card)
			if mode == "single" {
				m = tallSingle(t, &ops, 170, 40, card)
			}
			m = update(m, tea.KeyPressMsg{Code: tea.KeyTab})
			time.Sleep(core.ArmDelay + 100*time.Millisecond)
			m = update(m, tea.WindowSizeMsg{Width: 79, Height: 40})
			if !strings.Contains(screen2(m), "Terminal too small") {
				t.Fatalf("not too small:\n%s", screen2(m))
			}
			m = update(m, tea.WindowSizeMsg{Width: 170, Height: 40})
			m = update(m, letter('x'))
			if len(ops) != 0 {
				t.Fatalf("pressed right after the terminal grew back: %+v", ops)
			}
			time.Sleep(core.ArmDelay + 100*time.Millisecond)
			if update(m, letter('x')); len(ops) != 1 {
				t.Fatalf("ops %+v", ops)
			}
		})
	}
}

// screen2 is what View draws, as plain text, whatever the size.
func screen2(m Model) string { return ansi.Strip(m.View().Content) }

// A reveal with no key before it (a resize) followed by a wait and one x
// presses: the first x is not refused, however long it was.
func TestAButtonRevealedByAResizeIsPressedByTheFirstXAfterTheWait(t *testing.T) {
	var ops []core.CardOp
	m := tallHub(t, &ops, 170, 40, tallButtons(t, 1, 22, 60))
	m = update(m, tea.KeyPressMsg{Code: tea.KeyTab})
	m = update(m, tea.KeyPressMsg{Code: tea.KeyUp})
	m = update(m, tea.WindowSizeMsg{Width: 170, Height: 24})
	if strings.Contains(screen(m), "Plan it") {
		t.Fatal("the button is still on the screen at 24 rows")
	}
	m = update(m, tea.WindowSizeMsg{Width: 170, Height: 40})
	if !strings.Contains(screen(m), "Plan it") {
		t.Fatal("the button is not back on the screen")
	}
	time.Sleep(core.ArmDelay + 100*time.Millisecond)
	if update(m, letter('x')); len(ops) != 1 {
		t.Fatalf("the first x after the wait: ops %+v", ops)
	}
}
