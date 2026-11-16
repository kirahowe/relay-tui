package render

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
	"pgregory.net/rapid"

	"github.com/kirahowe/relay-tui/internal/a2ui"
	"github.com/kirahowe/relay-tui/internal/a2ui/a2uitest"
	"github.com/kirahowe/relay-tui/internal/core"
)

// genResolved is a surface relay could send, its strings from the hostile
// pool.
func genResolved(t *rapid.T) a2ui.Surface { return a2uitest.GenResolved(t, genSurfaceText()) }

// A surface that passes a2ui.Resolved draws whole: no placeholder, at any
// width. With the focus on one of its inputs, the rows the drawing reports
// are inside the drawing and not empty; with no focus, there are none.
func TestSurfaceRowsOfAResolvedSurface(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := genResolved(t)
		if err := a2ui.Resolved(s); err != nil {
			t.Fatalf("generated surface fails Resolved: %v", err)
		}
		width := rapid.IntRange(1, 120).Draw(t, "width")
		outsider := rapid.Bool().Draw(t, "outsider")
		live := rapid.SampledFrom(liveSets).Draw(t, "live")
		rows, a, b, holes := surfaceDrawing(s, outsider, live, width, nil)
		if holes != 0 {
			t.Fatalf("width %d: %d placeholders in %q", width, holes, rows)
		}
		if a != 0 || b != 0 {
			t.Fatalf("no focus, but rows %d to %d", a, b)
		}
		ins := s.Inputs()
		if len(ins) == 0 {
			return
		}
		in := rapid.SampledFrom(ins).Draw(t, "input")
		rows, a, b, holes = surfaceDrawing(s, outsider, live, width, &a2ui.Focus{ID: in.ID})
		if holes != 0 {
			t.Fatalf("focused: %d placeholders", holes)
		}
		if !(0 <= a && a < b && b <= len(rows)) {
			t.Fatalf("width %d: focused rows %d to %d of %d", width, a, b, len(rows))
		}
	})
}

func TestSurfaceDrawingCountsPlaceholders(t *testing.T) {
	s := surfaceOf(t, a2ui.BasicCatalogID, `[{"id":"root","component":"Column","children":["i","gone","t"]},
 {"id":"i","component":"Image","url":"https://x.test/a.png"},
 {"id":"t","component":"Text","text":"fine"}]`, "")
	if _, _, _, n := surfaceDrawing(s, true, core.Links{}, 40, nil); n != 2 {
		t.Fatalf("%d placeholders, want 2", n)
	}
	if _, _, _, n := surfaceDrawing(a2ui.Surface{}, true, core.Links{}, 40, nil); n != 1 {
		t.Fatalf("%d placeholders for a surface with no root, want 1", n)
	}
	ok := surfaceOf(t, a2ui.RelayCatalogID, `[{"id":"root","component":"Text","text":"fine"}]`, "")
	if _, _, _, n := surfaceDrawing(ok, true, core.Links{}, 40, nil); n != 0 {
		t.Fatalf("%d placeholders", n)
	}
}

// Rows nested 12 deep with children at every level, about 200 components
// in all, draw whole at narrow, board and wide widths, and fast: a Row
// asks its children for their width without drawing them.
func TestSurfaceRowsAtRelaysLimitsDrawWhole(t *testing.T) {
	var comps []string
	id := "root"
	for i := range 12 {
		next := "r" + strconv.Itoa(i)
		kids := []string{next}
		for j := range 14 {
			leaf := fmt.Sprintf("l%d_%d", i, j)
			kids = append(kids, leaf)
			if j%2 == 0 {
				comps = append(comps, fmt.Sprintf(`{"id":%q,"component":"Text","text":"leaf %d %d"}`, leaf, i, j))
			} else {
				comps = append(comps, fmt.Sprintf(`{"id":%q,"component":"CheckBox","label":"item %d %d","value":false}`, leaf, i, j))
			}
		}
		comps = append(comps, fmt.Sprintf(`{"id":%q,"component":"Row","children":["%s"]}`, id, strings.Join(kids, `","`)))
		id = next
	}
	comps = append(comps, fmt.Sprintf(`{"id":%q,"component":"Text","text":"end"}`, id))
	s := surfaceOf(t, a2ui.RelayCatalogID, "["+strings.Join(comps, ",")+"]", "")
	if err := a2ui.Resolved(s); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	for _, w := range []int{20, 46, 120} {
		rows, _, _, d := drawSurface(s, false, core.Links{}, w, nil)
		if d.placeholders != 0 {
			t.Fatalf("width %d: %d placeholders in %q", w, d.placeholders, rows)
		}
		if d.visits > len(s.Components) {
			t.Fatalf("width %d: %d visits for %d components", w, d.visits, len(s.Components))
		}
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("took %v", took)
	}
}

