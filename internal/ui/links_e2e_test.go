package ui_test

// End to end for live links: the real ui.Model and internal/agent against
// the fake hub, drawn into a terminal emulator. What is checked is what
// the terminal ends up holding: which cells carry a hyperlink, and to
// what.

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/kirahowe/relay-tui/internal/core"
	"github.com/kirahowe/relay-tui/internal/fakehub"
)

const (
	e2eLink        = "https://artsfund.example/grants/2027"
	e2eLinkBroken  = "hxxps://artsfund[.]example/grants/2027"
	e2eLong        = "https://artscouncil.example/apply/spring-round?year=2027&ref=newsletter"
	e2eLongBroken  = "hxxps://artscouncil[.]example/apply/spring-round?year=2027&ref=newsletter"
	e2eOther       = "https://bit.example/3xQ9"
	e2eOtherBroken = "hxxps://bit[.]example/3xQ9"
)

// linksHub is a hub with one message ready that quotes three links from
// outside, as relay sends them: every one broken in the text, and the two
// that are live listed beside it, one of them long.
func linksHub() fakehub.Config {
	cfg := threeRoles()
	cfg.Links = []string{e2eLink, e2eLong}
	cfg.Ready = []json.RawMessage{json.RawMessage(`{"id": "message:4", "kind": "message", "at": "2026-10-14T12:00:00Z", "seq": 4,
		"title": "Two grants worth a look",
		"lines": ["Arts fund spring round, due 1 Dec: ` + e2eLinkBroken + `.", "The council's form: ` + e2eLongBroken + `",
		          "A forum post pointed here too: ` + e2eOtherBroken + `"],
		"from": "Drew on: the web page grants-2027.html", "outsider": true,
		"message": 4, "messageKind": "note", "urgency": "normal", "thread": 4, "replyTo": 4, "replies": []}`)}
	return cfg
}

// linkAt is the hyperlink target of the cell at column x of row y, or "".
func (t *terminal) linkAt(x, y int) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if c := t.emu.CellAt(x, y); c != nil {
		return c.Link.URL
	}
	return ""
}

// place is where text starts on the screen: its row, and its first
// column in cells. ok is false when it is not there.
func place(screen, text string) (x, y int, ok bool) {
	for y, row := range strings.Split(screen, "\n") {
		if i := strings.Index(row, text); i >= 0 {
			return ansi.StringWidth(row[:i]), y, true
		}
	}
	return 0, 0, false
}

// linked says every cell of text on the screen is a hyperlink to target,
// and the cells either side of it are not.
func (h *hubHarness) linked(text, target string) bool {
	x, y, ok := place(h.screen(), text)
	if !ok {
		return false
	}
	for i := range ansi.StringWidth(text) {
		if h.emu.linkAt(x+i, y) != target {
			return false
		}
	}
	return h.emu.linkAt(x-1, y) != target && h.emu.linkAt(x+ansi.StringWidth(text), y) != target
}

// unlinked says text is on the screen and no cell of it is a hyperlink.
func (h *hubHarness) unlinked(text string) bool {
	x, y, ok := place(h.screen(), text)
	if !ok {
		return false
	}
	for i := range ansi.StringWidth(text) {
		if h.emu.linkAt(x+i, y) != "" {
			return false
		}
	}
	return true
}

var writtenLink = regexp.MustCompile("\x1b\\]8;[^;\x07\x1b]*;([^\x07\x1b]*)(?:\x07|\x1b\\\\)")

// targets is every hyperlink target the program has written.
func (h *hubHarness) targets() map[string]bool {
	out := map[string]bool{}
	for _, m := range writtenLink.FindAllStringSubmatch(h.emu.written(), -1) {
		if m[1] != "" {
			out[m[1]] = true
		}
	}
	return out
}

// noAddressAsText fails when the program has written an address anywhere
// but in a hyperlink's target: on a row, a terminal's own link finder
// could follow it, or some of it.
func (h *hubHarness) noAddressAsText(t *testing.T) {
	t.Helper()
	text := writtenLink.ReplaceAllString(h.emu.written(), "")
	for _, scheme := range []string{"https://", "http://"} {
		if i := strings.Index(text, scheme); i >= 0 {
			t.Fatalf("an address was written as text: %q", text[i:min(len(text), i+60)])
		}
	}
}

