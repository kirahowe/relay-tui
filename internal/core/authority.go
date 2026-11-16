package core

import (
	"fmt"
	"time"
)

// Three things ask a yes of the operator: ending a conversation,
// archiving a card, and keeping or dropping a link. Each shows all it is
// about where it is asked, but a stray key must not answer it. Pressing a
// button of a surface card is a fourth: it wakes a role and costs a model
// turn, so the key that presses it is held to the same rule, with the
// button as what was asked (PressID).
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
	// needsShown is a yes to a link's question pressed while the frame
	// did not show the link's whole host: it is never let through.
	needsShown
)

// EndID names the question whether to end session's conversation.
func EndID(session string) string { return fmt.Sprintf("end/%q", session) }

// ArchiveID names the question whether to archive card id, as it stood
// at rev.
func ArchiveID(id, rev int64) string { return fmt.Sprintf("archive/%d/%d", id, rev) }

// PressID names the button the board's cursor is on, in card id as it
// stood at rev: the thing a press key would act on. A press is armed
// afresh when the cursor arrives on a button, and when the card changes
// in any way, since the label the operator read may be another one.
func PressID(id, rev int64, component string) string {
	return fmt.Sprintf("press/%d/%d/%q", id, rev, component)
}

// pressing is the id of the button the board's cursor is on, or "" when
// it is not on one.
func pressing(a App) string {
	b := a.Board
	if b.Items || !b.Detail {
		return ""
	}
	c, i := BoardCard(b)
	if i < 0 || c.Kind != CardSurface || c.Surface == nil {
		return ""
	}
	line := BoardLine(b, c)
	if line == 0 {
		return ""
	}
	if in := c.Inputs()[line-1]; in.Button {
		return PressID(c.ID, c.Rev, in.ID)
	}
	return ""
}

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
	case SurfaceReadyLinkAsk:
		extra = LinkAskID("ready", *h.Ready.Links.Ask)
	}
	return surfaceID(surf, h.App, extra)
}

// SingleSurfaceID names what has the keys against a single agent.
func SingleSurfaceID(x Single) string { return surfaceID(SingleSurface(x), x.App, "") }

func surfaceID(surf Surface, a App, extra string) string {
	switch surf.Kind {
	case SurfaceArchive:
		return ArchiveID(a.Board.Confirm, a.Board.ConfirmRev)
	case SurfaceLinkAsk:
		return LinkAskID("pane", *a.Board.Links.Ask)
	case SurfaceReadyLinkAsk:
		return extra
	case SurfaceEnd:
		return EndID(extra)
	case SurfaceInput:
		return InputID(extra)
	case SurfaceConversation:
		return fmt.Sprintf("%d/%q", surf.Kind, extra)
	case SurfacePane:
		if id := pressing(a); id != "" {
			return id
		}
	}
	return fmt.Sprintf("%d/%d", surf.Kind, surf.Column)
}

// authorityOf is what e needs, on surface surf of app a.
func authorityOf(e Event, surf Surface, a App) authority {
	switch e := e.(type) {
	case PaneKey:
		if surf.Kind == SurfacePane && e.Action == PaneMark && pressing(a) != "" {
			return needsArmed
		}
		if surf.Kind == SurfaceLinkAsk && e.Action == PaneYes && e.Hidden {
			return needsShown
		}
		if (surf.Kind == SurfaceArchive || surf.Kind == SurfaceLinkAsk) && e.Action == PaneYes {
			return needsArmed
		}
	case ReadyKey:
		if surf.Kind == SurfaceReadyLinkAsk && e.Action == ReadyYes && e.Hidden {
			return needsShown
		}
		if surf.Kind == SurfaceReadyLinkAsk && e.Action == ReadyYes {
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
	switch need {
	case needsArmed:
		return !a.KeyReplayed && a.Became.Surface == id && at.Sub(a.Became.At) >= ArmDelay
	case needsShown:
		return false
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
	case need == needsShown:
		return LinkHiddenNote
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
