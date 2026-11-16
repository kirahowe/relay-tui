package render

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/kirahowe/relay-tui/internal/core"
)

// Against a relay that sends relay/inertText, a contaminated session's
// text arrives inert: every link broken, hidden characters shown as
// escapes, Markdown's link and HTML syntax escaped. relay-tui draws it as
// relay sent it, and a reply's hyperlinks are the links relay listed for
// it.

// inertSamples is relay's own output for conversation text, taken from
// its fuzzing (relay#104) with its Markdown escapes applied: broken
// schemes, [:]//, hidden characters as escapes, fullwidth letters, and
// emoji held together by a joiner, one of them after an escaped variation
// selector, which relay-tui's own breaker would take apart. The last is
// written here: the fuzz held no "](".
var inertSamples = []string{
	"hxxps://1・a--b・ai\\u{200b}/x",
	"hxxps://a:b@com[.]pdf@x",
	"=fxp://0x7f[.]data[.]évil܁ai",
	"h\\u{200b}ttps[:]//x[.]data%2Ecom:80",
	"\"h𜱫ttps[:]//пример%👨\u200d👩\u200d👧2Eme!",
	"x:ssh[:]x@se#\ufe0f\u20e3\U00011ca3cret\\u{00ad}[.]x﹒us/a/b?c=d",
	"?ssh[:]x@1[.]xn--e1afmkfd﹒json/[.]\\:1secret[.]attacker[.]com/x",
	"ｈｔｔｐｓ[:]//-x[.]\\u{200b}io/[.]\\:80/x",
	"\\<itms-services[:]//пример[.]do\\\\x",
	"\\<ｈ\u0301ｔｔｐｓ[:]//x﹒\\u{fe0f}\u20e3com[.]dev[.]/)",
	"\\<hxxps://0x7f܁ms!",
	"éｈｔｔｐｓ[:]//1%2Eevil܁secret[.]io/a/a/b?c=\u0323d",
	"me@a[.]bc:1hxxps://secret[.]d👨\u200d👩\u200d👧ev!",
	"\\u{00ad}hxxps:a--b[.]123[.]py❤👍\ufe0f\\u{fe0f}\u200d🔥/a/b?c=d?",
	"|hxxps:/-x\\u{200b}❤\u1161\\u{fe0f}\u200d🔥[.]com-x/",
	"a-WWW[.]例%2Ees-repo❤\\u{115f}\\u{fe0f}\u200d🔥rt[.]txt",
	"1http:\\\\\\\\data[.]museum!",
	"WWW[.]x-[.]zip:1secret[.]attacker[.]com/x",
	"www[.]da\u20e3ta・us:80/x",
	"x[.]yz:9https:/\\u{f4a2}a-b%2Exn--p1ai\\u{00ad}…",
	"[ssh[:]123[.]\\u{200b}evil\\u{00ad}[.]md5)",
	"Read [the fund]\\(hxxps://fund[.]example[.]org/apply) and [a]\\: \\<b> today.",
}

func jsonString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func inertLinks(addrs ...string) core.Links {
	l := core.LinksOf(addrs...)
	l.InertText = true
	return l
}

// inertReply is the rows a contaminated reply is drawn in, and the frame
// they make once gated.
func inertReply(text string, own []core.Link, live core.Links) (rows []string, frame string) {
	e := core.Entry{Kind: core.Agent, Text: text, Final: true, Defang: true, Links: own}
	rows = entry(e, live, 200, t0)
	return rows, gate(strings.Join(rows, "\n"), live)
}

// shownWords is the text of rows as a reader sees it, a space between
// words.
func shownWords(s string) string { return strings.Join(strings.Fields(ansi.Strip(s)), " ") }

func TestRelaysInertTextIsDrawnAsItCame(t *testing.T) {
	live := inertLinks("https://fund.example.org/apply")
	for _, s := range inertSamples {
		rows, frame := inertReply(s, nil, live)
		joined := strings.Join(rows, "\n")
		if got, want := shownWords(joined), shownWords(s); got != want {
			t.Errorf("drawn other than sent:\n got %q\nwant %q", got, want)
		}
		for _, r := range rows {
			if followable(ansi.Strip(r)) {
				t.Errorf("a row can be followed: %q", ansi.Strip(r))
			}
		}
		if frame != joined || hasLinks(frame) {
			t.Errorf("the gate changed relay's text, or a hyperlink was made:\n%q\n%q", joined, frame)
		}
	}
}

