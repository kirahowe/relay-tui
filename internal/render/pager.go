package render

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/kirahowe/relay-tui/internal/core"
)

// The review pager is the one surface consent is given from (see
// core/review.go). It shows the review text T verbatim: wrapped by
// display width, never cut, never escaped again, never shortened. Its
// lines, concatenated, are T, by construction (PagerLines), so what a
// frame reports drawn is exactly bytes of T. It fails closed: a T the
// core finds a problem with (a character that does not show as itself,
// a digest that does not match), a grapheme wider than the window, or a
// window too short for one row of it, is not shown, and the frame
// reports nothing drawn, so nothing arms.

// PagerLines is t wrapped at width display columns, measured by Method,
// as Bubble Tea measures them (see width.go). Each line is a piece of
// t that ends with the newline that ends it, if any, so the lines joined
// are t exactly; none is wider than width once its newline is dropped. A
// line breaks after a space where it can (not one at its own start), and
// between graphemes where it cannot; a grapheme is never split. ok is
// false when a grapheme is wider than width: then t cannot be shown at
// this width.
func PagerLines(t string, width int) (lines []string, ok bool) {
	width = max(width, 1)
	ok = true
	for rest := t; ; {
		seg, more := rest, ""
		nl := strings.IndexByte(rest, '\n')
		if nl >= 0 {
			seg, more = rest[:nl+1], rest[nl+1:]
		}
		ls, fits := wrapSegment(strings.TrimSuffix(seg, "\n"), width)
		ok = ok && fits
		if nl >= 0 {
			ls[len(ls)-1] += "\n"
		}
		lines = append(lines, ls...)
		if nl < 0 || more == "" {
			break
		}
		rest = more
	}
	return lines, ok
}

// wrapSegment wraps one line of text, with no newline in it, at width.
// It always returns at least one line.
func wrapSegment(seg string, width int) ([]string, bool) {
	var out []string
	ok := true
	start, cur, brk := 0, 0, -1 // brk: where the line can break, after a space
	for i := 0; i < len(seg); {
		g, _ := ansi.FirstGraphemeCluster(seg[i:], ansi.GraphemeWidth)
		if len(g) == 0 { // cannot happen with valid input; never loop
			g = seg[i : i+1]
		}
		w := Width(g)
		if w > width {
			ok = false
		}
		if cur+w > width && i > start {
			if brk > start {
				out = append(out, seg[start:brk])
				start = brk
				cur = Method.StringWidth(seg[brk:i])
			} else {
				out = append(out, seg[start:i])
				start, cur = i, 0
			}
			brk = -1
			if cur+w > width && i > start {
				out = append(out, seg[start:i])
				start, cur = i, 0
			}
		}
		cur += w
		i += len(g)
		if g == " " && i-len(g) > start {
			brk = i
		}
	}
	return append(out, seg[start:]), ok
}

// AmbiguousWidth is the first grapheme of t whose width by code point
// (wcwidth) and by grapheme cluster differ, such as an emoji with a skin
// tone: a terminal may draw it either way, so a review holding one could
// show less than the pager meant, and fails closed.
func AmbiguousWidth(t string) (string, bool) {
	for rest := t; rest != ""; {
		g, w := ansi.FirstGraphemeCluster(rest, ansi.GraphemeWidth)
		if len(g) == 0 {
			return "", false
		}
		if g != "\n" && w != ansi.StringWidthWc(g) {
			return g, true
		}
		rest = rest[len(g):]
	}
	return "", false
}

// PagerFrame is the review as drawn: the screen, what of T it drew
// (nil when it drew none, so nothing arms), and how far it scrolls and a
// page, for the keys.
type PagerFrame struct {
	Content   string
	Drawn     *core.ReviewDrawn
	Max, Page int
}

// ReviewHeading is what the review asks, by its subject.
func ReviewHeading(k core.SubjectKind) string {
	switch k {
	case core.SubjectRequest:
		return "Allow this tool call?"
	case core.SubjectCardTrust:
		return "Trust this card?"
	case core.SubjectArchive:
		return "Archive this card?"
	case core.SubjectDocTrust:
		return "Trust this doc?"
	}
	return "Review"
}

