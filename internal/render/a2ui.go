package render

import (
	"sort"
	"strings"

	"github.com/kirahowe/relay-tui/internal/a2ui"
	"github.com/kirahowe/relay-tui/internal/core"
)

// This file draws an A2UI surface (internal/a2ui) as terminal lines, in
// the look of the board: faint rounded boxes, the board's glyphs for done,
// open and skipped, and the board's tracker grid.
//
// Every string a surface supplies goes through cardText first, which
// escapes what does not print and defangs a live link, as text from
// outside is. Images, video, audio and any component the surface's catalog
// does not define draw as one line that names the type. Nothing is
// fetched, and no URL a surface holds is shown.

// rowGap is the columns between the children of a Row.
const rowGap = 2

// surfaceText is a string from a surface, safe to print and defanged. It
// may hold line breaks.
func surfaceText(s string) string { return cardText(s, true, core.Links{}) }

// surfaceLine is a string from a surface on one line.
func surfaceLine(s string) string { return cardLine(s, true, core.Links{}) }

// A2UI draws surface s in lines at most width wide. focus, when it names
// a component the root reaches, shows that component on the selection
// bar.
func A2UI(s a2ui.Surface, width int, focus *a2ui.Focus) []string {
	width = max(width, 1)
	if _, ok := s.Root(); !ok {
		return []string{truncate(styled(faint, "waiting for the root component"), width)}
	}
	d := &surfaceDraw{s: s, focus: focus, budget: a2ui.MaxNodes}
	return fit(d.node(a2ui.Child{ID: "root"}, width, 0, false), width)
}

// fit cuts any line wider than width.
func fit(lines []string, width int) []string {
	for i, l := range lines {
		if Method.StringWidth(l) > width {
			lines[i] = truncate(l, width)
		}
	}
	return lines
}

type surfaceDraw struct {
	s      a2ui.Surface
	focus  *a2ui.Focus
	budget int
}

// note is a one-line faint remark in brackets, such as a placeholder.
func note(text string, width int) []string {
	return []string{truncate(styled(faint, "["+text+"]"), width)}
}

// node draws one component at most width wide. With measure set, a
// divider or a card takes only the room it needs, so the caller can find
// how wide the component wants to be.
func (d *surfaceDraw) node(ch a2ui.Child, width, depth int, measure bool) []string {
	if d.budget--; d.budget < 0 {
		return note("…", width)
	}
	c, ok := d.s.Components[ch.ID]
	if !ok {
		return note("missing "+truncate(surfaceLine(ch.ID), 24), width)
	}
	if depth > a2ui.MaxDepth {
		return note("too deep", width)
	}
	var lines []string
	kind := d.s.Kind(c)
	switch kind {
	case "Text":
		lines = d.text(c, ch.Scope, width)
	case "Column":
		lines = d.column(c, ch.Scope, width, depth, measure)
	case "Row":
		lines = d.row(d.s.Children(c, ch.Scope), width, depth)
	case "List":
		if dir, _ := c.Props["direction"].(string); dir == "horizontal" {
			lines = d.row(d.s.Children(c, ch.Scope), width, depth)
		} else {
			lines = d.column(c, ch.Scope, width, depth, measure)
		}
	case "Card":
		lines = d.card(c, ch.Scope, width, depth, measure)
	case "Divider":
		lines = d.divider(c, width, measure)
	case "CheckBox":
		label := d.s.String(ch.Scope, c.Props["label"])
		done, _ := d.s.Bool(ch.Scope, c.Props["value"])
		lines = checkLines(core.CardElem{CardText: core.CardText{Text: surfaceText(label)}, Done: done}, core.Links{}, width)
	case "Button":
		lines = d.button(c, ch.Scope, width, depth)
	case "TextField":
		lines = d.textField(c, ch.Scope, width)
	case "ChoicePicker":
		lines = d.choices(c, ch.Scope, width)
	case "Tracker":
		lines = d.tracker(c, ch.Scope, width)
	default:
		name := strings.ToLower(c.Type)
		return note(truncate(surfaceLine(name), 24)+" not shown", width)
	}
	if d.focus != nil && d.focus.ID == c.ID && d.focus.Scope == ch.Scope {
		for i, l := range lines {
			lines[i] = barRow(l, width)
		}
	}
	return lines
}

