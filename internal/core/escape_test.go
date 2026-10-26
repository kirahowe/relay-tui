package core

import (
	"strconv"
	"strings"
	"testing"
	"unicode"

	"pgregory.net/rapid"
)

// genText is arbitrary text, weighted towards what hides: controls,
// bidi marks, zero-width characters, separators, private use, tags,
// unassigned code points, astral characters, backslashes and quotes.
func genText(t *rapid.T, label string) string {
	piece := rapid.OneOf(
		rapid.String(),
		rapid.SampledFrom([]string{"\x00", "\x1b[2J", "\x07", "\u202e", "\u202a", "\u2066", "\u200b", "\u200d", "\ufeff",
			"\u00a0", "\u2028", "\u2029", "\u3000", "\ue000", "\U000e0001", "\U0001f600", "\u0378", "\\", `"`, `\u202e`, "\n", "\t", "\r", "\u00e9", "\u4e16\u754c", "\u0085", "\x7f"}),
	)
	var b strings.Builder
	for range rapid.IntRange(0, 8).Draw(t, label+"N") {
		b.WriteString(piece.Draw(t, label))
	}
	return b.String()
}

// nothing hidden is left: every rune is printable or a space or newline.
func allShown(s string) (rune, bool) {
	for _, r := range s {
		if r != '\n' && !unicode.IsPrint(r) {
			return r, false
		}
	}
	return 0, true
}

func TestEscapeKnown(t *testing.T) {
	for in, want := range map[string]string{
		"a\u202eb":                `a\u202eb`,
		"\U000e0001":              `\udb40\udc01`,
		"zero\u200bw":             `zero\u200bw`,
		"nbsp\u00a0x":             `nbsp\u00a0x`,
		"\u00e9\u4e16 \U0001f600": "\u00e9\u4e16 \U0001f600",
		"\x1b[2J":                 `\u001b[2J`,
	} {
		if got := EscapeNonPrinting(in); got != want {
			t.Errorf("EscapeNonPrinting(%q) = %s, want %s", in, got, want)
		}
	}
	if got := EscapeLabel("a\\b\u202e\n\"q\""); got != `a\\b\u202e\n"q"` {
		t.Errorf("EscapeLabel = %s", got)
	}
}

// unescapeLabel undoes EscapeLabel.
func unescapeLabel(s string) string {
	var b strings.Builder
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		if rs[i] != '\\' {
			b.WriteRune(rs[i])
			continue
		}
		i++
		switch rs[i] {
		case '\\':
			b.WriteRune('\\')
		case 'n':
			b.WriteRune('\n')
		case 'r':
			b.WriteRune('\r')
		case 't':
			b.WriteRune('\t')
		case 'u':
			n, _ := strconv.ParseUint(string(rs[i+1:i+5]), 16, 32)
			i += 4
			r := rune(n)
			if r >= 0xD800 && r < 0xDC00 { // the low half follows
				lo, _ := strconv.ParseUint(string(rs[i+3:i+7]), 16, 32)
				i += 6
				r = 0x10000 + (r-0xD800)<<10 + (rune(lo) - 0xDC00)
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

func TestEscapeLabelRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := genText(t, "label")
		e := EscapeLabel(s)
		if r, ok := allShown(e); !ok || strings.ContainsRune(e, '\n') {
			t.Fatalf("U+%04X or a newline is raw in %q", r, e)
		}
		if got := unescapeLabel(e); got != s {
			t.Fatalf("%q -> %q -> %q", s, e, got)
		}
	})
}

// Regression (Codex review): variation selectors and other code points
// that draw as nothing pass unicode.IsPrint, so a run of them could carry
// text the person never sees. They show escaped.
func TestInvisibleCodePointsAreEscaped(t *testing.T) {
	for _, r := range []rune{0xFE0F, 0xE0100, 0xE01EF, 0x180B, 0x034F, 0x115F, 0x3164, 0x2800, 0x1D159, 0xE0041} {
		if got := EscapeLabel("a" + string(r) + "b"); got == "a"+string(r)+"b" {
			t.Errorf("U+%04X shown as itself", r)
		}
		if got := EscapeNonPrinting(string(r)); !strings.HasPrefix(got, `\u`) {
			t.Errorf("U+%04X not escaped: %q", r, got)
		}
	}
	if EscapeLabel("café ünïcode") != "café ünïcode" {
		t.Error("ordinary letters escaped")
	}
}
