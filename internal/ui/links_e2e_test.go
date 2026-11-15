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
	e2eLink       = "https://artsfund.example/grants/2027"
	e2eLong       = "https://artscouncil.example/apply/spring-round?year=2027&ref=newsletter"
	e2eLongBroken = "hxxps://artscouncil[.]example/apply/spring-round?year=2027&ref=newsletter"
	e2eOther      = "https://bit.example/3xQ9"
	e2eBroken     = "hxxps://bit[.]example/3xQ9"
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
		          "A forum post pointed here too: ` + e2eBroken + `"],
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

func TestALiveLinkReachesTheTerminalAsAHyperlink(t *testing.T) {
	h := startHub(t, linksHub(), 100, 30, "")
	h.waitLog("links")
	h.keys("R")
	h.waitScreen("the ready view", contains("Two grants worth a look"))

	// The live link is whole, and the terminal holds a hyperlink to
	// exactly that address under every cell of it.
	h.wait("the live link as a hyperlink", h.screen, func(string) bool { return h.linked("<"+e2eLink+">", e2eLink) })
	// The long one fits a row of this width, so it is whole too.
	h.wait("the long link as a hyperlink", h.screen, func(string) bool { return h.linked("<"+e2eLong+">", e2eLong) })
	// The third is not live: broken, and no hyperlink under it.
	if !h.unlinked(e2eBroken) {
		t.Fatalf("the broken link is not plain text:\n%s", h.screen())
	}

	// Kept on another device, it becomes a hyperlink here.
	h.say("keep-link " + e2eBroken)
	h.wait("the kept link as a hyperlink", h.screen, func(string) bool { return h.linked("<"+e2eOther+">", e2eOther) })

	// Dropped, a link is broken again and the terminal no longer holds a
	// hyperlink for it.
	h.say("drop-link " + e2eLink)
	h.wait("the dropped link broken", h.screen, func(s string) bool {
		return h.unlinked("hxxps://artsfund[.]example/grants/2027")
	})

	// Nothing but those three addresses was ever written as a target.
	for target := range h.targets() {
		if target != e2eLink && target != e2eLong && target != e2eOther {
			t.Errorf("a hyperlink to %q was written", target)
		}
	}
}

// On a narrow screen the long link does not fit a row: it shows broken,
// so no row holds a shorter address, and every piece of it still opens
// the whole one.
func TestALinkLongerThanItsRowShowsBrokenAndOpensTheWholeAddress(t *testing.T) {
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
		x, y, ok := place(s, "<hxxps://artscouncil[.]example")
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
	if shown != "<"+e2eLongBroken+">" {
		t.Fatalf("the rows show %q, want the whole link in its broken form", shown)
	}
	screen := h.screen()
	if strings.Contains(screen, "https://artscouncil") {
		t.Fatalf("part of the long link shows as it is:\n%s", screen)
	}
	// The short one still fits a row here, and is whole.
	if !h.linked("<"+e2eLink+">", e2eLink) {
		t.Fatalf("the short link is not a whole hyperlink:\n%s", screen)
	}
	for target := range h.targets() {
		if target != e2eLink && target != e2eLong {
			t.Errorf("a hyperlink to %q was written", target)
		}
	}
}

// A relay without the method leaves every link broken, as before.
func TestWithoutRelaysLinksEveryLinkStaysBroken(t *testing.T) {
	cfg := linksHub()
	cfg.Links = nil
	h := startHub(t, cfg, 100, 30, "")
	h.waitLog("ready")
	h.keys("R")
	h.waitScreen("the ready view", contains("Two grants worth a look"))
	screen := h.waitScreen("the links broken", contains("hxxps://artsfund[.]example/grants/2027"))
	if strings.Contains(screen, "https://") {
		t.Fatalf("a link is whole:\n%s", screen)
	}
	if got := h.targets(); len(got) != 0 {
		t.Fatalf("hyperlinks were written: %v", got)
	}
}
