package core

import (
	"fmt"
	"slices"
	"time"
)

// Consent has a surface of its own: the review, a full-screen pager over
// the text being consented to, T. Every subject goes through it: a
// permission request's allow, a card's trust, a doc's trust, a card's
// archive. The dashboard only summarises a waiting subject, lossily, and
// nothing there arms or answers anything; enter (or t, or X) opens the
// review.
//
// T is relay's when relay sent one (Review), and built here otherwise
// (RequestText, for a plain ACP agent's request). The pager shows T
// verbatim, wrapped by display width and never cut, so its lines,
// concatenated, are T. It fails closed: a T with a character that does
// not show as itself, or whose digest is not the one relay sent, is not
// shown and arms nothing (Subject.Problem).
//
// An allow, a trust or an archive is taken only once every part of this
// T has been drawn (the pager says what it drew, Drawn.Review, as byte
// ranges of T) by a live key at least ArmDelay after the last part not
// yet drawn came on screen. A subject's identity includes T's digest, so
// a T that changes is read again from the start. Refusing, cancelling
// and going back work at once, with a live key.

// SubjectKind is what a review is about.
type SubjectKind int

const (
	SubjectNone SubjectKind = iota
	// SubjectRequest is the permission request on offer, opened with
	// enter from the dashboard.
	SubjectRequest
	// SubjectCardTrust, SubjectArchive and SubjectDocTrust are the
	// pane's questions: t on a card, X on a card, t on a doc.
	SubjectCardTrust
	SubjectArchive
	SubjectDocTrust
)

// Subject is what a review shows: its identity, T, the digest to give
// back with a yes (relay's H; "" for a T built here), why it cannot be
// consented to here ("" when it can), and relay's word on an earlier
// answer to the same request.
type Subject struct {
	Kind    SubjectKind
	ID      string
	Text    string
	Digest  string
	Problem string
	Note    string
}

// Reviewing is a request whose review is open, by its id (Ask). After an
// allow that carried relay's digest, Ask is 0 and Allowed says the allow
// went out for CallID: if relay refuses it, it asks again with the same
// tool call (_meta["relay/refused"]), and the review opens again on it.
type Reviewing struct {
	Ask     uint64
	CallID  string
	Allowed bool
}

// reviewing says the review of the request on offer is open.
func (s State) reviewing() bool {
	return s.Reviewing != nil && len(s.Asks) > 0 && s.Reviewing.Ask == s.Asks[0].ID
}

// ReviewSubject is what the review that has the keys shows. surf is the
// surface (HubSurface, SingleSurface); a and s the app and the
// conversation it is over.
func ReviewSubject(surf Surface, a App, s State) Subject {
	if surf.Kind != SurfaceReview && surf.Under != SurfaceReview {
		return Subject{}
	}
	switch surf.Subject {
	case SubjectRequest:
		if len(s.Asks) > 0 {
			return requestSubject(s.Asks[0], s.Labels)
		}
	case SubjectDocTrust:
		if t := s.Pane.Trust; t != nil {
			it := t.Item
			return reviewed(SubjectDocTrust, fmt.Sprintf("doctrust/%q/%s", it.Path, it.Version), it.Review, "doc")
		}
	case SubjectCardTrust, SubjectArchive:
		id := a.Board.Confirm
		kind := "archive"
		if surf.Subject == SubjectCardTrust && a.Board.Trust != nil {
			id, kind = a.Board.Trust.Card, "cardtrust"
		}
		i := slices.IndexFunc(a.Board.Cards, func(c Card) bool { return c.ID == id })
		if i < 0 {
			return Subject{Kind: surf.Subject, ID: fmt.Sprintf("%s/%d", kind, id), Problem: "the card has left the board"}
		}
		c := a.Board.Cards[i]
		return reviewed(surf.Subject, fmt.Sprintf("%s/%d/%d", kind, c.ID, c.Rev), c.Review, "card")
	}
	return Subject{Kind: surf.Subject, ID: "review/none", Problem: "there is nothing to review"}
}

// requestSubject is the review of request p, in a session with labels.
func requestSubject(p Permission, labels Labels) Subject {
	var sub Subject
	if p.Review != nil {
		sub = reviewed(SubjectRequest, fmt.Sprintf("request/%d", p.ID), p.Review, "request")
	} else {
		t := RequestText(p, labels)
		sub = Subject{Kind: SubjectRequest, ID: fmt.Sprintf("request/%d/%s", p.ID, Digest(t)), Text: t, Problem: reviewTextProblem(t)}
	}
	if p.Refused != "" {
		sub.Note = "relay refused the last answer: " + VisibleLine(p.Refused)
	}
	return sub
}

// reviewed is the subject for relay's review rv of the object named
// object, or one that fails closed when relay sent none, or one that
// does not check out.
func reviewed(kind SubjectKind, object string, rv *Review, what string) Subject {
	if rv == nil {
		return Subject{Kind: kind, ID: object + "/none", Problem: "relay sent no review text for this " + what}
	}
	sub := Subject{Kind: kind, ID: object + "/" + Digest(rv.Text), Text: rv.Text, Digest: rv.SHA256}
	switch {
	case Digest(rv.Text) != rv.SHA256:
		sub.Problem = "relay's digest does not match its text"
	default:
		sub.Problem = reviewTextProblem(rv.Text)
	}
	return sub
}

// Span is bytes From to To of a review's text.
type Span struct{ From, To int }

// Pager is the review on screen: its identity, the first row shown, what
// of its text has been drawn, and when the last part not drawn before
// came on screen, once every part has (ReadAt). Its zero ReadAt says not
// all of it has.
type Pager struct {
	ID     string
	Scroll int
	Seen   []Span
	ReadAt time.Time
}

