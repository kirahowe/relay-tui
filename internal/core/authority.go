package core

import (
	"fmt"
	"time"
)

// Two things ask a yes of the operator: ending a conversation and
// archiving a card. Both act on nothing outside relay and show all they
// are about where they are asked, but a stray key must not answer them.
// A yes is taken only from a key the operator pressed, not one replayed
// from input held while connecting, and ArmDelay after the question came
// up (App.Became), so a key typed for what was there before cannot answer
// it. Saying no, cancelling and going back need nothing. Replayed input
// may type and move, and nothing else.

// ArmDelay is how long a question is up before a key can answer it yes.
const ArmDelay = 600 * time.Millisecond

// Since is a surface and when it got the keys.
type Since struct {
	Surface string
	At      time.Time
}

// authority is what an operator's event needs before it is taken.
type authority int

const (
	needsNothing authority = iota
	needsArmed             // a live key, ArmDelay after the surface came up
)

// EndID names the question whether to end session's conversation.
func EndID(session string) string { return fmt.Sprintf("end/%q", session) }

// ArchiveID names the question whether to archive card id, as it stood
// at rev.
func ArchiveID(id, rev int64) string { return fmt.Sprintf("archive/%d/%d", id, rev) }

// InputID names the input of role ("" against a single agent).
func InputID(role string) string { return fmt.Sprintf("input/%q", role) }

// HubSurfaceID names what has the keys in hub mode, finely enough that
// two different things to answer never share a name.
func HubSurfaceID(h Hub) string {
	surf := HubSurface(h)
	extra := ""
	switch surf.Kind {
	case SurfaceEnd:
		extra = h.EndSession
	case SurfaceInput, SurfaceConversation:
		extra = h.Focus
	}
	return surfaceID(surf, h.App, extra)
}

// SingleSurfaceID names what has the keys against a single agent.
func SingleSurfaceID(x Single) string { return surfaceID(SingleSurface(x), x.App, "") }

func surfaceID(surf Surface, a App, extra string) string {
	switch surf.Kind {
	case SurfaceArchive:
		return ArchiveID(a.Board.Confirm, a.Board.ConfirmRev)
	case SurfaceEnd:
		return EndID(extra)
	case SurfaceInput:
		return InputID(extra)
	case SurfaceConversation:
		return fmt.Sprintf("%d/%q", surf.Kind, extra)
	}
	return fmt.Sprintf("%d/%d", surf.Kind, surf.Column)
}

// authorityOf is what e needs, on surface surf.
func authorityOf(e Event, surf Surface) authority {
	switch e := e.(type) {
	case PaneKey:
		if surf.Kind == SurfaceArchive && e.Action == PaneYes {
			return needsArmed
		}
	case ConfirmEnd:
		if e.Yes {
			return needsArmed
		}
	}
	return needsNothing
}

// mayTake says the app lets an event that needs need through, on the
// surface named id.
func (a App) mayTake(need authority, id string, at time.Time) bool {
	if need == needsArmed {
		return !a.KeyReplayed && a.Became.Surface == id && at.Sub(a.Became.At) >= ArmDelay
	}
	return true
}

// Notes for a key the app did not let through.
const (
	ReplayedNote = "a key typed while connecting answers nothing: press it again"
	TooSoonNote  = "too soon after it came on screen: press it again"
)

// refusal is what to say about an event mayTake refused.
func (a App) refusal(need authority) string {
	switch {
	case need == needsNothing:
		return ""
	case a.KeyReplayed:
		return ReplayedNote
	}
	return TooSoonNote
}

// settleSurface is the bookkeeping after an event, once the surface it
// leaves is known: a surface that has just come up is armed afresh (its
// Became is now).
func settleSurface(a App, id string, at time.Time) App {
	if a.Became.Surface != id {
		a.Became = Since{Surface: id, At: at}
	}
	return a
}
