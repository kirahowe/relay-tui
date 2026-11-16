package core

import (
	"fmt"
	"strings"
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

// Reveal says whether the button the focus is on was on the screen when
// the last key came, and how many times it has come back on screen since
// the focus reached it. Key names the button.
type Reveal struct {
	Key   string
	Shown bool
	N     int
}

// CardDrawn is the shell's word on what the frame now shows of the open
// card, sent after any update that may have changed it: a resize, a card
// arriving, scrolling, the focus moving. Card.Shown says the focused line
// is fully on the screen.
type CardDrawn struct {
	At   time.Time
	Card CardScroll
}

func (e CardDrawn) at() time.Time { return e.At }

// DrawnHub and DrawnSingle fold CardDrawn into the state. It is not an
// event the reducers act on: it only keeps what arming knows of the frame,
// and so leaves the notices and everything else as they are.
func DrawnHub(h Hub, e CardDrawn) Hub {
	h.App = noteReveal(h.App, e)
	h.App = settleSurface(h.App, HubSurfaceID(h), e.At)
	return h
}

// DrawnSingle is DrawnHub against a single agent.
func DrawnSingle(x Single, e CardDrawn) Single {
	x.App = noteReveal(x.App, e)
	x.App = settleSurface(x.App, SingleSurfaceID(x), e.At)
	return x
}

// WantsDrawn says the shell should tell the core what the frame shows of
// the open card (CardDrawn): the focus is on a button, or the core still
// holds what it knew of one.
func WantsDrawn(a App) bool { return pressing(a) != "" || a.Board.Reveal != (Reveal{}) }

// noteReveal updates a.Board.Reveal from what the frame shows of the open
// card's focused line, as a key of the pane carries it or CardDrawn tells
// it. A press is armed from when the button came (back) on screen, so a
// button that was scrolled away, brought back by x or a resize, or too
// tall to be whole until now, is a new thing to arm: the operator must
// see its whole label for ArmDelay before a key can press it. When the
// focus first comes to a button, its state is what the frame shows, never
// assumed. It runs before the event's authority is decided, and the ids it
// changes are the ones the event is judged against.
func noteReveal(a App, e Event) App {
	var v *CardScroll
	switch e := e.(type) {
	case PaneKey:
		v = e.Card
	case CardDrawn:
		v = &e.Card
	}
	if v == nil {
		return a
	}
	id := pressing(a)
	if id == "" {
		a.Board.Reveal = Reveal{}
		return a
	}
	key := id[:strings.LastIndex(id, "/")]
	c, _ := BoardCard(a.Board)
	shown := v.Card == c.ID && v.Shown
	r := a.Board.Reveal
	if r.Key != key {
		r = Reveal{Key: key, Shown: shown}
	} else if shown && !r.Shown {
		r.N++
	}
	r.Shown = shown
	a.Board.Reveal = r
	return a
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
		id := PressID(c.ID, c.Rev, in.ID)
		n := 0
		if b.Reveal.Key == id {
			n = b.Reveal.N
		}
		return fmt.Sprintf("%s/%d", id, n)
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

// RearmHub starts the current surface's wait afresh at at: a question or
// a button that came up while the screen said "connecting" was never
// drawn, so it is armed from the first moment the operator can see it.
func RearmHub(h Hub, at time.Time) Hub {
	h.App.Became = Since{Surface: HubSurfaceID(h), At: at}
	return h
}

// RearmSingle is RearmHub against a single agent.
func RearmSingle(x Single, at time.Time) Single {
	x.App.Became = Since{Surface: SingleSurfaceID(x), At: at}
	return x
}
