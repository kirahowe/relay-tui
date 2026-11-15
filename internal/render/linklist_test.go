package render

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"pgregory.net/rapid"

	"github.com/kirahowe/relay-tui/internal/core"
)

const (
	linkC       = "https://bit.example/3xQ9"
	linkCBroken = "hxxps://bit[.]example/3xQ9"
	linkDBroken = "hxxps://wiki[.]example/wiki/Fund_(arts)"
	// linkB as a row shows it.
	brokenB = "hxxps://artscouncil[.]example/apply/spring-round?year=2027&ref=newsletter"
)

// The three links as relay lists them once each is live.
var (
	ownA = core.Link{Address: linkA, Shown: brokenA}
	ownB = core.Link{Address: linkB, Shown: brokenB}
	ownC = core.Link{Address: linkC, Shown: linkCBroken}
)

// liveRow is a listed link's row in the list when it is live.
func liveRow(l core.Link) core.ShownLink {
	return core.ShownLink{Text: l.Shown, Address: l.Address, Live: true}
}

// grantsItem is a message a scout's finding led to, as relay sends it:
// every link broken in its text, the two the web showed the scout listed
// as live, and one from a forum post that relay cannot place. kept is
// what relay lists besides once the operator has kept it.
func grantsItem(third string, kept ...core.Link) core.ReadyItem {
	return core.ReadyItem{ID: "message:4", Kind: core.ReadyMessage, At: t0.Add(-time.Minute).Format(time.RFC3339),
		Title: "Two grants worth a look", Outsider: true, From: "Drew on: the web page grants-2027.html", Message: 4,
		Lines: []string{"Arts fund spring round, due 1 Dec: " + brokenA, "The council's form: " + brokenB,
			"A forum post pointed here too: " + third},
		Links: append([]core.Link{ownA, ownB}, kept...)}
}

// grantsHub is the ready view open on that message, with relay's live
// links as given.
func grantsHub(third string, live ...string) core.Hub {
	h := hubStep(hubFixture(),
		core.ReadyLoaded{At: t0, Rev: 9, Items: []core.ReadyItem{grantsItem(third), readyFixtureItems()[1]}},
		core.LinksLoaded{At: t0, Rev: 9, Live: live},
		core.StopTyping{At: t0}, core.ReadyOpen{At: t0})
	return h
}

func readyKeys(h core.Hub, at time.Time, actions ...core.ReadyAction) core.Hub {
	for _, a := range actions {
		h = hubStep(h, core.ReadyKey{At: at, Action: a})
	}
	return h
}

func TestReadyLinksGolden(t *testing.T) {
	list := readyKeys(grantsHub(linkCBroken, linkA, linkB), t0, core.ReadyLinksKey)
	ask := readyKeys(list, t0, core.ReadyOpenKey)
	tooSoon := readyKeys(ask, t0, core.ReadyYes)
	sent := readyKeys(ask, t0.Add(core.ArmDelay), core.ReadyYes)
	// relay answers, and sends the set and then the message again, its
	// text as it was and the link now listed.
	kept := hubStep(sent, core.LinkDone{At: t0, Address: linkCBroken, Keep: true, Changed: true},
		core.LinksChanged{At: t0, Rev: 10, Live: []string{linkC}},
		core.ReadyChanged{At: t0, Rev: 10, Items: []core.ReadyItem{grantsItem(linkCBroken, ownC)}})
	live := readyKeys(list, t0, core.ReadyUp)
	drop := readyKeys(live, t0, core.ReadyDismiss)
	refusedAsk := readyKeys(readyKeys(grantsHub(linkDBroken, linkA, linkB), t0, core.ReadyLinksKey, core.ReadyOpenKey), t0.Add(core.ArmDelay), core.ReadyYes)
	refused := hubStep(refusedAsk, core.LinkDone{At: t0, Address: linkDBroken, Keep: true,
		Err: "A link is one http or https address in plain ASCII, with no spaces, no brackets and no user name before its host. Write a host in other letters in its xn-- form."})
	for _, c := range []struct {
		name string
		h    core.Hub
	}{{"hint", grantsHub(linkCBroken, linkA, linkB)}, {"list", list}, {"live_selected", live}, {"keep_question", ask}, {"too_soon", tooSoon},
		{"kept", kept}, {"drop_question", drop}, {"refused", refused}, {"keys", readyKeys(list, t0, core.ReadyHelp)}} {
		for _, w := range []int{59, 119} {
			t.Run(fmt.Sprintf("%s_%d", c.name, w), func(t *testing.T) {
				requireGolden(t, HubView(c.h, "", "", t0, w, 16).Content)
			})
		}
	}
	if f := HubView(ask, "", "", t0, 119, 16); f.Active != core.SurfaceReadyLinkAsk {
		t.Errorf("the question is drawn as %v", f.Active)
	}
	if f := HubView(list, "", "", t0, 119, 16); f.Active != core.SurfaceReady {
		t.Errorf("the list is drawn as %v", f.Active)
	}
}

