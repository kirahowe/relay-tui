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
	switch rapid.IntRange(0, 28).Draw(t, "op") {
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
	case 25, 26:
		return OpenReview{At: at}
	case 27:
		return ReviewKey{At: at, Action: ReviewAction(rapid.IntRange(0, int(ReviewBack)).Draw(t, "raction")), Max: 3, Page: 2}
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

// drawSingle and drawHub are a renderer's report after an event: the
// review that has the keys drawn whole, or only its first half (part).
func drawSingle(x Single, at time.Time, part bool) Drawn {
	return halve(wholeReview(SingleSurface(x), x.App, x.State, at), part)
}

func drawHub(h Hub, at time.Time, part bool) Drawn {
	surf := HubSurface(h)
	return halve(wholeReview(surf, h.App, h.surfaceState(surf), at), part)
}

func halve(d Drawn, part bool) Drawn {
	if part && d.Review != nil {
		r := *d.Review
		r.To /= 2
		d.Review = &r
	}
	return d
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
// the agent's events, and Drawn for the review that has the keys, whole
// or in part. It checks that a typing surface's event never answers,
// trusts, confirms or sends a card op but the editor's own; that an
// event of the agent's never takes the keys from a typing surface; that
// no consent (an allow, a card's or a doc's trust, an archive) is given
// but by a live key from its review, all of which was drawn ArmDelay
// before, about what it showed then; and that the inline questions (an
// end, a paste's trust) are answered yes only by a live key ArmDelay
// after they came up.
func surfaceChecks[S any](t *rapid.T, x S, reduce func(S, Event) (S, []Effect), surface func(S) Surface,
	surfaceID func(S) string, shows func(S) string, agent func(t *rapid.T, at time.Time, n uint64) Event,
	draw func(S, time.Time, bool) Drawn) {
	at := t0
	// A key first: before any, the start may move the keys (App.Pressed).
	x, _ = reduce(x, KeyPressed{At: t0})
	became, becameAt := surfaceID(x), t0
	readID, readAt, readShows := "", time.Time{}, ""
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
			d := draw(x, at, k == 2)
			e = d
			if d.Review != nil && k == 1 && readID != id {
				readID, readAt, readShows = id, at, shows(x)
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
				_, end := f.(EndConversation)
				_, paste := f.(ReplaceInput)
				switch {
				case !operator || replayed:
					t.Fatalf("step %d: %T (operator %v, replayed %v) authorized %#v", step, e, operator, replayed, f)
				case end || paste:
					if became != id || at.Sub(becameAt) < ArmDelay {
						t.Fatalf("step %d: %T answered %#v on %q, up since %q %v before", step, e, f, id, became, at.Sub(becameAt))
					}
				case surf.Kind != SurfaceReview:
					t.Fatalf("step %d: %T consented %#v from %v, not a review", step, e, f, surf.Kind)
				case readID != id || at.Sub(readAt) < ArmDelay:
					t.Fatalf("step %d: %T consented %#v on %q, read whole %q %v before", step, e, f, id, readID, at.Sub(readAt))
				case shows(x) != readShows:
					t.Fatalf("step %d: %T consented %#v on %q, which shows %q, read showing %q", step, e, f, id, shows(x), readShows)
				}
			}
		}
		if !operator && surf.Typing() && surface(next) != surf {
			t.Fatalf("step %d: %T took the keys from %+v to %+v", step, e, surf, surface(next))
		}
		x = next
		if surfaceID(x) != id {
			became, becameAt, readID = surfaceID(x), at, "" // a new surface, up now, not yet drawn
		}
	}
}

// confirmShows is what a question the pane asks is about, as the board
// and the items hold it now: the card's rev and digest, or the doc's
// version. It is read from the board, not from the question, so a
// question that kept pointing at a card that changed shows a change.
func confirmShows(a App, s State) string {
	b := a.Board
	card := func(id int64) string {
		for _, c := range b.Cards {
			if c.ID == id {
				return fmt.Sprintf("%d/%d/%v/%q", c.ID, c.Rev, c.Review, c.Title.Text)
			}
		}
		return "gone"
	}
	switch {
	case b.Confirm != 0:
		return "archive " + card(b.Confirm)
	case b.Trust != nil:
		return "trust " + card(b.Trust.Card)
	case s.Pane.Trust != nil:
		for _, it := range PaneItems(s) {
			if it.Path == s.Pane.Trust.Item.Path {
				return fmt.Sprintf("doc %s/%v", it.Version, it.Review)
			}
		}
		return "doc gone"
	}
	return ""
}

// boardEvent is a change to the board from outside: a card changed, a
// card archived (the one an editor may be open on), or the board sent
// again without one.
func boardEvent(t *rapid.T, at time.Time, n uint64) Event {
	rev := int64(100 + n)
	switch rapid.IntRange(0, 3).Draw(t, "board") {
	case 0:
		c := checklist(10, rev, false, true)
		c.Title.Text = fmt.Sprint("retitled ", n)
		return CardsChanged{At: at, Rev: rev, Cards: []Card{c}}
	case 3:
		c := contaminatedCard()
		c.Rev, c.SHA256 = rev, fmt.Sprint("sha", n)
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
		surfaceChecks(t, h, ReduceHub, HubSurface, HubSurfaceID, func(h Hub) string {
			var s State
			if c := h.Focused(); c != nil {
				s = c.State
			}
			return confirmShows(h.App, s)
		}, func(t *rapid.T, at time.Time, n uint64) Event {
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
		}, drawHub)
	})
}

