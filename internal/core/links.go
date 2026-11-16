package core

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
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
	// Own and Thing are set only on the copy that draws one card or one
	// ready item (Of): the links relay listed for that thing, and that
	// the text being drawn is such a thing's. There the list alone says
	// what is a link. They are never kept in the connection's state.
	Own   []Link
	Thing bool
	// InertText says relay makes a contaminated session's conversation
	// text inert before it sends it (initialize's relay/inertText), and
	// lists a reply's live links beside each chunk (Entry.Links). Such
	// text is drawn as relay sent it, and the client's own breaker is
	// not run on it. It is set once, when initialize answers.
	InertText bool
}

// Link is a live link relay attached to a card or a ready item: the
// address, and that address as the thing's text shows it, broken. The
// text itself holds no address to read. A terminal is given Address, and
// only where Shown stands in the text as relay wrote it (Anchored).
type Link struct{ Address, Shown string }

// MaxOwnLinks is the most links relay lists for one thing.
const MaxOwnLinks = 50

// Of is l for drawing one card or one ready item: the connection's set,
// with the links relay listed for that thing.
func (l Links) Of(own []Link) Links {
	l.Own, l.Thing = own, true
	return l
}

// Has says addr is a live link: a member of the set, character for
// character, in the one shape a live link has.
func (l Links) Has(addr string) bool {
	if _, ok := l.Live[addr]; !ok {
		return false
	}
	return LinkShape(addr)
}

