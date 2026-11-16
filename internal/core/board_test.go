package core

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"testing"

	"pgregory.net/rapid"

	"github.com/kirahowe/relay-tui/internal/a2ui"
	"github.com/kirahowe/relay-tui/internal/a2ui/a2uitest"
)

func txt(s string, rev int64) CardText { return CardText{Text: s, Rev: rev} }

func checklist(id, rev int64, done ...bool) Card {
	c := Card{ID: id, Kind: CardChecklist, Status: CardActive, Rev: rev, Owner: "hub", Title: txt("Steps", id)}
	for i, d := range done {
		c.Items = append(c.Items, CardElem{ID: i + 1, CardText: txt("step", id), Done: d})
		c.Total++
		if d {
			c.Done++
		}
	}
	return c
}

func tracker(id, rev int64, cells ...Cell) Card {
	return Card{ID: id, Kind: CardTracker, Status: CardActive, Rev: rev, Title: txt("Accounts", id),
		Rows:    []CardElem{{ID: 1, CardText: txt("BMO", id)}, {ID: 2, CardText: txt("RBC", id)}},
		Columns: []CardElem{{ID: 1, CardText: txt("Jun", id)}, {ID: 2, CardText: txt("Jul", id)}},
		Cells:   cells}
}

// onBoard is a session with the pane focused on a loaded board.
func onBoard(cards ...Card) Single {
	s, _ := feedS(Single{}, Connected{At: t0}, key(t0, PaneToggle),
		CardsLoaded{At: t0, Rev: 100, Cards: cards})
	return s
}

func boardKeys(s Single, actions ...PaneAction) (Single, []Effect) {
	var evs []Event
	for _, a := range actions {
		evs = append(evs, key(t0, a))
	}
	return feedS(s, evs...)
}

func typeText(s Single, text string) Single {
	for _, r := range text {
		s, _ = ReduceSingle(s, PaneKey{At: t0, Action: PaneChar, Text: string(r)})
	}
	return s
}

func TestTheBoardIsTheDefaultAndAskedForOnceOnConnect(t *testing.T) {
	s, fx := feedS(Single{}, Connected{At: t0})
	if s.Board.Items || len(effectsOf[RequestCards](fx)) != 1 {
		t.Fatalf("items %v, effects %v", s.Board.Items, fx)
	}
	s, fx = feedS(s, key(t0, PaneToggle), key(t0, PaneBoard), key(t0, PaneBoard))
	if s.Board.Items || len(effectsOf[RequestCards](fx)) != 0 {
		t.Fatalf("asked again on switching: %v", fx)
	}
	// Unfocused, the items stay the pane's view once chosen.
	s, _ = feedS(s, key(t0, PaneBoard), key(t0, PaneEsc))
	if s.PaneFocus || !s.Board.Items {
		t.Fatal("esc should leave the pane on the items")
	}
}

// onItems is a session whose board has loaded while the pane shows the
// workspace items, unfocused.
func onItems(cards ...Card) Single {
	s, _ := feedS(Single{}, Connected{At: t0}, CardsLoaded{At: t0, Rev: 100, Cards: cards},
		key(t0, PaneToggle), key(t0, PaneBoard), key(t0, PaneEsc))
	return s
}

func TestAChangedCardBringsTheBoardIntoView(t *testing.T) {
	s, _ := ReduceSingle(onItems(checklist(10, 10)), CardsChanged{At: t0, Rev: 101, Cards: []Card{tracker(20, 101)}})
	b := s.Board
	if b.Items || b.SelID != 20 || s.PaneFocus {
		t.Fatalf("open %v, selected %d, focus %v", !b.Items, b.SelID, s.PaneFocus)
	}
	s, _ = feedS(onItems(checklist(10, 10)), CardsChanged{At: t0, Rev: 101, Cards: []Card{checklist(10, 101, true)}})
	if s.Board.Items || s.Board.SelID != 10 {
		t.Fatal("a card that changed did not bring the board into view")
	}
}

func TestBoardWithoutTheMethod(t *testing.T) {
	s, _ := feedS(Single{}, Connected{At: t0}, key(t0, PaneToggle), CardsLoaded{At: t0, Unavailable: true})
	if !s.Board.Unavailable {
		t.Fatal("not unavailable")
	}
	_, fx := boardKeys(s, PaneRefresh, PaneBoard, PaneBoard)
	if len(effectsOf[RequestCards](fx)) != 0 {
		t.Fatal("asked again after method-not-found")
	}
}

func TestBoardLoadFailureAsksAgain(t *testing.T) {
	s, _ := feedS(Single{}, Connected{At: t0}, key(t0, PaneToggle), CardsLoaded{At: t0, Err: "boom"})
	if s.Board.Failure != "boom" {
		t.Fatalf("failure %q", s.Board.Failure)
	}
	_, fx := boardKeys(s, PaneBoard, PaneBoard)
	if len(effectsOf[RequestCards](fx)) != 1 {
		t.Fatal("reopening after a failure should ask again")
	}
}

func TestCardsChangedReplacesAndDropsArchived(t *testing.T) {
	s := onBoard(checklist(10, 10, false), checklist(20, 20, false))
	done := checklist(10, 30, true)
	gone := checklist(20, 31)
	gone.Status = CardArchived
	added := tracker(40, 40)
	s, _ = ReduceSingle(s, CardsChanged{At: t0, Rev: 40, Cards: []Card{done, gone, added}})
	b := s.Board
	if len(b.Cards) != 2 || !reflect.DeepEqual(b.Cards[0], done) || !reflect.DeepEqual(b.Cards[1], added) || b.Rev != 100 {
		t.Fatalf("cards %+v rev %d", b.Cards, b.Rev)
	}
	// An older copy of a card never replaces a newer one.
	s, _ = ReduceSingle(s, CardsChanged{At: t0, Rev: 200, Cards: []Card{checklist(10, 12, false)}})
	if !reflect.DeepEqual(s.Board.Cards[0], done) || s.Board.Rev != 200 {
		t.Fatal("a stale card replaced a newer one")
	}
}

func TestSnapshotKeepsANewerNotification(t *testing.T) {
	s, _ := feedS(Single{}, Connected{At: t0}, key(t0, PaneToggle),
		CardsChanged{At: t0, Rev: 50, Cards: []Card{checklist(10, 50, true)}},
		CardsLoaded{At: t0, Rev: 40, Cards: []Card{checklist(10, 40, false), checklist(20, 20)}})
	b := s.Board
	if len(b.Cards) != 2 || b.Cards[0].Rev != 50 || b.Rev != 50 {
		t.Fatalf("cards %+v rev %d", b.Cards, b.Rev)
	}
}

// Whatever order relay's cards arrive in, in notifications or answers,
// the board holds the newest copy of each card, and no card whose newest
// copy is archived: a late answer cannot bring an archived card back.
func TestBoardHoldsTheNewestOfEachCard(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := onBoard()
		newest := map[int64]Card{}
		// A rev fixes a copy's content, as it does in relay's log.
		copyAt := func(id, rev int64) Card {
			c := checklist(id, rev, rev%2 == 0)
			if rev%5 == 0 {
				c.Status = CardArchived
			}
			return c
		}
		for range rapid.IntRange(0, 30).Draw(t, "events") {
			var cards []Card
			for range rapid.IntRange(1, 3).Draw(t, "cards") {
				cards = append(cards, copyAt(rapid.Int64Range(1, 5).Draw(t, "id"), rapid.Int64Range(1, 60).Draw(t, "rev")))
			}
			var e Event = CardsChanged{At: t0, Cards: cards}
			if rapid.Bool().Draw(t, "answer") {
				cards = cards[:1]
				e = CardOpDone{At: t0, CardID: cards[0].ID, Card: &cards[0]}
			}
			for _, c := range cards {
				if old, ok := newest[c.ID]; !ok || c.Rev > old.Rev {
					newest[c.ID] = c
				}
			}
			s, _ = ReduceSingle(s, e)
		}
		want := map[int64]Card{}
		for id, c := range newest {
			if c.Status == CardActive {
				want[id] = c
			}
		}
		got := map[int64]Card{}
		for _, c := range s.Board.Cards {
			if _, dup := got[c.ID]; dup {
				t.Fatalf("card %d twice", c.ID)
			}
			got[c.ID] = c
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("board %v, want %v", got, want)
		}
	})
}

func TestTickSendsAnOpAndWaits(t *testing.T) {
	s := onBoard(checklist(10, 10, false, true))
	s, fx := boardKeys(s, PaneEnter, PaneDown, PaneMark)
	ops := effectsOf[SendCardOp](fx)
	if len(ops) != 1 || ops[0].Card != 10 || ops[0].Op.Op != "tick" || *ops[0].Op.Item != 1 || ops[0].Op.Seen != nil {
		t.Fatalf("ops %+v", ops)
	}
	b := s.Board
	if b.Sending != 10 || b.Cards[0].Items[0].Done {
		t.Fatalf("sending %d, item done %v: no optimistic change", b.Sending, b.Cards[0].Items[0].Done)
	}
	// A second op waits for the first.
	s, fx = boardKeys(s, PaneDown, PaneMark)
	if len(effectsOf[SendCardOp](fx)) != 0 || s.Board.Message == "" {
		t.Fatalf("sent while sending: %v", fx)
	}
	after := checklist(10, 11, true, true)
	s, _ = ReduceSingle(s, CardOpDone{At: t0, CardID: 10, Card: &after})
	if s.Board.Sending != 0 || !s.Board.Cards[0].Items[0].Done {
		t.Fatal("the answer did not replace the card")
	}
	_, fx = boardKeys(s, PaneMark)
	if ops := effectsOf[SendCardOp](fx); len(ops) != 1 || ops[0].Op.Op != "untick" || *ops[0].Op.Item != 2 {
		t.Fatalf("ops %+v", ops)
	}
}

func TestRefusalIsShown(t *testing.T) {
	s := onBoard(checklist(10, 10, false))
	s, _ = boardKeys(s, PaneEnter, PaneDown, PaneMark)
	s, _ = ReduceSingle(s, CardOpDone{At: t0, CardID: 10, Err: "Card 10 is archived."})
	b := s.Board
	if b.Message != "Refused." || b.Failure != "Card 10 is archived." || b.Sending != 0 {
		t.Fatalf("%+v", b)
	}
}

