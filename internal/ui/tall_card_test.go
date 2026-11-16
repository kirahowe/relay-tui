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
		width = 60
	}
	var m Model
	if strings.HasSuffix(mode, "hub") {
		m = tallHub(t, &ops, width, 20, tallNote(1, 60))
	} else {
		m = tallSingle(t, &ops, width, 20, tallNote(1, 60))
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
	m := tallHub(t, &ops, 170, 20, c)
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
		m := tallHub(t, &ops, 170, 20, tallSurface(t, 1, tc.gaps, tc.tail))
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
		height := rapid.IntRange(8, 40).Draw(rt, "height")
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

// On a pane too short to draw the card's body, tab focuses a box nobody
// can see, and x sends nothing, whether the pane was short from the
// start or became short after the box was focused.
func TestABoxOffTheScreenIsNotSent(t *testing.T) {
	for _, resize := range []bool{false, true} {
		var ops []core.CardOp
		height := 10
		if resize {
			height = 20
		}
		m := tallSingle(t, &ops, 60, height, tallSurface(t, 1, []int{0}, 0))
		m = update(m, tea.KeyPressMsg{Code: tea.KeyTab})
		if resize {
			m = update(m, tea.WindowSizeMsg{Width: 60, Height: 10})
		}
		before := screen(m)
		m = update(m, letter('x'))
		if !strings.Contains(before, "box 00") && len(ops) > 0 {
			t.Fatalf("resize %v: a box off the screen was sent: %+v\n%s", resize, ops[0], before)
		}
	}
}

// Tab to a box, scroll it out of sight, press x: the box comes back into
// view and nothing is sent. The next x ticks it.
func TestXOnABoxScrolledAwayBringsItBack(t *testing.T) {
	var ops []core.CardOp
	m := tallHub(t, &ops, 170, 20, tallSurface(t, 1, []int{2}, 60))
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
