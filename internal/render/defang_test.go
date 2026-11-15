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
		// Regression: a www. host with a scheme later in its word is
		// broken too, and so is a scheme that reads as a www. host.
		"www.evil.test/?u=http://x.test/a": "www[.]evil[.]test/?u=http[:]//x[.]test/a",
		"seewww.evil.test/xhttps://a.io":   "seewww[.]evil[.]test/xhttps[:]//a[.]io",
		"www.x-y.io://foo.bar":             "www[.]x-y[.]io[:]//foo[.]bar",
		// What is broken already stays as it is, and the tail a cut
		// leaves of a broken scheme is broken without doubling a bracket.
		"hxxps://evil[.]test/x?d=a[.]b": "hxxps://evil[.]test/x?d=a[.]b",
		"s://evil[.]test/x":             "s[:]//evil[.]test/x",
		"://evil[.]test/x":              "[:]//evil[.]test/x",
	} {
		if got := Defang(in); got != want {
			t.Errorf("Defang(%q) = %q, want %q", in, got, want)
		}
	}
}

// A link finder follows some schemes with a colon and no "//" after it
// (relay#126). A web scheme written short is broken as relay breaks it,
// and so is each scheme Ghostty's finder takes with no slashes. Every dot
// in such a word goes too.
func TestDefangBreaksASchemeWithNoDoubleSlash(t *testing.T) {
	for in, want := range map[string]string{
		// A web scheme with one slash, with none, and with backslashes.
		"https:/x.pl":               "hxxps:/x[.]pl",
		"http:/evil.test/x":         "hxxp:/evil[.]test/x",
		"https:evil.test":           "hxxps:evil[.]test",
		`https:\\evil.test`:         `hxxps:\\evil[.]test`,
		"ftp:/files.example/a.txt":  "fxp:/files[.]example/a[.]txt",
		"HTTPS:/Evil.test":          "hxxps:/Evil[.]test",
		"see (http:/evil.test/x).":  "see (hxxp:/evil[.]test/x).",
		"x=https:evil.test/?d=1.2,": "x=hxxps:evil[.]test/?d=1[.]2,",
		// The schemes a terminal's finder takes with no slashes at all.
		"ssh:CANARY@evil.test":       "ssh[:]CANARY@evil[.]test",
		"sftp:u@evil.test":           "sftp[:]u@evil[.]test",
		"file:/tmp/x.command":        "file[:]/tmp/x[.]command",
		"magnet:?xt=urn:btih:CANARY": "magnet[:]?xt=urn:btih:CANARY",
		"news:alt.CANARY":            "news[:]alt[.]CANARY",
		"git:evil.test/x.git":        "git[:]evil[.]test/x[.]git",
		"SSH:u@evil.test":            "SSH[:]u@evil[.]test",
		"ssh:ssh:u@evil.test":        "ssh[:]ssh[:]u@evil[.]test",
		"run `ssh:u@evil.test`.":     "run `ssh[:]u@evil[.]test`.",
		// A dot broken in such a word lays bare a scheme that ran on
		// from it, and that one is broken too.
		"hxxps:/x.ssh:u@evil.test": "hxxps:/x[.]ssh[:]u@evil[.]test",
		"ssh:u@a.file:/b.c":        "ssh[:]u@a[.]file[:]/b[.]c",
		// With "//" after the colon each keeps the rule it had.
		"ssh://evil.test/x": "ssh[:]//evil[.]test/x",
		"https://evil.test": "hxxps://evil[.]test",
	} {
		if got := Defang(in); got != want {
			t.Errorf("Defang(%q) = %q, want %q", in, got, want)
		}
		if got := Defang(Defang(in)); got != want {
			t.Errorf("Defang twice of %q = %q, want %q", in, got, want)
		}
	}
	// Left as they are: mailto, tel and sms, which wait for an answer
	// (relay#121); a name with nothing after its colon, or a space; a
	// longer name that only ends like one of these; a web scheme before
	// anything but a slash, a letter or a digit; and what relay broke
	// already.
	for _, same := range []string{
		"mailto:me@evil.test", "tel:+15551234567", "sms:+15551234567",
		"the file: here", "in the news: rain", "git:", "https: //x",
		"xssh:u@evil.test", "Makefile:12", "a1file:x", "https:?x.y",
		"hxxps:/x[.]pl", "fxp:x", "ssh[:]user@evil[.]test", "file[:]/tmp/x[.]command", "news[:]alt[.]CANARY",
	} {
		if got := Defang(same); got != same {
			t.Errorf("Defang(%q) = %q, want it unchanged", same, got)
		}
	}
}

