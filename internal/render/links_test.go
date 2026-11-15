package render

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"pgregory.net/rapid"

	"github.com/kirahowe/relay-tui/internal/core"
)

const (
	linkA = "https://artsfund.example/grants/2027"
	linkB = "https://artscouncil.example/apply/spring-round?year=2027&ref=newsletter"
	// linkA as a row shows it.
	brokenA = "hxxps://artsfund[.]example/grants/2027"
)

func TestFang(t *testing.T) {
	live := core.LinksOf(linkA)
	for in, want := range map[string]string{
		"see " + linkA + " now":   "see " + span(linkA) + " now",
		"see <" + linkA + "> now": "see " + span(linkA) + " now",
		"(" + linkA + ").":        "(" + span(linkA) + ").",
		"**<" + linkA + ">**":     "**" + span(linkA) + "**",
		// What stands before a live link in its word has its dots broken.
		"x.y/" + linkA: "x[.]y/" + span(linkA),
		// Text run on after a live link makes another address.
		linkA + "/more":         "hxxps://artsfund[.]example/grants/2027/more",
		"https://evil.test/x":   "hxxps://evil[.]test/x",
		"<https://evil.test/x>": "<hxxps://evil[.]test/x>",
		"www.evil.test":         "www[.]evil[.]test",
		// What relay broke already is left as it is.
		"hxxps://evil[.]test/x": "hxxps://evil[.]test/x",
		"no links here. Really": "no links here. Really",
	} {
		if got := fang(in, live); got != want {
			t.Errorf("fang(%q) = %q, want %q", in, got, want)
		}
	}
	if got := fang("see "+linkA, core.Links{}); got != "see hxxps://artsfund[.]example/grants/2027" {
		t.Errorf("with no live links: %q", got)
	}
}

// plainRows is rows as they read on screen.
func plainRows(rows []string) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = ansi.Strip(r)
	}
	return out
}

// spansOn is the targets of the hyperlinks on a row, and whether each is
// both started and ended there.
func spansOn(t *testing.T, row string) []string {
	t.Helper()
	var targets []string
	open := ""
	for _, m := range osc8RE.FindAllStringSubmatch(row, -1) {
		switch {
		case m[1] != "" && open != "":
			t.Fatalf("a link starts inside another on %q", row)
		case m[1] != "":
			open = m[1]
			targets = append(targets, m[1])
		case open == "":
			t.Fatalf("a link ends that never started on %q", row)
		default:
			open = ""
		}
	}
	if open != "" {
		t.Fatalf("a link is left open at the end of %q", row)
	}
	return targets
}

// A live link is wrapped as any other text is: whole on a row where it
// fits, cut where the row ends where it does not. Its text is broken
// either way, and each row's piece of it opens the whole address.
func TestALiveLinkIsWrappedAsAnyOtherTextIs(t *testing.T) {
	rows := wrap("Arts fund spring round, due 1 Dec: "+span(linkA)+" and", 38)
	want := []string{"Arts fund spring round, due 1 Dec:", brokenA, "and"}
	if got := plainRows(rows); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("rows %q, want %q", got, want)
	}
	if got := spansOn(t, rows[1]); len(got) != 1 || got[0] != linkA {
		t.Fatalf("the link's row links to %q", got)
	}

	rows = wrap("Council form: "+span(linkB), 38)
	got := plainRows(rows)
	want = []string{"Council form:", "hxxps://artscouncil[.]example/apply/sp", "ring-round?year=2027&ref=newsletter"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("rows %q, want %q", got, want)
	}
	for _, r := range rows[1:] {
		if s := spansOn(t, r); len(s) != 1 || s[0] != linkB {
			t.Fatalf("row %q links to %q", r, s)
		}
	}
	for _, r := range got {
		if linkableRow(r) {
			t.Fatalf("row %q can be followed as text", r)
		}
	}
}

// A cut can fall anywhere in a live link. What is left of it still opens
// the whole address, and reads as no address.
func TestACutLiveLinkStillOpensTheWholeAddress(t *testing.T) {
	s := "see " + span(linkA) + " and more"
	for width := 5; width < len("see "+brokenA+" and more"); width++ {
		cut := truncate(s, width)
		plain := ansi.Strip(cut)
		if want := ("see " + brokenA + " and more")[:width-1] + "…"; plain != want {
			t.Fatalf("cut to %d: %q, want %q", width, plain, want)
		}
		if linkableRow(plain) {
			t.Fatalf("cut to %d: %q can be followed as text", width, plain)
		}
		if s := spansOn(t, cut); len(s) != 1 || s[0] != linkA {
			t.Fatalf("cut to %d: the link goes to %q", width, s)
		}
	}
}