// Review draws the review that has the keys (surf over a and s) on the
// whole of a width by height screen. note is the status word to show
// (the hub's notice, the conversation's message).
func Review(surf core.Surface, a core.App, s core.State, note string, now time.Time, width, height int) PagerFrame {
	sub := core.ReviewSubject(surf, a, s)
	width, height = max(width, 1), max(height, 1)
	// The heading is relay-tui's own words only: what the agent wrote is
	// in T's body, whole, and nowhere else on the review.
	head := styled(bold|reverse, truncate(" "+ReviewHeading(sub.Kind)+" ", width))
	top := []string{head}
	if sub.Note != "" {
		for _, l := range capLines(wrap(styled(bold|warnColour, sub.Note), width), 2, width) {
			top = append(top, l)
		}
	}
	request := sub.Kind == core.SubjectRequest
	var foot []string
	pickDrawn := -1
	if request && len(s.Asks) > 0 {
		row, ok := optionsLine(s, core.ReviewArmed(surf, a, s, now), width)
		foot = append(foot, row)
		if ok {
			pickDrawn = s.Pick
		}
	}
	back := "esc cancel"
	if request {
		back = "n reject · esc cancel · q back"
	}
	fail := func(why string) PagerFrame {
		lines := append(top, wrap(styled(bold, why), width)...)
		lines = append(lines, wrap(styled(faint, "Nothing can be "+doneWord(sub.Kind)+" here. "+back), width)...)
		return PagerFrame{Content: strings.Join(fit(lines, height), "\n")}
	}
	if sub.Problem != "" {
		return fail("This review cannot be shown: " + sub.Problem + ".")
	}
	if g, ok := AmbiguousWidth(sub.Text); ok {
		var cps []string
		for _, r := range g {
			cps = append(cps, fmt.Sprintf("U+%04X", r))
		}
		return fail("This review holds " + strings.Join(cps, " ") + ", which terminals draw at different widths, so not all of it may show.")
	}
	lines, ok := PagerLines(sub.Text, width)
	if !ok {
		return fail("The window is too narrow for this review: make it wider.")
	}
	rows := height - len(top) - 1 - len(foot) - 2 // the rule, and the two rows of keys
	if rows < 1 {
		return fail("The window is too small for this review: make it larger.")
	}
	scroll := 0
	if a.Pager != nil {
		scroll = a.Pager.Scroll
	}
	mx := max(len(lines)-rows, 0)
	scroll = min(max(scroll, 0), mx)
	end := min(scroll+rows, len(lines))
	from := 0
	for _, l := range lines[:scroll] {
		from += len(l)
	}
	to := from
	out := slicesClone(top)
	for _, l := range lines[scroll:end] {
		to += len(l)
		out = append(out, strings.TrimSuffix(l, "\n"))
	}
	for len(out) < len(top)+rows {
		out = append(out, "")
	}
	out = append(out, styled(faint, strings.Repeat("─", width)))
	out = append(out, foot...)
	out = append(out, keysLines(sub, a, s, surf, note, now, lines, scroll, end, rows, width)...)
	return PagerFrame{
		Content: strings.Join(out, "\n"),
		Drawn:   &core.ReviewDrawn{ID: sub.ID, From: from, To: to, Pick: pickDrawn},
		Max:     mx, Page: rows,
	}
}

func slicesClone(s []string) []string { return append([]string(nil), s...) }

// fit is lines padded or cut to height rows.
func fit(lines []string, height int) []string {
	for len(lines) < height {
		lines = append(lines, "")
	}
	return lines[:height]
}

// doneWord is what a yes would do, for "nothing can be … here".
func doneWord(k core.SubjectKind) string {
	switch k {
	case core.SubjectRequest:
		return "allowed"
	case core.SubjectArchive:
		return "archived"
	}
	return "trusted"
}

