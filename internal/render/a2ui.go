package render

import (
	"sort"
	"strings"

	"github.com/kirahowe/relay-tui/internal/a2ui"
	"github.com/kirahowe/relay-tui/internal/core"
)

// This file draws an A2UI surface (internal/a2ui) as terminal lines, in
// the look of the board: faint rounded boxes and the board's glyphs for
// done and open.
//
// Every string a surface supplies is made safe with the board's own
// functions (cardText, cardLine, noteLines, checkLines), with the
// surface's outsider flag and the live links of the card it is on. A
// string from outside has its links broken, all but the live ones, and its
// rows are in the warning colour. Images, video, audio and any component
// the surface's catalog does not define draw as one line that names the
// type. Nothing is fetched, and no URL a surface holds is shown.
//
// The rows of the finished drawing go through drawn as the last step
// before they are cut to the width, since a row can show an address that
// no single string held: two strings side by side, or a wrap that splits
// a broken link between its scheme and its host.

// rowGap is the columns between the children of a Row.
const rowGap = 2

// A2UI draws surface s in lines at most width wide, for a view outside the
// dashboard. Every string is outside text with no live links. focus, when
// it names a component the root reaches, shows that component on the
// selection bar.
func A2UI(s a2ui.Surface, width int, focus *a2ui.Focus) []string {
	rows, _, _ := surfaceRows(s, true, core.Links{}, width, focus)
	if len(rows) == 0 {
		return rows
	}
	return strings.Split(gate(strings.Join(rows, "\n"), core.Links{}), "\n")
}

// surfaceRows draws s for the board, width wide. outsider and live are the
// card's: every string is made safe with them. selA and selB bound the rows
// the focused component is on, both zero when there is none.
func surfaceRows(s a2ui.Surface, outsider bool, live core.Links, width int, focus *a2ui.Focus) (rows []string, selA, selB int) {
	rows, selA, selB, _ = surfaceDrawing(s, outsider, live, width, focus)
	return rows, selA, selB
}

// surfaceDrawing is surfaceRows and the number of placeholder lines the
// drawing holds.
func surfaceDrawing(s a2ui.Surface, outsider bool, live core.Links, width int, focus *a2ui.Focus) (rows []string, selA, selB, placeholders int) {
	rows, selA, selB, d := drawSurface(s, outsider, live, width, focus)
	return rows, selA, selB, d.placeholders
}

// drawSurface is surfaceRows and the draw that made the rows, which holds
// counts of what it did.
func drawSurface(s a2ui.Surface, outsider bool, live core.Links, width int, focus *a2ui.Focus) (rows []string, selA, selB int, d *surfaceDraw) {
	width = max(width, 1)
	d = &surfaceDraw{s: s, outsider: outsider, live: live, focus: focus,
		budget: a2ui.MaxNodes, wanted: map[a2ui.Child]int{}, measuring: map[a2ui.Child]bool{}}
	if _, ok := s.Root(); !ok {
		d.placeholders++
		return []string{truncate(styled(faint, "waiting for the root component"), width)}, 0, 0, d
	}
	rows, selA, selB = d.node(a2ui.Child{ID: "root"}, width, 0)
	return fit(drawn(rows, outsider), width), selA, selB, d
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
	s        a2ui.Surface
	outsider bool
	live     core.Links
	focus    *a2ui.Focus

	budget       int                 // component visits left
	visits       int                 // component visits so far
	placeholders int                 // placeholder lines drawn so far
	wanted       map[a2ui.Child]int  // widths components asked for, by want
	measuring    map[a2ui.Child]bool // components want is working out
}

// text is a string from the surface, safe to print. It may hold line
// breaks.
func (d *surfaceDraw) text(s string) string { return cardText(s, d.outsider, d.live) }

// line is a string from the surface on one line, safe to print.
func (d *surfaceDraw) line(s string) string { return cardLine(s, d.outsider, d.live) }

// note is a one-line faint remark in brackets, such as a placeholder.
func (d *surfaceDraw) note(text string, width int) []string {
	d.placeholders++
	return []string{truncate(styled(faint, "["+text+"]"), width)}
}