// text draws Text: markdown for the body variant, plain faint text for a
// caption.
func (d *surfaceDraw) text(c a2ui.Component, sc a2ui.Scope, width int) []string {
	t := surfaceText(d.s.String(sc, c.Props["text"]))
	if t == "" {
		return nil
	}
	if v, _ := c.Props["variant"].(string); v == "caption" {
		var out []string
		for _, l := range strings.Split(t, "\n") {
			if l == "" {
				out = append(out, "")
				continue
			}
			out = append(out, wrap(styled(faint, l), width)...)
		}
		return out
	}
	return defangLines(Markdown(t, width))
}

func (d *surfaceDraw) column(c a2ui.Component, sc a2ui.Scope, width, depth int, measure bool) []string {
	var out []string
	for _, k := range d.s.Children(c, sc) {
		out = append(out, d.node(k, width, depth+1, measure)...)
	}
	return out
}

// card draws Card as a box. In a column it fills the width; measured, it
// wraps its child closely.
func (d *surfaceDraw) card(c a2ui.Component, sc a2ui.Scope, width, depth int, measure bool) []string {
	inner := width
	if boxed(width) {
		inner = width - 4
	}
	var lines []string
	for _, k := range d.s.Children(c, sc) {
		lines = append(lines, d.node(k, inner, depth+1, measure)...)
	}
	if measure && boxed(width) {
		w := 1
		for _, l := range lines {
			w = max(w, Method.StringWidth(l))
		}
		width = min(width, max(w+4, minBox))
	}
	return box("", lines, width)
}

func (d *surfaceDraw) divider(c a2ui.Component, width int, measure bool) []string {
	if axis, _ := c.Props["axis"].(string); axis == "vertical" {
		return []string{styled(faint, "│")}
	}
	if measure {
		return []string{styled(faint, "─")}
	}
	return []string{styled(faint, strings.Repeat("─", width))}
}

// weightOf is a component's layout weight, or zero.
func (d *surfaceDraw) weightOf(id string) float64 {
	c, ok := d.s.Components[id]
	if !ok {
		return 0
	}
	w, _ := c.Props["weight"].(float64)
	return max(w, 0)
}

// isVertical says the child is a divider that runs down.
func (d *surfaceDraw) isVertical(id string) bool {
	c, ok := d.s.Components[id]
	if !ok || d.s.Kind(c) != "Divider" {
		return false
	}
	axis, _ := c.Props["axis"].(string)
	return axis == "vertical"
}