func TestGateDressesALiveLink(t *testing.T) {
	live := core.LinksOf(linkA)
	out := gate("see "+span(linkA)+" now", live)
	want := "see " + linkOpen(linkA) + sgr(linkStyle(0)) + brokenA + sgr(0) + linkEnd + " now"
	if out != want {
		t.Fatalf("gate gave %q, want %q", out, want)
	}
	// Inside styled text the link keeps the style's attributes, takes
	// its own colour, and hands the style back after it.
	st := bold | warnColour
	out = gate(styled(st, "see "+span(linkA)+" now"), live)
	want = sgr(st) + "see " + linkOpen(linkA) + sgr(linkStyle(st)) + brokenA + sgr(st) + linkEnd + " now" + sgr(0)
	if out != want {
		t.Fatalf("gate gave %q, want %q", out, want)
	}
	// Text with no link span passes untouched.
	if plain := styled(bold, "nothing here"); gate(plain, live) != plain {
		t.Fatal("gate changed text with no link in it")
	}
}

func TestGateLetsOnlyALiveLinkThrough(t *testing.T) {
	live := core.LinksOf(linkA)
	evil := "https://evil.test/x?d=1"
	for name, in := range map[string]string{
		"a target that is not live":       "see " + span(evil) + " now",
		"a target rewritten on the way":   "see " + strings.ToUpper(span(linkA)) + " now",
		"a target with more run on":       "see " + linkOpen(linkA+"/more") + "<" + linkA + ">" + linkEnd,
		"a target out of a link's shape":  "see " + linkOpen("javascript:alert(1)") + "click" + linkEnd,
		"a live target over its address":  "see " + linkOpen(linkA) + linkA + linkEnd,
		"a live target over an autolink":  "see " + linkOpen(linkA) + "<" + linkA + ">" + linkEnd,
		"a live target over other text":   "see " + linkOpen(linkA) + "<" + evil + ">" + linkEnd,
		"a live target over a cut of it":  "see " + linkOpen(linkA) + "<https://artsfund.exa" + linkEnd,
		"a live target over a www. host":  "see " + linkOpen(linkA) + "www.evil.test" + linkEnd,
		"a link span inside another's":    "see " + linkOpen(evil) + span(linkA) + linkEnd,
		"a link started and never ended":  "see " + linkOpen(evil) + "<" + evil + ">",
		"a link over styled outside text": "see " + linkOpen(evil) + styled(bold, "<"+evil+">") + linkEnd,
	} {
		out := gate(in, live)
		for _, m := range osc8RE.FindAllStringSubmatch(out, -1) {
			if m[1] != "" && m[1] != linkA {
				t.Errorf("%s: a hyperlink to %q got through: %q", name, m[1], out)
			}
		}
		if plain := ansi.Strip(out); linkable(plain) {
			t.Errorf("%s: %q can be followed as text", name, plain)
		}
		if Width(out) > Width(in) {
			t.Errorf("%s: the row grew from %d to %d cells", name, Width(in), Width(out))
		}
	}
}

// A hard wrap can fall inside a broken link's scheme, and the next row
// then starts with "://host[.]tld" or "s://host[.]tld". Nothing can
// follow that, so the row stays as relay sent it. The same letters
// anywhere else on a row are a scheme, and are broken.
func TestARowThatStartsInsideABrokenSchemeStaysAsSent(t *testing.T) {
	live := core.LinksOf(linkB)
	for _, tail := range []string{"://evil[.]test/x", "s://evil[.]test/x", "ps://evil[.]test/x", "  xxps://evil[.]test/x", "p://evil[.]test"} {
		rows := drawn([]string{"hxxp", tail}, true)
		if rows[1] != tail {
			t.Errorf("drawn changed %q to %q", tail, rows[1])
		}
	}
	for _, real := range []string{"see s://evil.test/x", "x ps://evil.test", "https://evil.test", "shttp://evil.test", "see www.evil.test"} {
		rows := drawn([]string{real}, true)
		if got := ansi.Strip(rows[0]); linkable(got) || got == real {
			t.Errorf("drawn left %q as %q", real, got)
		}
	}
	// The same tail as a piece of a live link shown broken: the piece is
	// left alone and keeps its target.
	piece := linkOpen(linkB) + "://artscouncil[.]example/apply/s" + linkEnd
	out := gate(piece, live)
	if s := spansOn(t, out); len(s) != 1 || s[0] != linkB {
		t.Fatalf("the piece links to %q", s)
	}
	if plain := ansi.Strip(out); plain != "://artscouncil[.]example/apply/s" {
		t.Fatalf("gate changed the piece to %q", plain)
	}
}