func TestUnchangedOpSaysSo(t *testing.T) {
	s := onBoard(checklist(10, 10, true))
	s, _ = boardKeys(s, PaneEnter, PaneDown, PaneMark)
	same := checklist(10, 10, true)
	s, _ = ReduceSingle(s, CardOpDone{At: t0, CardID: 10, Card: &same, Unchanged: true})
	b := s.Board
	if b.Message != "Nothing to change." || b.Sending != 0 || b.Failure != "" {
		t.Fatalf("%+v", b)
	}
}

// A notification can reach the client before the answer to _relay/cards
// it follows, carrying a card the answer predates. The card stays.
func TestNotificationBeforeTheAnswerKeepsANewCard(t *testing.T) {
	s, _ := feedS(Single{}, Connected{At: t0}, key(t0, PaneToggle),
		CardsChanged{At: t0, Rev: 101, Cards: []Card{checklist(101, 101)}},
		CardsLoaded{At: t0, Rev: 100, Cards: []Card{checklist(50, 60)}})
	var ids []int64
	for _, c := range s.Board.Cards {
		ids = append(ids, c.ID)
	}
	if !slices.Equal(ids, []int64{50, 101}) {
		t.Fatalf("cards %v", ids)
	}
	// A card held from an older answer that relay no longer lists goes.
	s, _ = ReduceSingle(s, CardsLoaded{At: t0, Rev: 120, Cards: []Card{checklist(101, 101)}})
	if len(s.Board.Cards) != 1 || s.Board.Cards[0].ID != 101 {
		t.Fatalf("%+v", s.Board.Cards)
	}
	// A notification that archived a card before the answer keeps it off.
	gone := checklist(101, 130)
	gone.Status = CardArchived
	s, _ = ReduceSingle(s, CardsChanged{At: t0, Rev: 130, Cards: []Card{gone}})
	s, _ = ReduceSingle(s, CardsLoaded{At: t0, Rev: 125, Cards: []Card{checklist(101, 101)}})
	if len(s.Board.Cards) != 0 {
		t.Fatalf("%+v", s.Board.Cards)
	}
}

func TestTrackerCellCycles(t *testing.T) {
	s := onBoard(tracker(30, 30))
	s, _ = boardKeys(s, PaneEnter, PaneDown, PaneRight)
	var states []string
	for _, cell := range []Cell{{}, {Row: 1, Column: 2, State: CellDone}, {Row: 1, Column: 2, State: CellSkip}} {
		c := tracker(30, 31+int64(len(states)), cell)
		s, _ = ReduceSingle(s, CardsChanged{At: t0, Cards: []Card{c}})
		var fx []Effect
		s, fx = boardKeys(s, PaneMark)
		op := effectsOf[SendCardOp](fx)[0].Op
		if op.Op != "set" || *op.Row != 1 || *op.Column != 2 {
			t.Fatalf("op %+v", op)
		}
		states = append(states, *op.State)
		s, _ = ReduceSingle(s, CardOpDone{At: t0, CardID: 30, Card: &c})
	}
	if !slices.Equal(states, []string{CellDone, CellSkip, CellOpen}) {
		t.Fatalf("states %q", states)
	}
}

func TestAddSendsTheTypedText(t *testing.T) {
	s := onBoard(tracker(30, 30))
	s, _ = boardKeys(s, PaneEnter, PaneAddColumn)
	if in := s.Board.Input; in == nil || in.Op.Op != "add_column" {
		t.Fatalf("input %+v", s.Board.Input)
	}
	s = typeText(s, "Aug\x1b​")
	s, fx := boardKeys(s, PaneBackspace, PaneEnter)
	ops := effectsOf[SendCardOp](fx)
	if len(ops) != 1 || ops[0].Op.Op != "add_column" || *ops[0].Op.Text != "Au" || ops[0].Op.Seen != nil {
		t.Fatalf("ops %+v", ops)
	}
	if s.Board.Input != nil {
		t.Fatal("input still open")
	}
}

// An edit carries the rev the operator saw when the input opened, not
// the rev of a change that arrived while they typed: relay then refuses
// it rather than overwrite the newer text.
func TestEditCarriesTheRevItStartedFrom(t *testing.T) {
	s := onBoard(checklist(10, 15, false))
	s, _ = boardKeys(s, PaneEnter, PaneDown, PaneEdit)
	in := s.Board.Input
	if in == nil || in.Text != "step" || in.Op.Op != "edit" || *in.Op.Item != 1 {
		t.Fatalf("input %+v", in)
	}
	newer := checklist(10, 22, false)
	newer.Items[0].Text, newer.Items[0].Rev = "step, changed elsewhere", 22
	s, _ = ReduceSingle(s, CardsChanged{At: t0, Rev: 22, Cards: []Card{newer}})
	s = typeText(s, " one")
	_, fx := boardKeys(s, PaneEnter)
	ops := effectsOf[SendCardOp](fx)
	if len(ops) != 1 || *ops[0].Op.Seen != 15 || *ops[0].Op.Text != "step one" {
		t.Fatalf("ops %+v", ops)
	}
}

func TestEditOfOutsideTextStartsEmpty(t *testing.T) {
	c := checklist(10, 10, false)
	c.Title.Outsider = true
	s := onBoard(c)
	s, _ = boardKeys(s, PaneEnter, PaneEdit)
	if in := s.Board.Input; in == nil || in.Text != "" || in.Op.Op != "retitle" {
		t.Fatalf("input %+v", in)
	}
	// Enter on an empty input still sends: relay says what is wrong.
	_, fx := boardKeys(s, PaneEnter)
	if len(effectsOf[SendCardOp](fx)) != 1 {
		t.Fatal("not sent")
	}
}

func TestUnchangedEditIsNotSent(t *testing.T) {
	s := onBoard(checklist(10, 10, false))
	s, fx := boardKeys(s, PaneEnter, PaneEdit, PaneEnter)
	if len(effectsOf[SendCardOp](fx)) != 0 || s.Board.Message != "Unchanged." {
		t.Fatalf("%v %q", fx, s.Board.Message)
	}
}

func TestArchiveNeedsConfirming(t *testing.T) {
	s := onBoard(checklist(10, 10), checklist(20, 20))
	s, fx := boardKeys(s, PaneDown, PaneArchive, PaneEsc)
	if len(effectsOf[SendCardOp](fx)) != 0 || s.Board.Message != "Archive cancelled." {
		t.Fatal("archived without a y")
	}
	// y right away does nothing: the question is armed ArmDelay after
	// it was drawn.
	s, fx = boardKeys(s, PaneArchive, PaneYes)
	if len(effectsOf[SendCardOp](fx)) != 0 || s.Board.Confirm == 0 {
		t.Fatalf("archived before the question was armed: %v", fx)
	}
	s, fx = feedS(s, key(t0.Add(ArmDelay), PaneYes))
	ops := effectsOf[SendCardOp](fx)
	if len(ops) != 1 || ops[0].Card != 20 || ops[0].Op.Op != "archive" {
		t.Fatalf("ops %+v", ops)
	}
	gone := checklist(20, 21)
	gone.Status = CardArchived
	s, _ = ReduceSingle(s, CardOpDone{At: t0, CardID: 20, Card: &gone})
	if len(s.Board.Cards) != 1 || s.Board.SelID != 10 {
		t.Fatalf("cards %+v sel %d", s.Board.Cards, s.Board.SelID)
	}
}

// An open card leaving the board closes it, unless the operator is
// typing in its editor: then the editor stays, with its text, says the
// card has gone, and enter sends nothing (Codex round 3, finding 2).
func TestOpenCardLeavingTheBoardClosesIt(t *testing.T) {
	s := onBoard(checklist(10, 10, false))
	s, _ = boardKeys(s, PaneEnter, PaneDown)
	gone := checklist(10, 11)
	gone.Status = CardArchived
	g, _ := ReduceSingle(s, CardsChanged{At: t0, Cards: []Card{gone}})
	if b := g.Board; b.Detail || b.Input != nil || len(b.Cards) != 0 || b.Message == "" {
		t.Fatalf("%+v", b)
	}
	s, _ = feedS(s, KeyPressed{At: t0}, key(t0, PaneEdit))
	s = typeText(s, "happy")
	s, _ = ReduceSingle(s, CardsChanged{At: t0, Cards: []Card{gone}})
	b := s.Board
	if SingleSurface(s).Kind != SurfaceEditor || b.Input == nil || b.Input.Text != "stephappy" || b.Message != editorGoneNote(10) {
		t.Fatalf("surface %+v board %+v", SingleSurface(s), b)
	}
	s, fx := feedS(s, key(t0, PaneEnter))
	if len(effectsOf[SendCardOp](fx)) != 0 || s.Board.Input == nil {
		t.Fatalf("enter sent to a card that has gone: %v", fx)
	}
	if s, _ = feedS(s, key(t0, PaneEsc)); s.Board.Input != nil {
		t.Fatal("esc did not discard it")
	}
}

// Keys typed into the input never act on the board, and the board's keys
// do nothing while the pane is not focused.
func TestInputTakesEveryKey(t *testing.T) {
	s := onBoard(checklist(10, 10, false))
	s, _ = boardKeys(s, PaneEnter, PaneAdd)
	s, fx := boardKeys(s, PaneMark, PaneArchive, PaneDown, PaneBoard)
	if len(fx) != 0 || s.Board.Input == nil || s.Board.Items {
		t.Fatalf("a key escaped the input: %v", fx)
	}
	s, _ = boardKeys(s, PaneEsc, PaneEsc, PaneEsc)
	if s.PaneFocus {
		t.Fatal("esc should close the input, the card, then the pane")
	}
	_, fx = boardKeys(s, PaneMark, PaneArchive, PaneYes)
	if len(fx) != 0 {
		t.Fatalf("keys acted on an unfocused pane: %v", fx)
	}
}