func TestALiveLinkReachesTheTerminalAsAHyperlink(t *testing.T) {
	h := startHub(t, linksHub(), 100, 30, "")
	h.waitLog("links")
	h.keys("R")
	h.waitScreen("the ready view", contains("Two grants worth a look"))

	// The live link reads broken, as every link does, and the terminal
	// holds a hyperlink to exactly its address under every cell of it.
	h.wait("the live link as a hyperlink", h.screen, func(string) bool { return h.linked(e2eLinkBroken, e2eLink) })
	h.wait("the long link as a hyperlink", h.screen, func(string) bool { return h.linked(e2eLongBroken, e2eLong) })
	// The third is not live: the same kind of text, with no hyperlink
	// under it.
	if !h.unlinked(e2eOtherBroken) {
		t.Fatalf("the broken link is not plain text:\n%s", h.screen())
	}

	// Kept on another device, it becomes a hyperlink here.
	h.say("keep-link " + e2eOtherBroken)
	h.wait("the kept link as a hyperlink", h.screen, func(string) bool { return h.linked(e2eOtherBroken, e2eOther) })

	// Dropped, a link reads as it did and the terminal no longer holds a
	// hyperlink for it.
	h.say("drop-link " + e2eLink)
	h.wait("the dropped link with no hyperlink", h.screen, func(s string) bool { return h.unlinked(e2eLinkBroken) })

	// Nothing but those three addresses was ever written as a target, and
	// no address was written as text.
	for target := range h.targets() {
		if target != e2eLink && target != e2eLong && target != e2eOther {
			t.Errorf("a hyperlink to %q was written", target)
		}
	}
	h.noAddressAsText(t)
}

// On a narrow screen the long link does not fit a row. It is wrapped as
// any text is, and every piece of it opens the whole address.
func TestALinkLongerThanItsRowOpensTheWholeAddressFromEachRow(t *testing.T) {
	h := startHub(t, linksHub(), 60, 30, "")
	h.waitLog("links")
	h.keys("R")
	h.waitScreen("the ready view", contains("Two grants"))
	// Opened whole, so the list's cut to a few rows is out of the way.
	h.keys("\r")
	// rowText is what a row holds from column x on, without its frame.
	rowText := func(s string, x, y int) string {
		rows := strings.Split(s, "\n")
		if y >= len(rows) {
			return ""
		}
		cells := []rune(rows[y])
		if x >= len(cells) {
			return ""
		}
		return strings.TrimRight(string(cells[x:]), " │")
	}
	var shown string
	h.wait("the long link broken over rows", h.screen, func(s string) bool {
		x, y, ok := place(s, "hxxps://artscouncil[.]example")
		if !ok {
			return false
		}
		// Every cell of the link, on this row and the next, opens the
		// whole address.
		shown = ""
		for dy := range 2 {
			text := rowText(s, x, y+dy)
			if text == "" {
				return false
			}
			for i := range ansi.StringWidth(text) {
				if h.emu.linkAt(x+i, y+dy) != e2eLong {
					return false
				}
			}
			shown += text
		}
		return true
	})
	if shown != e2eLongBroken {
		t.Fatalf("the rows show %q, want the link in its broken form", shown)
	}
	// The short one fits a row here.
	if !h.linked(e2eLinkBroken, e2eLink) {
		t.Fatalf("the short link is not a hyperlink:\n%s", h.screen())
	}
	for target := range h.targets() {
		if target != e2eLink && target != e2eLong {
			t.Errorf("a hyperlink to %q was written", target)
		}
	}
	h.noAddressAsText(t)
}

// A relay without the method leaves every link broken, as before.
func TestWithoutRelaysLinksEveryLinkStaysBroken(t *testing.T) {
	cfg := linksHub()
	cfg.Links = nil
	h := startHub(t, cfg, 100, 30, "")
	h.waitLog("ready")
	h.keys("R")
	h.waitScreen("the ready view", contains("Two grants worth a look"))
	h.waitScreen("the links broken", contains(e2eLinkBroken))
	if got := h.targets(); len(got) != 0 {
		t.Fatalf("hyperlinks were written: %v", got)
	}
	h.noAddressAsText(t)
}

// armed waits until a question that has just come up can be answered:
// a yes counts only core.ArmDelay after it was drawn.
func armed() { time.Sleep(core.ArmDelay + 150*time.Millisecond) }

