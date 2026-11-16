package render

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/kirahowe/relay-tui/internal/core"
)

// relay#130: a character that takes no cell of its own (a combining mark,
// a conjoining jamo, a joiner or selector that draws as nothing, keycap
// glue) can sit inside an address where a breaker that goes by letters
// does not see the address, while a link finder that skips such
// characters follows it. These tests judge every drawn row with the
// tests' own rule for what is skipped (skipped, in links_test.go).

// hidingInputs are the issue's table and the reviewer's inputs, with the
// three forms relay once left as written after a mark: a host with no
// dot, and a one-dot host that ends like a file.
var hidingInputs = []string{
	"https:\u0301//evil.com/steal?d=1",       // after https:
	"https:\u1161//evil.com/steal?d=1",       // a conjoining vowel after https:
	"https:\U0001107f//evil.com/steal?d=1",   // the Brahmi number joiner after https:
	"https:\U0001bc9d//evil.com/steal?d=1",   // the Duployan thick letter selector after https:
	"https:\ufe0f\u20e3//evil.com/steal?d=1", // keycap glue after https:
	"https:/\u0301/evil.com/steal?d=1",       // between the slashes
	"htt\u0301ps://evil.com/steal?d=1",       // inside https
	"ww\u0323w.evil.com/steal?d=1",           // inside www
	"evil\u0301.com/steal?d=1",               // inside a bare host
	"evil.co\u0301m",                         // inside the ending, no path
	"https:*\ufe0f\u20e3//evil.test*",        // the reviewer's Markdown case
	"https:\u0301//localhost/steal?d=1",      // a host with no dot
	"https:\u0301//3232235777/steal?d=1",
	"https:\u0301//0xC0A80001/steal?d=1",
	"https:\u0301//[2001:db8::1]/steal?d=1",
	"https:\u0301//evil.md", // a one-dot host that ends like a file
	"https:\u0301//setup.sh",
	"https:\u0301//notes.zip",
	"https:\u0301//notes.py",
}

// hidingSent is relay's conversation text for "Read " + each of
// hidingInputs + " today.", in the same order, made by
// relay.text/conversation-text on relay's integration at puznxukn
// (d4e82b1b) and pasted here as it came.
var hidingSent = []string{
	"Read hxxps://evil[.]com/steal?d=1 today.",
	"Read hxxps://evil[.]com/steal?d=1 today.",
	"Read hxxps://evil[.]com/steal?d=1 today.",
	"Read hxxps://evil[.]com/steal?d=1 today.",
	"Read hxxps://evil[.]com/steal?d=1 today.",
	"Read hxxps:/\u0301/evil[.]com/steal?d=1 today.",
	"Read hxxps://evil[.]com/steal?d=1 today.",
	"Read ww\u0323w[.]evil[.]com/steal?d=1 today.",
	"Read evil\u0301[.]com/steal?d=1 today.",
	"Read evil[.]co\u0301m today.",
	"Read https:*\ufe0f\u20e3//evil[.]test* today.",
	"Read hxxps://localhost/steal?d=1 today.",
	"Read hxxps://3232235777/steal?d=1 today.",
	"Read hxxps://0xC0A80001/steal?d=1 today.",
	"Read hxxps://[2001:db8::1]/steal?d=1 today.",
	"Read hxxps://evil[.]md today.",
	"Read hxxps://setup[.]sh today.",
	"Read hxxps://notes[.]zip today.",
	"Read hxxps://notes[.]py today.",
}

// noAddressSkipped fails on a drawn row that holds an address a finder
// skipping characters of no width would follow.
func noAddressSkipped(t *testing.T, what, content string) {
	t.Helper()
	for _, row := range strings.Split(ansi.Strip(content), "\n") {
		if followable(skipped(row)) {
			t.Errorf("%s: a finder that skips characters of no width follows\n%q", what, row)
		}
	}
}

// hidingState is a contaminated session in which an outsider's text is a
// reply, a thought and a tool call.
func hidingState(said string) core.State {
	st := core.State{Phase: core.Idle, Ready: true, Labels: core.Labels{Contaminated: true}, Entries: []core.Entry{
		{Kind: core.Agent, Text: said, Final: true, Defang: true},
		{Kind: core.Thought, Text: said, Final: true, Defang: true},
		{Kind: core.Tool, Calls: []core.Call{{ID: "1", Name: "web_get", Title: said, Input: said, Output: said, Status: core.OK}}, Final: true, Defang: true},
	}}
	st.Transcript.Tools, st.Transcript.Thinking = true, true
	return st
}

// Against a relay without relay/inertText, relay-tui breaks an
// outsider's links itself, wherever that text is drawn.
func TestNoCharacterOfNoWidthHidesAnAddressFromTheBreaker(t *testing.T) {
	for _, addr := range hidingInputs {
		said := "Read " + addr + " today."
		noticed := hubFixture()
		noticed.Notice = said
		for name, content := range map[string]string{
			"a reply, a thought and a call": TranscriptView(hidingState(said), core.Links{}, t0, 100, 40).Content,
			"the status line":               StatusText(core.StatusLine{Kind: core.StatusRunning, Detail: true, Tools: 1, Tool: said}, "", t0, 120),
			"a notice":                      HubView(noticed, "", "", t0, 120, 30).Content,
		} {
			noAddressSkipped(t, strconv.QuoteToASCII(addr)+" in "+name, content)
		}
	}
}

