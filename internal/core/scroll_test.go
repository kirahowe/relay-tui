package core

import (
	"testing"

	"pgregory.net/rapid"
)

// An open surface read like a page: for any keys, with the card as the
// pane drew it, only tab and shift+tab move the focus; the window stays
// within what the pane said it can scroll; and x sends an op only while
// the pane says the focused box is on the screen.
func TestOnlyTheFocusKeysMoveTheFocus(t *testing.T) {
	card := surfaceCard(t, 10, 12, box{"/a", "Tent", true}, box{"/b", "Map", false}, box{"/c", "Stove", false})
	rapid.Check(t, func(rt *rapid.T) {
		s := onBoard(card)
		s, _ = boardKeys(s, PaneEnter)
		for range 30 {
			a := rapid.SampledFrom([]PaneAction{PaneUp, PaneDown, PagePrev, PageNext, PaneTab, PaneBackTab, PaneMark}).Draw(rt, "key")
			v := CardScroll{Card: 10, Top: rapid.IntRange(0, 40).Draw(rt, "top"), Page: rapid.IntRange(1, 20).Draw(rt, "page"),
				Shown: rapid.Bool().Draw(rt, "shown")}
			v.Max = v.Top + rapid.IntRange(0, 40).Draw(rt, "more")
			c, _ := BoardCard(s.Board)
			line := BoardLine(s.Board, c)
			var fx []Effect
			s, fx = ReduceSingle(s, PaneKey{At: t0, Action: a, Card: &v})
			if s.Board.Sending != 0 {
				// The op's answer is not under test: take it as refused.
				s, _ = ReduceSingle(s, CardOpDone{At: t0, CardID: 10, Err: "refused"})
			}
			now := BoardLine(s.Board, c)
			switch a {
			case PaneTab, PaneBackTab:
				d := map[PaneAction]int{PaneTab: 1, PaneBackTab: -1}[a]
				if now != (line+d+c.Lines())%c.Lines() || !s.Board.Follow {
					rt.Fatalf("%v from %d: on %d, follow %v", a, line, now, s.Board.Follow)
				}
			case PaneMark:
				sent := len(effectsOf[SendCardOp](fx)) > 0
				if line > 0 && sent != v.Shown {
					rt.Fatalf("x on line %d, shown %v: sent %v", line, v.Shown, sent)
				}
				if line > 0 && !v.Shown && !s.Board.Follow {
					rt.Fatalf("x off the screen did not bring the box back")
				}
			default:
				if now != line {
					rt.Fatalf("%v moved the focus from %d to %d", a, line, now)
				}
				if top, follow := BoardScroll(s.Board, c); follow || top < 0 || top > v.Max {
					rt.Fatalf("%v: window at %d of 0..%d, follow %v", a, top, v.Max, follow)
				}
			}
		}
	})
}
