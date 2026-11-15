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
// Text an outsider may have shaped is inert: every link in it is shown
// broken (Defang), relay's live links included. A live link is an address
// that is exactly a member of the set relay sent this connection
// (core.Links), and it differs from any other in one way: its broken text
// is an OSC 8 hyperlink whose target is the address. The address reaches
// the terminal in that target and nowhere else, so what a click opens
// never depends on how a terminal, or anything else, reads text. Where
// OSC 8 does not get through there is nothing to follow.
//
// Two steps hold that up, each on its own:
//
//   - fang makes outside text safe where it enters: every link is
//     broken, and a live one becomes a link span over its broken form.
//   - gate, on every frame that leaves render, lets a hyperlink through
//     only when its target is in the set, and breaks whatever in its text
//     could be followed. Whatever happened to a span between fang and the
//     screen (a case change, a cut, a rewrite), a target that is not a
//     live link never reaches the terminal.
//
// A link span is plain text with an OSC 8 sequence either side and no
// style of its own: gate gives it its look, so the code between can
// style, tint and recolour the text around it as it always has. It can be
// wrapped or cut anywhere, since no part of a broken link can be
// followed, and each row's piece of it opens the whole address.

// linkEnd ends a hyperlink.
const linkEnd = "\x1b]8;;\x07"

// linkOpen starts a hyperlink to target.
func linkOpen(target string) string { return "\x1b]8;;" + target + "\x07" }

// osc8RE finds a hyperlink sequence. Its group is the target, empty where
// a link ends.
var osc8RE = regexp.MustCompile("\x1b\\]8;;([^\x07\x1b]*)\x07")

// hasLinks says s may hold a link span.
func hasLinks(s string) bool { return strings.Contains(s, "\x1b]8;") }

// span is a live link as fang writes it: a hyperlink to the address,
// over the address in its broken form. A dot that ends the address is
// broken with the rest: it is part of the address, where the last dot of
// a word of prose is a full stop.
func span(link string) string { return linkOpen(link) + breakDots(Defang(link), true) + linkEnd }

// autolink is a live link between Markdown's angle brackets, where the
// parser reads it as a link. Only Markdown source holds one, never a row.
func autolink(link string) string { return "<" + link + ">" }

var wordRE = regexp.MustCompile(`\S+`)

// fang makes outside text safe to draw: every link in it broken, and a
// live link in a word (core.LinkIn) made a link span over its broken
// form. What stands before the link in its word has every dot broken, as
// relay breaks a word that holds a link. t has no escape sequences: the
// caller has escaped or sanitized it. With no live links no word gets a
// span.
//
// Every word goes through Defang, whatever it looks like: one function
// says what a link is, and no second test decides which words it sees.
// Defang changes nothing in a word it or relay broke already, so text
// relay made safe is drawn as it came.
func fang(t string, live core.Links) string {
	return wordRE.ReplaceAllStringFunc(t, func(w string) string {
		before, link, after, ok := core.LinkIn(w, live)
		if !ok {
			return Defang(w)
		}
		before, after = unbracket(before, after)
		return breakDots(Defang(before), true) + span(link) + after
	})
}

// fangSource is fang for Markdown source: a live link stays as text, in
// Markdown's own angle brackets, so the parser reads it as a link and the
// renderer draws the span over its broken form (mdRenderer.link). Every
// other link is broken here.
func fangSource(t string, live core.Links) string {
	return wordRE.ReplaceAllStringFunc(t, func(w string) string {
		before, link, after, ok := core.LinkIn(w, live)
		if !ok {
			return Defang(w)
		}
		before, after = unbracket(before, after)
		return breakDots(Defang(before), true) + autolink(link) + after
	})
}

// unbracket takes the angle brackets a live link already stands between
// off what is before and after it.
func unbracket(before, after string) (string, string) {
	if strings.HasSuffix(before, "<") && strings.HasPrefix(after, ">") {
		return before[:len(before)-1], after[1:]
	}
	return before, after
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

// linkable says whether text holds something a terminal could follow by
// itself: a "://" after anything but a broken scheme, or a www. host. It
// is the check on rows as drawn, and what it finds is what Defang breaks
// (schemeSepRE is Defang's own).
func linkable(plain string) bool {
	for _, m := range schemeSepRE.FindAllString(plain, -1) {
		if len(m) == 3 {
			return true
		}
	}
	return wwwHostRE.MatchString(plain)
}

// wwwHostRE finds www. with more of a word after it. A www. that ends its
// word, or has only a closing mark after it, names no host: Defang leaves
// that dot as relay does, the full stop after a word that happens to be
// www. Anything else after it may be read as a host, or as a user name
// before one (www.@evil.test).
var wwwHostRE = regexp.MustCompile(`(?i)www\.[^\s)>".,;!\]':?*~]`)

// cutTailRE is what a row can begin with when a wrap or a cut fell
// inside the scheme of a broken link, or straight after it: the end of
// hxxp, hxxps or fxp, or nothing, then "://". None of them is a scheme,
// and the address after it is broken.
var cutTailRE = regexp.MustCompile(`^\s*(?i:xxps?|xps?|ps?|s)?://`)

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
// terminal could follow as text. A hyperlink's own text counts as any
// other does: a live link's is its broken form.
func unsafeRow(row string) bool {
	for _, p := range pieces(row) {
		if linkableRow(ansi.Strip(p.text)) {
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
// text that holds no address a terminal could follow. A span to any other
// target loses its hyperlink, and text in a span that could be followed is
// broken, cut back to the cells it had so the frame keeps its shape. What
// passes gets a link's look.
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
			if linkableRow(ansi.Strip(text)) {
				text = truncate(Defang(text), Method.StringWidth(text))
			}
			b.WriteString(linkOpen(p.target) + sgr(linkStyle(cur)))
			through(text, true)
			b.WriteString(sgr(cur) + linkEnd)
		}
	}
	return b.String()
}