// The check on rows as drawn finds the same schemes, by the function
// Defang breaks them with: what a wrap leaves at the start of a row is
// then drawn broken.
func TestTheRowCheckFindsASchemeWithNoDoubleSlash(t *testing.T) {
	for _, row := range []string{"ssh:u@evil.test", "see https:/x.pl", "https:evil.test", "file:/tmp/x", "(news:alt.x)", "git:x"} {
		if !linkable(row) {
			t.Errorf("%q is not taken for something to follow", row)
		}
	}
	for _, row := range []string{"ssh[:]u@evil[.]test", "hxxps:/x[.]pl", "mailto:me@evil.test", "the file: here", "xssh:u@evil.test", "ssh://evil", "09:30 news at ten"} {
		if strings.Contains(row, "://") {
			continue // that form is the other rule's
		}
		if linkable(row) {
			t.Errorf("%q is taken for something to follow", row)
		}
	}
	// After anything but an ASCII letter or digit a scheme is broken
	// where it stands, as relay breaks it.
	for in, want := range map[string]string{
		"a-git:x.y": "a-git[:]x[.]y", "my.file:x.y": "my[.]file[:]x[.]y", "done.ssh:root@evil.test": "done[.]ssh[:]root@evil[.]test",
		"-ssh:u@evil.test": "-ssh[:]u@evil[.]test", "e.g.file:/etc/passwd": "e[.]g[.]file[:]/etc/passwd",
		"hxxps:/x[.]ssh:u@evil[.]test": "hxxps:/x[.]ssh[:]u@evil[.]test",
		// relay escapes a hidden character after it breaks links, so a
		// scheme before one's escape is relay's own text, left as sent.
		`https:\u{0080}//evil[.]com/x`: `https:\u{0080}//evil[.]com/x`,
		`https:\u{200b}evil[.]com`:     `https:\u{200b}evil[.]com`,
	} {
		if got := Defang(in); got != want {
			t.Errorf("Defang(%q) = %q, want %q", in, got, want)
		}
	}
	// A cut in the middle of a word can lay a web scheme bare. The row it
	// starts is broken there, with every character kept, since the broken
	// name is as long as the name, and keeps its style. A slashless one is
	// left as the word was, which relay leaves too: it runs on from a
	// letter.
	for text, want := range map[string]string{
		"abcdefghijhttps:/x.pl and more": "abcdefghij|hxxps:/x.p|l",
		"abcdefghijssh:u@evil.test":      "abcdefghij|ssh:u@evil|.test",
	} {
		rows := wrap(styled(bold, text), 10)
		plain := strings.Join(plainRows(rows), "|")
		if !strings.Contains(plain, want) {
			t.Errorf("wrap(%q) = %q, want %q in it", text, plain, want)
		}
		for i, r := range rows {
			if Width(r) > 10 || !strings.Contains(r, sgr(bold)) {
				t.Errorf("wrap(%q): row %d is %q", text, i, r)
			}
		}
	}
	// No character is lost to a broken scheme at a row's start, at any
	// width: the reviewer's case drew "evi" nowhere.
	for _, text := range []string{"aaaaaaaa-ssh:u@evil.test/0123456789", "abcdefghijhttps:/evil.test/0123456789", "xxhttp:x.y and more"} {
		want := strings.Join(strings.Fields(fang(text, core.Links{})), "")
		for width := 4; width < 40; width++ {
			if got := strings.Join(plainRows(wrap(fang(text, core.Links{}), width)), ""); strings.ReplaceAll(strings.ReplaceAll(got, "hxxp", "http"), " ", "") != strings.ReplaceAll(want, "hxxp", "http") {
				t.Errorf("wrap(%q, %d) = %q, from %q", text, width, got, want)
			}
		}
	}
	// Text wrapped at a space is as it was written: a path of the
	// operator's own that starts a row is not a cut.
	if got := strings.Join(plainRows(wrap("see file:/home/x and git:main", 14)), "|"); got != "see|file:/home/x|and git:main" {
		t.Errorf("wrapped at spaces: %q", got)
	}
	// A longer name is no scheme, so its word is left whole, and a hard
	// wrap can then put ssh: at the start of a row. That row is drawn
	// broken.
	rows := plainRows(cardLines("abcdefgxssh:user@evil.test", true, core.Links{}, 8))
	for _, r := range rows {
		if followable(r) {
			t.Errorf("a wrapped row can be followed: %q in %q", r, rows)
		}
	}
	if joined := strings.Join(rows, ""); !strings.Contains(joined, "ssh[:]") {
		t.Errorf("the row that began with ssh: was not broken: %q", rows)
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
	if rapid.IntRange(0, 3).Draw(t, "colon") == 0 {
		// The same host after a scheme with no "//".
		short := rapid.SampledFrom([]string{"https:/", "http:", "HTTPS:", "ftp:/", `https:\\`, "ssh:u@", "sftp:", "file:/", "magnet:?", "news:", "git:", "mailto:", "xssh:"}).Draw(t, "short")
		url = short + host + path
	}
	around := rapid.SampledFrom([]string{"%s", "see %s now", "www.evil.test/?u=%s", "www.evil.io://%s", "**%s**", "_%s_", "[l](%s)", "<%s>", "`%s`", "```\n%s\n```", "- %s\n- %s", "a\x1b]8;;%s\x07click\x1b]8;;\x07", "h\x1bttp://%s", "http:*/*/%s", "https:/\u200b/%s"}).Draw(t, "around")
	return strings.ReplaceAll(around, "%s", url)
}

// Defang changes nothing in text it has already been through, so a row
// that is checked twice reads the same.
func TestDefangIsIdempotent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		once := Defang(sanitize(genURLText(t)))
		if twice := Defang(once); twice != once {
			t.Fatalf("Defang(%q) = %q", once, twice)
		}
	})
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
				out := strings.Join(entriesAs(nil, []core.Entry{e}, x, core.Links{}, width, t0), "\n")
				if strings.Contains(out, "\x1b]") || strings.ContainsRune(sgrRE.ReplaceAllString(out, ""), 0x1b) || strings.ContainsRune(out, 0x07) {
					t.Fatalf("kind %v: an escape other than SGR in %q", e.Kind, out)
				}
				plain := strings.ToLower(strings.ReplaceAll(ansi.Strip(out), "\n", ""))
				if defang && width >= 20 && (followable(plain) || strings.Contains(plain, "www.")) {
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