// Against a relay that sends relay/inertText, each input is drawn as
// relay sent it, and the rows drawn are judged as they stand.
func TestNoCharacterOfNoWidthHidesAnAddressInRelaysText(t *testing.T) {
	for i, sent := range hidingSent {
		content := TranscriptView(hidingState(sent), inertLinks(), t0, 100, 40).Content
		noAddressSkipped(t, strconv.QuoteToASCII(hidingInputs[i])+" as relay sent it", content)
	}
}

// The reviewer's case: Markdown takes the stars for emphasis, and the row
// drawn holds the selector and the keycap mark inside "://". The reply
// path draws no address there, before relay/inertText and after.
func TestMarkdownCannotMoveKeycapGlueIntoAnAddress(t *testing.T) {
	for name, c := range map[string]struct {
		text string
		live core.Links
	}{
		"relay-tui's own breaker": {"Read " + "https:*\ufe0f\u20e3//evil.test*" + " today.", core.Links{}},
		"relay's inert text":      {"Read https:*\ufe0f\u20e3//evil[.]test* today.", inertLinks()},
	} {
		st := core.State{Phase: core.Idle, Ready: true, Labels: core.Labels{Contaminated: true},
			Entries: []core.Entry{{Kind: core.Agent, Text: c.text, Final: true, Defang: true}}}
		content := TranscriptView(st, c.live, t0, 100, 30).Content
		noAddressSkipped(t, name, content)
		if !strings.Contains(skipped(ansi.Strip(content)), "evil[.]test") {
			t.Errorf("%s: the host is not drawn broken:\n%q", name, ansi.Strip(content))
		}
	}
}

// Words that hold no link keep their marks: only a word whose skipped
// form is an address is broken as that form.
func TestOrdinaryTextWithMarksIsLeftAsWritten(t *testing.T) {
	for _, s := range []string{
		"Cafe\u0301 cre\u0300me, de\u0301ja\u0300 vu.",
		"re\u0301sume\u0301.pdf and e\u0301.g. this",
		"\u0928\u092e\u0938\u094d\u0924\u0947 \u0926\u0941\u0928\u093f\u092f\u093e",
		"\u0e20\u0e32\u0e29\u0e32\u0e44\u0e17\u0e22",
		"\u05e9\u05c1\u05b8\u05dc\u05d5\u05b9\u05dd",
		"\u1112\u1161\u11ab\u1100\u1173\u11af",
		"Press 1\ufe0f\u20e3 or #\ufe0f\u20e3 now.",
	} {
		if got := Defang(s); got != s {
			t.Errorf("Defang(%q) = %q", s, got)
		}
		if linkable(s) {
			t.Errorf("%q is taken for a link", s)
		}
		st := core.State{Phase: core.Idle, Ready: true, Labels: core.Labels{Contaminated: true},
			Entries: []core.Entry{{Kind: core.Agent, Text: s, Final: true, Defang: true}}}
		if plain := ansi.Strip(TranscriptView(st, core.Links{}, t0, 100, 30).Content); !strings.Contains(plain, s) {
			t.Errorf("%q is drawn as\n%q", s, plain)
		}
	}
}

// The width table Alacritty uses gives no cell to more than marks: the
// spacing marks Unicode lists as Other_Grapheme_Extend, the characters
// that join the one after them, and U+A8FA. One after https: hides the
// address from a breaker that goes by letters as a mark does. A review
// of relay#130 found these.
var spacingInputs = []string{
	"https:\u0bbe//evil.com/steal?d=1",     // a Tamil vowel sign, a spacing mark of no cell
	"https:\u09be//evil.com/steal?d=1",     // a Bengali vowel sign
	"https:\u302e//evil.com/steal?d=1",     // a Hangul tone mark
	"https:\U0001d165//evil.com/steal?d=1", // a musical stem
	"https:\u0d4e//evil.com/steal?d=1",     // a Malayalam prefix letter
	"https:\U000111c2//evil.com/steal?d=1", // a Sharada prefix
	"https:\ua8fa//evil.com/steal?d=1",     // a Devanagari caret
}

func TestASpacingMarkOrAPrefixHidesNoAddress(t *testing.T) {
	for _, addr := range spacingInputs {
		said := "Read " + addr + " today."
		noticed := hubFixture()
		noticed.Notice = said
		for name, content := range map[string]string{
			"a reply, a thought and a call": TranscriptView(hidingState(said), core.Links{}, t0, 100, 40).Content,
			"the status line":               StatusText(core.StatusLine{Kind: core.StatusRunning, Detail: true, Tools: 1, Tool: said}, "", t0, 120),
			"a notice":                      HubView(noticed, "", "", t0, 120, 30).Content,
		} {
			noAddressSkipped(t, strconv.QuoteToASCII(addr)+" in "+name, content)
		}
		// relay's inert text keeps such a scheme with its host broken:
		// drawn as sent, the row is checked as it stands.
		sent := "Read " + strings.Replace(strings.Replace(addr, "https:", "https:", 1), ".com", "[.]com", 1) + " today."
		inert := core.Links{InertText: true}
		noAddressSkipped(t, strconv.QuoteToASCII(addr)+" as relay sends it", TranscriptView(hidingState(sent), inert, t0, 100, 40).Content)
	}
}