// Outside text is drawn exactly as relay sent it: no character is taken
// away and none is changed, apart from the line breaks wrapping adds. So
// what relay checked is what the screen shows. The marks a note of the
// operator's own is dressed by stay in an outsider's.
func TestOutsideTextIsDrawnAsItWasSent(t *testing.T) {
	// Text with every mark the note's dressing reads, and nothing a link
	// could be made of that relay would not have broken already.
	piece := rapid.SampledFrom([]string{"ev**il.**example/steal?d=1", "**bold**", "# Heading", "## Two", "- item", "* star", "1. one",
		"2) two", "word", "a.b", "x_y_z", "`code`", "~~no~~", "[l](p)", "<tag>", "hxxps://evil[.]test/x", "é", "世界", "  ", "\n", "\n\n", "\t",
		"*", "**", "#", ">", "www", "ht", "tp", ":", "//", "."})
	solid := func(s string) string { return strings.Join(strings.Fields(s), "") }
	rapid.Check(t, func(t *rapid.T) {
		text := strings.Join(rapid.SliceOfN(piece, 1, 12).Draw(t, "pieces"), rapid.SampledFrom([]string{" ", "", "\n"}).Draw(t, "sep"))
		// Only text relay could send: nothing in it can be followed.
		if linkable(text) {
			t.Skip("holds a link relay would have broken")
		}
		width := rapid.IntRange(8, 80).Draw(t, "width")
		want := solid(core.EscapeNonPrinting(strings.ReplaceAll(text, "\t", "    ")))
		for name, rows := range map[string][]string{
			"a note":       noteLines(text, true, core.Links{}, width),
			"card text":    cardLines(text, true, core.Links{}, width),
			"a check item": checkLines(core.CardElem{CardText: core.CardText{Text: strings.ReplaceAll(text, "\n", " "), Outsider: true}}, core.Links{}, width+2),
		} {
			got := solid(strings.Join(plainRows(rows), "\n"))
			if name == "a check item" {
				// Its ring and its text on one line.
				got = strings.TrimPrefix(got, "○")
				if got != solid(core.EscapeNonPrinting(strings.ReplaceAll(strings.ReplaceAll(text, "\n", " "), "\t", "    "))) {
					t.Fatalf("%s at %d drew %q for %q", name, width, got, text)
				}
				continue
			}
			if got != want {
				t.Fatalf("%s at %d drew %q, want %q (from %q)", name, width, got, want, text)
			}
		}
	})
}

// The same note from the operator is dressed, and from an outsider it is
// not: the bold marks stay, so the row cannot read as an address.
func TestAnOutsidersNoteKeepsItsMarks(t *testing.T) {
	text := "# Grants\n- see ev**il.**example/steal?d=1"
	if got := strings.Join(plainRows(noteLines(text, false, core.Links{}, 60)), "|"); got != "Grants|- see evil.example/steal?d=1" {
		t.Fatalf("the operator's own note drew %q", got)
	}
	got := strings.Join(plainRows(noteLines(text, true, core.Links{}, 60)), "|")
	if got != "# Grants|- see ev**il.**example/steal?d=1" {
		t.Fatalf("an outsider's note drew %q", got)
	}
}

// A live link's address is never on screen as it is, whatever it holds
// and however it was written. Link finders disagree on where an address
// ends: Ghostty's stops at an apostrophe, kitty's follows a second address
// inside the first, WezTerm's reads past a closing bracket. So none is
// given an address to read. The text is the broken form and the hyperlink
// carries the address.
func TestALiveLinkIsNeverShownAsItIs(t *testing.T) {
	for _, link := range []string{
		linkA, linkB, "http://y.example", "https://x.example:8443/a_b/c-d.html?q=1&r=%20+x#top", "https://x.example/~me/",
		"https://example.org/o'brien", "https://x.example/a*b", "https://x.example/a,b", "https://x.example/a;b",
		"https://x.example/a!b", "https://x.example/a$b", "https://x.example/[1]", "https://x.example/a=",
		"https://x.example/a.", "https://x.example/a:", "https://x.example/a?", "https://x.example/a-", "https://x.example/a_",
		"https://x.example/?u=https://y.example/b", "https://x.example/www.y.example",
	} {
		live := core.LinksOf(link)
		if !live.Has(link) {
			t.Fatalf("%q is not a link a set can hold", link)
		}
		for _, text := range []string{"See " + link + " now", "See <" + link + "> now", "See <" + link + ">> now", "See (" + link + ")."} {
			for _, width := range []int{80, 24, 9} {
				out := gate(strings.Join(wrap(fang(text, live), width), "\n"), live)
				for _, target := range spansOn(t, strings.ReplaceAll(out, "\n", "")) {
					if target != link {
						t.Errorf("%q at %d: a hyperlink goes to %q", text, width, target)
					}
				}
				if !strings.Contains(out, linkOpen(link)) {
					t.Errorf("%q at %d: no hyperlink to the link in %q", text, width, out)
				}
				plain := ansi.Strip(out)
				for _, row := range strings.Split(plain, "\n") {
					if linkableRow(row) {
						t.Errorf("%q at %d: row %q can be followed as text", text, width, row)
					}
				}
				if joined := strings.ReplaceAll(plain, "\n", ""); !strings.Contains(joined, Defang(link)) {
					t.Errorf("%q at %d is drawn as %q", text, width, plain)
				}
			}
		}
		// Whatever writes such a link as it is, gate breaks its text.
		for _, text := range []string{link, "<" + link + ">"} {
			out := gate("See "+linkOpen(link)+text+linkEnd, live)
			if plain := ansi.Strip(out); linkable(plain) {
				t.Errorf("gate left %q", plain)
			}
		}
	}
}

