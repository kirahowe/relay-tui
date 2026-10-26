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
		{ID: "move:12", Kind: "move", At: t0.Add(-5 * time.Minute).Format(time.RFC3339), Seq: 12, Title: "2 files",
			Lines: []string{"2 files in Documents, into 2 folders", "Downloads/statement-oct.pdf → Bank/2025-10.pdf",
				"Downloads/w2.pdf → Taxes/2025/w2.pdf"},
			From: "from a mail by bank@example.org", Outsider: true, Move: 12, Role: "steward", Undo: 12},
		{ID: "move:10", Kind: "move", At: t0.Add(-9 * time.Minute).Format(time.RFC3339), Seq: 10, Title: "notes.txt",
			Lines: []string{"1 file in Documents", "notes.txt → Notes/notes.txt (undone)"}, Move: 10, Role: "steward", Undone: true},
	}}, core.StopTyping{At: t0}, core.ReadyOpen{At: t0})
	undoRefused := hubStep(move, core.ReadyKey{At: t0, Action: core.ReadyUndoKey},
		core.UndoDone{At: t0, Move: 12, Err: "Move 12 was not undone, and nothing changed: Bank/2025-10.pdf has changed since it was moved."})
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
	if got, want := whenLine(it, now), "Fri 20 Nov · 19:00–20:30"; got != want {
		t.Errorf("%q want %q", got, want)
	}
	it.End = "2026-11-22T01:30:00Z"
	if got, want := whenLine(it, now), "Fri 20 Nov · 19:00 – Sat 21 Nov · 20:30"; got != want {
		t.Errorf("%q want %q", got, want)
	}
	for _, c := range []struct {
		at   string
		want string
	}{{t0.Add(-10 * time.Second).Format(time.RFC3339), "now"}, {t0.Add(-5 * time.Minute).Format(time.RFC3339), "5m"},
		{t0.Add(-3 * time.Hour).Format(time.RFC3339), "3h"}, {t0.Add(-30 * time.Hour).Format(time.RFC3339), "yesterday"},
		{t0.Add(-72 * time.Hour).Format(time.RFC3339), "12 Oct"}, {"nonsense", ""}} {
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

// --- papercuts: narrow titles, moves, the whole message, keys

func deadLetters() []core.ReadyItem {
	mk := func(id string, seq int64, tail string) core.ReadyItem {
		return core.ReadyItem{ID: id, Kind: "message", At: t0.Add(-time.Minute).Format(time.RFC3339), Seq: seq,
			Title: `Could not finish "Dead letter: scout could not read the page about ` + tail + `"`,
			Lines: []string{"It will try again."}, Message: seq, Urgency: "normal"}
	}
	return []core.ReadyItem{mk("message:2", 2, "the choir hall"), mk("message:1", 1, "the town budget")}
}

// A title too long for the border moves into the panel, wrapped, so the
// age stays and two titles that start alike can be told apart.
func TestReadyNarrowKeepsTheAgeAndTheWholeTitle(t *testing.T) {
	h := hubStep(core.NewHub(""), core.HubConnected{At: t0}, core.ReadyLoaded{At: t0, Rev: 1, Items: deadLetters()}, core.ReadyOpen{At: t0})
	plain := ansi.Strip(HubView(h, "", "", t0, 59, 30).Content)
	for _, want := range []string{"choir hall", "town budget"} {
		if !strings.Contains(strings.Join(strings.Fields(plain), " "), want) {
			t.Errorf("%q is cut:\n%s", want, plain)
		}
	}
	tops := 0
	for _, l := range strings.Split(plain, "\n") {
		if strings.HasPrefix(l, "╭─ Message") {
			tops++
			if !strings.Contains(l, " 1m ─╮") {
				t.Errorf("the age is not in the border: %q", l)
			}
		}
	}
	if tops != 2 {
		t.Errorf("%d panels:\n%s", tops, plain)
	}
	requireGolden(t, HubView(h, "", "", t0, 59, 20).Content)
}

// Whatever the title and the width, the age is in the top border and no
// row is too wide.
func TestReadyAgeNeverDropsOut(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		title := rapid.StringMatching(`[a-zA-Z0-9 :"]{0,120}`).Draw(t, "title")
		w := rapid.IntRange(20, 120).Draw(t, "w")
		it := core.ReadyItem{ID: "message:1", Kind: "message", At: t0.Add(-5 * time.Minute).Format(time.RFC3339), Title: title, Message: 1}
		panelRows := readyPanel(it, t0, min(w, readyMaxWidth), false)
		if top := ansi.Strip(panelRows[0]); !strings.Contains(top, " 5m ─╮") {
			t.Fatalf("w=%d title=%q: %q", w, title, top)
		}
		for _, l := range panelRows {
			if Width(l) != min(w, readyMaxWidth) {
				t.Fatalf("row %q is %d wide, want %d", l, Width(l), min(w, readyMaxWidth))
			}
		}
	})
}

