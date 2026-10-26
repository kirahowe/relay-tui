package render

import (
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"pgregory.net/rapid"

	"github.com/kirahowe/relay-tui/internal/core"
)

var (
	fromWeb = core.Item{
		Ref: "shown:2026-10-18/t1", Path: "shown/2026-10-18/t1.md", CallID: "t1",
		Title: "Concert venues near the hall",
		Text:  "Three venues are open late.\nDetails: https://example.org/venues\n\nA second paragraph with more words so that the text wraps in a narrow pane.",
		Sources: []core.Source{{Title: "Venue list", Ref: "web:https://example.org/venues"},
			{Title: "Hall site", Ref: "web:https://hall.example.com/about"}},
		Labels: core.Labels{Private: true, Contaminated: true},
	}
	ownItem = core.Item{
		Ref: "shown:2026-10-18/t0", Title: "Hall address", Text: "1 Main Street", Labels: core.Labels{Private: true},
	}
	fromMail = core.Item{
		Ref: "shown:2026-10-18/t2", CallID: "t2", Title: "A note from a stranger",
		Text:    "Please wire the deposit. See https://send.example.net/campaign",
		Sources: []core.Source{{Title: "Mail", Ref: "mail:stranger@example.com"}},
		Labels:  core.Labels{Contaminated: true},
	}
)

func paneState(focus bool, items ...core.Item) core.Single {
	s := core.Single{App: core.App{Live: true, PaneFocus: focus, Board: core.Board{Items: true}},
		State: core.State{Phase: core.Idle, Ready: true}}
	s.Pane = core.Pane{Relay: true, Server: items}
	s.Labels = core.Labels{Private: true, Contaminated: true}
	return s
}

func goldenLines(t *testing.T, lines []string) {
	t.Helper()
	requireGolden(t, strings.Join(lines, "\n"))
}

// The side pane leaves the transcript alone: what goes to scrollback is
// the same lines the frame would print without it, at the reduced width.
func TestSidePaneLeavesPrintAlone(t *testing.T) {
	s := paneState(false, fromWeb)
	for i := range 30 {
		s.Entries = append(s.Entries, core.Entry{Kind: core.Agent, ID: "m", Final: true, Text: strings.Repeat("word ", 5+i%7)})
	}
	with := View(s, "› ", "", t0, 140, 20)
	s.Pane = core.Pane{}
	without := View(s, "› ", "", t0, MainWidth(140), 20)
	if strings.Join(with.Print, "\n") != strings.Join(without.Print, "\n") || with.PrintTo != without.PrintTo {
		t.Errorf("print differs:\n%q\n%q", with.Print, without.Print)
	}
	for _, l := range with.Print {
		if strings.Contains(l, "│") || Width(l) > MainWidth(140) {
			t.Errorf("print line %q holds the pane or is too wide", l)
		}
	}
}

func TestNarrowOverlayPrintsNothing(t *testing.T) {
	s := paneState(true, fromWeb)
	for i := range 30 {
		s.Entries = append(s.Entries, core.Entry{Kind: core.Agent, ID: "m", Final: true, Text: strings.Repeat("word ", 5+i%7)})
	}
	if f := View(s, "› ", "", t0, 60, 20); len(f.Print) != 0 {
		t.Errorf("printed %q under the overlay", f.Print)
	}
}

func TestSideThreshold(t *testing.T) {
	if Side(SideMinWidth-1) || !Side(SideMinWidth) {
		t.Error("threshold")
	}
	for w := SideMinWidth; w < 400; w++ {
		if MainWidth(w)+gap+PaneWidth(w) != w || MainWidth(w) < 40 {
			t.Fatalf("width %d: main %d pane %d", w, MainWidth(w), PaneWidth(w))
		}
	}
}

// Characters that attack a terminal or a reader.
var nasty = []string{
	"plain", "", "\x1b[31mred\x1b[0m", "\x1b]8;;https://evil.test\x07click\x1b]8;;\x07", "bell\x07", "a\u202eb", "zero\u200bwidth",
	"\u2028line", "tab\there", "cr\rhere", "c1\u009b31m", "https://evil.test/x?d=1", "www.evil.test", "line\nbreak", "\u0000nul",
	"wide \u4e16\u754c \U0001f389", "[[fact:1]]", "ftp://h.test", "\ufeffbom", "\u00e9", "\x7f",
}

func paneText0(t *rapid.T, label string) string {
	return rapid.OneOf(rapid.SampledFrom(nasty), rapid.String()).Draw(t, label)
}