// grantsCard is a checklist a role made from the same finding.
func grantsCard() core.Card {
	out := func(s string) core.CardText { return core.CardText{Text: s, Outsider: true, Rev: 1} }
	return core.Card{ID: 1900, Kind: core.CardChecklist, Status: core.CardActive, Rev: 1900, Owner: "steward",
		Title: out("Grants to apply for"), Total: 3, Labels: core.Labels{Contaminated: true},
		Items: []core.CardElem{{ID: 1, CardText: out("Arts fund spring round, due 1 Dec: " + brokenA)},
			{ID: 2, CardText: out("Council form: " + brokenB)},
			{ID: 3, CardText: out("A forum post pointed here too: " + linkCBroken)}},
		Links: []core.Link{ownA, ownB}}
}

func grantsBoard() core.Single {
	s := boardState(true, grantsCard(), cardNote)
	s.App.Links = core.LinksOf(linkA, linkB)
	return s
}

func boardKey(s core.Single, at time.Time, actions ...core.PaneAction) core.Single {
	for _, a := range actions {
		s, _ = core.ReduceSingle(s, core.PaneKey{At: at, Action: a})
	}
	return s
}

func TestBoardLinksGolden(t *testing.T) {
	list := boardKey(grantsBoard(), t0, core.PaneLinks)
	ask := boardKey(list, t0, core.PaneEnter)
	sent := boardKey(ask, t0.Add(core.ArmDelay), core.PaneYes)
	kept, _ := core.ReduceSingle(sent, core.LinkDone{At: t0, Address: linkCBroken, Keep: true, Changed: true})
	refused, _ := core.ReduceSingle(sent, core.LinkDone{At: t0, Address: linkCBroken, Keep: true, Err: "A link is one http or https address in plain ASCII."})
	drop := boardKey(list, t0, core.PaneUp, core.PaneDrop)
	for name, s := range map[string]core.Single{"hint": grantsBoard(), "list": list, "keep_question": ask, "kept": kept, "refused": refused, "drop_question": drop} {
		t.Run(name, func(t *testing.T) {
			goldenLines(t, PaneLines(s.App, s.State, t0, 44, 22))
		})
	}
	if got := paneActive(ask.App, ask.State); got != core.SurfaceLinkAsk {
		t.Errorf("the question is drawn as %v", got)
	}
	if got := paneActive(list.App, list.State); got != core.SurfacePane {
		t.Errorf("the list is drawn as %v", got)
	}
}