// Each component the root reaches is drawn once, so a component id names
// one drawn component.
func TestSurfaceRowsDrawEachComponentOnce(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := genResolved(t)
		reached := 0
		s.Walk(func(a2ui.Node) { reached++ })
		width := rapid.IntRange(1, 120).Draw(t, "width")
		_, _, _, d := drawSurface(s, rapid.Bool().Draw(t, "outsider"), core.Links{}, width, nil)
		if d.visits > reached {
			t.Fatalf("width %d: %d visits, the root reaches %d", width, d.visits, reached)
		}
	})
}

// The focus is a look, not a layout: with any input focused, the rows are
// the rows drawn with no focus once styles and trailing spaces are gone.
func TestSurfaceRowsFocusDoesNotChangeTheLayout(t *testing.T) {
	plain := func(rows []string) []string {
		out := make([]string, len(rows))
		for i, r := range rows {
			out[i] = strings.TrimRight(ansiPlain(osc8RE.ReplaceAllString(r, "")), " ")
		}
		return out
	}
	rapid.Check(t, func(t *rapid.T) {
		s := genResolved(t)
		ins := s.Inputs()
		if len(ins) == 0 {
			return
		}
		width := rapid.IntRange(1, 120).Draw(t, "width")
		outsider := rapid.Bool().Draw(t, "outsider")
		live := rapid.SampledFrom(liveSets).Draw(t, "live")
		bare, _, _ := surfaceRows(s, outsider, live, width, nil)
		in := rapid.SampledFrom(ins).Draw(t, "input")
		with, _, _ := surfaceRows(s, outsider, live, width, &a2ui.Focus{ID: in.ID})
		if a, b := plain(bare), plain(with); !slices.Equal(a, b) {
			t.Fatalf("width %d, focus %s:\nbare %q\nwith %q", width, in.ID, a, b)
		}
	})
}

// selection surfaces: a caption, a boxed card with a checkbox that wraps
// to three rows, and a row of two checkboxes.
const selectionComponents = `[
 {"id":"root","component":"Column","children":["cap","card","row"]},
 {"id":"cap","component":"Text","variant":"caption","text":"two lines\nof caption"},
 {"id":"card","component":"Card","child":"inner"},
 {"id":"inner","component":"Column","children":["x","long"]},
 {"id":"x","component":"CheckBox","label":"short","value":{"@path":"/x"}},
 {"id":"long","component":"CheckBox","label":"a label long enough to need two rows","value":{"@path":"/long"}},
 {"id":"row","component":"Row","children":["p","q"]},
 {"id":"p","component":"CheckBox","label":"left","value":true},
 {"id":"q","component":"CheckBox","label":"right","value":{"@path":"/q"}}]`

func TestSurfaceRowsReportTheFocusedRows(t *testing.T) {
	s := surfaceOf(t, a2ui.RelayCatalogID, selectionComponents, `{"x":false,"long":true,"q":false}`)
	for _, c := range []struct {
		id         string
		selA, selB int
	}{
		{"x", 3, 4},    // 2 caption rows, the card's top border, then x
		{"long", 4, 7}, // three rows
		{"q", 8, 9},    // after the card's bottom border at 7
	} {
		rows, a, b := surfaceRows(s, false, core.Links{}, 24, &a2ui.Focus{ID: c.id})
		if a != c.selA || b != c.selB {
			t.Errorf("%s: rows %d to %d, want %d to %d\n%s", c.id, a, b, c.selA, c.selB, strings.Join(rows, "\n"))
		}
		for i, r := range rows {
			if barred := strings.Contains(r, sgr(selBar)); barred != (i >= a && i < b) {
				t.Errorf("%s: row %d bar is %v: %q", c.id, i, barred, r)
			}
		}
	}
}

