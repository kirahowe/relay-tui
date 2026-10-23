package core

import (
	"slices"
	"testing"
)

// The board in hub mode is the app's: one copy for the connection, there
// before any conversation opens.

// boardHub is a hub connection with the roster in and the board loaded,
// on the list, with no conversation attached: Dev has a live one in
// relay, Mail none.
func boardHub(t testing.TB, cards ...Card) Hub {
	t.Helper()
	if len(cards) == 0 {
		cards = []Card{checklist(10, 10, false, false), tracker(20, 20), checklist(30, 30)}
	}
	h, _ := apply(t, NewHub(""), HubConnected{At: t0},
		RosterUpdate{At: t0, Roles: []Role{role("dev", "s1"), role("mail", "")}},
		CardsLoaded{At: t0, Rev: 100, Cards: cards})
	return h
}

// focusMail puts Mail in focus, a role with no conversation, and gives
// the pane the keys.
func focusMail(t testing.TB, h Hub) Hub {
	t.Helper()
	h, fx := apply(t, h, FocusIndex{At: t0, Index: 1}, PaneKey{At: t0, Action: PaneToggle})
	if len(attachesIn(fx)) != 0 || h.Focused() != nil || !h.App.PaneFocus {
		t.Fatalf("focus %q, conversation %v, pane %v, effects %v", h.Focus, h.Focused(), h.App.PaneFocus, fx)
	}
	return h
}

// The bug: the board was asked for, and held, only by an attached
// conversation, so a client with no conversation open had none.
func TestHubAsksForTheBoardOnceConnectedWithNoConversation(t *testing.T) {
	h, fx := apply(t, NewHub(""), HubConnected{At: t0})
	if got := effectsOf[RequestCards](fx); len(got) != 1 || len(effectsOf[ForSession](fx)) != 0 {
		t.Fatalf("effects %v", fx)
	}
	h, fx = apply(t, h, RosterUpdate{At: t0, Roles: []Role{role("dev", "s1"), role("mail", "")}},
		CardsLoaded{At: t0, Rev: 100, Cards: []Card{checklist(10, 10)}})
	if len(h.Convs) != 0 || !h.App.Board.Loaded || len(h.App.Board.Cards) != 1 || h.App.Board.SelID != 10 {
		t.Fatalf("convs %d, board %+v", len(h.Convs), h.App.Board)
	}
	if len(effectsOf[RequestCards](fx)) != 0 {
		t.Fatalf("asked again: %v", fx)
	}
	// Attaching a conversation does not ask again: the board is not its.
	_, fx = apply(t, h, FocusIndex{At: t0, Index: 0}, Attached{At: t0, Role: "dev", Session: "s1"},
		Replayed{At: t0, Session: "s1"})
	if len(effectsOf[RequestCards](fx)) != 0 || slices.ContainsFunc(fx, func(f Effect) bool {
		fs, ok := f.(ForSession)
		_, cards := fs.Effect.(RequestCards)
		return ok && cards
	}) {
		t.Fatalf("a conversation asked for the board: %v", fx)
	}
}

func TestHubBoardIsTheSameWhicheverRoleIsInFocus(t *testing.T) {
	h := focusMail(t, boardHub(t))
	h, _ = apply(t, h, PaneKey{At: t0, Action: PaneDown}, PaneKey{At: t0, Action: PaneDown}, PaneKey{At: t0, Action: PaneEnter})
	want := h.App.Board
	if want.SelID != 30 || !want.Detail {
		t.Fatalf("selection %d, detail %v", want.SelID, want.Detail)
	}
	// Dev, with its conversation attached and talking, then back.
	h, _ = apply(t, h, FocusIndex{At: t0, Index: 0}, Attached{At: t0, Role: "dev", Session: "s1"},
		routed("s1", TextChunk{At: t0, Text: "hello"}), Replayed{At: t0, Session: "s1"})
	if h.Focused() == nil || h.App.Board.SelID != 30 || !h.App.Board.Detail {
		t.Fatalf("on dev: %+v", h.App.Board)
	}
	if h.App.PaneFocus {
		t.Fatal("the pane kept the keys on another role")
	}
	h, _ = apply(t, h, PaneKey{At: t0, Action: PaneToggle}, PaneKey{At: t0, Action: PaneUp})
	if h.App.Board.SelID != 30 || h.App.Board.Line != 0 {
		t.Fatalf("keys on dev's pane moved the card's line, not the card: %+v", h.App.Board)
	}
	h, _ = apply(t, h, FocusIndex{At: t0, Index: 1}, Move{At: t0, Delta: -1}, ListMove{At: t0, Delta: -1})
	if h.App.Board.SelID != 30 || !h.App.Board.Detail || len(h.App.Board.Cards) != 3 {
		t.Fatalf("on the list: %+v", h.App.Board)
	}
}