// optionsLine is the request's options by their number in T and their
// ACP kind, the protocol's own words, the highlighted one marked. The
// names are the agent's, so they are only in T. Before the review is
// armed the allows are faint. When not all fit, the row shows a window
// of them that always holds the highlighted one, with ‹ and › where more
// lie; drawn says the highlighted entry is on the row whole, which enter
// needs (core's pickArmed). One that cannot fit at all is not drawn.
func optionsLine(s core.State, armed bool, width int) (line string, drawn bool) {
	p := s.Asks[0]
	if len(p.Options) == 0 {
		return styled(faint, truncate("nothing to choose · esc cancel", width)), false
	}
	entries := make([]string, len(p.Options))
	for i, o := range p.Options {
		mark := "  "
		if i == s.Pick {
			mark = "▸ "
		}
		t := fmt.Sprintf("%s%d %s", mark, p.Number(i), o.Kind)
		if k := optionKey(o.Kind); k != "" {
			t += " (" + k + ")"
		}
		st := style(0)
		if i == s.Pick {
			st = bold
		}
		if !armed && !o.Kind.Rejects() {
			st |= faint
		}
		entries[i] = styled(st, t)
	}
	const gap, more = "   ", "‹ "
	span := func(lo, hi int) int {
		w := Width(strings.Join(entries[lo:hi+1], gap))
		if lo > 0 {
			w += Width(more)
		}
		if hi < len(entries)-1 {
			w += Width(more)
		}
		return w
	}
	pick := min(max(s.Pick, 0), len(entries)-1)
	lo, hi := pick, pick
	if span(lo, hi) > width {
		return styled(bold, truncate("the highlighted option does not fit: widen the window", width)), false
	}
	for grew := true; grew; {
		grew = false
		if hi+1 < len(entries) && span(lo, hi+1) <= width {
			hi, grew = hi+1, true
		}
		if lo > 0 && span(lo-1, hi) <= width {
			lo, grew = lo-1, true
		}
	}
	line = strings.Join(entries[lo:hi+1], gap)
	if lo > 0 {
		line = styled(faint, "‹ ") + line
	}
	if hi < len(entries)-1 {
		line += styled(faint, " ›")
	}
	return line, s.Pick >= 0 && s.Pick < len(entries)
}

// keysLines is the review's foot: where it is in T and how much is left,
// then the keys, the yes shown armed once it is (or the status word,
// note, in their place, so the layout never moves).
func keysLines(sub core.Subject, a core.App, s core.State, surf core.Surface, note string, now time.Time, lines []string, scroll, end, rows, width int) []string {
	total := len(lines)
	pages := max((total+rows-1)/rows, 1)
	page := min(scroll/rows+1, pages)
	if end >= total {
		page = pages
	}
	where := fmt.Sprintf("page %d of %d", page, pages)
	if left := (total - end + rows - 1) / rows; left > 0 {
		where += fmt.Sprintf(" · %d more below: space", left)
	} else if n := unread(a.Pager, lines, rows); n > 0 && pages > 1 && !a.Pager.Read() {
		where += fmt.Sprintf(" · %s not on screen long enough yet: pgup to go back", plural(n, "page"))
	}
	yes := map[core.SubjectKind]string{
		core.SubjectRequest: "y allow", core.SubjectCardTrust: "y trust",
		core.SubjectDocTrust: "y trust", core.SubjectArchive: "y archive",
	}[sub.Kind]
	armed := core.ReviewArmed(surf, a, s, now)
	switch {
	case armed:
		yes = styled(bold|accent, yes)
	case !a.Pager.Read():
		yes = styled(faint, yes+" once all is read")
	default:
		yes = styled(faint, yes+" (a moment)")
	}
	rest := "esc cancel"
	switch sub.Kind {
	case core.SubjectRequest:
		rest = "n reject · esc cancel · q back · tab, enter choose"
	case core.SubjectArchive, core.SubjectCardTrust, core.SubjectDocTrust:
		rest = "n or esc cancel"
	}
	keys := yes + styled(faint, " · "+rest) // cut from the end: the keys that matter come first
	second := truncate(keys, width)
	if note != "" {
		second = styled(bold, truncate("! "+oneLine(note), width))
	}
	return []string{styled(faint, truncate(where, width)), second}
}

// unread is how many of the review's pages, as laid out now, hold a part
// of the text not yet drawn long enough to count (Pager.Seen).
func unread(p *core.Pager, lines []string, rows int) int {
	if p == nil {
		return 0
	}
	n, off := 0, 0
	for i := 0; i < len(lines); i += rows {
		from := off
		for _, l := range lines[i:min(i+rows, len(lines))] {
			off += len(l)
		}
		covered := from == off
		for _, sp := range p.Seen {
			if sp.From <= from && sp.To >= off {
				covered = true
			}
		}
		if !covered {
			n++
		}
	}
	return n
}
