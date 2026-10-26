package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"pgregory.net/rapid"

	"github.com/kirahowe/relay-tui/internal/core"
)

func TestDefangKnown(t *testing.T) {
	for in, want := range map[string]string{
		"see https://evil.com/x?d=secret.1 now": "see hxxps://evil[.]com/x?d=secret[.]1 now",
		// Regression: a second host in the query stays broken too.
		"https://a.test/?next=www.evil.test/?d=SECRET": "hxxps://a[.]test/?next=www[.]evil[.]test/?d=SECRET",
		"HTTP://a.b.c":           "hxxp://a[.]b[.]c",
		"ftp://h.example/f":      "ftp[:]//h[.]example/f",
		"www.example.com/a":      "www[.]example[.]com/a",
		"[x](https://a.io/p)":    "[x](hxxps://a[.]io/p)",
		"no links here. Really.": "no links here. Really.",
	} {
		if got := Defang(in); got != want {
			t.Errorf("Defang(%q) = %q, want %q", in, got, want)
		}
	}
}

func genURLText(t *rapid.T) string {
	host := rapid.SampledFrom([]string{"evil.com", "a.b.c.example", "localhost", "x-y.io", "192.168.0.1:8080"}).Draw(t, "host")
	scheme := rapid.SampledFrom([]string{"http", "https", "HTTPS", "ftp", "ssh", "x-y+z", ""}).Draw(t, "scheme")
	path := rapid.SampledFrom([]string{"", "/", "/a/b?d=1&e=2#f", "/_x_/*y*", "/(p)"}).Draw(t, "path")
	url := host + path
	if scheme != "" {
		url = scheme + "://" + url
	} else {
		url = "www." + url
	}
	around := rapid.SampledFrom([]string{"%s", "see %s now", "**%s**", "_%s_", "[l](%s)", "<%s>", "`%s`", "```\n%s\n```", "- %s\n- %s", "a\x1b]8;;%s\x07click\x1b]8;;\x07", "h\x1bttp://%s", "http:*/*/%s", "https:/\u200b/%s"}).Draw(t, "around")
	return strings.ReplaceAll(around, "%s", url)
}

// P-defang1: in a contaminated session no rendered agent text keeps a
// "://" or a www. host, and none carries an OSC 8 hyperlink; the text is
// otherwise still shown.
func TestDefangedEntries(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		text := genURLText(t)
		for _, e := range []core.Entry{
			{Kind: core.Agent, Text: text, Final: true},
			{Kind: core.Note, Text: text, Detail: text, Final: true},
			{Kind: core.Tool, Calls: []core.Call{{Name: "web_fetch", Title: text, Input: `{"url":"` + text + `"}`, Status: core.OK}}, Final: true},
			{Kind: core.Tool, Calls: []core.Call{{Name: "web_fetch", Title: text, Input: `{"url":"` + text + `"}`, Output: text, Status: core.OK}}, Final: true},
			{Kind: core.Thought, Text: text, Final: true},
		} {
			width := rapid.IntRange(1, 90).Draw(t, "width")
			x := expand{tools: rapid.Bool().Draw(t, "tools"), thinking: rapid.Bool().Draw(t, "thinking")}
			for _, defang := range []bool{true, false} {
				e.Defang = defang
				out := strings.Join(entriesAs(nil, []core.Entry{e}, x, width, t0), "\n")
				if strings.Contains(out, "\x1b]") || strings.ContainsRune(sgrRE.ReplaceAllString(out, ""), 0x1b) || strings.ContainsRune(out, 0x07) {
					t.Fatalf("kind %v: an escape other than SGR in %q", e.Kind, out)
				}
				plain := strings.ToLower(strings.ReplaceAll(ansi.Strip(out), "\n", ""))
				if defang && width >= 20 && (live(plain) || strings.Contains(plain, "www.")) {
					t.Fatalf("kind %v width %d: a live URL in %q (from %q)", e.Kind, width, plain, text)
				}
			}
		}
	})
}

// The same text in a session that is not contaminated is left alone, so
// the operator can still read and copy a link.
func TestNotDefangedWhenClean(t *testing.T) {
	out := strings.Join(Entries(nil, []core.Entry{{Kind: core.Agent, Text: "see https://example.com/a", Final: true}}, 60, t0), "\n")
	if !strings.Contains(ansi.Strip(out), "https://example.com/a") {
		t.Fatalf("%q", out)
	}
	e := core.Entry{Kind: core.Agent, Text: "see https://example.com/a", Final: true, Defang: true}
	out = strings.Join(Entries(nil, []core.Entry{e}, 60, t0), "\n")
	if !strings.Contains(ansi.Strip(out), "see hxxps://example[.]com/a") {
		t.Fatalf("%q", out)
	}
}

// Through the reducer: text and tool output written after the session
// turns contaminated is defanged; text from before is not.
func TestContaminatedStampsEntries(t *testing.T) {
	s, _ := core.Reduce(core.State{}, core.Connected{At: t0})
	s, _ = core.Reduce(s, core.Submit{At: t0, Text: "go"})
	s, _ = core.Reduce(s, core.TextChunk{At: t0, MessageID: "a", Text: "before https://ok.example/1\n\n"})
	s, _ = core.Reduce(s, core.ToolStarted{At: t0, ID: "c", Name: "web_fetch", Status: core.Running})
	s, _ = core.Reduce(s, core.LabelsSeen{At: t0, Labels: core.Labels{Contaminated: true}})
	out := "done"
	st := core.OK
	s, _ = core.Reduce(s, core.ToolUpdated{At: t0, ID: "c", Status: &st, Output: &out})
	s, _ = core.Reduce(s, core.TextChunk{At: t0, MessageID: "b", Text: "after https://evil.example/2 "})
	var lines []string
	for _, e := range s.Entries {
		lines = append(lines, Entries(nil, []core.Entry{e}, 80, t0)...)
	}
	all := ansi.Strip(strings.Join(lines, "\n"))
	if !strings.Contains(all, "https://ok.example/1") || !strings.Contains(all, "hxxps://evil[.]example/2") || strings.Contains(all, "https://evil") {
		t.Fatalf("\n%s", all)
	}
}

func isPrintOrSpace(r rune) bool {
	return r == ' ' || (r > ' ' && r != 0x7f && !(r >= 0x80 && r < 0xa0) && strings.ContainsRune(string(r), r) && isPrint(r))
}

func isPrint(r rune) bool { return core.EscapeNonPrinting(string(r)) == string(r) }
