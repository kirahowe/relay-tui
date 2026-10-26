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
// space without being one: the braille blank, the musical null notehead
// and the Khitan filler.
func blankGlyph(r rune) bool {
	switch r {
	case 0x2800, 0x1D159, 0x16FE4:
		return true
	}
	for _, rg := range defaultIgnorable {
		if r >= rg[0] && r <= rg[1] {
			return true
		}
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
