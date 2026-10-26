package render

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/kirahowe/relay-tui/internal/core"
)

// The workspace pane is drawn from data relay sent: displays, and what
// relay answered about the references in the latest reply. Model text is
// never shown as it is, only searched for references. Everything that
// prints goes through core.EscapeLabel or core.EscapeNonPrinting, so no
// control, bidi or format character reaches the terminal, and text that
// came from outside says where from, in plain words (core.Provenance),
// with its URLs defanged. The pane never writes a hyperlink.

// SideMinWidth is the narrowest drawing width that gets a side pane. The
// shell draws one column short of the screen, so this is a 120-column
// screen. Anything narrower shows the pane as an overlay.
const SideMinWidth = 119

// PaneKey is the key that focuses the pane, as the hints name it.
const PaneKey = "l"

// Side reports whether the pane stands beside the transcript at width.
func Side(width int) bool { return width >= SideMinWidth }

// PaneWidth is how wide the side pane is at width.
func PaneWidth(width int) int { return min(max(width/3, 36), 52) }

// gap is the columns between the transcript and the side pane: a space,
// a rule, a space.
const gap = 3

// MainWidth is the width the transcript and the input box are drawn at:
// all of width, or, beside the pane, what is left of it.
func MainWidth(width int) int {
	if Side(width) {
		return width - PaneWidth(width) - gap
	}
	return width
}

// joinColumns puts the pane to the right of the transcript's lines. Each
// left line is padded to left columns.
func joinColumns(lines, pane []string, left int) []string {
	n := max(len(lines), len(pane))
	out := make([]string, n)
	for i := range out {
		l, r := "", ""
		if i < len(lines) {
			l = lines[i]
		}
		if i < len(pane) {
			r = pane[i]
		}
		if w := Method.StringWidth(l); w < left {
			l += strings.Repeat(" ", left-w)
		}
		out[i] = l + sgr(0) + " " + styled(faint, "│") + " " + r
	}
	return out
}

// line is text on one line, safe to print.
func paneLine(text string, defang bool) string {
	t := core.EscapeLabel(text)
	if defang {
		t = Defang(t)
	}
	return t
}

// paneText is multi-line text, safe to print. Newlines stay.
func paneText(text string, defang bool) string {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\t", "    ")
	t := core.EscapeNonPrinting(text)
	if defang {
		t = Defang(t)
	}
	return t
}

// textLines wraps paneText to width, keeping its line breaks.
func textLines(text string, defang bool, width int) []string {
	var out []string
	for _, l := range strings.Split(paneText(text, defang), "\n") {
		if l == "" {
			out = append(out, "")
			continue
		}
		out = append(out, wrap(l, width)...)
	}
	return out
}

// hang wraps text to width less the lead, which is first on the first
// line and the same width of blanks on the rest.
func hang(first, text string, width int) []string {
	lead := Method.StringWidth(first)
	rest := strings.Repeat(" ", lead)
	var out []string
	for i, l := range wrap(text, max(width-lead, 1)) {
		if i == 0 {
			out = append(out, first+l)
		} else {
			out = append(out, rest+l)
		}
	}
	return out
}

// fromMark is the words for where something came from, in the warning
// colour, or nothing.
func fromMark(l core.Labels, sources []core.Source) string {
	if p := core.Provenance(l, sources); p != "" {
		return styled(warnColour, p)
	}
	return ""
}

func itemTitle(it core.Item, defang bool) string {
	if t := paneLine(it.Title, defang); strings.TrimSpace(t) != "" {
		return t
	}
	return "(untitled)"
}