func TestNoteOffersOnlyItsTitle(t *testing.T) {
	n := Card{ID: 50, Kind: CardNote, Status: CardActive, Rev: 50, Title: txt("Note", 50), Note: &CardText{Text: "a\nb"}}
	s := onBoard(n)
	s, fx := boardKeys(s, PaneEnter, PaneDown, PaneMark, PaneAdd)
	if len(fx) != 0 || s.Board.Input != nil {
		t.Fatalf("%v", fx)
	}
	s, _ = boardKeys(s, PaneEdit)
	if in := s.Board.Input; in == nil || in.Op.Op != "retitle" {
		t.Fatalf("input %+v", in)
	}
}

// Opened before the session exists, the board asks once it does.
func TestBoardOpenedBeforeConnectAsksOnConnect(t *testing.T) {
	s, fx := feedS(Single{}, key(t0, PaneToggle))
	if len(effectsOf[RequestCards](fx)) != 0 {
		t.Fatal("asked before the session exists")
	}
	_, fx = feedS(s, Connected{At: t0})
	if len(effectsOf[RequestCards](fx)) != 1 {
		t.Fatalf("not asked on connect: %v", fx)
	}
}

func TestArchivingAnOpenCardSaysSo(t *testing.T) {
	s := onBoard(checklist(10, 10))
	s, _ = boardKeys(s, PaneEnter, PaneArchive, PaneYes)
	gone := checklist(10, 11)
	gone.Status = CardArchived
	s, _ = ReduceSingle(s, CardOpDone{At: t0, CardID: 10, Card: &gone})
	if s.Board.Message != "Archived card 10." || s.Board.Detail {
		t.Fatalf("%+v", s.Board)
	}
}

// A refused op that carried text gives the operator back what they
// typed. A stale edit reopens with the rev now held, so Enter resends it
// on purpose.
func TestRefusalGivesTheTypedTextBack(t *testing.T) {
	s := onBoard(checklist(10, 15, false))
	s, _ = boardKeys(s, PaneEnter, PaneDown, PaneEdit)
	s = typeText(s, " mine")
	s, _ = boardKeys(s, PaneEnter)
	newer := checklist(10, 16, false)
	newer.Items[0].Text = "changed again"
	s, _ = ReduceSingle(s, CardsChanged{At: t0, Rev: 16, Cards: []Card{newer}})
	s, _ = ReduceSingle(s, CardOpDone{At: t0, CardID: 10, Err: "Item 1 was changed at rev 16, after rev 15."})
	b := s.Board
	in := b.Input
	if in == nil || in.Text != "step mine" || in.Op.Op != "edit" || *in.Op.Item != 1 || *in.Op.Seen != 16 || in.Op.Text != nil {
		t.Fatalf("input %+v", in)
	}
	if b.Message != "Refused." || b.Failure == "" {
		t.Fatalf("%+v", b)
	}
	_, fx := boardKeys(s, PaneEnter)
	ops := effectsOf[SendCardOp](fx)
	if len(ops) != 1 || *ops[0].Op.Seen != 16 || *ops[0].Op.Text != "step mine" {
		t.Fatalf("ops %+v", ops)
	}

	// An add refused for its length comes back too.
	s = onBoard(checklist(10, 10))
	s, _ = boardKeys(s, PaneEnter, PaneAdd)
	s = typeText(s, "yyy")
	s, _ = boardKeys(s, PaneEnter)
	s, _ = ReduceSingle(s, CardOpDone{At: t0, CardID: 10, Err: "An item is at most 200 characters."})
	if in := s.Board.Input; in == nil || in.Text != "yyy" || in.Op.Op != "add" || in.Op.Seen != nil {
		t.Fatalf("input %+v", in)
	}

	// A refused tick has no text to give back.
	s = onBoard(checklist(10, 10, false))
	s, _ = boardKeys(s, PaneEnter, PaneDown, PaneMark)
	s, _ = ReduceSingle(s, CardOpDone{At: t0, CardID: 10, Err: "no"})
	if s.Board.Input != nil {
		t.Fatal("input opened for a tick")
	}

	// An input the operator opened meanwhile is not replaced.
	s = onBoard(checklist(10, 10), checklist(20, 20))
	s, _ = boardKeys(s, PaneEnter, PaneAdd)
	s = typeText(s, "first")
	s, _ = boardKeys(s, PaneEnter, PaneEsc, PaneDown, PaneArchive)
	s, _ = ReduceSingle(s, CardOpDone{At: t0, CardID: 10, Err: "no"})
	if s.Board.Input != nil || s.Board.Confirm != 20 {
		t.Fatalf("%+v", s.Board)
	}
}

func TestBusyNoticeGoesWhenTheOpIsDone(t *testing.T) {
	s := onBoard(checklist(10, 10, false))
	s, _ = boardKeys(s, PaneEnter, PaneDown, PaneMark, PaneMark)
	if s.Board.Message == "" {
		t.Fatal("no busy notice")
	}
	after := checklist(10, 11, true)
	s, _ = ReduceSingle(s, CardOpDone{At: t0, CardID: 10, Card: &after})
	if s.Board.Message != "" {
		t.Fatalf("message %q after the op was done", s.Board.Message)
	}
}

func TestCtrlXKeepsTheTypedText(t *testing.T) {
	s := onBoard(checklist(10, 10))
	s, _ = boardKeys(s, PaneEnter, PaneAdd)
	s = typeText(s, "half")
	s, _ = boardKeys(s, PaneToggle, PaneToggle)
	if in := s.Board.Input; in == nil || in.Text != "half" {
		t.Fatalf("input %+v", in)
	}
}

// A notification that adds or removes an item above the cursor keeps the
// cursor on the item the operator chose, so x ticks that item.
func TestSelectionFollowsTheElementAcrossReplacements(t *testing.T) {
	s := onBoard(checklist(10, 10, false, false, false))
	s, _ = boardKeys(s, PaneEnter, PaneDown, PaneDown) // item 2
	fewer := checklist(10, 11, false, false, false)
	fewer.Items = fewer.Items[1:] // item 1 removed elsewhere
	s, _ = ReduceSingle(s, CardsChanged{At: t0, Rev: 11, Cards: []Card{fewer}})
	_, fx := boardKeys(s, PaneMark)
	if ops := effectsOf[SendCardOp](fx); len(ops) != 1 || *ops[0].Op.Item != 2 {
		t.Fatalf("after a removal above: ops %+v", ops)
	}
	if s.Board.Message != "" {
		t.Fatalf("message %q when nothing moved under the cursor", s.Board.Message)
	}
	more := checklist(10, 12, false, false, false)
	more.Items = append([]CardElem{{ID: 9, CardText: txt("new", 12)}}, more.Items[1:]...)
	s, _ = ReduceSingle(s, CardsChanged{At: t0, Rev: 12, Cards: []Card{more}})
	_, fx = boardKeys(s, PaneMark)
	if ops := effectsOf[SendCardOp](fx); len(ops) != 1 || *ops[0].Op.Item != 2 {
		t.Fatalf("after an insertion above: ops %+v", ops)
	}

	// The tracker's row and column follow too.
	s = onBoard(tracker(30, 30))
	s, _ = boardKeys(s, PaneEnter, PaneDown, PaneDown, PaneRight) // RBC × Jul
	moved := tracker(30, 31)
	moved.Rows = []CardElem{moved.Rows[1]}
	moved.Columns = []CardElem{{ID: 3, CardText: txt("May", 31)}, moved.Columns[0], moved.Columns[1]}
	s, _ = ReduceSingle(s, CardsChanged{At: t0, Rev: 31, Cards: []Card{moved}})
	_, fx = boardKeys(s, PaneMark)
	if ops := effectsOf[SendCardOp](fx); len(ops) != 1 || *ops[0].Op.Row != 2 || *ops[0].Op.Column != 2 {
		t.Fatalf("tracker ops %+v", ops)
	}
}

// When the selected item itself goes, the cursor keeps its place and a
// note says what happened under it.
func TestSelectionOnAGoneElementSaysSo(t *testing.T) {
	s := onBoard(checklist(10, 10, false, false, false))
	s, _ = boardKeys(s, PaneEnter, PaneDown, PaneDown) // item 2
	without := checklist(10, 11, false, false, false)
	without.Items = slices.Delete(without.Items, 1, 2)
	s, _ = ReduceSingle(s, CardsChanged{At: t0, Rev: 11, Cards: []Card{without}})
	b := s.Board
	if b.Line != 2 || b.Message == "" {
		t.Fatalf("line %d message %q", b.Line, b.Message)
	}
	// The last item going leaves the cursor on the one now last.
	s, _ = boardKeys(s, PaneDown) // item 3
	last := checklist(10, 12, false)
	s, _ = ReduceSingle(s, CardsChanged{At: t0, Rev: 12, Cards: []Card{last}})
	if b := s.Board; b.Line != 1 || b.Message == "" {
		t.Fatalf("line %d message %q", b.Line, b.Message)
	}
}

// A refusal stays only until relay takes the next op or the operator
// closes the input it reopened.
func TestRefusalGoesOnSuccessOrEsc(t *testing.T) {
	s := onBoard(checklist(10, 10))
	s, _ = boardKeys(s, PaneEnter, PaneAdd)
	s = typeText(s, "yyy")
	s, _ = boardKeys(s, PaneEnter)
	s, _ = ReduceSingle(s, CardOpDone{At: t0, CardID: 10, Err: "too long"})
	s, _ = boardKeys(s, PaneBackspace, PaneEnter)
	if s.Board.Message != "Refused." {
		t.Fatalf("message %q while the resend is on its way", s.Board.Message)
	}
	after := checklist(10, 11, false)
	s, _ = ReduceSingle(s, CardOpDone{At: t0, CardID: 10, Card: &after})
	if b := s.Board; b.Message != "" || b.Failure != "" {
		t.Fatalf("refusal stayed after success: %q %q", b.Message, b.Failure)
	}

	s, _ = boardKeys(s, PaneAdd)
	s = typeText(s, "zzz")
	s, _ = boardKeys(s, PaneEnter)
	s, _ = ReduceSingle(s, CardOpDone{At: t0, CardID: 10, Err: "too long"})
	if s.Board.Input == nil {
		t.Fatal("input not reopened")
	}
	s, _ = boardKeys(s, PaneEsc)
	if b := s.Board; b.Input != nil || b.Message != "" || b.Failure != "" {
		t.Fatalf("refusal stayed after esc: %+v", b)
	}
}