// A Card that is the root has no box; one further down has.
func TestSurfaceRowsRootCardHasNoFrame(t *testing.T) {
	root := surfaceOf(t, a2ui.RelayCatalogID, `[{"id":"root","component":"Card","child":"t"},{"id":"t","component":"Text","text":"hello"}]`, "")
	rows, _, _ := surfaceRows(root, false, core.Links{}, 30, nil)
	if got := ansiPlain(strings.Join(rows, "\n")); got != "hello" {
		t.Fatalf("root card draws %q", got)
	}
	inner := surfaceOf(t, a2ui.RelayCatalogID, `[{"id":"root","component":"Column","children":["c"]},{"id":"c","component":"Card","child":"t"},{"id":"t","component":"Text","text":"hello"}]`, "")
	rows, _, _ = surfaceRows(inner, false, core.Links{}, 30, nil)
	if len(rows) != 3 || !strings.Contains(rows[0], "╭") || !strings.Contains(rows[2], "╯") {
		t.Fatalf("inner card draws %q", rows)
	}
}

// A Row, and a horizontal List, keep two columns between children, so two
// strings are never flush.
func TestSurfaceRowsKeepTwoColumnsBetweenChildren(t *testing.T) {
	for _, kind := range []string{`"component":"Row"`, `"component":"List","direction":"horizontal"`} {
		s := surfaceOf(t, a2ui.RelayCatalogID, `[{"id":"root",`+kind+`,"children":["a","b","c"]},
 {"id":"a","component":"Text","text":"hxxps://ev"},{"id":"b","component":"Text","text":"il[.]example"},
 {"id":"c","component":"CheckBox","label":"c","value":false}]`, "")
		for width := 6; width <= 60; width++ {
			rows, _, _ := surfaceRows(s, true, core.Links{}, width, nil)
			for _, r := range rows {
				plain := ansiPlain(r)
				if strings.Contains(plain, "hxxps://evil") || strings.Contains(plain, "ev il") {
					t.Fatalf("%s, width %d: strings flush in %q", kind, width, plain)
				}
				if i := strings.Index(plain, "hxxps://ev"); i >= 0 && strings.Contains(plain, "il[.]") {
					if !strings.HasPrefix(plain[i+len("hxxps://ev"):], "  ") {
						t.Fatalf("%s, width %d: less than two columns in %q", kind, width, plain)
					}
				}
			}
		}
	}
}

// Only the body variant is dressed as a note, and a caption keeps its
// line breaks. Neither reads Markdown.
func TestSurfaceRowsDressText(t *testing.T) {
	s := surfaceOf(t, a2ui.RelayCatalogID, `[{"id":"root","component":"Column","children":["b","c","m"]},
 {"id":"b","component":"Text","text":"one **two**\n# head\n- item"},
 {"id":"c","component":"Text","variant":"caption","text":"first\n\nthird **x**"},
 {"id":"m","component":"Text","text":"[a link](https://evil.test/x) and *it* and `+"`code`"+`"}]`, "")
	rows, _, _ := surfaceRows(s, false, core.Links{}, 60, nil)
	plain := ansiPlain(strings.Join(rows, "\n"))
	want := "one two\nhead\n- item\nfirst\n\nthird **x**\n[a link](https://evil.test/x) and *it* and `code`"
	if plain != want {
		t.Fatalf("got\n%s\nwant\n%s", plain, want)
	}
	if !strings.Contains(rows[0], sgr(bold)+"two") {
		t.Errorf("**two** is not bold: %q", rows[0])
	}
}

// Dressing a Text takes characters away after relay checked the string, so
// an outsider's text is drawn as sent: asterisks stay, and no address
// forms. The same string from a surface that is not from outside is
// dressed.
func TestSurfaceRowsDoNotDressAnOutsidersText(t *testing.T) {
	const text = "ev**il.**example/steal?d=1"
	s := surfaceOf(t, a2ui.RelayCatalogID, `[{"id":"root","component":"Text","text":"`+text+`"}]`, "")
	rows, _, _ := surfaceRows(s, true, core.Links{}, 60, nil)
	if got := ansiPlain(strings.Join(rows, "\n")); !strings.Contains(got, "**") || strings.Contains(got, "evil.example") || strings.Contains(got, "evil[.]example") {
		t.Fatalf("outsider: %q", got)
	}
	rows, _, _ = surfaceRows(s, false, core.Links{}, 60, nil)
	got := ansiPlain(strings.Join(rows, "\n"))
	if strings.Contains(got, "*") || !strings.Contains(got, "evil.example/steal?d=1") {
		t.Fatalf("not outsider: %q", got)
	}
	if !strings.Contains(rows[0], sgr(bold)+"il.") {
		t.Fatalf("not outsider, no bold: %q", rows[0])
	}
}