// The list shows every link in full, in its broken form, over as many
// rows as it takes. Each row of a live one is a hyperlink to the address
// relay listed for it.
func TestTheListShowsEachLinkInFull(t *testing.T) {
	live := core.LinksOf(linkA, linkB)
	links := []core.ShownLink{liveRow(ownA), liveRow(ownB), {Text: linkCBroken}}
	for _, inner := range []int{20, 34, 40, 60, 100} {
		rows, selA, selB := linkRows(links, 1, live, inner)
		var plain []string
		for i, r := range rows {
			if Width(r) > inner {
				t.Fatalf("at %d: row %q is %d wide", inner, ansi.Strip(r), Width(r))
			}
			text := strings.TrimSpace(strings.TrimLeft(ansi.Strip(r), "●○ "))
			plain = append(plain, text)
			if selected := i >= selA && i < selB; selected != (strings.Contains(r, sgr(selBar)) || strings.Contains(r, ";7")) && tinted() {
				t.Fatalf("at %d: row %d selected %v, drawn otherwise: %q", inner, i, selected, r)
			}
		}
		joined := strings.Join(plain, "")
		for l, broken := range map[string]string{linkA: brokenA, linkB: brokenB} {
			if !strings.Contains(joined, broken) || strings.Contains(joined, l) {
				t.Fatalf("at %d: %q is not shown in full: %q", inner, l, plain)
			}
		}
		if !strings.Contains(joined, linkCBroken) {
			t.Fatalf("at %d: the broken link is not shown in full: %q", inner, plain)
		}
		// Every hyperlink on the rows is to one of the two live links.
		out := gate(strings.Join(rows, "\n"), live)
		for _, m := range osc8RE.FindAllStringSubmatch(out, -1) {
			if m[1] != "" && m[1] != linkA && m[1] != linkB {
				t.Fatalf("at %d: a hyperlink to %q", inner, m[1])
			}
		}
		if !strings.Contains(out, linkOpen(linkA)) || !strings.Contains(out, linkOpen(linkB)) {
			t.Fatalf("at %d: a live link is not a hyperlink: %q", inner, out)
		}
	}
	if got := linkLegend(links, 0); got != "2 live · 1 broken" {
		t.Errorf("legend %q", got)
	}
	if got := linkLegend(links[:2], 0); got != "2 live" {
		t.Errorf("legend %q", got)
	}
	if got := linkLegend(links[2:], 0); got != "1 broken" {
		t.Errorf("legend %q", got)
	}
	// relay lists no more than fifty links for one thing, and says how
	// many it left out. The list passes that on.
	if got := linkLegend(links, 3); got != "2 live · 1 broken · 3 more not listed" {
		t.Errorf("legend %q", got)
	}
	if got := linkLegend(nil, 1); got != "1 more not listed" {
		t.Errorf("legend %q", got)
	}
	if rows, _, _ := linkRows(nil, 0, live, 40); len(rows) != 1 || ansi.Strip(rows[0]) != core.NoLinksNote {
		t.Errorf("an empty list: %q", rows)
	}
}

// The list keeps the selected link in view when it is taller than the
// room, and the frame fits at any size, with the question up too.
func TestTheLinkListFitsAtAnySize(t *testing.T) {
	many := grantsItem(linkCBroken)
	for i := range 12 {
		many.Lines = append(many.Lines, fmt.Sprintf("more hxxps://more[.]example/%d/", i)+strings.Repeat("long-", i))
	}
	base := hubStep(hubFixture(), core.ReadyLoaded{At: t0, Rev: 9, Items: []core.ReadyItem{many}},
		core.LinksLoaded{At: t0, Rev: 9, Live: []string{linkA, linkB}}, core.StopTyping{At: t0}, core.ReadyOpen{At: t0})
	list := readyKeys(base, t0, core.ReadyLinksKey)
	last := readyKeys(list, t0, core.ReadyBottom)
	views := map[string]core.Hub{"list": list, "last": last, "keep": readyKeys(last, t0, core.ReadyOpenKey),
		"drop": readyKeys(readyKeys(list, t0, core.ReadyTop), t0, core.ReadyDismiss)}
	for name, v := range views {
		for _, w := range []int{1, 8, 20, 40, 59, 72, 99, 119, 160} {
			for _, rows := range []int{2, 5, 10, 24, 40} {
				f := HubView(v, "", "hint", t0, w, rows)
				lines := strings.Split(f.Content, "\n")
				if len(lines) != rows {
					t.Errorf("%s %dx%d: %d rows", name, w, rows, len(lines))
				}
				for i, l := range lines {
					if got := Width(l); got > w {
						t.Errorf("%s %dx%d: row %d is %d wide: %q", name, w, rows, i, got, l)
					}
				}
			}
		}
	}
	// The last link is selected and on screen, though the list is taller
	// than a 12-row screen.
	if got := ansi.Strip(HubView(last, "", "", t0, 100, 12).Content); !strings.Contains(got, "hxxps://more[.]example/11/") {
		t.Fatalf("the selected link is out of view:\n%s", got)
	}
	// The board's list, in a pane of any size.
	b := boardKey(grantsBoard(), t0, core.PaneLinks, core.PaneEnter)
	for _, w := range []int{1, 8, 20, 36, 44, 52, 80} {
		for _, rows := range []int{0, 3, 8, 22, 40} {
			lines := PaneLines(b.App, b.State, t0, w, rows)
			if len(lines) != rows {
				t.Errorf("board %dx%d: %d rows", w, rows, len(lines))
			}
			for i, l := range lines {
				if got := Width(l); got > w {
					t.Errorf("board %dx%d: row %d is %d wide: %q", w, rows, i, got, l)
				}
			}
		}
	}
}

