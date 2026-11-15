package core

import (
	"maps"
	"regexp"
	"strings"
	"time"
)

// A link in text an outsider may have shaped is broken wherever the
// dashboard shows it, with one exception: relay's live links, the
// addresses relay can say came from somewhere it takes links from (the
// web showed one to a scout, or the operator called it safe). relay sends
// this connection that set, and the dashboard lets a terminal follow an
// address only when it is exactly a member of it. The client judges no
// address for itself, and asks nothing about the host a link points at.

// Links is relay's live links as this connection was last sent them
// (_relay/links, then _relay/links/changed). Live is replaced, never
// changed in place, so a copy of Links stays as it was.
type Links struct {
	// Asked says _relay/links has been called; Loaded, that it answered.
	// Unavailable says the agent has no such method (a plain ACP agent,
	// or a relay from before it): every link then stays broken.
	Asked, Loaded, Unavailable bool
	// Rev is the newest rev seen, and LoadedRev the rev of the last
	// answer: a change up to it is already in the answer.
	Rev, LoadedRev int64
	Live           map[string]struct{}
}

// Has says addr is a live link: a member of the set, character for
// character, in the one shape a live link has.
func (l Links) Has(addr string) bool {
	if _, ok := l.Live[addr]; !ok {
		return false
	}
	return LinkShape(addr)
}

// Empty says there is no live link.
func (l Links) Empty() bool { return len(l.Live) == 0 }

// LinksOf is the set that holds addrs. One that is not in a live link's
// shape is left out: nothing else is ever written into a hyperlink.
func LinksOf(addrs ...string) Links {
	live := make(map[string]struct{}, len(addrs))
	for _, a := range addrs {
		if LinkShape(a) {
			live[a] = struct{}{}
		}
	}
	return Links{Live: live}
}

// Events about the live links, and the effect that asks relay for them.
type (
	// LinksLoaded is relay's answer to _relay/links: the whole set.
	// Unavailable says the agent has no such method; Err, that the call
	// failed another way.
	LinksLoaded struct {
		At          time.Time
		Rev         int64
		Live        []string
		Unavailable bool
		Err         string
	}
	// LinksChanged is a _relay/links/changed notification: the links
	// that became live, and the ones that no longer are.
	LinksChanged struct {
		At   time.Time
		Rev  int64
		Live []string
		Gone []string
	}
	// RequestLinks asks relay for the live links (_relay/links).
	RequestLinks struct{}
)

func (e LinksLoaded) at() time.Time  { return e.At }
func (e LinksChanged) at() time.Time { return e.At }
func (RequestLinks) effect()         {}

// askLinks asks relay for the live links once the connection can take
// it, so relay sends each change after. It asks again only after a
// failure.
func (r *reducer) askLinks() {
	l := &r.a.Links
	if !l.Asked && !l.Unavailable && r.a.Live {
		l.Asked = true
		r.emit(RequestLinks{})
	}
}

// retryLinks asks again when the last call failed: a reload of the board
// or of the ready view is the operator's way to say try again.
func (r *reducer) retryLinks() {
	if l := &r.a.Links; !l.Loaded {
		l.Asked = false
	}
	r.askLinks()
}

func (r *reducer) links(e Event) {
	l := &r.a.Links
	switch e := e.(type) {
	case LinksLoaded:
		switch {
		case e.Unavailable:
			l.Unavailable, l.Loaded, l.Live = true, false, nil
		case e.Err != "":
			// Every link stays broken, which is safe. A reload asks again.
			l.Asked = false
		case e.Rev < l.Rev:
			// A change newer than this answer got here first, and relay
			// counts it as sent. Taking the answer would undo it, so ask
			// again for one that is current.
			l.Asked = false
			r.askLinks()
		default:
			l.Loaded, l.Unavailable = true, false
			l.Live = LinksOf(e.Live...).Live
			l.Rev, l.LoadedRev = max(l.Rev, e.Rev), e.Rev
		}
	case LinksChanged:
		if l.Loaded && e.Rev <= l.LoadedRev {
			return // the answer already holds it
		}
		live := maps.Clone(l.Live)
		if live == nil {
			live = map[string]struct{}{}
		}
		for _, a := range e.Live {
			if LinkShape(a) {
				live[a] = struct{}{}
			}
		}
		for _, a := range e.Gone {
			delete(live, a)
		}
		l.Live = live
		l.Rev = max(l.Rev, e.Rev)
	}
}

// MaxLink is the most characters in a live link, as relay.text/max-link
// has it.
const MaxLink = 2000

// linkShape is the widest shape a live link has had (relay.text/link-shape
// is stricter now): http or https in lower case, a host of ASCII letters,
// digits, dots and hyphens, a port, then only what RFC 3986 allows in a
// path, a query or a fragment, less the round brackets. relay decides
// what is live; this only holds what the client will write into a
// hyperlink to printable ASCII, so an address can sit inside an escape
// sequence without ending it.
var linkShape = regexp.MustCompile(`^https?://[A-Za-z0-9](?:[A-Za-z0-9.\-]*[A-Za-z0-9])?(?::[0-9]{1,5})?(?:[/?#][A-Za-z0-9\-._~:/?#\[\]@!$&'*+,;=%]*)?$`)

// LinkShape says s can be a live link.
func LinkShape(s string) bool {
	return len(s) <= MaxLink && linkShape.MatchString(s)
}

// linkClosers is what may stand straight after a link in a word without
// being part of it (relay.text/link-closers): the marks that end a clause
// or close a bracket or a quote, and those of a question, a list and
// Markdown's links and emphasis.
const linkClosers = `)>".,;!]':?*_~`

// maxClosers is how many closers may follow a link, as in
// **[a fund](https://x.example/a)**.
const maxClosers = 6

var schemeRE = regexp.MustCompile(`(?i)[a-z][a-z0-9+.\-]*://`)

// LinkIn finds the live link a word holds (a word has no space in it):
// what stands before it, the link, and what stands after. The link
// starts at the word's first scheme and runs to the word's end, less at
// most maxClosers closers; the longest that is live wins. Text that runs
// on after an address is part of it, so it makes another address, never
// a live one with more after it. This is relay.text/link-in.
func LinkIn(word string, live Links) (before, link, after string, ok bool) {
	if live.Empty() {
		return "", "", "", false
	}
	loc := schemeRE.FindStringIndex(word)
	if loc == nil {
		return "", "", "", false
	}
	run := word[loc[0]:]
	for n := 0; n <= maxClosers && n < len(run); n++ {
		cut := len(run) - n
		if n > 0 && !strings.ContainsRune(linkClosers, rune(run[cut])) {
			break // what follows is no longer only closers
		}
		if live.Has(run[:cut]) {
			return word[:loc[0]], run[:cut], run[cut:], true
		}
	}
	return "", "", "", false
}