// Drawing an outsider's text only wraps it: every character of the safe
// text appears in the rows, in order, with the marks a dresser looks for
// among them.
func TestSurfaceRowsOnlyWrapAnOutsidersText(t *testing.T) {
	marks := rapid.SampledFrom([]string{"**", "# ", "- ", "\n# ", "\n- ", "*", "ev**il.**example/x", "<" + keptLink + ">"})
	gen := rapid.Map(rapid.SliceOfN(rapid.OneOf(genSurfaceText(), marks), 0, 6), func(p []string) string { return strings.Join(p, "") })
	nonSpace := func(s string) string {
		s = osc8RE.ReplaceAllString(ansiPlain(s), "")
		return strings.Join(strings.Fields(s), "")
	}
	rapid.Check(t, func(t *rapid.T) {
		raw := gen.Draw(t, "text")
		live := rapid.SampledFrom(liveSets).Draw(t, "live")
		// A column that holds no whole character shows none, so the width
		// is wider than any character the generator makes.
		width := rapid.IntRange(8, 120).Draw(t, "width")
		comp := a2ui.Component{ID: "root", Type: "Text", Props: map[string]any{"text": raw}}
		s := a2ui.Surface{ID: "s", CatalogID: a2ui.RelayCatalogID, Components: map[string]a2ui.Component{"root": comp}}
		rows, _, _ := surfaceRows(s, true, live, width, nil)
		want := nonSpace(cardText(raw, true, live))
		got := nonSpace(strings.Join(rows, "\n"))
		// A hard wrap can split a broken link right after its scheme. drawn
		// then shows that row defanged, which is longer than the row, and
		// fit cuts it with an ellipsis. That row is the only one that loses
		// characters.
		if strings.Contains(got, "…") && !strings.Contains(want, "…") {
			t.Skip()
		}
		// A link shows broken where it is not live or does not fit its row,
		// so the marks that break one are set aside on both sides.
		unbreak := strings.NewReplacer("hxxp", "http", "[:]", ":", "[.]", ".")
		if !isSubsequence(unbreak.Replace(want), unbreak.Replace(got)) {
			t.Fatalf("width %d: %q does not hold %q", width, got, want)
		}
	})
}

// isSubsequence says every rune of sub appears in s, in order.
func isSubsequence(sub, s string) bool {
	rest := []rune(sub)
	for _, r := range s {
		if len(rest) > 0 && rest[0] == r {
			rest = rest[1:]
		}
	}
	return len(rest) == 0
}

// Outside text is in the warning colour, as an outsider's note is on the
// board; text that is not from outside is not.
func TestSurfaceRowsColourOutsideText(t *testing.T) {
	s := surfaceOf(t, a2ui.RelayCatalogID, `[{"id":"root","component":"Column","children":["b","c"]},
 {"id":"b","component":"Text","text":"body"},{"id":"c","component":"Text","variant":"caption","text":"caption"}]`, "")
	for _, outsider := range []bool{true, false} {
		rows, _, _ := surfaceRows(s, outsider, core.Links{}, 40, nil)
		for _, r := range rows {
			if got := strings.Contains(r, sgr(warnColour)); got != outsider {
				t.Errorf("outsider %v: warning colour is %v in %q", outsider, got, r)
			}
		}
	}
}

