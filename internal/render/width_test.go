package render

import (
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"pgregory.net/rapid"

	"github.com/kirahowe/relay-tui/internal/core"
)

// Codex round 1, finding 1: Bubble Tea measures by code point unless the
// terminal negotiates grapheme widths, and a terminal may do either. A
// review's text, drawn into a screen buffer measuring either way, shows
// exactly the bytes the frame says it drew: every body row of the buffer
// is the next line of T from From, to To. A grapheme the two methods
// measure differently (an emoji with a skin tone, a ZWJ family) makes
// the review draw none.
func TestTheBufferShowsWhatTheReviewCertifies(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		text := core.Visible(pagerText(t)) + strings.Repeat("👍🏽", rapid.IntRange(0, 12).Draw(t, "thumbs")) + "DELETE_PRIVATE_FILES"
		x := reviewing(reviewOf(text))
		width := rapid.IntRange(8, 60).Draw(t, "width")
		height := rapid.IntRange(6, 24).Draw(t, "height")
		f := Review(core.SingleSurface(x), x.App, x.State, "", t0, width, height)
		if f.Drawn == nil {
			if _, amb := AmbiguousWidth(text); !amb {
				if _, fits := PagerLines(text, width-len(Margin)); fits {
					t.Fatalf("drew nothing at %dx%d:\n%s", width, height, f.Content)
				}
			}
			return
		}
		lines, _ := PagerLines(text, width-len(Margin))
		for _, m := range []ansi.Method{ansi.WcWidth, ansi.GraphemeWidth} {
			buf := uv.NewScreenBuffer(width, height)
			buf.Method = m
			uv.NewStyledString(f.Content).Draw(buf, buf.Bounds())
			off, i := 0, 0
			for off < f.Drawn.From {
				off += len(lines[i])
				i++
			}
			for y := 1; off < f.Drawn.To; y++ {
				want := bodyRow(lines, i)
				if got := strings.TrimRight(buf.Line(y).String(), " "); got != strings.TrimRight(want, " ") {
					t.Fatalf("method %v, row %d shows %q, the frame certifies %q", m, y, got, want)
				}
				off += len(lines[i])
				i++
			}
		}
	})
}
