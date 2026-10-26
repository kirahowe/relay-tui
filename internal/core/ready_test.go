package core

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"pgregory.net/rapid"
)

func at(min int) string { return t0.Add(time.Duration(min) * time.Minute).Format(time.RFC3339) }

func msg(id int64, min int) ReadyItem {
	return ReadyItem{ID: fmt.Sprintf("message:%d", id), Kind: ReadyMessage, At: at(min), Title: fmt.Sprintf("Message %d", id),
		Message: id, ReplyTo: id}
}

func draft(id int64, min int) ReadyItem {
	return ReadyItem{ID: fmt.Sprintf("prepared:%d", id), Kind: ReadyDraft, At: at(min), Title: fmt.Sprintf("Draft %d", id)}
}

// readyHub is a hub connection with the roster in, on the list, and
// relay's answer to what is ready loaded.
func readyHub(t interface{ Helper() }, items ...ReadyItem) Hub {
	t.Helper()
	h, _ := apply(t, NewHub(""), HubConnected{At: t0},
		RosterUpdate{At: t0, Roles: []Role{role("dev", "s1"), role("mail", "")}},
		ReadyLoaded{At: t0, Rev: 10, Items: items})
	return h
}

func ids(r Ready) []string {
	var out []string
	for _, it := range r.Items {
		out = append(out, it.ID)
	}
	return out
}

func TestReadyIsAskedForOnceConnected(t *testing.T) {
	h, fx := apply(t, NewHub(""), HubConnected{At: t0})
	if got := effectsOf[RequestReady](fx); len(got) != 1 {
		t.Fatalf("effects %v", fx)
	}
	h, fx = apply(t, h, ReadyLoaded{At: t0, Rev: 5, Items: []ReadyItem{msg(4, 1)}}, ReadyOpen{At: t0})
	if len(effectsOf[RequestReady](fx)) != 0 || !h.Ready.Loaded || !h.Ready.Open {
		t.Fatalf("asked again, or did not open: %v %+v", fx, h.Ready)
	}
}

func TestReadyHoldsItemsNewestFirstAndKeepsOnePerID(t *testing.T) {
	h := readyHub(t, draft(5, 1), msg(4, 3), draft(5, 2), msg(6, 3))
	// The same time breaks by id; a repeated id keeps its first copy.
	if got, want := ids(h.Ready), []string{"message:4", "message:6", "prepared:5"}; !slices.Equal(got, want) {
		t.Fatalf("%v want %v", got, want)
	}
	if h.Ready.Sel != "message:4" {
		t.Errorf("selected %q", h.Ready.Sel)
	}
}

func TestReadyChangedAddsReplacesAndRemoves(t *testing.T) {
	h := readyHub(t, msg(4, 1), draft(5, 2))
	h, _ = apply(t, h, ReadyKey{At: t0, Action: ReadyUp}) // not open: ignored
	changed := msg(4, 1)
	changed.Replies = []ReadyReply{{At: at(5), Text: "Thanks"}}
	h, _ = apply(t, h, ReadyChanged{At: t0, Rev: 11, Items: []ReadyItem{changed, draft(9, 4)}, Gone: []string{"prepared:5"}})
	if got, want := ids(h.Ready), []string{"prepared:9", "message:4"}; !slices.Equal(got, want) {
		t.Fatalf("%v want %v", got, want)
	}
	if h.Ready.Items[1].Replies[0].Text != "Thanks" || h.Ready.Rev != 11 {
		t.Errorf("%+v", h.Ready)
	}
}

func TestReadyKeepsTheSelectionOnTheSameItem(t *testing.T) {
	h := readyHub(t, msg(4, 1), draft(5, 2), draft(6, 3))
	h, _ = apply(t, h, ReadyOpen{At: t0}, ReadyKey{At: t0, Action: ReadyDown}, ReadyKey{At: t0, Action: ReadyDown})
	if h.Ready.Sel != "message:4" {
		t.Fatalf("selected %q", h.Ready.Sel)
	}
	// A newer item arrives above it: the selection stays on its item.
	h, _ = apply(t, h, ReadyChanged{At: t0, Rev: 11, Items: []ReadyItem{draft(9, 9)}})
	if h.Ready.Sel != "message:4" {
		t.Errorf("selected %q after one arrived", h.Ready.Sel)
	}
	// It goes: the nearest one is selected.
	h, _ = apply(t, h, ReadyChanged{At: t0, Rev: 12, Gone: []string{"message:4"}})
	if h.Ready.Sel != "prepared:5" {
		t.Errorf("selected %q after it went", h.Ready.Sel)
	}
	h, _ = apply(t, h, ReadyChanged{At: t0, Rev: 13, Gone: ids(h.Ready)})
	if h.Ready.Sel != "" {
		t.Errorf("selected %q with nothing left", h.Ready.Sel)
	}
}