// The list of an item's links, end to end: L lists them, Enter on a
// broken one asks, a y at once is refused and says why, a y once the
// question has been up keeps it, relay's refusal shows with its reason,
// and d then y drops a live one.
func TestKeepingAndDroppingALinkFromTheReadyView(t *testing.T) {
	cfg := linksHub()
	cfg.Ready = []json.RawMessage{json.RawMessage(`{"id": "message:4", "kind": "message", "at": "2026-10-14T12:00:00Z", "seq": 4,
		"title": "Two grants worth a look",
		"lines": ["Arts fund spring round, due 1 Dec: ` + e2eLinkBroken + `", "The council's form: ` + e2eLongBroken + `",
		          "A forum post pointed here too: ` + e2eOtherBroken + `", "A wiki page: hxxps://wiki[.]example/wiki/Fund_(arts)"],
		"from": "Drew on: the web page grants-2027.html", "outsider": true,
		"message": 4, "messageKind": "note", "urgency": "normal", "thread": 4, "replyTo": 4, "replies": []}`)}
	h := startHub(t, cfg, 100, 30, "")
	h.waitLog("links")
	h.keys("R")
	h.waitScreen("the ready view", contains("L links"))
	h.keys("L")
	h.waitScreen("the list", func(s string) bool {
		return strings.Contains(s, "Links · Two grants worth a look") && strings.Contains(s, "2 live · 2 broken") && strings.Contains(s, "enter keep")
	})

	// Enter asks, and a y typed with it comes too soon to answer.
	h.keys("\ry")
	h.waitScreen("the question and the refusal of the early y", func(s string) bool {
		return strings.Contains(s, "relay cannot say where this link came from. Keep it?") && strings.Contains(s, "y keep") &&
			strings.Contains(s, "too soon after it came on screen")
	})
	if got := h.logged("link-keep"); len(got) != 0 {
		t.Fatalf("an early y kept the link: %v", got)
	}
	armed()
	h.keys("y")
	if got := h.waitLog("link-keep"); !strings.Contains(got, `hxxps://bit[.]example/3xQ9`) {
		t.Fatalf("relay was sent %q, not the link as the list showed it", got)
	}
	h.waitScreen("kept", contains("Kept. relay lists it as live, and any client can open it."))
	h.wait("the kept link as a hyperlink in the list", h.screen, func(s string) bool {
		return strings.Contains(s, "3 live · 1 broken") && h.linked(e2eOtherBroken, e2eOther)
	})

	// The wiki link holds round brackets: relay refuses it and says why.
	h.keys("j\r")
	h.waitScreen("the question about the wiki link", contains("Keep it?"))
	armed()
	h.keys("y")
	h.waitScreen("relay's refusal", func(s string) bool {
		flat := strings.Join(strings.Fields(s), " ")
		return strings.Contains(flat, "Not kept.") && strings.Contains(flat, "no brackets and no user name before its host")
	})
	if !h.unlinked("hxxps://wiki[.]example/wiki/Fund_(arts)") {
		t.Fatalf("the refused link is not plain text:\n%s", h.screen())
	}

	// d on a live link asks, any other key leaves it, and y drops it.
	h.keys("gd")
	h.waitScreen("the drop question", func(s string) bool {
		return strings.Contains(s, "Drop this link?") && strings.Contains(s, "y drop")
	})
	h.keys("n")
	h.waitScreen("left live", contains("It stays live."))
	if got := h.logged("link-drop"); len(got) != 0 {
		t.Fatalf("a no dropped the link: %v", got)
	}
	h.keys("d")
	h.waitScreen("the drop question again", contains("Drop this link?"))
	armed()
	h.keys("y")
	if got := h.waitLog("link-drop"); !strings.Contains(got, e2eLink) {
		t.Fatalf("relay was sent %q", got)
	}
	h.waitScreen("dropped", contains("Dropped. Nothing opens it now"))
	h.wait("the dropped link broken in the list", h.screen, func(s string) bool {
		return strings.Contains(s, "2 live · 2 broken") && h.unlinked("hxxps://artsfund[.]example/grants/2027")
	})

	// Esc goes back to the message, which reads as relay now sends it.
	h.keys("\x1b")
	h.wait("the message again", h.screen, func(s string) bool {
		return strings.Contains(s, "Message · Two grants worth a look") && h.linked(e2eOtherBroken, e2eOther)
	})
	for target := range h.targets() {
		if target != e2eLink && target != e2eLong && target != e2eOther {
			t.Errorf("a hyperlink to %q was written", target)
		}
	}
	h.noAddressAsText(t)
}