func TestHubCardsChangedWithNoConversationOpen(t *testing.T) {
	h := boardHub(t)
	gone := tracker(20, 101)
	gone.Status = CardArchived
	h, _ = apply(t, h, CardsChanged{At: t0, Rev: 102, Cards: []Card{checklist(10, 102, true, false), gone, checklist(40, 102)}})
	b := h.App.Board
	if len(h.Convs) != 0 || len(b.Cards) != 3 || b.Rev != 102 {
		t.Fatalf("board %+v", b)
	}
	if !b.Cards[0].Items[0].Done || slices.ContainsFunc(b.Cards, func(c Card) bool { return c.ID == 20 }) ||
		b.Cards[2].ID != 40 {
		t.Fatalf("cards %+v", b.Cards)
	}
}

func TestHubCardOpWithNoConversationOpen(t *testing.T) {
	h := focusMail(t, boardHub(t))
	h, fx := apply(t, h, PaneKey{At: t0, Action: PaneEnter}, PaneKey{At: t0, Action: PaneDown}, PaneKey{At: t0, Action: PaneMark})
	ops := effectsOf[SendCardOp](fx)
	if len(ops) != 1 || ops[0].Card != 10 || ops[0].Op.Op != "tick" || *ops[0].Op.Item != 1 {
		t.Fatalf("effects %v", fx)
	}
	if len(effectsOf[ForSession](fx)) != 0 || len(attachesIn(fx)) != 0 || len(h.Convs) != 0 {
		t.Fatalf("an op opened or named a conversation: %v", fx)
	}
	done := checklist(10, 101, true, false)
	h, _ = apply(t, h, CardOpDone{At: t0, CardID: 10, Card: &done})
	if b := h.App.Board; b.Sending != 0 || !b.Cards[0].Items[0].Done {
		t.Fatalf("after: %+v", b)
	}
	// The stale-edit check holds here as anywhere: an edit carries the rev
	// it started from.
	h, fx = apply(t, h, PaneKey{At: t0, Action: PaneEdit}, PaneKey{At: t0, Action: PaneChar, Text: "!"},
		PaneKey{At: t0, Action: PaneEnter})
	if ops := effectsOf[SendCardOp](fx); len(ops) != 1 || ops[0].Op.Seen == nil || *ops[0].Op.Seen != 101 {
		t.Fatalf("edit %v", fx)
	}
	_ = h
}

func TestHubCardTrustWithNoConversationOpen(t *testing.T) {
	h := focusMail(t, boardHub(t, outsideCard(40, shaA)))
	h, fx := apply(t, h, PaneKey{At: t0, Action: PaneTrust}, PaneKey{At: t0, Action: PaneYes})
	if h.App.Board.Trust == nil || len(effectsOf[SendCardTrust](fx)) != 0 {
		t.Fatalf("y before the delay: %+v %v", h.App.Board.Trust, fx)
	}
	h, fx = apply(t, h, PaneKey{At: t0.Add(ArmDelay), Action: PaneYes})
	want := SendCardTrust{Card: 30, Rev: 40, SHA256: shaA}
	if got := effectsOf[SendCardTrust](fx); len(got) != 1 || got[0] != want || len(effectsOf[ForSession](fx)) != 0 {
		t.Fatalf("effects %v", fx)
	}
	clean := checklist(30, 41, false)
	h, _ = apply(t, h, CardTrustDone{At: t0, CardID: 30, Card: &clean})
	if b := h.App.Board; b.Trust != nil || b.Message != "Trusted." || b.Cards[0].Labels.Contaminated {
		t.Fatalf("after: %+v", b)
	}
	// relay's refusal is shown, and nothing is trusted.
	h = focusMail(t, boardHub(t, outsideCard(40, shaA)))
	h, _ = apply(t, h, PaneKey{At: t0, Action: PaneTrust}, PaneKey{At: t0.Add(ArmDelay), Action: PaneYes},
		CardTrustDone{At: t0, CardID: 30, Err: "This conversation does not take trust from its client."})
	if b := h.App.Board; b.Trust != nil || b.Message != "Trust failed." || !b.Cards[0].Labels.Contaminated {
		t.Fatalf("refused: %+v", b)
	}
}