func manyMoves() core.ReadyItem {
	lines := []string{"41 files in notes, into 13 folders"}
	for i := 0; i < 41; i++ {
		lines = append(lines, fmt.Sprintf("meeting %02d.md → meetings/2026-09-%02d-board-meeting-with-a-long-name.md", i, i%28+1))
	}
	return core.ReadyItem{ID: "move:3", Kind: "move", At: t0.Add(-time.Minute).Format(time.RFC3339), Title: "41 files",
		Lines: lines, Move: 3, Undo: 3}
}

// A move with many files is a summary, a few files one row each, and a
// count; Enter lists them all.
func TestReadyMovePanelIsASummaryAndACount(t *testing.T) {
	h := hubStep(core.NewHub(""), core.HubConnected{At: t0}, core.ReadyLoaded{At: t0, Rev: 1, Items: []core.ReadyItem{manyMoves()}}, core.ReadyOpen{At: t0})
	plain := ansi.Strip(HubView(h, "", "", t0, 59, 20).Content)
	for _, want := range []string{"41 files in notes, into 13 folders", "meeting 00.md", "and 38 more · enter lists all"} {
		if !strings.Contains(plain, want) {
			t.Errorf("%q missing:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "meeting 05.md") {
		t.Errorf("more than the first files:\n%s", plain)
	}
	requireGolden(t, HubView(h, "", "", t0, 59, 20).Content)
	opened := hubStep(h, core.ReadyKey{At: t0, Action: core.ReadyOpenKey})
	f := HubView(opened, "", "", t0, 59, 20)
	if f.Max == 0 || f.Page == 0 {
		t.Errorf("a long list should scroll: max %d page %d", f.Max, f.Page)
	}
	end := hubStep(opened, core.ReadyKey{At: t0, Action: core.ReadyBottom, Max: f.Max, Page: f.Page})
	if got := ansi.Strip(HubView(end, "", "", t0, 59, 20).Content); !strings.Contains(got, "meeting 40.md") {
		t.Errorf("the end of the list is not reachable:\n%s", got)
	}
}

func dailyNote() core.ReadyItem {
	return core.ReadyItem{ID: "message:9", Kind: "message", At: t0.Add(-time.Minute).Format(time.RFC3339), Seq: 9, Message: 9, ReplyTo: 9,
		Title: "Daily note for Monday 26 October: 2 changes to check",
		Lines: []string{
			`1. New how-to "grants" (1 line added), shaped by a mail by stranger@example.org. To undo it: relay revert 14, or reply "revert 1".`,
			`2. A skill changed, shaped by the web. To undo it: relay revert 15, or reply "revert 2".`,
			`3. Dentist on Thursday. Want me to book a reminder?`,
			`Reply "not N" to drop a suggestion. It will not come back.`,
		},
		Acts: []core.ReadyAct{{N: 1, Op: core.OpRevert, Line: 0}, {N: 2, Op: core.OpRevert, Line: 1, Done: "reverted"}, {N: 3, Op: core.OpDismiss, Line: 2, Offer: true}},
		From: "from a mail by stranger@example.org", Outsider: true}
}

// dailyNoteWithHeadings is dailyNote as relay sends it with its sections.
func dailyNoteWithHeadings() core.ReadyItem {
	it := dailyNote()
	it.Sections = []core.ReadySection{{Heading: "Changes to check", Lines: it.Lines[:2]}, {Heading: "Coming up", Lines: it.Lines[2:3]},
		{Lines: it.Lines[3:]}}
	return it
}

func TestAnOpenNoteKeepsItsHeadingsAndMarksWhatWasDone(t *testing.T) {
	it := dailyNoteWithHeadings()
	it.Lines = append([]string(nil), it.Lines...)
	it.Lines[1] += " (undone)" // relay marks its own lines too
	h := hubStep(core.NewHub(""), core.HubConnected{At: t0}, core.ReadyLoaded{At: t0, Rev: 1, Items: []core.ReadyItem{it}},
		core.ReadyOpen{At: t0}, core.ReadyKey{At: t0, Action: core.ReadyPick, Text: "3"})
	plain := ansi.Strip(HubView(h, "", "", t0, 59, 40).Content)
	for _, want := range []string{"Changes to check", "Coming up", "▸ 3. Dentist"} {
		if !strings.Contains(plain, want) {
			t.Errorf("%q missing:\n%s", want, plain)
		}
	}
	if strings.Count(plain, "(undone)") != 1 {
		t.Errorf("a done item is marked once:\n%s", plain)
	}
	if !strings.Contains(plain, "n no thanks") {
		t.Errorf("an item with an offer offers n:\n%s", plain)
	}
	h = hubStep(h, core.ActDone{At: t0, Message: 9, N: 3, Op: core.OpDecline})
	plain = ansi.Strip(HubView(h, "", "", t0, 59, 40).Content)
	if !strings.Contains(strings.Join(strings.Fields(strings.ReplaceAll(plain, "│", " ")), " "), "(offer declined)") {
		t.Errorf("a declined offer is marked:\n%s", plain)
	}
}

func TestARefusedUndoIsReadWholeOnAPhone(t *testing.T) {
	h := hubStep(core.NewHub(""), core.HubConnected{At: t0},
		core.ReadyLoaded{At: t0, Rev: 1, Items: []core.ReadyItem{{ID: "move:22", Kind: core.ReadyMove, At: t0.Format(time.RFC3339), Title: "41 files", Move: 22, Undo: 22}}},
		core.ReadyOpen{At: t0}, core.ReadyKey{At: t0, Action: core.ReadyUndoKey},
		core.UndoDone{At: t0, Move: 22, Err: "Move 22 was not undone, and nothing changed: todo/todo.txt has changed since it was moved there." +
			" recipes/soup.md has changed since it was moved there. Once each is as the move left it, relay undo 22 puts it back."})
	plain := ansi.Strip(HubView(h, "", "", t0, 59, 22).Content)
	flat := strings.Join(strings.Fields(plain), " ")
	for _, want := range []string{"todo/todo.txt has changed", "recipes/soup.md has changed", "relay undo 22 puts it back."} {
		if !strings.Contains(flat, want) {
			t.Errorf("%q is cut:\n%s", want, plain)
		}
	}
}

func noteHub() core.Hub {
	return hubStep(core.NewHub(""), core.HubConnected{At: t0}, core.ReadyLoaded{At: t0, Rev: 1, Items: []core.ReadyItem{dailyNote()}},
		core.ReadyOpen{At: t0}, core.ReadyKey{At: t0, Action: core.ReadyOpenKey})
}

func TestReadyOpenItemGolden(t *testing.T) {
	open := noteHub()
	next := hubStep(open, core.ReadyKey{At: t0, Action: core.ReadyNext}, core.ReadyKey{At: t0, Action: core.ReadyNext})
	help := hubStep(open, core.ReadyKey{At: t0, Action: core.ReadyHelp})
	listHelp := hubStep(readyHubFixture(), core.ReadyKey{At: t0, Action: core.ReadyHelp})
	for _, c := range []struct {
		name string
		h    core.Hub
	}{{"note", open}, {"note_third", next}, {"note_keys", help}, {"list_keys", listHelp}} {
		for _, w := range []int{59, 119} {
			t.Run(fmt.Sprintf("%s_%d", c.name, w), func(t *testing.T) {
				requireGolden(t, HubView(c.h, "", "", t0, w, 22).Content)
			})
		}
	}
}

// The whole message is on screen across scrolling, and the keys list
// names every key and wraps instead of cutting.
func TestReadyOpenItemShowsTheWholeMessage(t *testing.T) {
	plain := ansi.Strip(HubView(noteHub(), "", "", t0, 59, 40).Content)
	flat := strings.Join(strings.Fields(strings.NewReplacer("│", " ", "▸", " ").Replace(plain)), " ")
	for _, want := range []string{"Daily note for Monday 26 October: 2 changes to check", "relay revert 14", "(undone)",
		"Dentist on Thursday", "It will not come back.", "v revert", "from a mail by stranger"} {
		if !strings.Contains(flat, want) {
			t.Errorf("%q missing:\n%s", want, plain)
		}
	}
	if !strings.Contains(plain, "▸ 1. New how-to") {
		t.Errorf("the picked item is not marked:\n%s", plain)
	}
	help := ansi.Strip(HubView(hubStep(noteHub(), core.ReadyKey{At: t0, Action: core.ReadyHelp}), "", "", t0, 59, 40).Content)
	for _, r := range ReadyDetailKeys {
		for _, word := range strings.Fields(r.What) {
			if !strings.Contains(help, word) {
				t.Errorf("%q of %q is cut at 59 columns:\n%s", word, r.What, help)
			}
		}
	}
	if strings.Contains(help, "…") {
		t.Errorf("the keys list is cut:\n%s", help)
	}
}

// Whatever is open, in whatever size, the frame fits.
func TestReadyOpenItemFitsAtAnySize(t *testing.T) {
	views := map[string]core.Hub{
		"note":  noteHub(),
		"keys":  hubStep(noteHub(), core.ReadyKey{At: t0, Action: core.ReadyHelp}),
		"moves": hubStep(core.NewHub(""), core.HubConnected{At: t0}, core.ReadyLoaded{At: t0, Rev: 1, Items: []core.ReadyItem{manyMoves()}}, core.ReadyOpen{At: t0}, core.ReadyKey{At: t0, Action: core.ReadyOpenKey}),
		"reply": hubStep(noteHub(), core.ReadyKey{At: t0, Action: core.ReadyReplyKey}, core.ReadyKey{At: t0, Action: core.ReadyChar, Text: strings.Repeat("long ", 30)}),
	}
	for name, v := range views {
		for _, w := range []int{1, 8, 20, 40, 59, 100, 160} {
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
				if f.Top < 0 || f.Top > f.Max {
					t.Errorf("%s %dx%d: top %d max %d", name, w, rows, f.Top, f.Max)
				}
			}
		}
	}
}

// The picked numbered item stays on screen as the pick moves.
func TestReadyFollowsThePickedItem(t *testing.T) {
	it := dailyNote()
	for i := 0; i < 12; i++ {
		it.Lines = append(it.Lines, fmt.Sprintf("%d. Another suggestion with some words to fill a row or two on a phone.", i+4))
		it.Acts = append(it.Acts, core.ReadyAct{N: i + 4, Op: core.OpDismiss, Line: len(it.Lines) - 1})
	}
	h := hubStep(core.NewHub(""), core.HubConnected{At: t0}, core.ReadyLoaded{At: t0, Rev: 1, Items: []core.ReadyItem{it}},
		core.ReadyOpen{At: t0}, core.ReadyKey{At: t0, Action: core.ReadyOpenKey})
	for i := 0; i < len(it.Acts)-1; i++ {
		f := HubView(h, "", "", t0, 59, 14)
		h = hubStep(h, core.ReadyKey{At: t0, Action: core.ReadyNext, Top: f.Top, Max: f.Max, Page: f.Page})
		plain := ansi.Strip(HubView(h, "", "", t0, 59, 14).Content)
		want := fmt.Sprintf("▸ %d. ", h.Ready.Pick+1)
		if h.Ready.Pick >= 3 {
			want = "▸ " + fmt.Sprint(h.Ready.Pick+1) + ". "
		}
		if !strings.Contains(plain, want) {
			t.Fatalf("pick %d: %q is not on screen:\n%s", h.Ready.Pick, want, plain)
		}
	}
}

// Times show in the zone relay named, whatever the machine's zone is, and
// day then month, as relay's own output has them.
func TestReadyShowsTimesInRelaysZone(t *testing.T) {
	mac := time.FixedZone("AST", -4*3600) // the Mac says Halifax in winter
	now := time.Date(2027, 1, 4, 9, 0, 0, 0, mac)
	h := hubStep(hubFixture(), core.ReadyLoaded{At: now, Rev: 1, Zone: "America/Toronto", Items: []core.ReadyItem{
		{ID: "event:1", Kind: "event", At: "2027-01-04T14:00:00Z", Title: "Load-in", Start: "2027-01-04T15:00:00Z", End: "2027-01-04T15:30:00Z"},
	}}, core.StopTyping{At: now}, core.ReadyOpen{At: now})
	out := ansi.Strip(HubView(h, "", "", now, 80, 20).Content)
	if !strings.Contains(out, "Mon 4 Jan · 10:00–10:30") {
		t.Errorf("want Toronto's 10:00:\n%s", out)
	}
	// No zone from relay: the machine's.
	h.Ready.Zone = ""
	if out := ansi.Strip(HubView(h, "", "", now, 80, 20).Content); !strings.Contains(out, "Mon 4 Jan · 11:00–11:30") {
		t.Errorf("want the machine's 11:00:\n%s", out)
	}
	h.Ready.Zone = "Not/AZone"
	if out := ansi.Strip(HubView(h, "", "", now, 80, 20).Content); !strings.Contains(out, "11:00") {
		t.Errorf("an unknown zone should fall back:\n%s", out)
	}
}

// New items out of sight above the selection show in the top bar; once
// they are in sight, or reached, the cue goes.
func TestReadyBarCountsNewItemsOutOfSight(t *testing.T) {
	var items []core.ReadyItem
	for i := 1; i <= 6; i++ {
		items = append(items, core.ReadyItem{ID: fmt.Sprintf("message:%d", i), Kind: "message", Message: int64(i),
			At: t0.Add(-time.Duration(i) * time.Hour).Format(time.RFC3339), Title: fmt.Sprintf("Old %d", i), Lines: []string{"a line"}})
	}
	h := hubStep(hubFixture(), core.ReadyLoaded{At: t0, Rev: 1, Items: items}, core.StopTyping{At: t0}, core.ReadyOpen{At: t0},
		core.ReadyKey{At: t0, Action: core.ReadyBottom})
	h = hubStep(h, core.ReadyChanged{At: t0, Rev: 2, Items: []core.ReadyItem{
		{ID: "message:20", Kind: "message", Message: 20, At: t0.Format(time.RFC3339), Title: "Load-in added", Lines: []string{"x"}}}})
	bar := func(h core.Hub) string {
		return strings.SplitN(ansi.Strip(HubView(h, "", "", t0, 60, 14).Content), "\n", 2)[0]
	}
	if b := bar(h); !strings.Contains(b, "↑ 1 new") {
		t.Errorf("bar %q lacks the cue", b)
	}
	if b := bar(hubStep(h, core.ReadyKey{At: t0, Action: core.ReadyTop})); strings.Contains(b, "new") {
		t.Errorf("bar %q still has the cue at the top", b)
	}
	// Tall enough for all of them: the new one is in sight, so no cue.
	if b := strings.SplitN(ansi.Strip(HubView(h, "", "", t0, 60, 80).Content), "\n", 2)[0]; strings.Contains(b, "new") {
		t.Errorf("bar %q has the cue though the item shows", b)
	}
}
