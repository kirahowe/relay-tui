package core

import (
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