// A role that may add roles names them, and it may have read text from
// outside first. relay sends the title as it was written, so the roster
// breaks every link in one, live or not.
func TestARolesTitleHasItsLinksBroken(t *testing.T) {
	evil := "https://evil.test/?d=CANARY"
	for _, live := range []core.Links{{}, core.LinksOf(evil)} {
		h := hubStep(hubFixture(), core.RosterUpdate{At: t0, Roles: []core.Role{{ID: "dev", Title: evil}, {ID: "helper", Title: "see www.evil.test"}}})
		h.App.Links = live
		for _, hub := range []core.Hub{h, hubStep(h, core.StopTyping{At: t0}, core.Move{At: t0, Delta: -1})} {
			for _, width := range []int{60, 100, 160} {
				content := HubView(hub, "", "", t0, width, 30).Content
				plain := ansi.Strip(content)
				if linkable(plain) || hasLinks(content) {
					t.Fatalf("at %d a role's title can be followed:\n%s", width, plain)
				}
				// A narrow screen shows the role in focus alone.
				if !strings.Contains(plain, "hxxps://evil[.]") || (width > 60 && !strings.Contains(plain, "www[.]evil")) {
					t.Fatalf("at %d the titles are not shown broken:\n%s", width, plain)
				}
			}
		}
	}
}

// Where a ready item came from, and why relay refused something, are
// relay's words about text from outside: a page's name, a mail's sender.
// Every link in them is broken.
func TestRelaysOwnWordsAboutOutsideTextHaveTheirLinksBroken(t *testing.T) {
	it := core.ReadyItem{ID: "message:4", Kind: core.ReadyMessage, At: t0.Format(time.RFC3339), Title: "Two grants", Outsider: true,
		Lines: []string{"A line."}, From: "Drew on: the web page www.evil.test/?d=1 and https://evil.test/x", Message: 4, ReplyTo: 4}
	list := hubStep(hubFixture(), core.ReadyLoaded{At: t0, Rev: 9, Items: []core.ReadyItem{it}}, core.StopTyping{At: t0}, core.ReadyOpen{At: t0})
	refused := hubStep(list, core.ReadyKey{At: t0, Action: core.ReadyReplyKey})
	refused.Ready.Message, refused.Ready.Failure = core.ReplyRefusedNote, "No reply to http://evil.test/y is possible."
	for name, hub := range map[string]core.Hub{"the list": list, "an item open": hubStep(list, core.ReadyKey{At: t0, Action: core.ReadyOpenKey}), "a refusal": refused} {
		for _, width := range []int{50, 100, 160} {
			content := HubView(hub, "", "", t0, width, 30).Content
			if plain := ansi.Strip(content); linkable(plain) || hasLinks(content) {
				t.Fatalf("%s at %d can be followed:\n%s", name, width, plain)
			}
		}
	}
	plain := ansi.Strip(HubView(refused, "", "", t0, 160, 30).Content)
	for _, want := range []string{"www[.]evil[.]test", "hxxps://evil[.]test/x", "hxxp://evil[.]test/y"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("%q is not on screen:\n%s", want, plain)
		}
	}

	// What an undo did, a notice at the foot, and the status line's
	// words for a running tool and for a retry.
	undone := list
	undone.Ready.Message = "Put back https://evil.test/a.pdf"
	noticed := hubFixture()
	noticed.Notice = "No role called https://evil.test/r is here."
	for name, got := range map[string]string{
		"an undo":   HubView(undone, "", "", t0, 120, 30).Content,
		"a notice":  HubView(noticed, "", "", t0, 120, 30).Content,
		"a tool":    StatusText(core.StatusLine{Kind: core.StatusRunning, Detail: true, Tools: 1, Tool: "web_get https://evil.test/?d=1"}, "", t0, 120),
		"a retry":   StatusText(core.StatusLine{Kind: core.StatusRetrying, RetryIn: time.Second, Reason: "https://evil.test/ said no"}, "", t0, 120),
		"a message": StatusText(core.StatusLine{Kind: core.StatusIdle, Message: "see www.evil.test"}, "", t0, 120),
	} {
		plain := ansi.Strip(got)
		if linkable(plain) || hasLinks(got) {
			t.Fatalf("%s can be followed:\n%s", name, plain)
		}
		if !strings.Contains(plain, "evil[.]test") {
			t.Fatalf("%s does not show its link broken:\n%s", name, plain)
		}
	}
}