// A broken link that a hard wrap splits right after its scheme leaves a
// row that starts "://", which a terminal would follow. The row is shown
// broken, for a caption and for a CheckBox label, outside the dashboard
// and on the board.
func TestSurfaceRowsBrokenLinkWrappedAfterTheSchemeIsNotFollowable(t *testing.T) {
	const link = "hxxps://evil[.]test/apply?who=kira"
	for _, c := range []struct{ name, comp string }{
		{"caption", `{"id":"root","component":"Text","variant":"caption","text":"` + link + `"}`},
		{"body", `{"id":"root","component":"Text","text":"` + link + `"}`},
		{"checkbox", `{"id":"root","component":"CheckBox","label":"` + link + `","value":false}`},
	} {
		s := surfaceOf(t, a2ui.RelayCatalogID, "["+c.comp+"]", "")
		for width := 1; width <= 40; width++ {
			rows, _, _ := surfaceRows(s, true, core.Links{}, width, nil)
			for _, row := range append(rows, A2UI(s, width, nil)...) {
				if followable(ansi.Strip(row)) {
					t.Fatalf("%s, width %d: unsafe row %q", c.name, width, row)
				}
			}
		}
	}
}

// card is a surface card as relay sends it, with the live links relay
// lists beside it. Its pair is what a role wrote, which says which links
// the connection keeps live.
type card struct {
	Surface struct {
		Outsider bool             `json:"outsider"`
		A2UI     []jsontext.Value `json:"a2ui"`
	} `json:"surface"`
	Links []struct {
		Address string `json:"address"`
		Shown   string `json:"shown"`
	} `json:"links"`
}

// readCard reads relay's fixture card of that name: its surface, whether
// it is from outside, and the links to draw it with, which are the
// connection's set from the role's file and the card's own list.
func readCard(t testing.TB, name string) (a2ui.Surface, bool, core.Links) {
	t.Helper()
	read := func(file string, into any) {
		raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "surfaces", file))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, into); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
	}
	var c card
	read(name+".card.json", &c)
	var role struct {
		Live []string `json:"live"`
	}
	read(name+".json", &role)
	var ss a2ui.Surfaces
	for _, raw := range c.Surface.A2UI {
		m, err := a2ui.Decode(raw)
		if err != nil {
			t.Fatal(err)
		}
		if ss, err = a2ui.Apply(ss, m); err != nil {
			t.Fatal(err)
		}
	}
	if len(ss) != 1 {
		t.Fatalf("%s: %d surfaces", name, len(ss))
	}
	var own []core.Link
	for _, l := range c.Links {
		own = append(own, core.Link{Address: l.Address, Shown: l.Shown})
	}
	return ss[0], c.Surface.Outsider, core.LinksOf(role.Live...).Of(own)
}

// relay's fixture cards each draw whole at the widths of the board's
// list and its detail. The goldens show ESC as ␛ and BEL as ␇.
func TestSurfaceCardsGolden(t *testing.T) {
	for _, name := range []string{"checklist", "note", "hostile", "buttons"} {
		s, outsider, live := readCard(t, name)
		if err := a2ui.Resolved(s); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, w := range []int{46, 72} {
			t.Run(name+"_"+strconv.Itoa(w), func(t *testing.T) {
				rows, _, _, holes := surfaceDrawing(s, outsider, live, w, nil)
				if holes != 0 {
					t.Fatalf("%d placeholders", holes)
				}
				out := strings.ReplaceAll(strings.ReplaceAll(strings.Join(rows, "\n"), "\x1b", "␛"), "\x07", "␇")
				golden.RequireEqual(t, out+"\n")
			})
		}
	}
}

// Of everything the hostile card holds, one hyperlink reaches the
// terminal: the live link. Its two half-addresses stay apart.
func TestSurfaceCardsHostileKeepsOneLink(t *testing.T) {
	s, outsider, live := readCard(t, "hostile")
	if !outsider {
		t.Fatal("the hostile card is not from outside")
	}
	for _, w := range []int{46, 72} {
		rows, _, _ := surfaceRows(s, outsider, live, w, nil)
		out := gate(strings.Join(rows, "\n"), live)
		var targets []string
		for _, m := range osc8RE.FindAllStringSubmatch(out, -1) {
			if m[1] != "" {
				targets = append(targets, m[1])
			}
		}
		if len(targets) != 1 || targets[0] != "https://kept.example/fund" {
			t.Errorf("width %d: hyperlinks %q", w, targets)
		}
		for _, r := range strings.Split(out, "\n") {
			if followable(ansi.Strip(r)) {
				t.Errorf("width %d: unsafe row %q", w, r)
			}
		}
	}
}

