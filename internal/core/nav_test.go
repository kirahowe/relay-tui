package core

import (
	"testing"

	"pgregory.net/rapid"
)

// Moving and typing, and the columns: the list (hub mode only), the
// conversation and the pane.

// where is the column with the keys and the mode, as one word each.
func where(h Hub) (string, bool) {
	return [...]string{"list", "conv", "pane"}[h.col()], h.App.Moving
}

func TestHubColumnsLeftToRight(t *testing.T) {
	h := boardHub(t)
	if c, moving := where(h); c != "list" || !moving {
		t.Fatalf("start: %s moving %v", c, moving)
	}
	// With no role in focus there is no conversation column: l goes to
	// the pane, h comes back.
	g, _ := apply(t, h, Move{At: t0, Delta: 1})
	if c, _ := where(g); c != "pane" {
		t.Fatalf("l with no role: %s", c)
	}
	// Enter on the list opens the role and types in it.
	h, _ = apply(t, h, ListOpen{At: t0})
	if c, moving := where(h); c != "conv" || moving || h.Focus != "dev" {
		t.Fatalf("enter: %s moving %v focus %q", c, moving, h.Focus)
	}
	h, _ = apply(t, h, StopTyping{At: t0})
	for _, step := range []struct {
		delta int
		want  string
	}{{1, "pane"}, {1, "pane"}, {-1, "conv"}, {-1, "list"}, {-1, "list"}, {1, "conv"}} {
		h, _ = apply(t, h, Move{At: t0, Delta: step.delta})
		if c, moving := where(h); c != step.want || !moving {
			t.Fatalf("move %+d: %s moving %v, want %s", step.delta, c, moving, step.want)
		}
	}
	// i from the pane or the list types in the conversation.
	for _, from := range [][]Event{{Move{At: t0, Delta: 1}}, {Move{At: t0, Delta: -1}}} {
		g, _ := apply(t, h, append(from, Type{At: t0})...)
		if c, moving := where(g); c != "conv" || moving {
			t.Errorf("i after %v: %s moving %v", from, c, moving)
		}
	}
}

// 1 to 9 keep the mode: moving stays moving, typing stays typing, and
// either way the keys go to the role's conversation.
func TestHubJumpKeepsTheMode(t *testing.T) {
	h, _ := apply(t, boardHub(t), ListOpen{At: t0})
	h, _ = apply(t, h, FocusIndex{At: t0, Index: 1})
	if c, moving := where(h); c != "conv" || moving || h.Focus != "mail" {
		t.Fatalf("typing: %s moving %v focus %q", c, moving, h.Focus)
	}
	for _, from := range []Event{StopTyping{At: t0}, Move{At: t0, Delta: 1}, Move{At: t0, Delta: -1}} {
		g, _ := apply(t, h, StopTyping{At: t0}, from, FocusIndex{At: t0, Index: 0})
		if c, moving := where(g); c != "conv" || !moving || g.Focus != "dev" {
			t.Errorf("moving, from %T: %s moving %v focus %q", from, c, moving, g.Focus)
		}
	}
}

// i with no role in focus has nothing to type into, and says so.
func TestHubTypeWithNoRole(t *testing.T) {
	h, _ := apply(t, boardHub(t), Type{At: t0})
	if c, moving := where(h); c != "list" || !moving || h.Notice == "" {
		t.Fatalf("%s moving %v notice %q", c, moving, h.Notice)
	}
}

// A letter that does nothing says how to type, and the next key clears
// the note.
func TestUnboundSaysHowToType(t *testing.T) {
	h, _ := apply(t, boardHub(t), Unbound{At: t0})
	if h.Notice != UnboundNote {
		t.Fatalf("notice %q", h.Notice)
	}
	if h, _ = apply(t, h, ListMove{At: t0, Delta: 1}); h.Notice != "" {
		t.Fatalf("notice %q after a key", h.Notice)
	}
	x, _ := feedS(Single{}, Unbound{At: t0})
	if x.State.Message.Text != UnboundNote {
		t.Fatalf("single: %q", x.State.Message.Text)
	}
}