// node draws one component at most width wide. Each component the root
// reaches is drawn once. selA and selB are the rows of the result that the
// focused component is on, both zero when it is not there.
func (d *surfaceDraw) node(ch a2ui.Child, width, depth int) (lines []string, selA, selB int) {
	d.visits++
	if d.budget--; d.budget < 0 {
		return d.note("…", width), 0, 0
	}
	c, ok := d.s.Components[ch.ID]
	if !ok {
		return d.leaf(ch, width), 0, 0
	}
	if depth > a2ui.MaxDepth {
		return d.note("too deep", width), 0, 0
	}
	switch d.s.Kind(c) {
	case "Column":
		lines, selA, selB = d.column(c, ch.Scope, width, depth)
	case "Row":
		lines, selA, selB = d.row(d.s.Children(c, ch.Scope), width, depth)
	case "List":
		if dir, _ := c.Props["direction"].(string); dir == "horizontal" {
			lines, selA, selB = d.row(d.s.Children(c, ch.Scope), width, depth)
		} else {
			lines, selA, selB = d.column(c, ch.Scope, width, depth)
		}
	case "Card":
		lines, selA, selB = d.card(c, ch.Scope, width, depth)
	case "Divider":
		lines = d.divider(c, width)
	case "Button":
		lines = d.button(c, ch.Scope, width, depth)
	default:
		lines = d.leaf(ch, width)
		if d.s.Kind(c) == "" {
			return lines, 0, 0
		}
	}
	if d.focus != nil && d.focus.ID == c.ID && d.focus.Scope == ch.Scope {
		for i, l := range lines {
			lines[i] = barRow(l, width)
		}
		selA, selB = 0, len(lines)
	}
	return lines, selA, selB
}

// leaf draws a component that has no children to lay out: text, a
// checkbox, a field or a picker, or the placeholder for a component that
// is missing or that the catalog does not define.
func (d *surfaceDraw) leaf(ch a2ui.Child, width int) []string {
	c, ok := d.s.Components[ch.ID]
	if !ok {
		return d.note("missing "+truncate(d.line(ch.ID), 24), width)
	}
	switch d.s.Kind(c) {
	case "Text":
		return d.textLines(c, ch.Scope, width)
	case "CheckBox":
		label := d.s.String(ch.Scope, c.Props["label"])
		done, _ := d.s.Bool(ch.Scope, c.Props["value"])
		return checkLines(core.CardElem{CardText: core.CardText{Text: label, Outsider: d.outsider}, Done: done}, d.live, width)
	case "TextField":
		return d.textField(c, ch.Scope, width)
	case "ChoicePicker":
		return d.choices(c, ch.Scope, width)
	}
	name := strings.ToLower(c.Type)
	return d.note(truncate(d.line(name), 24)+" not shown", width)
}

// wantRoom is the width a leaf is drawn at to find the width it asks for.
const wantRoom = 1 << 14

// want is the width a component asks for when it has all the room it likes.
// A leaf is drawn once, without the focus, and its widest row is the
// answer. A column asks for its widest child, a row for its children side
// by side with the gaps, a box for its child and four, the root card for
// its child, and a divider for one. The answers are kept. A component
// that is already being worked out, or is past the depth limit, asks for
// one, so a surface that is not Resolved cannot loop here.
func (d *surfaceDraw) want(ch a2ui.Child, depth int) int {
	if w, ok := d.wanted[ch]; ok {
		return w
	}
	if depth > a2ui.MaxDepth || d.measuring[ch] {
		return 1
	}
	d.measuring[ch] = true
	w := d.asks(ch, depth)
	delete(d.measuring, ch)
	d.wanted[ch] = w
	return w
}

func (d *surfaceDraw) asks(ch a2ui.Child, depth int) int {
	c, ok := d.s.Components[ch.ID]
	kind := ""
	if ok {
		kind = d.s.Kind(c)
	}
	var kids []a2ui.Child
	if ok {
		kids = d.s.Children(c, ch.Scope)
	}
	widest, total := 1, 0
	for _, k := range kids {
		w := d.want(k, depth+1)
		widest = max(widest, w)
		total += w
	}
	switch kind {
	case "Column":
		return widest
	case "List":
		if dir, _ := c.Props["direction"].(string); dir != "horizontal" {
			return widest
		}
		fallthrough
	case "Row":
		return max(total+rowGap*max(len(kids)-1, 0), 1)
	case "Card":
		if depth == 0 {
			return widest
		}
		return max(widest+4, minBox)
	case "Divider":
		return 1
	case "Button":
		if len(kids) == 0 {
			return len("button") + 4
		}
		return widest + 4
	}
	held := d.placeholders
	w := 1
	for _, l := range d.leaf(ch, wantRoom) {
		w = max(w, Method.StringWidth(l))
	}
	d.placeholders = held
	return w
}

