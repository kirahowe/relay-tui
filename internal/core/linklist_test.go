package core

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"
)

const (
	liveA   = "https://artsfund.example/grants/2027"
	shownA  = "hxxps://artsfund[.]example/grants/2027"
	brokenC = "hxxps://bit[.]example/3xQ9"
	keptC   = "https://bit.example/3xQ9"
)

// ownA and ownC are the two links as relay lists them for a thing, each
// once it is live: the address, and the broken form the text shows.
var (
	ownA = Link{Address: liveA, Shown: shownA}
	ownC = Link{Address: keptC, Shown: brokenC}
)

// liveRow is a listed link's row in the list when it is live.
func liveRow(l Link) ShownLink { return ShownLink{Text: l.Shown, Address: l.Address, Live: true} }

func TestLinksIn(t *testing.T) {
	apart := Link{Address: "https://apart.example/in-no-text", Shown: "hxxps://apart[.]example/in-no-text"}
	live := LinksOf(liveA, apart.Address)
	own := []Link{ownA, apart, {Address: "https://not.live/x", Shown: "hxxps://not[.]live/x"}}
	got := LinksIn(own, live,
		"Arts fund: "+shownA+" and again "+shownA+".",
		"A forum post: "+brokenC+").",
		"**[x](hxxp://old[.]example/a)** see:hxxps://b[.]example/c?d=1",
		"not links: ftp[:]//h[.]example/f evil[.]icu www[.]x[.]com hxxps:// HXXPS://UP[.]example/a hxxpfoo fxp://f[.]example/a plain words",
		brokenC,
		"listed, and not in the set: hxxps://not[.]live/x",
		"it runs on: "+shownA+"/more and (hxxps://evil[.]example/?u="+shownA+")",
		"a bracket of its own: hxxps://wiki[.]example/wiki/Fund_(arts). and (hxxps://w[.]example/a_(b))")
	want := []ShownLink{liveRow(ownA), {Text: brokenC}, {Text: "hxxp://old[.]example/a"}, {Text: "hxxps://b[.]example/c?d=1"},
		{Text: "hxxps://not[.]live/x"}, {Text: shownA + "/more"}, {Text: "hxxps://evil[.]example/?u=" + shownA},
		{Text: "hxxps://wiki[.]example/wiki/Fund_(arts)"}, {Text: "hxxps://w[.]example/a_(b)"},
		// Listed and live, and in no text read here: it comes last.
		liveRow(apart)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	// A listed link is live only with the connection's set behind it, and
	// a link is live only when relay listed it. A whole address in the
	// text is no link at all: nothing is read out of it.
	if got := LinksIn(own, Links{}, "see "+shownA); !reflect.DeepEqual(got, []ShownLink{{Text: shownA}}) {
		t.Fatalf("with no set: %+v", got)
	}
	if got := LinksIn(nil, live, "see "+shownA+" and <"+liveA+"> and "+liveA); !reflect.DeepEqual(got, []ShownLink{{Text: shownA}}) {
		t.Fatalf("with nothing listed: %+v", got)
	}
	// At most six closing marks are taken off a broken link, as relay
	// takes off no more when it looks for one.
	if got := LinksIn(nil, Links{}, brokenC+".......", brokenC+"......"); !reflect.DeepEqual(got, []ShownLink{{Text: brokenC + "."}, {Text: brokenC}}) {
		t.Fatalf("closing marks: %+v", got)
	}
}

// sixClosersAfter says some occurrence of s in text has six closing marks
// straight after it.
func sixClosersAfter(text, s string) bool {
	for i := strings.Index(text, s); i >= 0; {
		after := text[i+len(s):]
		if len(after) >= 6 && strings.Trim(after[:6], linkClosers) == "" {
			return true
		}
		j := strings.Index(text[i+1:], s)
		if j < 0 {
			break
		}
		i += 1 + j
	}
	return false
}

// Whatever the text, the list and the set: a link listed as live is one
// relay listed for the thing, with relay's address, and that address is
// in the set. One listed as broken has no address, starts with a broken
// http scheme and has more after it. No link is listed twice.
func TestLinksInListsOnlyWhatRelaySent(t *testing.T) {
	pool := []Link{ownA, ownC, {Address: "http://y.example", Shown: "hxxp://y[.]example"}, {Address: "https://z.example/p?q=1", Shown: "hxxps://z[.]example/p?q=1"}}
	shown := []string{}
	for _, l := range pool {
		shown = append(shown, l.Shown, l.Address)
	}
	part := rapid.SampledFrom(append([]string{" ", " ", "<", ">", "(", ").", "see:", "HXXPS://A[.]B/c", "hxxp://", "hxxps://x[.]y/a**",
		"ftp[:]//h[.]example", "www[.]x[.]com", "words", "\n", "https://evil.test/x", "/more", "hxxps://evil[.]test/?u="}, shown...))
	rapid.Check(t, func(t *rapid.T) {
		var set []string
		var own []Link
		for _, l := range pool {
			if rapid.Bool().Draw(t, "in the set") {
				set = append(set, l.Address)
			}
			if rapid.Bool().Draw(t, "listed") {
				own = append(own, l)
			}
		}
		if rapid.Bool().Draw(t, "stray") {
			// Listed with an address the set does not hold, under a form
			// another link shows.
			own = append(own, Link{Address: "https://evil.test/listed", Shown: shownA})
		}
		live := LinksOf(set...)
		text := strings.Join(rapid.SliceOfN(part, 0, 12).Draw(t, "parts"), "")
		links := LinksIn(own, live, text)
		seen := map[ShownLink]bool{}
		for _, l := range links {
			if seen[l] {
				t.Fatalf("%+v listed twice in %+v", l, links)
			}
			seen[l] = true
			if l.Live {
				if !slices.Contains(set, l.Address) || !slices.Contains(own, Link{Address: l.Address, Shown: l.Text}) {
					t.Fatalf("%+v listed live; the set is %v and relay listed %v", l, set, own)
				}
				continue
			}
			rest, ok := strings.CutPrefix(l.Text, "hxxps://")
			if !ok {
				rest, ok = strings.CutPrefix(l.Text, "hxxp://")
			}
			// It may end in a closing mark only where more than six followed
			// it, since no more than six are taken off.
			last := l.Text[len(l.Text)-1:]
			if l.Address != "" || !ok || rest == "" || (strings.ContainsAny(last, linkClosers) && last != ")" && !sixClosersAfter(text, l.Text)) {
				t.Fatalf("%+v listed broken from %q", l, text)
			}
		}
		// Every listed link that is in the set is there, live.
		for _, o := range own {
			if slices.Contains(set, o.Address) && !seen[liveRow(o)] {
				t.Fatalf("%+v is listed and in the set, and not in %+v", o, links)
			}
		}
	})
}

// linkCard is a checklist with two links in its outside text, the first
// one listed by relay and so live once liveA is in the set, and two links
// in the operator's own text, which are not listed.
func linkCard(id, rev int64) Card {
	c := checklist(id, rev)
	c.Items = []CardElem{
		{ID: 1, CardText: CardText{Text: "Arts fund: " + shownA, Outsider: true, Rev: rev}},
		{ID: 2, CardText: CardText{Text: "A forum post: " + brokenC + ".", Outsider: true, Rev: rev}},
		{ID: 3, CardText: CardText{Text: "mine https://own.example/x and hxxps://own[.]example/y", Rev: rev}},
	}
	c.Links = []Link{ownA}
	return c
}

// keptCard is linkCard as relay sends it again once its second link is
// live: the same text, with the link listed.
func keptCard(id, rev int64) Card {
	c := linkCard(id, rev)
	c.Links = []Link{ownA, ownC}
	return c
}

func onLinkBoard() Single {
	s := onBoard(linkCard(10, 10), checklist(20, 20, false))
	s, _ = feedS(s, LinksLoaded{At: t0, Rev: 1, Live: []string{liveA}})
	return s
}

func TestCardLinksAreItsOutsideTextsOnly(t *testing.T) {
	c := linkCard(10, 10)
	c.Title = CardText{Text: "see hxxp://t[.]example/title", Outsider: true}
	got := CardLinks(c, LinksOf(liveA))
	want := []ShownLink{{Text: "hxxp://t[.]example/title"}, liveRow(ownA), {Text: brokenC}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	other := Card{ID: 3, Kind: "kanban", Text: "Todo: read " + brokenC, Labels: Labels{Contaminated: true}}
	if got := CardLinks(other, Links{}); len(got) != 1 || got[0].Text != brokenC {
		t.Fatalf("a card of another kind: %+v", got)
	}
	other.Labels = Labels{}
	if got := CardLinks(other, Links{}); len(got) != 0 {
		t.Fatalf("a card with nothing from outside lists %+v", got)
	}
}

func TestTheBoardListsACardsLinksAndKeepsOneAfterAYes(t *testing.T) {
	s := onLinkBoard()
	s, _ = boardKeys(s, PaneLinks)
	_, links, ok := BoardLinks(s.Board, s.Links)
	if !ok || !reflect.DeepEqual(links, []ShownLink{liveRow(ownA), {Text: brokenC}}) {
		t.Fatalf("links %+v open %v", links, ok)
	}
	if s.Board.Links.Sel != 1 {
		t.Fatalf("the list opened on link %d, not the first broken one", s.Board.Links.Sel)
	}
	// Enter on the broken link asks, and the question has the keys.
	s, fx := boardKeys(s, PaneEnter)
	if ask := s.Board.Links.Ask; ask == nil || *ask != (LinkAsk{Text: brokenC, Keep: true}) || len(fx) != 0 {
		t.Fatalf("ask %+v effects %v", s.Board.Links.Ask, fx)
	}
	if SingleSurface(s).Kind != SurfaceLinkAsk {
		t.Fatalf("surface %+v", SingleSurface(s))
	}
	// y at once answers nothing, and says why.
	s, fx = boardKeys(s, PaneYes)
	if len(effectsOf[SendLink](fx)) != 0 || s.Board.Links.Ask == nil || s.State.Message.Text != TooSoonNote {
		t.Fatalf("kept before the question was armed: %v, message %q", fx, s.State.Message.Text)
	}
	// Nor does a key held while connecting and replayed.
	late := t0.Add(2 * ArmDelay)
	r, fx := feedS(s, KeyPressed{At: late, Replayed: true}, key(late, PaneYes))
	if len(effectsOf[SendLink](fx)) != 0 || r.Board.Links.Ask == nil || r.State.Message.Text != ReplayedNote {
		t.Fatalf("a replayed key kept the link: %v, message %q", fx, r.State.Message.Text)
	}
	// Any other key says no.
	n, fx := feedS(s, key(late, PaneOther))
	if len(effectsOf[SendLink](fx)) != 0 || n.Board.Links == nil || n.Board.Links.Ask != nil || n.Board.Message != LinkStaysBrokenNote {
		t.Fatalf("no: %v, %+v, %q", fx, n.Board.Links, n.Board.Message)
	}
	// y once the question has been up for ArmDelay keeps it: relay is
	// sent the link exactly as the list showed it.
	s, fx = feedS(s, key(t0.Add(ArmDelay), PaneYes))
	if got := effectsOf[SendLink](fx); !reflect.DeepEqual(got, []SendLink{{Address: brokenC, Keep: true}}) {
		t.Fatalf("sent %+v", got)
	}
	if s.Board.Links.Ask != nil || s.Board.Links.Sending == nil || SingleSurface(s).Kind != SurfacePane {
		t.Fatalf("after the yes: %+v", s.Board.Links)
	}
	// One at a time.
	s, fx = boardKeys(s, PaneEnter)
	if s.Board.Links.Ask != nil || s.Board.Message != LinkBusyNote || len(fx) != 0 {
		t.Fatalf("a second keep while one is out: %+v %q", s.Board.Links, s.Board.Message)
	}
	// relay's answer, and its notes after it.
	s, _ = feedS(s, LinkDone{At: t0, Address: brokenC, Keep: true, Changed: true, Notes: []string{"Removed 1 invisible character from what you pasted."}})
	if s.Board.Links.Sending != nil || s.Board.Message != LinkKeptNote+" Removed 1 invisible character from what you pasted." {
		t.Fatalf("after relay's answer: %+v %q", s.Board.Links, s.Board.Message)
	}
	// relay sends the set and then the card, its text as it was and the
	// link now listed: both links are live.
	s, _ = feedS(s, LinksChanged{At: t0, Rev: 2, Live: []string{keptC}}, CardsChanged{At: t0, Rev: 100, Cards: []Card{keptCard(10, 10)}})
	if _, links, _ := BoardLinks(s.Board, s.Links); !reflect.DeepEqual(links, []ShownLink{liveRow(ownA), liveRow(ownC)}) {
		t.Fatalf("links %+v", links)
	}
}

func TestTheBoardDropsALiveLinkAfterAYesAndShowsARefusal(t *testing.T) {
	s := onLinkBoard()
	s, _ = boardKeys(s, PaneLinks, PaneUp)
	// Enter on a live link, and d on a broken one, say what would work.
	s, _ = boardKeys(s, PaneEnter)
	if s.Board.Links.Ask != nil || s.Board.Message != LinkLiveAlreadyNote {
		t.Fatalf("enter on a live link: %+v %q", s.Board.Links, s.Board.Message)
	}
	b, _ := boardKeys(s, PaneDown, PaneDrop)
	if b.Board.Links.Ask != nil || b.Board.Message != LinkBrokenNote {
		t.Fatalf("d on a broken link: %+v %q", b.Board.Links, b.Board.Message)
	}
	s, _ = boardKeys(s, PaneDrop)
	if ask := s.Board.Links.Ask; ask == nil || *ask != (LinkAsk{Text: shownA, Address: liveA}) {
		t.Fatalf("ask %+v", s.Board.Links.Ask)
	}
	n, _ := feedS(s, key(t0.Add(ArmDelay), PaneEsc))
	if n.Board.Links == nil || n.Board.Links.Ask != nil || n.Board.Message != LinkStaysLiveNote {
		t.Fatalf("esc on the question: %+v %q", n.Board.Links, n.Board.Message)
	}
	s, fx := feedS(s, key(t0.Add(ArmDelay), PaneYes))
	if got := effectsOf[SendLink](fx); !reflect.DeepEqual(got, []SendLink{{Address: liveA, Keep: false}}) {
		t.Fatalf("sent %+v", got)
	}
	s, _ = feedS(s, LinkDone{At: t0, Address: liveA, Keep: false, Err: "relay refused it."})
	if s.Board.Message != LinkNotDroppedNote || s.Board.Failure != "relay refused it." || s.Board.Links.Sending != nil {
		t.Fatalf("after a refusal: %q %q", s.Board.Message, s.Board.Failure)
	}
	// The other outcomes, each in words.
	for _, c := range []struct {
		done LinkDone
		want string
	}{
		{LinkDone{Keep: true, Changed: true}, LinkKeptNote},
		{LinkDone{Keep: true}, LinkWasLiveNote},
		{LinkDone{Changed: true}, LinkDroppedNote},
		{LinkDone{}, LinkWasDroppedNote},
	} {
		// A keep sends the form the list showed, a drop the address
		// relay listed for it.
		ll := LinkList{Sending: &LinkAsk{Text: "x", Keep: true}}
		if !c.done.Keep {
			ll = LinkList{Sending: &LinkAsk{Text: "shown", Address: "x"}}
		}
		c.done.Address = "x"
		if _, msg, failure, ok := ll.done(c.done); !ok || msg != c.want || failure != "" {
			t.Errorf("%+v: %q %q %v", c.done, msg, failure, ok)
		}
	}
	// An answer to another call changes nothing here: about another
	// link, or a drop's answer to a keep of the same text.
	ll := LinkList{Sending: &LinkAsk{Text: "x", Keep: true}}
	if _, _, _, ok := ll.done(LinkDone{Address: "y", Keep: true, Changed: true}); ok {
		t.Error("an answer about another link was taken")
	}
	if _, _, _, ok := ll.done(LinkDone{Address: "x", Changed: true}); ok {
		t.Error("a drop's answer was taken for a keep")
	}
	// A drop's answer names the address it sent, never the form shown.
	ll = LinkList{Sending: &LinkAsk{Text: "shown", Address: "x"}}
	if _, _, _, ok := ll.done(LinkDone{Address: "shown", Changed: true}); ok {
		t.Error("an answer about the shown form was taken for a drop")
	}
	// Esc closes the list, and L on a card with no link from outside
	// says so.
	s, _ = boardKeys(s, PaneEsc)
	if s.Board.Links != nil {
		t.Fatal("esc left the list open")
	}
	s, _ = boardKeys(s, PaneDown, PaneLinks)
	if s.Board.Links != nil || s.Board.Message != NoLinksNote {
		t.Fatalf("a card with no links: %+v %q", s.Board.Links, s.Board.Message)
	}
}

func TestTheBoardsQuestionGoesWhenItsLinkChanges(t *testing.T) {
	asked := func() Single {
		s := onLinkBoard()
		s, _ = boardKeys(s, PaneLinks, PaneEnter)
		if s.Board.Links == nil || s.Board.Links.Ask == nil {
			t.Fatal("no question")
		}
		return s
	}
	// The link was kept elsewhere while the question was up.
	s, _ := feedS(asked(), LinksChanged{At: t0, Rev: 2, Live: []string{keptC}}, CardsChanged{At: t0, Rev: 100, Cards: []Card{keptCard(10, 10)}})
	if s.Board.Links == nil || s.Board.Links.Ask != nil || s.Board.Message != LinkChangedNote {
		t.Fatalf("the question stayed: %+v %q", s.Board.Links, s.Board.Message)
	}
	_, fx := feedS(s, key(t0.Add(time.Hour), PaneYes))
	if len(effectsOf[SendLink](fx)) != 0 {
		t.Fatalf("a y after the question went kept something: %v", fx)
	}
	// The card's text changed and the link is gone from it.
	edited := linkCard(10, 11)
	edited.Items = edited.Items[:1]
	s, _ = feedS(asked(), CardsChanged{At: t0, Rev: 101, Cards: []Card{edited}})
	if s.Board.Links == nil || s.Board.Links.Ask != nil || s.Board.Links.Sel != 0 {
		t.Fatalf("after the link left the card: %+v", s.Board.Links)
	}
	// The card left the board: the list goes with it.
	gone := linkCard(10, 12)
	gone.Status = CardArchived
	s, _ = feedS(asked(), CardsChanged{At: t0, Rev: 102, Cards: []Card{gone}})
	if s.Board.Links != nil || SingleSurface(s).Kind != SurfacePane {
		t.Fatalf("the list outlived its card: %+v", s.Board.Links)
	}
	// Switching to the workspace items closes it.
	s, _ = boardKeys(onLinkBoard(), PaneLinks, PaneBoard)
	if s.Board.Links != nil || !s.Board.Items {
		t.Fatalf("b left the list open: %+v", s.Board.Links)
	}
}

// linkItem is a message ready for the operator that drew on the web and
// quotes two links, the first one listed by relay and so live once liveA
// is in the set.
func linkItem() ReadyItem {
	it := msg(4, 1)
	it.Outsider, it.From = true, "from the web"
	it.Title = "Two grants"
	it.Lines = []string{"Arts fund: " + shownA, "A forum post: " + brokenC}
	it.Links = []Link{ownA}
	return it
}

// keptItem is linkItem as relay sends it again once its second link is
// live: the same text, with the link listed.
func keptItem() ReadyItem {
	it := linkItem()
	it.Links = []Link{ownA, ownC}
	return it
}

// An item relay did not mark as an outsider's can hold a line that drew
// on outside text, as a daily note does. relay breaks that line's links
// and lists the live ones all the same, so the list has them: the flag is
// about the item, and the links are about the line.
func TestAnItemOfRelaysOwnListsWhatRelayListed(t *testing.T) {
	note := linkItem()
	note.Outsider, note.From = false, ""
	if got := ReadyItemLinks(note, LinksOf(liveA)); !reflect.DeepEqual(got, []ShownLink{liveRow(ownA), {Text: brokenC}}) {
		t.Fatalf("a note of relay's own lists %+v", got)
	}
	// Whatever the flag, a live row is one relay listed, and nothing in
	// the text makes one: with no list every link in it is broken.
	note.Links = nil
	for _, l := range ReadyItemLinks(note, LinksOf(liveA, keptC)) {
		if l.Live || l.Address != "" {
			t.Fatalf("with nothing listed, %+v is live", l)
		}
	}
	plain := msg(6, 1)
	plain.Outsider = false
	if got := ReadyItemLinks(plain, LinksOf(liveA)); len(got) != 0 {
		t.Fatalf("an item with no link lists %+v", got)
	}
}

func linkReady(t *testing.T) Hub {
	t.Helper()
	h := readyHub(t, linkItem(), draft(5, 0))
	h, _ = apply(t, h, LinksLoaded{At: t0, Rev: 1, Live: []string{liveA}}, ReadyOpen{At: t0})
	return h
}

func readyKeys(t *testing.T, h Hub, actions ...ReadyAction) (Hub, []Effect) {
	t.Helper()
	var evs []Event
	for _, a := range actions {
		evs = append(evs, ReadyKey{At: t0, Action: a})
	}
	return apply(t, h, evs...)
}

func TestTheReadyViewListsAnItemsLinksAndKeepsOneAfterAYes(t *testing.T) {
	h := linkReady(t)
	h, _ = readyKeys(t, h, ReadyLinksKey)
	it, links, ok := ReadyLinks(h.Ready, h.App.Links)
	if !ok || it.ID != "message:4" || !reflect.DeepEqual(links, []ShownLink{liveRow(ownA), {Text: brokenC}}) || h.Ready.Links.Sel != 1 {
		t.Fatalf("list %+v for %q, sel %+v", links, it.ID, h.Ready.Links)
	}
	h, _ = readyKeys(t, h, ReadyOpenKey)
	if ask := h.Ready.Links.Ask; ask == nil || *ask != (LinkAsk{Text: brokenC, Keep: true}) {
		t.Fatalf("ask %+v", h.Ready.Links.Ask)
	}
	if HubSurface(h).Kind != SurfaceReadyLinkAsk {
		t.Fatalf("surface %+v", HubSurface(h))
	}
	// y at once answers nothing, and the view is told why.
	h, fx := readyKeys(t, h, ReadyYes)
	if len(effectsOf[SendLink](fx)) != 0 || h.Ready.Links.Ask == nil || h.Notice != TooSoonNote {
		t.Fatalf("kept before the question was armed: %v, notice %q", fx, h.Notice)
	}
	// A replayed key answers nothing either.
	late := t0.Add(2 * ArmDelay)
	r, fx := apply(t, h, KeyPressed{At: late, Replayed: true}, ReadyKey{At: late, Action: ReadyYes})
	if len(effectsOf[SendLink](fx)) != 0 || r.Ready.Links.Ask == nil || r.Notice != ReplayedNote {
		t.Fatalf("a replayed key kept the link: %v, notice %q", fx, r.Notice)
	}
	// R does not close the view over the question, and esc says no.
	n, _ := apply(t, h, ReadyOpen{At: late})
	if !n.Ready.Open || n.Ready.Links == nil || n.Ready.Links.Ask == nil {
		t.Fatalf("R closed the view over the question: %+v", n.Ready)
	}
	n, fx = apply(t, h, ReadyKey{At: late, Action: ReadyEsc})
	if len(effectsOf[SendLink](fx)) != 0 || n.Ready.Links == nil || n.Ready.Links.Ask != nil || n.Ready.Message != LinkStaysBrokenNote {
		t.Fatalf("esc: %v %+v %q", fx, n.Ready.Links, n.Ready.Message)
	}
	h, fx = apply(t, h, ReadyKey{At: t0.Add(ArmDelay), Action: ReadyYes})
	if got := effectsOf[SendLink](fx); !reflect.DeepEqual(got, []SendLink{{Address: brokenC, Keep: true}}) {
		t.Fatalf("sent %+v", got)
	}
	h, _ = apply(t, h, LinkDone{At: t0, Address: brokenC, Keep: true, Changed: true})
	if h.Ready.Message != LinkKeptNote || h.Ready.Links.Sending != nil {
		t.Fatalf("after relay's answer: %q %+v", h.Ready.Message, h.Ready.Links)
	}
	// A refusal shows relay's reason.
	h, _ = readyKeys(t, h, ReadyOpenKey)
	h, _ = apply(t, h, ReadyKey{At: t0.Add(time.Hour), Action: ReadyYes},
		LinkDone{At: t0, Address: brokenC, Keep: true, Err: "A link is one http or https address in plain ASCII."})
	if h.Ready.Message != LinkNotKeptNote || h.Ready.Failure != "A link is one http or https address in plain ASCII." {
		t.Fatalf("after a refusal: %q %q", h.Ready.Message, h.Ready.Failure)
	}
	// d on the live link asks to drop it.
	h, _ = readyKeys(t, h, ReadyUp, ReadyDismiss)
	if ask := h.Ready.Links.Ask; ask == nil || *ask != (LinkAsk{Text: shownA, Address: liveA}) {
		t.Fatalf("ask %+v", h.Ready.Links.Ask)
	}
	h, fx = apply(t, h, ReadyKey{At: t0.Add(2 * time.Hour), Action: ReadyYes})
	if got := effectsOf[SendLink](fx); !reflect.DeepEqual(got, []SendLink{{Address: liveA, Keep: false}}) {
		t.Fatalf("sent %+v", got)
	}
	// L closes the list, and R the view with it.
	c, _ := readyKeys(t, h, ReadyLinksKey)
	if c.Ready.Links != nil || !c.Ready.Open {
		t.Fatalf("L left the list open: %+v", c.Ready.Links)
	}
	c, _ = apply(t, h, ReadyOpen{At: t0})
	if c.Ready.Open || c.Ready.Links != nil {
		t.Fatalf("R left the list: %+v", c.Ready)
	}
}

func TestTheReadyListIsForTheItemInFront(t *testing.T) {
	h := linkReady(t)
	// The draft has nothing from outside.
	n, _ := readyKeys(t, h, ReadyDown, ReadyLinksKey)
	if n.Ready.Links != nil || n.Ready.Message != NoLinksNote {
		t.Fatalf("a list for an item with no links: %+v %q", n.Ready.Links, n.Ready.Message)
	}
	// With an item open whole, the list is for that one, and esc goes
	// back to it.
	h, _ = readyKeys(t, h, ReadyOpenKey, ReadyLinksKey)
	if h.Ready.Links == nil || h.Ready.Links.Item != "message:4" || h.Ready.Detail != "message:4" {
		t.Fatalf("list %+v detail %q", h.Ready.Links, h.Ready.Detail)
	}
	h, _ = readyKeys(t, h, ReadyEsc)
	if h.Ready.Links != nil || h.Ready.Detail != "message:4" {
		t.Fatalf("esc: list %+v detail %q", h.Ready.Links, h.Ready.Detail)
	}
	// The keys of the list are its own: ? shows them.
	h, _ = readyKeys(t, h, ReadyLinksKey, ReadyHelp)
	if !h.Ready.Help || h.Ready.Links == nil {
		t.Fatalf("help %v list %+v", h.Ready.Help, h.Ready.Links)
	}
}

func TestTheReadyQuestionGoesWhenItsLinkChanges(t *testing.T) {
	asked := func() Hub {
		h := linkReady(t)
		h, _ = readyKeys(t, h, ReadyLinksKey, ReadyOpenKey)
		if h.Ready.Links == nil || h.Ready.Links.Ask == nil {
			t.Fatal("no question")
		}
		return h
	}
	kept := keptItem()
	h, _ := apply(t, asked(), LinksChanged{At: t0, Rev: 11, Live: []string{keptC}})
	// The set changed first: the item does not list the link yet, so the
	// question stands until the item is sent again.
	if h.Ready.Links.Ask == nil {
		t.Fatal("the question went before the item changed")
	}
	h, _ = apply(t, h, ReadyChanged{At: t0, Rev: 12, Items: []ReadyItem{kept}})
	if h.Ready.Links == nil || h.Ready.Links.Ask != nil || h.Ready.Message != LinkChangedNote || HubSurface(h).Kind != SurfaceReady {
		t.Fatalf("the question stayed: %+v %q", h.Ready.Links, h.Ready.Message)
	}
	_, fx := apply(t, h, ReadyKey{At: t0.Add(time.Hour), Action: ReadyYes})
	if len(effectsOf[SendLink](fx)) != 0 {
		t.Fatalf("a y after the question went kept something: %v", fx)
	}
	// A live link dropped elsewhere while the drop question is up.
	d := linkReady(t)
	d, _ = readyKeys(t, d, ReadyLinksKey, ReadyUp, ReadyDismiss)
	d, _ = apply(t, d, LinksChanged{At: t0, Rev: 11, Gone: []string{liveA}})
	if d.Ready.Links == nil || d.Ready.Links.Ask != nil || d.Ready.Message != LinkChangedNote {
		t.Fatalf("the drop question stayed: %+v %q", d.Ready.Links, d.Ready.Message)
	}
	// The item left the view: the list goes with it.
	h, _ = apply(t, asked(), ReadyChanged{At: t0, Rev: 13, Gone: []string{"message:4"}})
	if h.Ready.Links != nil || HubSurface(h).Kind != SurfaceReady {
		t.Fatalf("the list outlived its item: %+v", h.Ready.Links)
	}
}

// sentListed says address is the one relay lists for the broken form
// shown, by the two links these tests know and not by the list under
// test.
func sentListed(address, shown string) bool {
	return slices.Contains([]Link{ownA, ownC}, Link{Address: address, Shown: shown})
}

// linkQuestionsAnswered counts the keeps and drops the property below
// saw go out, so it can say it reached what it is about.
var linkQuestionsAnswered int

// The question about a link is about the link it showed. Whatever
// changes, and whenever, a keep or a drop only goes out for the link the
// question was asked about, still listed as it was then, from a key
// pressed ArmDelay after the question came up.
func TestTheLinkQuestionIsAboutWhatItShowed(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		at := t0
		x := onLinkBoard()
		x, _ = feedS(x, KeyPressed{At: t0})
		became, becameAt := SingleSurfaceID(x), t0
		liveC := false
		for step := range rapid.IntRange(1, 60).Draw(t, "steps") {
			at = at.Add(time.Duration(rapid.IntRange(0, 900).Draw(t, "ms")) * time.Millisecond)
			var e Event
			switch what := rapid.IntRange(0, 11).Draw(t, "what"); {
			case what <= 3 && x.Board.Links == nil:
				e = PaneKey{At: at, Action: PaneLinks}
			case what <= 3: // in the list: move, and ask
				e = PaneKey{At: at, Action: rapid.SampledFrom([]PaneAction{PaneUp, PaneDown, PaneEnter, PaneEnter, PaneDrop, PaneDrop}).Draw(t, "key")}
			case what == 4: // out of the question, or of the list
				e = PaneKey{At: at, Action: rapid.SampledFrom([]PaneAction{PaneEsc, PaneOther, PaneLinks}).Draw(t, "leave")}
			case what == 5: // the card is sent again, its second link listed or not
				c := linkCard(10, int64(100+step))
				if liveC = rapid.Bool().Draw(t, "listed"); liveC {
					c = keptCard(10, int64(100+step))
				}
				e = CardsChanged{At: at, Rev: c.Rev, Cards: []Card{c}}
			case what == 6: // the set changes
				ch := LinksChanged{At: at, Rev: int64(100 + step)}
				for _, l := range []string{liveA, keptC} {
					if rapid.Bool().Draw(t, "live") {
						ch.Live = append(ch.Live, l)
					} else {
						ch.Gone = append(ch.Gone, l)
					}
				}
				e = ch
			default:
				e = PaneKey{At: at, Action: PaneYes}
			}
			replayed := rapid.IntRange(0, 5).Draw(t, "replayed") == 0
			x, _ = ReduceSingle(x, KeyPressed{At: at, Replayed: replayed})
			id := SingleSurfaceID(x)
			var asked *LinkAsk
			if x.Board.Links != nil {
				asked = x.Board.Links.Ask
			}
			_, listed, _ := BoardLinks(x.Board, x.Links)
			next, fx := ReduceSingle(x, e)
			for _, f := range effectsOf[SendLink](fx) {
				linkQuestionsAnswered++
				switch {
				case asked == nil || f.Keep != asked.Keep:
					t.Fatalf("step %d: sent %+v for the question %+v", step, f, asked)
				case f.Keep && (f.Address != asked.Text || asked.Address != ""):
					t.Fatalf("step %d: kept %q, and the question showed %+v", step, f.Address, asked)
				case !f.Keep && !sentListed(f.Address, asked.Text):
					t.Fatalf("step %d: dropped %q for the question %+v, which is not the address relay listed for what it showed", step, f.Address, asked)
				case !slices.Contains(listed, ShownLink{Text: asked.Text, Address: asked.Address, Live: !asked.Keep}):
					t.Fatalf("step %d: sent %+v, and the link was no longer listed that way: %+v", step, f, listed)
				case replayed || became != id || at.Sub(becameAt) < ArmDelay:
					t.Fatalf("step %d: %+v answered on %q, up since %q %v before (replayed %v)", step, f, id, became, at.Sub(becameAt), replayed)
				}
			}
			x = next
			if SingleSurfaceID(x) != id {
				became, becameAt = SingleSurfaceID(x), at
			}
		}
	})
	if linkQuestionsAnswered == 0 {
		t.Fatal("no run kept or dropped a link: the property did not reach its question")
	}
	t.Logf("%d links kept or dropped", linkQuestionsAnswered)
}