func genItem(t *rapid.T) core.Item {
	it := core.Item{
		Ref: paneText0(t, "ref"), Path: paneText0(t, "path"), CallID: paneText0(t, "call"),
		Title: paneText0(t, "title"), Text: paneText0(t, "text"),
		Labels: core.Labels{Private: rapid.Bool().Draw(t, "p"), Contaminated: rapid.Bool().Draw(t, "c")},
	}
	for range rapid.IntRange(0, 3).Draw(t, "sources") {
		it.Sources = append(it.Sources, core.Source{Title: paneText0(t, "stitle"), Ref: paneText0(t, "sref")})
	}
	return it
}

func genPaneState(t *rapid.T) core.Single {
	var items []core.Item
	for range rapid.IntRange(0, 4).Draw(t, "items") {
		items = append(items, genItem(t))
	}
	s := paneState(rapid.Bool().Draw(t, "focus"), items...)
	s.Labels = core.Labels{Contaminated: rapid.Bool().Draw(t, "session")}
	s.Pane.Relay = rapid.Bool().Draw(t, "relay")
	s.Pane.Local = items
	if len(items) > 0 {
		s.Pane.Sel = rapid.IntRange(0, len(items)).Draw(t, "sel")
	}
	s.Pane.Detail = rapid.Bool().Draw(t, "detail")
	s.Pane.Scroll = rapid.IntRange(0, 30).Draw(t, "scroll")
	refs := ""
	for range rapid.IntRange(0, 3).Draw(t, "refs") {
		ref := "[[" + rapid.SampledFrom([]string{"fact", "doc", "web"}).Draw(t, "kind") + ":" + rapid.StringMatching(`[a-z0-9:/.%\x00-\x1f]{1,12}`).Draw(t, "id") + "]]"
		refs += ref + " "
		if rapid.Bool().Draw(t, "resolved") {
			s.Pane.Cites = append(s.Pane.Cites, core.Citation{Ref: ref, Status: rapid.SampledFrom([]string{"", "ok", "missing", "external"}).Draw(t, "status"),
				Title: paneText0(t, "ctitle"), Target: paneText0(t, "target"), Labels: core.Labels{Contaminated: rapid.Bool().Draw(t, "cc")}})
		}
	}
	s.Entries = []core.Entry{{Kind: core.Agent, ID: "m", Final: true, Text: refs}}
	return s
}

// unprintable says whether r must never reach the terminal as itself.
func unprintable(r rune) bool {
	return r == 0x1b || unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) ||
		unicode.Is(unicode.Co, r) || r == 0xFFFD
}

// Whatever relay (or a model) puts in an item, the pane draws exactly the
// room it was given, no wider than the width, with no character that could
// move the cursor, start a link or reorder text.
// The items view says how many cards the board holds and how to reach
// it, so a card relay adds is never out of sight without a word.
func TestPaneNamesTheBoard(t *testing.T) {
	s := paneState(false, ownItem)
	if text := ansi.Strip(strings.Join(PaneLines(s.App, s.State, t0, 44, 12), "\n")); strings.Contains(text, "board") {
		t.Fatalf("named a board that has not loaded:\n%s", text)
	}
	s.Board = core.Board{Items: true, Loaded: true, Cards: []core.Card{{ID: 1}, {ID: 2}}}
	lines := PaneLines(s.App, s.State, t0, 44, 12)
	if head := ansi.Strip(lines[0]); !strings.HasPrefix(head, "Workspace 1") || !strings.HasSuffix(head, "b  board 2") {
		t.Fatalf("header %q", head)
	}
	if text := ansi.Strip(strings.Join(lines, "\n")); !strings.Contains(text, "l to focus, then b for the board") {
		t.Fatalf("no way to the board:\n%s", text)
	}
	// Too narrow for both, the header keeps its own view.
	if head := ansi.Strip(PaneLines(s.App, s.State, t0, 16, 12)[0]); strings.Contains(head, "board") || !strings.HasPrefix(head, "Workspace") {
		t.Fatalf("narrow header %q", head)
	}
	s.Board.Unavailable = true
	if head := ansi.Strip(PaneLines(s.App, s.State, t0, 44, 12)[0]); strings.Contains(head, "board") {
		t.Fatalf("named a board the agent does not have: %q", head)
	}
}

