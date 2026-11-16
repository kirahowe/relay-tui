package render

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/kirahowe/relay-tui/internal/a2ui"
	"github.com/kirahowe/relay-tui/internal/core"
)

// An open card in a short pane: its top and bottom borders are the
// pane's rows under the head and over the foot whatever its body holds,
// so the title, the owner and "from outside" stay on the screen.
func TestATallOpenCardKeepsItsFrameOnTheScreen(t *testing.T) {
	c, _ := surfaceCard(t, "hostile", 1)
	for _, height := range []int{10, 14, 20} {
		s := boardState(true, c)
		for line := 0; line < c.Lines(); line++ {
			s = detail(s, c.ID, line, 0)
			for _, top := range []int{0, 3, 1000} {
				s.Board.Scroll, s.Board.ScrollCard, s.Board.Follow = top, c.ID, true
				out := strings.Join(PaneLines(s.App, s.State, t0, 44, height), "\n")
				flat := ansi.Strip(out)
				for _, want := range []string{"╭─", "╰─", surfaceOwner(c), "from outside"} {
					if !strings.Contains(flat, want) {
						t.Fatalf("height %d, line %d, scroll %d: no %q:\n%s", height, line, top, want, flat)
					}
				}
				if line > 0 && !strings.Contains(out, sgr(selBar)) {
					t.Fatalf("height %d, line %d, scroll %d: the selected line is not drawn:\n%s", height, line, top, flat)
				}
			}
		}
	}
}

// walk presses the key that moves through card c, n times, through the
// real loop: the pane drawn at width by height hands the card as drawn to
// the core with each key. Down on a list, tab on a surface. It gives the
// line focused after each key.
func walk(s core.Single, c core.Card, width, height, n int) []int {
	a := core.PaneDown
	if c.Scrolls() {
		a = core.PaneTab
	}
	var path []int
	for range n {
		_, pd := paneRender(s.App, s.State, t0, width, height)
		s, _ = core.ReduceSingle(s, core.PaneKey{At: t0, Action: a, Card: &pd.Card})
		path = append(path, core.BoardLine(s.Board, c))
	}
	return path
}

// On any pane, the keys that move the focus reach every line, in order:
// what they reach does not depend on what is drawn.
func TestTheFocusReachesEveryLineOnAnyPane(t *testing.T) {
	var cards []core.Card
	for _, name := range []string{"checklist", "hostile"} {
		c, _ := surfaceCard(t, name, 1)
		cards = append(cards, c)
	}
	cards = append(cards, cardTracker, cardChecklist)
	for _, c := range cards {
		for _, width := range []int{30, 44, 60} {
			for height := 1; height <= 30; height++ {
				path := walk(detail(boardState(true, c), c.ID, 0, 0), c, width, height, c.Lines()-1)
				for i, l := range path {
					if l != i+1 {
						t.Fatalf("%s at %dx%d: key %d focused line %d: %v", c.Title.Text, width, height, i+1, l, path)
					}
				}
			}
		}
	}
}

// Review of relay#110's rebuild: page down moved by the window's rows,
// and the marks at its ends hid one row from both pages. Paging down from
// the top shows every row of a note.
func TestPageDownPassesOverNoRow(t *testing.T) {
	var lines []string
	for i := range 80 {
		lines = append(lines, fmt.Sprintf("row %02d", i))
	}
	c := cardNote
	c.Note = &core.CardText{Text: strings.Join(lines, "\n"), Rev: 1}
	for height := 8; height <= 30; height++ {
		s := detail(boardState(true, c), c.ID, 0, 0)
		seen := map[string]bool{}
		for range 100 {
			rows, pd := paneRender(s.App, s.State, t0, 60, height)
			for _, l := range lines {
				if strings.Contains(ansi.Strip(strings.Join(rows, "\n")), l) {
					seen[l] = true
				}
			}
			s, _ = core.ReduceSingle(s, core.PaneKey{At: t0, Action: core.PageNext, Card: &pd.Card})
		}
		for _, l := range lines {
			if !seen[l] {
				t.Fatalf("height %d: page down never showed %q", height, l)
			}
		}
	}
}

