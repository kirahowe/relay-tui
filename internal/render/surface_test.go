package render

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

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

// card is a surface card as relay sends it, and its pair: what a role
// wrote, which says which links are live.
type card struct {
	Surface struct {
		Outsider bool             `json:"outsider"`
		A2UI     []jsontext.Value `json:"a2ui"`
	} `json:"surface"`
}

func readCard(t *testing.T, name string) (a2ui.Surface, bool, core.Links) {
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
	return ss[0], c.Surface.Outsider, core.LinksOf(role.Live...)
}

// relay's three fixture cards each draw whole at the widths of the board's
// list and its detail. The goldens show ESC as ␛ and BEL as ␇.
func TestSurfaceCardsGolden(t *testing.T) {
	for _, name := range []string{"checklist", "note", "hostile"} {
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