func TestPaneNeverEmitsRawControls(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := genPaneState(t)
		w := rapid.IntRange(1, 70).Draw(t, "width")
		h := rapid.IntRange(0, 40).Draw(t, "height")
		now := t0.Add(time.Duration(rapid.IntRange(0, 2000).Draw(t, "ms")) * time.Millisecond)
		lines := PaneLines(s.App, s.State, now, w, h)
		if len(lines) != h {
			t.Fatalf("%d lines, want %d", len(lines), h)
		}
		for _, l := range lines {
			if strings.Contains(l, "\x1b]") {
				t.Fatalf("operating system command in %q", l)
			}
			plain := ansi.Strip(l)
			if Width(l) > w {
				t.Fatalf("line %q is %d wide, limit %d", l, Width(l), w)
			}
			for _, r := range plain {
				if unprintable(r) && r != 0xFFFD {
					t.Fatalf("raw %U in %q (plain %q)", r, l, plain)
				}
			}
			if strings.ContainsAny(plain, "\r\n") {
				t.Fatalf("line break inside a line: %q", plain)
			}
		}
		// And in a whole frame, beside the transcript or over it.
		fw := rapid.SampledFrom([]int{50, 80, 120, 160}).Draw(t, "frameWidth")
		f := View(s, "› ", "hint", now, fw, 24)
		for _, l := range strings.Split(f.Content, "\n") {
			if Width(l) > fw {
				t.Fatalf("frame row %q is %d wide, limit %d", l, Width(l), fw)
			}
			for _, r := range ansi.Strip(l) {
				if r == 0x1b || (unprintable(r) && r != 0xFFFD) {
					t.Fatalf("raw %U in frame row %q", r, l)
				}
			}
		}
	})
}

func TestPaneShowsMissingInReverse(t *testing.T) {
	s := paneState(false)
	s.Entries = []core.Entry{{Kind: core.Agent, ID: "m", Final: true, Text: "[[doc:gone]] [[fact:1]]"}}
	s.Pane.Cites = []core.Citation{{Ref: "[[doc:gone]]", Status: core.CiteMissing}}
	out := strings.Join(PaneLines(s.App, s.State, t0, 50, 12), "\n")
	if !strings.Contains(out, sgr(reverse)+" missing ") {
		t.Errorf("missing is not in reverse video:\n%q", out)
	}
	if !strings.Contains(ansi.Strip(out), "unchecked [[fact:1]]") {
		t.Errorf("an unresolved reference is not marked unchecked:\n%s", ansi.Strip(out))
	}
}

func TestDetailMaxMatchesWhatIsDrawn(t *testing.T) {
	s := paneState(true, fromWeb)
	s.Pane.Detail = true
	_, mx := paneRender(s.App, s.State, t0, 30, 10)
	if mx <= 0 {
		t.Fatal("no scroll for a long text in a short pane")
	}
	s.Pane.Scroll = mx
	last := ansi.Strip(strings.Join(paneBodyTail(s), "\n"))
	if !strings.Contains(last, "hall") {
		t.Errorf("the last scroll position does not reach the end:\n%s", last)
	}
	s.Pane.Scroll = mx + 50
	a, _ := paneRender(s.App, s.State, t0, 30, 10)
	s.Pane.Scroll = mx
	b, _ := paneRender(s.App, s.State, t0, 30, 10)
	if strings.Join(a, "\n") != strings.Join(b, "\n") {
		t.Error("scrolling past the end changes the picture")
	}
}

// paneBodyTail is the pane at its furthest scroll, 30 wide and 10 tall.
func paneBodyTail(s core.Single) []string {
	l, _ := paneRender(s.App, s.State, t0, 30, 10)
	return l
}