// Review of relay#110's rebuild: a checklist item taller than the window
// showed only its first rows, since every key moved the selection. The
// page keys scroll a list too, so the item's tail comes into view.
func TestALongChecklistItemCanBeReadToItsEnd(t *testing.T) {
	c := cardChecklist
	c.Items = []core.CardElem{{ID: 1, CardText: ct(strings.Repeat("word ", 400) + "TAIL")}}
	s := detail(boardState(true, c), c.ID, 0, 0)
	s, _ = core.ReduceSingle(s, core.PaneKey{At: t0, Action: core.PaneDown})
	for range 200 {
		rows, pd := paneRender(s.App, s.State, t0, 60, 12)
		if strings.Contains(ansi.Strip(strings.Join(rows, "\n")), "TAIL") {
			return
		}
		s, _ = core.ReduceSingle(s, core.PaneKey{At: t0, Action: core.PageNext, Card: &pd.Card})
	}
	t.Fatal("page down never brought the item's end into view")
}

// An open card's pane at every height (cardDraw): below the least it
// needs, the pane says it is too small and tells the keys so, and the
// focused line counts as not shown. At or above it, the frame and the
// focused line are on the screen, and the line under the card (a
// tracker's cell) is there whole or not at all. Reviews of relay#110 found
// the key hints, a tracker's long cell line and an empty surface each
// crowding the card out of a short pane, and a cell line cut in half.
func TestAnOpenCardIsDrawnWholeOrSaysThePaneIsTooSmall(t *testing.T) {
	long := cardTracker
	long.Rows = []core.CardElem{{ID: 1, CardText: ct(strings.Repeat("long row ", 20) + "TAIL")}}
	long.Columns, long.Cells = nil, nil
	for i := range 8 {
		long.Columns = append(long.Columns, core.CardElem{ID: i + 1, CardText: ct(fmt.Sprintf("Week%02d", i+1))})
	}
	empty := plainSurface(t, 1, true, "")
	empty.Owner = "steward"
	empty.Surface.A2UI.Components = map[string]a2ui.Component{"root": {ID: "root", Type: "Column", Props: map[string]any{"children": []any{}}}}
	hostile, _ := surfaceCard(t, "hostile", 1)
	for _, c := range []core.Card{cardTracker, long, cardChecklist, cardNote, empty, hostile} {
		for _, width := range []int{30, 44, 60} {
			for height := 1; height <= 16; height++ {
				line := min(1, c.Lines()-1)
				s := detail(boardState(true, c), c.ID, line, 0)
				pr := paneParts(s.App, s.State, t0, width)
				rows, pd := paneRender(s.App, s.State, t0, width, height)
				flat := ansi.Strip(strings.Join(rows, "\n"))
				least := len(pr.head) + len(pr.foot) + 2 + 3
				where := fmt.Sprintf("%s at %dx%d", c.Title.Text, width, height)
				if height < least {
					if !pd.Card.Small || pd.Card.Shown && line > 0 || height > len(pr.head) && !strings.Contains(flat, "Too small") {
						t.Fatalf("%s, under %d rows: small %v, shown %v:\n%s", where, least, pd.Card.Small, pd.Card.Shown, flat)
					}
					continue
				}
				if pd.Card.Small || !strings.Contains(flat, "╭") || !strings.Contains(flat, "╰") || !pd.Card.Shown {
					t.Fatalf("%s: small %v, shown %v:\n%s", where, pd.Card.Small, pd.Card.Shown, flat)
				}
				if info := strings.Join(pr.info, ""); info != "" && strings.Contains(flat, "cell:") {
					for _, l := range pr.info {
						if !strings.Contains(flat, strings.TrimSpace(ansi.Strip(l))) {
							t.Fatalf("%s: the cell line is cut:\n%s", where, flat)
						}
					}
				}
			}
		}
	}
}
