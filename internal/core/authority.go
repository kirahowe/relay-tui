package core

import (
	"fmt"
	"slices"
	"time"
)

// Authorization needs a live key: one the operator pressed, not one
// replayed from input held while connecting. Consent to a subject (an
// allow, a card's or a doc's trust, a card's archive) is taken only from
// its review, once all of it was drawn, ArmDelay after the last part
// came on screen (see review.go). The two inline questions, ending a
// conversation and trusting a paste (ctrl+y), act on nothing outside and
// show all they are about where they are asked; they are taken ArmDelay
// after their surface came up (App.Became), so a key typed for what was
// there before cannot answer them. Refusing, cancelling and answering no
// need only a live key. Replayed input may type and move, and nothing
// else.

// Drawn says the review named in Surfaces was drawn, by a frame now on
// screen at At, and what of its text the frame drew (Review). The shell
// sends it only for the review pager. A frame that drew no review (it
// does not fit, or fails closed) names none, and the review is forgotten
// as drawn. With OnlyForget the frame is one the shell knows will
// replace the last at once: it can forget but not arm, as it is not on
// screen yet.
type Drawn struct {
	At         time.Time
	Surfaces   []string
	OnlyForget bool
	Review     *ReviewDrawn
}

// shown is the app after e, for the surface named id.
func (a App) shown(e Drawn, id string) App {
	switch {
	case !slices.Contains(e.Surfaces, id):
		a.Shown = Shown{}
	case a.Shown.Surface != id && !e.OnlyForget:
		a.Shown = Shown{Surface: id, At: e.At}
	}
	return a
}

// RequestID names the request on offer in s.
func RequestID(s State) string {
	if len(s.Asks) == 0 {
		return ""
	}
	return fmt.Sprintf("request/%d", s.Asks[0].ID)
}

// EndID names the question whether to end session's conversation.
func EndID(session string) string { return fmt.Sprintf("end/%q", session) }

// InputID names the input of role ("" against a single agent), where
// ctrl+y trusts a paste.
func InputID(role string) string { return fmt.Sprintf("input/%q", role) }

func (e Drawn) at() time.Time { return e.At }

// Shown is a surface and a time: the review last drawn as active, while
// it still is (App.Shown), or the surface that has the keys and when it
// got them (App.Became).
type Shown struct {
	Surface string
	At      time.Time
}

// authority is what an operator's event needs before it is taken.
type authority int

const (
	needsNothing       authority = iota
	needsLiveKey                 // safe answers: no, cancel, back, scrolling a review
	needsArmed                   // a live key, ArmDelay after the surface came up
	needsReview                  // a live key, the review read whole ArmDelay ago
	needsReviewAndPick           // needsReview, and the highlighted option drawn whole ArmDelay ago
)

// HubSurfaceID names what has the keys in hub mode, finely enough that
// two different things to authorize never share a name.
func HubSurfaceID(h Hub) string {
	surf := HubSurface(h)
	return surfaceID(surf, h.App, h.surfaceState(surf), h.surfaceExtra(surf))
}

// surfaceState is the conversation under surf: the one in focus, unless
// the list has the keys.
func (h Hub) surfaceState(surf Surface) State {
	if c := h.Focused(); c != nil && surf.Column != ColumnList {
		return c.State
	}
	return State{}
}

func (h Hub) surfaceExtra(surf Surface) string {
	switch surf.Kind {
	case SurfaceEnd:
		return h.EndSession
	case SurfaceInput, SurfaceConversation:
		return h.Focus
	}
	return ""
}

// SingleSurfaceID names what has the keys against a single agent.
func SingleSurfaceID(x Single) string { return surfaceID(SingleSurface(x), x.App, x.State, "") }

func surfaceID(surf Surface, a App, s State, extra string) string {
	switch surf.Kind {
	case SurfaceRequest:
		return "summary/" + RequestID(s)
	case SurfaceReview:
		return ReviewSubject(surf, a, s).ID
	case SurfaceEnd:
		return EndID(extra)
	case SurfaceInput:
		return InputID(extra)
	case SurfaceConversation:
		return fmt.Sprintf("%d/%q", surf.Kind, extra)
	}
	return fmt.Sprintf("%d/%d", surf.Kind, surf.Column)
}

// authorityOf is what e needs, on surface surf over conversation s.
func authorityOf(e Event, surf Surface, s State) authority {
	switch e := e.(type) {
	case PermissionQuick:
		if e.Kind.Allows() {
			return needsReview
		}
		return needsLiveKey
	case PermissionConfirm:
		if len(s.Asks) > 0 && s.Pick >= 0 && s.Pick < len(s.Asks[0].Options) && !s.Asks[0].Options[s.Pick].Kind.Rejects() {
			// An allow, or a kind this client does not know: what is sent
			// is the highlighted option, so it must have been seen too.
			return needsReviewAndPick
		}
		return needsLiveKey
	case PermissionCancel, PermissionMove, OpenReview, ReviewKey:
		return needsLiveKey
	case PaneKey:
		if surf.Kind == SurfaceReview {
			if e.Action == PaneYes {
				return needsReview
			}
			return needsLiveKey
		}
	case ConfirmEnd:
		if e.Yes {
			return needsArmed
		}
	case TrustPaste:
		return needsArmed
	}
	return needsNothing
}

// mayTake says the app lets e through, given what it needs, on the
// surface surf named id over conversation s.
func (a App) mayTake(need authority, surf Surface, id string, s State, at time.Time) bool {
	switch need {
	case needsLiveKey:
		return !a.KeyReplayed
	case needsArmed:
		return !a.KeyReplayed && a.Became.Surface == id && at.Sub(a.Became.At) >= ArmDelay
	case needsReview:
		return a.reviewArmed(ReviewSubject(surf, a, s), at)
	case needsReviewAndPick:
		return a.reviewArmed(ReviewSubject(surf, a, s), at) && a.pickArmed(s, at)
	}
	return true
}

// Notes for a key the app did not let through.
const (
	ReplayedNote = "a key typed while connecting answers nothing: press it again"
	TooSoonNote  = "too soon after it came on screen: press it again"
	NotShownNote = "the review has not been on screen long enough, or does not fit the window: press it again"
)

// refusal is what to say about an event mayTake refused at at.
func (a App) refusal(need authority, surf Surface, s State, at time.Time) string {
	switch {
	case need == needsNothing:
		return ""
	case need == needsReview:
		return a.reviewRefusal(ReviewSubject(surf, a, s))
	case need == needsReviewAndPick:
		if sub := ReviewSubject(surf, a, s); !a.reviewArmed(sub, at) {
			return a.reviewRefusal(sub)
		}
		return PickNote
	case a.KeyReplayed:
		return ReplayedNote
	}
	return TooSoonNote
}

// settleSurface is the bookkeeping after an event, once the surface it
// leaves is known: a surface that has just come up is armed afresh (its
// Became is now, and it is not drawn yet), and the pager follows the
// review.
func settleSurface(a App, surf Surface, id string, at time.Time) App {
	if a.Became.Surface != id {
		a.Became = Shown{Surface: id, At: at}
		a.Shown = Shown{}
	}
	return syncPager(a, surf, id)
}