// A refusal goes when its card leaves the board under the input it
// reopened, and the board says the card has gone. Before any key the
// input closes; once the operator has typed, it stays, with its text.
func TestRefusalGoesWhenTheCardLeaves(t *testing.T) {
	s := onBoard(checklist(10, 10))
	s, _ = boardKeys(s, PaneEnter, PaneAdd)
	s = typeText(s, "yyy")
	s, _ = boardKeys(s, PaneEnter)
	s, _ = ReduceSingle(s, CardOpDone{At: t0, CardID: 10, Err: "too long"})
	gone := checklist(10, 20)
	gone.Status = CardArchived
	g, _ := ReduceSingle(s, CardsChanged{At: t0, Rev: 20, Cards: []Card{gone}})
	if b := g.Board; b.Input != nil || b.Message != "That card has left the board." || b.Failure != "" {
		t.Fatalf("%+v", b)
	}
	s, _ = feedS(s, KeyPressed{At: t0})
	s, _ = ReduceSingle(s, CardsChanged{At: t0, Rev: 20, Cards: []Card{gone}})
	if b := s.Board; b.Input == nil || b.Input.Text != "yyy" || b.Message != editorGoneNote(10) || b.Failure != "" {
		t.Fatalf("%+v", b)
	}
}

// From the list, a, A and E do nothing: they act on what only an open
// card shows. e retitles the selected card, as the list's hint says.
func TestListOffersOnlyRetitle(t *testing.T) {
	s := onBoard(tracker(30, 30))
	s, fx := boardKeys(s, PaneAdd, PaneAddColumn, PaneEditColumn)
	if len(fx) != 0 || s.Board.Input != nil {
		t.Fatalf("a list key opened %+v (%v)", s.Board.Input, fx)
	}
	s, _ = boardKeys(s, PaneEdit)
	if in := s.Board.Input; in == nil || in.Op.Op != "retitle" {
		t.Fatalf("input %+v", in)
	}
}

// The status line names the view the pane shows and counts it.
func TestStatusNamesThePaneView(t *testing.T) {
	s, _ := feedS(Single{}, Connected{At: t0}, CardsLoaded{At: t0, Rev: 1, Cards: []Card{checklist(1, 1)}})
	if l := Status(s.App, s.State, t0); l.PaneView != "board" || l.PaneItems != 1 {
		t.Fatalf("board: %q %d", l.PaneView, l.PaneItems)
	}
	s, _ = feedS(s, key(t0, PaneToggle), key(t0, PaneBoard))
	if l := Status(s.App, s.State, t0); l.PaneView != "workspace" || l.PaneItems != 0 {
		t.Fatalf("items: %q %d", l.PaneView, l.PaneItems)
	}
}

// relay's log as the board sees it: each rev changes one card, which may
// leave the board (archived) and does not come back.
type cardLog struct{ copies []Card }

func (l cardLog) newest(id, upto int64) (Card, bool) {
	var out Card
	ok := false
	for _, c := range l.copies {
		if c.ID == id && c.Rev <= upto {
			out, ok = c, true
		}
	}
	return out, ok
}

// snapshot is relay's _relay/cards answer at rev: every card active then,
// as it was then.
func (l cardLog) snapshot(rev int64) CardsLoaded {
	e := CardsLoaded{At: t0, Rev: rev, Cards: []Card{}}
	seen := map[int64]bool{}
	for _, c := range l.copies {
		if seen[c.ID] {
			continue
		}
		seen[c.ID] = true
		if n, ok := l.newest(c.ID, rev); ok && n.Status == CardActive {
			e.Cards = append(e.Cards, n)
		}
	}
	return e
}

// However answers to _relay/cards and _relay/cards/changed notifications
// interleave, late or out of order, the board never holds a copy older
// than one it was sent, never holds a card it has been told left (by its
// archived copy, or by an answer that left out a card it held), and once
// everything has arrived holds exactly what relay holds.
func TestBoardSnapshotsAndNotificationsInAnyOrder(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		var log cardLog
		gone := map[int64]bool{}
		var ids []int64
		n := rapid.IntRange(1, 25).Draw(t, "revs")
		for rev := int64(1); rev <= int64(n); rev++ {
			var id int64
			if len(ids) == 0 || rapid.IntRange(0, 3).Draw(t, "new") == 0 {
				id = int64(len(ids) + 1)
				ids = append(ids, id)
			} else {
				id = rapid.SampledFrom(ids).Draw(t, "id")
			}
			if gone[id] {
				continue
			}
			c := checklist(id, rev, rev%2 == 0)
			if rapid.IntRange(0, 4).Draw(t, "archive") == 0 {
				c.Status = CardArchived
				gone[id] = true
			}
			log.copies = append(log.copies, c)
		}
		// Every change is notified once; some snapshots are taken too.
		var msgs []Event
		for _, c := range log.copies {
			msgs = append(msgs, CardsChanged{At: t0, Rev: c.Rev, Cards: []Card{c}})
		}
		for range rapid.IntRange(0, 4).Draw(t, "snapshots") {
			msgs = append(msgs, log.snapshot(rapid.Int64Range(0, int64(n)).Draw(t, "at")))
		}
		order := rapid.Permutation(msgs).Draw(t, "order")

		s, _ := feedS(Single{}, Connected{At: t0})
		sent := map[int64]Card{}  // the newest copy of each card sent so far
		left := map[int64]int64{} // the rev by which a card was said to have left
		note := func(c Card) {
			if old, ok := sent[c.ID]; !ok || c.Rev > old.Rev {
				sent[c.ID] = c
			}
		}
		for _, e := range order {
			switch e := e.(type) {
			case CardsChanged:
				for _, c := range e.Cards {
					note(c)
				}
			case CardsLoaded:
				in := map[int64]bool{}
				for _, c := range e.Cards {
					note(c)
					in[c.ID] = true
				}
				// A card the board held that the answer leaves out had left
				// by then.
				for _, c := range s.Board.Cards {
					if !in[c.ID] && c.Rev <= e.Rev {
						left[c.ID] = max(left[c.ID], e.Rev)
					}
				}
			}
			s, _ = ReduceSingle(s, e)
			for _, c := range s.Board.Cards {
				if c.Rev < sent[c.ID].Rev {
					t.Fatalf("card %d at rev %d after rev %d was sent", c.ID, c.Rev, sent[c.ID].Rev)
				}
				if sent[c.ID].Status == CardArchived {
					t.Fatalf("card %d is on the board after it was archived", c.ID)
				}
				if r, ok := left[c.ID]; ok && c.Rev <= r {
					t.Fatalf("card %d at rev %d is on the board, though it was not on it at rev %d", c.ID, c.Rev, r)
				}
			}
		}
		want := map[int64]Card{}
		for _, c := range log.snapshot(int64(n)).Cards {
			want[c.ID] = c
		}
		got := map[int64]Card{}
		for _, c := range s.Board.Cards {
			got[c.ID] = c
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("board %v, want %v", got, want)
		}
	})
}

// A display brings the items back from the board, but leaves a pane the
// operator is working in alone.
func TestADisplayBringsTheItemsIntoView(t *testing.T) {
	s, _ := feedS(busySingle(), CardsLoaded{At: t0, Rev: 1}, displayEvent("d", "Venues", Labels{}))
	if !s.Board.Items {
		t.Error("the items did not come into view")
	}
	s, _ = feedS(busySingle(), CardsLoaded{At: t0, Rev: 1}, key(t0, PaneToggle), displayEvent("d", "Venues", Labels{}))
	if s.Board.Items {
		t.Error("a display took the focused pane")
	}
}

// However cards change, the board never takes the pane from the
// operator while they work in it.
func TestTheBoardNeverTakesThePane(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := onItems(checklist(10, 10))
		s, _ = feedS(s, key(t0, PaneToggle))
		for i := range rapid.IntRange(1, 8).Draw(t, "notifications") {
			c := checklist(rapid.Int64Range(10, 14).Draw(t, "id"), int64(101+i), rapid.Bool().Draw(t, "done"))
			if rapid.Bool().Draw(t, "archived") {
				c.Status = CardArchived
			}
			s, _ = ReduceSingle(s, CardsChanged{At: t0, Rev: int64(101 + i), Cards: []Card{c}})
			if !s.Board.Items {
				t.Fatalf("the board took the pane at notification %d", i)
			}
		}
	})
}

func TestTheBoardStaysOutOfTheWay(t *testing.T) {
	added := CardsChanged{At: t0, Rev: 101, Cards: []Card{tracker(20, 101)}}
	gone := checklist(10, 101)
	gone.Status = CardArchived
	focused, _ := feedS(onItems(), key(t0, PaneToggle))
	unloaded, _ := feedS(Single{}, Connected{At: t0}, key(t0, PaneToggle), key(t0, PaneBoard), key(t0, PaneEsc))
	for name, c := range map[string]struct {
		s Single
		e CardsChanged
	}{
		"while the operator works in the pane": {focused, added},
		"before the board has loaded":          {unloaded, added},
		"for a card that left the board":       {onItems(checklist(10, 10)), CardsChanged{At: t0, Rev: 101, Cards: []Card{gone}}},
		"for a copy already held":              {onItems(checklist(10, 10)), CardsChanged{At: t0, Rev: 101, Cards: []Card{checklist(10, 10)}}},
	} {
		s, _ := ReduceSingle(c.s, c.e)
		if !s.Board.Items {
			t.Errorf("%s: the board came into view", name)
		}
	}
}

// box is a CheckBox of a test surface. A box with no path is literal: its
// value is fixed, and the operator cannot change it.
type box struct {
	Path, Label string
	On          bool
}