// A trust of a card not yet sent goes when the screen changes under it,
// so a y typed for something else cannot answer it; the operator asks
// again.
func TestHubCardTrustGoesWhenTheScreenChanges(t *testing.T) {
	for name, away := range map[string][]Event{
		"another role": {FocusIndex{At: t0, Index: 0}, FocusIndex{At: t0, Index: 1}, PaneKey{At: t0, Action: PaneToggle}},
		"the list":     {Move{At: t0, Delta: -1}, Move{At: t0, Delta: -1}, Move{At: t0, Delta: 1}, Move{At: t0, Delta: 1}},
		"the help":     {ToggleHelp{At: t0}, ToggleHelp{At: t0}},
		"h and l":      {Move{At: t0, Delta: -1}, Move{At: t0, Delta: 1}},
		"typing":       {Type{At: t0}, StopTyping{At: t0}, Move{At: t0, Delta: 1}},
	} {
		t.Run(name, func(t *testing.T) {
			h := focusMail(t, boardHub(t, outsideCard(40, shaA)))
			h, _ = apply(t, h, PaneKey{At: t0, Action: PaneTrust})
			h, _ = apply(t, h, away...)
			h, fx := apply(t, h, PaneKey{At: t0.Add(ArmDelay), Action: PaneYes})
			if h.App.Board.Trust != nil || len(effectsOf[SendCardTrust](fx)) != 0 {
				t.Fatalf("trust %+v, effects %v", h.App.Board.Trust, fx)
			}
		})
	}
	// One that is on its way is relay's to answer, whatever the screen.
	h := focusMail(t, boardHub(t, outsideCard(40, shaA)))
	h, _ = apply(t, h, PaneKey{At: t0, Action: PaneTrust}, PaneKey{At: t0.Add(ArmDelay), Action: PaneYes}, ToggleHelp{At: t0})
	if t2 := h.App.Board.Trust; t2 == nil || !t2.Sent {
		t.Fatalf("a sent trust was dropped: %+v", t2)
	}
}

// What a conversation shows brings the workspace items into view only
// when it is the conversation on screen: another's display does not take
// the pane from the board.
func TestHubOnlyTheConversationOnScreenMovesThePane(t *testing.T) {
	h, _ := apply(t, boardHub(t), FocusIndex{At: t0, Index: 0}, Attached{At: t0, Role: "dev", Session: "s1"},
		Replayed{At: t0, Session: "s1"}, routed("s9", TextChunk{At: t0, Text: "elsewhere"}))
	h, _ = apply(t, h, routed("s9", displayEvent("d", "Venues", Labels{})))
	if h.App.Board.Items {
		t.Fatal("a conversation off screen took the pane")
	}
	h, _ = apply(t, h, routed("s1", displayEvent("d", "Venues", Labels{})))
	if !h.App.Board.Items {
		t.Fatal("the conversation on screen did not bring its items into view")
	}
}

