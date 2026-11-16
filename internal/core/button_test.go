package core

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/kirahowe/relay-tui/internal/a2ui"
)

// buttonSurface is a resolved surface of a box, a button that sets it, an
// event button in a list item (remove__1) and a primary event button, in
// that drawing order.
func buttonSurface() a2ui.Surface {
	text := func(id, s string) a2ui.Component {
		return a2ui.Component{ID: id, Type: "Text", Props: map[string]any{"text": s}}
	}
	event := func(id, child string, extra map[string]any) a2ui.Component {
		p := map[string]any{"child": child, "action": map[string]any{"event": map[string]any{"name": "remove_item"}}}
		for k, v := range extra {
			p[k] = v
		}
		return a2ui.Component{ID: id, Type: "Button", Props: p}
	}
	comps := []a2ui.Component{
		{ID: "root", Type: "Column", Props: map[string]any{"children": []any{"box", "set", "remove__1", "go"}}},
		{ID: "box", Type: "CheckBox", Props: map[string]any{"label": "Boots", "value": map[string]any{"@path": "/done"}}},
		{ID: "set", Type: "Button", Props: map[string]any{"child": "setText", "action": map[string]any{
			"functionCall": map[string]any{"@call": "relay:setInput", "args": map[string]any{"path": "/done", "value": true}}}}},
		text("setText", "Packed"),
		event("remove__1", "removeText", nil),
		text("removeText", "Remove Boots"),
		event("go", "goText", map[string]any{"variant": "primary"}),
		text("goText", "Plan the route"),
	}
	s := a2ui.Surface{ID: "card_10", CatalogID: a2ui.RelayCatalogID, Components: map[string]a2ui.Component{}, Data: map[string]any{"done": false}}
	for _, c := range comps {
		s.Components[c.ID] = c
	}
	return s
}

func buttonCard(rev int64) Card {
	return Card{ID: 10, Kind: CardSurface, Status: CardActive, Rev: rev, Owner: "steward", Title: txt("Pack", 10),
		Surface: &RoleSurface{A2UI: buttonSurface()}}
}

// opened is a board with the card open and the cursor on line n, put there
// by key presses at t0.
func opened(card Card, n int) Single {
	s := onBoard(card)
	s, _ = boardKeys(s, PaneEnter)
	for range n {
		s, _ = boardKeys(s, PaneTab)
	}
	return s
}

// press is the keys that press, from a live key at at.
func press(s Single, at time.Time) (Single, []SendCardOp) {
	s, fx := feedS(s, KeyPressed{At: at}, PaneKey{At: at, Action: PaneMark})
	return s, effectsOf[SendCardOp](fx)
}

func TestTabVisitsBoxesAndButtonsInDrawnOrder(t *testing.T) {
	card := buttonCard(10)
	want := []string{"box", "set", "remove__1", "go"}
	s := onBoard(card)
	s, _ = boardKeys(s, PaneEnter)
	for i, id := range want {
		s, _ = boardKeys(s, PaneTab)
		c, _ := BoardCard(s.Board)
		if line := BoardLine(s.Board, c); line != i+1 || c.Inputs()[line-1].ID != id {
			t.Fatalf("after %d downs: line %d on %q, want %q", i+1, line, c.Inputs()[line-1].ID, id)
		}
	}
	// Past the last the focus goes back to the title, and shift+tab goes
	// back through them. Up and down scroll and leave the focus alone.
	s, _ = boardKeys(s, PaneTab)
	if c, _ := BoardCard(s.Board); BoardLine(s.Board, c) != 0 {
		t.Fatal("tab past the last button did not return to the title")
	}
	s, _ = boardKeys(s, PaneBackTab)
	if c, _ := BoardCard(s.Board); BoardLine(s.Board, c) != 4 {
		t.Fatal("shift+tab from the title did not reach the last button")
	}
	s, _ = boardKeys(s, PaneUp, PaneDown, PaneDown)
	if c, _ := BoardCard(s.Board); BoardLine(s.Board, c) != 4 {
		t.Fatal("scrolling moved the focus")
	}
	if c := buttonCard(10); !c.Inputs()[1].Button || c.Inputs()[1].Event != "" || c.Inputs()[2].Event != "remove_item" || c.Inputs()[0].Button {
		t.Fatalf("inputs %+v", c.Inputs())
	}
}