func TestReadyOpensAndClosesAndOwnsTheKeys(t *testing.T) {
	h := readyHub(t, msg(4, 1))
	h, _ = apply(t, h, ReadyOpen{At: t0})
	if s := HubSurface(h); s.Kind != SurfaceReady || s.Typing() {
		t.Fatalf("surface %+v", s)
	}
	// Keys meant for the pane, the conversation or the list do nothing
	// while it is open.
	before := h
	h, _ = apply(t, h, PaneKey{At: t0, Action: PaneEnter}, ListMove{At: t0, Delta: 1}, AskEnd{At: t0}, Type{At: t0},
		Move{At: t0, Delta: 1}, TranscriptKey{At: t0, Action: TranscriptUp})
	if h.List != before.List || h.EndAsk || h.App.Moving != before.App.Moving || h.Focus != before.Focus {
		t.Errorf("a key reached behind the view: %+v", h)
	}
	h, _ = apply(t, h, ReadyKey{At: t0, Action: ReadyEsc})
	if h.Ready.Open || HubSurface(h).Kind == SurfaceReady {
		t.Errorf("esc did not close it: %+v", h.Ready)
	}
	h, _ = apply(t, h, ReadyOpen{At: t0}, ReadyOpen{At: t0})
	if h.Ready.Open {
		t.Errorf("R did not close it")
	}
}

func TestReadyIsNotOpenedOverAQuestion(t *testing.T) {
	h := readyHub(t, msg(4, 1))
	h, _ = apply(t, h, Move{At: t0, Delta: 1}, ToggleHelp{At: t0})
	h, _ = apply(t, h, ReadyOpen{At: t0})
	if h.Ready.Open {
		t.Errorf("opened over the help")
	}
}

func moved(id int64, min int, undo int64) ReadyItem {
	return ReadyItem{ID: fmt.Sprintf("move:%d", id), Kind: ReadyMove, At: at(min), Seq: id, Title: "statement.pdf",
		Move: id, Role: "steward", Undo: undo}
}

func TestItemsAtTheSameTimeAreNewestFirstByTheLogsOrder(t *testing.T) {
	a, b, c := draft(9, 1), msg(4, 1), moved(2, 1, 2)
	a.Seq, b.Seq = 3, 7
	h := readyHub(t, a, b, c, msg(1, 2))
	if got, want := ids(h.Ready), []string{"message:1", "message:4", "prepared:9", "move:2"}; !slices.Equal(got, want) {
		t.Fatalf("%v want %v", got, want)
	}
}

func TestReadyBeforeIsATotalOrder(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		gen := rapid.Custom(func(t *rapid.T) ReadyItem {
			return ReadyItem{ID: fmt.Sprintf("x:%d", rapid.IntRange(0, 3).Draw(t, "id")),
				At: at(rapid.IntRange(0, 2).Draw(t, "min")), Seq: rapid.Int64Range(0, 3).Draw(t, "seq")}
		})
		a, b := gen.Draw(t, "a"), gen.Draw(t, "b")
		if readyBefore(a, b) != -readyBefore(b, a) {
			t.Fatalf("not antisymmetric: %+v %+v", a, b)
		}
		if a.At == b.At && a.Seq > b.Seq && readyBefore(a, b) >= 0 {
			t.Fatalf("the later in the log is not first: %+v %+v", a, b)
		}
		if readyBefore(a, b) == 0 && a.ID != b.ID {
			t.Fatalf("a tie between different items: %+v %+v", a, b)
		}
	})
}