// surfaceCard is a card of kind surface whose surface is a Column of
// boxes, as relay would send it. Any title line comes from the card.
func surfaceCard(t testing.TB, id, rev int64, boxes ...box) Card {
	t.Helper()
	comps := map[string]a2ui.Component{}
	var kids []any
	var data any = map[string]any{}
	for i, b := range boxes {
		cid := "box" + strconv.Itoa(i)
		kids = append(kids, cid)
		var value any = b.On
		if b.Path != "" {
			value = map[string]any{"@path": b.Path}
			var err error
			if data, err = a2ui.Set(data, b.Path, b.On); err != nil {
				t.Fatal(err)
			}
		}
		comps[cid] = a2ui.Component{ID: cid, Type: "CheckBox", Props: map[string]any{"label": b.Label, "value": value}}
	}
	if kids == nil {
		kids = []any{}
	}
	comps["root"] = a2ui.Component{ID: "root", Type: "Column", Props: map[string]any{"children": kids}}
	s := a2ui.Surface{ID: "card_" + strconv.FormatInt(id, 10), CatalogID: a2ui.RelayCatalogID, Components: comps, Data: data}
	if err := a2ui.Resolved(s); err != nil {
		t.Fatal(err)
	}
	return Card{ID: id, Kind: CardSurface, Status: CardActive, Rev: rev, Owner: "steward", Title: txt("Pack", id),
		Surface: &RoleSurface{A2UI: s}, Text: "Card text"}
}

func TestASurfaceCardHasALinePerInput(t *testing.T) {
	c := surfaceCard(t, 10, 10, box{"/a", "Tent", true}, box{"", "Fixed", true}, box{"/b", "Map", false})
	if !c.Native() || c.Lines() != 3 || len(c.Inputs()) != 2 {
		t.Fatalf("native %v, lines %d, inputs %v", c.Native(), c.Lines(), c.Inputs())
	}
	if ins := c.Inputs(); ins[0].Path != "/a" || ins[1].Label != "Map" || !ins[0].On || ins[1].On {
		t.Fatalf("inputs %+v", ins)
	}
	if (Card{Kind: CardSurface}).Native() || checklist(1, 1).Inputs() != nil {
		t.Fatal("a surface card with no surface is native, or a checklist has inputs")
	}
}

func TestTickingAnInputSendsTheInputOpAndWaits(t *testing.T) {
	s := onBoard(surfaceCard(t, 10, 12, box{"/a", "Tent", true}, box{"/b", "Map", false}))
	s, fx := boardKeys(s, PaneEnter, PaneTab, PaneTab, PaneMark)
	ops := effectsOf[SendCardOp](fx)
	if len(ops) != 1 || ops[0].Card != 10 {
		t.Fatalf("ops %+v", ops)
	}
	op := ops[0].Op
	if op.Op != "input" || op.Path == nil || *op.Path != "/b" || op.Value == nil || !*op.Value || op.Seen == nil || *op.Seen != 12 ||
		op.Item != nil || op.Text != nil {
		t.Fatalf("op %+v", op)
	}
	if s.Board.Sending != 10 || s.Board.Cards[0].Inputs()[1].On {
		t.Fatal("the board changed the value itself, or is not waiting")
	}
	// A key while the op is on its way is not sent, and says so.
	s, fx = boardKeys(s, PaneBackTab, PaneMark)
	if len(effectsOf[SendCardOp](fx)) != 0 || s.Board.Message != busyNotice {
		t.Fatalf("sent while sending: %v, message %q", fx, s.Board.Message)
	}
	after := surfaceCard(t, 10, 13, box{"/a", "Tent", true}, box{"/b", "Map", true})
	s, _ = ReduceSingle(s, CardOpDone{At: t0, CardID: 10, Card: &after})
	if s.Board.Sending != 0 || !s.Board.Cards[0].Inputs()[1].On {
		t.Fatal("relay's answer did not replace the card")
	}
	// The next tick is the opposite of what relay now holds.
	_, fx = boardKeys(s, PaneTab, PaneMark)
	if ops := effectsOf[SendCardOp](fx); len(ops) != 1 || *ops[0].Op.Path != "/b" || *ops[0].Op.Value || *ops[0].Op.Seen != 13 {
		t.Fatalf("ops %+v", ops)
	}
}

func TestARefusedInputShowsRelaysText(t *testing.T) {
	s := onBoard(surfaceCard(t, 10, 10, box{"/a", "Tent", false}))
	s, _ = boardKeys(s, PaneEnter, PaneTab, PaneMark)
	s, _ = ReduceSingle(s, CardOpDone{At: t0, CardID: 10, Err: "The card changed since you looked."})
	if b := s.Board; b.Message != refusedNotice || b.Failure != "The card changed since you looked." || b.Sending != 0 {
		t.Fatalf("%+v", b)
	}
}

func TestALiteralCheckBoxIsNotALine(t *testing.T) {
	s := onBoard(surfaceCard(t, 10, 10, box{"", "Fixed", true}))
	s, fx := boardKeys(s, PaneEnter, PaneTab, PaneTab, PaneMark)
	if s.Board.Line != 0 || len(effectsOf[SendCardOp](fx)) != 0 {
		t.Fatalf("line %d, effects %v", s.Board.Line, fx)
	}
}

func TestAddAndEditSendNothingOnAnInput(t *testing.T) {
	for _, a := range []PaneAction{PaneAdd, PaneAddColumn, PaneEdit, PaneEditColumn} {
		s := onBoard(surfaceCard(t, 10, 10, box{"/a", "Tent", false}))
		s, fx := boardKeys(s, PaneEnter, PaneTab, a)
		if s.Board.Input != nil || len(effectsOf[SendCardOp](fx)) != 0 {
			t.Errorf("action %v: input %+v, effects %v", a, s.Board.Input, fx)
		}
	}
}

func TestERetitlesASurfaceCardFromItsTitleLine(t *testing.T) {
	s := onBoard(surfaceCard(t, 10, 10, box{"/a", "Tent", false}))
	s, _ = boardKeys(s, PaneEnter, PaneEdit)
	if in := s.Board.Input; in == nil || in.Op.Op != "retitle" || in.Text != "Pack" {
		t.Fatalf("input %+v", in)
	}
	s, _ = boardKeys(onBoard(surfaceCard(t, 10, 10, box{"/a", "Tent", false})), PaneEnter, PaneArchive)
	if s.Board.Confirm != 10 {
		t.Fatal("archive did not ask")
	}
}

const boxesChanged = "The card's boxes changed; select one again."

func TestTheCursorStaysOnABoxWhileTheListOfBoxesIsTheSame(t *testing.T) {
	old := surfaceCard(t, 10, 10, box{"/i/0", "Tent", false}, box{"/i/1", "Map", false}, box{"/i/2", "Stove", false})
	onMap := func() Single {
		s := onBoard(old)
		s, _ = boardKeys(s, PaneEnter, PaneTab, PaneTab)
		return s
	}
	// Another client's tick changes values and leaves the list alone.
	s, _ := ReduceSingle(onMap(), CardsChanged{At: t0, Rev: 101, Cards: []Card{
		surfaceCard(t, 10, 11, box{"/i/0", "Tent", true}, box{"/i/1", "Map", true}, box{"/i/2", "Stove", false})}})
	if b := s.Board; b.Line != 2 || b.Message != "" {
		t.Fatalf("another client's tick: line %d, message %q", b.Line, b.Message)
	}
	// relay's answer to the operator's own input does too.
	s, fx := boardKeys(onMap(), PaneMark)
	if len(effectsOf[SendCardOp](fx)) != 1 {
		t.Fatal("no op sent")
	}
	answer := surfaceCard(t, 10, 11, box{"/i/0", "Tent", false}, box{"/i/1", "Map", true}, box{"/i/2", "Stove", false})
	s, _ = ReduceSingle(s, CardOpDone{At: t0, CardID: 10, Card: &answer})
	if b := s.Board; b.Line != 2 || b.Message != "" {
		t.Fatalf("own answer: line %d, message %q", b.Line, b.Message)
	}
	_, fx = boardKeys(s, PaneMark)
	if ops := effectsOf[SendCardOp](fx); len(ops) != 1 || *ops[0].Op.Path != "/i/1" {
		t.Fatalf("ops %+v", ops)
	}
}

// Any change to the list of boxes sends the cursor to the title, where x
// does nothing, and says so.
func TestAChangedListOfBoxesSendsTheCursorToTheTitle(t *testing.T) {
	two := func(rev int64) Card {
		return surfaceCard(t, 10, rev, box{"/a", "Done", false}, box{"/b", "Done", false})
	}
	for name, c := range map[string]Card{
		"the selected box is removed, the other has its label": surfaceCard(t, 10, 11, box{"/a", "Done", false}),
		"an item is inserted above":                            surfaceCard(t, 10, 11, box{"/z", "New", false}, box{"/a", "Done", false}, box{"/b", "Done", false}),
		"a box is relabelled":                                  surfaceCard(t, 10, 11, box{"/a", "Done", false}, box{"/b", "Finished", false}),
		"a box moves to another path":                          surfaceCard(t, 10, 11, box{"/a", "Done", false}, box{"/c", "Done", false}),
		"the boxes are swapped":                                surfaceCard(t, 10, 11, box{"/b", "Done", false}, box{"/a", "Done", false}),
		"all boxes are gone":                                   surfaceCard(t, 10, 11),
	} {
		t.Run(name, func(t *testing.T) {
			s := onBoard(two(10))
			s, _ = boardKeys(s, PaneEnter, PaneTab, PaneTab) // on /b
			s, _ = ReduceSingle(s, CardsChanged{At: t0, Rev: 101, Cards: []Card{c}})
			if b := s.Board; b.Line != 0 || b.Message != boxesChanged {
				t.Fatalf("line %d, message %q", b.Line, b.Message)
			}
			if _, fx := boardKeys(s, PaneMark); len(effectsOf[SendCardOp](fx)) != 0 {
				t.Fatalf("x sent %v", fx)
			}
		})
	}
}

