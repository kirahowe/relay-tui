package core

import (
	"slices"
	"time"
)

// Allowing a call vouches for its arguments, so the whole input must have
// been on screen first. The request box shows the input when it fits;
// past that, v opens the input view, which shows all of it, wrapped and
// never cut, a screen at a time. The renderer reports, with each frame,
// which parts of the input it drew (InputDrawn, by line and rune range of
// the input as shown), and the shell's Drawn carries that. The request
// keeps the union (Permission.Seen); an allow is taken only once every
// line has been drawn whole, at some point, and ArmDelay after the last
// part was. Refusing and cancelling need no reading.

// Span is part of a line of the input as shown: runes From to To of line
// Line.
type Span struct{ Line, From, To int }

// InputDrawn is what a frame drew of the input of the request named
// Request (RequestID): the length in runes of each line of the input as
// shown, and the spans of it on screen.
type InputDrawn struct {
	Request string
	Lens    []int
	Spans   []Span
}

// InputView is the input of request Ask opened with v: Scroll is the
// first row shown. It shows only while Ask is the request on offer.
type InputView struct {
	Ask    uint64
	Scroll int
}

// viewing says the input view is open on the request on offer.
func (s State) viewing() bool {
	return s.InputView != nil && len(s.Asks) > 0 && s.InputView.Ask == s.Asks[0].ID
}

// InputViewKey scrolls or closes the input view. Max is how far it can
// scroll as drawn, and Page how many rows a page is.
type InputViewKey struct {
	At        time.Time
	Action    TranscriptAction
	Max, Page int
}

func (e InputViewKey) at() time.Time { return e.At }

// InputNote is what a too early allow says while the input has not all
// been on screen.
const InputNote = "read the whole input with v to allow"

// seeInput adds what a frame drew of the input to the request on offer.
func seeInput(s *State, d InputDrawn, at time.Time) {
	if len(s.Asks) == 0 || RequestID(*s) != d.Request || len(d.Spans) == 0 {
		return
	}
	s.Asks = slices.Clone(s.Asks)
	p := &s.Asks[0]
	if !slices.Equal(p.SeenLens, d.Lens) {
		p.SeenLens, p.Seen, p.SeenAt = slices.Clone(d.Lens), nil, time.Time{} // another rendering: read it again
	}
	p.Seen = append(slices.Clone(p.Seen), d.Spans...)
	if p.SeenAt.IsZero() && p.inputSeen() {
		p.SeenAt = at
	}
}

// inputSeen says every line of the input has been drawn whole.
func (p Permission) inputSeen() bool {
	if len(FaithfulLines(p.Input)) == 0 {
		return true
	}
	if len(p.SeenLens) == 0 {
		return false
	}
	for line, n := range p.SeenLens {
		covered := 0
		for covered < n {
			next := covered
			for _, sp := range p.Seen {
				if sp.Line == line && sp.From <= covered && sp.To > next {
					next = sp.To
				}
			}
			if next == covered {
				return false
			}
			covered = next
		}
	}
	return true
}

// inputArmed says the request on offer may be allowed at at, as far as
// its input goes: all of it was on screen, ArmDelay before.
func inputArmed(s State, at time.Time) bool {
	if len(s.Asks) == 0 {
		return false
	}
	p := s.Asks[0]
	if len(FaithfulLines(p.Input)) == 0 {
		return true // nothing to read
	}
	return p.inputSeen() && !p.SeenAt.IsZero() && at.Sub(p.SeenAt) >= ArmDelay
}

// InputUnread says the request on offer has input not yet all on screen.
func InputUnread(s State) bool { return len(s.Asks) > 0 && !s.Asks[0].inputSeen() }

// inputRefusal is what an allow refused for its input says.
func inputRefusal(s State) string {
	if InputUnread(s) {
		return InputNote
	}
	return TooSoonNote
}

// isAllow says e would allow the request on offer in s.
func isAllow(e Event, s State) bool {
	switch e := e.(type) {
	case PermissionQuick:
		return e.Kind.Allows()
	case PermissionConfirm:
		return len(s.Asks) > 0 && s.Pick >= 0 && s.Pick < len(s.Asks[0].Options) && !s.Asks[0].Options[s.Pick].Kind.Rejects()
	}
	return false
}

func (r *reducer) inputViewKey(e InputViewKey) {
	s := &r.s
	v := s.InputView
	if v == nil {
		return
	}
	n := *v
	switch e.Action {
	case TranscriptUp:
		n.Scroll--
	case TranscriptDown:
		n.Scroll++
	case TranscriptPageUp:
		n.Scroll -= max(e.Page, 1)
	case TranscriptPageDown:
		n.Scroll += max(e.Page, 1)
	case TranscriptTop:
		n.Scroll = 0
	case TranscriptBottom:
		n.Scroll = e.Max
	case TranscriptClose:
		s.InputView = nil
		return
	}
	n.Scroll = min(max(n.Scroll, 0), max(e.Max, 0))
	s.InputView = &n
}
