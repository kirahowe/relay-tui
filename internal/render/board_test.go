package render

import (
	"encoding/json/v2"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"pgregory.net/rapid"

	"github.com/kirahowe/relay-tui/internal/a2ui"
	"github.com/kirahowe/relay-tui/internal/a2ui/a2uitest"
	"github.com/kirahowe/relay-tui/internal/core"
)

func ct(s string) core.CardText { return core.CardText{Text: s, Rev: 1} }

var (
	cardTracker = core.Card{ID: 1840, Kind: core.CardTracker, Status: core.CardActive, Rev: 1842, Owner: "hub",
		Title: ct("Catch up monthly financials"), Done: 3, Total: 8, Labels: core.Labels{Private: true},
		Rows: []core.CardElem{{ID: 1, CardText: ct("BMO")}, {ID: 2, CardText: ct("RBC")}},
		Columns: []core.CardElem{{ID: 1, CardText: ct("Jun")}, {ID: 2, CardText: ct("Jul")},
			{ID: 3, CardText: ct("Aug")}, {ID: 4, CardText: ct("Sep")}},
		Cells: []core.Cell{{Row: 1, Column: 1, State: core.CellDone}, {Row: 1, Column: 2, State: core.CellDone},
			{Row: 2, Column: 1, State: core.CellSkip}}}
	cardChecklist = core.Card{ID: 1851, Kind: core.CardChecklist, Status: core.CardActive, Rev: 1853, Owner: "hub",
		Title: ct("How to work through it"), Done: 1, Total: 3, Labels: core.Labels{Private: true, Contaminated: true},
		Items: []core.CardElem{{ID: 1, CardText: ct("Make a folder for the statements"), Done: true},
			{ID: 2, CardText: core.CardText{Text: "Read hxxps://evil[.]test/x?d=1 and approve", Outsider: true, Rev: 1852}},
			{ID: 3, CardText: ct("Import into the app, then the spreadsheet")}}}
	cardNote = core.Card{ID: 1860, Kind: core.CardNote, Status: core.CardActive, Rev: 1860, Owner: "operator",
		Title: ct("Where the statements are"), Labels: core.Labels{Private: true},
		Note: &core.CardText{Text: "BMO: online banking, Documents.\nRBC: mailed.", Rev: 1860}}
	cardUnknown = core.Card{ID: 1870, Kind: "kanban", Status: core.CardActive, Rev: 1870, Owner: "hub",
		Title: ct("Moves"), Labels: core.Labels{Private: true},
		Text: "Card 1870: Moves (kanban) [[card:1870]]\nTodo: pack\nDone: book van"}
)

func boardState(focus bool, cards ...core.Card) core.Single {
	s := core.Single{App: core.App{Live: true}, State: core.State{Phase: core.Idle, Ready: true}}
	s.PaneFocus = focus
	s.Board = core.Board{Asked: true, Loaded: true, Rev: 1900, Cards: cards}
	if len(cards) > 0 {
		s.Board.SelID = cards[0].ID
	}
	return s
}

func detail(s core.Single, id int64, line, col int) core.Single {
	s.Board.SelID, s.Board.Detail, s.Board.Line, s.Board.Col = id, true, line, col
	// A line on a surface card is a box only on the copy the key press
	// put the cursor on.
	s.Board.Box = nil
	if c, _ := core.BoardCard(s.Board); c.Kind == core.CardSurface && line > 0 {
		s.Board.Box = c.Surface
	}
	return s
}

