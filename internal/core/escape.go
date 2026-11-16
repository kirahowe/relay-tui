package core

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// This file is how text the agent wrote is shown. Nothing is dropped: a
// character that cannot be seen as itself shows as a \u escape, so what
// is on screen is what was sent. The escapes are JSON's, so the text
// reads back to the original.

// escapeRune appends r to b, as itself when it prints, else as \uXXXX
// (a surrogate pair above U+FFFF, as JSON does).
func escapeRune(b *strings.Builder, r rune) {
	switch {
	case r > 0xFFFF && r <= unicode.MaxRune:
		r -= 0x10000
		fmt.Fprintf(b, `\u%04x\u%04x`, 0xD800+(r>>10), 0xDC00+(r&0x3FF))
	default:
		fmt.Fprintf(b, `\u%04x`, r)
	}
}

// printable says whether r can be shown as itself. A space is the only
// blank that can: unicode.IsPrint rejects every other separator, the
// controls, the format characters (bidi marks among them), private use,
// surrogates and unassigned code points. Neither can a code point that
// draws as nothing or as an empty space (blankGlyph), since a run of them
// can carry text nobody sees, such as variation selectors.
func printable(r rune) bool { return unicode.IsPrint(r) && !blankGlyph(r) }

// defaultIgnorable is Unicode's Default_Ignorable_Code_Point ranges, as
// relay.text keeps them: code points a renderer draws as nothing. Some
// are marks or letters, so IsPrint lets them through.
var defaultIgnorable = [][2]rune{
	{0x00AD, 0x00AD}, {0x034F, 0x034F}, {0x061C, 0x061C}, {0x115F, 0x1160}, {0x17B4, 0x17B5},
	{0x180B, 0x180F}, {0x200B, 0x200F}, {0x202A, 0x202E}, {0x2060, 0x206F}, {0x3164, 0x3164},
	{0xFE00, 0xFE0F}, {0xFEFF, 0xFEFF}, {0xFFA0, 0xFFA0}, {0xFFF0, 0xFFF8}, {0x1BCA0, 0x1BCA3},
	{0x1D173, 0x1D17A}, {0xE0000, 0xE0FFF},
}

// blankGlyph says whether r is default-ignorable, or draws as an empty
// space or as nothing without being one: the braille blank, the musical
// null notehead, the Khitan filler, and two marks that are invisible by
// design, the Brahmi number joiner and the Duployan thick letter
// selector (relay.text/renders-blank).
func blankGlyph(r rune) bool {
	switch r {
	case 0x2800, 0x1D159, 0x16FE4, 0x1107F, 0x1BC9D:
		return true
	}
	for _, rg := range defaultIgnorable {
		if r >= rg[0] && r <= rg[1] {
			return true
		}
	}
	return false
}

// NoAdvance says r takes no cell of its own, so a link finder may read
// past it as though it were not there (relay#130): what a common width
// table (Rust's unicode-width, which Alacritty uses) gives no cell. That
// is a Grapheme_Extend character (Mn, Me, and the spacing marks Unicode
// lists as Other_Grapheme_Extend, U+0BBE among them), a character that
// joins the one after it (Grapheme_Cluster_Break=Prepend, and U+A8FA), a
// format character, a character that draws as nothing (blankGlyph), or a
// Hangul jamo vowel or final that joins the syllable before it. Such a
// character can hide an address from a breaker that goes by letters.
func NoAdvance(r rune) bool {
	if r < 0x300 {
		return r == 0xAD
	}
	return unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf, unicode.Other_Grapheme_Extend) || blankGlyph(r) ||
		(r >= 0x1160 && r <= 0x11FF) || (r >= 0xD7B0 && r <= 0xD7FF) || prepend(r)
}

// prepend says r is Grapheme_Cluster_Break=Prepend (Unicode 16's list,
// which Go's tables do not carry) or U+A8FA, which unicode-width also
// gives no cell.
func prepend(r rune) bool {
	switch {
	case r >= 0x0600 && r <= 0x0605, r == 0x06DD, r == 0x070F, r >= 0x0890 && r <= 0x0891, r == 0x08E2,
		r == 0x0D4E, r == 0x110BD, r == 0x110CD, r >= 0x111C2 && r <= 0x111C3, r == 0x113D1, r == 0x1193F,
		r == 0x11941, r == 0x11A3A, r >= 0x11A84 && r <= 0x11A89, r == 0x11D46, r == 0x11F02, r == 0xA8FA:
		return true
	}
	return false
}

// DropHidden is s with every hidden character taken out but the ones that
// hold an emoji together, and with a line or paragraph separator made a
// line break. It is for text whose links are about to be broken, where
// the escapes above would spoil ordinary emoji. A character nobody sees
// inside an address hides the address from whatever looks for one by its
// letters, and a terminal's link finder may read straight past it. With
// them gone, what is looked at is what is seen.
//
// What is hidden and what is glue are relay's own rules (relay.text:
// hidden?, emoji-glue? and the way sanitize strips), so relay and its
// clients take out the same characters.
func DropHidden(s string) string {
	rs := []rune(s)
	out := make([]rune, 0, len(rs))
	for i, r := range rs {
		switch {
		case r == 0x85 || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r):
			out = append(out, '\n')
		case !hidden(r), r == '\r', r >= 0x1F000 && r <= 0x1FFFF && unassigned(r):
			// A carriage return stays, as relay keeps it, for whoever
			// lays the text out. So does a code point of the emoji
			// blocks that this build's Unicode has no name for yet: it
			// is more likely a newer emoji than nothing.
			out = append(out, r)
		default:
			var prev, next rune = -1, -1
			if len(out) > 0 {
				prev = out[len(out)-1]
			}
			if i+1 < len(rs) {
				next = rs[i+1]
			}
			if emojiGlue(r, prev, next) {
				out = append(out, r)
			}
		}
	}
	if len(out) == len(rs) && string(out) == s {
		return s
	}
	return string(out)
}