// The same in the ready view: a keep or a drop goes out only for the
// link its question showed, still listed that way, from a live key
// pressed ArmDelay after the question came up.
func TestTheReadyLinkQuestionIsAboutWhatItShowed(t *testing.T) {
	answered := 0
	rapid.Check(t, func(t *rapid.T) {
		at := t0
		h := readyHub(t, linkItem(), draft(5, 0))
		h, _ = apply(t, h, LinksLoaded{At: t0, Rev: 1, Live: []string{liveA}}, KeyPressed{At: t0}, ReadyOpen{At: t0})
		became, becameAt := HubSurfaceID(h), t0
		for step := range rapid.IntRange(1, 60).Draw(t, "steps") {
			at = at.Add(time.Duration(rapid.IntRange(0, 900).Draw(t, "ms")) * time.Millisecond)
			var e Event
			switch what := rapid.IntRange(0, 11).Draw(t, "what"); {
			case what <= 3 && h.Ready.Links == nil:
				e = ReadyKey{At: at, Action: ReadyLinksKey}
			case what <= 3:
				e = ReadyKey{At: at, Action: rapid.SampledFrom([]ReadyAction{ReadyUp, ReadyDown, ReadyOpenKey, ReadyOpenKey, ReadyDismiss, ReadyDismiss}).Draw(t, "key")}
			case what == 4:
				e = ReadyKey{At: at, Action: rapid.SampledFrom([]ReadyAction{ReadyEsc, ReadyOther, ReadyLinksKey}).Draw(t, "leave")}
			case what == 5: // the item is sent again, its second link listed or not
				it := linkItem()
				if rapid.Bool().Draw(t, "listed") {
					it = keptItem()
				}
				e = ReadyChanged{At: at, Rev: int64(100 + step), Items: []ReadyItem{it}}
			case what == 6: // the set changes
				ch := LinksChanged{At: at, Rev: int64(100 + step)}
				for _, l := range []string{liveA, keptC} {
					if rapid.Bool().Draw(t, "live") {
						ch.Live = append(ch.Live, l)
					} else {
						ch.Gone = append(ch.Gone, l)
					}
				}
				e = ch
			default:
				e = ReadyKey{At: at, Action: ReadyYes}
			}
			replayed := rapid.IntRange(0, 5).Draw(t, "replayed") == 0
			h, _ = ReduceHub(h, KeyPressed{At: at, Replayed: replayed})
			id := HubSurfaceID(h)
			var asked *LinkAsk
			if h.Ready.Links != nil {
				asked = h.Ready.Links.Ask
			}
			_, listed, _ := ReadyLinks(h.Ready, h.App.Links)
			next, fx := ReduceHub(h, e)
			for _, f := range effectsOf[SendLink](fx) {
				answered++
				switch {
				case asked == nil || f.Keep != asked.Keep:
					t.Fatalf("step %d: sent %+v for the question %+v", step, f, asked)
				case f.Keep && (f.Address != asked.Text || asked.Address != ""):
					t.Fatalf("step %d: kept %q, and the question showed %+v", step, f.Address, asked)
				case !f.Keep && !sentListed(f.Address, asked.Text):
					t.Fatalf("step %d: dropped %q for the question %+v, which is not the address relay listed for what it showed", step, f.Address, asked)
				case !slices.Contains(listed, ShownLink{Text: asked.Text, Address: asked.Address, Live: !asked.Keep}):
					t.Fatalf("step %d: sent %+v, and the link was no longer listed that way: %+v", step, f, listed)
				case replayed || became != id || at.Sub(becameAt) < ArmDelay:
					t.Fatalf("step %d: %+v answered on %q, up since %q %v before (replayed %v)", step, f, id, became, at.Sub(becameAt), replayed)
				}
			}
			h = next
			if HubSurfaceID(h) != id {
				became, becameAt = HubSurfaceID(h), at
			}
		}
	})
	if answered == 0 {
		t.Fatal("no run kept or dropped a link: the property did not reach its question")
	}
	t.Logf("%d links kept or dropped", answered)
}

