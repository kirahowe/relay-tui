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

func readyFixtureItems() []core.ReadyItem {
	return []core.ReadyItem{
		{ID: "card:7", Kind: "finding", At: t0.Add(-1 * time.Minute).Format(time.RFC3339), Title: "Venues to call",
			Lines: []string{"0 of 2 done"}, Card: 7},
		{ID: "event:6", Kind: "event", At: t0.Add(-2 * time.Minute).Format(time.RFC3339), Title: "Choir concert",
			Lines: []string{"Town hall"}, Start: "2026-11-21T00:00:00Z", End: "2026-11-21T01:00:00Z"},
		{ID: "prepared:8", Kind: "draft", At: t0.Add(-30 * time.Minute).Format(time.RFC3339), Title: "Reply to the booker",
			Lines: []string{"Saved draft c2, version 1, to the group Subscribers.", "Hello, thank you for the offer of the hall on the ninth."},
			From:  "from a mail by booker@example.org", Outsider: true},
		{ID: "message:4", Kind: "message", At: t0.Add(-3 * time.Hour).Format(time.RFC3339), Title: "Concert email is drafted",
			Lines: []string{"It is in your drafts, ready to read."}, From: "", Message: 4, ReplyTo: 4, Urgency: "normal",
			Replies: []core.ReadyReply{{At: t0.Format(time.RFC3339), Text: "Shorter please."}}},
		{ID: "message:3", Kind: "message", At: t0.Add(-26 * time.Hour).Format(time.RFC3339), Title: "The budget is at 80 percent",
			Lines: []string{"Your weekly budget."}, Message: 3, Urgency: "high"},
	}
}

func readyHubFixture() core.Hub {
	return hubStep(hubFixture(),
		core.ReadyLoaded{At: t0, Rev: 9, Items: readyFixtureItems()},
		core.StopTyping{At: t0}, core.ReadyOpen{At: t0})
}

func TestReadyGolden(t *testing.T) {
	h := readyHubFixture()
	moved := hubStep(h, core.ReadyKey{At: t0, Action: core.ReadyBottom})
	replying := hubStep(moved, core.ReadyKey{At: t0, Action: core.ReadyUp}, core.ReadyKey{At: t0, Action: core.ReadyReplyKey},
		core.ReadyKey{At: t0, Action: core.ReadyChar, Text: "Thank you"})
	empty := hubStep(core.NewHub(""), core.HubConnected{At: t0}, core.ReadyLoaded{At: t0, Rev: 1, Items: nil}, core.ReadyOpen{At: t0})
	move := hubStep(hubFixture(), core.ReadyLoaded{At: t0, Rev: 9, Items: []core.ReadyItem{
		{ID: "move:12", Kind: "move", At: t0.Add(-5 * time.Minute).Format(time.RFC3339), Seq: 12, Title: "2 things",
			Lines: []string{"By steward.", "/Users/k/Downloads/statement-oct.pdf → /Users/k/Bank/2025-10.pdf",
				"/Users/k/Downloads/w2.pdf → /Users/k/Taxes/2025/w2.pdf"},
			From: "from a mail by bank@example.org", Outsider: true, Move: 12, Role: "steward", Undo: 12},
		{ID: "move:10", Kind: "move", At: t0.Add(-9 * time.Minute).Format(time.RFC3339), Seq: 10, Title: "notes.txt (undone)",
			Lines: []string{"By steward.", "/Users/k/notes.txt → /Users/k/Notes/notes.txt (undone)"}, Move: 10, Role: "steward", Undone: true},
	}}, core.StopTyping{At: t0}, core.ReadyOpen{At: t0})
	undoRefused := hubStep(move, core.ReadyKey{At: t0, Action: core.ReadyUndoKey},
		core.UndoDone{At: t0, Move: 12, Err: "Move 12 was not undone, and nothing changed: /Users/k/Bank/2025-10.pdf has changed since it was moved."})
	for _, c := range []struct {
		name string
		h    core.Hub
	}{{"items", h}, {"last", moved}, {"reply", replying}, {"empty", empty}, {"move", move}, {"undo_refused", undoRefused}} {
		// A 60 column phone and a 120 column screen; the shell draws one
		// column short of the screen.
		for _, w := range []int{59, 119} {
			t.Run(fmt.Sprintf("%s_%d", c.name, w), func(t *testing.T) {
				requireGolden(t, HubView(c.h, "", "", t0, w, 16).Content)
			})
		}
	}
}