func TestAMoveIsUndoneOnceAndARefusalSaysWhy(t *testing.T) {
	h := readyHub(t, moved(5, 2, 5), msg(4, 1), moved(3, 0, 0))
	h, _ = apply(t, h, ReadyOpen{At: t0})
	h, fx := apply(t, h, ReadyKey{At: t0, Action: ReadyUndoKey})
	if got := effectsOf[SendUndo](fx); len(got) != 1 || got[0].Move != 5 || h.Ready.Undoing != 5 {
		t.Fatalf("%v %+v", fx, h.Ready)
	}
	// One undo at a time.
	h, fx = apply(t, h, ReadyKey{At: t0, Action: ReadyUndoKey})
	if len(effectsOf[SendUndo](fx)) != 0 || h.Ready.Message != UndoBusyNote {
		t.Fatalf("%v %+v", fx, h.Ready)
	}
	h, _ = apply(t, h, UndoDone{At: t0, Move: 5, Err: "Move 5 was not undone, and nothing changed: /f/s.pdf has changed since it was moved."})
	if h.Ready.Undoing != 0 || h.Ready.Message != UndoRefusedNote || h.Ready.Failure == "" {
		t.Fatalf("%+v", h.Ready)
	}
	h, fx = apply(t, h, ReadyKey{At: t0, Action: ReadyUndoKey})
	h, _ = apply(t, h, UndoDone{At: t0, Move: 5, Lines: []string{"Undid move 5: put back 1 of 1."}})
	if len(effectsOf[SendUndo](fx)) != 1 || h.Ready.Message != "Undid move 5: put back 1 of 1." || h.Ready.Failure != "" {
		t.Fatalf("%v %+v", fx, h.Ready)
	}
	// A message, and a move with nothing left to undo, offer none.
	for _, down := range []int{1, 2} {
		h, _ = apply(t, h, ReadyKey{At: t0, Action: ReadyTop})
		for range down {
			h, _ = apply(t, h, ReadyKey{At: t0, Action: ReadyDown})
		}
		h, fx = apply(t, h, ReadyKey{At: t0, Action: ReadyUndoKey})
		if len(effectsOf[SendUndo](fx)) != 0 || h.Ready.Message != NoUndoNote {
			t.Fatalf("%d: %v %+v", down, fx, h.Ready)
		}
	}
}

func TestAReplyIsWrittenAndSentOnce(t *testing.T) {
	h := readyHub(t, draft(5, 2), msg(4, 1))
	h, _ = apply(t, h, ReadyOpen{At: t0})
	// A draft has nobody to answer.
	h, _ = apply(t, h, ReadyKey{At: t0, Action: ReadyReplyKey})
	if h.Ready.Input != nil || h.Ready.Message != NoReplyNote {
		t.Fatalf("%+v", h.Ready)
	}
	h, _ = apply(t, h, ReadyKey{At: t0, Action: ReadyDown}, ReadyKey{At: t0, Action: ReadyReplyKey})
	if h.Ready.Input == nil || h.Ready.Input.Message != 4 || HubSurface(h).Kind != SurfaceReadyInput || !HubSurface(h).Typing() {
		t.Fatalf("%+v", h.Ready)
	}
	for _, c := range "Thanks!" {
		h, _ = apply(t, h, ReadyKey{At: t0, Action: ReadyChar, Text: string(c)})
	}
	h, _ = apply(t, h, ReadyKey{At: t0, Action: ReadyBackspace})
	if h.Ready.Input.Text != "Thanks" {
		t.Fatalf("typed %q", h.Ready.Input.Text)
	}
	h, fx := apply(t, h, ReadyKey{At: t0, Action: ReadyEnter})
	sent := effectsOf[SendReply](fx)
	if len(sent) != 1 || sent[0].Message != 4 || sent[0].Text != "Thanks" || h.Ready.Input != nil || h.Ready.Sending != 4 {
		t.Fatalf("%v %+v", fx, h.Ready)
	}
	// While it is on its way a second reply waits.
	h, fx = apply(t, h, ReadyKey{At: t0, Action: ReadyReplyKey})
	if h.Ready.Input != nil || h.Ready.Message != ReplyBusyNote || len(effectsOf[SendReply](fx)) != 0 {
		t.Fatalf("%+v", h.Ready)
	}
	h, _ = apply(t, h, ReplyDone{At: t0, Message: 4})
	if h.Ready.Sending != 0 || h.Ready.Message != ReplySentNote {
		t.Errorf("%+v", h.Ready)
	}
}

