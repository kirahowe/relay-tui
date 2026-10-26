package ui_test

import (
	"strings"
	"testing"
	"time"
)

func (h *harness) extensionCalls(method string) []string {
	var out []string
	for _, l := range strings.Split(h.agentLog(), "\n") {
		if rest, ok := strings.CutPrefix(l, "extension "+method+" "); ok {
			out = append(out, rest)
		}
	}
	return out
}

// wideWorkspace plays the workspace script on a wide screen to its end,
// with the pane beside the chat. The script has the agent ask permission
// for a call, as a relay from before it dropped its holds does: relay-tui
// refuses it without a word.
func wideWorkspace(t *testing.T) *harness {
	h := start(t, "workspace.jsonl", 140, 34)
	h.keys("go\r")
	h.waitIdle()
	return h
}

func TestWorkspaceSidePane(t *testing.T) {
	h := wideWorkspace(t)
	scr := h.waitScreen("the pane", func(s string) bool { return strings.Contains(s, "Concert venues near the hall") })
	for _, want := range []string{"Workspace 1", "from the web", "hxxps://example[.]org/venues"} {
		if !strings.Contains(scr, want) {
			t.Errorf("%q not on screen:\n%s", want, scr)
		}
	}
	for _, not := range []string{"https://example.org", "https://send.example.net", "untrusted", "contaminated", "permission needed", "[new domain]"} {
		if strings.Contains(scr, not) {
			t.Errorf("%q on screen:\n%s", not, scr)
		}
	}
	// The citations of the latest reply, as relay answered them.
	scr = h.waitScreen("the citations", func(s string) bool { return strings.Contains(s, "Fact 12 [venues]") && strings.Contains(s, "missing") })
	if !strings.Contains(scr, "[[doc:gone]]") {
		t.Errorf("the missing reference is not named:\n%s", scr)
	}
	// The request for permission was refused with a plain cancel, and the
	// turn carried on to its reply.
	h.waitOutcome(`{"outcome":"cancelled"}`)
	if got := len(h.extensionCalls("_relay/resolve")); got != 2 {
		t.Errorf("%d resolve calls, want one per distinct reference", got)
	}
	if len(h.extensionCalls("_relay/items")) < 2 {
		t.Errorf("items asked for %d times; want on connect and after the display:\n%s",
			len(h.extensionCalls("_relay/items")), h.agentLog())
	}
	// Nothing in the pane takes a trust: relay-tui has no key for it.
	h.escape()
	h.keys("l")
	h.waitScreen("focus", func(s string) bool { return strings.Contains(s, "↑↓ select") })
	h.keys("t")
	time.Sleep(100 * time.Millisecond)
	if strings.Contains(h.screen(), "rust") {
		t.Errorf("t does something in the pane:\n%s", h.screen())
	}
	if p := h.prompts(); len(p) != 1 {
		t.Errorf("prompts %q", p)
	}
}

const ctrlXHint = "l to focus"

func TestWorkspaceOverlayOnANarrowScreen(t *testing.T) {
	h := start(t, "workspace.jsonl", 80, 28)
	h.keys("go\r")
	h.waitIdle()
	h.waitScreen("the hint", func(s string) bool { return strings.Contains(s, "l workspace (1)") })
	h.escape()
	h.keys("l")
	scr := h.waitScreen("the overlay", func(s string) bool {
		return strings.Contains(s, "Workspace 1") && strings.Contains(s, "↑↓ select")
	})
	for _, want := range []string{"from the web", "Concert venues near the hall", "hxxps://example[.]org/venues"} {
		if !strings.Contains(scr, want) {
			t.Errorf("%q not in the overlay:\n%s", want, scr)
		}
	}
	// Enter opens the item; it scrolls; esc goes back, esc again closes.
	h.keys("\r")
	h.waitScreen("the detail", func(s string) bool { return strings.Contains(s, "↑↓ scroll") })
	h.keys("\x1b")
	h.waitScreen("the list", func(s string) bool { return strings.Contains(s, "↑↓ select") })
	h.keys("\x1b")
	h.waitScreen("closed", func(s string) bool {
		return !strings.Contains(s, "↑↓ select") && strings.Contains(s, "l workspace (1)")
	})
}

// A display is for the operator: it is not in the tool output that
// ctrl+t shows, and not in the transcript.
func TestDisplayStaysOutOfToolOutput(t *testing.T) {
	h := start(t, "workspace.jsonl", 80, 40)
	h.keys("go\r")
	h.waitIdle()
	h.keys("\x14") // ctrl+t
	all := h.waitScreen("the output in full", func(s string) bool { return strings.Contains(s, "search result text") })
	if strings.Contains(all, "Three venues are open late") {
		t.Errorf("the display's text is in the transcript:\n%s", all)
	}
}

// Against an agent without the extension methods the pane still works
// from what the client saw.
func TestWorkspaceWithoutRelay(t *testing.T) {
	h := start(t, "workspace_plain.jsonl", 140, 30)
	h.keys("go\r")
	h.waitIdle()
	scr := h.waitScreen("the display", func(s string) bool { return strings.Contains(s, "Concert venues near the hall") })
	if !strings.Contains(scr, "from the web") || strings.Contains(scr, "untrusted") {
		t.Errorf("no plain provenance:\n%s", scr)
	}
	h.escape()
	h.keys("l")
	h.waitScreen("focus", func(s string) bool { return strings.Contains(s, "↑↓ select") })
	// The first calls (connect, the display) can be out before the first
	// answer; after method-not-found it stops asking.
	if got := len(h.extensionCalls("_relay/items")); got < 1 || got > 3 {
		t.Errorf("%d items calls; after method-not-found it should stop asking:\n%s", got, h.agentLog())
	}
	if strings.Contains(h.agentLog(), "_relay/resolve") {
		t.Errorf("resolve asked after method-not-found:\n%s", h.agentLog())
	}
}

func lineWith(screen, sub string) string {
	for _, l := range strings.Split(screen, "\n") {
		if strings.Contains(l, sub) {
			return l
		}
	}
	return ""
}