// A press sends the five fields and nothing else, with the id as sent, and
// the board waits for relay's answer before taking another op. Nothing is
// written to the card locally.
func TestAPressSendsExactlyTheFiveFieldOpAndWaits(t *testing.T) {
	for line, id := range map[int]string{2: "set", 3: "remove__1", 4: "go"} {
		s := opened(buttonCard(10), line)
		before, _ := BoardCard(s.Board)
		s, ops := press(s, t0.Add(ArmDelay))
		if len(ops) != 1 {
			t.Fatalf("line %d: %d ops", line, len(ops))
		}
		want := SendCardOp{Card: 10, Op: CardOp{Op: "press", Component: strp(id), Seen: int64p(10)}}
		if !reflect.DeepEqual(ops[0], want) {
			t.Fatalf("line %d: op %+v, want %+v", line, ops[0], want)
		}
		if s.Board.Sending != 10 || s.Board.SendingOp != "press" {
			t.Fatalf("line %d: not waiting: %+v", line, s.Board.Sending)
		}
		after, _ := BoardCard(s.Board)
		if !reflect.DeepEqual(before.Surface.A2UI.Data, after.Surface.A2UI.Data) || before.Rev != after.Rev {
			t.Fatalf("line %d: the card changed before relay answered", line)
		}
		// A second press while waiting sends nothing.
		s, ops = press(s, t0.Add(3*ArmDelay))
		if len(ops) != 0 || s.Board.Message != busyNotice {
			t.Fatalf("line %d: pressed again while waiting: %v, %q", line, ops, s.Board.Message)
		}
	}
}

func TestTheCardRelayAnswersWithIsTheCardDrawn(t *testing.T) {
	s := opened(buttonCard(10), 2)
	s, _ = press(s, t0.Add(ArmDelay))
	answer := buttonCard(11)
	answer.Surface.A2UI.Data = map[string]any{"done": true}
	s, _ = feedS(s, CardOpDone{At: t0.Add(ArmDelay), CardID: 10, Card: &answer})
	c, _ := BoardCard(s.Board)
	if c.Rev != 11 || c.Surface.A2UI.Data.(map[string]any)["done"] != true || s.Board.Sending != 0 {
		t.Fatalf("card %+v, sending %d", c, s.Board.Sending)
	}
	// The cursor stays on the button: the boxes and buttons are the same.
	if line := BoardLine(s.Board, c); line != 2 {
		t.Fatalf("cursor on line %d", line)
	}
}

func TestARefusalOfAPressShowsRelaysText(t *testing.T) {
	s := opened(buttonCard(10), 3)
	s, _ = press(s, t0.Add(ArmDelay))
	const why = "steward has not yet taken up your last press of that button."
	s, _ = feedS(s, CardOpDone{At: t0.Add(ArmDelay), CardID: 10, Err: why})
	if s.Board.Sending != 0 || s.Board.Failure != why || s.Board.Message != refusedNotice {
		t.Fatalf("sending %d, message %q, failure %q", s.Board.Sending, s.Board.Message, s.Board.Failure)
	}
}

// A press needs a live key ArmDelay after the button came under the
// cursor, with the card as drawn, and never one replayed while connecting.
func TestAPressIsTakenOnlyFromAnArmedLiveKey(t *testing.T) {
	// Within ArmDelay of the cursor arriving.
	s := opened(buttonCard(10), 3)
	s, ops := press(s, t0.Add(ArmDelay-time.Millisecond))
	if len(ops) != 0 || s.Board.Sending != 0 {
		t.Fatalf("sent %v within ArmDelay", ops)
	}
	if msg := s.State.Message.Text; msg != TooSoonNote {
		t.Fatalf("note %q", msg)
	}
	// Exactly ArmDelay after: taken.
	s = opened(buttonCard(10), 3)
	if _, ops = press(s, t0.Add(ArmDelay)); len(ops) != 1 {
		t.Fatalf("%d ops at ArmDelay", len(ops))
	}
	// A replayed key, however late.
	s = opened(buttonCard(10), 3)
	s, fx := feedS(s, KeyPressed{At: t0.Add(time.Hour), Replayed: true}, PaneKey{At: t0.Add(time.Hour), Action: PaneMark})
	if len(effectsOf[SendCardOp](fx)) != 0 || s.State.Message.Text != ReplayedNote {
		t.Fatalf("a replayed key pressed: %v, %q", fx, s.State.Message.Text)
	}
	// Moving the cursor onto another button arms afresh.
	s = opened(buttonCard(10), 2)
	s, _ = feedS(s, KeyPressed{At: t0.Add(time.Hour)}, PaneKey{At: t0.Add(time.Hour), Action: PaneTab})
	if _, ops = press(s, t0.Add(time.Hour+ArmDelay-time.Millisecond)); len(ops) != 0 {
		t.Fatal("a press right after moving was taken")
	}
	// A box is ticked at once, as before.
	s = opened(buttonCard(10), 1)
	if _, ops = press(s, t0); len(ops) != 1 || ops[0].Op.Op != "input" {
		t.Fatalf("a box did not tick at once: %v", ops)
	}
}