// surfaceCard is relay's fixture card of that name as a card on the board,
// with the id given: its surface folded from the card's messages and the
// links relay lists beside it. The set returned is the connection's, the
// live links its role's file names.
func surfaceCard(t testing.TB, name string, id int64) (core.Card, core.Links) {
	t.Helper()
	s, outsider, live := readCard(t, name)
	s.ID = "card_" + strconv.FormatInt(id, 10)
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "surfaces", name+".card.json"))
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Rev    int64    `json:"rev"`
		Owner  string   `json:"owner"`
		Labels []string `json:"labels"`
		Title  struct {
			Text string `json:"text"`
		} `json:"title"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	c := core.Card{ID: id, Kind: core.CardSurface, Status: core.CardActive, Rev: wire.Rev, Owner: wire.Owner,
		Title: core.CardText{Text: wire.Title.Text, Outsider: outsider, Rev: wire.Rev}, Text: wire.Text,
		Surface: &core.RoleSurface{Outsider: outsider, A2UI: s}, Links: live.Own}
	for _, l := range wire.Labels {
		c.Labels.Private = c.Labels.Private || l == "private"
		c.Labels.Contaminated = c.Labels.Contaminated || l == "contaminated"
	}
	return c, live.Of(nil)
}

func TestBoardGolden(t *testing.T) {
	all := boardState(true, cardTracker, cardChecklist, cardNote, cardUnknown)
	surface, _ := surfaceCard(t, "checklist", 1880)
	withSurface := boardState(true, cardTracker, cardChecklist, cardNote, cardUnknown, surface)
	withSurface.Board.SelID = surface.ID
	hostile, live := surfaceCard(t, "hostile", 1881)
	hostile.Labels.Contaminated = true
	outside := detail(boardState(true, hostile), hostile.ID, 0, 0)
	outside.App.Links = live
	text := core.Card{ID: 1882, Kind: "surface?", Status: core.CardActive, Rev: 1882, Owner: "steward",
		Title: ct("Pack for the lake weekend"), Labels: core.Labels{Private: true},
		Text: "Card 1882: Pack for the lake weekend (surface) [[card:1882]]\nTent and stakes: done\nSleeping bags: open"}
	buttons, buttonLive := surfaceCard(t, "buttons", 1883)
	buttons.Labels.Contaminated = true
	closedButtons := boardState(true, buttons)
	closedButtons.App.Links = buttonLive
	openButtons := func(line int) core.Single {
		s := detail(closedButtons, buttons.ID, line, 0)
		return s
	}
	cases := map[string]core.Single{
		"buttons":         closedButtons,
		"buttons_box":     openButtons(1),
		"buttons_button":  openButtons(2),
		"buttons_primary": openButtons(7),
		"surface":         withSurface,
		"surface_detail":  detail(withSurface, surface.ID, 3, 0),
		"surface_outside": outside,
		"surface_text":    detail(boardState(true, text), text.ID, 0, 0),
		"list":            all,
		"tracker":         detail(all, 1840, 1, 2),
		"checklist":       detail(all, 1851, 2, 0),
		"note":            detail(all, 1860, 0, 0),
		"unknown":         detail(all, 1870, 0, 0),
	}
	sending := detail(all, 1851, 1, 0)
	sending.Board.Sending = 1851
	cases["sending"] = sending
	refused := detail(all, 1851, 1, 0)
	refused.Board.Message, refused.Board.Failure = "Refused.", "Item 9 is not on card 1851."
	cases["refused"] = refused
	input := detail(all, 1851, 3, 0)
	input.Board.Input = &core.BoardInput{Card: 1851, Op: core.CardOp{Op: "edit"}, Prompt: "item 3", Text: "Import"}
	cases["input"] = input
	confirm := all
	confirm.Board.Confirm = 1840
	cases["confirm"] = confirm
	none := boardState(true)
	none.Board.Unavailable, none.Board.Loaded = true, false
	cases["no_board"] = none
	for name, s := range cases {
		t.Run(name, func(t *testing.T) {
			goldenLines(t, PaneLines(s.App, s.State, t0, 44, 26))
		})
	}
}

func TestBoardSideBySide(t *testing.T) {
	s := detail(boardState(false, cardChecklist), 1851, 0, 0)
	f := View(s, "› ", "ctrl+j for a new line", t0, 130, 20)
	plain := ansi.Strip(f.Content)
	if !strings.Contains(plain, "Board 1") || !strings.Contains(plain, "Make a folder") ||
		!strings.Contains(plain, "l to focus") {
		t.Errorf("board not beside the chat:\n%s", plain)
	}
}

// A tracker too wide for one line per row puts each row's cells under
// its name, and every line still fits.
func TestWideTrackerWraps(t *testing.T) {
	c := cardTracker
	c.Columns = nil
	for i := range 24 {
		c.Columns = append(c.Columns, core.CardElem{ID: i + 1, CardText: ct("M" + string(rune('a'+i)))})
	}
	s := detail(boardState(true, c), c.ID, 1, 23)
	lines := PaneLines(s.App, s.State, t0, 40, 30)
	for _, l := range lines {
		if Width(l) > 40 {
			t.Fatalf("line %q too wide", l)
		}
	}
	joined := ansi.Strip(strings.Join(lines, "\n"))
	if !regexp.MustCompile(`│ BMO +│`).MatchString(joined) {
		t.Errorf("row name not on its own line:\n%s", joined)
	}
}

// Outside text shows in the warning colour, as relay sent it, with no
// live link, and its card's footer says the card holds outside text.
func TestOutsiderTextIsMarked(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		url := rapid.SampledFrom([]string{"https://evil.test/x?d=1", "www.evil.test", "hxxps://evil[.]test/x"}).Draw(t, "url")
		text := "see " + url + " now"
		c := cardChecklist
		c.Title = core.CardText{Text: text, Outsider: true}
		c.Items = []core.CardElem{{ID: 1, CardText: core.CardText{Text: text, Outsider: true}}}
		w := rapid.IntRange(30, 70).Draw(t, "width")
		for _, s := range []core.Single{boardState(true, c), detail(boardState(true, c), c.ID, 1, 0)} {
			lines := PaneLines(s.App, s.State, t0, w, 30)
			plain := ansi.Strip(strings.Join(lines, "\n"))
			if !strings.Contains(plain, "from outside") {
				t.Fatalf("the footer does not say so:\n%s", plain)
			}
			for _, l := range lines {
				if strings.Contains(ansi.Strip(l), "see") && !strings.Contains(l, ";33m") {
					t.Fatalf("outside text not in the warning colour: %q", l)
				}
			}
			if followable(plain) {
				t.Fatalf("a live link:\n%s", plain)
			}
		}
	})
}

func genBoardText(t *rapid.T, label string) core.CardText {
	return core.CardText{Text: paneText0(t, label), Outsider: rapid.Bool().Draw(t, label+"Out")}
}

func genBoardCard(t *rapid.T, id int64) core.Card {
	c := core.Card{ID: id, Kind: rapid.SampledFrom([]string{core.CardChecklist, core.CardTracker, core.CardNote, core.CardSurface, paneText0(t, "kind")}).Draw(t, "kind"),
		Status: core.CardActive, Owner: paneText0(t, "owner"), Title: genBoardText(t, "title"), Text: paneText0(t, "text"),
		Done: rapid.IntRange(0, 9).Draw(t, "done"), Total: rapid.IntRange(0, 9).Draw(t, "total"),
		Complete: rapid.Bool().Draw(t, "complete"), Labels: core.Labels{Contaminated: rapid.Bool().Draw(t, "c")}}
	elems := func(label string) []core.CardElem {
		var out []core.CardElem
		for i := range rapid.IntRange(0, 5).Draw(t, label) {
			out = append(out, core.CardElem{ID: i + 1, CardText: genBoardText(t, label+"Text"), Done: rapid.Bool().Draw(t, label+"Done")})
		}
		return out
	}
	switch c.Kind {
	case core.CardChecklist:
		c.Items = elems("items")
	case core.CardTracker:
		c.Rows, c.Columns = elems("rows"), elems("columns")
		for _, r := range c.Rows {
			for _, k := range c.Columns {
				if st := rapid.SampledFrom([]string{"", core.CellDone, core.CellSkip, paneText0(t, "state")}).Draw(t, "cell"); st != "" {
					c.Cells = append(c.Cells, core.Cell{Row: r.ID, Column: k.ID, State: st})
				}
			}
		}
	case core.CardNote:
		n := genBoardText(t, "note")
		c.Note = &n
	case core.CardSurface:
		sc := a2uitest.GenResolved(t, genSurfaceText())
		sc.ID = "card_" + strconv.FormatInt(id, 10)
		c.Surface = &core.RoleSurface{Outsider: rapid.Bool().Draw(t, "outsider"), A2UI: sc}
	}
	return c
}

// plainSurface is a card whose surface is a Column of a body text and the
// given boxes, each bound to a path of its own, as relay would send it.
func plainSurface(t interface {
	Helper()
	Fatal(args ...any)
}, id int64, outsider bool, text string, labels ...string) core.Card {
	t.Helper()
	comps := map[string]a2ui.Component{"t": {ID: "t", Type: "Text", Props: map[string]any{"text": text}}}
	kids := []any{"t"}
	data := map[string]any{}
	for i, l := range labels {
		cid, key := "b"+strconv.Itoa(i), "k"+strconv.Itoa(i)
		kids = append(kids, cid)
		data[key] = i%2 == 0
		comps[cid] = a2ui.Component{ID: cid, Type: "CheckBox", Props: map[string]any{"label": l, "value": map[string]any{"@path": "/" + key}}}
	}
	comps["root"] = a2ui.Component{ID: "root", Type: "Column", Props: map[string]any{"children": kids}}
	s := a2ui.Surface{ID: "card_" + strconv.FormatInt(id, 10), CatalogID: a2ui.RelayCatalogID, Components: comps, Data: data}
	if err := a2ui.Resolved(s); err != nil {
		t.Fatal(err)
	}
	return core.Card{ID: id, Kind: core.CardSurface, Status: core.CardActive, Rev: id, Title: core.CardText{Text: "Surface", Outsider: outsider},
		Labels: core.Labels{Contaminated: outsider}, Surface: &core.RoleSurface{Outsider: outsider, A2UI: s}}
}

// A surface from outside says so, draws every row in the warning colour,
// and leaves nothing a terminal could follow, in the list and in the card.
func TestAnOutsidersSurfaceIsMarked(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		url := rapid.SampledFrom([]string{"https://evil.test/x?d=1", "www.evil.test", "hxxps://evil[.]test/x"}).Draw(t, "url")
		text := "see " + url + " now"
		c := plainSurface(t, 1890, true, text, text)
		c.Surface.A2UI.Components["cap"] = a2ui.Component{ID: "cap", Type: "Text", Props: map[string]any{"text": text, "variant": "caption"}}
		kids := c.Surface.A2UI.Components["root"].Props["children"].([]any)
		c.Surface.A2UI.Components["root"].Props["children"] = append(kids, "cap")
		w := rapid.IntRange(30, 70).Draw(t, "width")
		for _, s := range []core.Single{boardState(true, c), detail(boardState(true, c), c.ID, 1, 0)} {
			lines := PaneLines(s.App, s.State, t0, w, 30)
			plain := ansi.Strip(strings.Join(lines, "\n"))
			if !strings.Contains(plain, "from outside") {
				t.Fatalf("the footer does not say so:\n%s", plain)
			}
			for _, l := range lines {
				if strings.Contains(ansi.Strip(l), "see") && !strings.Contains(l, ";33m") {
					t.Fatalf("outside text not in the warning colour: %q", l)
				}
				if followable(ansi.Strip(l)) {
					t.Fatalf("unsafe row %q", l)
				}
			}
		}
	})
}

// With the cursor on an input of an open surface card, the rows the panel
// reports as selected are the rows that hold that input's label.
func TestTheSelectedRowsOfASurfaceHoldTheInputsLabel(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		var labels []string
		for i := range rapid.IntRange(1, 6).Draw(t, "boxes") {
			labels = append(labels, "box"+strconv.Itoa(i)+rapid.StringMatching(`[a-z]{0,4}`).Draw(t, "label"))
		}
		c := plainSurface(t, 1891, rapid.Bool().Draw(t, "outsider"), rapid.StringMatching(`[a-z ]{0,40}`).Draw(t, "text"), labels...)
		k := rapid.IntRange(0, len(labels)-1).Draw(t, "k")
		s := detail(boardState(true, c), c.ID, k+1, 0)
		w := rapid.IntRange(24, 70).Draw(t, "width")
		out, a, z := cardPanel(s.Board, c, core.Links{}, w, true, true)
		if a < 1 || z <= a || z > len(out) {
			t.Fatalf("selected rows %d to %d of %d", a, z, len(out))
		}
		if got := ansi.Strip(strings.Join(out[a:z], " ")); !strings.Contains(got, labels[k]) {
			t.Fatalf("selected rows %q lack %q", got, labels[k])
		}
	})
}

// Whatever relay sends, the board draws exactly the room it was given, no
// wider than the width, with no character that could move the cursor,
// start a link or reorder text.
func TestBoardNeverEmitsRawControls(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		var cards []core.Card
		for i := range rapid.IntRange(0, 4).Draw(t, "cards") {
			cards = append(cards, genBoardCard(t, int64(i+1)))
		}
		s := boardState(rapid.Bool().Draw(t, "focus"), cards...)
		b := &s.Board
		if len(cards) > 0 {
			b.SelID = rapid.Int64Range(0, int64(len(cards))).Draw(t, "sel")
			b.Detail = rapid.Bool().Draw(t, "detail")
			b.Line, b.Col = rapid.IntRange(-1, 7).Draw(t, "line"), rapid.IntRange(-1, 7).Draw(t, "col")
			if c, _ := core.BoardCard(*b); c.Kind == core.CardSurface && rapid.Bool().Draw(t, "onABox") {
				b.Box = c.Surface
			}
			if rapid.Bool().Draw(t, "input") {
				b.Input = &core.BoardInput{Card: cards[0].ID, Prompt: paneText0(t, "prompt"), Text: paneText0(t, "typed")}
			}
		}
		b.Message, b.Failure = paneText0(t, "msg"), paneText0(t, "fail")
		b.Unavailable = rapid.IntRange(0, 9).Draw(t, "unavailable") == 0
		w := rapid.IntRange(1, 70).Draw(t, "width")
		h := rapid.IntRange(0, 40).Draw(t, "height")
		lines := PaneLines(s.App, s.State, t0, w, h)
		if len(lines) != h {
			t.Fatalf("%d lines, want %d", len(lines), h)
		}
		for _, l := range lines {
			if strings.Contains(l, "\x1b]") {
				t.Fatalf("operating system command in %q", l)
			}
			if Width(l) > w {
				t.Fatalf("line %q is %d wide, limit %d", l, Width(l), w)
			}
			plain := ansi.Strip(l)
			for _, r := range plain {
				if unprintable(r) && r != 0xFFFD {
					t.Fatalf("raw %U in %q (plain %q)", r, l, plain)
				}
			}
			if strings.ContainsAny(plain, "\r\n") {
				t.Fatalf("line break inside a line: %q", plain)
			}
		}
	})
}

// Every board view's hint names the keys that work there: r and b in the
// list and in each card, and no r where it would do nothing.
func TestBoardHintsNameTheKeys(t *testing.T) {
	boxed := plainSurface(t, 1892, false, "t", "one", "two")
	noInputs := plainSurface(t, 1893, false, "t")
	all := boardState(true, cardTracker, cardChecklist, cardNote, boxed, noInputs)
	views := map[string]core.Single{
		"surface":   detail(all, 1892, 1, 0),
		"no inputs": detail(all, 1893, 0, 0),
		"list":      all,
		"tracker":   detail(all, 1840, 1, 0),
		"checklist": detail(all, 1851, 1, 0),
		"note":      detail(all, 1860, 0, 0),
	}
	for name, s := range views {
		foot := strings.Join(PaneLines(s.App, s.State, t0, 200, 26), " ")
		for _, want := range []string{"r reload", "b workspace", "X archive"} {
			if !strings.Contains(ansi.Strip(foot), want) {
				t.Errorf("%s: no %q in %q", name, want, ansi.Strip(foot))
			}
		}
	}
	for id, wantTick := range map[int64]bool{1892: true, 1893: false} {
		s := detail(all, id, 0, 0)
		if foot := ansi.Strip(strings.Join(PaneLines(s.App, s.State, t0, 200, 26), " ")); strings.Contains(foot, "x tick") != wantTick {
			t.Errorf("surface %d: x tick in %q is not %v", id, foot, wantTick)
		}
	}
	if foot := ansi.Strip(strings.Join(PaneLines(all.App, all.State, t0, 200, 26), " ")); !strings.Contains(foot, "e retitle") {
		t.Errorf("list: no e retitle in %q", foot)
	}
	none := boardState(true)
	none.Board.Unavailable, none.Board.Loaded = true, false
	if foot := ansi.Strip(strings.Join(PaneLines(none.App, none.State, t0, 200, 26), " ")); strings.Contains(foot, "r reload") {
		t.Errorf("no board offers r: %q", foot)
	}
}

// The frame of a surface card is the client's own: it names the card's
// owner, as outside text on one line, and says "from outside" when the
// surface, the title or the labels say so.
func TestASurfaceCardsFrameNamesItsOwnerAndSaysWhenItIsFromOutside(t *testing.T) {
	frame := func(c core.Card) string {
		s := boardState(true, c)
		return ansi.Strip(strings.Join(PaneLines(s.App, s.State, t0, 60, 12), "\n"))
	}
	mk := func(surface, title, labels bool) core.Card {
		c := plainSurface(t, 1895, surface, "text", "box")
		c.Title.Outsider, c.Labels.Contaminated, c.Owner = title, labels, "steward"
		return c
	}
	if f := frame(mk(true, false, false)); !strings.Contains(f, "from outside") || !strings.Contains(f, "─ steward ─") {
		t.Errorf("a surface from outside, labels private only:\n%s", f)
	}
	if f := frame(mk(false, true, false)); !strings.Contains(f, "from outside") {
		t.Errorf("a title from outside:\n%s", f)
	}
	if f := frame(mk(false, false, true)); !strings.Contains(f, "from outside") {
		t.Errorf("labels contaminated:\n%s", f)
	}
	if f := frame(mk(false, false, false)); strings.Contains(f, "from outside") || !strings.Contains(f, "─ steward ─") {
		t.Errorf("a surface from the house:\n%s", f)
	}
	none := mk(false, false, false)
	none.Owner = ""
	if f := frame(none); !strings.Contains(f, "─ surface ─") {
		t.Errorf("no owner:\n%s", f)
	}
	// The owner is a string from relay that carries no mark of its source.
	hostile := mk(false, false, false)
	hostile.Owner = "st\x1b]8;;http://evil.test\aeward https://evil.test/leak?d=1\u202e"
	lines := PaneLines(boardState(true, hostile).App, boardState(true, hostile).State, t0, 80, 12)
	for _, l := range lines {
		if strings.Contains(l, "\x1b]") || strings.Contains(l, "\u202e") || followable(ansi.Strip(l)) {
			t.Errorf("the owner reached the frame raw: %q", l)
		}
	}
	if f := ansi.Strip(strings.Join(lines, "\n")); !strings.Contains(f, "─ st") {
		t.Errorf("the owner is not in the frame:\n%s", f)
	}
}

// The corner names the owner in every state of a surface card: while an
// op is on its way, and on a card relay calls complete, where the other
// kinds say only that.
func TestASurfaceCardsCornerAlwaysNamesItsOwner(t *testing.T) {
	c := plainSurface(t, 1896, false, "text", "box")
	c.Owner = "steward"
	top := func(s core.Single, width int) string {
		for _, l := range PaneLines(s.App, s.State, t0, width, 12) {
			if p := ansi.Strip(l); strings.HasPrefix(p, "╭") {
				return p
			}
		}
		t.Fatal("no top border")
		return ""
	}
	sending := boardState(true, c)
	sending.Board.Sending = c.ID
	if got := top(sending, 60); !strings.Contains(got, "─ steward · sending… ─╮") {
		t.Errorf("while sending: %s", got)
	}
	// With little room the owner is what stays.
	if got := top(sending, 24); !strings.Contains(got, "steward") {
		t.Errorf("while sending, 24 wide: %s", got)
	}
	done := c
	done.Complete = true
	if got := top(boardState(true, done), 60); !strings.Contains(got, "─ steward ─╮") {
		t.Errorf("complete: %s", got)
	}
	for _, s := range []core.Single{sending, detail(sending, c.ID, 1, 0)} {
		for width := 1; width <= 80; width++ {
			for _, l := range PaneLines(s.App, s.State, t0, width, 12) {
				if w := Method.StringWidth(l); w > width {
					t.Fatalf("width %d: a line is %d wide: %q", width, w, l)
				}
			}
		}
	}
}

// An owner that draws as nothing names nobody. The corner says the kind
// in its place, and the top border stays inside the panel.
func TestAnOwnerOfNoWidthLeavesTheKindInTheCorner(t *testing.T) {
	c := plainSurface(t, 1897, false, "text", "box")
	c.Title.Text, c.Owner = "Pack for the lake", "\u0301\u0301\u0301"
	s := boardState(true, c)
	if f := ansi.Strip(strings.Join(PaneLines(s.App, s.State, t0, 60, 12), "\n")); !strings.Contains(f, "─ surface ─╮") {
		t.Errorf("the corner:\n%s", f)
	}
	for width := 1; width <= 40; width++ {
		for _, l := range PaneLines(s.App, s.State, t0, width, 12) {
			if w := Method.StringWidth(l); w > width {
				t.Fatalf("width %d: a line is %d wide: %q", width, w, l)
			}
		}
	}
}