// Children of a Row that cannot all keep their text stack instead of
// being cut to an ellipsis.
func TestSurfaceRowsStackChildrenThatWouldLoseText(t *testing.T) {
	box := func(i int, label string) string {
		return fmt.Sprintf(`{"id":"b%d","component":"CheckBox","label":%q,"value":false}`, i, label)
	}
	var comps, ids []string
	for i, l := range strings.Split("abcdefghijklmnop", "") {
		comps = append(comps, box(i, l))
		ids = append(ids, fmt.Sprintf("b%d", i))
	}
	for _, c := range []struct {
		name  string
		n     int
		width int
	}{{"three at width seven", 3, 7}, {"sixteen at width forty six", 16, 46}} {
		t.Run(c.name, func(t *testing.T) {
			s := surfaceOf(t, a2ui.RelayCatalogID, `[{"id":"root","component":"Row","children":["`+strings.Join(ids[:c.n], `","`)+`"]},`+strings.Join(comps[:c.n], ",")+`]`, "")
			rows, _, _ := surfaceRows(s, false, core.Links{}, c.width, nil)
			plain := ansiPlain(strings.Join(rows, "\n"))
			if strings.Contains(plain, "…") || strings.Count(plain, "○") != c.n {
				t.Fatalf("drawing\n%s", plain)
			}
			for i := range c.n {
				if !strings.Contains(plain, "○ "+string(rune('a'+i))) {
					t.Fatalf("label %c is not after its mark:\n%s", 'a'+i, plain)
				}
			}
		})
	}
}

// The allotment gives every child at least its floor and fits the room,
// whatever the wants and weights, when the floors fit.
func TestAllotKeepsEveryFloorAndFitsTheRoom(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 8).Draw(t, "n")
		want, floor, weight := make([]int, n), make([]int, n), make([]float64, n)
		for i := range n {
			want[i] = rapid.IntRange(1, 40).Draw(t, "want")
			floor[i] = rapid.IntRange(1, want[i]).Draw(t, "floor")
			weight[i] = rapid.SampledFrom([]float64{0, 0, 1, 2.5}).Draw(t, "weight")
		}
		avail := sum(floor) + rapid.IntRange(0, 60).Draw(t, "spare")
		got := allot(want, floor, weight, avail)
		if len(got) != n || sum(got) > avail {
			t.Fatalf("shares %v do not fit %d", got, avail)
		}
		for i := range n {
			if got[i] < floor[i] {
				t.Fatalf("share %d of child %d is under its floor %d (wants %v)", got[i], i, floor[i], want)
			}
		}
	})
}

// For surfaces with short labels, every input's label is drawn whole, right
// after its mark (a button's between its brackets), at any width from ten,
// and no row is only an ellipsis.
func TestSurfaceRowsKeepEveryLabel(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := a2uitest.GenResolved(t, rapid.StringMatching(`[a-z]{1,2}`))
		width := rapid.IntRange(10, 120).Draw(t, "width")
		rows, _, _ := surfaceRows(s, rapid.Bool().Draw(t, "outsider"), core.Links{}, width, nil)
		plain := ansiPlain(strings.Join(rows, "\n"))
		for _, r := range strings.Split(plain, "\n") {
			if strings.TrimSpace(r) == "…" {
				t.Fatalf("a row is only an ellipsis at width %d:\n%s", width, plain)
			}
		}
		for _, in := range s.Inputs() {
			if in.Button {
				if !strings.Contains(plain, "[ "+in.Label+" ]") {
					t.Fatalf("width %d: button %q is not drawn between brackets:\n%s", width, in.Label, plain)
				}
				continue
			}
			if !strings.Contains(plain, "✓ "+in.Label) && !strings.Contains(plain, "○ "+in.Label) {
				t.Fatalf("width %d: label %q is not after a mark:\n%s", width, in.Label, plain)
			}
		}
	})
}

