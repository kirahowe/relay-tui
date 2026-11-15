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

	"github.com/charmbracelet/x/ansi"

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
// outside, as relay sends them: two live, one of them long, and one
// broken.
func linksHub() fakehub.Config {
	cfg := threeRoles()
	cfg.Links = []string{e2eLink, e2eLong}
	cfg.Ready = []json.RawMessage{json.RawMessage(`{"id": "message:4", "kind": "message", "at": "2026-10-14T12:00:00Z", "seq": 4,
		"title": "Two grants worth a look",
		"lines": ["Arts fund spring round, due 1 Dec: <` + e2eLink + `>", "The council's form: <` + e2eLong + `>",
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