func TestSurfaceRulesInTheSingleCore(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		x, _ := feedS(Single{}, Connected{At: t0, Embedded: true},
			CardsLoaded{At: t0, Rev: 40, Cards: []Card{checklist(10, 10, false, true), contaminatedCard()}},
			ItemsLoaded{At: t0, Items: []Item{trustable}})
		surfaceChecks(t, x, ReduceSingle, SingleSurface, SingleSurfaceID, func(x Single) string { return confirmShows(x.App, x.State) },
			func(t *rapid.T, at time.Time, n uint64) Event {
				switch rapid.IntRange(0, 3).Draw(t, "agent") {
				case 0:
					return boardEvent(t, at, n)
				case 1:
					return PromptDone{At: at, Stop: EndTurn}
				}
				return PermissionAsked{At: at, Permission: Permission{ID: n, CallID: fmt.Sprint(n), Options: surfaceOpts}}
			}, drawSingle)
	})
}

// A question's identity is the version of what it shows: an archive or
// a trust is about the card at the rev it was asked at, and a doc's trust
// about the version listed. Whatever changes, and whenever, a yes only
// ever goes out for the object exactly as it was when the question was
// drawn, ArmDelay before; a change cancels the question with a note.
func TestAQuestionIsAboutWhatItShowed(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		at := t0
		x, _ := feedS(Single{}, Connected{At: t0, Embedded: true},
			CardsLoaded{At: t0, Rev: 40, Cards: []Card{checklist(10, 10, false, true), contaminatedCard()}},
			ItemsLoaded{At: t0, Items: []Item{trustable}}, KeyPressed{At: t0}, StopTyping{At: t0}, Move{At: t0, Delta: 1})
		if SingleSurface(x).Kind != SurfacePane {
			t.Fatalf("not on the pane: %+v", SingleSurface(x))
		}
		drawnShows, drawnAt := "", time.Time{}
		for step := range rapid.IntRange(1, 40).Draw(t, "steps") {
			at = at.Add(time.Duration(rapid.IntRange(0, 700).Draw(t, "ms")) * time.Millisecond)
			var e Event
			switch rapid.IntRange(0, 6).Draw(t, "what") {
			case 0:
				e = PaneKey{At: at, Action: rapid.SampledFrom([]PaneAction{PaneUp, PaneDown, PaneArchive, PaneTrust, PaneBoard}).Draw(t, "open")}
			case 1:
				e = Drawn{At: at, Surfaces: []string{SingleSurfaceID(x)}}
				if confirmShows(x.App, x.State) != drawnShows {
					drawnShows, drawnAt = confirmShows(x.App, x.State), at
				}
			case 2:
				e = boardEvent(t, at, uint64(step))
			case 3:
				it := trustable
				it.Version = rapid.SampledFrom([]string{trustable.Version, "999"}).Draw(t, "version")
				e = ItemsLoaded{At: at, Items: []Item{it}}
			default:
				e = PaneKey{At: at, Action: PaneYes}
			}
			x, _ = ReduceSingle(x, KeyPressed{At: at})
			before := confirmShows(x.App, x.State)
			next, fx := ReduceSingle(x, e)
			for _, f := range fx {
				if !authorizes(f) {
					continue
				}
				if before != drawnShows || at.Sub(drawnAt) < ArmDelay {
					t.Fatalf("step %d: %#v about %q, drawn showing %q %v before", step, f, before, drawnShows, at.Sub(drawnAt))
				}
				if op, ok := f.(SendCardOp); ok && (op.Op.Seen == nil || !strings.Contains(before, fmt.Sprintf("%d/%d/", op.Card, *op.Op.Seen))) {
					t.Fatalf("step %d: archive %+v does not carry the rev shown, %q", step, op, before)
				}
			}
			if confirmShows(next.App, next.State) != before && next.Board.Confirm != 0 && x.Board.Confirm == next.Board.Confirm {
				t.Fatalf("step %d: the archive question outlived a change to its card: %q to %q", step, before, confirmShows(next.App, next.State))
			}
			x = next
		}
	})
}

// A doc's trust is cancelled with a note when relay lists a newer
// version of the doc while the question waits.
func TestADocsTrustGoesWithANewVersion(t *testing.T) {
	s := focused(trustable)
	s, _ = feed(s, key(t0, PaneTrust))
	newer := trustable
	newer.Version, newer.SHA256 = "413", "ffff"
	s, _ = feed(s, ItemsLoaded{At: t0, Items: []Item{newer}})
	if s.Pane.Trust != nil || s.Pane.Message != "The doc changed, so nothing was trusted. Look at it again." {
		t.Fatalf("trust %+v message %q", s.Pane.Trust, s.Pane.Message)
	}
	if _, fx := feed(s, key(t0.Add(time.Hour), PaneYes)); len(effectsOf[SendTrust](fx)) != 0 {
		t.Fatalf("effects %v", fx)
	}
}