func TestABlankReplyIsNotSentAndEscCancelsIt(t *testing.T) {
	h := readyHub(t, msg(4, 1))
	h, _ = apply(t, h, ReadyOpen{At: t0}, ReadyKey{At: t0, Action: ReadyReplyKey}) // enter writes
	h, fx := apply(t, h, ReadyKey{At: t0, Action: ReadyChar, Text: "  "}, ReadyKey{At: t0, Action: ReadyEnter})
	if len(effectsOf[SendReply](fx)) != 0 || h.Ready.Input == nil {
		t.Fatalf("a blank reply went: %v %+v", fx, h.Ready)
	}
	h, _ = apply(t, h, ReadyKey{At: t0, Action: ReadyEsc})
	if h.Ready.Input != nil || !h.Ready.Open {
		t.Errorf("esc should cancel the reply and keep the view: %+v", h.Ready)
	}
}

func TestARefusedReplyComesBackToTheOperator(t *testing.T) {
	h := readyHub(t, msg(4, 1))
	h, _ = apply(t, h, ReadyOpen{At: t0}, ReadyKey{At: t0, Action: ReadyReplyKey},
		ReadyKey{At: t0, Action: ReadyChar, Text: "Hello"}, ReadyKey{At: t0, Action: ReadyEnter})
	h, _ = apply(t, h, ReplyDone{At: t0, Message: 4, Err: "The one who told you that is no longer here."})
	if h.Ready.Input == nil || h.Ready.Input.Text != "Hello" || h.Ready.Message != ReplyRefusedNote ||
		h.Ready.Failure == "" || h.Ready.Sending != 0 {
		t.Fatalf("%+v", h.Ready)
	}
}

func TestAPasteIntoTheReplyGoesNowhereAndSaysSo(t *testing.T) {
	h := readyHub(t, msg(4, 1))
	h, _ = apply(t, h, ReadyOpen{At: t0}, ReadyKey{At: t0, Action: ReadyReplyKey}, Unbound{At: t0})
	if h.Notice != EditorPasteNote || h.Ready.Input.Text != "" {
		t.Errorf("%q %+v", h.Notice, h.Ready.Input)
	}
}

func TestReadyLoadFailuresAreSaid(t *testing.T) {
	h, _ := apply(t, NewHub(""), HubConnected{At: t0}, ReadyLoaded{At: t0, Err: "boom"})
	if h.Ready.Failure != "boom" || h.Ready.Asked {
		t.Errorf("%+v", h.Ready)
	}
	h, _ = apply(t, h, ReadyLoaded{At: t0, Unavailable: true})
	if !h.Ready.Unavailable {
		t.Errorf("%+v", h.Ready)
	}
}

// Whatever events arrive in whatever order, the view holds each id once,
// newest first, within its bound, with a selection that is one of its
// items, and the surface is always one the reducer can leave.
func TestReadyInvariantsHoldForAnyRun(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		h := readyHub(t)
		n := rapid.IntRange(1, 60).Draw(t, "n")
		for i := 0; i < n; i++ {
			var e Event
			item := func() ReadyItem {
				id := rapid.Int64Range(1, 8).Draw(t, "id")
				m := rapid.IntRange(0, 5).Draw(t, "min")
				switch rapid.IntRange(0, 2).Draw(t, "kind") {
				case 0:
					it := msg(id, m)
					it.Seq = rapid.Int64Range(0, 3).Draw(t, "seq")
					return it
				case 1:
					return moved(id, m, rapid.SampledFrom([]int64{0, id}).Draw(t, "undo"))
				}
				return draft(id, m)
			}
			switch rapid.IntRange(0, 7).Draw(t, "ev") {
			case 0:
				e = ReadyLoaded{At: t0, Rev: int64(i), Items: rapid.SliceOfN(rapid.Custom(func(t *rapid.T) ReadyItem { return item() }), 0, 6).Draw(t, "items")}
			case 1:
				e = ReadyChanged{At: t0, Rev: int64(i), Items: rapid.SliceOfN(rapid.Custom(func(t *rapid.T) ReadyItem { return item() }), 0, 4).Draw(t, "items"),
					Gone: []string{fmt.Sprintf("message:%d", rapid.Int64Range(1, 8).Draw(t, "g"))}}
			case 2:
				e = ReadyOpen{At: t0}
			case 3:
				e = ReadyKey{At: t0, Action: ReadyAction(rapid.IntRange(0, int(ReadyUndoKey)).Draw(t, "a")), Text: "x"}
			case 4:
				e = ReplyDone{At: t0, Message: rapid.Int64Range(1, 8).Draw(t, "m"), Err: rapid.SampledFrom([]string{"", "no"}).Draw(t, "err")}
			case 5:
				e = Move{At: t0, Delta: rapid.SampledFrom([]int{-1, 1}).Draw(t, "d")}
			case 6:
				e = UndoDone{At: t0, Move: rapid.Int64Range(1, 8).Draw(t, "u"), Err: rapid.SampledFrom([]string{"", "no"}).Draw(t, "uerr")}
			default:
				e = ListMove{At: t0, Delta: 1}
			}
			h, _ = ReduceHub(h, e)
			seen := map[string]bool{}
			for j, it := range h.Ready.Items {
				if seen[it.ID] {
					t.Fatalf("%q twice in %v", it.ID, ids(h.Ready))
				}
				seen[it.ID] = true
				if j > 0 && readyBefore(h.Ready.Items[j-1], it) > 0 {
					t.Fatalf("out of order: %v", ids(h.Ready))
				}
			}
			if len(h.Ready.Items) > maxReadyItems {
				t.Fatalf("%d items", len(h.Ready.Items))
			}
			if _, i := ReadySelected(h.Ready); (i < 0) != (len(h.Ready.Items) == 0) {
				t.Fatalf("selection %d with %d items", i, len(h.Ready.Items))
			}
			if h.Ready.Input != nil && !h.Ready.Open {
				t.Fatalf("a reply is open in a closed view: %+v", h.Ready)
			}
			if k := HubSurface(h).Kind; (k == SurfaceReady || k == SurfaceReadyInput) != h.Ready.Open {
				t.Fatalf("surface %d with open=%v", k, h.Ready.Open)
			}
		}
	})
}

