package ui_test

import (
	"strings"
	"testing"
	"time"
)

// Ctrl-O opens the transcript with the thinking in full while the agent
// works. Afterwards scrollback has the reply and the one-line marker,
// never the thinking itself.
func TestTranscriptView(t *testing.T) { bothSizes(t, testTranscriptView) }

func testTranscriptView(t *testing.T, cols, rows int) {
	h := start(t, "thinking.jsonl", cols, rows)
	h.waitIdle()
	h.keys("go\r")
	h.waitScreen("the marker", func(s string) bool { return strings.Contains(s, "∴ thinking") })
	h.keys("\x0f") // ctrl+o
	h.waitScreen("the thinking", func(s string) bool { return strings.Contains(s, "The page has") })
	// The turn goes on behind the view; leaving it prints what it held.
	time.Sleep(1200 * time.Millisecond)
	h.keys("q")
	h.waitIdle()
	h.waitAll("Fetched.")
	// Lines on their way to scrollback are in neither it nor the frame
	// for a moment, so wait for the marker to land.
	all := h.waitAll("∴ thought")
	if strings.Contains(all, "The page has") {
		t.Fatalf("the thinking reached the inline view:\n%s", all)
	}

	// Inside the view, each key flips its own kind. On the short screen
	// the thinking is above what shows.
	h.keys("\x14") // ctrl+t: open, tools in full; thinking stays as it was
	if rows < 24 {
		// The request's input, read with v, is below the call: up to it.
		h.waitScreen("the transcript", func(s string) bool { return strings.Contains(s, "esc back") })
		h.keys("kkkkkkkk")
	}
	h.waitScreen("both in full", func(s string) bool {
		return strings.Contains(s, "url: https") && (rows < 24 || strings.Contains(s, "The page has"))
	})
	h.keys("o")
	h.waitScreen("thinking hidden", func(s string) bool {
		return !strings.Contains(s, "The page has") && strings.Contains(s, "url: https")
	})
	scroll := strings.Join(h.emu.scrollback(), "\n")
	h.keys("q")
	h.waitIdle()
	if again := strings.Join(h.emu.scrollback(), "\n"); again != scroll {
		t.Fatalf("scrollback changed:\n%s\n---\n%s", scroll, again)
	}

	// The next turn still reaches scrollback, in order, once.
	// Lines on their way to scrollback are in neither it nor the frame
	// for a moment, so wait for them to land.
	h.keys("again\r")
	h.wait("the next turn after the first", h.all, func(all string) bool {
		f, a, s := strings.Index(all, "Fetched."), strings.Index(all, "› again"), strings.Index(all, "Second.")
		return count(all, "Fetched.") == 1 && f >= 0 && f < a && a < s
	})
}

// ? on an empty input shows the keys; any key closes the list and does
// nothing else, since the list covered what it would act on.
func TestHelpOverlay(t *testing.T) {
	h := start(t, "simple.jsonl", 80, 24)
	h.waitIdle()
	h.keys("?")
	h.waitScreen("the keys", func(s string) bool { return strings.Contains(s, "any key closes this") })
	h.keys("x")
	h.waitScreen("closed", func(s string) bool { return !strings.Contains(s, "any key closes this") })
	if strings.Contains(h.screen(), "› x") {
		t.Errorf("the key that closed the list was typed:\n%s", h.screen())
	}
	h.keys("h?")
	h.waitScreen("a ? typed after text", func(s string) bool {
		return strings.Contains(s, "› h?") && !strings.Contains(s, "any key closes this")
	})
}

// Bubble Tea would write a print into the alternate screen, where it is
// lost, so nothing prints while the view is open. What the agent wrote
// meanwhile reaches scrollback once the view closes, once and in order.
func TestNothingLostBehindTranscript(t *testing.T) {
	h := start(t, "tool_burst.jsonl", 40, 10)
	h.waitIdle()
	h.keys("go\r\x14")
	h.waitScreen("the end, in the view", func(s string) bool {
		return strings.Contains(s, "All thirty read.") && strings.Contains(s, "transcript")
	})
	h.keys("\x1b")
	h.wait("everything once, in order", h.all, func(all string) bool {
		c, r, a := strings.Index(all, "Checking everything."), strings.Index(all, "worked"), strings.Index(all, "All thirty read.")
		return count(all, "Checking everything.") == 1 && count(all, "All thirty read.") == 1 && c < r && r < a && c >= 0
	})
}

// Ctrl-D in the view still leaves everything in scrollback.
func TestQuitFromTranscriptKeepsScrollback(t *testing.T) {
	h := start(t, "tool_burst.jsonl", 40, 10)
	h.waitIdle()
	h.keys("go\r\x14")
	h.waitScreen("the end, in the view", func(s string) bool {
		return strings.Contains(s, "All thirty read.") && strings.Contains(s, "transcript")
	})
	h.keys("\x04")
	select {
	case <-h.done:
	case <-time.After(5 * time.Second):
		t.Fatal("did not quit")
	}
	all := h.all()
	if count(all, "Checking everything.") != 1 || count(all, "All thirty read.") != 1 || !strings.Contains(all, "› go") {
		t.Fatalf("after quitting:\n%s", all)
	}
}