// A refusal or a failed load still showing keeps the notice away, and
// the cursor still leaves the box, so x acts on nothing.
func TestAChangedListOfBoxesMovesTheCursorWhileAFailureShows(t *testing.T) {
	s := onBoard(surfaceCard(t, 10, 10, box{"/a", "Tent", false}, box{"/b", "Map", false}))
	s, _ = boardKeys(s, PaneEnter, PaneTab, PaneTab) // on Map
	s.Board.Message, s.Board.Failure = refusedNotice, "Card 10 changed."
	s, _ = ReduceSingle(s, CardsChanged{At: t0, Rev: 101, Cards: []Card{surfaceCard(t, 10, 11, box{"/a", "Tent", false})}})
	if b := s.Board; b.Line != 0 || b.Failure == "" || b.Message == boxesChanged {
		t.Fatalf("line %d, message %q, failure %q", b.Line, b.Message, b.Failure)
	}
	if _, fx := boardKeys(s, PaneMark); len(effectsOf[SendCardOp](fx)) != 0 {
		t.Fatalf("x sent %v", fx)
	}
}

// textBox is a row of a test surface: a Text, and a CheckBox beside it.
type textBox struct {
	Text string
	box
}

// captionedCard is a card of kind surface whose surface is a Column of a
// caption and a Row for each textBox, as relay would send it.
func captionedCard(t testing.TB, id, rev int64, caption string, rows ...textBox) Card {
	t.Helper()
	comps := map[string]a2ui.Component{
		"cap": {ID: "cap", Type: "Text", Props: map[string]any{"text": caption, "variant": "caption"}},
	}
	kids := []any{"cap"}
	var data any = map[string]any{}
	for i, r := range rows {
		n := strconv.Itoa(i)
		kids = append(kids, "row"+n)
		comps["row"+n] = a2ui.Component{ID: "row" + n, Type: "Row", Props: map[string]any{"children": []any{"t" + n, "b" + n}}}
		comps["t"+n] = a2ui.Component{ID: "t" + n, Type: "Text", Props: map[string]any{"text": r.Text}}
		var err error
		if data, err = a2ui.Set(data, r.Path, r.On); err != nil {
			t.Fatal(err)
		}
		comps["b"+n] = a2ui.Component{ID: "b" + n, Type: "CheckBox", Props: map[string]any{"label": r.Label, "value": map[string]any{"@path": r.Path}}}
	}
	comps["root"] = a2ui.Component{ID: "root", Type: "Column", Props: map[string]any{"children": kids}}
	s := a2ui.Surface{ID: "card_" + strconv.FormatInt(id, 10), CatalogID: a2ui.RelayCatalogID, Components: comps, Data: data}
	if err := a2ui.Resolved(s); err != nil {
		t.Fatal(err)
	}
	return Card{ID: id, Kind: CardSurface, Status: CardActive, Rev: rev, Title: txt("Pack", id), Surface: &RoleSurface{A2UI: s}}
}

func noOp(t *testing.T, s Single) {
	t.Helper()
	if _, fx := boardKeys(s, PaneMark); len(effectsOf[SendCardOp](fx)) != 0 {
		t.Fatalf("x sent %v", fx)
	}
}

// Two rows that differ only in their text, each with a box labelled the
// same: when a role swaps the text, the paths, labels and values are the
// same and the box under the cursor is another row's.
func TestTheCursorLeavesABoxWhenARoleEditsTheTextAroundIt(t *testing.T) {
	rows := func(a, b string) []textBox {
		return []textBox{{a, box{"/items/0/done", "Done", false}}, {b, box{"/items/1/done", "Done", false}}}
	}
	s := onBoard(captionedCard(t, 10, 10, "0 of 2 done", rows("Tent", "Stove")...))
	s, _ = boardKeys(s, PaneEnter, PaneTab) // Tent's box
	s, _ = ReduceSingle(s, CardsChanged{At: t0, Rev: 101, Cards: []Card{captionedCard(t, 10, 11, "0 of 2 done", rows("Stove", "Tent")...)}})
	if b := s.Board; b.Line != 0 || b.Message != boxesChanged {
		t.Fatalf("line %d, message %q", b.Line, b.Message)
	}
	noOp(t, s)
}

// A copy that is not a surface, then a surface with another box: the line
// the operator had stays gone.
func TestALineThatStoppedExistingDoesNotComeBack(t *testing.T) {
	s := onBoard(surfaceCard(t, 10, 10, box{"/a", "Tent", false}, box{"/b", "Map", false}))
	s, _ = boardKeys(s, PaneEnter, PaneTab)
	broken := Card{ID: 10, Kind: "surface?", Status: CardActive, Rev: 11, Title: txt("Pack", 10), Text: "Card 10"}
	s, _ = ReduceSingle(s, CardsChanged{At: t0, Rev: 101, Cards: []Card{broken}})
	if s.Board.Line != 0 {
		t.Fatalf("line %d on a card with one line", s.Board.Line)
	}
	s, _ = ReduceSingle(s, CardsChanged{At: t0, Rev: 102, Cards: []Card{surfaceCard(t, 10, 12, box{"/c", "Stove", false})}})
	if s.Board.Line != 0 {
		t.Fatalf("line %d came back", s.Board.Line)
	}
	noOp(t, s)
}

func TestTheCursorStaysWhenRelaysAnswerToTheOperatorsOwnTickChangesTheCaption(t *testing.T) {
	rows := func(on bool) []textBox {
		return []textBox{{"Tent", box{"/i/0", "Done", on}}, {"Stove", box{"/i/1", "Done", false}}}
	}
	s := onBoard(captionedCard(t, 10, 10, "0 of 2 done", rows(false)...))
	s, fx := boardKeys(s, PaneEnter, PaneTab, PaneMark)
	if len(effectsOf[SendCardOp](fx)) != 1 {
		t.Fatal("no op sent")
	}
	answer := captionedCard(t, 10, 11, "1 of 2 done", rows(true)...)
	s, _ = ReduceSingle(s, CardOpDone{At: t0, CardID: 10, Card: &answer})
	if b := s.Board; b.Line != 1 || b.Message != "" {
		t.Fatalf("line %d, message %q", b.Line, b.Message)
	}
	_, fx = boardKeys(s, PaneMark)
	if ops := effectsOf[SendCardOp](fx); len(ops) != 1 || *ops[0].Op.Path != "/i/0" || *ops[0].Op.Value {
		t.Fatalf("ops %+v", ops)
	}
}

func TestAnotherClientsTickThatChangesTheCaptionSendsTheCursorToTheTitle(t *testing.T) {
	rows := func(a, b bool) []textBox {
		return []textBox{{"Tent", box{"/i/0", "Done", a}}, {"Stove", box{"/i/1", "Done", b}}}
	}
	s := onBoard(captionedCard(t, 10, 10, "0 of 2 done", rows(false, false)...))
	s, _ = boardKeys(s, PaneEnter, PaneTab)
	s, _ = ReduceSingle(s, CardsChanged{At: t0, Rev: 101, Cards: []Card{captionedCard(t, 10, 11, "1 of 2 done", rows(false, true)...)}})
	if b := s.Board; b.Line != 0 || b.Message != boxesChanged {
		t.Fatalf("line %d, message %q", b.Line, b.Message)
	}
	noOp(t, s)
	// A change of values alone keeps the cursor.
	s = onBoard(captionedCard(t, 10, 10, "0 of 2 done", rows(false, false)...))
	s, _ = boardKeys(s, PaneEnter, PaneTab)
	s, _ = ReduceSingle(s, CardsChanged{At: t0, Rev: 101, Cards: []Card{captionedCard(t, 10, 11, "0 of 2 done", rows(false, true)...)}})
	if b := s.Board; b.Line != 1 || b.Message != "" {
		t.Fatalf("data only: line %d, message %q", b.Line, b.Message)
	}
}

// insertAbove is c with a new box drawn before every other, or with its
// first input taken out of the list that holds it. It is how relay's
// newer copy of a surface differs when a role adds or removes an item.
func insertAbove(c Card, remove bool, n int) Card {
	s := c.Surface.A2UI
	comps := maps.Clone(s.Components)
	removed := false
	if remove && len(s.Inputs()) > 0 {
		in := s.Inputs()[0].ID
		for id, k := range comps {
			kids, ok := k.Props["children"].([]any)
			if !ok || !slices.Contains(kids, any(in)) {
				continue
			}
			props := maps.Clone(k.Props)
			props["children"] = slices.DeleteFunc(slices.Clone(kids), func(x any) bool { return x == any(in) })
			comps[id] = a2ui.Component{ID: id, Type: k.Type, Props: props}
			removed = true
			break
		}
	}
	if !removed {
		key := "ins" + strconv.Itoa(n)
		oldRoot := "old_root" + strconv.Itoa(n)
		comps[key] = a2ui.Component{ID: key, Type: "CheckBox", Props: map[string]any{"label": "inserted", "value": map[string]any{"@path": "/" + key}}}
		old := comps["root"]
		old.ID = oldRoot
		comps[oldRoot] = old
		comps["root"] = a2ui.Component{ID: "root", Type: "Column", Props: map[string]any{"children": []any{key, oldRoot}}}
		s.Data, _ = a2ui.Set(s.Data, "/"+key, false)
	}
	s.Components = comps
	c.Surface = &RoleSurface{Outsider: c.Surface.Outsider, A2UI: s}
	return c
}

func onTitle(t *testing.T, s Single) {
	t.Helper()
	c, _ := BoardCard(s.Board)
	if got := BoardLine(s.Board, c); got != 0 {
		t.Fatalf("the cursor is on line %d, not the title", got)
	}
	noOp(t, s)
}

// The saved view holds a line number, and a box has no id: a role's edit
// while the client was closed moves which box has that line.
func TestASavedViewDoesNotPutTheCursorOnABoxNobodyChose(t *testing.T) {
	old := surfaceCard(t, 10, 10, box{"/i/0", "Tent", false}, box{"/i/1", "Map", false}, box{"/i/2", "Stove", false})
	s := onBoard(old)
	s, _ = boardKeys(s, PaneEnter, PaneTab, PaneTab) // Map
	v := SaveSingle(s)
	next := surfaceCard(t, 10, 11, box{"/i/0", "Passport", false}, box{"/i/1", "Tent", false}, box{"/i/2", "Map", false}, box{"/i/3", "Stove", false})
	x, _ := feedS(RestoreSingle(Single{}, v), Connected{At: t0}, CardsLoaded{At: t0, Rev: 101, Cards: []Card{next}})
	if !x.Board.Detail {
		t.Fatal("the card did not reopen")
	}
	onTitle(t, x)
}