// --- asking for the set again after a failure

func TestAFailedLinksCallIsMadeAgainOnceRelayAnswers(t *testing.T) {
	failed := func() Hub {
		h, fx := apply(t, NewHub(""), HubConnected{At: t0})
		if len(effectsOf[RequestLinks](fx)) != 1 {
			t.Fatalf("asked %d times at the start", len(effectsOf[RequestLinks](fx)))
		}
		h, fx = apply(t, h, LinksLoaded{At: t0, Err: "relay has not read its links yet. Try again."})
		if len(effectsOf[RequestLinks](fx)) != 0 {
			t.Fatal("asked again at once, with nothing to say relay is ready")
		}
		return h
	}
	for name, e := range map[string]Event{
		"the board loads":      CardsLoaded{At: t0, Rev: 5, Cards: []Card{checklist(10, 5)}},
		"the ready view loads": ReadyLoaded{At: t0, Rev: 5, Items: []ReadyItem{msg(4, 1)}},
		"the ready view opens": ReadyOpen{At: t0},
	} {
		h, fx := apply(t, failed(), e)
		if len(effectsOf[RequestLinks](fx)) != 1 {
			t.Errorf("%s: asked %d times", name, len(effectsOf[RequestLinks](fx)))
		}
		// And not a second time while that call is out.
		_, fx = apply(t, h, CardsLoaded{At: t0, Rev: 6, Cards: []Card{checklist(10, 6)}},
			ReadyLoaded{At: t0, Rev: 6, Items: []ReadyItem{msg(4, 1)}}, ReadyOpen{At: t0}, ReadyOpen{At: t0})
		if len(effectsOf[RequestLinks](fx)) != 0 {
			t.Errorf("%s: asked again while a call was out", name)
		}
	}
	// A board or a ready view that failed to load says nothing about
	// relay answering.
	_, fx := apply(t, failed(), CardsLoaded{At: t0, Err: "no"}, ReadyLoaded{At: t0, Err: "no"})
	if len(effectsOf[RequestLinks](fx)) != 0 {
		t.Fatalf("asked again after a failed load: %v", fx)
	}
	// A reload key asks again too.
	h := failed()
	h, _ = apply(t, h, ReadyLoaded{At: t0, Rev: 5, Items: []ReadyItem{msg(4, 1)}}, LinksLoaded{At: t0, Err: "still not"})
	_, fx = apply(t, h, ReadyOpen{At: t0}, LinksLoaded{At: t0, Err: "still not"}, ReadyKey{At: t0, Action: ReadyRefresh})
	if len(effectsOf[RequestLinks](fx)) != 2 {
		t.Fatalf("opening and then reloading asked %d times, want 2", len(effectsOf[RequestLinks](fx)))
	}
}