// row lays children side by side. A child with no weight gets the width
// it wants, or less when they do not all fit (the widest give way first).
// What is left over is shared among the weighted children by weight. When
// there is not room for every child, they stack.
func (d *surfaceDraw) row(kids []a2ui.Child, width, depth int) []string {
	n := len(kids)
	if n == 0 {
		return nil
	}
	avail := width - rowGap*(n-1)
	if avail < n {
		var out []string
		for _, k := range kids {
			out = append(out, d.node(k, width, depth+1, false)...)
		}
		return out
	}
	base := make([]int, n)
	weight := make([]float64, n)
	var weighted float64
	for i, k := range kids {
		weight[i] = d.weightOf(k.ID)
		weighted += weight[i]
		if weight[i] > 0 {
			base[i] = 1
			continue
		}
		w := 1
		for _, l := range d.node(k, avail, depth+1, true) {
			w = max(w, Method.StringWidth(l))
		}
		base[i] = min(w, avail)
	}
	alloc := shrink(base, avail)
	if left := avail - sum(alloc); left > 0 && weighted > 0 {
		given := 0
		for i := range alloc {
			if weight[i] > 0 {
				share := int(float64(left) * weight[i] / weighted)
				alloc[i] += share
				given += share
			}
		}
		for i := 0; given < left && i < n; i++ {
			if weight[i] > 0 {
				alloc[i]++
				given++
			}
		}
	}
	cols := make([][]string, n)
	height := 0
	for i, k := range kids {
		if d.isVertical(k.ID) {
			continue
		}
		cols[i] = fit(d.node(k, alloc[i], depth+1, false), alloc[i])
		height = max(height, len(cols[i]))
	}
	for i, k := range kids {
		if d.isVertical(k.ID) {
			for range max(height, 1) {
				cols[i] = append(cols[i], styled(faint, "│"))
			}
		}
	}
	height = max(height, 1)
	out := make([]string, height)
	for r := range out {
		var b strings.Builder
		for i := range cols {
			l := ""
			if r < len(cols[i]) {
				l = truncate(cols[i][r], alloc[i])
			}
			b.WriteString(l)
			if i < n-1 {
				b.WriteString(strings.Repeat(" ", alloc[i]-Method.StringWidth(l)+rowGap))
			}
		}
		out[r] = b.String()
	}
	return selfContained(out)
}

func sum(xs []int) int {
	t := 0
	for _, x := range xs {
		t += x
	}
	return t
}

// shrink returns widths no larger than want that add up to at most total,
// cutting the widest first.
func shrink(want []int, total int) []int {
	out := append([]int(nil), want...)
	if sum(out) <= total {
		return out
	}
	level := sort.Search(total+1, func(l int) bool {
		t := 0
		for _, w := range want {
			t += min(w, l)
		}
		return t > total
	}) - 1
	level = max(level, 1)
	for i, w := range want {
		out[i] = min(w, level)
	}
	for i := 0; sum(out) < total && i < len(out); i++ {
		if out[i] < want[i] {
			out[i]++
		}
	}
	return out
}

// button draws Button as its child between brackets: a bold accent frame
// for the primary variant, and an underlined, frameless link for the
// borderless one.
func (d *surfaceDraw) button(c a2ui.Component, sc a2ui.Scope, width, depth int) []string {
	inner := max(width-4, 1)
	var body []string
	for _, k := range d.s.Children(c, sc) {
		body = append(body, d.node(k, inner, depth+1, false)...)
	}
	if len(body) == 0 {
		body = []string{"button"}
	}
	variant, _ := c.Props["variant"].(string)
	var out []string
	for _, l := range body {
		l = truncate(l, inner)
		switch variant {
		case "borderless":
			out = append(out, styled(underline|accent, ansiPlain(l)))
		case "primary":
			out = append(out, styled(accent|bold, "[ ")+l+styled(accent|bold, " ]"))
		default:
			out = append(out, styled(faint, "[ ")+l+styled(faint, " ]"))
		}
	}
	return out
}

// ansiPlain is a line without its styles.
func ansiPlain(l string) string { return sgrRE.ReplaceAllString(l, "") }

// textField draws a label and the field's value in brackets: a faint
// placeholder when it is empty, dots for an obscured field, and a box for
// long text.
func (d *surfaceDraw) textField(c a2ui.Component, sc a2ui.Scope, width int) []string {
	label := surfaceLine(d.s.String(sc, c.Props["label"]))
	value := surfaceText(d.s.String(sc, c.Props["value"]))
	variant, _ := c.Props["variant"].(string)
	shown := value
	if variant == "obscured" {
		shown = strings.Repeat("•", min(len([]rune(value)), 12))
	}
	if variant == "longText" {
		var body []string
		for _, l := range strings.Split(shown, "\n") {
			body = append(body, wrap(l, max(width-4, 1))...)
		}
		if shown == "" {
			body = []string{styled(faint, surfaceLine(d.s.String(sc, c.Props["placeholder"])))}
		}
		return box(label, body, width)
	}
	shown = strings.ReplaceAll(shown, "\n", " ")
	if shown == "" {
		shown = styled(faint, surfaceLine(d.s.String(sc, c.Props["placeholder"])))
	}
	return []string{truncate(styled(faint, label+" ")+styled(faint, "[")+shown+styled(faint, "]"), width)}
}