// A text op refused after the operator has gone to another card puts the
// refused card's input back, and must not carry the other card's line or
// column, whatever kind either card is: the refused card opens on its
// title, where x does nothing.
func TestALateRefusalDoesNotCarryALineToAnotherCard(t *testing.T) {
	kinds := map[string]func(id int64) Card{
		"checklist": func(id int64) Card { return checklist(id, id, false, false, false) },
		"tracker":   func(id int64) Card { return tracker(id, id) },
		"surface": func(id int64) Card {
			return surfaceCard(t, id, id, box{"/b/0", "Tent", false}, box{"/b/1", "Map", false}, box{"/b/2", "Stove", false})
		},
	}
	for refused, a := range kinds {
		for other, b := range kinds {
			t.Run(refused+" refused while on a "+other, func(t *testing.T) {
				s := onBoard(a(10), b(11))
				s, _ = boardKeys(s, PaneEdit)
				s = typeText(s, "!")
				s, fx := boardKeys(s, PaneEnter)
				if ops := effectsOf[SendCardOp](fx); len(ops) != 1 || ops[0].Op.Op != "retitle" || ops[0].Card != 10 {
					t.Fatalf("no retitle went out: %+v", ops)
				}
				// While it is on its way: the other card, its second line
				// and, on a tracker, its second column.
				s, _ = boardKeys(s, PaneTab, PaneEnter, PaneTab, PaneTab, PaneRight)
				if c, _ := BoardCard(s.Board); c.ID != 11 || BoardLine(s.Board, c) != 2 {
					t.Fatalf("not on line 2 of card 11: card %d line %d", c.ID, BoardLine(s.Board, c))
				}
				s, _ = ReduceSingle(s, CardOpDone{At: t0, CardID: 10, Err: "Card 10 was changed at rev 12, after rev 10."})
				s, _ = boardKeys(s, PaneEsc) // closes the reopened input
				if c, _ := BoardCard(s.Board); c.ID != 10 {
					t.Fatalf("card %d is open, want the refused card 10", c.ID)
				}
				if s.Board.Line != 0 || s.Board.Col != 0 || s.Board.Box != nil {
					t.Fatalf("the board kept line %d, column %d, box %v from the other card", s.Board.Line, s.Board.Col, s.Board.Box != nil)
				}
				onTitle(t, s)
			})
		}
	}
}

// A refusal for the card the operator is still on leaves the cursor
// where they put it: that line was chosen on this card.
func TestARefusalOnTheOpenCardKeepsItsLine(t *testing.T) {
	s := onBoard(checklist(10, 10, false, false, false), checklist(11, 11, false, false, false))
	s, _ = boardKeys(s, PaneEnter, PaneTab, PaneTab, PaneEdit)
	s = typeText(s, "!")
	s, _ = boardKeys(s, PaneEnter)
	s, _ = ReduceSingle(s, CardOpDone{At: t0, CardID: 10, Err: "Card 10 was changed at rev 12, after rev 10."})
	s, _ = boardKeys(s, PaneEsc)
	if c, _ := BoardCard(s.Board); c.ID != 10 || BoardLine(s.Board, c) != 2 {
		t.Fatalf("card %d line %d, want card 10 line 2", c.ID, BoardLine(s.Board, c))
	}
}

func TestACardThatChangesKindKeepsNoLine(t *testing.T) {
	s := onBoard(checklist(10, 10, false, false, false))
	s, _ = boardKeys(s, PaneEnter, PaneTab, PaneTab)
	next := surfaceCard(t, 10, 11, box{"/a", "Pay rent", false}, box{"/b", "Cancel insurance", false}, box{"/c", "Sell car", false})
	s, _ = ReduceSingle(s, CardsChanged{At: t0, Rev: 101, Cards: []Card{next}})
	onTitle(t, s)
}

// editText is c with the text of one Text of its surface changed, or with
// a Text added above everything when it has none. No input changes.
func editText(c Card, n int) Card {
	s := c.Surface.A2UI
	comps := maps.Clone(s.Components)
	done := false
	for _, id := range slices.Sorted(maps.Keys(comps)) {
		if k := comps[id]; k.Type == "Text" {
			props := maps.Clone(k.Props)
			props["text"] = "edited " + strconv.Itoa(n)
			comps[id] = a2ui.Component{ID: id, Type: "Text", Props: props}
			done = true
			break
		}
	}
	if !done {
		key, oldRoot := "txt"+strconv.Itoa(n), "old_root"+strconv.Itoa(n)
		comps[key] = a2ui.Component{ID: key, Type: "Text", Props: map[string]any{"text": "added"}}
		old := comps["root"]
		old.ID = oldRoot
		comps[oldRoot] = old
		comps["root"] = a2ui.Component{ID: "root", Type: "Column", Props: map[string]any{"children": []any{key, oldRoot}}}
	}
	s.Components = comps
	c.Surface = &RoleSurface{Outsider: c.Surface.Outsider, A2UI: s}
	return c
}

// Over any surface and any run of keys, answers, role edits, malformed
// copies and recoveries, an op is only sent for the box the cursor is on,
// with the opposite value and the held rev, and only while the card's
// components are the ones the cursor arrived on by a key press, or the
// card has since been replaced only by relay's answers to the operator's
// own inputs that left the list of boxes as it was, or by copies with the
// components it held.
func TestSurfaceOpsAreForTheInputTheCursorIsOn(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		sc := a2uitest.GenResolved(t, rapid.String())
		sc.ID = "card_10"
		if len(sc.Inputs()) == 0 {
			t.Skip()
		}
		rev := int64(10)
		card := Card{ID: 10, Kind: CardSurface, Status: CardActive, Rev: rev, Title: txt("T", 10), Surface: &RoleSurface{A2UI: sc}}
		s := onBoard(card)
		s, _ = boardKeys(s, PaneEnter)
		lastValid := card
		var inFlight *CardOp
		// base is the components the cursor arrived on by a key press, and
		// clean says the card has only been replaced since by relay's
		// answers to the operator's own inputs with the same boxes.
		base := card.Surface.A2UI.Components
		clean := true
		replaced := func(before Card, own bool, next Card) {
			sameList := before.Kind == CardSurface && next.Kind == CardSurface && slices.Equal(before.inputKeys(), next.inputKeys())
			same := next.Surface != nil && before.Surface != nil && reflect.DeepEqual(before.Surface.A2UI.Components, next.Surface.A2UI.Components)
			if !(own && sameList) && !same {
				clean = false
			}
			if next.Kind == CardSurface {
				lastValid = next
			}
		}
		for i := range rapid.IntRange(1, 50).Draw(t, "steps") {
			held, _ := BoardCard(s.Board)
			line := BoardLine(s.Board, held)
			rev++
			switch step := rapid.SampledFrom([]string{"up", "down", "mark", "answer", "notify", "edit", "malformed", "recover"}).Draw(t, "step"); step {
			case "up", "down":
				a := PaneBackTab
				if step == "down" {
					a = PaneTab
				}
				s, _ = boardKeys(s, a)
				if now, _ := BoardCard(s.Board); now.Surface != nil {
					base, clean = now.Surface.A2UI.Components, true
				} else {
					base, clean = nil, false
				}
			case "mark":
				busy := s.Board.Sending != 0
				var fx []Effect
				s, fx = boardKeys(s, PaneMark)
				ops := effectsOf[SendCardOp](fx)
				if busy || line == 0 {
					if len(ops) != 0 {
						t.Fatalf("sent while busy %v or on the title %v", busy, line == 0)
					}
					break
				}
				if held.Inputs()[line-1].Button {
					// Every key of this run is at t0, so no press is armed.
					if len(ops) != 0 {
						t.Fatalf("a press went out on line %d with no ArmDelay", line)
					}
					break
				}
				if len(ops) != 1 {
					t.Fatalf("%d ops sent on input line %d", len(ops), line)
				}
				in, op := held.Inputs()[line-1], ops[0].Op
				if ops[0].Card != 10 || op.Op != "input" || op.Path == nil || *op.Path != in.Path ||
					op.Value == nil || *op.Value != !in.On || op.Seen == nil || *op.Seen != held.Rev {
					t.Fatalf("op %+v for input %+v of rev %d", op, in, held.Rev)
				}
				if !clean && !reflect.DeepEqual(held.Surface.A2UI.Components, base) {
					t.Fatalf("an op was sent for a box on a card whose components are not the ones the cursor arrived on")
				}
				inFlight = &op
			case "answer":
				if inFlight == nil || s.Board.Sending == 0 {
					break
				}
				next := lastValid
				next.Rev = rev
				data, err := a2ui.Set(lastValid.Surface.A2UI.Data, *inFlight.Path, *inFlight.Value)
				if err != nil {
					t.Fatal(err)
				}
				next.Surface = &RoleSurface{A2UI: lastValid.Surface.A2UI}
				next.Surface.A2UI.Data = data
				if rapid.Bool().Draw(t, "shift") {
					next = insertAbove(next, rapid.Bool().Draw(t, "remove"), i)
				}
				if rapid.Bool().Draw(t, "caption") {
					next = editText(next, i)
				}
				replaced(held, true, next)
				s, _ = ReduceSingle(s, CardOpDone{At: t0, CardID: 10, Card: &next})
				inFlight = nil
			case "notify":
				next := insertAbove(lastValid, rapid.Bool().Draw(t, "remove"), i)
				next.Rev = rev
				replaced(held, false, next)
				s, _ = ReduceSingle(s, CardsChanged{At: t0, Rev: rev, Cards: []Card{next}})
			case "edit":
				next := editText(lastValid, i)
				next.Rev = rev
				replaced(held, false, next)
				s, _ = ReduceSingle(s, CardsChanged{At: t0, Rev: rev, Cards: []Card{next}})
			case "malformed":
				next := Card{ID: 10, Kind: "surface?", Status: CardActive, Rev: rev, Title: txt("T", 10), Text: "Card 10"}
				replaced(held, false, next)
				s, _ = ReduceSingle(s, CardsChanged{At: t0, Rev: rev, Cards: []Card{next}})
			case "recover":
				next := lastValid
				next.Rev = rev
				if rapid.Bool().Draw(t, "different") {
					next = insertAbove(next, false, i)
				}
				replaced(held, false, next)
				s, _ = ReduceSingle(s, CardsChanged{At: t0, Rev: rev, Cards: []Card{next}})
			}
		}
	})
}