func TestNothingIsAskedTwiceWhileTheFirstLinksCallIsOut(t *testing.T) {
	h, fx := apply(t, NewHub(""), HubConnected{At: t0},
		CardsLoaded{At: t0, Rev: 5, Cards: []Card{checklist(10, 5)}},
		ReadyLoaded{At: t0, Rev: 5, Items: []ReadyItem{msg(4, 1)}},
		ReadyOpen{At: t0}, ReadyKey{At: t0, Action: ReadyRefresh}, ReadyOpen{At: t0})
	if n := len(effectsOf[RequestLinks](fx)); n != 1 {
		t.Fatalf("asked %d times before the first answer", n)
	}
	// Nor once it has answered.
	_, fx = apply(t, h, LinksLoaded{At: t0, Rev: 5, Live: []string{liveA}},
		CardsLoaded{At: t0, Rev: 6, Cards: []Card{checklist(10, 6)}}, ReadyOpen{At: t0}, ReadyKey{At: t0, Action: ReadyRefresh})
	if n := len(effectsOf[RequestLinks](fx)); n != 0 {
		t.Fatalf("asked %d times after the answer", n)
	}
	// Against a single agent the same holds for the board's reload key.
	x, fx := feedS(Single{}, Connected{At: t0}, key(t0, PaneToggle), key(t0, PaneRefresh),
		CardsLoaded{At: t0, Rev: 5, Cards: []Card{checklist(10, 5)}}, key(t0, PaneRefresh))
	if n := len(effectsOf[RequestLinks](fx)); n != 1 {
		t.Fatalf("a single agent was asked %d times before the first answer", n)
	}
	_ = x
}