// With the list open, and with its question up, nothing drawn can be
// followed but a live link: the property of links_test.go, over the
// list's own frames.
func TestTheLinkListFollowsNothingButALiveLink(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		live := genLive(t)
		w := rapid.IntRange(20, 140).Draw(t, "width")
		h := rapid.IntRange(6, 50).Draw(t, "height")
		steps := rapid.SliceOfN(rapid.SampledFrom([]string{"down", "up", "ask", "drop", "yes", "no"}), 0, 6).Draw(t, "keys")

		// The ready view.
		item := core.ReadyItem{ID: "message:4", Kind: core.ReadyMessage, At: t0.Add(-time.Minute).Format(time.RFC3339),
			Title: genOutside(t, "readyTitle"), Outsider: true, From: "from the web", Message: 4,
			Lines: []string{genOutside(t, "line1"), genOutside(t, "line2")}, Links: genOwn(t, "item"),
			LinksLeftOut: rapid.IntRange(0, 3).Draw(t, "leftOut")}
		hub := hubStep(hubFixture(), core.ReadyLoaded{At: t0, Rev: 9, Items: []core.ReadyItem{item}}, core.StopTyping{At: t0}, core.ReadyOpen{At: t0})
		hub.App.Links = live
		hub = readyKeys(hub, t0, core.ReadyLinksKey)
		at := t0
		for _, k := range steps {
			at = at.Add(core.ArmDelay)
			hub = readyKeys(hub, at, map[string]core.ReadyAction{"down": core.ReadyDown, "up": core.ReadyUp, "ask": core.ReadyOpenKey,
				"drop": core.ReadyDismiss, "yes": core.ReadyYes, "no": core.ReadyEsc}[k])
			content := HubView(hub, "", "", t0, w, h).Content
			checkFollowed(t, "the ready view's list of links", content, live)
			checkListed(t, "the ready view's list of links", content, item.Links)
		}

		// The board.
		card := genOutsideCard(t, 7)
		s := boardState(true, card)
		s.App.Links = live
		s = boardKey(s, t0, core.PaneLinks)
		at = t0
		for _, k := range steps {
			at = at.Add(core.ArmDelay)
			s = boardKey(s, at, map[string]core.PaneAction{"down": core.PaneDown, "up": core.PaneUp, "ask": core.PaneEnter,
				"drop": core.PaneDrop, "yes": core.PaneYes, "no": core.PaneEsc}[k])
			content := View(s, "", "", t0, w, h).Content
			checkFollowed(t, "the board's list of links", content, live)
			checkListed(t, "the board's list of links", content, card.Links)
		}
	})
}

// A row is a hyperlink only when it is a live one, to the address relay
// listed, and only while that address is in the connection's set. A
// broken row is text whatever it reads as: the broken form of a link in
// the set, or a whole address.
func TestOnlyALiveRowIsAHyperlink(t *testing.T) {
	spans := func(links []core.ShownLink, live core.Links) []string {
		rows, _, _ := linkRows(links, 0, live, 60)
		var targets []string
		for _, m := range osc8RE.FindAllStringSubmatch(gate(strings.Join(rows, "\n"), live), -1) {
			if m[1] != "" {
				targets = append(targets, m[1])
			}
		}
		return targets
	}
	set := core.LinksOf(linkA, linkB)
	if got := spans([]core.ShownLink{liveRow(ownA)}, set); len(got) != 1 || got[0] != linkA {
		t.Errorf("a live row links to %q", got)
	}
	for name, row := range map[string]core.ShownLink{
		"a broken row that reads as a live link's form": {Text: brokenA},
		"a broken row that is a whole address":          {Text: linkA},
		"a broken row with an address":                  {Text: brokenA, Address: linkA},
		"a live row whose address is not in the set":    liveRow(ownC),
		"a live row whose address is no link":           {Text: brokenA, Address: "javascript:alert(1)", Live: true},
	} {
		if got := spans([]core.ShownLink{row}, set); len(got) != 0 {
			t.Errorf("%s links to %q", name, got)
		}
		rows, _, _ := linkRows([]core.ShownLink{row}, 0, set, 60)
		if plain := ansi.Strip(strings.Join(rows, "\n")); followable(plain) {
			t.Errorf("%s can be followed as text: %q", name, plain)
		}
	}
	// A live row's hyperlink goes to the address relay listed, whatever
	// its text reads as.
	odd := core.ShownLink{Text: brokenB, Address: linkA, Live: true}
	if got := spans([]core.ShownLink{odd}, set); len(got) == 0 || got[0] != linkA {
		t.Errorf("a live row links to %q, want the listed address", got)
	}
}