// An outsider's column name is drawn as it was sent. The operator's own
// is in capitals.
func TestAnOutsidersColumnNameKeepsItsCase(t *testing.T) {
	c := cardTracker
	c.Columns = []core.CardElem{{ID: 1, CardText: core.CardText{Text: "aB12", Outsider: true}}, {ID: 2, CardText: ct("Jul")}}
	s := detail(boardState(true, c), c.ID, 1, 0)
	plain := ansi.Strip(strings.Join(PaneLines(s.App, s.State, t0, 60, 20), "\n"))
	if !strings.Contains(plain, "aB12") || strings.Contains(plain, "AB12") {
		t.Fatalf("the outsider's column name changed case:\n%s", plain)
	}
	if !strings.Contains(plain, "JUL") {
		t.Fatalf("the operator's own column is not in capitals:\n%s", plain)
	}
}

// --- the property: whatever outside text relay or an agent sends, and
// whatever relay's live links are, nothing drawn can be followed but a
// live link.

var (
	// Addresses in a live link's shape, any of which may be live.
	goodLinks = []string{linkA, linkB, "https://x.example/a-b-c", "http://y.example", "https://x.example/a.",
		"https://example.org/o'brien", "https://x.example/a,b;c!d$e", "https://x.example/q?a=",
		"https://z.example/p?q=1&r=*a*&s=_b_&t=**c**", "https://x.example/" + strings.Repeat("long-", 30)}
	// Addresses that are never live: out of shape, or not in the set.
	badLinks = []string{"https://evil.test/x?d=1", "HTTPS://evil.test/x", "https://evil.test/a(b)", "ftp://evil.test/f",
		"www.evil.test/x", "ssh://evil.test", "https://x.example/A-B-C", "https://user@evil.test/", "evil.test/x"}
	// Ways text puts an address together from pieces, or hides one.
	tricks = []string{"https:/**/**evil.test/x", "ht**tp**s://evil.test/x", "http:*/*/evil.test", "https:/\u200b/evil.test",
		"**https:**//evil.test", "# https:/**/**evil.test", "- https:/**/**evil.test/x", "h\x1bttp://evil.test",
		"a\x1b]8;;https://evil.test\x07click\x1b]8;;\x07", "<https://evil.test/x>", "ww**w.**evil.test"}
	around = []string{"%s", "<%s>", "(%s).", "**%s**", "**<%s>**", "[a fund](%s)", "[a fund](<%s>)", "see:%s", "x.y/%s",
		"%s/more", "%s...", "_%s_", "`%s`", "# %s", "- %s", "1. %s", "%s?", "%s%s", "https://evil.test/?u=%s", "\"%s\"", "> %s"}
)

func genLive(t *rapid.T) core.Links {
	var set []string
	for _, l := range goodLinks {
		if rapid.Bool().Draw(t, "live") {
			set = append(set, l)
		}
	}
	return core.LinksOf(set...)
}

// genOutside is text an outsider might send: words, addresses that may be
// live and ones that never are, each in some wrapping, and tricks.
func genOutside(t *rapid.T, label string) string {
	var parts []string
	for range rapid.IntRange(1, 6).Draw(t, label+"N") {
		var p string
		switch rapid.IntRange(0, 5).Draw(t, label+"Kind") {
		case 0:
			p = rapid.SampledFrom([]string{"see", "the grant page", "due 1 Dec", "Council form:", "now"}).Draw(t, label+"Word")
		case 1, 2:
			p = strings.ReplaceAll(rapid.SampledFrom(around).Draw(t, label+"Around"), "%s", rapid.SampledFrom(goodLinks).Draw(t, label+"Good"))
		case 3:
			p = strings.ReplaceAll(rapid.SampledFrom(around).Draw(t, label+"Around"), "%s", rapid.SampledFrom(badLinks).Draw(t, label+"Bad"))
		case 4:
			p = rapid.SampledFrom(tricks).Draw(t, label+"Trick")
		case 5:
			p = rapid.SampledFrom(nasty).Draw(t, label+"Nasty")
		}
		parts = append(parts, p)
	}
	return strings.Join(parts, rapid.SampledFrom([]string{" ", " ", "\n", ""}).Draw(t, label+"Sep"))
}

// brokenTails is what a row can start with when a wrap or a cut fell
// inside the scheme of a broken link: the end of hxxp or hxxps. None is a
// scheme anything answers to, and the address after it has its dots
// broken, so no terminal follows it.
var brokenTails = map[string]bool{"xxp": true, "xxps": true, "xp": true, "xps": true, "p": true, "ps": true, "s": true, "": true}

var schemeBefore = regexp.MustCompile(`([A-Za-z0-9+.\-]*)://`)