// hidden says r is a character relay shows escaped, or takes out
// (relay.text/hidden?): a control, a format character, a surrogate, one
// in private use or not assigned, a line or paragraph separator, or one
// that draws as nothing or as an empty space without being a space.
// Newline and tab are not.
func hidden(r rune) bool {
	if r == '\n' || r == '\t' {
		return false
	}
	return unicode.In(r, unicode.Cc, unicode.Cf, unicode.Cs, unicode.Co, unicode.Zl, unicode.Zp) || unassigned(r) || blankGlyph(r)
}

// unassigned says no character is assigned to r in the Unicode this was
// built with. The four kinds of Other are named one by one: unicode.C
// holds the unassigned code points too.
func unassigned(r rune) bool {
	return !unicode.In(r, unicode.L, unicode.M, unicode.N, unicode.P, unicode.S, unicode.Z,
		unicode.Cc, unicode.Cf, unicode.Cs, unicode.Co)
}

// pictographicRanges is Unicode's Extended_Pictographic, near enough, as
// relay.text keeps it: the code points emoji sequences are made of. The
// enclosed and squared Latin letters and the regional indicators are left
// out, as Unicode leaves them out.
var pictographicRanges = [][2]rune{
	{0x00A9, 0x00A9}, {0x00AE, 0x00AE}, {0x203C, 0x203C}, {0x2049, 0x2049}, {0x2122, 0x2122}, {0x2139, 0x2139},
	{0x2194, 0x2199}, {0x21A9, 0x21AA}, {0x231A, 0x231B}, {0x2328, 0x2328}, {0x2388, 0x2388}, {0x23CF, 0x23CF},
	{0x23E9, 0x23F3}, {0x23F8, 0x23FA}, {0x24C2, 0x24C2}, {0x25AA, 0x25AB}, {0x25B6, 0x25B6}, {0x25C0, 0x25C0},
	{0x25FB, 0x25FE}, {0x2600, 0x27BF}, {0x2934, 0x2935}, {0x2B05, 0x2B07}, {0x2B1B, 0x2B1C}, {0x2B50, 0x2B50},
	{0x2B55, 0x2B55}, {0x3030, 0x3030}, {0x303D, 0x303D}, {0x3297, 0x3297}, {0x3299, 0x3299},
	{0x1F000, 0x1F0FF}, {0x1F10D, 0x1F10F}, {0x1F12F, 0x1F12F}, {0x1F16C, 0x1F171}, {0x1F17E, 0x1F17F},
	{0x1F18E, 0x1F18E}, {0x1F191, 0x1F19A}, {0x1F1AD, 0x1F1E5}, {0x1F201, 0x1FAFF}, {0x1FC00, 0x1FFFD},
}

// pictographic says r is an emoji, or part of one such as a skin tone,
// that a zero-width joiner or a variation selector may follow.
func pictographic(r rune) bool {
	for _, rg := range pictographicRanges {
		if r >= rg[0] && r <= rg[1] {
			return true
		}
	}
	return false
}

// emojiGlue says r, a zero-width joiner or a variation selector, holds an
// emoji sequence together (relay.text/emoji-glue?): a joiner between an
// emoji, or its selector, and the next emoji, as in a family; a selector
// right after an emoji, or after a keycap's digit before the keycap. prev
// is the character kept before it and next the one after it in the
// text, each -1 when there is none.
func emojiGlue(r, prev, next rune) bool {
	switch r {
	case 0x200D:
		return prev >= 0 && next >= 0 && pictographic(next) && (pictographic(prev) || prev == 0xFE0F || prev == 0xFE0E)
	case 0xFE0F, 0xFE0E:
		return prev >= 0 && (pictographic(prev) || ((prev == '#' || prev == '*' || (prev >= '0' && prev <= '9')) && next == 0x20E3))
	}
	return false
}

// EscapeNonPrinting escapes every character in s that does not print,
// except newline, which the caller uses for layout. Backslashes and
// quotes stay as they are, so it suits text that is already JSON.
func EscapeNonPrinting(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && n == 1:
			b.WriteString(`�`) // a byte that is not UTF-8
		case r == '\n' || printable(r):
			b.WriteRune(r)
		default:
			escapeRune(&b, r)
		}
		i += n
	}
	return b.String()
}

// EscapeLabel is a name or title on one line: backslash and the
// non-printing characters escaped, quotes left alone.
func EscapeLabel(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && n == 1:
			b.WriteString(`�`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case printable(r):
			b.WriteRune(r)
		default:
			escapeRune(&b, r)
		}
		i += n
	}
	return b.String()
}
