package render

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"pgregory.net/rapid"

	"github.com/kirahowe/relay-tui/internal/core"
)

// pagerPieces is what random review texts are made of: wide and narrow,
// combining marks, emoji with skin tones and ZWJ sequences, flags, long
// words, spaces, empty lines.
var pagerPieces = []string{
	"a", "word ", "longwordwithoutanybreakatallthatgoesonandon", " ", "  ", "\n", "\n\n", "漢字", "かな ", "😀", "👍🏽",
	"👩‍👩‍👧", "🇨🇦", "e\u0301", "a\u0308\u0323", "\u0915\u094d\u0937", "é", "x", "\\", "\"", "١٢٣", "ﷺ", "—", "…",
}

func pagerText(t *rapid.T) string {
	parts := rapid.SliceOfN(rapid.SampledFrom(pagerPieces), 0, 60).Draw(t, "parts")
	return strings.Join(parts, "")
}

// The pager's lines, joined, are the text exactly, for any text and
// width; each line is within the width; a newline ends a line or nothing;
// and no grapheme is split. A text with a grapheme wider than the width
// says so (ok false).
func TestPagerLinesAreTheTextExactly(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		text := pagerText(t)
		width := rapid.IntRange(1, 50).Draw(t, "width")
		lines, ok := PagerLines(text, width)
		if got := strings.Join(lines, ""); got != text {
			t.Fatalf("joined %q, want %q", got, text)
		}
		if len(lines) == 0 {
			t.Fatal("no lines")
		}
		wide := false
		for rest := text; rest != ""; {
			g, w := ansi.FirstGraphemeCluster(rest, ansi.GraphemeWidth)
			wide = wide || w > width
			rest = rest[len(g):]
		}
		if ok == wide {
			t.Fatalf("ok %v with a grapheme wider than %d: %v", ok, width, wide)
		}
		bounds := map[int]bool{len(text): true}
		for at := 0; at < len(text); {
			bounds[at] = true
			g, _ := ansi.FirstGraphemeCluster(text[at:], ansi.GraphemeWidth)
			at += len(g)
		}
		at := 0
		for i, l := range lines {
			body := strings.TrimSuffix(l, "\n")
			if strings.Contains(body, "\n") || (i < len(lines)-1 && l == "") {
				t.Fatalf("line %d %q", i, l)
			}
			if ok && ansi.StringWidth(body) > width {
				t.Fatalf("line %d %q is %d wide at %d", i, body, ansi.StringWidth(body), width)
			}
			if at += len(l); !bounds[at] {
				t.Fatalf("line %d %q ends inside a grapheme", i, l)
			}
		}
	})
}

// reviewOf is relay's review of text t.
func reviewOf(t string) *core.Review { return &core.Review{Text: t, SHA256: core.Digest(t)} }

// reviewing is a single agent's session with a request on offer whose
// review is open, its text rv.
func reviewing(rv *core.Review) core.Single {
	s, _ := core.Reduce(core.State{}, core.Connected{At: t0})
	s, _ = core.Reduce(s, core.Submit{At: t0, Text: "x"})
	s, _ = core.Reduce(s, core.PermissionAsked{At: t0, Permission: core.Permission{ID: 1, CallID: "c", Name: "send_mail", Review: rv,
		Options: []core.PermOption{{ID: "a", Name: "Allow once", Kind: core.AllowOnce}, {ID: "r", Name: "Reject", Kind: core.RejectOnce}}}})
	s, _ = core.Reduce(s, core.OpenReview{At: t0})
	x, _ := core.ReduceSingle(core.Single{App: core.App{Moving: true}, State: s}, core.Tick{At: t0})
	return x
}

// Paging through the review draws every part of the text, and what each
// frame reports drawn is exactly the text its body shows: the body rows,
// joined with the newlines they end on, are bytes From to To of the
// text. At any size, a frame either reports what it drew or draws none.
func TestReviewFramesReportWhatTheyShow(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		text := core.Visible(pagerText(t)) // relay's text is visible already
		x := reviewing(reviewOf(text))
		width := rapid.IntRange(1, 60).Draw(t, "width")
		height := rapid.IntRange(1, 30).Draw(t, "height")
		lines, ok := PagerLines(text, width)
		seen := make([]bool, len(text))
		for range len(lines) + 2 {
			f := Review(core.SingleSurface(x), x.App, x.State, "", t0, width, height)
			rows := strings.Split(f.Content, "\n")
			if len(rows) != height {
				t.Fatalf("%d rows at height %d", len(rows), height)
			}
			for _, r := range rows {
				if w := ansi.StringWidth(r); w > width {
					t.Fatalf("row %q is %d wide at %d", r, w, width)
				}
			}
			if f.Drawn == nil {
				if ok && height >= 6 {
					t.Fatalf("drew nothing at %dx%d:\n%s", width, height, f.Content)
				}
				return
			}
			if !ok {
				t.Fatal("drew a text with a grapheme wider than the window")
			}
			d := *f.Drawn
			// The body is the rows after the heading: each the next line
			// of the text from From, until To.
			off, i := 0, 0
			for off < d.From {
				off += len(lines[i])
				i++
			}
			if off != d.From {
				t.Fatalf("From %d is not where a line starts", d.From)
			}
			for j, r := range rows[1 : 1+f.Page] {
				if off >= d.To {
					break
				}
				if r != strings.TrimSuffix(lines[i+j], "\n") {
					t.Fatalf("row %q is not line %q", r, lines[i+j])
				}
				off += len(lines[i+j])
			}
			if off != d.To {
				t.Fatalf("the rows end at %d, the frame reports %d", off, d.To)
			}
			for i := d.From; i < d.To; i++ {
				seen[i] = true
			}
			x, _ = core.ReduceSingle(x, core.ReviewKey{At: t0, Action: core.ReviewPageDown, Max: f.Max, Page: f.Page})
		}
		for i, s := range seen {
			if !s {
				t.Fatalf("byte %d of %q never drawn paging through at %dx%d", i, text, width, height)
			}
		}
	})
}