// choices draws ChoicePicker: the label, then each option with a mark for
// whether it is chosen. The chips display runs the options together on
// wrapped lines, the chosen ones in the accent colour.
func (d *surfaceDraw) choices(c a2ui.Component, sc a2ui.Scope, width int) []string {
	var out []string
	if label := surfaceLine(d.s.String(sc, c.Props["label"])); label != "" {
		out = append(out, truncate(styled(bold, label), width))
	}
	chosen := map[string]bool{}
	for _, v := range d.s.Strings(sc, c.Props["value"]) {
		chosen[v] = true
	}
	multi := false
	if v, _ := c.Props["variant"].(string); v == "multipleSelection" {
		multi = true
	}
	chips := false
	if v, _ := c.Props["displayStyle"].(string); v == "chips" {
		chips = true
	}
	opts, _ := c.Props["options"].([]any)
	var chipText []string
	for i, o := range opts {
		if i >= a2ui.MaxItems {
			break
		}
		m, _ := o.(map[string]any)
		label := surfaceLine(d.s.String(sc, m["label"]))
		on := chosen[a2ui.Text(m["value"])]
		switch {
		case chips && on:
			chipText = append(chipText, styled(accent|bold, "● "+label))
		case chips:
			chipText = append(chipText, styled(faint, "○ "+label))
		default:
			mark := "( )"
			switch {
			case multi && on:
				mark = styled(okColour, "[x]")
			case multi:
				mark = styled(faint, "[ ]")
			case on:
				mark = styled(okColour, "(•)")
			default:
				mark = styled(faint, "( )")
			}
			out = append(out, hang(mark+" ", label, width)...)
		}
	}
	if chips && len(chipText) > 0 {
		out = append(out, wrap(strings.Join(chipText, "  "), width)...)
	}
	return out
}

// tracker draws Tracker with the board's own grid: the surface's rows,
// columns and cells become a core.Card, and cardLinesFor draws it, so the
// grid, its glyphs and the list a too-wide grid falls back to are the
// board's.
func (d *surfaceDraw) tracker(c a2ui.Component, sc a2ui.Scope, width int) []string {
	t := d.s.Tracker(c, sc)
	card := core.Card{Kind: core.CardTracker}
	rowID, colID := map[string]int{}, map[string]int{}
	for i, r := range t.Rows {
		rowID[r.ID] = i + 1
		card.Rows = append(card.Rows, core.CardElem{ID: i + 1, CardText: core.CardText{Text: surfaceText(r.Label)}})
	}
	for i, k := range t.Columns {
		colID[k.ID] = i + 1
		card.Columns = append(card.Columns, core.CardElem{ID: i + 1, CardText: core.CardText{Text: surfaceText(k.Label)}})
	}
	for _, x := range t.Cells {
		r, rok := rowID[x.Row]
		k, kok := colID[x.Column]
		if rok && kok {
			card.Cells = append(card.Cells, core.Cell{Row: r, Column: k, State: x.State})
		}
	}
	lines, _, _ := cardLinesFor(core.Board{}, card, core.Links{}, width, true, false)
	return lines
}

// A2UIName is the line that names surface s, for a view that shows more
// than one. The id is a string from the surface, so it is escaped and
// defanged as the rest are.
func A2UIName(s a2ui.Surface, width int) string {
	return truncate(styled(faint, "surface "+surfaceLine(s.ID)), max(width, 1))
}
