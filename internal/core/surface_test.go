package core

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// The surface in the core: whatever event the shell sends, the reducers
// act only where the keys are. These run every operator event at random,
// not only the ones the shell's router would make of a key, so they hold
// even if the router were wrong.

var surfaceOpts = []PermOption{{ID: "allow", Kind: AllowOnce}, {ID: "reject", Kind: RejectOnce}}

// genOperator is any event of the operator's, with random fields.
func genOperator(t *rapid.T, at time.Time) Event {
	actions := []PaneAction{PaneToggle, PaneUp, PaneDown, PagePrev, PageNext, PaneEnter, PaneEsc, PaneTrust, PaneYes,
		PaneBoard, PaneRefresh, PaneLeft, PaneRight, PaneMark, PaneAdd, PaneAddColumn, PaneEdit, PaneEditColumn,
		PaneArchive, PaneChar, PaneBackspace, PaneOther}
	switch rapid.IntRange(0, 25).Draw(t, "op") {
	case 0:
		return Submit{At: at, Text: "hello"}
	case 1:
		return Interrupt{At: at, InputEmpty: rapid.Bool().Draw(t, "empty")}
	case 2:
		return Pasted{At: at, Text: "pasted"}
	case 3:
		return TrustPaste{At: at, Draft: "[pasted 1 line · untrusted #1]"}
	case 4, 5, 6:
		return PaneKey{At: at, Action: rapid.SampledFrom(actions).Draw(t, "action"), Text: "w"}
	case 7:
		return ShowTools{At: at}
	case 8:
		return TranscriptKey{At: at, Action: TranscriptAction(rapid.IntRange(0, 6).Draw(t, "taction")), Max: 10, Page: 3}
	case 9:
		return ToggleHelp{At: at}
	case 10:
		return PermissionMove{At: at, Delta: 1}
	case 11:
		return PermissionConfirm{At: at}
	case 12, 13:
		return PermissionQuick{At: at, Kind: AllowOnce}
	case 14:
		return PermissionCancel{At: at}
	case 15:
		return FocusIndex{At: at, Index: rapid.IntRange(0, 2).Draw(t, "index")}
	case 16, 17:
		return Move{At: at, Delta: rapid.SampledFrom([]int{-1, 1}).Draw(t, "delta")}
	case 18:
		return Type{At: at}
	case 19:
		return StopTyping{At: at}
	case 20:
		return Unbound{At: at}
	case 21:
		return ListMove{At: at, Delta: 1}
	case 22:
		return ListOpen{At: at}
	case 23:
		return AskEnd{At: at}
	case 24:
		return ConfirmEnd{At: at, Yes: true}
	}
	return StartConversation{At: at}
}

// contaminatedCard is a card the operator can be asked to trust.
func contaminatedCard() Card {
	c := checklist(30, 40, false, false)
	c.Labels = Labels{Contaminated: true}
	c.SHA256 = "abc"
	return c
}

// authorizes says an effect acts on the operator's authority: an allow,
// a trust of a card, a doc or a paste, an archive, an end.
func authorizes(f Effect) bool {
	switch f := f.(type) {
	case AnswerPermission:
		return f.OptionID == "allow"
	case SendCardTrust, SendTrust, EndConversation:
		return true
	case SendCardOp:
		return f.Op.Op == "archive"
	case ReplaceInput:
		return strings.Contains(f.New, "· trusted #")
	}
	return false
}