// firstText is the first line of text with something on it.
func firstText(text string) string {
	for l := range strings.Lines(text) {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return ""
}

// listBlock is an item's rows in the list.
func listBlock(it core.Item, selected bool, width int) []string {
	def := it.Labels.Contaminated
	marker := "  "
	st := style(0)
	if selected {
		marker, st = "▸ ", bold
	}
	out := hang(marker, styled(st, itemTitle(it, def)), width)
	if m := fromMark(it.Labels, it.Sources); m != "" {
		out = append(out, "  "+m)
	}
	if l := firstText(it.Text); l != "" {
		out = append(out, styled(faint, "  "+truncate(oneLinePane(l, def), max(width-2, 1))))
	}
	for i, src := range it.Sources {
		if i == 3 {
			out = append(out, styled(faint, fmt.Sprintf("  + %d more sources", len(it.Sources)-i)))
			break
		}
		out = append(out, sourceLines(src, def, width)...)
	}
	return out
}

// paneLine2 prints a line of already-escaped text, defanged if asked.
func paneLine2(l string, defang bool) string {
	if defang {
		return Defang(l)
	}
	return l
}

// oneLinePane is text on one line, safe to print.
func oneLinePane(text string, defang bool) string {
	return strings.Join(strings.Fields(paneText(text, defang)), " ")
}

// sourceLines is one source of a shown item.
func sourceLines(src core.Source, def bool, width int) []string {
	text := paneLine(src.Title, def)
	if text == "" {
		text = "(untitled)"
	}
	text += styled(faint, " · ") + paneLine(src.Ref, def)
	return hang("  ↳ ", text, width)
}

// citationBlock is one reference of the latest reply, as relay answered.
func citationBlock(s core.State, ref string, width int) []string {
	c := s.Pane.Cite(ref)
	def := c.Labels.Contaminated || s.Labels.Contaminated
	name := paneLine(ref, def)
	var text string
	switch c.Status {
	case core.CiteOK:
		text = paneLine(c.Title, def) + styled(faint, " · "+name)
		if m := fromMark(c.Labels, nil); m != "" {
			text += styled(faint, " · ") + m
		}
	case core.CiteMissing:
		text = styled(reverse, " missing ") + " " + name
	case core.CiteExternal:
		text = name
		if c.Target != "" {
			text += styled(faint, " → ") + paneLine(c.Target, def)
		}
	default:
		text = styled(faint, "unchecked ") + name
	}
	return hang("  ", text, width)
}

// parts are the pieces of a pane, top to bottom: a header, a body that
// may be longer than the room, and a footer. selA and selB are the body
// rows of the selected item, kept in view; scrolls says the body scrolls
// by Pane.Scroll.
type parts struct {
	head, body, foot []string
	selA, selB       int
	scrolls          bool
	// keys is the pane's key hints, "key what · key what"; a panel puts
	// them on the screen's row of hints rather than in the pane.
	keys string
}

func paneParts(a core.App, s core.State, now time.Time, width int) parts {
	p := s.Pane
	if !a.Board.Items {
		return boardParts(a, s, now, width)
	}
	items := core.PaneItems(s)
	var pr parts
	other := ""
	if hasBoard(a.Board) {
		other = fmt.Sprintf("board %d", len(a.Board.Cards))
	}
	pr.head = paneHead("Workspace", len(items), other, a.PaneFocus, width)

	sel := core.SelectedIndex(s)
	switch {
	case p.Detail && sel >= 0:
		pr.body = detailBody(items[sel], width)
		pr.scrolls = true
		pr.keys = "↑↓ scroll · esc back"
	default:
		for i, it := range items {
			if i > 0 {
				pr.body = append(pr.body, "")
			}
			if i == sel {
				pr.selA = len(pr.body)
			}
			pr.body = append(pr.body, listBlock(it, a.PaneFocus && i == sel, width)...)
			if i == sel {
				pr.selB = len(pr.body)
			}
		}
		if len(items) == 0 {
			pr.body = append(pr.body, styled(faint, "nothing yet"))
		}
		if refs := core.LatestRefs(s); len(refs) > 0 {
			pr.body = append(pr.body, "", styled(faint, fmt.Sprintf("cited in the latest reply (%d)", len(refs))))
			for _, ref := range refs {
				pr.body = append(pr.body, citationBlock(s, ref, width)...)
			}
		}
		switch {
		case a.PaneFocus:
			pr.keys = "↑↓ select · enter open · b board · esc back"
		case hasBoard(a.Board):
			pr.keys = PaneKey + " to focus, then b for the board"
		default:
			pr.keys = PaneKey + " to focus"
		}
	}
	return pr
}

// hasBoard says the agent has a board and it has loaded, so the pane can
// say how many cards it holds.
func hasBoard(b core.Board) bool { return b.Loaded && !b.Unavailable }

// paneHead is the pane's header: the view's name, in the accent colour
// while the pane has the keys, its count (none while it is -1), and at the
// right a chip for b, which switches to the other view. The chip is left
// out when the line has no room for it.
func paneHead(name string, count int, other string, focus bool, width int) []string {
	st := bold
	if focus {
		st |= accent
	}
	title := styled(st, name)
	if count >= 0 {
		title += styled(faint, fmt.Sprintf(" %d", count))
	}
	if other == "" {
		return []string{truncate(title, width)}
	}
	right := chip("b", other)
	pad := width - Method.StringWidth(title) - Method.StringWidth(right)
	if pad < 1 {
		return []string{truncate(title, width)}
	}
	return []string{title + strings.Repeat(" ", pad) + right}
}

func detailBody(it core.Item, width int) []string {
	def := it.Labels.Contaminated
	var out []string
	out = append(out, hang("", styled(bold, itemTitle(it, def)), width)...)
	if m := fromMark(it.Labels, it.Sources); m != "" {
		out = append(out, hang("  ", m+styled(faint, " · links are broken on purpose"), width)...)
	}
	inner := max(width-2, 1)
	var ids []string
	for _, id := range []string{it.Ref, it.Path} {
		if id != "" {
			ids = append(ids, paneLine(id, def))
		}
	}
	if len(ids) > 0 {
		out = append(out, hang("  ", styled(faint, strings.Join(ids, " · ")), width)...)
	}
	out = append(out, "")
	for _, l := range textLines(it.Text, def, inner) {
		out = append(out, "  "+l)
	}
	if len(it.Sources) > 0 {
		out = append(out, "", styled(faint, "sources"))
		for _, src := range it.Sources {
			out = append(out, sourceLines(src, def, width)...)
		}
	}
	return out
}

// paneRender draws the pane exactly height lines tall and no wider than
// width, and says how far its detail view can scroll. Its key hints are
// its last rows.
func paneRender(a core.App, s core.State, now time.Time, width, height int) ([]string, int) {
	pr := paneParts(a, s, now, max(width, 1))
	if pr.keys != "" {
		pr.foot = append(pr.foot, wrap(styleKeys(pr.keys), max(width, 1))...)
	}
	return paneDraw(s, pr, width, height)
}

// paneRenderKeys draws the pane without its key hints, and gives them
// back for the screen's row of hints.
func paneRenderKeys(a core.App, s core.State, now time.Time, width, height int) ([]string, int, string) {
	pr := paneParts(a, s, now, max(width, 1))
	lines, mx := paneDraw(s, pr, width, height)
	return lines, mx, pr.keys
}

// paneNeed is how many rows the pane must have for its foot to show whole:
// the question, the editor and the keys that answer them.
func paneNeed(a core.App, s core.State, now time.Time, width int) int {
	pr := paneParts(a, s, now, max(width, 1))
	n := len(pr.foot)
	if pr.keys != "" {
		n += len(wrap(styleKeys(pr.keys), max(width, 1)))
	}
	return n
}

func paneDraw(s core.State, pr parts, width, height int) ([]string, int) {
	height = max(height, 0)
	width = max(width, 1)
	avail := height - len(pr.head) - len(pr.foot)
	var all []string
	maxScroll := 0
	if avail <= 0 {
		// Too little room: the foot has what the operator answers, so the
		// head is what goes.
		all = append(append([]string(nil), pr.head...), pr.foot...)
		all = all[max(len(all)-height, 0):]
	} else {
		maxScroll = max(len(pr.body)-avail, 0)
		start := 0
		switch {
		case pr.scrolls:
			start = min(max(s.Pane.Scroll, 0), maxScroll)
		case len(pr.body) > avail:
			start = max(pr.selB-avail, 0)
			if pr.selA < start {
				start = pr.selA
			}
			start = min(start, maxScroll)
		}
		end := min(start+avail, len(pr.body))
		win := slices.Clone(pr.body[start:end])
		if avail >= 3 {
			if start > 0 {
				win[0] = styled(faint, fmt.Sprintf("↑ %d above", start))
			}
			if end < len(pr.body) {
				win[len(win)-1] = styled(faint, fmt.Sprintf("↓ %d below", len(pr.body)-end+1))
			}
		}
		for len(win) < avail {
			win = append(win, "")
		}
		all = append(append(append([]string(nil), pr.head...), win...), pr.foot...)
	}
	all = all[:min(len(all), height)]
	for i, l := range all {
		all[i] = truncate(l, width)
	}
	for len(all) < height {
		all = append(all, "")
	}
	if !pr.scrolls {
		maxScroll = 0
	}
	return selfContained(all), maxScroll
}

// paneActive is what the pane draws while it has the keys: its editor,
// the archive question, or itself.
func paneActive(a core.App, s core.State) core.SurfaceKind {
	switch b := a.Board; {
	case !b.Items && b.Input != nil:
		return core.SurfaceEditor // the prompt and its text
	case !b.Items && b.Confirm != 0:
		return core.SurfaceArchive
	}
	return core.SurfacePane
}

// PaneLines is the pane drawn at width and height.
func PaneLines(a core.App, s core.State, now time.Time, width, height int) []string {
	lines, _ := paneRender(a, s, now, width, height)
	return lines
}