// relay can send a change right after its answer, and the two reach the
// reducer in either order. An older load must not undo a newer change.
func TestAnOlderLoadDoesNotUndoANewerChange(t *testing.T) {
	h, _ := apply(t, NewHub(""), HubConnected{At: t0})
	replied := msg(4, 1)
	replied.Replies = []ReadyReply{{At: at(2), Text: "Thanks."}}
	h, _ = apply(t, h, ReadyChanged{At: t0, Rev: 50, Items: []ReadyItem{replied, draft(9, 3)}})
	h, fx := apply(t, h, ReadyLoaded{At: t0, Rev: 40, Items: []ReadyItem{msg(4, 1)}})
	if got, want := ids(h.Ready), []string{"prepared:9", "message:4"}; !slices.Equal(got, want) {
		t.Fatalf("%v want %v", got, want)
	}
	if len(h.Ready.Items[1].Replies) != 1 || h.Ready.Rev != 50 {
		t.Errorf("the reply marker was lost: %+v", h.Ready)
	}
	if len(effectsOf[RequestReady](fx)) != 1 {
		t.Errorf("the stale load should be asked for again: %v", fx)
	}
	// The answer to that second ask is current, and is taken.
	h, fx = apply(t, h, ReadyLoaded{At: t0, Rev: 51, Items: []ReadyItem{msg(4, 1)}})
	if !slices.Equal(ids(h.Ready), []string{"message:4"}) || len(effectsOf[RequestReady](fx)) != 0 {
		t.Errorf("%+v %v", h.Ready, fx)
	}
}

func TestARefusalStaysUntilTheNextKey(t *testing.T) {
	h := readyHub(t, msg(4, 1))
	h, _ = apply(t, h, ReadyOpen{At: t0}, ReadyKey{At: t0, Action: ReadyReplyKey},
		ReadyKey{At: t0, Action: ReadyChar, Text: "Hello"}, ReadyKey{At: t0, Action: ReadyEnter})
	h, _ = apply(t, h, ReplyDone{At: t0, Message: 4, Err: "A reply is at most 4000 characters."})
	h2, _ := apply(t, h, ReadyKey{At: t0, Action: ReadyEnter}) // sends it again; relay will refuse again
	if h2.Ready.Sending != 4 {
		t.Fatalf("%+v", h2.Ready)
	}
	h3, _ := apply(t, h, ReadyKey{At: t0, Action: ReadyChar, Text: "!"})
	if h3.Ready.Failure != "" || h3.Ready.Message != "" || h3.Ready.Input.Text != "Hello!" {
		t.Errorf("a key should clear the note and keep the text: %+v", h3.Ready)
	}
}

