package render

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/kirahowe/relay-tui/internal/core"
)

// This file decides what the dashboard gives a terminal to follow. It
// is the only place that does.
//
// Text an outsider may have shaped has its links broken (Defang), all
// but relay's live links: an address that is exactly a member of the set
// relay sent this connection (core.Links) is drawn as an OSC 8
// hyperlink, between angle brackets. The terminal is told the whole
// address, so a click opens the one relay judged whatever the pane does
// to the text it shows for it.
//
// Three steps hold that up, each on its own:
//
//   - fang makes outside text safe where it enters: a live link becomes
//     a link span, everything else is broken.
//   - wrap and truncate keep a link span whole on one row, or show it in
//     its broken form. A row never shows part of a live link as it is,
//     since the start of an address is another address, one nobody took,
//     and a terminal that does not know OSC 8 would follow it.
//   - gate, on every frame that leaves render, lets a hyperlink through
//     only when its target is in the set, and only over text that is the
//     whole address or no address at all. Whatever happened to a span
//     between fang and the screen (a case change, a cut, a rewrite), a
//     target that is not a live link never reaches the terminal.
//
// A link span is plain text with an OSC 8 sequence either side and no
// style of its own: gate gives it its look, so the code between can
// style, tint and recolour the text around it as it always has.

// linkEnd ends a hyperlink.
const linkEnd = "\x1b]8;;\x07"

// linkOpen starts a hyperlink to target.
func linkOpen(target string) string { return "\x1b]8;;" + target + "\x07" }

// osc8RE finds a hyperlink sequence. Its group is the target, empty where
// a link ends.
var osc8RE = regexp.MustCompile("\x1b\\]8;;([^\x07\x1b]*)\x07")

// hasLinks says s may hold a link span.
func hasLinks(s string) bool { return strings.Contains(s, "\x1b]8;") }

// span is a live link as fang writes it: the address between angle
// brackets, the marks RFC 3986 gives for an address in running text,
// inside a hyperlink to it.
func span(link string) string { return linkOpen(link) + whole(link) + linkEnd }

// whole is how a live link reads on a row that has room for it.
func whole(link string) string { return "<" + link + ">" }

// brokenForm is how a live link reads where a row has no room for it:
// broken as any other link is, so no part of it can be followed as text.
func brokenForm(link string) string { return "<" + Defang(link) + ">" }

var wordRE = regexp.MustCompile(`\S+`)

// fang makes outside text safe to draw: every link in it broken, except
// that a word holding a live link (core.LinkIn) keeps it, as a link
// span. What stands before the link in its word has every dot broken, as
// relay breaks a word that holds a link. t has no escape sequences: the
// caller has escaped or sanitized it. With no live links every link is
// broken.
func fang(t string, live core.Links) string {
	return wordRE.ReplaceAllStringFunc(t, func(w string) string {
		before, link, after, ok := core.LinkIn(w, live)
		if !ok {
			return breakWord(w)
		}
		before, after = unbracket(before, after)
		return breakDots(breakWord(before)) + span(link) + after
	})
}

// fangSource is fang for Markdown source: a live link stays as text, in
// Markdown's own angle brackets, so the parser reads it as a link and the
// renderer draws the span (mdRenderer.link). Every other link is broken.
func fangSource(t string, live core.Links) string {
	return wordRE.ReplaceAllStringFunc(t, func(w string) string {
		before, link, after, ok := core.LinkIn(w, live)
		if !ok {
			return breakWord(w)
		}
		before, after = unbracket(before, after)
		return breakDots(breakWord(before)) + whole(link) + after
	})
}

// breakWord breaks the links in a word that holds one a terminal could
// follow. A word relay broke already passes as it is: Defang would break
// its dots a second time.
func breakWord(w string) string {
	if linkable(w) {
		return Defang(w)
	}
	return w
}

// unbracket takes the angle brackets a live link already stands between
// off what is before and after it.
func unbracket(before, after string) (string, string) {
	if strings.HasSuffix(before, "<") && strings.HasPrefix(after, ">") {
		return before[:len(before)-1], after[1:]
	}
	return before, after
}

// breakDots breaks every dot in s.
func breakDots(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "[.]", "."), ".", "[.]")
}

// piece is a run of a row: its text, with whatever styles it has, and
// the hyperlink it sits in, empty outside one.
type piece struct{ text, target string }

// pieces splits a row at its hyperlink sequences.
func pieces(row string) []piece {
	var out []piece
	target, at := "", 0
	for _, m := range osc8RE.FindAllStringSubmatchIndex(row, -1) {
		if m[0] > at {
			out = append(out, piece{row[at:m[0]], target})
		}
		target, at = row[m[2]:m[3]], m[1]
	}
	if at < len(row) {
		out = append(out, piece{row[at:], target})
	}
	return out
}

// join puts pieces back into a row.
func join(ps []piece) string {
	var b strings.Builder
	for _, p := range ps {
		if p.target == "" {
			b.WriteString(p.text)
			continue
		}
		b.WriteString(linkOpen(p.target) + p.text + linkEnd)
	}
	return b.String()
}

// hyphenStand stands for a hyphen in a link that is to stay on one row,
// while the text is wrapped: the wrapper breaks a word at a hyphen.
const hyphenStand = "\ue000"