// srvDoc is the server's copy of a card of the model below.
type srvDoc struct {
	id, rev   int64
	caption   string
	names     []string
	done      []bool
	checklist bool // relay sent the card as a checklist
	broken    bool // relay sent it as a copy that is not a surface
}

func (d srvDoc) card(t testing.TB) Card {
	switch {
	case d.checklist:
		return checklist(d.id, d.rev, d.done...)
	case d.broken:
		return Card{ID: d.id, Kind: "surface?", Status: CardActive, Rev: d.rev, Title: txt("T", d.id), Text: "Card"}
	}
	var rows []textBox
	for i, n := range d.names {
		rows = append(rows, textBox{n, box{fmt.Sprintf("/items/%d/done", i), "Done", d.done[i]}})
	}
	return captionedCard(t, d.id, d.rev, d.caption, rows...)
}

// Whatever happens to the card under the cursor, an input op is only sent
// for a box the operator moved onto by a key press on the components now
// held, or that the cursor kept when relay's answer to their own input
// left the list of boxes as it was or a copy kept the components. The
// events are keys, role edits, other clients' ticks, relay's answers, a
// refused retitle that arrives after the operator opened another card, a
// card that changes kind, malformed copies, and a restart from the saved
// view after a role's edit.
func TestSurfaceCursorAcrossEveryPath(t *testing.T) {
	outer := t
	rapid.Check(t, func(t *rapid.T) {
		rev := int64(100)
		srv := map[int64]*srvDoc{}
		for _, id := range []int64{10, 11} {
			d := &srvDoc{id: id, rev: id, caption: "cap"}
			for i := range rapid.IntRange(1, 4).Draw(t, "rows") {
				d.names = append(d.names, fmt.Sprintf("n%d_%d", id, i))
				d.done = append(d.done, false)
			}
			srv[id] = d
		}
		cards := func() []Card { return []Card{srv[10].card(outer), srv[11].card(outer)} }
		s := onBoard(cards()...)

		// chosen is what the operator moved onto, by a key press.
		type choice struct {
			valid bool
			card  int64
			comps map[string]a2ui.Component
		}
		var chosen choice
		var flight *CardOp
		var flightCard int64

		held := func(id int64) (Card, bool) {
			i := slices.IndexFunc(s.Board.Cards, func(c Card) bool { return c.ID == id })
			if i < 0 {
				return Card{}, false
			}
			return s.Board.Cards[i], true
		}
		// apply feeds an event and moves chosen along with what relay
		// did to the card it names.
		apply := func(e Event, own bool) {
			before, had := held(chosen.card)
			s, _ = ReduceSingle(s, e)
			if !chosen.valid {
				return
			}
			after, has := held(chosen.card)
			switch {
			case !had || !has || before.Surface == nil || after.Surface == nil:
				chosen.valid = false
			case reflect.DeepEqual(before.Surface.A2UI.Components, after.Surface.A2UI.Components):
			case own && slices.Equal(before.inputKeys(), after.inputKeys()):
				chosen.comps = after.Surface.A2UI.Components
			default:
				chosen.valid = false
			}
		}
		bump := func(d *srvDoc) { rev++; d.rev = rev }
		pick := func(label string) *srvDoc {
			return srv[rapid.SampledFrom([]int64{10, 11}).Draw(t, label)]
		}
		steps := []string{"up", "down", "down", "enter", "esc", "mark", "mark", "mark", "role", "role", "tick", "answer", "retitle", "refuse", "kind", "malformed", "restart"}
		for range rapid.IntRange(10, 80).Draw(t, "events") {
			switch step := rapid.SampledFrom(steps).Draw(t, "step"); step {
			case "up", "down":
				if s.Board.Input != nil || s.Board.Confirm != 0 {
					break
				}
				a := PaneBackTab
				if step == "down" {
					a = PaneTab
				}
				s, _ = boardKeys(s, a)
				c, i := BoardCard(s.Board)
				chosen = choice{}
				if s.Board.Detail && i >= 0 && c.Surface != nil && BoardLine(s.Board, c) > 0 {
					chosen = choice{true, c.ID, c.Surface.A2UI.Components}
				}
			case "enter", "esc":
				if s.Board.Input != nil || s.Board.Confirm != 0 || (step == "esc" && !s.Board.Detail) {
					break
				}
				a := PaneEnter
				if step == "esc" {
					a = PaneEsc
				}
				wasDetail := s.Board.Detail
				s, _ = boardKeys(s, a)
				if !(step == "enter" && wasDetail) {
					chosen = choice{}
				}
			case "mark":
				if s.Board.Input != nil || s.Board.Confirm != 0 {
					break
				}
				c, i := BoardCard(s.Board)
				line := 0
				if i >= 0 {
					line = BoardLine(s.Board, c)
				}
				var fx []Effect
				s, fx = boardKeys(s, PaneMark)
				ops := effectsOf[SendCardOp](fx)
				if len(ops) == 0 {
					break
				}
				op := ops[0]
				if op.Op.Op != "input" {
					break
				}
				if line == 0 || c.Surface == nil {
					t.Fatalf("an input op %+v from the title or a card with no boxes", op)
				}
				in := c.Inputs()[line-1]
				if op.Card != c.ID || *op.Op.Path != in.Path || *op.Op.Value != !in.On || *op.Op.Seen != c.Rev {
					t.Fatalf("op %+v for box %+v of card %d rev %d", op, in, c.ID, c.Rev)
				}
				if !chosen.valid || chosen.card != c.ID || !reflect.DeepEqual(chosen.comps, c.Surface.A2UI.Components) {
					t.Fatalf("an op for box %+v that the operator did not move onto on these components", in)
				}
				flight, flightCard = &op.Op, op.Card
			case "role": // a role edits the text, or the boxes
				d := pick("roleCard")
				if d.checklist || d.broken {
					break
				}
				switch rapid.IntRange(0, 2).Draw(t, "edit") {
				case 0:
					d.caption += "!"
				case 1:
					d.names = slices.Clone(d.names)
					slices.Reverse(d.names)
				default:
					d.names = append([]string{"first"}, d.names...)
					d.done = append([]bool{false}, d.done...)
				}
				bump(d)
				c := d.card(outer)
				apply(CardsChanged{At: t0, Rev: rev, Cards: []Card{c}}, false)
			case "tick": // another client ticks a box
				d := pick("tickCard")
				if d.checklist || d.broken || len(d.done) == 0 {
					break
				}
				d.done = slices.Clone(d.done)
				d.done[rapid.IntRange(0, len(d.done)-1).Draw(t, "box")] = rapid.Bool().Draw(t, "value")
				bump(d)
				if rapid.Bool().Draw(t, "caption") {
					d.caption += "+"
				}
				c := d.card(outer)
				apply(CardsChanged{At: t0, Rev: rev, Cards: []Card{c}}, false)
			case "answer": // relay answers the operator's own input
				if flight == nil || s.Board.Sending != flightCard {
					break
				}
				d := srv[flightCard]
				if !d.checklist && !d.broken && flight.Op == "input" {
					for i := range d.done {
						if *flight.Path == fmt.Sprintf("/items/%d/done", i) {
							d.done = slices.Clone(d.done)
							d.done[i] = *flight.Value
						}
					}
					bump(d)
					if rapid.Bool().Draw(t, "caption") {
						d.caption += "?"
					}
				}
				c := d.card(outer)
				apply(CardOpDone{At: t0, CardID: flightCard, Card: &c}, flight.Op == "input")
				flight = nil
			case "retitle": // from the list or the title line of an open card
				if s.Board.Input != nil || s.Board.Confirm != 0 || s.Board.Sending != 0 || len(s.Board.Cards) == 0 {
					break
				}
				c, _ := BoardCard(s.Board)
				if s.Board.Detail && BoardLine(s.Board, c) != 0 {
					break
				}
				s, _ = boardKeys(s, PaneEdit)
				if s.Board.Input == nil {
					break
				}
				s = typeText(s, "x")
				var fx []Effect
				s, fx = boardKeys(s, PaneEnter)
				if ops := effectsOf[SendCardOp](fx); len(ops) == 1 {
					flight, flightCard = &ops[0].Op, ops[0].Card
				}
			case "refuse": // relay refuses the retitle, late
				if flight == nil || s.Board.Sending != flightCard {
					break
				}
				apply(CardOpDone{At: t0, CardID: flightCard, Err: "refused"}, false)
				flight = nil
			case "kind": // relay sends card 10 as another kind
				d := srv[10]
				d.checklist, d.broken = !d.checklist, false
				bump(d)
				c := d.card(outer)
				apply(CardsChanged{At: t0, Rev: rev, Cards: []Card{c}}, false)
			case "malformed", "recover":
				d := pick("badCard")
				d.broken = !d.broken
				bump(d)
				c := d.card(outer)
				apply(CardsChanged{At: t0, Rev: rev, Cards: []Card{c}}, false)
			case "restart": // the client closes and starts again, after a role's edit
				v := SaveSingle(s)
				d := pick("editedCard")
				if !d.checklist && !d.broken {
					d.names = append([]string{"first"}, d.names...)
					d.done = append([]bool{false}, d.done...)
					bump(d)
				}
				s, _ = feedS(RestoreSingle(Single{}, v), Connected{At: t0}, CardsLoaded{At: t0, Rev: rev, Cards: cards()})
				chosen, flight = choice{}, nil
			}
		}
	})
}