func TestARelayWithoutLinksIsNeverAskedAgain(t *testing.T) {
	h, _ := apply(t, NewHub(""), HubConnected{At: t0}, LinksLoaded{At: t0, Unavailable: true})
	_, fx := apply(t, h, CardsLoaded{At: t0, Rev: 5, Cards: []Card{checklist(10, 5)}},
		ReadyLoaded{At: t0, Rev: 5, Items: []ReadyItem{msg(4, 1)}}, ReadyOpen{At: t0}, ReadyKey{At: t0, Action: ReadyRefresh})
	if n := len(effectsOf[RequestLinks](fx)); n != 0 {
		t.Fatalf("asked %d times of a relay without the method", n)
	}
}

// A keep or a drop still on its way when its list closes is not
// forgotten. relay's answer is said where the list was, a list opened in
// the meantime takes the call back and stays busy until the answer, and
// an answer to some other call says nothing.
func TestAnAnswerToAClosedListIsStillSaid(t *testing.T) {
	// The board.
	s := onLinkBoard()
	s, _ = boardKeys(s, PaneLinks, PaneEnter)
	s, fx := feedS(s, key(t0.Add(ArmDelay), PaneYes))
	if len(effectsOf[SendLink](fx)) != 1 {
		t.Fatalf("sent %v", fx)
	}
	s, _ = boardKeys(s, PaneEsc)
	if s.Board.Links != nil || s.Board.LinkOut == nil || *s.Board.LinkOut != (LinkAsk{Text: brokenC, Keep: true}) {
		t.Fatalf("after esc: list %+v, out %+v", s.Board.Links, s.Board.LinkOut)
	}
	other, _ := feedS(s, LinkDone{At: t0, Address: "hxxps://other[.]example/x", Keep: true, Changed: true})
	if other.Board.Message != "" || other.Board.LinkOut == nil {
		t.Fatalf("an answer to another call: %q, out %+v", other.Board.Message, other.Board.LinkOut)
	}
	said, _ := feedS(s, LinkDone{At: t0, Address: brokenC, Keep: true, Changed: true})
	if said.Board.Message != LinkKeptNote || said.Board.LinkOut != nil {
		t.Fatalf("the answer after the list closed: %q, out %+v", said.Board.Message, said.Board.LinkOut)
	}
	refused, _ := feedS(s, LinkDone{At: t0, Address: brokenC, Keep: true, Err: "relay refused it."})
	if refused.Board.Message != LinkNotKeptNote || refused.Board.Failure != "relay refused it." {
		t.Fatalf("a refusal after the list closed: %q %q", refused.Board.Message, refused.Board.Failure)
	}
	// Opened again before the answer, the list has the call and is busy.
	again, _ := boardKeys(s, PaneLinks)
	if again.Board.Links == nil || again.Board.Links.Sending == nil || again.Board.LinkOut != nil {
		t.Fatalf("opened again: %+v, out %+v", again.Board.Links, again.Board.LinkOut)
	}
	busy, fx := boardKeys(again, PaneEnter)
	if busy.Board.Links.Ask != nil || busy.Board.Message != LinkBusyNote || len(fx) != 0 {
		t.Fatalf("a second keep while one is out: %+v %q", busy.Board.Links, busy.Board.Message)
	}
	again, _ = feedS(again, LinkDone{At: t0, Address: brokenC, Keep: true, Changed: true})
	if again.Board.Links.Sending != nil || again.Board.Message != LinkKeptNote {
		t.Fatalf("the answer in the list opened again: %+v %q", again.Board.Links, again.Board.Message)
	}
	// Switching to the workspace items closes the list the same way.
	b, _ := boardKeys(s, PaneLinks, PaneBoard)
	if b.Board.Links != nil || b.Board.LinkOut == nil {
		t.Fatalf("b over a list with a call out: %+v, out %+v", b.Board.Links, b.Board.LinkOut)
	}

	// The ready view: a drop, and the view closed over it.
	h := linkReady(t)
	h, _ = readyKeys(t, h, ReadyLinksKey, ReadyUp, ReadyDismiss)
	h, fx = apply(t, h, ReadyKey{At: t0.Add(ArmDelay), Action: ReadyYes})
	if got := effectsOf[SendLink](fx); !reflect.DeepEqual(got, []SendLink{{Address: liveA}}) {
		t.Fatalf("sent %+v", got)
	}
	h, _ = readyKeys(t, h, ReadyEsc)
	if h.Ready.Links != nil || h.Ready.LinkOut == nil || *h.Ready.LinkOut != (LinkAsk{Text: shownA, Address: liveA}) {
		t.Fatalf("after esc: list %+v, out %+v", h.Ready.Links, h.Ready.LinkOut)
	}
	// An answer about the form the list showed is not this drop's: a
	// drop sends the address.
	wrong, _ := apply(t, h, LinkDone{At: t0, Address: shownA, Changed: true})
	if wrong.Ready.Message != "" || wrong.Ready.LinkOut == nil {
		t.Fatalf("an answer about the shown form: %q", wrong.Ready.Message)
	}
	h, _ = apply(t, h, LinkDone{At: t0, Address: liveA, Changed: true})
	if h.Ready.Message != LinkDroppedNote || h.Ready.LinkOut != nil {
		t.Fatalf("the answer after the list closed: %q, out %+v", h.Ready.Message, h.Ready.LinkOut)
	}
}