// The same on the board, through the pane: the list of a card's links,
// the question in the pane's foot, and the card as relay sends it again,
// its text as it was and the link listed.
func TestKeepingALinkFromTheBoard(t *testing.T) {
	cfg := threeRoles()
	cfg.Links = []string{e2eLink}
	cfg.Cards = []json.RawMessage{json.RawMessage(`{"id": 7, "kind": "checklist", "status": "active", "complete": false, "rev": 3,
		"owner": "steward", "addedAt": "2026-10-14T12:00:00Z", "labels": ["contaminated"], "progress": {"done": 0, "total": 2},
		"title": {"text": "Grants to apply for", "outsider": true, "rev": 3},
		"items": [{"id": 1, "text": "Arts fund: ` + e2eLinkBroken + `", "done": false, "outsider": true, "rev": 3},
		          {"id": 2, "text": "A forum post: ` + e2eOtherBroken + `", "done": false, "outsider": true, "rev": 3}],
		"text": "Card 7"}`)}
	h := startHub(t, cfg, 170, 40, "dev")
	h.waitScreen("dev", contains("hello from dev"))
	h.waitScreen("the board beside it", contains("Grants to apply for"))
	h.escape()
	h.keys("l")
	h.waitScreen("the pane's keys", contains("L links"))
	h.keys("L")
	h.waitScreen("the list", func(s string) bool {
		return strings.Contains(s, "1 live · 1 broken") && strings.Contains(s, "enter keep")
	})
	if !h.linked(e2eLinkBroken, e2eLink) || !h.unlinked(e2eOtherBroken) {
		t.Fatalf("the list's links are not drawn as they are:\n%s", h.screen())
	}
	h.keys("\ry")
	h.waitScreen("the question and the refusal of the early y", func(s string) bool {
		flat := strings.Join(strings.Fields(strings.NewReplacer("│", " ", "╯", " ", "╰", " ", "─", " ").Replace(s)), " ")
		return strings.Contains(s, "Keep it?") && strings.Contains(s, "y keep") && strings.Contains(flat, "too soon after it came on screen")
	})
	if got := h.logged("link-keep"); len(got) != 0 {
		t.Fatalf("an early y kept the link: %v", got)
	}
	armed()
	h.keys("y")
	h.waitLog("link-keep")
	h.wait("the kept link as a hyperlink in the list", h.screen, func(s string) bool {
		return strings.Contains(s, "2 live") && !strings.Contains(s, "broken ─") && h.linked(e2eOtherBroken, e2eOther)
	})
	h.keys("\x1b")
	h.wait("the card with the link live", h.screen, func(s string) bool {
		return strings.Contains(s, "Grants to apply for") && !strings.Contains(s, "Links") && h.linked(e2eOtherBroken, e2eOther)
	})
	for target := range h.targets() {
		if target != e2eLink && target != e2eOther {
			t.Errorf("a hyperlink to %q was written", target)
		}
	}
	h.noAddressAsText(t)
}

// A long host is never cut in the question. On a phone-sized 30x6 the
// question's first rows give way to it, "Keep it?" stays, and a y keeps
// the link; on a screen too small to show the host and "Keep it?" a y
// keeps nothing, however long the question has been up.
func TestAYesKeepsALongHostOnlyWhereItIsOnScreen(t *testing.T) {
	const long = "hxxps://accounts[.]bank[.]example[.]secure-login-verify-session-id[.]evil[.]test/x"
	const host = "hxxps://accounts[.]bank[.]example[.]secure-login-verify-session-id[.]evil[.]test"
	for _, c := range []struct {
		w, h int
		kept bool
	}{{30, 6, true}, {30, 5, false}, {22, 4, false}} {
		cfg := linksHub()
		cfg.Ready = []json.RawMessage{json.RawMessage(`{"id": "message:4", "kind": "message", "at": "2026-10-14T12:00:00Z", "seq": 4,
			"title": "Sign in", "lines": ["Your bank: ` + long + `"], "from": "Drew on: a mail", "outsider": true,
			"message": 4, "messageKind": "note", "urgency": "normal", "thread": 4, "replyTo": 4, "replies": []}`)}
		h := startHub(t, cfg, c.w, c.h, "")
		h.waitLog("links")
		h.keys("R")
		h.waitScreen("the ready view", contains("Ready"))
		h.keys("L")
		h.waitScreen("the list", contains("Links"))
		h.keys("\r")
		screen := h.waitScreen("the question", contains("y keep"))
		solid := strings.Join(strings.Fields(strings.NewReplacer("│", "", "╭", "", "╰", "", "─", "", "╮", "", "╯", "").Replace(screen)), "")
		if on := strings.Contains(solid, host) && strings.Contains(solid, "Keepit?"); on != c.kept {
			t.Fatalf("at %dx%d the host on screen is %v:\n%s", c.w, c.h, on, screen)
		}
		armed()
		h.keys("y")
		if c.kept {
			if got := h.waitLog("link-keep"); !strings.Contains(got, long) {
				t.Fatalf("at %dx%d relay was sent %q", c.w, c.h, got)
			}
			continue
		}
		// Any other key says no and closes the question. Keys are dealt
		// with in order, so once the list is back without it the y has
		// been too.
		h.keys("n")
		h.waitScreen("the list again", func(s string) bool { return !strings.Contains(s, "y keep") && strings.Contains(s, "select") })
		if got := h.logged("link-keep"); len(got) != 0 {
			t.Fatalf("at %dx%d a y with the host off screen kept the link: %v", c.w, c.h, got)
		}
	}
}