// surfaceChecks runs events at random through reduce, as the shell
// sends them: each operator event after a KeyPressed, live or replayed,
// the agent's events, and Drawn for what has the keys. It checks that a
// typing surface's event never answers, trusts, confirms or sends a card
// op but the editor's own; that an event of the agent's never takes the
// keys from a typing surface; and that nothing is authorized but by a
// live key at least ArmDelay after the surface it lands on was drawn.
func surfaceChecks[S any](t *rapid.T, x S, reduce func(S, Event) (S, []Effect), surface func(S) Surface,
	surfaceID func(S) string, agent func(t *rapid.T, at time.Time, n uint64) Event) {
	at := t0
	// A key first: before any, the start may move the keys (App.Pressed).
	x, _ = reduce(x, KeyPressed{At: t0})
	drawnID, drawnAt := surfaceID(x), t0 // the set-up drew it at t0
	var n uint64
	for step := range rapid.IntRange(1, 100).Draw(t, "steps") {
		at = at.Add(time.Duration(rapid.IntRange(0, 900).Draw(t, "ms")) * time.Millisecond)
		surf, id := surface(x), surfaceID(x)
		var e Event
		replayed := false
		switch k := rapid.IntRange(0, 9).Draw(t, "kind"); {
		case k == 0:
			n++
			e = agent(t, at, n)
		case k <= 2:
			e = Drawn{At: at, Surface: id}
			if drawnID != id {
				drawnID, drawnAt = id, at
			}
		default:
			replayed = rapid.IntRange(0, 3).Draw(t, "replayed") == 0
			x, _ = reduce(x, KeyPressed{At: at, Replayed: replayed})
			e = genOperator(t, at)
		}
		_, operator := e.(Drawn)
		operator = !operator && fromOperator(e)
		next, fx := reduce(x, e)
		for _, f := range fx {
			if ff, ok := f.(ForSession); ok {
				f = ff.Effect
			}
			if surf.Typing() && operator {
				switch f := f.(type) {
				case AnswerPermission:
					if !f.Cancelled {
						t.Fatalf("step %d: %T while typing (%v) answered", step, e, surf.Kind)
					}
				case SendCardTrust, SendTrust, EndConversation:
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
				case drawnID != id || at.Sub(drawnAt) < ArmDelay:
					t.Fatalf("step %d: %T authorized %#v on %q, drawn %q %v before", step, e, f, id, drawnID, at.Sub(drawnAt))
				}
			}
		}
		if !operator && surf.Typing() && surface(next) != surf {
			t.Fatalf("step %d: %T took the keys from %+v to %+v", step, e, surf, surface(next))
		}
		x = next
		if surfaceID(x) != id {
			drawnID = "" // a new surface, not yet drawn
		}
	}
}

// boardEvent is a change to the board from outside: a card changed, a
// card archived (the one an editor may be open on), or the board sent
// again without one.
func boardEvent(t *rapid.T, at time.Time, n uint64) Event {
	rev := int64(100 + n)
	switch rapid.IntRange(0, 2).Draw(t, "board") {
	case 0:
		c := checklist(10, rev, false, true)
		return CardsChanged{At: at, Rev: rev, Cards: []Card{c}}
	case 1:
		c := checklist(rapid.SampledFrom([]int64{10, 30}).Draw(t, "archived"), rev)
		c.Status = CardArchived
		return CardsChanged{At: at, Rev: rev, Cards: []Card{c}}
	}
	return CardsLoaded{At: at, Rev: rev, Cards: []Card{contaminatedCard()}}
}

func TestSurfaceRulesInTheHubCore(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		h, _ := apply(t, NewHub("dev"), HubConnected{At: t0}, RosterUpdate{At: t0, Roles: []Role{role("dev", "s1"), role("mail", "s2")}},
			Attached{At: t0, Role: "dev", Session: "s1"}, Attached{At: t0, Role: "mail", Session: "s2"},
			CardsLoaded{At: t0, Rev: 40, Cards: []Card{checklist(10, 10, false, true), contaminatedCard()}})
		surfaceChecks(t, h, ReduceHub, HubSurface, HubSurfaceID, func(t *rapid.T, at time.Time, n uint64) Event {
			session := rapid.SampledFrom([]string{"s1", "s2"}).Draw(t, "session")
			switch rapid.IntRange(0, 7).Draw(t, "agent") {
			case 0:
				return boardEvent(t, at, n)
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
			case 4:
				return Routed{Session: session, Event: PromptDone{At: at, Stop: EndTurn}}
			case 5:
				return PermissionSettled{At: at, Session: session, CallID: fmt.Sprint(n - 1)}
			}
			return Routed{Session: session, Event: PermissionAsked{At: at, Permission: Permission{ID: n, CallID: fmt.Sprint(n), Options: surfaceOpts}}}
		})
	})
}

func TestSurfaceRulesInTheSingleCore(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		x, _ := feedS(Single{}, Connected{At: t0, Embedded: true},
			CardsLoaded{At: t0, Rev: 40, Cards: []Card{checklist(10, 10, false, true), contaminatedCard()}},
			ItemsLoaded{At: t0, Items: []Item{trustable}})
		surfaceChecks(t, x, ReduceSingle, SingleSurface, SingleSurfaceID, func(t *rapid.T, at time.Time, n uint64) Event {
			switch rapid.IntRange(0, 3).Draw(t, "agent") {
			case 0:
				return boardEvent(t, at, n)
			case 1:
				return PromptDone{At: at, Stop: EndTurn}
			}
			return PermissionAsked{At: at, Permission: Permission{ID: n, CallID: fmt.Sprint(n), Options: surfaceOpts}}
		})
	})
}