// followable says a row's text holds something a terminal's own link
// finder would take: a "://" after anything but a broken scheme (hxxp,
// hxxps, with or without text run on before it) or the tail a cut left
// of one, or a www. host.
func followable(plain string) bool {
	for _, m := range schemeBefore.FindAllStringSubmatch(plain, -1) {
		run := strings.ToLower(m[1])
		if strings.HasSuffix(run, "hxxp") || strings.HasSuffix(run, "hxxps") || brokenTails[run] {
			continue
		}
		return true
	}
	return strings.Contains(strings.ToLower(plain), "www.")
}

// checkFollowed fails when a drawn frame gives a terminal anything to
// follow that is not a live link: a hyperlink to any other target, or
// text a terminal's own link finder would take, in a hyperlink or out of
// one. It also fails on any escape that is neither a style nor a
// hyperlink.
func checkFollowed(t *rapid.T, what, content string, live core.Links) {
	t.Helper()
	for _, m := range osc8RE.FindAllStringSubmatch(content, -1) {
		if m[1] != "" && !live.Has(m[1]) {
			t.Fatalf("%s: a hyperlink to %q, which is not a live link, in\n%q", what, m[1], content)
		}
	}
	if rest := osc8RE.ReplaceAllString(sgrRE.ReplaceAllString(content, ""), ""); strings.ContainsAny(rest, "\x1b\x07") {
		t.Fatalf("%s: an escape that is neither a style nor a hyperlink in\n%q", what, content)
	}
	for _, row := range strings.Split(content, "\n") {
		open := false
		for _, m := range osc8RE.FindAllStringSubmatch(row, -1) {
			open = m[1] != ""
		}
		if open {
			t.Fatalf("%s: a hyperlink is left open at the end of row %q", what, row)
		}
		// The row as a terminal with no OSC 8 reads it, hyperlinks and
		// all.
		if plain := ansi.Strip(row); followable(plain) {
			t.Fatalf("%s: %q can be followed as text", what, plain)
		}
	}
}

// genLink is an address a set can hold, or text close to one: the
// shape's own characters, with the pieces that make text read as another
// address mixed in.
func genLink(t *rapid.T) string {
	link := rapid.SampledFrom([]string{"http://", "https://"}).Draw(t, "scheme") +
		rapid.StringMatching(`[a-z0-9]([a-z0-9.\-]{0,12}[a-z0-9])?(:[0-9]{1,4})?`).Draw(t, "host")
	bits := rapid.SliceOfN(rapid.SampledFrom([]string{"a", "Z", "9", "-", ".", "_", "~", ":", "/", "?", "#", "[", "]", "@", "!", "$", "&", "'",
		"*", "+", ",", ";", "=", "%20", "www.", "WWW.", "http://", "https://", "hxxps://", "://", "evil.test", "[.]", "ftp://", "long-long-"}), 0, 20).Draw(t, "rest")
	if len(bits) > 0 {
		link += rapid.SampledFrom([]string{"/", "?", "#"}).Draw(t, "after") + strings.Join(bits, "")
	}
	return link
}

// Whatever a live link's address holds, however it is written into
// outside text and whatever the width, its rows read as no address, and
// the only hyperlink is the one to it.
func TestAnyLiveLinkIsInertTextAtAnyWidth(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		link := genLink(t)
		if !core.LinkShape(link) {
			t.Skip("not a link a set can hold")
		}
		live := core.LinksOf(link)
		text := strings.ReplaceAll(rapid.SampledFrom([]string{"%s", "see %s now", "<%s>", "see <%s>> now", "(%s).", "x.y/%s", "**%s**", "see:%s", "\"%s\""}).Draw(t, "around"), "%s", link)
		width := rapid.IntRange(1, 120).Draw(t, "width")
		for name, rows := range map[string][]string{
			"wrapped":   wrap(fang(text, live), width),
			"cut":       {truncate(fang(text, live), width)},
			"card text": cardLines(text, true, live, width),
		} {
			out := gate(strings.Join(rows, "\n"), live)
			checkFollowed(t, name, out, live)
			if strings.Contains(ansi.Strip(out), link) {
				t.Fatalf("%s: the address is on screen as it is:\n%q", name, out)
			}
			if name == "wrapped" && !strings.Contains(out, linkOpen(link)) {
				t.Fatalf("%s: no hyperlink to the link in\n%q", name, out)
			}
		}
	})
}

