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

// surfaceChecks runs events at random through reduce, and checks that a
// typing surface's event never answers, trusts, confirms or sends a card
// op but the editor's own, that an event of the agent's never takes the
// keys from a typing surface, and that no request is answered (other
// than cancelled) within ArmDelay of becoming the surface.
func surfaceChecks[S any](t *rapid.T, x S, reduce func(S, Event) (S, []Effect), surface func(S) Surface,
	offered func(S) string, ask func(at time.Time, id uint64) Event) {
	at := t0
	since, offer := time.Time{}, ""
	var id uint64
	for step := range rapid.IntRange(1, 80).Draw(t, "steps") {
		at = at.Add(time.Duration(rapid.IntRange(0, 900).Draw(t, "ms")) * time.Millisecond)
		surf := surface(x)
		var e Event
		operator := rapid.IntRange(0, 4).Draw(t, "kind") > 0
		if operator {
			e = genOperator(t, at)
		} else {
			id++
			e = ask(at, id)
		}
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
			if a, ok := f.(AnswerPermission); ok && !a.Cancelled && at.Sub(since) < ArmDelay {
				t.Fatalf("step %d: %T answered %v after the request became the surface", step, e, at.Sub(since))
			}
		}
		if !operator && surf.Typing() && surface(next) != surf {
			t.Fatalf("step %d: a request took the keys from %+v to %+v", step, surf, surface(next))
		}
		x = next
		if o := offered(x); o != offer {
			offer, since = o, at
		}
	}
}

func TestSurfaceRulesInTheHubCore(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		h, _ := apply(t, NewHub("dev"), HubConnected{At: t0}, RosterUpdate{At: t0, Roles: []Role{role("dev", "s1"), role("mail", "s2")}},
			Attached{At: t0, Role: "dev", Session: "s1"}, Attached{At: t0, Role: "mail", Session: "s2"},
			CardsLoaded{At: t0, Rev: 40, Cards: []Card{checklist(10, 10, false, true), contaminatedCard()}})
		surfaceChecks(t, h, ReduceHub, HubSurface, func(h Hub) string {
			if HubSurface(h).Kind != SurfaceRequest {
				return ""
			}
			c := h.Focused()
			return fmt.Sprintf("%s/%d", c.Session, c.State.Asks[0].ID)
		}, func(at time.Time, id uint64) Event {
			session := "s1"
			if id%3 == 0 {
				session = "s2"
			}
			return Routed{Session: session, Event: PermissionAsked{At: at, Permission: Permission{ID: id, CallID: fmt.Sprint(id), Options: surfaceOpts}}}
		})
	})
}

func TestSurfaceRulesInTheSingleCore(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		x, _ := feedS(Single{}, Connected{At: t0, Embedded: true},
			CardsLoaded{At: t0, Rev: 40, Cards: []Card{checklist(10, 10, false, true), contaminatedCard()}},
			ItemsLoaded{At: t0, Items: []Item{trustable}})
		surfaceChecks(t, x, ReduceSingle, SingleSurface, func(x Single) string {
			if SingleSurface(x).Kind != SurfaceRequest {
				return ""
			}
			return fmt.Sprint(x.State.Asks[0].ID)
		}, func(at time.Time, id uint64) Event {
			return PermissionAsked{At: at, Permission: Permission{ID: id, CallID: fmt.Sprint(id), Options: surfaceOpts}}
		})
	})
}