// textLines draws Text. A caption is faint plain text. A body is drawn as
// a note card's text is: dressed with **bold**, # headings and - list
// lines when the surface is not from outside, and as sent when it is.
// Every variant keeps every line break, and none is Markdown.
func (d *surfaceDraw) textLines(c a2ui.Component, sc a2ui.Scope, width int) []string {
	raw := d.s.String(sc, c.Props["text"])
	if raw == "" {
		return nil
	}
	var out []string
	if v, _ := c.Props["variant"].(string); v == "caption" {
		for _, l := range strings.Split(d.text(raw), "\n") {
			if l == "" {
				out = append(out, "")
				continue
			}
			out = append(out, wrap(styled(faint, l), width)...)
		}
	} else {
		out = noteLines(raw, d.outsider, d.live, width)
	}
	if d.outsider {
		for i, l := range out {
			if l != "" {
				out[i] = recolour(l, warnColour)
			}
		}
	}
	return out
}

func (d *surfaceDraw) column(c a2ui.Component, sc a2ui.Scope, width, depth int) (out []string, selA, selB int) {
	for _, k := range d.s.Children(c, sc) {
		lines, a, b := d.node(k, width, depth+1)
		if b > a && selB == 0 {
			selA, selB = len(out)+a, len(out)+b
		}
		out = append(out, lines...)
	}
	return out, selA, selB
}

// card draws Card as a box that fills the width it is given. The root Card has no box: the card it is on
// has a frame already.
func (d *surfaceDraw) card(c a2ui.Component, sc a2ui.Scope, width, depth int) (out []string, selA, selB int) {
	framed := depth > 0
	inner := width
	if framed && boxed(width) {
		inner = width - 4
	}
	var lines []string
	for _, k := range d.s.Children(c, sc) {
		kl, a, b := d.node(k, inner, depth+1)
		if b > a && selB == 0 {
			selA, selB = len(lines)+a, len(lines)+b
		}
		lines = append(lines, kl...)
	}
	if !framed {
		return lines, selA, selB
	}
	if boxed(width) {
		if selB > 0 {
			selA, selB = selA+1, selB+1
		}
	}
	return box("", lines, width), selA, selB
}

func (d *surfaceDraw) divider(c a2ui.Component, width int) []string {
	if axis, _ := c.Props["axis"].(string); axis == "vertical" {
		return []string{styled(faint, "│")}
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
func (d *surfaceDraw) row(kids []a2ui.Child, width, depth int) (out []string, selA, selB int) {
	n := len(kids)
	if n == 0 {
		return nil, 0, 0
	}
	avail := width - rowGap*(n-1)
	if avail < n {
		for _, k := range kids {
			lines, a, b := d.node(k, width, depth+1)
			if b > a && selB == 0 {
				selA, selB = len(out)+a, len(out)+b
			}
			out = append(out, lines...)
		}
		return out, selA, selB
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
		base[i] = min(d.want(k, depth+1), avail)
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
		lines, a, b := d.node(k, alloc[i], depth+1)
		if b > a && selB == 0 {
			selA, selB = a, b
		}
		cols[i] = fit(lines, alloc[i])
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
	out = make([]string, height)
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
	return selfContained(out), selA, selB
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
		lines, _, _ := d.node(k, inner, depth+1)
		body = append(body, lines...)
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
	label := d.line(d.s.String(sc, c.Props["label"]))
	value := d.text(d.s.String(sc, c.Props["value"]))
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
			body = []string{styled(faint, d.line(d.s.String(sc, c.Props["placeholder"])))}
		}
		return box(label, body, width)
	}
	shown = strings.ReplaceAll(shown, "\n", " ")
	if shown == "" {
		shown = styled(faint, d.line(d.s.String(sc, c.Props["placeholder"])))
	}
	return []string{truncate(styled(faint, label+" ")+styled(faint, "[")+shown+styled(faint, "]"), width)}
}

// choices draws ChoicePicker: the label, then each option with a mark for
// whether it is chosen. The chips display runs the options together on
// wrapped lines, the chosen ones in the accent colour.
func (d *surfaceDraw) choices(c a2ui.Component, sc a2ui.Scope, width int) []string {
	var out []string
	if label := d.line(d.s.String(sc, c.Props["label"])); label != "" {
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
		label := d.line(d.s.String(sc, m["label"]))
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

// A2UIName is the line that names surface s, for a view that shows more
// than one. The id is a string from the surface, so it is escaped and
// defanged as the rest are.
func A2UIName(s a2ui.Surface, width int) string {
	return truncate(styled(faint, "surface "+cardLine(s.ID, true, core.Links{})), max(width, 1))
}
