package render

import (
	"strings"
)

// Wide hub mode draws its parts as panels: a rounded frame with the
// panel's title in its top border and a count or note at the right of it.
// The panel that has the keys has its frame in the accent colour; the
// others are faint. Key hints are written key first, the key in the
// accent colour and what it does faint.

// framed is the inside width of a panel width wide: a border and a space
// on each side.
func framed(width int) int { return max(width-4, 1) }

// panel draws lines in a frame exactly width wide and height tall. title
// and right sit in the top border, foot in the bottom one; all three are
// styled already. Lines are cut or padded to the inside width, and there
// are always height-2 of them, blank at the end.
func panel(title, right, foot string, lines []string, width, height int, focus bool) []string {
	if !boxed(width) || height < 2 {
		out := make([]string, 0, height)
		for i := 0; i < height; i++ {
			l := ""
			if i < len(lines) {
				l = truncate(lines[i], width)
			}
			out = append(out, l)
		}
		return out
	}
	bc := faint
	if focus {
		bc = accent
	}
	inner := framed(width)
	top := styled(bc, "╭─")
	if title != "" {
		top += " " + truncate(title, max(width-6, 1)) + " "
	}
	end := styled(bc, "╮")
	if right != "" {
		r := truncate(right, max(width-Method.StringWidth(top)-6, 0))
		if r != "" {
			end = " " + r + " " + styled(bc, "─╮")
		}
	}
	out := []string{edge(top, end, bc, width)}
	for i := 0; i < height-2; i++ {
		l := ""
		if i < len(lines) {
			l = lines[i]
		}
		l = truncate(l, inner)
		l += strings.Repeat(" ", max(inner-Method.StringWidth(l), 0))
		out = append(out, styled(bc, "│")+" "+l+sgr(0)+" "+styled(bc, "│"))
	}
	bottom := styled(bc, "╰")
	if foot != "" {
		bottom = styled(bc, "╰─") + " " + truncate(foot, max(width-6, 1)) + " "
	}
	return append(out, edge(bottom, styled(bc, "╯"), bc, width))
}

// edge fills a border row between left and right with ─ in style bc.
func edge(left, right string, bc style, width int) string {
	n := width - Method.StringWidth(left) - Method.StringWidth(right)
	return left + styled(bc, strings.Repeat("─", max(n, 0))) + right
}

// barRow is a row of a panel tinted edge to edge, as a selection is: line
// padded to width, every part of it on the tint.
func barRow(line string, width int) string {
	line = truncate(line, width)
	line += strings.Repeat(" ", max(width-Method.StringWidth(line), 0))
	return tint(line, selBar)
}

// tint lays bg under every part of an already styled line.
func tint(line string, bgst style) string {
	var b strings.Builder
	cur := style(0)
	b.WriteString(sgr(bgst))
	rest := line
	for {
		loc := sgrRE.FindStringIndex(rest)
		if loc == nil {
			b.WriteString(rest)
			break
		}
		b.WriteString(rest[:loc[0]])
		cur = parseSGR(rest[loc[0]:loc[1]])
		b.WriteString(sgr(cur&^(0xff<<bgShift) | bgst))
		rest = rest[loc[1]:]
	}
	b.WriteString(sgr(0))
	return b.String()
}

// tinted says the terminal's colours are known, so tints show.
func tinted() bool {
	_, ok := tintOf(tintChip)
	return ok
}

// pill is a label as the status line and a panel's title show it, when
// tints show: its name in colour on a tint. ok is false otherwise, and the
// caller shows the label its own way.
func pill(name string, colour style) (string, bool) {
	if !tinted() {
		return "", false
	}
	return styled(colour|pillBg|bold, " "+name+" "), true
}

// block lays lines on a tint edge to edge, width wide, with a tinted row
// above and below: the operator's own words in the conversation.
func block(lines []string, width int) []string {
	pad := func(l string) string {
		l = truncate(l, width)
		return tint(l+strings.Repeat(" ", max(width-Method.StringWidth(l), 0)), chipBg)
	}
	out := []string{pad("")}
	for _, l := range lines {
		out = append(out, pad(l))
	}
	return append(out, pad(""))
}

// chip is a key as a hint shows it on its own: the key in the accent
// colour on a tint, then what it does.
func chip(key, what string) string {
	return styled(accent|chipBg, " "+key+" ") + styled(faint, " "+what)
}

// styleKeys styles a hint written "key what · key what": each key in the
// accent colour, what it does faint, and a faint • between them. A part
// with no space in it is all key.
func styleKeys(hint string) string {
	var parts []string
	for _, p := range strings.Split(hint, " · ") {
		k, what, _ := strings.Cut(p, " ")
		s := styled(accent, k)
		if what != "" {
			s += styled(faint, " "+what)
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, styled(faint, " • "))
}

// hintRow is the row of key hints along the bottom of the screen, cut to
// width at a whole hint.
func hintRow(hint string, width int) string {
	parts := strings.Split(hint, " · ")
	for n := len(parts); n > 0; n-- {
		l := " " + styleKeys(strings.Join(parts[:n], " · "))
		if Method.StringWidth(l) <= width {
			return l
		}
	}
	return truncate(" "+styleKeys(hint), width)
}