// ReviewDrawn is what a frame drew of the review named ID: bytes From to
// To of its text.
type ReviewDrawn struct {
	ID       string
	From, To int
}

// see is p after a frame drew d at at, of a text n bytes long.
func (p Pager) see(d ReviewDrawn, n int, at time.Time) Pager {
	if d.ID != p.ID || d.From < 0 || d.To > n || d.From > d.To {
		return p
	}
	p.Seen = mergeSpans(append(slices.Clone(p.Seen), Span{d.From, d.To}))
	if p.ReadAt.IsZero() && covers(p.Seen, n) {
		p.ReadAt = at
	}
	return p
}

// mergeSpans is spans sorted, with those that touch or overlap joined.
func mergeSpans(spans []Span) []Span {
	slices.SortFunc(spans, func(a, b Span) int { return a.From - b.From })
	var out []Span
	for _, s := range spans {
		if n := len(out); n > 0 && s.From <= out[n-1].To {
			out[n-1].To = max(out[n-1].To, s.To)
			continue
		}
		out = append(out, s)
	}
	return out
}

// covers says spans, merged, hold every byte of a text n long.
func covers(spans []Span, n int) bool {
	if n == 0 {
		return true
	}
	return len(spans) == 1 && spans[0].From == 0 && spans[0].To >= n
}

// Read says every part of the pager's text has been drawn.
func (p *Pager) Read() bool { return p != nil && !p.ReadAt.IsZero() }

// ArmedAt is when the review may be consented to: ArmDelay after the
// last part of it came on screen, once all has.
func (p *Pager) ArmedAt() (time.Time, bool) {
	if !p.Read() {
		return time.Time{}, false
	}
	return p.ReadAt.Add(ArmDelay), true
}

// ReviewAction is a key in the review that answers nothing: it scrolls,
// or goes back to the dashboard.
type ReviewAction int

const (
	ReviewUp ReviewAction = iota
	ReviewDown
	ReviewPageUp
	ReviewPageDown
	ReviewTop
	ReviewBottom
	// ReviewBack closes a request's review and answers nothing; the
	// request waits on the dashboard. The pane's questions have no back:
	// esc or q cancels them.
	ReviewBack
)

// Events of the review.
type (
	// OpenReview is enter on a waiting request on the dashboard.
	OpenReview struct{ At time.Time }
	// ReviewKey scrolls the review or closes it. Max is how far it
	// scrolls as drawn, and Page how many rows a page is.
	ReviewKey struct {
		At        time.Time
		Action    ReviewAction
		Max, Page int
	}
)

func (e OpenReview) at() time.Time { return e.At }
func (e ReviewKey) at() time.Time  { return e.At }

// scrolled is p after e.
func scrolled(p *Pager, e ReviewKey) *Pager {
	if p == nil {
		return nil
	}
	n := *p
	switch e.Action {
	case ReviewUp:
		n.Scroll--
	case ReviewDown:
		n.Scroll++
	case ReviewPageUp:
		n.Scroll -= max(e.Page, 1)
	case ReviewPageDown:
		n.Scroll += max(e.Page, 1)
	case ReviewTop:
		n.Scroll = 0
	case ReviewBottom:
		n.Scroll = e.Max
	}
	n.Scroll = min(max(n.Scroll, 0), max(e.Max, 0))
	return &n
}

// syncPager keeps the app's pager to the surface: a fresh one for a
// review that has just come up, or whose text changed, and none
// elsewhere. It returns a only changed where it must.
func syncPager(a App, surf Surface, id string) App {
	switch {
	case surf.Kind != SurfaceReview:
		a.Pager = nil
	case a.Pager == nil || a.Pager.ID != id:
		a.Pager = &Pager{ID: id}
	}
	return a
}

// seeReview adds what a frame drew of the review to the app's pager.
func (a App) seeReview(e Drawn, sub Subject) App {
	if e.OnlyForget || e.Review == nil || a.Pager == nil || e.Review.ID != sub.ID || sub.Problem != "" {
		return a
	}
	p := a.Pager.see(*e.Review, len(sub.Text), e.At)
	a.Pager = &p
	return a
}

// Notes for a consent the review did not take.
const (
	NotReadNote = "read it to the end first: space or pgdown for the next page"
)

// reviewArmed says the subject may be consented to at at: a live key,
// the review drawn and every part of it read, ArmDelay ago.
func (a App) reviewArmed(sub Subject, at time.Time) bool {
	t, ok := a.Pager.ArmedAt()
	return !a.KeyReplayed && sub.Problem == "" && sub.ID != "" && a.Shown.Surface == sub.ID &&
		a.Pager != nil && a.Pager.ID == sub.ID && ok && !at.Before(t)
}

// reviewRefusal is what to say about a consent reviewArmed refused.
func (a App) reviewRefusal(sub Subject) string {
	switch {
	case a.KeyReplayed:
		return ReplayedNote
	case sub.Problem != "":
		return "nothing can be allowed here: " + sub.Problem
	case a.Shown.Surface != sub.ID:
		return NotShownNote
	case !a.Pager.Read():
		return NotReadNote
	}
	return TooSoonNote
}

// ReviewArmed says the review that has the keys may be consented to at
// now: the renderer shows its yes as armed from then.
func ReviewArmed(surf Surface, a App, s State, now time.Time) bool {
	sub := ReviewSubject(surf, a, s)
	live := a
	live.KeyReplayed = false
	return live.reviewArmed(sub, now)
}

func (r *reducer) openReview(at time.Time) {
	s := &r.s
	if len(s.Asks) == 0 {
		return
	}
	p := s.Asks[0]
	s.Reviewing = &Reviewing{Ask: p.ID, CallID: p.CallID}
	s.Pick = DefaultPick(p.Options)
}