// The list says how many live links relay left out, in its top border,
// in the ready view and on the board.
func TestTheListSaysWhatRelayLeftOut(t *testing.T) {
	it := grantsItem(linkCBroken)
	it.LinksLeftOut = 4
	hub := hubStep(hubFixture(), core.ReadyLoaded{At: t0, Rev: 9, Items: []core.ReadyItem{it}},
		core.LinksLoaded{At: t0, Rev: 9, Live: []string{linkA, linkB}}, core.StopTyping{At: t0}, core.ReadyOpen{At: t0})
	if plain := ansi.Strip(HubView(readyKeys(hub, t0, core.ReadyLinksKey), "", "", t0, 119, 16).Content); !strings.Contains(plain, "2 live · 1 broken · 4 more not listed") {
		t.Errorf("the ready view's list:\n%s", plain)
	}
	c := grantsCard()
	c.LinksLeftOut = 4
	s := boardState(true, c)
	s.App.Links = core.LinksOf(linkA, linkB)
	if plain := ansi.Strip(strings.Join(PaneLines(boardKey(s, t0, core.PaneLinks).App, s.State, t0, 60, 22), "\n")); !strings.Contains(plain, "4 more not listed") {
		t.Errorf("the board's list:\n%s", plain)
	}
}

// An item relay did not mark as an outsider's, a daily note with one line
// that drew on outside text, has its links listed like any other. Its
// list gives a terminal nothing to follow but a link relay listed for it
// that is in the set too, whatever the item lists and whatever the set
// holds.
func TestTheListOfAnItemOfRelaysOwnFollowsOnlyWhatRelayListed(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		live := genLive(t)
		w := rapid.IntRange(20, 140).Draw(t, "width")
		h := rapid.IntRange(6, 50).Draw(t, "height")
		// relay's own text, with the outside line's links broken as relay
		// sends them.
		var lines []string
		for range rapid.IntRange(1, 4).Draw(t, "lines") {
			shape := rapid.SampledFrom([]string{"%s", "(%s).", "**%s**", "see:%s", "%s/more", "\"%s\"", "x=%s", "hxxps://evil[.]test/?u=%s", "%s %s"}).Draw(t, "shape")
			lines = append(lines, "Today: "+strings.ReplaceAll(shape, "%s", brokenOf(rapid.SampledFrom(goodLinks).Draw(t, "shown"))))
		}
		item := core.ReadyItem{ID: "message:4", Kind: core.ReadyMessage, At: t0.Add(-time.Minute).Format(time.RFC3339),
			Title: "Your day", Outsider: false, Message: 4, Lines: lines, Links: genOwn(t, "item"),
			LinksLeftOut: rapid.IntRange(0, 3).Draw(t, "leftOut")}
		hub := hubStep(hubFixture(), core.ReadyLoaded{At: t0, Rev: 9, Items: []core.ReadyItem{item}}, core.StopTyping{At: t0}, core.ReadyOpen{At: t0})
		hub.App.Links = live
		hub = readyKeys(hub, t0, core.ReadyLinksKey)
		at := t0
		for _, k := range rapid.SliceOfN(rapid.SampledFrom([]core.ReadyAction{core.ReadyDown, core.ReadyUp, core.ReadyOpenKey, core.ReadyDismiss, core.ReadyYes, core.ReadyEsc}), 0, 6).Draw(t, "keys") {
			at = at.Add(core.ArmDelay)
			hub = readyKeys(hub, at, k)
			content := HubView(hub, "", "", t0, w, h).Content
			checkFollowed(t, "the list of a note of relay's own", content, live)
			checkListed(t, "the list of a note of relay's own", content, item.Links)
		}
	})
}