func genOutsideCard(t *rapid.T, id int64) core.Card {
	text := func(label string) core.CardText {
		return core.CardText{Text: genOutside(t, label), Outsider: true, Rev: 1}
	}
	c := core.Card{ID: id, Status: core.CardActive, Rev: 2, Title: text("title"), Text: genOutside(t, "text"),
		Kind:   rapid.SampledFrom([]string{core.CardChecklist, core.CardTracker, core.CardNote, "kanban"}).Draw(t, "kind"),
		Labels: core.Labels{Contaminated: true}}
	elems := func(label string) []core.CardElem {
		var out []core.CardElem
		for i := range rapid.IntRange(1, 3).Draw(t, label) {
			out = append(out, core.CardElem{ID: i + 1, CardText: text(label + "Text"), Done: rapid.Bool().Draw(t, label+"Done")})
		}
		return out
	}
	switch c.Kind {
	case core.CardChecklist:
		c.Items = elems("items")
	case core.CardTracker:
		c.Rows, c.Columns = elems("rows"), elems("columns")
	case core.CardNote:
		n := text("note")
		c.Note = &n
	}
	return c
}

func TestNothingFromOutsideIsFollowedButALiveLink(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		live := genLive(t)
		w := rapid.IntRange(20, 140).Draw(t, "width")
		h := rapid.IntRange(6, 50).Draw(t, "height")

		// The board: in the pane's list, and one card open.
		card := genOutsideCard(t, 7)
		for _, s := range []core.Single{boardState(true, card), detail(boardState(true, card), card.ID, rapid.IntRange(0, 3).Draw(t, "line"), 0)} {
			s.App.Links = live
			if rapid.Bool().Draw(t, "question") {
				s.Board.Confirm, s.Board.ConfirmRev = card.ID, card.Rev
			}
			checkFollowed(t, "the board", View(s, "", "", t0, w, h).Content, live)
		}

		// The ready view: the list, and an item open whole.
		item := core.ReadyItem{ID: "message:4", Kind: rapid.SampledFrom([]string{core.ReadyMessage, core.ReadyMove, core.ReadyFinding, "other"}).Draw(t, "kind"),
			At: t0.Add(-time.Minute).Format(time.RFC3339), Title: genOutside(t, "readyTitle"), Outsider: true,
			From: genOutside(t, "from"), Message: 4, ReplyTo: 4,
			Lines:   []string{genOutside(t, "line1"), genOutside(t, "line2")},
			Replies: []core.ReadyReply{{At: t0.Format(time.RFC3339), Text: "ok"}}}
		if rapid.Bool().Draw(t, "sections") {
			item.Sections = []core.ReadySection{{Heading: genOutside(t, "heading"), Lines: item.Lines}}
		}
		list := hubStep(hubFixture(), core.ReadyLoaded{At: t0, Rev: 9, Items: []core.ReadyItem{item}}, core.StopTyping{At: t0}, core.ReadyOpen{At: t0})
		open := hubStep(list, core.ReadyKey{At: t0, Action: core.ReadyOpenKey})
		replying := hubStep(list, core.ReadyKey{At: t0, Action: core.ReadyReplyKey})
		// relay's own words can quote text from outside: where an item
		// came from above, and here why a reply was refused.
		refused := replying
		refused.Ready.Message, refused.Ready.Failure = core.ReplyRefusedNote, genOutside(t, "failure")
		// What an undo did is relay's line too, and it names files.
		undone := list
		undone.Ready.Message = genOutside(t, "undone")
		for _, hub := range []core.Hub{list, open, replying, refused, undone} {
			hub.App.Links = live
			checkFollowed(t, "the ready view", HubView(hub, "", "", t0, w, h).Content, live)
		}

		// The roster: a role's title is what the role that added it wrote.
		titled := hubStep(hubFixture(), core.RosterUpdate{At: t0, Roles: []core.Role{
			{ID: "dev", Title: genOutside(t, "title1")}, {ID: "mail", Title: genOutside(t, "title2")}}})
		titled.App.Links = live
		// A notice at the foot is relay's reason for something.
		titled.Notice = genOutside(t, "notice")
		for _, hub := range []core.Hub{titled, hubStep(titled, core.StopTyping{At: t0}, core.Move{At: t0, Delta: -1})} {
			checkFollowed(t, "the roster", HubView(hub, "", "", t0, w, h).Content, live)
		}

		// The status line names the tool that is running, by the title
		// the agent gave the call, and says why a turn is being retried.
		for _, l := range []core.StatusLine{
			{Kind: core.StatusRunning, Detail: true, Tools: 1, Tool: genOutside(t, "tool")},
			{Kind: core.StatusRetrying, RetryIn: time.Second, Reason: genOutside(t, "reason")},
			{Kind: core.StatusIdle, Message: genOutside(t, "message")},
		} {
			checkFollowed(t, "the status line", StatusText(l, "", t0, w), live)
		}

		// A conversation in a contaminated session, every kind of entry
		// the agent writes: inline, in the hub, and in the transcript.
		text := genOutside(t, "said")
		st := core.State{Phase: core.Idle, Ready: true, Labels: core.Labels{Contaminated: true}, Entries: []core.Entry{
			{Kind: core.Agent, Text: text, Final: true, Defang: true},
			{Kind: core.Note, Text: text, Detail: text, Final: true, Defang: true},
			{Kind: core.Thought, Text: text, Final: true, Defang: true},
			{Kind: core.Tool, Calls: []core.Call{{ID: "1", Name: "web_fetch", Title: text, Input: `{"url":"` + text + `"}`, Output: text, Status: core.OK}}, Final: true, Defang: true},
		}}
		st.Transcript.Tools, st.Transcript.Thinking = rapid.Bool().Draw(t, "tools"), rapid.Bool().Draw(t, "thinking")
		f := View(core.Single{App: core.App{Live: true, Links: live, Board: core.Board{Items: true}}, State: st}, "", "", t0, w, h)
		checkFollowed(t, "the conversation", f.Content+"\n"+strings.Join(f.Print, "\n"), live)
		hub := hubOf(st)
		hub.App.Links = live
		checkFollowed(t, "the conversation in the hub", HubView(hub, "", "", t0, w, h).Content, live)
		checkFollowed(t, "the transcript", TranscriptView(st, live, t0, w, h).Content, live)
	})
}