// unionLinks is a with each link of b that a does not hold put after
// it, in b's order. It never changes a in place: a may be shared with an
// earlier state.
func unionLinks(a, b []Link) []Link {
	out := a
	for _, l := range b {
		if !slices.Contains(out, l) {
			out = append(slices.Clip(out), l)
		}
	}
	return out
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
// it, so relay sends each change after. It asks once. A call that failed
// is asked again the next time something shows relay is answering (the
// board or the ready view loads, the ready view opens, a reload key),
// never while a call is out, and never of an agent without the method.
func (r *reducer) askLinks() {
	l := &r.a.Links
	if !l.Asked && !l.Unavailable && r.a.Live {
		l.Asked = true
		r.emit(RequestLinks{})
	}
}

func (r *reducer) links(e Event) {
	l := &r.a.Links
	switch e := e.(type) {
	case LinksLoaded:
		switch {
		case e.Unavailable:
			l.Unavailable, l.Loaded, l.Live = true, false, nil
		case e.Err != "":
			// Every link stays broken, which is safe. The call is made
			// again once relay is seen to answer (askLinks).
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

// Anchored finds the link a word of a thing's text shows (a word has no
// space in it): what stands before it, the link, and what stands after.
// The word's run from its first "hxxp" to its end, less at most
// maxClosers closers, has to be exactly a link's Shown. Text that runs on
// after a broken address is part of another address, so it is never a
// listed link with more after it. Nothing is read out of the word: the
// address is the one relay listed. This is relay.text/anchored.
func Anchored(word string, links []Link) (before string, link Link, after string, ok bool) {
	if len(links) == 0 {
		return "", Link{}, "", false
	}
	start := strings.Index(word, "hxxp")
	if start < 0 {
		return "", Link{}, "", false
	}
	run := word[start:]
	for n := 0; n <= maxClosers && n < len(run); n++ {
		cut := len(run) - n
		if n > 0 && !strings.ContainsRune(linkClosers, rune(run[cut])) {
			break // what follows is no longer only closers
		}
		for _, l := range links {
			if l.Shown == run[:cut] {
				return word[:start], l, run[cut:], true
			}
		}
	}
	return "", Link{}, "", false
}

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

// --- the links in one thing, listed

// ShownLink is one link in a thing's outside text, as the list of its
// links shows it. Text is its broken form, as the text has it. A live one
// is a link relay listed for the thing, and Address is the address relay
// gave for it. A broken one has no address: its text is read only to be
// shown, and to be sent back to relay as it stands when the operator
// keeps it. Nothing is ever followed but Address.
type ShownLink struct {
	Text    string
	Address string
	Live    bool
}

// LinksIn lists the links of one thing, each once, live and broken in
// the order its text first shows them. own is the links relay listed for
// the thing and texts its outside text.
//
// A live link is one of own whose address is in the connection's set
// too. It is listed where its broken form stands in a word as relay
// wrote it (Anchored), or after the rest when the text read here does
// not show it. A broken link is any other broken http address in a word:
// the word's run from its first "hxxp", less the marks that close a link
// in prose. That is the form relay takes back when the link is kept, so
// the client works out no address. Nothing else is a link here: relay
// breaks every other kind for good.
func LinksIn(own []Link, live Links, texts ...string) []ShownLink {
	var out []ShownLink
	seen := map[ShownLink]bool{}
	add := func(l ShownLink) {
		if !seen[l] {
			seen[l] = true
			out = append(out, l)
		}
	}
	for _, t := range texts {
		// Most words hold no broken address: the pane's hint asks this of
		// every card and item on every frame, so they are passed quickly.
		if !strings.Contains(t, "hxxp") {
			continue
		}
		for _, w := range strings.FieldsFunc(t, func(r rune) bool { return strings.ContainsRune(" \t\n\f\r", r) }) {
			if _, l, _, ok := Anchored(w, own); ok && live.Has(l.Address) {
				add(ShownLink{Text: l.Shown, Address: l.Address, Live: true})
				continue
			}
			if text, ok := brokenIn(w); ok {
				add(ShownLink{Text: text})
			}
		}
	}
	for _, l := range own {
		if live.Has(l.Address) {
			add(ShownLink{Text: l.Shown, Address: l.Address, Live: true})
		}
	}
	return out
}

// brokenIn is the broken http address a word holds, as it stands: the
// word's run from its first "hxxp", which has to be a broken scheme with
// something after it, less the marks that close a link in prose.
func brokenIn(word string) (string, bool) {
	start := strings.Index(word, "hxxp")
	if start < 0 {
		return "", false
	}
	run := word[start:]
	scheme := ""
	for _, s := range []string{"hxxps://", "hxxp://"} {
		if strings.HasPrefix(run, s) {
			scheme = s
			break
		}
	}
	text := unclosed(run)
	if scheme == "" || len(text) <= len(scheme) {
		return "", false
	}
	return text, true
}

// unclosed is a link without the marks that close it in prose after it,
// at most maxClosers of them: a full stop, a bracket, Markdown's stars.
// A round bracket that closes one the link opened is part of the link, as
// in /wiki/Fund_(arts).
func unclosed(link string) string {
	for n := 0; n < maxClosers && len(link) > 0 && strings.ContainsRune(linkClosers, rune(link[len(link)-1])); n++ {
		if link[len(link)-1] == ')' && strings.Count(link, "(") >= strings.Count(link, ")") {
			break
		}
		link = link[:len(link)-1]
	}
	return link
}

// ReadyItemLinks is the links of a ready item: the ones relay listed for
// it, and the broken ones in its title, its lines and its headings. The
// item's own flag is not asked. An item relay did not mark as an
// outsider's can still hold a line that drew on outside text, as a daily
// note does, and relay breaks and lists that line's links all the same.
func ReadyItemLinks(it ReadyItem, live Links) []ShownLink {
	texts := append([]string{it.Title}, it.Lines...)
	for _, s := range it.Sections {
		texts = append(texts, s.Heading)
	}
	return LinksIn(it.Links, live, texts...)
}

// CardLinks is the links of a card: the ones relay listed for it, and
// the broken ones in its outside text. That text is each piece relay
// marked as an outsider's, the text form of a card whose kind the client
// does not know when the card holds any text from outside, and every
// string a surface card draws.
func CardLinks(c Card, live Links) []ShownLink {
	var texts []string
	add := func(t CardText) {
		if t.Outsider {
			texts = append(texts, t.Text)
		}
	}
	add(c.Title)
	for _, group := range [][]CardElem{c.Items, c.Rows, c.Columns} {
		for _, e := range group {
			add(e.CardText)
		}
	}
	if c.Note != nil {
		add(*c.Note)
	}
	if !c.Native() && c.Labels.Contaminated {
		texts = append(texts, c.Text)
	}
	// A surface card's text is what its surface draws, resolved as the
	// drawing resolves it. Its flag is not asked, as an item's is not:
	// relay breaks and lists the links in whatever text it sends.
	if c.Kind == CardSurface && c.Surface != nil {
		texts = append(texts, c.Surface.A2UI.Texts()...)
	}
	return LinksIn(c.Links, live, texts...)
}

// LinkAsk is a question about one link: keep it, or drop it. Text is the
// link as the list showed it when the question was asked. A yes to a keep
// sends that text, the broken form relay mends. A yes to a drop sends
// Address, the address relay listed for the live link.
type LinkAsk struct {
	Text    string
	Address string
	Keep    bool
}

// Sends is what a yes to the question sends relay as the link's address.
func (a LinkAsk) Sends() string {
	if a.Keep {
		return a.Text
	}
	return a.Address
}

// LinkHost is a link's scheme and host as its text shows them: all of
// it up to the first "/", "?" or "#" after its "://". It is what the
// operator has to see whole to know where a link goes.
func LinkHost(text string) string {
	i := strings.Index(text, "://")
	if i < 0 {
		return text
	}
	if j := strings.IndexAny(text[i+3:], "/?#"); j >= 0 {
		return text[:i+3+j]
	}
	return text
}

// LinkAskID names the question, finely enough that a yes meant for one
// link can never answer another. where is "pane" or "ready".
func LinkAskID(where string, ask LinkAsk) string {
	return fmt.Sprintf("link/%s/%t/%q/%q", where, ask.Keep, ask.Text, ask.Address)
}

// LinkList is the list of the links in one thing, open over it: a ready
// item (Item) or a card (Card). The links themselves are not held: they
// are read off the thing as it stands (ReadyItemLinks, CardLinks), so
// the list follows relay.
type LinkList struct {
	Item string
	Card int64
	// Sel is the selected link, by its place in the list.
	Sel int
	// Ask is the question on screen. Sending is the keep or drop on its
	// way to relay.
	Ask     *LinkAsk
	Sending *LinkAsk
}

// Notes the list shows.
const (
	NoLinksNote         = "No links from outside here."
	LinkLiveAlreadyNote = "That link is live already. d drops it."
	LinkBrokenNote      = "That link is broken already. Enter keeps it."
	LinkBusyNote        = "A change is on its way: wait for it."
	LinkStaysBrokenNote = "It stays broken."
	LinkStaysLiveNote   = "It stays live."
	LinkChangedNote     = "The link changed, so nothing was done. Look at it again."
	LinkKeptNote        = "Kept. relay lists it as live, and any client can open it."
	LinkWasLiveNote     = "That link is already live."
	LinkNotKeptNote     = "Not kept."
	LinkDroppedNote     = "Dropped. Nothing opens it now, whoever shows it again."
	LinkWasDroppedNote  = "That link is already dropped."
	LinkNotDroppedNote  = "Not dropped."
)

// The questions, as they are asked.
const (
	KeepQuestion = "relay cannot say where this link came from. Keep it?"
	// LinkHiddenNote is said for a yes pressed while the link's host
	// was not all on screen: the question stays open.
	LinkHiddenNote = "Make the screen larger to answer: the question and its link are not all on screen."
	DropQuestion   = "Drop this link? Nothing will open it then, whoever shows it again."
)

// linkAct is what a key does in the list, wherever the list is open.
type linkAct int

const (
	linkNothing linkAct = iota
	linkMove            // by delta
	linkTop
	linkBottom
	linkKeep // enter on a broken link: ask
	linkDrop // d on a live link: ask
	linkYes  // y to the question
	linkNo   // any other key to the question
)

// step applies a key to the list of links: the list after it, what to
// say, and the keep or drop to send, if the key was a yes to a question.
// A yes sends the link the question was asked about, as it was shown
// then, and never one read off the selection.
func (ll LinkList) step(links []ShownLink, act linkAct, delta int) (LinkList, string, *LinkAsk) {
	ll.Sel = min(max(ll.Sel, 0), max(len(links)-1, 0))
	if ask := ll.Ask; ask != nil {
		ll.Ask = nil
		switch {
		case act != linkYes && ask.Keep:
			return ll, LinkStaysBrokenNote, nil
		case act != linkYes:
			return ll, LinkStaysLiveNote, nil
		case ll.Sending != nil:
			return ll, LinkBusyNote, nil
		}
		send := *ask
		ll.Sending = &send
		return ll, "", &send
	}
	var sel *ShownLink
	if ll.Sel < len(links) {
		sel = &links[ll.Sel]
	}
	switch act {
	case linkMove:
		ll.Sel = min(max(ll.Sel+delta, 0), max(len(links)-1, 0))
	case linkTop:
		ll.Sel = 0
	case linkBottom:
		ll.Sel = max(len(links)-1, 0)
	case linkKeep, linkDrop:
		keep := act == linkKeep
		switch {
		case sel == nil:
			return ll, NoLinksNote, nil
		case keep && sel.Live:
			return ll, LinkLiveAlreadyNote, nil
		case !keep && !sel.Live:
			return ll, LinkBrokenNote, nil
		case ll.Sending != nil:
			return ll, LinkBusyNote, nil
		}
		ll.Ask = &LinkAsk{Text: sel.Text, Address: sel.Address, Keep: keep}
	}
	return ll, "", nil
}

// kept is the list once what it lists may have changed: the selection in
// range, and the question dropped when its link is no longer listed as
// it was asked about (still broken for a keep, still live for a drop).
// The operator answers about the link they were shown, and only that.
func (ll LinkList) kept(links []ShownLink) (LinkList, bool) {
	ll.Sel = min(max(ll.Sel, 0), max(len(links)-1, 0))
	if ask := ll.Ask; ask != nil {
		i := slices.Index(links, ShownLink{Text: ask.Text, Address: ask.Address, Live: !ask.Keep})
		if i < 0 {
			ll.Ask = nil
			return ll, true
		}
		// The bar stays on the link the question is about, wherever the
		// new list puts it, so what is selected is what a yes sends.
		ll.Sel = i
	}
	return ll, false
}

// done is the list once relay has answered a keep or drop it sent, and
// what to say: the outcome, then relay's notes, and relay's reason when
// it refused. ok is false when the answer is not to this list's call.
func (ll LinkList) done(e LinkDone) (next LinkList, message, failure string, ok bool) {
	if !ll.Sending.sent(e) {
		return ll, "", "", false
	}
	ll.Sending = nil
	message, failure = linkOutcome(e)
	return ll, message, failure, true
}

// sent says e is relay's answer to the keep or drop a yes to this
// question sent. A nil question sent nothing.
func (a *LinkAsk) sent(e LinkDone) bool {
	return a != nil && a.Sends() == e.Address && a.Keep == e.Keep
}

// linkOutcome is relay's answer to a keep or drop, in words: the
// outcome, then relay's notes, and relay's reason when it refused.
func linkOutcome(e LinkDone) (message, failure string) {
	switch {
	case e.Err != "" && e.Keep:
		return LinkNotKeptNote, e.Err
	case e.Err != "":
		return LinkNotDroppedNote, e.Err
	case e.Keep && e.Changed:
		message = LinkKeptNote
	case e.Keep:
		message = LinkWasLiveNote
	case e.Changed:
		message = LinkDroppedNote
	default:
		message = LinkWasDroppedNote
	}
	if len(e.Notes) > 0 {
		message += " " + strings.Join(e.Notes, " ")
	}
	return message, ""
}

// closed is what a list leaves behind when it closes: the keep or drop
// still on its way to relay, or nil. The view keeps it (LinkOut), so
// relay's answer is still said when it comes, and a list opened in the
// meantime takes it back.
func (ll *LinkList) closed() *LinkAsk {
	if ll == nil {
		return nil
	}
	return ll.Sending
}

// Keeping and dropping a link, as relay link does: the effect that asks
// relay, and relay's answer.
type (
	// SendLink asks relay to keep the link live (_relay/links/keep) or to
	// drop it (_relay/links/drop). Address is what relay is sent. For a
	// keep it is the link as the list showed it, in the broken form,
	// which relay mends. For a drop it is the address relay listed for
	// the live link.
	SendLink struct {
		Address string
		Keep    bool
	}
	// LinkDone is relay's answer. Changed is false when the link was
	// already live, or already dropped. Notes are what relay cleaned out
	// of the address, and Err is its refusal.
	LinkDone struct {
		At      time.Time
		Address string
		Keep    bool
		Changed bool
		Notes   []string
		Err     string
	}
)

func (SendLink) effect()         {}
func (e LinkDone) at() time.Time { return e.At }