// checkListed fails when a drawn frame holds a hyperlink to an address
// relay did not list for the thing.
func checkListed(t *rapid.T, what, content string, own []core.Link) {
	t.Helper()
	for _, m := range osc8RE.FindAllStringSubmatch(content, -1) {
		listed := false
		for _, l := range own {
			listed = listed || l.Address == m[1]
		}
		if m[1] != "" && !listed {
			t.Fatalf("%s: a hyperlink to %q, which relay did not list for it, in\n%q", what, m[1], content)
		}
	}
}

// A list longer than its pane keeps the selected link on screen, all of
// it, as the selection moves down and back up: on the board, where the
// pane's window keeps a row for each of its "above" and "below" marks
// outside the selection, and in the ready view.
func TestTheSelectedLinkIsOnScreenInAShortPane(t *testing.T) {
	var items []core.CardElem
	var lines []string
	var want []string
	for i := range 14 {
		l := fmt.Sprintf("hxxps://grants[.]example/round/%02d", i)
		want = append(want, l)
		items = append(items, core.CardElem{ID: i + 1, CardText: core.CardText{Text: "See " + l, Outsider: true, Rev: 1}})
		lines = append(lines, "See "+l)
	}
	c := grantsCard()
	c.Items, c.Links, c.Total = items, nil, len(items)
	board := boardKey(boardState(true, c), t0, core.PaneLinks)
	it := grantsItem(linkCBroken)
	it.Lines, it.Links = lines, nil
	hub := readyKeys(hubStep(hubFixture(), core.ReadyLoaded{At: t0, Rev: 9, Items: []core.ReadyItem{it}}, core.StopTyping{At: t0}, core.ReadyOpen{At: t0}), t0, core.ReadyLinksKey)
	for _, height := range []int{9, 12, 16} {
		b, h := board, hub
		for i := range len(want) * 2 {
			sel := i
			if i >= len(want) {
				sel = 2*len(want) - 1 - i // and back up
			}
			plain := ansi.Strip(strings.Join(PaneLines(b.App, b.State, t0, 60, height), "\n"))
			if !strings.Contains(plain, want[sel]) {
				t.Fatalf("board at %d rows, link %d selected, not on screen:\n%s", height, sel, plain)
			}
			plain = ansi.Strip(HubView(h, "", "", t0, 100, height+4).Content)
			if !strings.Contains(plain, want[sel]) {
				t.Fatalf("ready view at %d rows, link %d selected, not on screen:\n%s", height+4, sel, plain)
			}
			if i < len(want)-1 {
				b, h = boardKey(b, t0, core.PaneDown), readyKeys(h, t0, core.ReadyDown)
			} else if i >= len(want) {
				b, h = boardKey(b, t0, core.PaneUp), readyKeys(h, t0, core.ReadyUp)
			}
		}
	}
}