// A yes pressed while the frame did not show the link's whole host keeps
// nothing: no call is made, the question stays, and a note says why. The
// same yes with the host on screen keeps it.
func TestAYesToAHostNotOnScreenIsRefused(t *testing.T) {
	if got := LinkHost("hxxps://a[.]b[.]evil[.]test/x?y#z"); got != "hxxps://a[.]b[.]evil[.]test" {
		t.Fatalf("LinkHost: %q", got)
	}
	if got := LinkHost("hxxps://a[.]example"); got != "hxxps://a[.]example" {
		t.Fatalf("LinkHost with no path: %q", got)
	}
	s, _ := boardKeys(onLinkBoard(), PaneLinks, PaneEnter)
	if s.Board.Links.Ask == nil {
		t.Fatal("no question")
	}
	at := t0.Add(ArmDelay)
	h, fx := feedS(s, PaneKey{At: at, Action: PaneYes, Hidden: true})
	if len(effectsOf[SendLink](fx)) != 0 || h.Board.Links.Ask == nil || h.State.Message.Text != LinkHiddenNote {
		t.Fatalf("a hidden yes: %v, %+v, %q", fx, h.Board.Links, h.State.Message.Text)
	}
	if _, fx := feedS(s, PaneKey{At: at, Action: PaneYes}); len(effectsOf[SendLink](fx)) != 1 {
		t.Fatalf("a yes with the host on screen sent %v", fx)
	}
}