// With no role in focus there is no conversation column, so l on the
// list goes straight to the board: card ops work with no role focused
// at all, and esc or h gives the keys back to the list, where the cursor
// was.
func TestHubListMovesToTheBoard(t *testing.T) {
	h, _ := apply(t, boardHub(t), ListMove{At: t0, Delta: 1})
	if !h.List || h.Focus != "" || h.Cursor != "mail" || !h.App.Moving {
		t.Fatalf("start: list %v focus %q cursor %q moving %v", h.List, h.Focus, h.Cursor, h.App.Moving)
	}
	h, fx := apply(t, h, Move{At: t0, Delta: 1})
	if h.List || !h.App.PaneFocus || h.Focus != "" || len(fx) != 0 {
		t.Fatalf("l: list %v pane %v focus %q effects %v", h.List, h.App.PaneFocus, h.Focus, fx)
	}
	h, fx = apply(t, h, PaneKey{At: t0, Action: PaneEnter}, PaneKey{At: t0, Action: PaneDown}, PaneKey{At: t0, Action: PaneMark})
	ops := effectsOf[SendCardOp](fx)
	if len(ops) != 1 || ops[0].Card != 10 || ops[0].Op.Op != "tick" || *ops[0].Op.Item != 1 {
		t.Fatalf("tick: %v", fx)
	}
	if len(attachesIn(fx)) != 0 || len(h.Convs) != 0 || len(effectsOf[ForSession](fx)) != 0 {
		t.Fatalf("an op opened or named a conversation: %v", fx)
	}
	// esc closes the card, then gives the keys back to the list.
	h, _ = apply(t, h, PaneKey{At: t0, Action: PaneEsc})
	if h.List || !h.App.PaneFocus {
		t.Fatal("the first esc left the pane")
	}
	h, _ = apply(t, h, PaneKey{At: t0, Action: PaneEsc})
	if !h.List || h.App.PaneFocus || h.Cursor != "mail" {
		t.Fatalf("esc: list %v pane %v cursor %q", h.List, h.App.PaneFocus, h.Cursor)
	}
	// l there and h back end on the list too; another l at the pane, the
	// right edge, stays.
	g, _ := apply(t, h, Move{At: t0, Delta: 1}, Move{At: t0, Delta: 1})
	if g.List || !g.App.PaneFocus {
		t.Errorf("l at the edge: list %v pane %v", g.List, g.App.PaneFocus)
	}
	g, _ = apply(t, g, Move{At: t0, Delta: -1})
	if !g.List || g.App.PaneFocus || g.Cursor != "mail" {
		t.Errorf("h: list %v pane %v cursor %q", g.List, g.App.PaneFocus, g.Cursor)
	}
	// h at the list, the left edge, stays.
	if g, _ = apply(t, g, Move{At: t0, Delta: -1}); !g.List {
		t.Error("h at the edge left the list")
	}
	// A role from the board gives the keys to its conversation, moving.
	g, _ = apply(t, h, Move{At: t0, Delta: 1}, FocusIndex{At: t0, Index: 1})
	if g.List || g.App.PaneFocus || g.Focus != "mail" || !g.App.Moving {
		t.Errorf("2: list %v pane %v focus %q moving %v", g.List, g.App.PaneFocus, g.Focus, g.App.Moving)
	}
}

// With no board and no conversation there is nothing to the right of
// the list, so it keeps the keys.
func TestHubListKeepsTheKeysWithNoPaneToShow(t *testing.T) {
	h, _ := apply(t, NewHub(""), HubConnected{At: t0}, RosterUpdate{At: t0, Roles: []Role{role("dev", "s1")}},
		CardsLoaded{At: t0, Unavailable: true})
	h, _ = apply(t, h, Move{At: t0, Delta: 1})
	if !h.List || h.App.PaneFocus {
		t.Fatalf("list %v pane %v", h.List, h.App.PaneFocus)
	}
}

// A role in focus, the board with the keys: a permission request in the
// focused conversation takes them, and the conversation has them.
func TestHubRequestTakesTheKeysFromTheBoard(t *testing.T) {
	h, _ := apply(t, boardHub(t), FocusIndex{At: t0, Index: 0}, Attached{At: t0, Role: "dev", Session: "s1"},
		Move{At: t0, Delta: -1}, Move{At: t0, Delta: 1}, Move{At: t0, Delta: 1})
	if h.List || !h.App.PaneFocus || h.Focus != "dev" {
		t.Fatalf("list %v pane %v focus %q", h.List, h.App.PaneFocus, h.Focus)
	}
	h, _ = apply(t, h, routed("s1", PermissionAsked{At: t0, Permission: Permission{ID: 1, CallID: "c", Name: "send"}}))
	if h.List || h.App.PaneFocus || h.onScreen() != "s1" {
		t.Fatalf("list %v pane %v on screen %q", h.List, h.App.PaneFocus, h.onScreen())
	}
	// And l cannot take them back to the pane while it waits.
	if h, _ = apply(t, h, Move{At: t0, Delta: 1}); h.App.PaneFocus {
		t.Fatal("l took the keys from the request")
	}
}
