package core

import (
	"reflect"
	"slices"
	"testing"

	"pgregory.net/rapid"
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

// The pane shows whatever relay put in front of the operator last: a
// display or a proposal brings the items back from the board. A display
// leaves a pane the operator is working in alone; a proposal takes the
// keys from the pane anyway.
func TestADisplayOrProposalBringsTheItemsIntoView(t *testing.T) {
	ask := PermissionAsked{At: t0, Permission: Permission{ID: 1, CallID: "c", Name: "send"}}
	for name, e := range map[string]Event{"display": displayEvent("d", "Venues", Labels{}), "proposal": ask} {
		s, _ := feedS(busySingle(), CardsLoaded{At: t0, Rev: 1}, e)
		if !s.Board.Items {
			t.Errorf("%s: the items did not come into view", name)
		}
	}
	s, _ := feedS(busySingle(), CardsLoaded{At: t0, Rev: 1}, key(t0, PaneToggle), displayEvent("d", "Venues", Labels{}))
	if s.Board.Items {
		t.Error("a display took the focused pane")
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

func TestTheBoardStaysOutOfTheWay(t *testing.T) {
	added := CardsChanged{At: t0, Rev: 101, Cards: []Card{tracker(20, 101)}}
	gone := checklist(10, 101)
	gone.Status = CardArchived
	focused, _ := feedS(onItems(), key(t0, PaneToggle))
	asking, _ := feedS(onItems(), PermissionAsked{At: t0, Permission: Permission{ID: 1, CallID: "c", Name: "send"}})
	unloaded, _ := feedS(Single{}, Connected{At: t0}, key(t0, PaneToggle), key(t0, PaneBoard), key(t0, PaneEsc))
	for name, c := range map[string]struct {
		s Single
		e CardsChanged
	}{
		"while the operator works in the pane": {focused, added},
		"while a proposal waits":               {asking, added},
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

// However cards change, the board never takes the pane from the
// operator while they work in it or a proposal waits for them.
func TestTheBoardNeverTakesThePane(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := onItems(checklist(10, 10))
		if rapid.Bool().Draw(t, "focused") {
			s, _ = feedS(s, key(t0, PaneToggle))
		} else {
			s, _ = feedS(s, PermissionAsked{At: t0, Permission: Permission{ID: 1, CallID: "c", Name: "send"}})
		}
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
	s, fx := boardKeys(s, PaneDown, PaneArchive, PaneOther)
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