// Against a single agent there is no list: the conversation is the left
// edge.
func TestSingleHasTwoColumns(t *testing.T) {
	x, _ := feedS(Single{}, Connected{At: t0}, CardsLoaded{At: t0, Rev: 1, Cards: viewCards()})
	if x.App.Moving || x.App.PaneFocus {
		t.Fatalf("start: moving %v pane %v", x.App.Moving, x.App.PaneFocus)
	}
	x, _ = feedS(x, StopTyping{At: t0}, Move{At: t0, Delta: -1})
	if !x.App.Moving || x.App.PaneFocus {
		t.Fatalf("h at the edge: moving %v pane %v", x.App.Moving, x.App.PaneFocus)
	}
	x, _ = feedS(x, Move{At: t0, Delta: 1}, Move{At: t0, Delta: 1})
	if !x.App.PaneFocus || !x.App.Moving {
		t.Fatalf("l: pane %v", x.App.PaneFocus)
	}
	x, _ = feedS(x, Type{At: t0})
	if x.App.PaneFocus || x.App.Moving {
		t.Fatalf("i from the pane: pane %v moving %v", x.App.PaneFocus, x.App.Moving)
	}
}

// The saved view keeps the column; the mode comes back from it: typing
// in a conversation, moving anywhere else.
func TestRestoreLandsInTheModeOfTheColumn(t *testing.T) {
	typing, _ := apply(t, boardHub(t), ListOpen{At: t0})
	for name, c := range map[string]struct {
		h      Hub
		moving bool
	}{
		"conversation":        {typing, false},
		"conversation moving": {func() Hub { h, _ := apply(t, typing, StopTyping{At: t0}); return h }(), false},
		"pane":                {func() Hub { h, _ := apply(t, typing, StopTyping{At: t0}, Move{At: t0, Delta: 1}); return h }(), true},
		"list":                {func() Hub { h, _ := apply(t, typing, StopTyping{At: t0}, Move{At: t0, Delta: -1}); return h }(), true},
	} {
		g, _ := reopen(t, SaveHub(c.h), []Role{role("dev", "s1"), role("mail", "")}, viewCards())
		if g.App.Moving != c.moving || g.col() != c.h.col() {
			t.Errorf("%s: moving %v column %d, want %v %d", name, g.App.Moving, g.col(), c.moving, c.h.col())
		}
	}
	x, _ := feedS(Single{}, Connected{At: t0}, StopTyping{At: t0}, Move{At: t0, Delta: 1})
	y, _ := feedS(RestoreSingle(Single{}, SaveSingle(x)), Connected{At: t0})
	if !y.App.PaneFocus || !y.App.Moving {
		t.Fatalf("single pane: pane %v moving %v", y.App.PaneFocus, y.App.Moving)
	}
}

// genNav is any navigation or pane event.
func genNav(t *rapid.T) Event {
	switch rapid.IntRange(0, 9).Draw(t, "nav") {
	case 0:
		return Move{At: t0, Delta: rapid.SampledFrom([]int{-1, 1}).Draw(t, "delta")}
	case 1:
		return Type{At: t0}
	case 2:
		return StopTyping{At: t0}
	case 3:
		return FocusIndex{At: t0, Index: rapid.IntRange(0, 2).Draw(t, "index")}
	case 4:
		return ListMove{At: t0, Delta: rapid.SampledFrom([]int{-1, 1}).Draw(t, "ldelta")}
	case 5:
		return ListOpen{At: t0}
	case 6:
		return Unbound{At: t0}
	case 7:
		return ToggleHelp{At: t0}
	}
	return PaneKey{At: t0, Action: PaneAction(rapid.IntRange(int(PaneUp), int(PaneBackspace)).Draw(t, "action"))}
}

// However the operator moves, typing is only ever in the conversation
// of a role in focus, and a key for the pane does nothing while typing.
func TestTypingIsOnlyInAConversation(t *testing.T) {
	start := boardHub(t)
	rapid.Check(t, func(t *rapid.T) {
		h := start
		for range rapid.IntRange(1, 40).Draw(t, "steps") {
			e := genNav(t)
			typing := !h.App.Moving
			var fx []Effect
			h, fx = ReduceHub(h, e)
			if !h.App.Moving && (h.List || h.App.PaneFocus || h.Focus == "") {
				t.Fatalf("typing in column %d with focus %q after %T", h.col(), h.Focus, e)
			}
			if _, pane := e.(PaneKey); pane && typing && len(effectsOf[SendCardOp](fx))+len(effectsOf[SendCardTrust](fx)) > 0 {
				t.Fatalf("a pane key while typing reached the board: %v", fx)
			}
		}
	})
}
