package core

import (
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

func TestLinkShape(t *testing.T) {
	for _, ok := range []string{
		"https://x.example/a",
		"http://a",
		"https://a.b:8080/p?q=1&r=2#f",
		"https://x.example/a*b~c_d-e",
		"https://x.example/a%20b,c;d=e",
		"https://x.example/[1]@!$'+",
		"https://xn--e1afmkfd.xn--p1ai/",
	} {
		if !LinkShape(ok) {
			t.Errorf("%q is not taken", ok)
		}
	}
	for _, no := range []string{
		"",
		"https://",
		"HTTPS://x.example/a",    // the scheme is lower case
		"ftp://x.example/a",      // http and https only
		"https://user@x.example", // no user name before the host
		"https://x.example/a(b)", // no round bracket
		"https://x.example/ä",    // plain ASCII only
		"https://пример.рф/",
		"https://-x.example/",
		"https://x.example./a b",
		"https://x.example/a\x1b]8;;https://evil.test\x07",
		"https://x.example/a\x07",
		"https://x.example/a\\b",
		"https://x.example/a\"b",
		"https://x.example/a<b>",
		" https://x.example/a",
		"https://x.example/" + strings.Repeat("a", MaxLink),
	} {
		if LinkShape(no) {
			t.Errorf("%q is taken", no)
		}
	}
}

func TestLinksOfLeavesOutWhatIsNotALink(t *testing.T) {
	l := LinksOf("https://x.example/a", "javascript:alert(1)", "https://x.example/a(b)", "HTTPS://x.example")
	if len(l.Live) != 1 || !l.Has("https://x.example/a") {
		t.Fatalf("%v", l.Live)
	}
	// A set that was put together some other way is still held to the shape.
	raw := Links{Live: map[string]struct{}{"https://x.example/a\x07evil": {}}}
	if raw.Has("https://x.example/a\x07evil") {
		t.Fatal("a member out of shape counts as live")
	}
}

func TestLinkIn(t *testing.T) {
	a := "https://x.example/a"
	live := LinksOf(a, "https://x.example/b.", "https://x.example/b")
	for _, c := range []struct {
		word, before, link, after string
	}{
		{a, "", a, ""},
		{"<" + a + ">", "<", a, ">"},
		{"(" + a + ").", "(", a, ")."},
		{"**[a](" + a + ")**", "**[a](", a, ")**"},
		{"see:" + a + "?", "see:", a, "?"},
		{"evil.example/" + a, "evil.example/", a, ""},
		// The longest that is live wins: a link that ends in a closer is
		// found before the shorter one.
		{"https://x.example/b.", "", "https://x.example/b.", ""},
		{"https://x.example/b.,", "", "https://x.example/b.", ","},
		{"https://x.example/b,", "", "https://x.example/b", ","},
	} {
		before, link, after, ok := LinkIn(c.word, live)
		if !ok || before != c.before || link != c.link || after != c.after {
			t.Errorf("LinkIn(%q) = %q %q %q %v, want %q %q %q", c.word, before, link, after, ok, c.before, c.link, c.after)
		}
	}
	for _, word := range []string{
		"",
		"plain",
		a + "/more",                    // text run on after an address is another address
		a + "x",                        //
		a + ".evil.test",               //
		a + ")))))))",                  // more closers than a link takes
		a + ")é",                       // not only closers after it
		"https://evil.test/?u=" + a,    // the word's first scheme starts the link
		"https://x.example/A",          // character for character
		"http://x.example/a",           //
		"https://x.example/a%2e",       //
		strings.ToUpper(a),             //
		"hxxps://x[.]example/a",        // a broken link is not a live one
		"https://x.example/c",          // not in the set
		"www.x.example/a",              // no scheme
		"https://x.example/a\u200b",    // a hidden character after it
		"https://x.example/a\x1b[0m.",  // an escape after it
		"https://x.example/a<https://", //
	} {
		if before, link, after, ok := LinkIn(word, live); ok {
			t.Errorf("LinkIn(%q) = %q %q %q, want none", word, before, link, after)
		}
	}
	if _, _, _, ok := LinkIn(a, Links{}); ok {
		t.Error("a link is live with no live links")
	}
}

// Whatever the word and the set: a link LinkIn finds is a member of the
// set in a live link's shape, the three parts are the word, nothing but
// closers follows the link, and no scheme stands before it.
func TestLinkInFindsOnlyLiveLinks(t *testing.T) {
	pool := []string{"https://x.example/a", "https://x.example/a.", "http://y.example", "https://z.example/p?q=1&r=*a*",
		"https://x.example/a(b)", "ftp://x.example/a", "HTTPS://x.example/a", "https://evil.test/?d=1"}
	part := rapid.SampledFrom(append([]string{"", "<", ">", "(", ")", ".", ",", "**", "_", "see:", "x.y/", "é", " ", "\x1b[m", "/more", "?", "!", "]", "[l](", "://", "www."}, pool...))
	rapid.Check(t, func(t *rapid.T) {
		var set []string
		for _, p := range pool {
			if rapid.Bool().Draw(t, "in") {
				set = append(set, p)
			}
		}
		live := LinksOf(set...)
		word := strings.Join(rapid.SliceOfN(part, 1, 6).Draw(t, "parts"), "")
		if strings.ContainsAny(word, " ") {
			word = strings.ReplaceAll(word, " ", "")
		}
		before, link, after, ok := LinkIn(word, live)
		if !ok {
			return
		}
		if before+link+after != word {
			t.Fatalf("%q + %q + %q is not %q", before, link, after, word)
		}
		if _, in := live.Live[link]; !in || !LinkShape(link) {
			t.Fatalf("%q is not a live link of %v", link, set)
		}
		if len(after) > maxClosers || strings.Trim(after, linkClosers) != "" {
			t.Fatalf("%q after the link in %q", after, word)
		}
		if schemeRE.MatchString(before) {
			t.Fatalf("a scheme before the link: %q in %q", before, word)
		}
	})
}

// --- the set as the connection holds it

func linksOfState(a App) []string {
	var out []string
	for l := range a.Links.Live {
		out = append(out, l)
	}
	slices.Sort(out)
	return out
}

func TestTheLiveLinksAreAskedForOnceTheConnectionIsUp(t *testing.T) {
	// Against a single agent: once the session exists, and before the
	// board, so the board's links can be held to them as soon as it draws.
	_, fx := feedS(Single{}, Connected{At: t0})
	var asked []string
	for _, f := range fx {
		switch f.(type) {
		case RequestLinks:
			asked = append(asked, "links")
		case RequestCards:
			asked = append(asked, "cards")
		}
	}
	if !slices.Equal(asked, []string{"links", "cards"}) {
		t.Fatalf("asked %v, want the links and then the board", asked)
	}
	// In hub mode: as soon as the hub is connected, with no conversation.
	_, fx = ReduceHub(NewHub(""), HubConnected{At: t0})
	if len(effectsOf[RequestLinks](fx)) != 1 {
		t.Fatalf("the hub asked %d times: %#v", len(effectsOf[RequestLinks](fx)), fx)
	}
}

func TestTheLiveLinksFollowRelay(t *testing.T) {
	a, b, c := "https://x.example/a", "https://x.example/b", "https://x.example/c"
	x, _ := feedS(Single{}, Connected{At: t0})
	if !x.Links.Empty() || x.Links.Loaded {
		t.Fatalf("links before relay answered: %+v", x.Links)
	}
	x, _ = feedS(x, LinksLoaded{At: t0, Rev: 5, Live: []string{a, b, "javascript:alert(1)", "https://x.example/a(b)"}})
	if got := linksOfState(x.App); !slices.Equal(got, []string{a, b}) || !x.Links.Loaded {
		t.Fatalf("after the answer: %v", got)
	}
	// What is not in a link's shape is never held, from a change either.
	before := x.Links
	x, _ = feedS(x, LinksChanged{At: t0, Rev: 6, Live: []string{c, "https://x.example/\x07"}, Gone: []string{a}})
	if got := linksOfState(x.App); !slices.Equal(got, []string{b, c}) {
		t.Fatalf("after a change: %v", got)
	}
	// The set a frame was drawn from is not changed under it.
	if !before.Has(a) || before.Has(c) {
		t.Fatal("a change rewrote the set an earlier state holds")
	}
	// A change the answer already holds is not applied again.
	x, _ = feedS(x, LinksChanged{At: t0, Rev: 5, Live: []string{a}})
	if x.Links.Has(a) {
		t.Fatal("a change older than the answer was applied")
	}
}

func TestAnAnswerOlderThanAChangeIsAskedForAgain(t *testing.T) {
	a, b := "https://x.example/a", "https://x.example/b"
	x, _ := feedS(Single{}, Connected{At: t0})
	// The change overtook the answer: relay counts it as sent.
	x, _ = feedS(x, LinksChanged{At: t0, Rev: 9, Live: []string{b}})
	x, fx := feedS(x, LinksLoaded{At: t0, Rev: 8, Live: []string{a}})
	if len(effectsOf[RequestLinks](fx)) != 1 {
		t.Fatalf("the stale answer was not asked for again: %#v", fx)
	}
	if !x.Links.Has(b) {
		t.Fatal("the stale answer undid the change")
	}
	x, _ = feedS(x, LinksLoaded{At: t0, Rev: 9, Live: []string{a, b}})
	if got := linksOfState(x.App); !slices.Equal(got, []string{a, b}) {
		t.Fatalf("after the current answer: %v", got)
	}
}

func TestWithoutTheMethodEveryLinkStaysBroken(t *testing.T) {
	x, _ := feedS(Single{}, Connected{At: t0})
	x, _ = feedS(x, LinksLoaded{At: t0, Unavailable: true})
	if !x.Links.Unavailable || !x.Links.Empty() {
		t.Fatalf("%+v", x.Links)
	}
	// Nothing asks again: the agent has no such method.
	x.Board.Loaded = true
	_, fx := feedS(x, key(t0, PaneRefresh))
	if len(effectsOf[RequestLinks](fx)) != 0 {
		t.Fatalf("asked again: %#v", fx)
	}
	// A call that failed another way is tried again on a reload.
	y, _ := feedS(Single{}, Connected{At: t0})
	y, _ = feedS(y, LinksLoaded{At: t0, Err: "timed out"})
	y.PaneFocus = true
	_, fx = feedS(y, key(t0, PaneRefresh))
	if len(effectsOf[RequestLinks](fx)) != 1 {
		t.Fatalf("a reload did not ask again: %#v", fx)
	}
}

func TestTheHubHoldsTheLiveLinksWithOrWithoutAConversation(t *testing.T) {
	a := "https://x.example/a"
	h, _ := ReduceHub(NewHub(""), HubConnected{At: t0})
	h, _ = ReduceHub(h, LinksLoaded{At: t0, Rev: 3, Live: []string{a}})
	if !h.App.Links.Has(a) {
		t.Fatal("no conversation: the set was not kept")
	}
	h, _ = ReduceHub(h, RosterUpdate{At: t0, Roles: []Role{role("dev", "s1")}})
	h, _ = ReduceHub(h, Attached{At: t0, Role: "dev", Session: "s1"})
	h, _ = ReduceHub(h, LinksChanged{At: t0, Rev: 4, Gone: []string{a}, Live: []string{"https://x.example/b"}})
	if h.App.Links.Has(a) || !h.App.Links.Has("https://x.example/b") {
		t.Fatalf("with a conversation in focus: %v", linksOfState(h.App))
	}
}

// relay's own cases for relay.text/anchored, sent by the session that
// wrote it: one link is listed, and each text is one string of a thing.
func TestAnchored(t *testing.T) {
	const address = "https://grants.example.org/fund?year=2027"
	const shown = "hxxps://grants[.]example[.]org/fund?year=2027"
	links := []Link{{Address: "https://other.example/a", Shown: "hxxps://other[.]example/a"}, {Address: address, Shown: shown}}
	// anchors is every word of text that shows a listed link, as
	// before|shown|after.
	anchors := func(text string, links []Link) []string {
		var out []string
		for _, w := range strings.Fields(text) {
			if before, l, after, ok := Anchored(w, links); ok {
				if l.Address != address || before+l.Shown+after != w {
					t.Errorf("%q: anchored to %q, as %q %q %q", w, l.Address, before, l.Shown, after)
				}
				out = append(out, before+"|"+l.Shown+"|"+after)
			}
		}
		return out
	}
	for text, want := range map[string]string{
		"Apply at " + shown + ". Also hxxps://evil[.]example/x": "|" + shown + "|.",
		"(" + shown + ").":               "(|" + shown + "|).",
		"[the fund](" + shown + ")":      "fund](|" + shown + "|)",
		"**" + shown + "**":              "**|" + shown + "|**",
		shown + "?":                      "|" + shown + "|?",
		shown + ":":                      "|" + shown + "|:",
		"<" + shown + ">,":               "<|" + shown + "|>,",
		"x=" + shown:                     "x=|" + shown + "|",
		"evil[.]example/SECRET/" + shown: "evil[.]example/SECRET/|" + shown + "|",
		shown + ")**.":                   "|" + shown + "|)**.",
		shown:                            "|" + shown + "|",
		"see " + shown + " and " + shown + " now.": "|" + shown + "|,|" + shown + "|",
	} {
		if got := strings.Join(anchors(text, links), ","); got != want {
			t.Errorf("%q: anchors %q, want %q", text, got, want)
		}
	}
	for _, text := range []string{
		shown + "/x", shown + "[.]html", shown + "&d=SECRET", shown + "SECRET", // text runs on after it
		shown + ".......",                              // seven closers
		"hxxps://evil[.]example/x",                     // not listed
		"hxxps://grant[.]example[.]org/fund?year=2027", // one letter off
		"See " + address,                               // the whole address is never an anchor
		"hxxps://evil[.]example/?u=" + shown,           // it is the tail of another address
		"HXXPS://GRANTS[.]EXAMPLE[.]ORG/FUND?YEAR=2027",
		"",
	} {
		if got := anchors(text, links); len(got) != 0 {
			t.Errorf("%q: anchors %q, want none", text, got)
		}
	}
	// With nothing listed, nothing is anchored.
	for _, none := range [][]Link{nil, {}} {
		if got := anchors("Apply at "+shown+".", none); len(got) != 0 {
			t.Errorf("with no links: %q", got)
		}
	}
}

// Whatever the word and whatever is listed, an anchor is a listed link
// whose shown form is in the word whole, with only closers after it.
func TestAnchoredOnlyFindsWhatIsListed(t *testing.T) {
	shown := []string{"hxxps://a[.]example/x", "hxxp://b[.]example", "hxxps://a[.]example/x/y", "hxxps://c[.]example/?q=1"}
	part := rapid.SampledFrom(append([]string{"(", ")", ".", ",", "**", "<", ">", "see:", "x=", "/", "?", "[.]", "hxxp", "hxxps://", "://", "evil[.]test/", "SECRET", "'", "~", "_"}, shown...))
	rapid.Check(t, func(t *rapid.T) {
		var links []Link
		for i, s := range shown {
			if rapid.Bool().Draw(t, "listed") {
				links = append(links, Link{Address: "https://listed.example/" + string(rune('a'+i)), Shown: s})
			}
		}
		word := strings.Join(rapid.SliceOfN(part, 1, 8).Draw(t, "parts"), "")
		before, l, after, ok := Anchored(word, links)
		if !ok {
			return
		}
		if !slices.Contains(links, l) {
			t.Fatalf("%q: anchored to %v, which is not listed", word, l)
		}
		if before+l.Shown+after != word {
			t.Fatalf("%q: read as %q %q %q", word, before, l.Shown, after)
		}
		if strings.Contains(before, "hxxp") {
			t.Fatalf("%q: the link is not the word's first broken address", word)
		}
		if len(after) > maxClosers || strings.Trim(after, linkClosers) != "" {
			t.Fatalf("%q: %q follows the link", word, after)
		}
	})
}