// A change to the card in any way, even one that leaves the button, arms
// the press afresh: the label read may be another one.
func TestAChangedCardArmsThePressAgain(t *testing.T) {
	s := opened(buttonCard(10), 3)
	at := t0.Add(time.Hour)
	next := buttonCard(11)
	s, _ = feedS(s, CardsChanged{At: at, Rev: 11, Cards: []Card{next}})
	if _, ops := press(s, at.Add(ArmDelay-time.Millisecond)); len(ops) != 0 {
		t.Fatal("a press right after the card changed was taken")
	}
	s, _ = feedS(s, CardsChanged{At: at, Rev: 12, Cards: []Card{buttonCard(12)}})
	s, ops := press(s, at.Add(ArmDelay))
	if len(ops) != 1 || *ops[0].Op.Seen != 12 {
		t.Fatalf("ops %v", ops)
	}
}

// Over any run of keys at any times, replayed or live, a press op goes out
// only for a button the cursor is on, from a live key at least ArmDelay
// after that button was under the cursor with the card at that rev, with
// the card's rev and nothing but the five fields.
func TestPressesAreArmedWhateverTheKeys(t *testing.T) {
	sent := 0
	defer func() {
		if sent == 0 {
			t.Error("the run never sent a press")
		}
	}()
	rapid.Check(t, func(t *rapid.T) {
		s := opened(buttonCard(10), 0)
		at := t0
		became, becameAt := SingleSurfaceID(s), t0
		for step := range rapid.IntRange(1, 60).Draw(t, "steps") {
			at = at.Add(time.Duration(rapid.IntRange(0, 900).Draw(t, "ms")) * time.Millisecond)
			replayed := rapid.IntRange(0, 3).Draw(t, "replayed") == 0
			a := rapid.SampledFrom([]PaneAction{PaneBackTab, PaneTab, PaneMark, PaneMark, PaneMark}).Draw(t, "action")
			c, _ := BoardCard(s.Board)
			line := BoardLine(s.Board, c)
			next, fx := feedS(s, KeyPressed{At: at, Replayed: replayed}, PaneKey{At: at, Action: a})
			for _, op := range effectsOf[SendCardOp](fx) {
				if op.Op.Op != "press" {
					continue
				}
				sent++
				in := c.Inputs()[line-1]
				switch {
				case a != PaneMark || line == 0 || !in.Button:
					t.Fatalf("step %d: press from %v on line %d", step, a, line)
				case replayed:
					t.Fatalf("step %d: press from a replayed key", step)
				case at.Sub(becameAt) < ArmDelay:
					t.Fatalf("step %d: press %v after the cursor arrived", step, at.Sub(becameAt))
				case op.Op.Component == nil || *op.Op.Component != in.ID || op.Op.Seen == nil || *op.Op.Seen != c.Rev ||
					op.Op.Path != nil || op.Op.Value != nil || op.Op.Item != nil || op.Op.Text != nil || op.Card != 10:
					t.Fatalf("step %d: op %+v for button %q", step, op, in.ID)
				}
			}
			if s.Board.Sending != 0 && next.Board.Sending == 0 {
				t.Fatal("sending cleared without an answer")
			}
			s = next
			if id := SingleSurfaceID(s); id != became {
				became, becameAt = id, at
			}
			// An answer now and then, so a later press can go out.
			if s.Board.Sending != 0 && rapid.Bool().Draw(t, "answer") {
				card, _ := BoardCard(s.Board)
				s, _ = feedS(s, CardOpDone{At: at, CardID: 10, Card: &card})
			}
		}
	})
}

// Whatever replaces the card with other components sends the cursor to
// the title, so no key acts on a button of a layout the operator did not
// see.
func TestAChangedLayoutSendsTheCursorToTheTitleNotAButton(t *testing.T) {
	s := opened(buttonCard(10), 4)
	other := buttonCard(11)
	comps := map[string]a2ui.Component{}
	for k, v := range other.Surface.A2UI.Components {
		comps[k] = v
	}
	comps["root"] = a2ui.Component{ID: "root", Type: "Column", Props: map[string]any{"children": []any{"go", "set", "box", "remove__1"}}}
	other.Surface = &RoleSurface{A2UI: a2ui.Surface{ID: "card_10", CatalogID: a2ui.RelayCatalogID, Components: comps, Data: other.Surface.A2UI.Data}}
	s, _ = feedS(s, CardsChanged{At: t0, Rev: 11, Cards: []Card{other}})
	c, _ := BoardCard(s.Board)
	if line := BoardLine(s.Board, c); line != 0 || !strings.Contains(s.Board.Message, "changed") {
		t.Fatalf("line %d, message %q", line, s.Board.Message)
	}
	if _, ops := press(s, t0.Add(time.Hour)); len(ops) != 0 {
		t.Fatalf("a press on the title sent %v", ops)
	}
}