// What a child of a Row needs comes from what it draws: a cluster of four
// cells, or a list line whose body hangs beside its marker. At a narrow
// width the drawing holds every character the wide one does.
func TestSurfaceRowsStackChildrenWhoseContentNeedsTheRoom(t *testing.T) {
	for _, c := range []struct {
		name  string
		comps string
		width int
		want  []string
	}{
		{"an emoji cluster beside a word", `[{"id":"root","component":"Row","children":["b","t"]},
 {"id":"b","component":"CheckBox","label":"👍🏽","value":false},
 {"id":"t","component":"Text","text":"abcdefgh"}]`, 10, []string{"○ 👍🏽", "abcdefgh"}},
		{"a list line beside a word", `[{"id":"root","component":"Row","children":["a","z"]},
 {"id":"a","component":"Text","text":"- abc"},
 {"id":"z","component":"Text","text":"zzzz"}]`, 6, []string{"abc", "zzzz"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := surfaceOf(t, a2ui.RelayCatalogID, c.comps, "")
			rows, _, _ := surfaceRows(s, false, core.Links{}, c.width, nil)
			wide, _, _ := surfaceRows(s, false, core.Links{}, 200, nil)
			plain := ansiPlain(strings.Join(rows, "\n"))
			for _, want := range c.want {
				if !strings.Contains(plain, want) {
					t.Fatalf("lacks %q:\n%s", want, plain)
				}
			}
			if solidRunes(rows) != solidRunes(wide) || strings.Contains(ansiPlain(strings.Join(rows, "\n")), "…") {
				t.Fatalf("narrow drawing lost text:\n%s", strings.Join(rows, "\n"))
			}
		})
	}
}

// solidRunes is the non-space characters of rows, with styles, links and
// the glyphs of boxes and rules taken out, in order of their code points.
// A box and a rule have as many glyphs as the width gives them.
func solidRunes(rows []string) string {
	var out []rune
	for _, r := range osc8RE.ReplaceAllString(ansiPlain(strings.Join(rows, "\n")), "") {
		if unicode.IsSpace(r) || (r >= 0x2500 && r <= 0x257f) {
			continue
		}
		out = append(out, r)
	}
	slices.Sort(out)
	return string(out)
}

// A surface drawn at any width from the narrowest it asks for holds every
// character it holds at a wide width, before drawn breaks a row, so no
// row loses text to a squeeze, whatever the text.
func TestSurfaceRowsLoseNothingAtOrAboveTheirLeast(t *testing.T) {
	text := rapid.Map(rapid.SliceOfN(rapid.OneOf(genSurfaceText(),
		rapid.SampledFrom([]string{"👍🏽", "世界", "é́", "- ", "# ", "**", "- x", "**b**", "# h", "a b c", "abcdefgh"})), 0, 4),
		func(p []string) string { return strings.Join(p, "") })
	rapid.Check(t, func(t *rapid.T) {
		s := a2uitest.GenResolved(t, text)
		outsider := rapid.Bool().Draw(t, "outsider")
		live := rapid.SampledFrom(liveSets).Draw(t, "live")
		_, _, _, wide := drawSurface(s, outsider, live, 4000, nil)
		want := solidRunes(wide.raw)
		d := &surfaceDraw{s: s, outsider: outsider, live: live, labels: buttonLabels(s), budget: a2ui.MaxNodes,
			wanted: map[a2ui.Child]int{}, leasts: map[a2ui.Child]int{}, measuring: map[a2ui.Child]bool{}, needing: map[a2ui.Child]bool{}}
		from := d.least(a2ui.Child{ID: "root"}, 0)
		width := rapid.IntRange(from, max(from, 120)).Draw(t, "width")
		_, _, _, got := drawSurface(s, outsider, live, width, nil)
		if g := solidRunes(fit(append([]string(nil), got.raw...), width)); g != want {
			t.Fatalf("width %d (least %d): drew %q, wide drawing holds %q", width, from, g, want)
		}
	})
}

