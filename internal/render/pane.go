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

// said is one line of words that are not the operator's own and carry
// no mark of where they came from: relay's about something (a refusal,
// what an undo did, a notice) or an agent's about its work (the tool it
// is running). Such a line can quote text from outside, a page's name or
// an address a call was given, and nothing in one needs a link that can
// be followed, so every link in it is broken, after every hidden
// character is gone (see seen).
func said(text string) string { return Defang(oneLine(core.DropHidden(text))) }

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
	// card is the open card, drawn in a frame that stays in place while
	// body, its inside, scrolls. top is where the window starts; with
	// follow it moves as little as shows selA to selB, the focused line,
	// when there is one.
	card   *openCard
	cardID int64
	top    int
	follow bool
	// hints is the key hints drawn in the pane, under the foot, and info
	// a line about an open card drawn under it (a tracker's cell). Both
	// give way to the card itself on a short pane (see cardDraw).
	hints, info []string
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
func paneRender(a core.App, s core.State, now time.Time, width, height int) ([]string, paneDrawn) {
	pr := paneParts(a, s, now, max(width, 1))
	if pr.keys != "" {
		pr.hints = wrap(styleKeys(pr.keys), max(width, 1))
	}
	return paneDraw(s, pr, width, height)
}

// paneRenderKeys draws the pane without its key hints, and gives them
// back for the screen's row of hints.
func paneRenderKeys(a core.App, s core.State, now time.Time, width, height int) ([]string, paneDrawn, string) {
	pr := paneParts(a, s, now, max(width, 1))
	lines, pd := paneDraw(s, pr, width, height)
	return lines, pd, pr.keys
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

// paneDrawn is what the keys need of how the pane was drawn: how far its
// detail view scrolls, and the open card.
type paneDrawn struct {
	Max  int
	Card core.CardScroll
}

func paneDraw(s core.State, pr parts, width, height int) ([]string, paneDrawn) {
	height = max(height, 0)
	width = max(width, 1)
	if pr.card != nil {
		return cardDraw(pr, width, height)
	}
	pr.foot = append(append(pr.info, pr.foot...), pr.hints...)
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
			// The window's first and last rows give way to the marks for
			// what is above and below, so the selection is kept clear of
			// both. One taller than the room shows from its first row.
			mark := 0
			if avail >= 3 {
				mark = 1
			}
			start = max(pr.selB+mark-avail, 0)
			if pr.selA-mark < start {
				start = max(pr.selA-mark, 0)
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
	return selfContained(all), paneDrawn{Max: maxScroll}
}

// cardDraw draws the pane with an open card: its frame in place between
// the pane's head and foot, and inside it the part of the body the window
// holds, with marks for what is above and below. A card that fits is
// framed as tall as it is.
//
// A pane without room for its head, its foot, the frame and three rows
// of body says it is too small. Otherwise the rows go out in one order:
// the head and the foot, since the foot holds what the operator is
// answering; then the frame's borders, which say whose card it is and
// where its text came from; then three rows of body; then the line about
// the card under it (info), whole or not at all; then the key hints; and
// the body takes the rest.
func cardDraw(pr parts, width, height int) ([]string, paneDrawn) {
	f := pr.card
	n := len(pr.body)
	focused := pr.selB > pr.selA
	cs := core.CardScroll{Card: pr.cardID, Shown: !focused}
	border := 0
	if boxed(width) {
		border = 2
	}
	left := height - len(pr.head) - len(pr.foot) - border
	if left < leastBody {
		// Too small for the card: say so rather than draw part of it, and
		// tell the keys, so nothing it would show can be acted on.
		cs.Shown, cs.Small = false, true
		all := append(slices.Clone(pr.head), styled(faint, "Too small to show this card."))
		return finish(all, width, height), paneDrawn{Card: cs}
	}
	least := min(n, leastBody)
	left -= least
	// The line under the card shows whole or not at all: part of it could
	// leave out what it is there to say.
	var info []string
	if len(pr.info) <= left {
		info = pr.info
	}
	left -= len(info)
	hints := pr.hints[:min(len(pr.hints), left)]
	left -= len(hints)
	room := least + left

	var win []string
	if room > 0 {
		// A page moves by the rows the window shows between its marks,
		// less one, so each page starts on the last row of the one
		// before and none is passed over.
		cs.Max, cs.Page = max(n-room, 0), max(room-1, 1)
		if room >= 3 {
			cs.Page = max(room-3, 1)
		}
		start := min(max(pr.top, 0), cs.Max)
		if pr.follow && focused {
			start = reveal(start, pr.selA, pr.selB, room, n)
		}
		cs.Top = start
		end := min(start+room, n)
		win = slices.Clone(pr.body[start:end])
		a, b := start, end
		if room >= 3 {
			if start > 0 {
				win[0] = styled(faint, fmt.Sprintf("↑ %d above", start))
				a++
			}
			if end < n {
				win[len(win)-1] = styled(faint, fmt.Sprintf("↓ %d below", n-end+1))
				b--
			}
		}
		// The focused line is on the screen when its first row is.
		cs.Shown = !focused || a <= pr.selA && pr.selA < b
	}
	if border > 0 {
		win = panel(f.title, f.right, f.foot, win, width, len(win)+2, f.focus)
	}
	for len(win) < room+border {
		win = append(win, "") // what is under the card stays at the bottom
	}
	all := append(append([]string(nil), pr.head...), win...)
	all = append(append(append(all, info...), pr.foot...), hints...)
	return finish(all, width, height), paneDrawn{Card: cs}
}

// leastBody is the fewest rows of body an open card is drawn with.
const leastBody = 3

// finish cuts lines to the pane: height rows, none wider than width.
func finish(all []string, width, height int) []string {
	all = all[:min(len(all), height)]
	for i, l := range all {
		all[i] = truncate(l, width)
	}
	for len(all) < height {
		all = append(all, "")
	}
	return selfContained(all)
}

// reveal is where a window of room rows over n starts so that rows a to
// b are on the screen, moved from start as little as it takes: the whole
// of them when they fit between the marks, else from their first row.
func reveal(start, a, b, room, n int) int {
	mark := 0
	if room >= 3 {
		mark = 1
	}
	if b-a > room-2*mark {
		b = a + 1 // too tall to show whole: from its first row
	}
	if b+mark > start+room {
		start = b + mark - room
	}
	if a-mark < start {
		start = a - mark
	}
	return min(max(start, 0), max(n-room, 0))
}

// paneActive is what the pane draws while it has the keys: its editor,
// the archive question, the question about a link, or itself.
func paneActive(a core.App, s core.State) core.SurfaceKind {
	switch b := a.Board; {
	case !b.Items && b.Input != nil:
		return core.SurfaceEditor // the prompt and its text
	case !b.Items && b.Confirm != 0:
		return core.SurfaceArchive
	case !b.Items && b.Links != nil && b.Links.Ask != nil:
		return core.SurfaceLinkAsk
	}
	return core.SurfacePane
}

// PaneLines is the pane drawn at width and height.
func PaneLines(a core.App, s core.State, now time.Time, width, height int) []string {
	lines, _ := paneRender(a, s, now, width, height)
	return lines
}