// The link a question is about is on screen with the question and the
// key that answers it, at any size: it has rows of its own under the
// question, and on a screen too short for everything the list and the
// bar go first, then the question's first rows. The judge is the
// question's own link, as the test wrote it, and "y".
func TestAQuestionShowsItsLinkOnAShortScreen(t *testing.T) {
	ask := readyKeys(grantsHub(linkCBroken, linkA, linkB), t0, core.ReadyLinksKey, core.ReadyDown, core.ReadyDown, core.ReadyOpenKey)
	if ask.Ready.Links == nil || ask.Ready.Links.Ask == nil || ask.Ready.Links.Ask.Text != linkCBroken {
		t.Fatalf("no question about %q: %+v", linkCBroken, ask.Ready.Links)
	}
	noticed := ask
	noticed.Notice = "A yes counts once the question has been up a moment."
	for _, h := range []core.Hub{ask, noticed} {
		for _, size := range [][2]int{{20, 6}, {24, 5}, {30, 4}, {16, 8}, {40, 7}, {60, 10}, {100, 30}} {
			plain := ansi.Strip(HubView(h, "", "", t0, size[0], size[1]).Content)
			solid := strings.Join(strings.Fields(plain), "")
			if !strings.Contains(solid, linkCBroken) || !strings.Contains(plain, "y keep") {
				t.Errorf("at %dx%d, notice %v, the link or the key is not on screen:\n%s", size[0], size[1], h.Notice != "", plain)
			}
		}
	}
	// A link far longer than the screen shows its start, the host among
	// it, with the question and the key; the list has it whole.
	long := "hxxps://evil[.]test/" + strings.Repeat("a", 300)
	it := grantsItem(long)
	longAsk := readyKeys(hubStep(hubFixture(), core.ReadyLoaded{At: t0, Rev: 9, Items: []core.ReadyItem{it}}, core.StopTyping{At: t0}, core.ReadyOpen{At: t0}),
		t0, core.ReadyLinksKey, core.ReadyDown, core.ReadyDown, core.ReadyOpenKey)
	if longAsk.Ready.Links == nil || longAsk.Ready.Links.Ask == nil || longAsk.Ready.Links.Ask.Text != long {
		t.Fatalf("no question about the long link: %+v", longAsk.Ready.Links)
	}
	for _, size := range [][2]int{{40, 8}, {30, 6}, {30, 4}, {24, 5}, {100, 30}} {
		plain := ansi.Strip(HubView(longAsk, "", "", t0, size[0], size[1]).Content)
		solid := strings.Join(strings.Fields(plain), "")
		if !strings.Contains(solid, "hxxps://evil[.]test/") || !strings.Contains(plain, "y keep") || !strings.Contains(solid, "Keepit?") {
			t.Errorf("at %dx%d the long link's host, the question or the key is not on screen:\n%s", size[0], size[1], plain)
		}
		if lines := strings.Split(plain, "\n"); len(lines) > size[1] {
			t.Errorf("at %dx%d the frame has %d rows", size[0], size[1], len(lines))
		}
	}
	// On the board too.
	b := boardKey(grantsBoard(), t0, core.PaneLinks, core.PaneDown, core.PaneDown, core.PaneEnter)
	if b.Board.Links == nil || b.Board.Links.Ask == nil || b.Board.Links.Ask.Text != linkCBroken {
		t.Fatalf("no board question about %q", linkCBroken)
	}
	for _, size := range [][2]int{{30, 6}, {44, 8}, {60, 22}} {
		plain := ansi.Strip(strings.Join(PaneLines(b.App, b.State, t0, size[0], size[1]), "\n"))
		if solid := strings.Join(strings.Fields(plain), ""); !strings.Contains(solid, linkCBroken) || !strings.Contains(plain, "y keep") {
			t.Errorf("board at %dx%d: the link or the key is not on screen:\n%s", size[0], size[1], plain)
		}
	}
	c := grantsCard()
	c.Items[2].Text = "A forum post: " + long
	lb := boardKey(boardState(true, c), t0, core.PaneLinks, core.PaneDown, core.PaneDown, core.PaneEnter)
	if lb.Board.Links == nil || lb.Board.Links.Ask == nil || lb.Board.Links.Ask.Text != long {
		t.Fatalf("no board question about the long link")
	}
	for _, size := range [][2]int{{30, 6}, {40, 8}, {60, 22}} {
		plain := ansi.Strip(strings.Join(PaneLines(lb.App, lb.State, t0, size[0], size[1]), "\n"))
		if solid := strings.Join(strings.Fields(plain), ""); !strings.Contains(solid, "hxxps://evil[.]test/") || !strings.Contains(plain, "y keep") {
			t.Errorf("board at %dx%d: the long link's host or the key is not on screen:\n%s", size[0], size[1], plain)
		}
	}
}

// When relay sends the thing again with a link before the asked one, the
// bar follows the asked link: what is selected is what a yes sends.
func TestTheBarStaysOnTheAskedLinkWhenTheListChanges(t *testing.T) {
	ask := readyKeys(grantsHub(linkCBroken, linkA, linkB), t0, core.ReadyLinksKey, core.ReadyDown, core.ReadyDown, core.ReadyOpenKey)
	it := grantsItem(linkCBroken)
	it.Lines = append([]string{"First: hxxps://other[.]example/x"}, it.Lines...)
	changed := hubStep(ask, core.ReadyChanged{At: t0, Rev: 10, Items: []core.ReadyItem{it}})
	ll := changed.Ready.Links
	if ll == nil || ll.Ask == nil {
		t.Fatalf("the question closed: %+v", ll)
	}
	_, links, _ := core.ReadyLinks(changed.Ready, changed.App.Links)
	if ll.Sel < 0 || ll.Sel >= len(links) || links[ll.Sel].Text != linkCBroken {
		t.Fatalf("the bar is on %+v, the question about %q", links[max(ll.Sel, 0)], linkCBroken)
	}
}

