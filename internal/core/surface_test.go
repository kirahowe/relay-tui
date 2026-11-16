package core

import (
	"fmt"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// The surface in the core: whatever event the shell sends, the reducers
// act only where the keys are. These run every operator event at random,
// not only the ones the shell's router would make of a key, so they hold
// even if the router were wrong.

// genOperator is any event of the operator's, with random fields.
func genOperator(t *rapid.T, at time.Time) Event {
	actions := []PaneAction{PaneToggle, PaneUp, PaneDown, PaneTab, PaneBackTab, PagePrev, PageNext, PaneEnter, PaneEsc, PaneYes,
		PaneBoard, PaneRefresh, PaneLeft, PaneRight, PaneMark, PaneAdd, PaneAddColumn, PaneEdit, PaneEditColumn,
		PaneArchive, PaneChar, PaneBackspace, PaneOther, PaneLinks, PaneLinks, PaneDrop, PaneDrop}
	// The ready view's keys, but for the one that opens a reply: its
	// input is not part of these rules.
	ready := []ReadyAction{ReadyOther, ReadyUp, ReadyDown, ReadyEsc, ReadyOpenKey, ReadyOpenKey, ReadyDismiss, ReadyDismiss,
		ReadyLinksKey, ReadyLinksKey, ReadyYes, ReadyYes, ReadyYes, ReadyEnter, ReadyHelp}
	switch rapid.IntRange(0, 27).Draw(t, "op") {
	case 0:
		return Submit{At: at, Text: "hello"}
	case 1:
		return Interrupt{At: at, InputEmpty: rapid.Bool().Draw(t, "empty")}
	case 2:
		return Pasted{At: at, Text: "pasted"}
	case 3, 4, 5, 6, 7:
		return PaneKey{At: at, Action: rapid.SampledFrom(actions).Draw(t, "action"), Text: "w"}
	case 8:
		return ShowTools{At: at}
	case 9:
		return TranscriptKey{At: at, Action: TranscriptAction(rapid.IntRange(0, 6).Draw(t, "taction")), Max: 10, Page: 3}
	case 10:
		return ToggleHelp{At: at}
	case 11:
		return FocusIndex{At: at, Index: rapid.IntRange(0, 2).Draw(t, "index")}
	case 12, 13:
		return Move{At: at, Delta: rapid.SampledFrom([]int{-1, 1}).Draw(t, "delta")}
	case 14:
		return Type{At: at}
	case 15:
		return StopTyping{At: at}
	case 16:
		return Unbound{At: at}
	case 17:
		return ListMove{At: at, Delta: 1}
	case 18:
		return ListOpen{At: at}
	case 19:
		return AskEnd{At: at}
	case 20, 21:
		return ConfirmEnd{At: at, Yes: true}
	case 22:
		return ReadyOpen{At: at}
	case 23, 24, 25, 26:
		return ReadyKey{At: at, Action: rapid.SampledFrom(ready).Draw(t, "ready"), Text: "w"}
	}
	return StartConversation{At: at}
}

// authorizes says an effect acts on the operator's yes: an archive, an
// end, a link kept or dropped, a button pressed.
func authorizes(f Effect) bool {
	switch f := f.(type) {
	case EndConversation, SendLink:
		return true
	case SendCardOp:
		return f.Op.Op == "archive" || f.Op.Op == "press"
	}
	return false
}

// asks is the surface whose yes an effect needs.
func asks(f Effect, surf Surface) bool {
	switch f := f.(type) {
	case EndConversation:
		return surf.Kind == SurfaceEnd
	case SendLink:
		return surf.Kind == SurfaceLinkAsk || surf.Kind == SurfaceReadyLinkAsk
	case SendCardOp:
		if f.Op.Op == "press" {
			return surf.Kind == SurfacePane
		}
	}
	return surf.Kind == SurfaceArchive
}

// surfaceChecks runs events at random through reduce, as the shell sends
// them: each operator event after a KeyPressed, live or replayed, and the
// agent's events. It checks that a typing surface's event never ends or
// archives anything, keeps or drops no link, and sends no card op but
// the editor's own; that an event of the agent's never takes the keys
// from a typing surface; and that the questions (an end, an archive, a
// link kept or dropped) are answered yes only by a live key, from the
// surface that asks, ArmDelay after it came up.
func surfaceChecks[S any](t *rapid.T, x S, reduce func(S, Event) (S, []Effect), surface func(S) Surface,
	surfaceID func(S) string, agent func(t *rapid.T, at time.Time, n uint64) Event) {
	at := t0
	// A key first: before any, the start may move the keys (App.Pressed).
	x, _ = reduce(x, KeyPressed{At: t0})
	became, becameAt := surfaceID(x), t0
	var n uint64
	for step := range rapid.IntRange(1, 100).Draw(t, "steps") {
		at = at.Add(time.Duration(rapid.IntRange(0, 900).Draw(t, "ms")) * time.Millisecond)
		surf, id := surface(x), surfaceID(x)
		var e Event
		replayed := false
		if rapid.IntRange(0, 9).Draw(t, "kind") == 0 {
			n++
			e = agent(t, at, n)
		} else {
			replayed = rapid.IntRange(0, 3).Draw(t, "replayed") == 0
			x, _ = reduce(x, KeyPressed{At: at, Replayed: replayed})
			e = genOperator(t, at)
		}
		operator := fromOperator(e)
		next, fx := reduce(x, e)
		for _, f := range fx {
			if ff, ok := f.(ForSession); ok {
				f = ff.Effect
			}
			if surf.Typing() && operator {
				switch f := f.(type) {
				case EndConversation, SendLink:
					t.Fatalf("step %d: %T while typing (%v) made %T", step, e, surf.Kind, f)
				case SendCardOp:
					if pk, ok := e.(PaneKey); !ok || surf.Kind != SurfaceEditor || pk.Action != PaneEnter {
						t.Fatalf("step %d: %T while typing (%v) sent a card op", step, e, surf.Kind)
					}
				}
			}
			if authorizes(f) {
				switch {
				case !operator || replayed:
					t.Fatalf("step %d: %T (operator %v, replayed %v) authorized %#v", step, e, operator, replayed, f)
				case became != id || at.Sub(becameAt) < ArmDelay:
					t.Fatalf("step %d: %T answered %#v on %q, up since %q %v before", step, e, f, id, became, at.Sub(becameAt))
				case !asks(f, surf):
					t.Fatalf("step %d: %T made %#v from %v, not its question", step, e, f, surf.Kind)
				}
				surfaceAnswered[surf.Kind]++
			}
		}
		if !operator && surf.Typing() && surface(next) != surf {
			t.Fatalf("step %d: %T took the keys from %+v to %+v", step, e, surf, surface(next))
		}
		x = next
		if surfaceID(x) != id {
			became, becameAt = surfaceID(x), at // a new surface, up now
		}
	}
}

// surfaceAnswered counts the questions the runs answered yes, by
// surface, so a test can say the property reached them.
var surfaceAnswered = map[SurfaceKind]int{}

// linksEvent is a change to relay's live links, or to what holds them,
// from outside: the set, the card with links in it, or the ready item.
func linksEvent(t *rapid.T, at time.Time, n uint64) Event {
	rev := int64(100 + n)
	switch rapid.IntRange(0, 2).Draw(t, "links") {
	case 0:
		ch := LinksChanged{At: at, Rev: rev}
		for _, l := range []string{liveA, keptC} {
			if rapid.Bool().Draw(t, "live") {
				ch.Live = append(ch.Live, l)
			} else {
				ch.Gone = append(ch.Gone, l)
			}
		}
		return ch
	case 1:
		c := linkCard(50, rev)
		if rapid.Bool().Draw(t, "listed") {
			c = keptCard(50, rev)
		}
		return CardsChanged{At: at, Rev: rev, Cards: []Card{c}}
	}
	it := linkItem()
	if rapid.Bool().Draw(t, "listed") {
		it = keptItem()
	}
	return ReadyChanged{At: at, Rev: rev, Items: []ReadyItem{it}}
}

// boardEvent is a change to the board from outside: a card changed, a
// card archived (the one an editor may be open on), or the board sent
// again.
func boardEvent(t *rapid.T, at time.Time, n uint64) Event {
	rev := int64(100 + n)
	switch rapid.IntRange(0, 2).Draw(t, "board") {
	case 0:
		c := checklist(10, rev, false, true)
		c.Title.Text = fmt.Sprint("retitled ", n)
		return CardsChanged{At: at, Rev: rev, Cards: []Card{c}}
	case 1:
		c := checklist(rapid.SampledFrom([]int64{10, 30}).Draw(t, "archived"), rev)
		c.Status = CardArchived
		return CardsChanged{At: at, Rev: rev, Cards: []Card{c}}
	}
	return CardsLoaded{At: at, Rev: rev, Cards: []Card{linkCard(50, rev), checklist(10, rev, false), checklist(30, rev, true), buttonsOn(60)}}
}

func TestSurfaceRulesInTheHubCore(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		h, _ := apply(t, NewHub("dev"), HubConnected{At: t0}, RosterUpdate{At: t0, Roles: []Role{role("dev", "s1"), role("mail", "s2")}},
			Attached{At: t0, Role: "dev", Session: "s1"}, Attached{At: t0, Role: "mail", Session: "s2"},
			CardsLoaded{At: t0, Rev: 40, Cards: []Card{linkCard(50, 40), checklist(10, 10, false, true), checklist(30, 40, false), buttonsOn(60)}},
			LinksLoaded{At: t0, Rev: 40, Live: []string{liveA}},
			ReadyLoaded{At: t0, Rev: 40, Items: []ReadyItem{linkItem()}})
		surfaceChecks(t, h, ReduceHub, HubSurface, HubSurfaceID, func(t *rapid.T, at time.Time, n uint64) Event {
			session := rapid.SampledFrom([]string{"s1", "s2"}).Draw(t, "session")
			switch rapid.IntRange(0, 6).Draw(t, "agent") {
			case 0:
				return boardEvent(t, at, n)
			case 6:
				return linksEvent(t, at, n)
			case 1: // a role removed, or back
				roles := []Role{role("mail", "s2")}
				if rapid.Bool().Draw(t, "dev stays") {
					roles = append(roles, role("dev", "s1"))
				}
				return RosterUpdate{At: at, Roles: roles}
			case 2:
				return ConversationEnded{At: at, Session: session}
			case 3:
				return Attached{At: at, Role: "dev", Err: "no"}
			}
			return Routed{Session: session, Event: PromptDone{At: at, Stop: EndTurn}}
		})
	})
}

