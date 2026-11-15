package render

import (
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"pgregory.net/rapid"

	"github.com/kirahowe/relay-tui/internal/a2ui"
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
		// And is broken as any word is, whatever it is made of.
		"é://evil.test/x=" + linkA: "é[:]//evil[.]test/x=" + span(linkA),
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

// Every word goes through Defang. When only the words a second test
// took for links did, these got past it: a hidden character or a letter
// of another script before the "://", which a terminal's finder may read
// as part of a scheme, and a scheme broken already over a host that is
// not. What relay broke itself is left as it came, its fxp for ftp too.
func TestNoWordSkipsTheLinkBreaker(t *testing.T) {
	for in, want := range map[string]string{
		"https\u200d://evil.test/?d=1": "https\u200d[:]//evil.test/?d=1",
		"é://evil.test/x":              "é[:]//evil.test/x",
		"hxxps://evil.test/?d=1":       "hxxps://evil[.]test/?d=1",
		"fxp://files[.]example/a":      "fxp://files[.]example/a",
		"hxxps://x[.]example/a?b=c.d":  "hxxps://x[.]example/a?b=c[.]d",
		"see hxxp://x[.]example/a now": "see hxxp://x[.]example/a now",
		"[:]//evil[.]test":             "[:]//evil[.]test",
	} {
		if got := fang(in, core.Links{}); got != want {
			t.Errorf("fang(%q) = %q, want %q", in, got, want)
		}
		if got := fang(in, core.Links{}); linkable(got) {
			t.Errorf("fang(%q) = %q can be followed", in, got)
		}
	}
	// In a reply written under contamination, as in a card.
	said := "Read https\u200d://evil.test/?d=CANARY and é://evil.test/x today."
	st := core.State{Phase: core.Idle, Ready: true, Labels: core.Labels{Contaminated: true},
		Entries: []core.Entry{{Kind: core.Agent, Text: said, Final: true, Defang: true}}}
	c := cardChecklist
	c.Items = []core.CardElem{{ID: 1, CardText: core.CardText{Text: said, Outsider: true}}}
	// A card shows the hidden character as an escape, which leaves a
	// "://" for Defang to break. A reply has it taken out first, so the
	// scheme comes together and is broken as https is.
	for name, content := range map[string]string{
		"a reply": TranscriptView(st, core.Links{}, t0, 100, 30).Content,
		"a card":  View(boardState(true, c), "", "", t0, 140, 30).Content,
	} {
		plain := ansi.Strip(content)
		if followable(plain) || !strings.Contains(plain, "é[:]//evil") {
			t.Errorf("%s leaves a scheme whole:\n%s", name, plain)
		}
	}
	if plain := ansi.Strip(TranscriptView(st, core.Links{}, t0, 100, 30).Content); !strings.Contains(plain, "hxxps://evil[.]test/?d=CANARY") {
		t.Errorf("a reply does not break the address it joined:\n%s", plain)
	}
}

// relay lists the live links a card or a ready item shows, each with its
// broken form, and the text holds no address. The broken form is a
// hyperlink to the listed address where it stands as relay wrote it, and
// only when the address is in the connection's set too. Nothing is read
// out of the text: a broken link relay did not list is no hyperlink, live
// or not.
func TestALinkRelayListedIsAHyperlinkOverItsBrokenText(t *testing.T) {
	const other = "https://bit.example/3xQ9"
	text := "Arts fund: " + brokenA + ". Also (" + brokenOf(other) + ") and " + brokenA + "/more"
	own := []core.Link{{Address: linkA, Shown: brokenA}}
	card := cardChecklist
	card.Items = []core.CardElem{{ID: 1, CardText: core.CardText{Text: text, Outsider: true}}}
	note := cardNote
	note.Note = &core.CardText{Text: "## Grants\n- " + text, Outsider: true}
	item := core.ReadyItem{ID: "message:4", Kind: core.ReadyMessage, At: t0.Format(time.RFC3339), Title: "Two grants", Outsider: true,
		Lines: []string{text}, From: "from the web", Message: 4}

	// draw is every place that shows the text, given the things' own
	// links and the connection's set.
	draw := func(own []core.Link, live core.Links) map[string]string {
		c, n, it := card, note, item
		c.Links, n.Links, it.Links = own, own, own
		c.Listed, n.Listed, it.Listed = true, true, true
		out := map[string]string{}
		for name, x := range map[string]core.Card{"a card": c, "a note card": n} {
			s := boardState(true, x)
			s.App.Links = live
			out[name] = View(s, "", "", t0, 150, 30).Content
		}
		hub := hubStep(hubFixture(), core.ReadyLoaded{At: t0, Rev: 9, Items: []core.ReadyItem{it}}, core.StopTyping{At: t0}, core.ReadyOpen{At: t0})
		hub.App.Links = live
		out["a ready item"] = HubView(hub, "", "", t0, 150, 30).Content
		out["a ready item open"] = HubView(hubStep(hub, core.ReadyKey{At: t0, Action: core.ReadyOpenKey}), "", "", t0, 150, 30).Content
		return out
	}
	// shows says every word of the text is on screen as relay sent it.
	shows := func(content string) bool {
		plain := ansi.Strip(content)
		for _, w := range strings.Fields(text) {
			if !strings.Contains(plain, w) {
				return false
			}
		}
		return true
	}
	for name, content := range draw(own, core.LinksOf(linkA, other)) {
		if !shows(content) {
			t.Errorf("%s does not show the text as relay sent it:\n%s", name, ansi.Strip(content))
		}
		// One hyperlink, to the listed address, over exactly its broken
		// form: the first one, which ends its word but for a full stop.
		var over []string
		for _, row := range strings.Split(content, "\n") {
			for _, p := range pieces(row) {
				if p.target != "" {
					if p.target != linkA {
						t.Errorf("%s: a hyperlink to %q", name, p.target)
					}
					over = append(over, ansi.Strip(p.text))
				}
			}
		}
		if len(over) != 1 || over[0] != brokenA {
			t.Errorf("%s: hyperlinks over %q, want one over %q", name, over, brokenA)
		}
	}
	for what, live := range map[string]core.Links{
		"the address is not in the connection's set": core.LinksOf(other),
		"there is no set": {},
	} {
		for name, content := range draw(own, live) {
			if hasLinks(content) || !shows(content) {
				t.Errorf("%s, when %s: %q", name, what, content)
			}
		}
	}
	// In the set, and not listed for the thing: no hyperlink is made from
	// the text.
	for name, content := range draw(nil, core.LinksOf(linkA, other)) {
		if hasLinks(content) {
			t.Errorf("%s, with nothing listed, has a hyperlink: %q", name, content)
		}
	}
	// Listed with an address that is not the one its form shows: the
	// hyperlink goes where relay said, never to what the text reads as.
	// The address has to be in the set, so this one makes none.
	for name, content := range draw([]core.Link{{Address: "https://evil.test/x", Shown: brokenA}}, core.LinksOf(linkA)) {
		if hasLinks(content) {
			t.Errorf("%s: a hyperlink for an address outside the set: %q", name, content)
		}
	}
}

// Where relay sent a list, the list alone says what is a link: a whole
// address in the text is no hyperlink, in the set or not. A relay from
// before the list sends none, and only then is a whole address that is a
// member of the set taken from the text. Its text is broken either way.
func TestAListedThingTakesNoLinkFromItsText(t *testing.T) {
	live := core.LinksOf(linkA)
	text := "Arts fund: <" + linkA + "> and " + linkA + "."
	for name, c := range map[string]struct {
		listed bool
		spans  int
	}{"with a list, empty": {true, 0}, "with no list": {false, 2}} {
		card := cardChecklist
		card.Items = []core.CardElem{{ID: 1, CardText: core.CardText{Text: text, Outsider: true}}}
		card.Listed = c.listed
		s := boardState(true, card)
		s.App.Links = live
		content := View(s, "", "", t0, 150, 30).Content
		if got := strings.Count(content, linkOpen(linkA)); got != c.spans {
			t.Errorf("%s: %d hyperlinks, want %d", name, got, c.spans)
		}
		if plain := ansi.Strip(content); strings.Contains(plain, linkA) || !strings.Contains(plain, brokenA) {
			t.Errorf("%s: the address is not shown broken:\n%s", name, plain)
		}
	}
}

// fang by itself, before gate has its say: a listed link whose address is
// not in the connection's set gets no span, and a shown form that was not
// broken is broken where it is written.
func TestFangHoldsAListedLinkToTheSetAndBreaksItsText(t *testing.T) {
	own := []core.Link{{Address: linkA, Shown: brokenA}}
	text := "see " + brokenA + " now"
	if got := fang(text, core.LinksOf(linkB).Of(own, true)); got != text {
		t.Errorf("not in the set: %q", got)
	}
	if got := fang(text, core.Links{}.Of(own, true)); got != text {
		t.Errorf("with no set: %q", got)
	}
	if got, want := fang(text, core.LinksOf(linkA).Of(own, true)), "see "+linkOpen(linkA)+brokenA+linkEnd+" now"; got != want {
		t.Errorf("in the set: %q, want %q", got, want)
	}
	// A list that names a form with its dots whole.
	loose := []core.Link{{Address: linkA, Shown: "hxxps://evil.test/www.x.test"}}
	got := fang("see hxxps://evil.test/www.x.test now", core.LinksOf(linkA).Of(loose, true))
	if want := "see " + linkOpen(linkA) + "hxxps://evil[.]test/www[.]x[.]test" + linkEnd + " now"; got != want {
		t.Errorf("a form that was not broken: %q, want %q", got, want)
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

// relaySends says relay could have sent text as outside text
// (relay.text/defang's result): in every word that holds a link's mark, a
// "://", a "[:]" or www., each "://" has a broken scheme before it and
// nothing else, and no dot is left with a character either side of it but
// the dot of a [.].
func relaySends(text string) bool {
	for _, w := range strings.Fields(text) {
		lower := strings.ToLower(w)
		if !strings.Contains(w, "://") && !strings.Contains(w, "[:]") && !strings.Contains(lower, "www.") {
			continue
		}
		for rest := lower; ; {
			i := strings.Index(rest, "://")
			if i < 0 {
				break
			}
			// The scheme is the run of a scheme's characters before the
			// "://", from its first letter.
			j := i
			for j > 0 && strings.ContainsRune("abcdefghijklmnopqrstuvwxyz0123456789+.-", rune(rest[j-1])) {
				j--
			}
			scheme := strings.TrimLeft(rest[j:i], "0123456789+.-")
			if scheme != "hxxp" && scheme != "hxxps" && scheme != "fxp" {
				return false
			}
			rest = rest[i+3:]
		}
		for i := 1; i < len(w)-1; i++ {
			if w[i] == '.' && w[i-1] != '[' && w[i+1] != ']' {
				return false
			}
		}
	}
	return true
}

// In a session that has read text from outside, a character that draws
// as nothing is taken out of what the agent writes before its links are
// broken. Left in, it sits inside "://" or "www." where Defang, which
// goes by the letters, does not find the link, and a terminal's finder
// may read straight past it. A session that has read nothing from
// outside keeps such characters: they join emoji.
func TestACharacterNobodySeesCannotSitInsideAnAddress(t *testing.T) {
	hidden := []string{
		"https:\u200d//evil.test/?d=CANARY", "https:/\ufe0f/evil.test/?d=CANARY", "www\u200d.evil.test/?d=CANARY",
		"ww\u034fw.evil.test", "https\u180b://evil.test", "h\U000E0100ttps://evil.test", "http\u17b5s:\u1160/\u2060/evil.test",
		// A code point with no character assigned, and a noncharacter.
		"https:\u0378//evil.test/?d=CANARY", "www\uffff.evil.test/?d=CANARY",
	}
	for _, addr := range hidden {
		said := "Read " + addr + " today."
		st := core.State{Phase: core.Idle, Ready: true, Labels: core.Labels{Contaminated: true}, Entries: []core.Entry{
			{Kind: core.Agent, Text: said, Final: true, Defang: true},
			{Kind: core.Thought, Text: said, Final: true, Defang: true},
			{Kind: core.Tool, Calls: []core.Call{{ID: "1", Name: "web_get", Title: said, Input: `{"url":"` + addr + `"}`, Output: said, Status: core.OK}}, Final: true, Defang: true},
		}}
		st.Transcript.Tools, st.Transcript.Thinking = true, true
		noticed := hubFixture()
		noticed.Notice = said
		for name, content := range map[string]string{
			"a reply, a thought and a call": TranscriptView(st, core.Links{}, t0, 100, 40).Content,
			"the status line":               StatusText(core.StatusLine{Kind: core.StatusRunning, Detail: true, Tools: 1, Tool: said}, "", t0, 120),
			"a notice":                      HubView(noticed, "", "", t0, 120, 30).Content,
		} {
			plain := ansi.Strip(content)
			if bare(plain) != plain || strings.ContainsAny(plain, "\u0378\uffff") {
				t.Errorf("%q: %s still holds a character nobody sees:\n%q", addr, name, plain)
			}
			if followable(plain) || !strings.Contains(plain, "evil[.]test") {
				t.Errorf("%q: %s does not show the address broken:\n%s", addr, name, plain)
			}
		}
	}
	// The order is what matters: the hidden character is taken out first,
	// so Defang is given the word a terminal would read, joined, and
	// breaks it as it breaks any address. Here one sits inside the scheme
	// and one inside the host.
	joined := core.State{Phase: core.Idle, Ready: true, Labels: core.Labels{Contaminated: true},
		Entries: []core.Entry{{Kind: core.Agent, Text: "Read h\u200dttps://ev\u200bil.test/x today.", Final: true, Defang: true}}}
	if plain := ansi.Strip(TranscriptView(joined, core.Links{}, t0, 100, 30).Content); !strings.Contains(plain, "Read hxxps://evil[.]test/x today.") {
		t.Errorf("the joined word is not broken as one address:\n%q", plain)
	}
	// What holds an emoji together stays, by relay's own rule, in a
	// session that has read from outside too.
	glued := core.State{Phase: core.Idle, Ready: true, Labels: core.Labels{Contaminated: true},
		Entries: []core.Entry{{Kind: core.Agent, Text: "Done \U0001F469\u200d\U0001F4BB and \u26a0\ufe0f", Final: true, Defang: true}}}
	if plain := ansi.Strip(TranscriptView(glued, core.Links{}, t0, 100, 30).Content); !strings.Contains(plain, "\U0001F469\u200d\U0001F4BB") || !strings.Contains(plain, "\u26a0\ufe0f") {
		t.Errorf("an emoji came apart in a contaminated session:\n%q", plain)
	}
	// With nothing read from outside the text is the agent's own, and an
	// emoji keeps the joiner that makes it one.
	clean := core.State{Phase: core.Idle, Ready: true, Entries: []core.Entry{{Kind: core.Agent, Text: "Done 👩\u200d💻 and ⚠\ufe0f", Final: true}}}
	if plain := ansi.Strip(TranscriptView(clean, core.Links{}, t0, 100, 30).Content); !strings.Contains(plain, "👩\u200d💻") {
		t.Errorf("a clean session's emoji came apart:\n%q", plain)
	}
}

// The check on rows as drawn takes www. for a host when more of its word
// follows than closing marks: a name, or a user name before one. The full
// stop after a word that is www is none.
func TestTheRowCheckTellsAHostFromAFullStop(t *testing.T) {
	for _, row := range []string{"see www.", "(see www.)", "is it www.?", "the end of [www.]", `"www."`, "www.**", "hxxps://x[.]example/www.", `Type www.\nthen the name`} {
		if linkable(row) {
			t.Errorf("%q is taken for a host", row)
		}
	}
	for _, row := range []string{"www.evil.test", "WWW.Evil.test", "www._x.evil.test", "www.-x", "www.@evil.test/?d=1", "www.%65vil.test", "www.[evil", "www.é"} {
		if !linkable(row) {
			t.Errorf("%q is not taken for a host", row)
		}
	}
	// A line break in a one-line text is written as a backslash and an n.
	// relay's "Type www." before one is drawn as it came, in a card's
	// title and in a checklist item.
	sent := "Type www.\nthen the name"
	c := cardChecklist
	c.Title = core.CardText{Text: sent, Outsider: true}
	c.Items = []core.CardElem{{ID: 1, CardText: core.CardText{Text: sent, Outsider: true}}}
	if plain := ansi.Strip(View(boardState(true, c), "", "", t0, 150, 30).Content); strings.Count(plain, `Type www.\nthen the name`) != 2 {
		t.Errorf("a card draws:\n%s", plain)
	}
	// Markdown can put one together from a link's text and what follows
	// it. The row is then drawn plain, and broken.
	said := `Look: [www.](www. "t")@evil.test/?d=1 now`
	st := core.State{Phase: core.Idle, Ready: true, Labels: core.Labels{Contaminated: true},
		Entries: []core.Entry{{Kind: core.Agent, Text: said, Final: true, Defang: true}}}
	if plain := ansi.Strip(TranscriptView(st, core.Links{}, t0, 100, 30).Content); strings.Contains(plain, "www.@") || !strings.Contains(plain, "www[.]@evil") {
		t.Errorf("a reply draws:\n%s", plain)
	}
}

// Text relay broke is drawn as it came, down to the full stop after a
// link: relay breaks a dot only with a character either side of it, and
// so does Defang.
func TestTextRelayBrokeIsDrawnAsItCame(t *testing.T) {
	for _, sent := range []string{
		"Apply at hxxps://grants[.]example[.]org/fund. Next",
		"see hxxps://evil[.]test/x.",
		"(hxxps://evil[.]test/x).",
		"hxxps://evil[.]test/x[.][.].",
		"hxxps://evil[.]test/a[.])",
		"see www[.]evil[.]test.",
		"fxp://files[.]example/a.",
		"s3[:]//bucket/key[.]txt.",
		".hidden hxxp://x[.]example. .end",
		"see [hxxps://evil[.]test/a.] now",
	} {
		if !relaySends(sent) {
			t.Fatalf("%q is not text relay sends", sent)
		}
		if got := fang(sent, core.Links{}); got != sent {
			t.Errorf("fang(%q) = %q", sent, got)
		}
		if got := strings.Join(plainRows(cardLines(sent, true, core.Links{}, 120)), "\n"); got != sent {
			t.Errorf("a card draws %q as %q", sent, got)
		}
	}
	// The same links as an outsider wrote them come out as relay would
	// have sent them.
	for in, want := range map[string]string{
		"Apply at https://grants.example.org/fund. Next": "Apply at hxxps://grants[.]example[.]org/fund. Next",
		"https://evil.test/x...":                         "hxxps://evil[.]test/x[.][.].",
		"https://evil.test/a.)":                          "hxxps://evil[.]test/a[.])",
		"see www.evil.test.":                             "see www[.]evil[.]test.",
	} {
		if got := fang(in, core.Links{}); got != want {
			t.Errorf("fang(%q) = %q, want %q", in, got, want)
		}
	}
	// What stands before a live link in its word keeps a first dot, as
	// relay's text does.
	live := core.LinksOf(linkA)
	if got := fang(".x.y/"+linkA, live); got != ".x[.]y/"+span(linkA) {
		t.Errorf("before a live link: %q", got)
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
		"2) two", "word", "a.b", "x_y_z", "`code`", "~~no~~", "[l](p)", "<tag>", "hxxps://evil[.]test/x", "fxp://files[.]example/a", "é", "世界", "  ", "\n", "\n\n", "\t",
		"*", "**", "#", ">", "www", "ht", "tp", ":", "//", "."})
	solid := func(s string) string { return strings.Join(strings.Fields(s), "") }
	rapid.Check(t, func(t *rapid.T) {
		text := strings.Join(rapid.SliceOfN(piece, 1, 12).Draw(t, "pieces"), rapid.SampledFrom([]string{" ", "", "\n"}).Draw(t, "sep"))
		// Only text relay could send. relaySends is a model of that,
		// written without the code under test. Asking Defang whether it
		// would change the text, as this did for a day, skipped every
		// text Defang changed that it should not have: a full stop after
		// a broken link went by.
		if !relaySends(core.EscapeNonPrinting(text)) {
			t.Skip("holds a link relay would have broken")
		}
		width := rapid.IntRange(8, 80).Draw(t, "width")
		want := solid(core.EscapeNonPrinting(strings.ReplaceAll(text, "\t", "    ")))
		// A surface of an outsider with the text as a body, a caption and a
		// label, each drawn alone.
		one := func(kind string, props map[string]any) []string {
			s := a2ui.Surface{ID: "s", CatalogID: a2ui.RelayCatalogID,
				Components: map[string]a2ui.Component{"root": {ID: "root", Type: kind, Props: props}}}
			rows, _, _ := surfaceRows(s, true, core.Links{}, width+2, nil)
			return rows
		}
		for name, rows := range map[string][]string{
			"a note":            noteLines(text, true, core.Links{}, width),
			"card text":         cardLines(text, true, core.Links{}, width),
			"a check item":      checkLines(core.CardElem{CardText: core.CardText{Text: strings.ReplaceAll(text, "\n", " "), Outsider: true}}, core.Links{}, width+2),
			"a surface text":    one("Text", map[string]any{"text": text}),
			"a surface caption": one("Text", map[string]any{"text": text, "variant": "caption"}),
			"a surface check":   one("CheckBox", map[string]any{"label": strings.ReplaceAll(text, "\n", " "), "value": false}),
		} {
			got := solid(strings.Join(plainRows(rows), "\n"))
			if strings.HasSuffix(name, "check item") || name == "a surface check" {
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
				// The broken form, worked out here and not by the code
				// under test: hxxp for http, and every dot in brackets.
				want := "hxxp" + strings.ReplaceAll(link[len("http"):], ".", "[.]")
				if i := strings.Index(want, "://"); i >= 0 {
					// A second scheme inside the address loses its "://".
					want = want[:i+3] + strings.ReplaceAll(want[i+3:], "://", "[:]//")
				}
				if joined := strings.ReplaceAll(plain, "\n", ""); !strings.Contains(joined, want) {
					t.Errorf("%q at %d is drawn as %q, want %q in it", text, width, plain, want)
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
	kept := list
	kept.Ready.Message, kept.Ready.Failure = core.UndoRefusedNote, "These stayed: https://evil.test/b.pdf is in the way."
	noticed := hubFixture()
	noticed.Notice = "No role called https://evil.test/r is here."
	// The same notice over the list of roles and over the pane, each
	// with a narrow screen to itself.
	listed := hubStep(hubFixture(), core.StopTyping{At: t0}, core.Move{At: t0, Delta: -1})
	listed.Notice = noticed.Notice
	paned := hubFixture()
	paned.Focus, paned.App.PaneFocus, paned.Notice = "", true, noticed.Notice
	for name, got := range map[string]string{
		"an undo":                 HubView(undone, "", "", t0, 120, 30).Content,
		"an undo refused":         HubView(kept, "", "", t0, 120, 30).Content,
		"an undo refused, narrow": HubView(kept, "", "", t0, 50, 30).Content,
		"a notice":                HubView(noticed, "", "", t0, 120, 30).Content,
		"a notice over the list":  HubView(listed, "", "", t0, 60, 30).Content,
		"a notice over the pane":  HubView(paned, "", "", t0, 60, 30).Content,
		"a tool":                  StatusText(core.StatusLine{Kind: core.StatusRunning, Detail: true, Tools: 1, Tool: "web_get https://evil.test/?d=1"}, "", t0, 120),
		"a retry":                 StatusText(core.StatusLine{Kind: core.StatusRetrying, RetryIn: time.Second, Reason: "https://evil.test/ said no"}, "", t0, 120),
		"a message":               StatusText(core.StatusLine{Kind: core.StatusIdle, Message: "see www.evil.test"}, "", t0, 120),
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
		"a\x1b]8;;https://evil.test\x07click\x1b]8;;\x07", "<https://evil.test/x>", "ww**w.**evil.test",
		"https\u200d://evil.test/?d=1", "é://evil.test/x", "https\u2060://evil.test", "x\u200d://evil.test", "hxxps://evil.test/?d=1",
		"fxp://files[.]example/a", "ftp\u200d://evil.test",
		// A character nobody sees inside the separator, the scheme or www.
		"https:\u200d//evil.test/?d=1", "https:/\ufe0f/evil.test", "www\u200d.evil.test", "ww\u034fw.evil.test",
		"https\u180b://evil.test", "h\U000E0100ttps://evil.test/x", "http\u17b5s:\u1160/\u2060/evil.test", "www.\u200bevil.test"}
	around = []string{"%s", "<%s>", "(%s).", "**%s**", "**<%s>**", "[a fund](%s)", "[a fund](<%s>)", "see:%s", "x.y/%s",
		"%s/more", "%s...", "_%s_", "`%s`", "# %s", "- %s", "1. %s", "%s?", "%s%s", "https://evil.test/?u=%s", "\"%s\"", "> %s"}
)

// brokenOf is an address as relay shows it broken, worked out here and
// not by the code under test: hxxp for http and every dot in brackets.
func brokenOf(link string) string {
	return "hxxp" + strings.ReplaceAll(strings.TrimPrefix(link, "http"), ".", "[.]")
}

// genOwn is the links relay might list for one thing: some of the
// addresses that may be live, one that never is, and one whose shown form
// is another link's.
func genOwn(t *rapid.T, label string) []core.Link {
	var own []core.Link
	for _, l := range goodLinks {
		if rapid.Bool().Draw(t, label+"Own") {
			own = append(own, core.Link{Address: l, Shown: brokenOf(l)})
		}
	}
	if rapid.Bool().Draw(t, label+"Stray") {
		own = append(own, core.Link{Address: "https://evil.test/x?d=1", Shown: brokenOf(linkA)},
			core.Link{Address: "https://evil.test/listed", Shown: "hxxps://evil[.]test/listed"})
	}
	return own
}

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
		switch rapid.IntRange(0, 6).Draw(t, label+"Kind") {
		case 6:
			// A link as relay shows one, broken, which the thing's own
			// list may or may not name.
			p = strings.ReplaceAll(rapid.SampledFrom(around).Draw(t, label+"Around"), "%s", brokenOf(rapid.SampledFrom(goodLinks).Draw(t, label+"Shown")))
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
// inside the scheme of a broken link, or straight after it: the end of
// hxxp, hxxps or fxp, or nothing. None is a scheme anything answers to,
// and the address after it has its dots broken, so no terminal follows
// it.
var brokenTails = map[string]bool{"xxp": true, "xxps": true, "xp": true, "xps": true, "p": true, "ps": true, "s": true, "": true}

// followable says a row's text holds something a terminal's own link
// finder would take. One is a www. host: www. and then more of the word,
// not the full stop after a word that is www. The other is a "://"
// after anything but a broken scheme (hxxp, hxxps, fxp, with or without
// text run on before it) or the tail a cut left of one. What stands
// before the "://" is read back to the last space, whatever it is made
// of: a finder's idea of a scheme's letters is wider than ASCII's, and a
// hidden character can sit there.
func followable(plain string) bool {
	rest := plain
	for {
		i := strings.Index(rest, "://")
		if i < 0 {
			break
		}
		run := rest[:i]
		if j := strings.LastIndexFunc(run, unicode.IsSpace); j >= 0 {
			run = run[j:]
			_, n := utf8.DecodeRuneInString(run)
			run = run[n:]
		}
		run = strings.ToLower(run)
		if !(strings.HasSuffix(run, "hxxp") || strings.HasSuffix(run, "hxxps") || strings.HasSuffix(run, "fxp") || brokenTails[run]) {
			return true
		}
		rest = rest[i+3:]
	}
	// A www. with more of its word after it than closing marks.
	lower := strings.ToLower(plain)
	for i := strings.Index(lower, "www."); i >= 0; {
		after, n := utf8.DecodeRuneInString(lower[i+4:])
		if n > 0 && !unicode.IsSpace(after) && !strings.ContainsRune(`)>".,;!]':?*~\`, after) {
			return true
		}
		j := strings.Index(lower[i+4:], "www.")
		if j < 0 {
			break
		}
		i += 4 + j
	}
	return false
}

// bare is s without the characters that draw as nothing, by Go's own
// Unicode tables and not the list the code under test keeps: the format
// characters, the variation selectors and the other default-ignorable
// code points.
func bare(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Variation_Selector, r) || unicode.Is(unicode.Other_Default_Ignorable_Code_Point, r) {
			return -1
		}
		return r
	}, s)
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
		// all, and as one that reads past what draws as nothing.
		plain := ansi.Strip(row)
		if followable(plain) {
			t.Fatalf("%s: %q can be followed as text", what, plain)
		}
		if b := bare(plain); b != plain && followable(b) {
			t.Fatalf("%s: %q can be followed as text once what nobody sees is read past", what, plain)
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
		Labels: core.Labels{Contaminated: true}, Links: genOwn(t, "card"), Listed: rapid.Bool().Draw(t, "cardListed")}
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
			From: genOutside(t, "from"), Message: 4, ReplyTo: 4, Links: genOwn(t, "item"), Listed: rapid.Bool().Draw(t, "itemListed"),
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
		// A refusal with no reply open has rows of its own.
		kept := list
		kept.Ready.Message, kept.Ready.Failure = core.UndoRefusedNote, genOutside(t, "kept")
		for _, hub := range []core.Hub{list, open, replying, refused, undone, kept} {
			hub.App.Links = live
			checkFollowed(t, "the ready view", HubView(hub, "", "", t0, w, h).Content, live)
		}

		// The roster: a role's title is what the role that added it wrote.
		titled := hubStep(hubFixture(), core.RosterUpdate{At: t0, Roles: []core.Role{
			{ID: "dev", Title: genOutside(t, "title1")}, {ID: "mail", Title: genOutside(t, "title2")}}})
		titled.App.Links = live
		// A notice at the foot is relay's reason for something. It is set
		// last, since a key clears it: beside a conversation, over the
		// list of roles, and over the pane when that has a narrow screen
		// to itself.
		listed := hubStep(titled, core.StopTyping{At: t0}, core.Move{At: t0, Delta: -1})
		paned := titled
		paned.Focus, paned.List, paned.App.PaneFocus = "", false, true
		for _, hub := range []core.Hub{titled, listed, paned} {
			hub.Notice = genOutside(t, "notice")
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
