package core

import (
	"fmt"
	"slices"
	"time"
)

// Authorization needs a live key after the surface was drawn. A key
// that allows a tool call, archives a card, trusts a card, a doc or a
// paste, or ends a conversation is taken only when the operator pressed
// it (not replayed from input held while connecting) and at least
// ArmDelay after the shell drew the surface it lands on as active. The
// shell says when it has drawn a surface (Drawn); the time comes from
// the shell's drawing, never from a key's time or a reducer's guess, and
// it is forgotten the moment the surface changes, so every surface is
// armed afresh each time it comes on screen. Refusing, cancelling and
// answering no are safe, and need only a live key. Replayed input may
// type and move, and nothing else.

// Drawn says what the renderer drew in full and readable in a frame
// that is now on screen, at At: the identities (RequestID, ConfirmID,
// EndID, InputID) of the things in it that a key could authorize. A
// thing clipped, covered or behind "too small" is not among them. The
// surface with the keys is armed from the first frame that drew it,
// and forgotten in the first that did not. With OnlyForget the frame is
// one the shell knows will replace the last at once: it can forget but
// not arm, as it is not on screen yet.
type Drawn struct {
	At         time.Time
	Surfaces   []string
	OnlyForget bool
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

// ConfirmID names the question the pane asks: a doc's trust, a card's
// archive or a card's trust, each with the version it is about; "" for
// none.
func ConfirmID(a App, s State) string {
	b := a.Board
	switch {
	case s.Pane.Trust != nil && !s.Pane.Trust.Sent:
		t := s.Pane.Trust.Item
		return fmt.Sprintf("doctrust/%q/%s/%s", t.Path, t.Version, t.SHA256)
	case b.Items:
	case b.Confirm != 0:
		return fmt.Sprintf("archive/%d/%d", b.Confirm, b.ConfirmRev)
	case b.Trust != nil && !b.Trust.Sent:
		return fmt.Sprintf("cardtrust/%d/%d/%s", b.Trust.Card, b.Trust.Rev, b.Trust.SHA256)
	}
	return ""
}

// EndID names the question whether to end session's conversation.
func EndID(session string) string { return fmt.Sprintf("end/%q", session) }

// InputID names the input of role ("" against a single agent), where
// ctrl+y trusts a paste.
func InputID(role string) string { return fmt.Sprintf("input/%q", role) }

func (e Drawn) at() time.Time { return e.At }

// Shown is the surface last drawn as active, while it still is.
type Shown struct {
	Surface string
	At      time.Time
}

// authority is what an operator's event needs before it is taken.
type authority int

const (
	needsNothing authority = iota
	needsLiveKey           // safe answers: no, cancel
	needsArmed             // a live key, ArmDelay after the surface was drawn
)

// HubSurfaceID names what has the keys in hub mode, finely enough that
// two different things to authorize never share a name.
func HubSurfaceID(h Hub) string {
	surf := HubSurface(h)
	var st State
	if c := h.Focused(); c != nil && surf.Column != ColumnList {
		st = c.State
	}
	extra := ""
	switch surf.Kind {
	case SurfaceEnd:
		extra = h.EndSession
	case SurfaceInput, SurfaceConversation:
		extra = h.Focus
	}
	return surfaceID(surf, h.App, st, extra)
}

// SingleSurfaceID names what has the keys against a single agent.
func SingleSurfaceID(x Single) string { return surfaceID(SingleSurface(x), x.App, x.State, "") }

func surfaceID(surf Surface, a App, s State, extra string) string {
	switch surf.Kind {
	case SurfaceRequest:
		return RequestID(s)
	case SurfaceConfirm:
		return ConfirmID(a, s)
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
			return needsArmed
		}
		return needsLiveKey
	case PermissionConfirm:
		if len(s.Asks) > 0 && s.Pick >= 0 && s.Pick < len(s.Asks[0].Options) && !s.Asks[0].Options[s.Pick].Kind.Rejects() {
			return needsArmed // an allow, or a kind this client does not know
		}
		return needsLiveKey
	case PermissionCancel:
		return needsLiveKey
	case PaneKey:
		if surf.Kind == SurfaceConfirm && e.Action == PaneYes {
			return needsArmed
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

// mayTake says the app lets e through, given what it needs: a live key,
// and for an authorization, the surface named id drawn ArmDelay before.
func (a App) mayTake(need authority, id string, at time.Time) bool {
	switch need {
	case needsLiveKey:
		return !a.KeyReplayed
	case needsArmed:
		return !a.KeyReplayed && a.Shown.Surface == id && !a.Shown.At.IsZero() && at.Sub(a.Shown.At) >= ArmDelay
	}
	return true
}

// Notes for a key the app did not let through.
const (
	ReplayedNote = "a key typed while connecting answers nothing: press it again"
	TooSoonNote  = "too soon after it came on screen: press it again"
	NotShownNote = "not all of it is on screen: make the window larger, then press it again"
)

// refusal is what to say about an event mayTake refused on the surface
// named id, "" for none.
func (a App) refusal(need authority, id string) string {
	switch {
	case need == needsNothing:
		return ""
	case a.KeyReplayed:
		return ReplayedNote
	case a.Shown.Surface != id:
		return NotShownNote
	}
	return TooSoonNote
}

// ArmedSince is when the surface named id may be authorized from: ArmDelay
// after it was drawn, if it was. The renderer shows a request's or a
// trust's keys as armed from then.
func (a App) ArmedSince(id string) (time.Time, bool) {
	if a.Shown.Surface != id || a.Shown.At.IsZero() {
		return time.Time{}, false
	}
	return a.Shown.At.Add(ArmDelay), true
}

// RequestArmed says the request on offer in s may be allowed at now: it
// was drawn as the surface ArmDelay before.
func RequestArmed(a App, s State, now time.Time) bool {
	if len(s.Asks) == 0 {
		return false
	}
	t, ok := a.ArmedSince(surfaceID(Surface{Kind: SurfaceRequest}, a, s, ""))
	return ok && !now.Before(t)
}

// ConfirmArmed says the question the pane asks (archive, card or doc
// trust) may be answered yes at now.
func ConfirmArmed(a App, s State, now time.Time) bool {
	t, ok := a.ArmedSince(surfaceID(Surface{Kind: SurfaceConfirm}, a, s, ""))
	return ok && !now.Before(t)
}
