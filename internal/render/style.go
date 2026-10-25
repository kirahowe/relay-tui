// Package render turns core state into terminal text. It is pure: state,
// a width and a time in, strings out. The one input that is not an
// argument is the terminal's own colours (SetTerminalColors), which the
// shell learns once at startup.
//
// Colours come from the terminal's palette (the ANSI colours its theme
// sets), so the look follows a light or a dark theme. The tinted
// backgrounds of a selection, a key chip or a label pill are blends of the
// terminal's background and text colours; a terminal that does not say
// what they are gets reverse video for a selection and no tint for the
// rest. Every line it returns is self-contained: it opens the styles it
// needs and resets them at its end, so lines can be printed, cut or
// truncated one at a time.
package render

import (
	"fmt"
	"image/color"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"unicode"
)

// style is a set of SGR attributes, with a text colour in bits 8-15 and a
// background in bits 16-23, each a hue.
type style uint32

const (
	bold style = 1 << iota
	faint
	italic
	reverse
	underline
	strike
)

// hue is a colour: one of the terminal's own, or a tint made from its
// background.
type hue uint8

const (
	hueNone hue = iota
	hueRed
	hueGreen
	hueYellow
	hueBlue
	hueMagenta
	hueCyan
	// Tints, for backgrounds only: a selected row, a key chip or a block
	// of the operator's own text, and a label pill.
	tintSel
	tintChip
	tintPill
)

const (
	fgShift = 8
	bgShift = 16
)

func fg(h hue) style { return style(h) << fgShift }
func bg(h hue) style { return style(h) << bgShift }

func (st style) fg() hue      { return hue(st >> fgShift) }
func (st style) bg() hue      { return hue(st >> bgShift) }
func (st style) attrs() style { return st & 0xff }

// The colours by what they mean.
var (
	accent      = fg(hueCyan)    // what has focus, and keys
	okColour    = fg(hueGreen)   // done, and a call that worked
	warnColour  = fg(hueYellow)  // text from outside
	errColour   = fg(hueRed)     // a failure
	labelColour = fg(hueMagenta) // a label pill's text
	selBar      = bg(tintSel)
	chipBg      = bg(tintChip)
	pillBg      = bg(tintPill)
)

type rgb struct{ r, g, b uint8 }

// terminalColors is the terminal's background and text colour, once it
// has said; nil until then.
var terminalColors atomic.Pointer[[2]rgb]

// SetTerminalColors records the terminal's background and text colours,
// from which the tints are blended. Either nil leaves them unknown.
func SetTerminalColors(background, foreground color.Color) {
	if background == nil || foreground == nil {
		terminalColors.Store(nil)
		return
	}
	terminalColors.Store(&[2]rgb{toRGB(background), toRGB(foreground)})
}

func toRGB(c color.Color) rgb {
	r, g, b, _ := c.RGBA()
	return rgb{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)}
}

// tintMix is how far each tint moves from the background toward the text.
var tintMix = map[hue]float64{tintSel: 0.16, tintChip: 0.08, tintPill: 0.14}

// tintOf is the colour of tint h, and false when the terminal's colours
// are not known.
func tintOf(h hue) (rgb, bool) {
	tc := terminalColors.Load()
	if tc == nil {
		return rgb{}, false
	}
	bgc, fgc, f := tc[0], tc[1], tintMix[h]
	mix := func(a, b uint8) uint8 { return uint8(float64(a) + (float64(b)-float64(a))*f + 0.5) }
	return rgb{mix(bgc.r, fgc.r), mix(bgc.g, fgc.g), mix(bgc.b, fgc.b)}, true
}

// sgr is the sequence that switches the terminal to exactly st.
func sgr(st style) string {
	if st == 0 {
		return "\x1b[m"
	}
	codes := "0"
	rev := st&reverse != 0
	if b := st.bg(); b >= tintSel {
		if c, ok := tintOf(b); ok {
			codes += fmt.Sprintf(";48;2;%d;%d;%d", c.r, c.g, c.b)
		} else if b == tintSel {
			rev = true // without the terminal's colours a selection is reversed
		}
	}
	if st&bold != 0 {
		codes += ";1"
	}
	if st&faint != 0 {
		codes += ";2"
	}
	if st&italic != 0 {
		codes += ";3"
	}
	if st&underline != 0 {
		codes += ";4"
	}
	if rev {
		codes += ";7"
	}
	if st&strike != 0 {
		codes += ";9"
	}
	if f := st.fg(); f >= hueRed && f <= hueCyan {
		codes += ";" + strconv.Itoa(30+int(f))
	}
	return "\x1b[" + codes + "m"
}

// styled wraps text, which has no escapes of its own, in st.
func styled(st style, text string) string {
	if st == 0 || text == "" {
		return text
	}
	return sgr(st) + text + sgr(0)
}

