package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

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

// With room for the borders and no row of body, the frame stays, with the
// title, the owner and "from outside" on it.
func TestTheFrameStaysWhenOnlyItsBordersFit(t *testing.T) {
	const width = 60
	c := plainSurface(t, 1, true, "")
	c.Owner = "steward"
	s := detail(boardState(true, c), c.ID, 0, 0)
	pr := paneParts(s.App, s.State, t0, width)
	height := len(pr.head) + len(pr.foot) + len(wrap(styleKeys(pr.keys), width)) + 2
	rows, _ := paneRender(s.App, s.State, t0, width, height)
	flat := ansi.Strip(strings.Join(rows, "\n"))
	for _, want := range []string{"╭", "╰", "Surface", "steward", "from outside"} {
		if !strings.Contains(flat, want) {
			t.Fatalf("no %q with room for both borders:\n%s", want, flat)
		}
	}
}