// A load at rev R holds every change up to R. A change computed against
// the old record can arrive after a newer answer, and must not undo it.
func TestAStaleChangeAfterALoadIsDropped(t *testing.T) {
	h, _ := apply(t, NewHub(""), HubConnected{At: t0})
	replied := msg(4, 1)
	replied.Replies = []ReadyReply{{At: at(2), Text: "Thanks."}}
	h, _ = apply(t, h, ReadyLoaded{At: t0, Rev: 60, Items: []ReadyItem{replied}})
	h, _ = apply(t, h, ReadyChanged{At: t0, Rev: 55, Items: []ReadyItem{msg(4, 1), draft(9, 3)}})
	if got, want := ids(h.Ready), []string{"message:4"}; !slices.Equal(got, want) {
		t.Fatalf("%v want %v", got, want)
	}
	if len(h.Ready.Items[0].Replies) != 1 {
		t.Errorf("the reply marker was lost: %+v", h.Ready)
	}
	// A change after the load is still applied.
	h, _ = apply(t, h, ReadyChanged{At: t0, Rev: 61, Items: []ReadyItem{draft(9, 3)}})
	if len(h.Ready.Items) != 2 {
		t.Errorf("%+v", h.Ready)
	}
}

// --- an item whole, and the keys on a daily note's numbered items

func note(id int64, min int) ReadyItem {
	it := msg(id, min)
	it.Title = "Daily note"
	it.Lines = []string{"1. A change", "2. Another change", "3. A suggestion", "Reply \"not N\"."}
	it.Acts = []ReadyAct{{N: 1, Op: OpRevert, Line: 0}, {N: 2, Op: OpRevert, Line: 1}, {N: 3, Op: OpDismiss, Line: 2}}
	return it
}

func openNote(t *testing.T) Hub {
	h := readyHub(t, note(7, 1), draft(5, 0))
	h, _ = apply(t, h, ReadyOpen{At: t0}, ReadyKey{At: t0, Action: ReadyOpenKey})
	if h.Ready.Detail != "message:7" || h.Ready.Pick != 0 {
		t.Fatalf("%+v", h.Ready)
	}
	return h
}

func TestEnterOpensTheItemAndAReplyHasItsOwnKey(t *testing.T) {
	h := readyHub(t, msg(4, 1))
	h, fx := apply(t, h, ReadyOpen{At: t0}, ReadyKey{At: t0, Action: ReadyEnter})
	if h.Ready.Detail != "message:4" || h.Ready.Input != nil || len(fx) > 1 {
		t.Fatalf("enter should open it whole, not start a reply: %+v", h.Ready)
	}
	h, _ = apply(t, h, ReadyKey{At: t0, Action: ReadyReplyKey})
	if h.Ready.Input == nil || h.Ready.Input.Message != 4 || HubSurface(h).Kind != SurfaceReadyInput {
		t.Fatalf("a replies from the open item: %+v", h.Ready)
	}
	h, _ = apply(t, h, ReadyKey{At: t0, Action: ReadyEsc}, ReadyKey{At: t0, Action: ReadyEsc})
	if h.Ready.Detail != "" || !h.Ready.Open {
		t.Errorf("esc goes back to the list first: %+v", h.Ready)
	}
	h, _ = apply(t, h, ReadyKey{At: t0, Action: ReadyEsc})
	if h.Ready.Open {
		t.Errorf("then closes the view: %+v", h.Ready)
	}
}

func TestScrollingAnOpenItemStaysInRange(t *testing.T) {
	h := openNote(t)
	k := func(a ReadyAction, top int) ReadyKey { return ReadyKey{At: t0, Action: a, Top: top, Max: 10, Page: 4} }
	for _, c := range []struct {
		a         ReadyAction
		top, want int
	}{{ReadyDown, 0, 1}, {ReadyUp, 0, 0}, {ReadyPageDown, 8, 10}, {ReadyPageUp, 2, 0}, {ReadyBottom, 3, 10}, {ReadyTop, 9, 0}} {
		h, _ = apply(t, h, k(c.a, c.top))
		if h.Ready.Top != c.want {
			t.Errorf("%v from %d: top %d want %d", c.a, c.top, h.Ready.Top, c.want)
		}
	}
}