// With a live link in the set, each place that draws outside text shows
// it broken, as a hyperlink to exactly that address, beside a link that
// is not live and is no hyperlink.
func TestALiveLinkIsAHyperlinkWhereOutsideTextIsDrawn(t *testing.T) {
	live := core.LinksOf(linkA)
	evil := "hxxps://evil[.]test/x"
	text := "Arts fund: <" + linkA + "> and " + evil
	shows := func(t *testing.T, content string) {
		t.Helper()
		plain := ansi.Strip(content)
		if !strings.Contains(plain, brokenA) || strings.Contains(plain, linkA) {
			t.Fatalf("the live link is not shown broken:\n%s", plain)
		}
		if !strings.Contains(content, linkOpen(linkA)) {
			t.Fatalf("no hyperlink to the live link:\n%q", content)
		}
		for _, m := range osc8RE.FindAllStringSubmatch(content, -1) {
			if m[1] != "" && m[1] != linkA {
				t.Fatalf("a hyperlink to %q:\n%q", m[1], content)
			}
		}
		if !strings.Contains(plain, evil) {
			t.Fatalf("the other link is not shown broken:\n%s", plain)
		}
	}
	t.Run("a card", func(t *testing.T) {
		c := cardChecklist
		c.Items = []core.CardElem{{ID: 1, CardText: core.CardText{Text: text, Outsider: true}}}
		s := boardState(true, c)
		s.App.Links = live
		shows(t, View(s, "", "", t0, 140, 30).Content)
	})
	t.Run("a note card", func(t *testing.T) {
		c := cardNote
		c.Note = &core.CardText{Text: "## Grants\n- **Arts fund:** <" + linkA + "> and " + evil, Outsider: true}
		s := boardState(true, c)
		s.App.Links = live
		shows(t, View(s, "", "", t0, 140, 30).Content)
	})
	t.Run("a ready item", func(t *testing.T) {
		it := core.ReadyItem{ID: "message:4", Kind: core.ReadyMessage, At: t0.Format(time.RFC3339), Title: "Two grants", Outsider: true,
			Lines: []string{text}, From: "from the web", Message: 4}
		hub := hubStep(hubFixture(), core.ReadyLoaded{At: t0, Rev: 9, Items: []core.ReadyItem{it}}, core.StopTyping{At: t0}, core.ReadyOpen{At: t0})
		hub.App.Links = live
		shows(t, HubView(hub, "", "", t0, 100, 30).Content)
		shows(t, HubView(hubStep(hub, core.ReadyKey{At: t0, Action: core.ReadyOpenKey}), "", "", t0, 100, 30).Content)
	})
	t.Run("a reply in a contaminated session", func(t *testing.T) {
		for _, said := range []string{
			"Arts fund: " + linkA + " and https://evil.test/x",
			"Arts fund: <" + linkA + "> and https://evil.test/x",
			"Arts **fund**: [the page](" + linkA + ") and https://evil.test/x",
		} {
			st := core.State{Phase: core.Idle, Ready: true, Labels: core.Labels{Contaminated: true},
				Entries: []core.Entry{{Kind: core.Agent, Text: said, Final: true, Defang: true}}}
			hub := hubOf(st)
			hub.App.Links = live
			shows(t, HubView(hub, "", "", t0, 100, 30).Content)
			shows(t, TranscriptView(st, live, t0, 100, 30).Content)
		}
	})
	// The same text with no live links is all broken, as before.
	t.Run("with no live links", func(t *testing.T) {
		c := cardChecklist
		c.Items = []core.CardElem{{ID: 1, CardText: core.CardText{Text: text, Outsider: true}}}
		got := View(boardState(true, c), "", "", t0, 140, 30).Content
		if hasLinks(got) || linkable(ansi.Strip(got)) {
			t.Fatalf("a link got through with no live links:\n%q", got)
		}
	})
}