// fitLinks readies the link spans in s for rows of width cells. A word
// that fits a row keeps its links whole and will not be broken across
// rows; in a word longer than a row the links are shown in their broken
// form, since such a word is cut wherever the row ends. held says hyphens
// were stood in for, to be put back once the text is wrapped.
func fitLinks(s string, width int) (out string, held bool) {
	if !hasLinks(s) {
		return s, false
	}
	// Text that already holds the stand-in cannot be told from one
	// stood in for, so nothing is held: every link shows broken.
	can := !strings.Contains(s, hyphenStand)
	out = wordRE.ReplaceAllStringFunc(s, func(w string) string {
		if !hasLinks(w) {
			return w
		}
		fits := can && Method.StringWidth(w) <= width
		ps := pieces(w)
		for i, p := range ps {
			switch {
			case p.target == "" || p.text != whole(p.target):
			case fits:
				ps[i].text = strings.ReplaceAll(p.text, "-", hyphenStand)
				held = true
			default:
				ps[i].text = brokenForm(p.target)
			}
		}
		return join(ps)
	})
	return out, held
}

// cutLinks shows in its broken form every whole link in s that a cut to
// width cells would not keep whole. Breaking a link makes it longer, so
// the cut still falls inside it, in text that is already broken.
func cutLinks(s string, width int) string {
	if !hasLinks(s) {
		return s
	}
	ps := pieces(s)
	col := 0
	for i, p := range ps {
		w := Method.StringWidth(p.text)
		// A cut keeps width-1 cells and ends with an ellipsis.
		if p.target != "" && p.text == whole(p.target) && col+w > width-1 {
			ps[i].text = brokenForm(p.target)
		}
		col += w
	}
	return join(ps)
}

// schemeRunRE finds a "://" with a scheme before it: a broken http
// scheme's, which is longer than the other match, one character of any
// other scheme and the "://".
var schemeRunRE = regexp.MustCompile(`(?i)hxxps?://|[a-z0-9+.\-]://`)

// linkable says whether text holds something a terminal could follow by
// itself: a "://" after a scheme that is not a broken http one, or a
// www. host. A "://" with no scheme before it is not an address.
func linkable(plain string) bool {
	for _, m := range schemeRunRE.FindAllString(plain, -1) {
		if len(m) == 4 {
			return true
		}
	}
	return strings.Contains(strings.ToLower(plain), "www.")
}

// cutTailRE is what a row can begin with when a wrap or a cut fell
// inside the scheme of a broken link: the end of hxxp or hxxps, then
// "://". None of them is a scheme, and the address after it is broken.
var cutTailRE = regexp.MustCompile(`^\s*(?i:xxps?|xps?|ps?|s)://`)

// linkableRow is linkable for a row, or a piece of one, as drawn: what a
// cut left of a broken scheme at its start does not count. A row that
// only holds the rest of a broken link then stays as relay sent it.
func linkableRow(plain string) bool {
	if loc := cutTailRE.FindStringIndex(plain); loc != nil {
		plain = plain[loc[1]:]
	}
	return linkable(plain)
}

// unsafeRow says a rendered row of outside text shows something a
// terminal could follow other than a whole live link in its span.
func unsafeRow(row string) bool {
	for _, p := range pieces(row) {
		plain := ansi.Strip(p.text)
		if p.target != "" && plain == whole(p.target) {
			continue
		}
		if linkableRow(plain) {
			return true
		}
	}
	return false
}

// linkStyle is how a live link looks on top of the style around it: its
// own colour, underlined.
func linkStyle(st style) style {
	return st&^(0xff<<fgShift) | fg(hueBlue) | underline
}

// gate is the last thing every frame goes through. It lets a hyperlink
// reach the terminal only when its target is a live link, and only over
// text that is that whole address or holds no address at all; any other
// span loses its hyperlink or has its text broken, cut back to the cells
// it had so the frame keeps its shape. What passes gets a link's look.
func gate(content string, live core.Links) string {
	if !hasLinks(content) {
		return content
	}
	rows := strings.Split(content, "\n")
	for i, r := range rows {
		if hasLinks(r) {
			rows[i] = gateRow(r, live)
		}
	}
	return strings.Join(rows, "\n")
}

func gateRow(row string, live core.Links) string {
	var b strings.Builder
	cur := style(0)
	// through writes text and follows the style it leaves on. In a link
	// every style in it is the link's look over that style.
	through := func(text string, link bool) {
		rest := text
		for {
			loc := sgrRE.FindStringIndex(rest)
			if loc == nil {
				b.WriteString(rest)
				return
			}
			b.WriteString(rest[:loc[0]])
			cur = parseSGR(rest[loc[0]:loc[1]])
			if link {
				b.WriteString(sgr(linkStyle(cur)))
			} else {
				b.WriteString(rest[loc[0]:loc[1]])
			}
			rest = rest[loc[1]:]
		}
	}
	for _, p := range pieces(row) {
		switch {
		case p.target == "":
			through(p.text, false)
		case !live.Has(p.target):
			// Not a live link: no hyperlink, and no address left in the
			// text either.
			through(truncate(Defang(p.text), Method.StringWidth(p.text)), false)
		default:
			text := p.text
			if plain := ansi.Strip(text); plain != whole(p.target) && linkableRow(plain) {
				text = truncate(Defang(text), Method.StringWidth(text))
			}
			b.WriteString(linkOpen(p.target) + sgr(linkStyle(cur)))
			through(text, true)
			b.WriteString(sgr(cur) + linkEnd)
		}
	}
	return b.String()
}