func TestPaneGolden(t *testing.T) {
	list := paneState(true, fromMail, fromWeb, ownItem)
	list.Pane.Sel, list.Pane.SelKey = 1, fromWeb.Key()
	list.Entries = []core.Entry{{Kind: core.Agent, ID: "m", Final: true, Text: "The hall seats 900 [[fact:12]], see [[doc:gone]], [[web:https://x.test/a]] and [[note:later]]."}}
	list.Pane.Cites = []core.Citation{
		{Ref: "[[fact:12]]", Kind: "fact", Status: core.CiteOK, Title: "Fact 12 [venues]"},
		{Ref: "[[doc:gone]]", Kind: "doc", Status: core.CiteMissing},
		{Ref: "[[web:https://x.test/a]]", Kind: "web", Status: core.CiteExternal, Target: "https://x.test/a"},
		{Ref: "[[note:later]]"},
	}

	t.Run("side", func(t *testing.T) {
		list.PaneFocus = false
		f := View(list, "› ", "ctrl+j for a new line", t0, 130, 24)
		goldenLines(t, strings.Split(f.Content, "\n"))
	})
	list.PaneFocus = true
	t.Run("side_focused", func(t *testing.T) {
		f := View(list, "› ", "ctrl+j for a new line", t0, 130, 24)
		goldenLines(t, strings.Split(f.Content, "\n"))
	})
	t.Run("overlay", func(t *testing.T) {
		f := View(list, "› ", "ctrl+j for a new line", t0, 60, 24)
		goldenLines(t, strings.Split(f.Content, "\n"))
	})
	t.Run("overlay_closed_hint", func(t *testing.T) {
		closed := list
		closed.PaneFocus = false
		f := View(closed, "› ", "ctrl+j for a new line", t0, 60, 10)
		goldenLines(t, strings.Split(f.Content, "\n"))
	})
	t.Run("detail_from_the_web", func(t *testing.T) {
		d := list
		d.Pane.Detail = true
		goldenLines(t, PaneLines(d.App, d.State, t0, 44, 30))
	})
	t.Run("detail_from_the_web_scrolled", func(t *testing.T) {
		d := list
		d.Pane.Detail, d.Pane.Scroll = true, 5
		goldenLines(t, PaneLines(d.App, d.State, t0, 44, 12))
	})
	t.Run("detail_from_outside", func(t *testing.T) {
		d := list
		d.Pane.Detail, d.Pane.Sel, d.Pane.SelKey = true, 0, fromMail.Key()
		goldenLines(t, PaneLines(d.App, d.State, t0, 44, 24))
	})
}

// Text from a contaminated item says where it came from, in plain words,
// with no live link, wherever it is drawn, and never uses a label's name.
func TestContaminatedItemsSayWhereTheyCameFrom(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		it := genItem(t)
		it.Labels.Contaminated = true
		it.Title = rapid.SampledFrom([]string{"Title", "see https://evil.test/a?d=1", "www.evil.test x"}).Draw(t, "title")
		it.Text = "go to " + rapid.SampledFrom([]string{"https://evil.test/x", "ftp://evil.test", "http://a.b.c/d", "www.e.test",
			"https://a.test/?next=www.evil.test/?d=SECRET", "https://a.test/r?u=https://evil.test/x"}).Draw(t, "url") + " now"
		web := rapid.Bool().Draw(t, "web")
		it.Sources = []core.Source{{Title: "s", Ref: "doc:notes/a.md"}}
		want := "from outside"
		if web {
			it.Sources = []core.Source{{Title: "s", Ref: "web:https://evil.test/src"}}
			want = "from the web"
		}
		s := paneState(true, it)
		s.Entries = nil
		s.Pane.Detail = rapid.Bool().Draw(t, "detail")
		out := strings.Join(PaneLines(s.App, s.State, t0, 60, 60), "\n")
		plain := ansi.Strip(out)
		if !strings.Contains(plain, want) {
			t.Fatalf("no %q (detail %v):\n%s", want, s.Pane.Detail, plain)
		}
		for _, l := range strings.Split(plain, "\n") {
			if live(l) {
				t.Fatalf("live link in %q", l)
			}
		}
		for _, w := range []string{"contaminated", "untrusted", "trusted"} {
			if strings.Contains(plain, w) {
				t.Fatalf("%q on the pane:\n%s", w, plain)
			}
		}
	})
}

// A payload hidden in variation selectors shows escaped in an item's
// detail, so what Kira reads is what is there.
func TestHiddenPayloadShowsInDetail(t *testing.T) {
	hidden := "Venues" + string(rune(0xE0100)) + string(rune(0xE0101)) + string(rune(0xFE0F))
	it := core.Item{Title: "t", Text: hidden, Path: "shown/2026-10-18/x.md", Labels: core.Labels{Contaminated: true}}
	s := paneState(true, it)
	s.Pane.Detail = true
	out := ansi.Strip(strings.Join(PaneLines(s.App, s.State, t0, 60, 60), "\n"))
	if !strings.Contains(out, `Venues\udb40\udd00\udb40\udd01\ufe0f`) || strings.ContainsRune(out, 0xE0100) || strings.ContainsRune(out, 0xFE0F) {
		t.Errorf("hidden code points not escaped:\n%s", out)
	}
}