// A thought, a tool call and a notice from such a session are drawn as
// relay sent them too.
func TestEverythingElseInertIsDrawnAsItCame(t *testing.T) {
	for _, s := range inertSamples {
		st := core.State{Phase: core.Idle, Ready: true, Labels: core.Labels{Contaminated: true},
			Transcript: core.Transcript{Tools: true, Thinking: true},
			Entries: []core.Entry{
				{Kind: core.Thought, Text: s, Final: true, Defang: true},
				{Kind: core.Tool, Calls: []core.Call{{ID: "1", Name: "web_get", Input: `{"q":` + jsonString(s) + `}`,
					Output: s, Status: core.OK}}, Final: true, Defang: true},
				{Kind: core.Tool, Calls: []core.Call{{ID: "2", Title: s, Status: core.OK}}, Final: true, Defang: true},
				{Kind: core.Note, Text: s, Final: true, Defang: true},
			}}
		frame := TranscriptView(st, inertLinks(), t0, 200, 40).Content
		plain := ansi.Strip(frame)
		if n := strings.Count(plain, strings.TrimSpace(s)); n != 5 {
			t.Errorf("%q is drawn as sent %d times of 5:\n%s", s, n, plain)
		}
		if followable(plain) || hasLinks(frame) {
			t.Errorf("something can be followed:\n%q", frame)
		}
	}
}

// hyperlinkRE finds a hyperlink and the text under it.
var hyperlinkRE = regexp.MustCompile("\x1b\\]8;;([^\x07\x1b]+)\x07(.*?)\x1b\\]8;;\x07")

// A reply's hyperlinks are the links relay listed for it, where the
// shown form stands in a word, and only to an address in the set.
// Nothing is read out of the text: a broken link relay did not list, and
// a whole address, which relay never sends, are no hyperlinks.
func TestAReplysHyperlinksAreTheLinksRelayListed(t *testing.T) {
	const (
		addr      = "https://fund.example.org/apply?round=2027"
		shown     = "hxxps://fund[.]example[.]org/apply?round=2027"
		notLive   = "https://bit.example/3xQ9"
		notLiveAs = "hxxps://bit[.]example/3xQ9"
		unlisted  = "hxxps://other[.]example/a"
	)
	own := []core.Link{{Address: addr, Shown: shown}, {Address: notLive, Shown: notLiveAs}}
	live := inertLinks(addr, "https://other.example/a")
	said := "Apply at (" + shown + "). Also " + notLiveAs + " and " + unlisted + " today."
	rows, frame := inertReply(said, own, live)
	if got, want := shownWords(frame), shownWords(said); got != want {
		t.Errorf("drawn other than sent:\n got %q\nwant %q", got, want)
	}
	links := hyperlinkRE.FindAllStringSubmatch(frame, -1)
	if len(links) != 1 || links[0][1] != addr || ansi.Strip(links[0][2]) != shown {
		t.Fatalf("want one hyperlink to %s over %s, got %q", addr, shown, links)
	}
	// The rows themselves hold no other hyperlink, before the gate.
	if links := hyperlinkRE.FindAllStringSubmatch(strings.Join(rows, "\n"), -1); len(links) != 1 || links[0][1] != addr {
		t.Fatalf("the rows link to %q", links)
	}
	for _, r := range rows {
		if followable(ansi.Strip(r)) {
			t.Errorf("a row can be followed: %q", ansi.Strip(r))
		}
	}

	// The same address whole, in a paragraph of its own and listed: no
	// hyperlink, and broken where it is drawn.
	_, frame = inertReply("See:\n\n"+addr+"\n\nand <"+addr+">", []core.Link{{Address: addr, Shown: addr}}, live)
	if hasLinks(frame) || followable(ansi.Strip(frame)) {
		t.Fatalf("a whole address was made followable:\n%q", frame)
	}
	// A text that holds the address whole beside its shown form: the
	// autolink it wrote itself is text, and the shown form is no link
	// either, so nothing in the text can pass for one of relay's.
	_, frame = inertReply("Apply at "+shown+"\n\nor <"+addr+">", own, live)
	if hasLinks(frame) || followable(ansi.Strip(frame)) {
		t.Fatalf("a whole address was made a hyperlink:\n%q", frame)
	}

	// Without inertText the reply is broken by relay-tui, as before: a
	// listed shown form is no hyperlink.
	plainLive := core.LinksOf(addr)
	if _, frame := inertReply(said, own, plainLive); hasLinks(frame) {
		t.Fatalf("a listed link was made a hyperlink without inertText:\n%q", frame)
	}
}