// The review fails closed: a text with a character that does not show as
// itself, a digest that does not match, or a grapheme wider than the
// window is not drawn, and the frame says why and reports nothing.
func TestReviewFailsClosed(t *testing.T) {
	for name, c := range map[string]struct {
		rv    *core.Review
		width int
		says  string
	}{
		"a control":    {reviewOf("send\x1b[2J now"), 60, "does not show as itself"},
		"a bidi mark":  {reviewOf("pay ‮ evil"), 60, "does not show as itself"},
		"a tab":        {reviewOf("a\tb"), 60, "does not show as itself"},
		"nbsp":         {reviewOf("a\u00a0b"), 60, "does not show as itself"},
		"wrong digest": {&core.Review{Text: "ok", SHA256: core.Digest("not ok")}, 60, "digest does not match"},
		"too narrow":   {reviewOf("漢字"), 1, "too narrow"},
		"none":         {nil, 60, ""},
	} {
		t.Run(name, func(t *testing.T) {
			x := reviewing(c.rv)
			if c.rv == nil {
				// A plain agent's request: the text is built here, and shows.
				if f := Review(core.SingleSurface(x), x.App, x.State, "", t0, c.width, 20); f.Drawn == nil {
					t.Fatalf("a built text was not drawn:\n%s", f.Content)
				}
				return
			}
			f := Review(core.SingleSurface(x), x.App, x.State, "", t0, c.width, 60)
			if f.Drawn != nil {
				t.Fatalf("drew %+v", f.Drawn)
			}
			plain := ansi.Strip(f.Content)
			if !strings.Contains(strings.Join(strings.Fields(plain), ""), strings.ReplaceAll(c.says, " ", "")) {
				t.Fatalf("does not say %q:\n%s", c.says, plain)
			}
			if strings.ContainsAny(plain, "\x1b\t\u202e\u00a0") {
				t.Fatalf("the text got through:\n%q", plain)
			}
		})
	}
}

// The heading names the subject; the keys say how much is left and when
// y is armed.
func TestReviewFooter(t *testing.T) {
	var b strings.Builder
	for i := range 40 {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	x := reviewing(reviewOf(b.String()))
	f := Review(core.SingleSurface(x), x.App, x.State, "", t0, 60, 13)
	plain := ansi.Strip(f.Content)
	for _, want := range []string{"Allow this tool call?", "page 1 of 5", "4 more below", "y allow once all is read", "q back"} {
		if !strings.Contains(plain, want) {
			t.Errorf("%q missing:\n%s", want, plain)
		}
	}
	for range 5 {
		x, _ = core.ReduceSingle(x, core.Drawn{At: t0, Surfaces: []string{f.Drawn.ID}, Review: f.Drawn})
		x, _ = core.ReduceSingle(x, core.ReviewKey{At: t0, Action: core.ReviewPageDown, Max: f.Max, Page: f.Page})
		f = Review(core.SingleSurface(x), x.App, x.State, "", t0, 60, 13)
	}
	x, _ = core.ReduceSingle(x, core.Drawn{At: t0, Surfaces: []string{f.Drawn.ID}, Review: f.Drawn})
	plain = ansi.Strip(Review(core.SingleSurface(x), x.App, x.State, "", t0.Add(time.Millisecond), 60, 13).Content)
	if !strings.Contains(plain, "page 5 of 5") || !strings.Contains(plain, "y allow (a moment)") {
		t.Errorf("at the end, before the delay:\n%s", plain)
	}
	plain = ansi.Strip(Review(core.SingleSurface(x), x.App, x.State, "", t0.Add(core.ArmDelay), 60, 13).Content)
	if !strings.Contains(plain, "y allow ·") || strings.Contains(plain, "once all is read") {
		t.Errorf("armed:\n%s", plain)
	}
}
