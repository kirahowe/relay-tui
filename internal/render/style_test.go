package render

import (
	"image/color"
	"testing"

	"pgregory.net/rapid"
)

func drawStyle(t *rapid.T) style {
	st := style(rapid.IntRange(0, 63).Draw(t, "attrs"))
	st |= fg(hue(rapid.IntRange(int(hueNone), int(hueCyan)).Draw(t, "fg")))
	if rapid.Bool().Draw(t, "tinted") {
		st |= bg(rapid.SampledFrom([]hue{tintSel, tintChip, tintPill}).Draw(t, "bg"))
	}
	return st
}

// With the terminal's colours known, every style comes back from the
// sequence that switches to it, so selfContained reopens it exactly.
func TestStyleRoundTrips(t *testing.T) {
	SetTerminalColors(color.RGBA{0x1d, 0x1f, 0x23, 0xff}, color.RGBA{0xc8, 0xcc, 0xd4, 0xff})
	defer SetTerminalColors(nil, nil)
	rapid.Check(t, func(t *rapid.T) {
		st := drawStyle(t)
		if got := parseSGR(sgr(st)); got != st {
			t.Fatalf("%q parsed to %#x, want %#x", sgr(st), got, st)
		}
	})
}

// Without them, a selection is reverse video and the other tints are
// left out; the rest of the style is kept.
func TestTintsWithoutTheTerminalsColours(t *testing.T) {
	SetTerminalColors(nil, nil)
	rapid.Check(t, func(t *rapid.T) {
		st := drawStyle(t)
		want := st &^ (0xff << bgShift)
		if st.bg() == tintSel {
			want |= reverse
		}
		if got := parseSGR(sgr(st)); got != want {
			t.Fatalf("%q parsed to %#x, want %#x", sgr(st), got, want)
		}
	})
}

// The tints are distinct from the background and from one another, on a
// dark theme and a light one.
func TestTintsAreDistinct(t *testing.T) {
	defer SetTerminalColors(nil, nil)
	for _, pair := range [][2]color.RGBA{
		{{0x1d, 0x1f, 0x23, 0xff}, {0xc8, 0xcc, 0xd4, 0xff}},
		{{0xfb, 0xfb, 0xfa, 0xff}, {0x2b, 0x2e, 0x34, 0xff}},
	} {
		SetTerminalColors(pair[0], pair[1])
		seen := map[rgb]bool{toRGB(pair[0]): true}
		for _, h := range []hue{tintSel, tintChip, tintPill} {
			c, ok := tintOf(h)
			if !ok || seen[c] {
				t.Fatalf("tint %d is %v, not distinct", h, c)
			}
			seen[c] = true
		}
	}
}