func TestVRevertsAndDDismissesThePickedItem(t *testing.T) {
	h := openNote(t)
	h, fx := apply(t, h, ReadyKey{At: t0, Action: ReadyRevert})
	got := effectsOf[SendAct](fx)
	if len(got) != 1 || got[0] != (SendAct{Message: 7, N: 1, Op: OpRevert}) || h.Ready.Acting != 7 {
		t.Fatalf("%v %+v", fx, h.Ready)
	}
	// One at a time.
	h, fx = apply(t, h, ReadyKey{At: t0, Action: ReadyRevert})
	if len(effectsOf[SendAct](fx)) != 0 || h.Ready.Message != ActBusyNote {
		t.Errorf("%v %+v", fx, h.Ready)
	}
	h, _ = apply(t, h, ActDone{At: t0, Message: 7, N: 1, Op: OpRevert})
	if h.Ready.Acting != 0 || h.Ready.Message != "Item 1 undone." || h.Ready.Items[0].Acts[0].Done != "reverted" {
		t.Fatalf("%+v", h.Ready)
	}
	// Item 1 is done: v says so and sends nothing.
	_, fx = apply(t, h, ReadyKey{At: t0, Action: ReadyRevert})
	if len(effectsOf[SendAct](fx)) != 0 {
		t.Errorf("sent an item that was done: %v", fx)
	}
	// The number keys pick the note's own numbers.
	h, _ = apply(t, h, ReadyKey{At: t0, Action: ReadyPick, Text: "3"})
	if h.Ready.Pick != 2 {
		t.Fatalf("%+v", h.Ready)
	}
	h, fx = apply(t, h, ReadyKey{At: t0, Action: ReadyDismiss})
	if got := effectsOf[SendAct](fx); len(got) != 1 || got[0] != (SendAct{Message: 7, N: 3, Op: OpDismiss}) {
		t.Fatalf("%v", fx)
	}
	h, _ = apply(t, h, ActDone{At: t0, Message: 7, N: 3, Op: OpDismiss})
	if h.Ready.Message != "Item 3 dropped." || h.Ready.Items[0].Acts[2].Done != "dismissed" {
		t.Errorf("%+v", h.Ready)
	}
}

func TestTheWrongKeyForAnItemDoesNothingAndSaysWhich(t *testing.T) {
	h := openNote(t)
	h, fx := apply(t, h, ReadyKey{At: t0, Action: ReadyDismiss})
	if len(effectsOf[SendAct](fx)) != 0 || h.Ready.Message != "Item 1: press v to undo it." {
		t.Errorf("%v %q", fx, h.Ready.Message)
	}
	h, _ = apply(t, h, ReadyKey{At: t0, Action: ReadyPick, Text: "3"})
	h, fx = apply(t, h, ReadyKey{At: t0, Action: ReadyRevert})
	if len(effectsOf[SendAct](fx)) != 0 || h.Ready.Message != "Item 3: press d to drop it." {
		t.Errorf("%v %q", fx, h.Ready.Message)
	}
	h, _ = apply(t, h, ReadyKey{At: t0, Action: ReadyPick, Text: "9"})
	if h.Ready.Message != "There is no item 9 here." || h.Ready.Pick != 2 {
		t.Errorf("%q %d", h.Ready.Message, h.Ready.Pick)
	}
	// In the list, and on an item with nothing to act on, they say so.
	h = readyHub(t, msg(4, 1))
	h, _ = apply(t, h, ReadyOpen{At: t0}, ReadyKey{At: t0, Action: ReadyRevert})
	if h.Ready.Message != OpenNoteNote {
		t.Errorf("%q", h.Ready.Message)
	}
	h, _ = apply(t, h, ReadyKey{At: t0, Action: ReadyOpenKey}, ReadyKey{At: t0, Action: ReadyRevert})
	if h.Ready.Message != NoActNote {
		t.Errorf("%q", h.Ready.Message)
	}
}

func TestARefusedActSaysWhy(t *testing.T) {
	h := openNote(t)
	h, _ = apply(t, h, ReadyKey{At: t0, Action: ReadyRevert}, ActDone{At: t0, Message: 7, N: 1, Op: OpRevert, Err: "Item 1 was reverted already."})
	if h.Ready.Acting != 0 || h.Ready.Message != ActRefusedNote || h.Ready.Failure == "" || h.Ready.Items[0].Acts[0].Done != "" {
		t.Errorf("%+v", h.Ready)
	}
}