func TestReadyFrameFitsAtAnySize(t *testing.T) {
	views := map[string]core.Hub{
		"items": readyHubFixture(),
		"reply": hubStep(readyHubFixture(), core.ReadyKey{At: t0, Action: core.ReadyDown}, core.ReadyKey{At: t0, Action: core.ReadyDown},
			core.ReadyKey{At: t0, Action: core.ReadyDown}, core.ReadyKey{At: t0, Action: core.ReadyReplyKey},
			core.ReadyKey{At: t0, Action: core.ReadyChar, Text: strings.Repeat("long ", 40)}),
		"refused": hubStep(readyHubFixture(), core.UndoDone{At: t0, Move: 1, Err: strings.Repeat("a reason that is long ", 20)}),
		"loading": hubStep(core.NewHub(""), core.ReadyOpen{At: t0}),
		"none":    hubStep(core.NewHub(""), core.HubConnected{At: t0}, core.ReadyLoaded{At: t0, Unavailable: true}, core.ReadyOpen{At: t0}),
	}
	for name, v := range views {
		for _, w := range []int{1, 8, 20, 40, 59, 72, 99, 119, 160} {
			for _, rows := range []int{2, 5, 10, 24, 40} {
				f := HubView(v, "", "hint", t0, w, rows)
				lines := strings.Split(f.Content, "\n")
				if len(lines) != rows {
					t.Errorf("%s %dx%d: %d rows", name, w, rows, len(lines))
				}
				for i, l := range lines {
					if got := Width(l); got > w {
						t.Errorf("%s %dx%d: row %d is %d wide: %q", name, w, rows, i, got, l)
					}
				}
			}
		}
	}
}

func TestReadyKeepsTheSelectedItemInView(t *testing.T) {
	h := readyHubFixture()
	for i := 0; i < 5; i++ {
		plain := ansi.Strip(HubView(h, "", "", t0, 59, 14).Content)
		_, sel := core.ReadySelected(h.Ready)
		if want := h.Ready.Items[sel].Title; !strings.Contains(plain, want) {
			t.Errorf("item %d (%q) is not on the screen:\n%s", sel, want, plain)
		}
		h = hubStep(h, core.ReadyKey{At: t0, Action: core.ReadyDown})
	}
}

func TestReadySaysWhereAThingCameFromInWords(t *testing.T) {
	plain := ansi.Strip(HubView(readyHubFixture(), "", "", t0, 119, 40).Content)
	for _, want := range []string{"from a mail by booker@example.org", "Draft", "Calendar", "Message", "Board",
		"↩ you replied: Shorter please.", "R close"} {
		if !strings.Contains(plain, want) {
			t.Errorf("%q is not on the screen:\n%s", want, plain)
		}
	}
}

// Nothing in the view names a label or a gate, whatever relay sends in
// "from", and text that does not print is escaped.
func TestReadyNeverNamesALabel(t *testing.T) {
	it := core.ReadyItem{ID: "x", Kind: "draft", At: t0.Format(time.RFC3339), Title: "T\x1b[31m",
		Lines: []string{"body \x07 https://evil.example/x"}, From: "from the contaminated private untrusted thing", Outsider: true}
	h := hubStep(core.NewHub(""), core.HubConnected{At: t0}, core.ReadyLoaded{At: t0, Rev: 1, Items: []core.ReadyItem{it}}, core.ReadyOpen{At: t0})
	for _, w := range []int{39, 119} {
		out := HubView(h, "", "", t0, w, 20).Content
		plain := strings.ToLower(ansi.Strip(out))
		for _, word := range []string{"contaminated", "untrusted", "private", "trust", "approv", "held", "permission"} {
			if strings.Contains(plain, word) {
				t.Errorf("%q on the screen at %d:\n%s", word, w, plain)
			}
		}
		if strings.Contains(out, "\x07") || strings.Contains(out, "\x1b[31m") {
			t.Errorf("an escape reached the screen: %q", out)
		}
		if strings.Contains(plain, "https://evil") {
			t.Errorf("a live link on the screen:\n%s", plain)
		}
	}
}