func TestSurfaceRulesInTheSingleCore(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		x, _ := feedS(Single{}, Connected{At: t0},
			CardsLoaded{At: t0, Rev: 40, Cards: []Card{linkCard(50, 40), checklist(10, 10, false, true), checklist(30, 40, false), buttonsOn(60)}},
			LinksLoaded{At: t0, Rev: 40, Live: []string{liveA}},
			ItemsLoaded{At: t0, Items: []Item{shownItem}})
		surfaceChecks(t, x, ReduceSingle, SingleSurface, SingleSurfaceID, func(t *rapid.T, at time.Time, n uint64) Event {
			switch rapid.IntRange(0, 2).Draw(t, "agent") {
			case 0:
				return boardEvent(t, at, n)
			case 1:
				return linksEvent(t, at, n)
			}
			return PromptDone{At: at, Stop: EndTurn}
		})
	})
}

// The archive question is about the card at the rev it was asked at.
// Whatever changes, and whenever, a yes only goes out for the card exactly
// as it was when the question came up, ArmDelay before, carrying that
// rev; a change cancels the question with a note.
func TestTheArchiveQuestionIsAboutWhatItShowed(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		at := t0
		x, _ := feedS(Single{}, Connected{At: t0},
			CardsLoaded{At: t0, Rev: 40, Cards: []Card{checklist(10, 10, false, true), checklist(30, 40, false)}},
			KeyPressed{At: t0}, StopTyping{At: t0}, Move{At: t0, Delta: 1})
		if SingleSurface(x).Kind != SurfacePane {
			t.Fatalf("not on the pane: %+v", SingleSurface(x))
		}
		for step := range rapid.IntRange(1, 40).Draw(t, "steps") {
			at = at.Add(time.Duration(rapid.IntRange(0, 700).Draw(t, "ms")) * time.Millisecond)
			var e Event
			switch rapid.IntRange(0, 4).Draw(t, "what") {
			case 0:
				e = PaneKey{At: at, Action: rapid.SampledFrom([]PaneAction{PaneUp, PaneDown, PaneArchive, PaneBoard}).Draw(t, "open")}
			case 1:
				e = boardEvent(t, at, uint64(step))
			default:
				e = PaneKey{At: at, Action: PaneYes}
			}
			x, _ = ReduceSingle(x, KeyPressed{At: at})
			asked, askedRev := x.Board.Confirm, x.Board.ConfirmRev
			var held Card
			if i := cardIndex(x.Board, asked); i >= 0 {
				held = x.Board.Cards[i]
			}
			next, fx := ReduceSingle(x, e)
			for _, f := range fx {
				op, ok := f.(SendCardOp)
				if !ok || op.Op.Op != "archive" {
					continue
				}
				if asked == 0 || op.Card != asked || op.Op.Seen == nil || *op.Op.Seen != askedRev || held.Rev != askedRev {
					t.Fatalf("step %d: archive %+v for the question %d at rev %d (card now at rev %d)", step, op, asked, askedRev, held.Rev)
				}
			}
			x = next
		}
	})
}

func cardIndex(b Board, id int64) int {
	for i, c := range b.Cards {
		if c.ID == id {
			return i
		}
	}
	return -1
}

// buttonsOn is the card of buttons with another id, for the runs that
// move among cards.
func buttonsOn(id int64) Card {
	c := buttonCard(40)
	c.ID = id
	return c
}