func TestAnyKeyClosesTheKeysListAndDoesOnlyThat(t *testing.T) {
	h := openNote(t)
	h, _ = apply(t, h, ReadyKey{At: t0, Action: ReadyHelp})
	if !h.Ready.Help {
		t.Fatal("? did nothing")
	}
	h, fx := apply(t, h, ReadyKey{At: t0, Action: ReadyRevert})
	if h.Ready.Help || len(effectsOf[SendAct](fx)) != 0 || h.Ready.Detail == "" {
		t.Errorf("%v %+v", fx, h.Ready)
	}
}

func TestTheUndoSummaryIsWhatTheStatusLineSays(t *testing.T) {
	m := ReadyItem{ID: "move:3", Kind: ReadyMove, At: at(1), Move: 3, Undo: 3}
	h := readyHub(t, m)
	h, _ = apply(t, h, ReadyOpen{At: t0}, ReadyKey{At: t0, Action: ReadyUndoKey},
		UndoDone{At: t0, Move: 3, Lines: []string{"Undid move 3: put back 3 of 3. Removed 2 empty folder(s) it had made."}, Summary: "Put back 3 files."})
	if h.Ready.Message != "Put back 3 files." {
		t.Errorf("%q", h.Ready.Message)
	}
}

func TestAnOpenItemThatGoesAwayClosesItself(t *testing.T) {
	h := openNote(t)
	h, _ = apply(t, h, ReadyChanged{At: t0, Rev: 11, Gone: []string{"message:7"}})
	if h.Ready.Detail != "" || !h.Ready.Open {
		t.Errorf("%+v", h.Ready)
	}
}

// Any run of keys on any items: the open item is always one that is
// there, the pick and the top stay in range, and an act is sent only for
// an item the note offers, with what it offered, and not twice.
func TestReadyKeysKeepTheirInvariants(t *testing.T) {
	actions := []ReadyAction{ReadyUp, ReadyDown, ReadyTop, ReadyBottom, ReadyOpenKey, ReadyEsc, ReadyPageUp, ReadyPageDown,
		ReadyNext, ReadyPrev, ReadyPick, ReadyRevert, ReadyDismiss, ReadyHelp, ReadyReplyKey, ReadyUndoKey, ReadyRefresh}
	rapid.Check(t, func(t *rapid.T) {
		items := []ReadyItem{note(7, 1), msg(4, 2), draft(5, 0)}
		items[0].Acts[rapid.IntRange(0, 2).Draw(t, "done")].Done = rapid.SampledFrom([]string{"", "reverted", "dismissed"}).Draw(t, "how")
		h := readyHub(t, items...)
		h, _ = apply(t, h, ReadyOpen{At: t0})
		for i := 0; i < rapid.IntRange(1, 40).Draw(t, "n"); i++ {
			a := rapid.SampledFrom(actions).Draw(t, "action")
			k := ReadyKey{At: t0, Action: a, Top: rapid.IntRange(0, 12).Draw(t, "top"), Max: rapid.IntRange(0, 12).Draw(t, "max"),
				Page: rapid.IntRange(0, 6).Draw(t, "page"), Text: rapid.SampledFrom([]string{"1", "2", "3", "9", "x"}).Draw(t, "text")}
			var fx []Effect
			h, fx = apply(t, h, k)
			if h.Ready.Input != nil {
				h, _ = apply(t, h, ReadyKey{At: t0, Action: ReadyEsc})
			}
			for _, s := range effectsOf[SendAct](fx) {
				it := items[0]
				idx := slices.IndexFunc(it.Acts, func(x ReadyAct) bool { return x.N == s.N })
				if s.Message != 7 || idx < 0 || it.Acts[idx].Op != s.Op || it.Acts[idx].Done != "" {
					t.Fatalf("sent %+v for %+v", s, it.Acts)
				}
				h, _ = apply(t, h, ActDone{At: t0, Message: s.Message, N: s.N, Op: s.Op})
				items[0].Acts[idx].Done = "x"
			}
			if h.Ready.Detail != "" {
				if _, i := ReadyDetail(h.Ready); i < 0 {
					t.Fatalf("the open item is not there: %+v", h.Ready)
				}
			}
			if h.Ready.Top < 0 || h.Ready.Top > 12 || h.Ready.Pick < 0 || h.Ready.Pick > 2 {
				t.Fatalf("out of range: %+v", h.Ready)
			}
		}
	})
}