// A layout weight is a JSON number from the surface. A huge one must not
// overflow the sums that share a row's room: it is capped, and a number
// that is not finite or not positive counts as none. The check is on the
// weight itself and on the allotment, so it does not depend on how the
// architecture converts a float that is not a number to an integer.
func TestAWeightIsCappedAndAnUnusableOneCountsAsNone(t *testing.T) {
	s := surfaceOf(t, a2ui.BasicCatalogID, `[{"id":"root","component":"Row","children":["a","b","c","d"]},
 {"id":"a","component":"Text","text":"left","weight":1e308},
 {"id":"b","component":"Text","text":"right","weight":1e308},
 {"id":"c","component":"Text","text":"neg","weight":-3},
 {"id":"d","component":"Text","text":"none"}]`, "")
	d := &surfaceDraw{s: s}
	if a, c, none := d.weightOf("a"), d.weightOf("c"), d.weightOf("d"); a <= 0 || a > maxWeight || c != 0 || none != 0 {
		t.Fatalf("weights %v %v %v", a, c, none)
	}
	for _, w := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), -1, 0} {
		s.Components["a"].Props["weight"] = w
		if got := d.weightOf("a"); got != 0 {
			t.Errorf("weight %v counts as %v", w, got)
		}
	}
	got := allot([]int{4, 5}, []int{4, 5}, []float64{maxWeight, maxWeight}, 40)
	if got[0] < 4 || got[1] < 5 || sum(got) > 40 {
		t.Fatalf("shares %v", got)
	}
}

func TestARowOfHugeWeightsDrawsWithinItsWidth(t *testing.T) {
	s := surfaceOf(t, a2ui.BasicCatalogID, `[{"id":"root","component":"Row","children":["a","b"]},
 {"id":"a","component":"Text","text":"left","weight":1e308},
 {"id":"b","component":"Text","text":"right","weight":1e308}]`, "")
	for width := 1; width <= 60; width++ {
		for _, l := range A2UI(s, width, nil) {
			if Method.StringWidth(l) > width {
				t.Fatalf("width %d: %q", width, l)
			}
		}
	}
}

// A Button's label is never a hyperlink, even when it is the text of a
// live link: nothing that looks like a button opens an address. The same
// text in a Text is a link, so the test can tell the two apart.
func TestAButtonsLabelIsNeverAHyperlink(t *testing.T) {
	const comps = `[{"id":"root","component":"Column","children":["go","t"]},
 {"id":"go","component":"Button","variant":%q,"child":"label","action":{"event":{"name":"go"}}},
 {"id":"label","component":"Text","text":%q},
 {"id":"t","component":"Text","text":%q}]`
	tb := t
	rapid.Check(t, func(t *rapid.T) {
		label := rapid.SampledFrom([]string{keptLink, "see " + keptLink + " now", "<" + keptLink + ">"}).Draw(t, "label")
		variant := rapid.SampledFrom([]string{"primary", ""}).Draw(t, "variant")
		s := surfaceOf(tb, a2ui.RelayCatalogID, fmt.Sprintf(comps, variant, label, label), "")
		if variant == "" {
			delete(s.Components["go"].Props, "variant")
		}
		if err := a2ui.Resolved(s); err != nil {
			t.Fatal(err)
		}
		width := rapid.IntRange(40, 120).Draw(t, "width")
		live := core.LinksOf(keptLink)
		rows, _, _ := surfaceRows(s, true, live, width, &a2ui.Focus{ID: "go"})
		var buttonRows, textRows []string
		for _, r := range rows {
			if strings.Contains(ansi.Strip(r), "[ ") {
				buttonRows = append(buttonRows, r)
			} else {
				textRows = append(textRows, r)
			}
		}
		if len(buttonRows) == 0 || len(textRows) == 0 {
			t.Fatalf("rows %q", rows)
		}
		for _, r := range buttonRows {
			out := gate(r, live)
			if osc8RE.MatchString(out) || strings.Contains(out, "\x1b]8") {
				t.Fatalf("hyperlink in a button's row %q", out)
			}
			if followable(ansi.Strip(out)) {
				t.Fatalf("a followable address in a button's row %q", ansi.Strip(out))
			}
			if underlined(out) {
				t.Fatalf("underline in a button's row %q", out)
			}
		}
		if !osc8RE.MatchString(gate(strings.Join(textRows, "\n"), live)) {
			t.Fatalf("the Text holding the same link is not a hyperlink: %q", textRows)
		}
	})
}

// underlined says an SGR sequence of s turns on underline.
func underlined(s string) bool {
	for _, m := range sgrRE.FindAllString(s, -1) {
		for _, p := range strings.Split(strings.Trim(m, "\x1b[m"), ";") {
			if p == "4" || strings.HasPrefix(p, "4:") {
				return true
			}
		}
	}
	return false
}