func TestReadyTimesAreInTheOperatorsZone(t *testing.T) {
	zone := time.FixedZone("EST", -5*3600)
	now := t0.In(zone)
	it := core.ReadyItem{Kind: "event", Start: "2026-11-21T00:00:00Z", End: "2026-11-21T01:30:00Z"}
	if got, want := whenLine(it, now), "Fri Nov 20 · 19:00–20:30"; got != want {
		t.Errorf("%q want %q", got, want)
	}
	it.End = "2026-11-22T01:30:00Z"
	if got, want := whenLine(it, now), "Fri Nov 20 · 19:00 – Sat Nov 21 · 20:30"; got != want {
		t.Errorf("%q want %q", got, want)
	}
	for _, c := range []struct {
		at   string
		want string
	}{{t0.Add(-10 * time.Second).Format(time.RFC3339), "now"}, {t0.Add(-5 * time.Minute).Format(time.RFC3339), "5m"},
		{t0.Add(-3 * time.Hour).Format(time.RFC3339), "3h"}, {t0.Add(-30 * time.Hour).Format(time.RFC3339), "yesterday"},
		{t0.Add(-72 * time.Hour).Format(time.RFC3339), "Oct 12"}, {"nonsense", ""}} {
		if got := ago(c.at, t0); got != c.want {
			t.Errorf("ago(%q) = %q want %q", c.at, got, c.want)
		}
	}
}

// Any items, any text, any size: the frame has the rows and width asked
// for, and shows no escape of its own making from the text.
func TestReadyFrameFitsWhateverRelaySends(t *testing.T) {
	text := rapid.StringMatching(`[a-zA-Z0-9 \n\t.:/\x1b\x07世界é‮-]{0,60}`)
	rapid.Check(t, func(t *rapid.T) {
		var items []core.ReadyItem
		for i := 0; i < rapid.IntRange(0, 5).Draw(t, "n"); i++ {
			items = append(items, core.ReadyItem{
				ID: fmt.Sprint("i", i), Kind: rapid.SampledFrom([]string{"message", "draft", "event", "finding", "move", "?"}).Draw(t, "kind"),
				At: t0.Format(time.RFC3339), Title: text.Draw(t, "title"),
				Lines:    rapid.SliceOfN(text, 0, 4).Draw(t, "lines"),
				From:     text.Draw(t, "from"),
				Outsider: rapid.Bool().Draw(t, "out"), ReplyTo: rapid.Int64Range(0, 3).Draw(t, "reply"),
				Start: rapid.SampledFrom([]string{"", "2026-11-21T00:00:00Z", "bad"}).Draw(t, "start")})
		}
		h := hubStep(core.NewHub(""), core.HubConnected{At: t0}, core.ReadyLoaded{At: t0, Rev: 1, Items: items}, core.ReadyOpen{At: t0})
		w, rows := rapid.IntRange(1, 130).Draw(t, "w"), rapid.IntRange(2, 40).Draw(t, "rows")
		lines := strings.Split(HubView(h, "", "", t0, w, rows).Content, "\n")
		if len(lines) != rows {
			t.Fatalf("%dx%d: %d rows", w, rows, len(lines))
		}
		for i, l := range lines {
			if got := Width(l); got > w {
				t.Fatalf("%dx%d: row %d is %d wide: %q", w, rows, i, got, l)
			}
			if strings.ContainsAny(ansi.Strip(l), "\x07\x1b") {
				t.Fatalf("a control character on row %d: %q", i, l)
			}
		}
	})
}

func TestARefusedReplyShowsWhyAndKeepsTheWords(t *testing.T) {
	h := hubStep(readyHubFixture(), core.ReadyKey{At: t0, Action: core.ReadyDown}, core.ReadyKey{At: t0, Action: core.ReadyDown},
		core.ReadyKey{At: t0, Action: core.ReadyDown}, core.ReadyKey{At: t0, Action: core.ReadyReplyKey},
		core.ReadyKey{At: t0, Action: core.ReadyChar, Text: "Thank you"}, core.ReadyKey{At: t0, Action: core.ReadyEnter},
		core.ReplyDone{At: t0, Message: 4, Err: "A reply is at most 4000 characters."})
	for _, w := range []int{59, 119} {
		t.Run(fmt.Sprint(w), func(t *testing.T) { refusedReplyAt(t, h, w) })
	}
}

func refusedReplyAt(t *testing.T, h core.Hub, w int) {
	{
		f := HubView(h, "", "", t0, w, 16)
		plain := ansi.Strip(f.Content)
		if !strings.Contains(plain, "Not sent.") || !strings.Contains(plain, "at most 4000") {
			t.Errorf("%d: the refusal is hidden:\n%s", w, plain)
		}
		if !strings.Contains(plain, "Thank you") {
			t.Errorf("%d: the words are gone:\n%s", w, plain)
		}
		if f.Active != core.SurfaceReadyInput {
			t.Errorf("%d: the input is not open", w)
		}
		requireGolden(t, f.Content)
	}
}
