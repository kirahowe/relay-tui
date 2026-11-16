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
// The Brahmi number joiner and the Duployan thick letter selector are
// marks that draw as nothing by design and are in neither Unicode's
// default-ignorable set nor relay's list of it, so relay names them in
// renders-blank (relay#130).
func TestInvisibleCodePointsAreEscaped(t *testing.T) {
	for _, r := range []rune{0xFE0F, 0xE0100, 0xE01EF, 0x180B, 0x034F, 0x115F, 0x3164, 0x2800, 0x1D159, 0xE0041, 0x1107F, 0x1BC9D} {
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

// DropHidden takes out what nobody sees, so that the pieces either side
// come together, and leaves the rest: a mark that shows on its letter,
// and the joiner or selector that holds an emoji together.
func TestDropHidden(t *testing.T) {
	for in, want := range map[string]string{
		"https:\u200d//evil.test":          "https://evil.test",
		"https:/\ufe0f/evil.test":          "https://evil.test",
		"www\u200d.evil.test":              "www.evil.test",
		"ww\u034fw.ev\u180bil.te\u17b5st":  "www.evil.test",
		"h\U000E0100ttps\u1160://x\u2060y": "https://xy",
		"a\u00adb\u200bc\u200ed\ufeffe":    "abcde",
		"a\u2800b\x1bc\ue000d":             "abcd",
		"e\u0301 stays":                    "e\u0301 stays",
		// Two marks invisible by design, which relay.text/hidden? takes too.
		"https:\U0001107F//evil.test": "https://evil.test",
		"www\U0001BC9D.evil.test":     "www.evil.test",
		"plain text\n\twith a tab":    "plain text\n\twith a tab",
		// A code point with no character assigned, and a noncharacter.
		"https:\u0378//evil.test": "https://evil.test",
		"www\uffff.evil.test":     "www.evil.test",
		// One of the emoji blocks that has no name yet stays, and so does
		// a carriage return.
		"new \U0001FFFD emoji": "new \U0001FFFD emoji",
		"a\r\nb":               "a\r\nb",
		"Done \U0001F469\u200d\U0001F4BB and ⚠\ufe0f": "Done \U0001F469\u200d\U0001F4BB and ⚠\ufe0f",
		"": "",
	} {
		if got := DropHidden(in); got != want {
			t.Errorf("DropHidden(%q) = %q, want %q", in, got, want)
		}
	}
}

// relay's own cases for what holds an emoji together and what does not
// (relay's test/relay/sanitize_test.clj, family-emoji-and-keycaps-survive),
// so relay and this client cannot come to differ without a test failing.
// man is U+1F468, woman U+1F469, girl U+1F467, boy U+1F466.
func TestDropHiddenKeepsEmojiGlueAsRelayDoes(t *testing.T) {
	const man, woman, girl, boy = "\U0001F468", "\U0001F469", "\U0001F467", "\U0001F466"
	for _, e := range []string{
		man + "\u200d" + woman + "\u200d" + girl + "\u200d" + boy, // a family
		"❤\ufe0f",                            // a red heart
		"1\ufe0f\u20e3",                      // a keycap
		"\U0001F3F3\ufe0f\u200d\U0001F308",   // a rainbow flag
		woman + "\U0001F3FD\u200d\U0001F4BB", // a technologist with a skin tone
	} {
		if got := DropHidden(e); got != e {
			t.Errorf("DropHidden(%q) = %q", e, got)
		}
		if got := DropHidden("our family " + e); got != "our family "+e {
			t.Errorf("in a sentence: %q", got)
		}
	}
	// Outside an emoji sequence they are still removed.
	for in, want := range map[string]string{
		man + "\u200da":              man + "a", // a joiner after an emoji, with no emoji after it
		man + "\u200d" + "1":         man + "1",
		"1\ufe0f":                    "1", // a selector after a digit with no keycap
		"#\ufe0fx":                   "#x",
		"a\u200db":                   "ab",
		man + "\u200d":               man,
		"\u200d" + woman:             woman,
		"a\ufe0f":                    "a",
		"\ufe0f" + woman:             woman,
		man + "\u200d\u200d" + woman: man + "\u200d" + woman,
		man + "\u200b\u200d" + woman: man + "\u200d" + woman,
		man + "\u2028\u200d" + woman: man + "\n" + woman,
		man + "\u0085\u200d" + woman: man + "\n" + woman,
		man + "\n\u200d" + woman:     man + "\n" + woman,
		"A\u200dB":                   "AB",
		"A\ufe0f":                    "A",
		"\U0001F130\u200d\U0001F131": "\U0001F130\U0001F131", // squared Latin letters
		"Ⓐ\ufe0f":                    "Ⓐ",                    // a circled Latin letter
	} {
		if got := DropHidden(in); got != want {
			t.Errorf("DropHidden(%q) = %q, want %q", in, got, want)
		}
	}
}

// Every range of the list of emoji is in use: one emoji from each keeps
// the selector after it, and its neighbours outside the list do not. The
// code points are written out here, apart from the list under test.
func TestEveryRangeOfEmojiKeepsItsSelector(t *testing.T) {
	for _, r := range []rune{0x00A9, 0x00AE, 0x203C, 0x2049, 0x2122, 0x2139, 0x2195, 0x21A9, 0x231A, 0x2328, 0x2388, 0x23CF,
		0x23EA, 0x23F8, 0x24C2, 0x25AA, 0x25B6, 0x25C0, 0x25FB, 0x2600, 0x2764, 0x27BF, 0x2934, 0x2B05, 0x2B1B, 0x2B50,
		0x2B55, 0x3030, 0x303D, 0x3297, 0x3299, 0x1F004, 0x1F10D, 0x1F12F, 0x1F170, 0x1F17E, 0x1F18E, 0x1F191,
		0x1F1AD, 0x1F201, 0x1F600, 0x1FAE0, 0x1FC00} {
		in := string(r) + "\ufe0f"
		if got := DropHidden(in); got != in {
			t.Errorf("%U lost its selector: %q", r, got)
		}
		if in := string(r) + "\u200d" + string(r); DropHidden(in) != in {
			t.Errorf("%U lost its joiner", r)
		}
	}
	for _, r := range []rune{'A', '9', 0x00AA, 0x2123, 0x2193, 0x24B6, 0x25FF, 0x27C0, 0x1F130, 0x1F1E6, 0x1F200, 0x20000} {
		if got := DropHidden(string(r) + "\ufe0f"); got != string(r) {
			t.Errorf("%U kept a selector: %q", r, got)
		}
	}
}

// What is left holds nothing hidden but emoji glue, judged by Go's own
// Unicode tables and not the lists this package keeps, and it is the
// same text when gone over again.
func TestDropHiddenLeavesNothingHiddenButGlue(t *testing.T) {
	unseen := func(r rune) bool {
		return unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Other_Default_Ignorable_Code_Point, r) || unicode.Is(unicode.Variation_Selector, r) ||
			unicode.Is(unicode.Cc, r) && r != '\n' && r != '\t' || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r)
	}
	emoji := func(r rune) bool { return r >= 0x1F300 || r == 0x2764 || r == 0x26A0 }
	piece := rapid.OneOf(rapid.StringMatching(`[a-z0-9:/.#*]{0,6}`), rapid.SampledFrom([]string{"\u200d", "\ufe0f", "\ufe0e", "\u034f", "\u180b", "\u17b5",
		"\u1160", "\U000E0100", "\u200b", "\u2060", "\u00ad", "\ufeff", "\u202e", "\u2028", "\u2029", "\u0085", "\x07", "\u20e3",
		"é", "世", "\U0001F469", "\U0001F468", "\U0001F4BB", "\U0001F3FD", "❤", "⚠", "\u0301", " ", "\n"}))
	rapid.Check(t, func(t *rapid.T) {
		in := strings.Join(rapid.SliceOfN(piece, 0, 14).Draw(t, "pieces"), "")
		got := DropHidden(in)
		if again := DropHidden(got); again != got {
			t.Fatalf("DropHidden(%q) = %q, and again %q", in, got, again)
		}
		rs := []rune(got)
		for i, r := range rs {
			if !unseen(r) {
				continue
			}
			// The only ones left stand right after an emoji, or after a
			// keycap's sign before the keycap.
			after := i > 0 && (emoji(rs[i-1]) || rs[i-1] == 0xFE0F || rs[i-1] == 0xFE0E ||
				(i+1 < len(rs) && rs[i+1] == 0x20E3 && strings.ContainsRune("0123456789#*", rs[i-1])))
			if (r != 0x200D && r != 0xFE0F && r != 0xFE0E) || !after {
				t.Fatalf("DropHidden(%q) = %q keeps %U at %d", in, got, r, i)
			}
		}
		// Nothing a reader sees is lost: the visible characters are the
		// input's, in order, with a separator read as a line break.
		visible := func(s string) string {
			return strings.Map(func(r rune) rune {
				switch {
				case r == 0x85 || r == 0x2028 || r == 0x2029:
					return '\n'
				case unseen(r):
					return -1
				}
				return r
			}, s)
		}
		if visible(got) != visible(in) {
			t.Fatalf("DropHidden(%q) = %q changes what is seen", in, got)
		}
	})
}