var sgrRE = regexp.MustCompile(`\x1b\[([0-9;]*)m`)

// parseSGR returns the style a sequence made by sgr switches to. A tint
// comes back as the tint whose colour it is now; reverse video stands for
// itself.
func parseSGR(seq string) style {
	var st style
	codes := strings.Split(sgrRE.FindStringSubmatch(seq)[1], ";")
	for i := 0; i < len(codes); i++ {
		switch n, _ := strconv.Atoi(codes[i]); {
		case n == 1:
			st |= bold
		case n == 2:
			st |= faint
		case n == 3:
			st |= italic
		case n == 4:
			st |= underline
		case n == 7:
			st |= reverse
		case n == 9:
			st |= strike
		case n >= 31 && n <= 36:
			st = st&^(0xff<<fgShift) | fg(hue(n-30))
		case n == 48 && i+4 < len(codes) && codes[i+1] == "2":
			r, _ := strconv.Atoi(codes[i+2])
			g, _ := strconv.Atoi(codes[i+3])
			b, _ := strconv.Atoi(codes[i+4])
			i += 4
			for _, t := range []hue{tintSel, tintChip, tintPill} {
				if c, ok := tintOf(t); ok && c == (rgb{uint8(r), uint8(g), uint8(b)}) {
					st = st&^(0xff<<bgShift) | bg(t)
				}
			}
		}
	}
	return st
}

// selfContained makes each line open the style the previous line left
// on, and reset at its end.
func selfContained(lines []string) []string {
	var cur style
	out := make([]string, len(lines))
	for i, l := range lines {
		open := cur
		for _, seq := range sgrRE.FindAllString(l, -1) {
			cur = parseSGR(seq)
		}
		if open != 0 {
			l = sgr(open) + l
		}
		if cur != 0 {
			l += sgr(0)
		}
		out[i] = l
	}
	return out
}

// sanitize drops control characters, including ESC, so text from the agent
// cannot move the cursor or change styles, and format characters. Newlines stay; a tab becomes a
// space.
func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n':
			return r
		case r == '\t':
			return ' '
		case r < 0x20, r == 0x7f, r >= 0x80 && r <= 0x9f:
			return -1
		case unicode.Is(unicode.Cf, r) && r != 0x200d:
			// Format characters (bidi controls, U+0600 and the like) have
			// no width of their own but change how their neighbours
			// cluster, which breaks width sums. The zero-width joiner stays
			// for emoji.
			return -1
		}
		return r
	}, s)
}

// oneLine is sanitized text with line breaks turned into spaces.
func oneLine(s string) string { return strings.ReplaceAll(sanitize(s), "\n", " ") }

// expandTabs replaces tabs with spaces to the next multiple of 4 columns,
// then sanitizes.
func expandTabs(line string) string {
	if !strings.Contains(line, "\t") {
		return sanitize(line)
	}
	var b strings.Builder
	col := 0
	for _, r := range line {
		if r == '\t' {
			n := 4 - col%4
			b.WriteString(strings.Repeat(" ", n))
			col += n
			continue
		}
		b.WriteRune(r)
		col += Method.StringWidth(string(r))
	}
	return sanitize(b.String())
}

// wrap word-wraps styled text to width and returns self-contained lines.
// ansi.Wrap can leave a line too wide (it does with "  -  ·   -" at 8
// columns), so any such line is then cut hard.
func wrap(s string, width int) []string {
	width = max(width, 1)
	var lines []string
	for _, l := range strings.Split(Method.Wrap(s, width, ""), "\n") {
		if Method.StringWidth(l) > width {
			lines = append(lines, hardwrap(l, width)...)
		} else {
			lines = append(lines, l)
		}
	}
	return selfContained(lines)
}

// hardwrap cuts unstyled text at width without looking for word breaks.
// A grapheme wider than the width on its own (a long ZWJ sequence,
// measured by code point) cannot be wrapped, and is cut: the dashboard
// may be lossy, and never draws past its width.
func hardwrap(s string, width int) []string {
	width = max(width, 1)
	lines := strings.Split(Method.Hardwrap(s, width, true), "\n")
	for i, l := range lines {
		if Width(l) > width {
			lines[i] = truncate(l, width)
		}
	}
	return lines
}

// truncate cuts one line to width, ending it with … when cut. ansi's
// Truncate can measure a cluster differently from StringWidth (it did
// with "A" and U+FE0F), so the result is measured and cut again if needed.
func truncate(s string, width int) string {
	if Method.StringWidth(s) <= width {
		return s
	}
	for n := width; n > 0; n-- {
		r := selfContained([]string{Method.Truncate(s, n, "…")})[0]
		if Method.StringWidth(r) <= width {
			return r
		}
	}
	return ""
}

// Faint is text on one line, faint, cut to width.
func Faint(text string, width int) string { return styled(faint, truncate(oneLine(text), width)) }