// A host made long to hide where it goes (a bank's name as a subdomain of
// another) is never cut in the question. Where the screen has room it is
// all on screen; where it has not, the frame says so, and a yes to it is
// refused. The judge reads the frame as plain text, joined, with its
// frames' marks taken out, and looks for the host the test wrote.
func TestAQuestionNeverShowsPartOfAHost(t *testing.T) {
	const host = "hxxps://accounts[.]bank[.]example[.]secure-login-verify-session-id[.]evil[.]test"
	long := host + "/x?session=" + strings.Repeat("a", 80)
	it := grantsItem(long)
	ready := readyKeys(hubStep(hubFixture(), core.ReadyLoaded{At: t0, Rev: 9, Items: []core.ReadyItem{it}}, core.StopTyping{At: t0}, core.ReadyOpen{At: t0}),
		t0, core.ReadyLinksKey, core.ReadyDown, core.ReadyDown, core.ReadyOpenKey)
	c := grantsCard()
	c.Items[2].Text = "A forum post: " + long
	board := boardKey(boardState(true, c), t0, core.PaneLinks, core.PaneDown, core.PaneDown, core.PaneEnter)
	// The same question with the hub's pane in focus.
	boardHub := hubFixture()
	boardHub.App = board.App
	boardHub.App.PaneFocus = true
	if ready.Ready.Links.Ask == nil || board.Board.Links.Ask == nil {
		t.Fatal("no question open")
	}
	read := func(content string) string {
		// In the hub's columns only what stands right of the sidebar is
		// read, as a reader reads one column.
		var rows []string
		for _, r := range strings.Split(ansi.Strip(content), "\n") {
			if i := strings.LastIndex(r, "││"); i >= 0 {
				r = r[i+len("││"):]
			}
			rows = append(rows, r)
		}
		content = strings.Join(rows, "\n")
		return strings.Map(func(r rune) rune {
			if r == ' ' || r == '\n' || (r >= 0x2500 && r <= 0x259F) {
				return -1
			}
			return r
		}, ansi.Strip(content))
	}
	for w := 20; w <= 120; w += 3 {
		for h := 4; h <= 30; h++ {
			f := HubView(ready, "", "", t0, w, h)
			if on := strings.Contains(read(f.Content), host) && strings.Contains(read(f.Content), "Keepit?"); on != f.AskShown {
				t.Fatalf("ready view at %dx%d: host on screen %v, frame says %v:\n%s", w, h, on, f.AskShown, ansi.Strip(f.Content))
			}
			if w >= 40 && h >= 10 && !f.AskShown {
				t.Fatalf("ready view at %dx%d has room and does not show the host:\n%s", w, h, ansi.Strip(f.Content))
			}
			if f.AskShown && !strings.Contains(ansi.Strip(f.Content), "y keep") {
				t.Fatalf("ready view at %dx%d shows the host without the key:\n%s", w, h, ansi.Strip(f.Content))
			}
			if f.AskShown && !strings.Contains(read(f.Content), "Keepit?") {
				t.Fatalf("ready view at %dx%d says the question is shown without its words:\n%s", w, h, ansi.Strip(f.Content))
			}
			bf := View(board, "", "", t0, w, h)
			hf := HubView(boardHub, "", "", t0, w, h)
			for name, fr := range map[string]struct {
				content string
				shown   bool
			}{"board": {bf.Content, bf.AskShown}, "board in the hub": {hf.Content, hf.AskShown}} {
				if on := strings.Contains(read(fr.content), host) && strings.Contains(read(fr.content), "Keepit?"); fr.shown && !on {
					t.Fatalf("%s at %dx%d: the frame says the question is shown and it is not:\n%s", name, w, h, ansi.Strip(fr.content))
				}
			}
		}
	}
	// With no question open, no frame claims one is shown.
	if f := HubView(hubFixture(), "", "", t0, 100, 30); f.AskShown {
		t.Fatal("a frame with no question says one is shown")
	}
}
