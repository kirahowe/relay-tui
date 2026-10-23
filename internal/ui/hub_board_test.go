package ui_test

// The board in hub mode, end to end: the real ui.Model and
// internal/agent against the fake hub with a board. The board is the
// connection's, so it shows, and works, with no conversation open.

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kirahowe/relay-tui/internal/core"
	"github.com/kirahowe/relay-tui/internal/fakehub"
)

const (
	cardTrip = `{"id": 14, "kind": "checklist", "status": "active", "complete": false, "rev": 14, "owner": "clerk",
		"title": {"text": "Trip prep", "outsider": false, "rev": 14}, "progress": {"done": 1, "total": 3}, "labels": ["private"],
		"items": [{"id": 1, "text": "Renew passport", "outsider": false, "rev": 14, "done": true},
		          {"id": 2, "text": "Book the kennel", "outsider": false, "rev": 14, "done": false},
		          {"id": 3, "text": "Print the tickets", "outsider": false, "rev": 14, "done": false}], "text": "Card 14"}`
	cardStatements = `{"id": 12, "kind": "note", "status": "active", "complete": false, "rev": 12, "owner": "operator",
		"title": {"text": "Where the statements are", "outsider": false, "rev": 12}, "labels": ["private"],
		"note": {"text": "BMO: online banking.", "outsider": false, "rev": 12}, "text": "Card 12"}`
	cardOutside = `{"id": 9, "kind": "note", "status": "active", "complete": false, "rev": 9, "owner": "hub",
		"sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"title": {"text": "From a web page", "outsider": true, "rev": 9}, "labels": ["contaminated", "private"],
		"note": {"text": "Read hxxps://evil[.]test/x", "outsider": true, "rev": 9}, "text": "Card 9"}`
)

// boardRoles is threeRoles with a board of three cards.
func boardRoles() fakehub.Config {
	cfg := threeRoles()
	for _, c := range []string{cardTrip, cardStatements, cardOutside} {
		cfg.Cards = append(cfg.Cards, json.RawMessage(c))
	}
	return cfg
}

// The bug: the board belonged to a conversation, so with none open there
// was no board. Here none is ever opened.
func TestHubBoardShowsBeforeAnyConversation(t *testing.T) {
	h := startHub(t, boardRoles(), 170, 40, "")
	s := h.waitScreen("the board beside the roles", contains("Board 3"))
	for _, want := range []string{"Trip prep", "Where the statements are", "From a web page", "News"} {
		if !strings.Contains(s, want) {
			t.Errorf("%q not on screen:\n%s", want, s)
		}
	}
	calls := h.logged("cards ")
	if len(calls) != 1 || calls[0] != "cards {}" {
		t.Errorf("cards calls %q", calls)
	}
	if opened := append(h.logged("new "), h.logged("load ")...); len(opened) != 0 {
		t.Errorf("a conversation was opened: %q", opened)
	}
}

// A role with no conversation, the pane given the keys: the board's ops
// and its changes from elsewhere work with no conversation open, and
// the board keeps its selection whichever role is in focus.
func TestHubBoardWorksWithNoConversationOpen(t *testing.T) {
	h := startHub(t, boardRoles(), 170, 40, "")
	h.waitScreen("the board", contains("Board 3"))
	h.keys("\x1b2") // alt+2: Mail, which has no conversation
	h.waitScreen("mail", contains("No conversation with Mail"))
	h.keys(ctrlX)
	h.waitScreen("the pane has the keys", contains("↑↓ select"))
	h.keys("\r") // Trip prep
	h.waitScreen("the checklist", contains("x tick"))
	h.keys("jj") // Book the kennel
	h.waitScreen("item 2", func(string) bool { return h.emu.selected("Book the kennel") })
	h.keys("x")
	h.waitScreen("the tick", contains("✓ Book the kennel"))
	if calls := h.logged("card-op "); len(calls) != 1 || calls[0] != `card-op {"card":14,"item":2,"op":"tick"}` {
		t.Errorf("op calls %q", calls)
	}
	// A card archived elsewhere leaves the board, with nothing open.
	h.keys("\x1b") // back to the list of cards
	h.waitScreen("the list", contains("enter open"))
	h.say("archive-card 12")
	s := h.waitScreen("the change", contains("Board 2"))
	if strings.Contains(s, "Where the statements are") {
		t.Errorf("the archived card is still on the board:\n%s", s)
	}
	if opened := append(h.logged("new "), h.logged("load ")...); len(opened) != 0 {
		t.Errorf("a conversation was opened: %q", opened)
	}
	// The same board on Dev, whose conversation opens: the selection is
	// where it was, so enter opens Trip prep.
	h.keys("\x1b1") // alt+1: Dev
	h.waitScreen("dev", contains("hello from dev"))
	h.keys(ctrlX)
	h.waitScreen("dev's pane", contains("↑↓ select"))
	h.keys("\r")
	h.waitScreen("Trip prep", contains("✓ Book the kennel"))
	if n := len(h.logged("cards ")); n != 1 {
		t.Errorf("%d cards calls", n)
	}
}

// t, a moment, then y trusts a card with no conversation open: relay is
// sent the rev and digest the operator was shown.
func TestHubTrustsACardWithNoConversationOpen(t *testing.T) {
	h := startHub(t, boardRoles(), 170, 40, "")
	h.waitScreen("the board", contains("From a web page"))
	h.keys("\x1b3") // alt+3: News, no conversation
	h.waitScreen("news", contains("No conversation with News"))
	h.keys(ctrlX + "jj")
	h.waitScreen("the outside card", contains("t trust"))
	h.keys("t")
	h.waitScreen("the question", contains("It clears outside"))
	time.Sleep(core.ArmDelay + 100*time.Millisecond)
	h.waitScreen("armed", func(s string) bool { return !strings.Contains(s, "wait a moment") })
	h.keys("y")
	h.waitScreen("trusted", func(s string) bool { return strings.Contains(s, "Trusted.") && !strings.Contains(s, "outside text") })
	calls := h.logged("card-trust ")
	if len(calls) != 1 || calls[0] != `card-trust {"card":9,"rev":9,"sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}` {
		t.Errorf("trust calls %q", calls)
	}
}

// With no role in focus, ctrl+x from the list gives the board the keys:
// a card is ticked with no role focused, and esc goes back to the list.
func TestHubListHandsTheKeysToTheBoard(t *testing.T) {
	h := startHub(t, boardRoles(), 170, 40, "")
	h.waitScreen("the board beside the list", contains("ctrl+x board"))
	h.keys(ctrlX)
	h.waitScreen("the pane has the keys", contains("↑↓ select"))
	h.keys("\r")
	h.waitScreen("the checklist", contains("x tick"))
	h.keys("jj")
	h.waitScreen("item 2", func(string) bool { return h.emu.selected("Book the kennel") })
	h.keys("x")
	h.waitScreen("the tick", contains("✓ Book the kennel"))
	if calls := h.logged("card-op "); len(calls) != 1 || calls[0] != `card-op {"card":14,"item":2,"op":"tick"}` {
		t.Errorf("op calls %q", calls)
	}
	h.keys("\x1b")
	h.waitScreen("the list of cards", contains("enter open"))
	h.keys("\x1b")
	h.waitScreen("the list has the keys", contains("ctrl+x board"))
	h.keys("j\r") // the list moves and opens: the keys are the list's
	h.waitScreen("mail", contains("No conversation with Mail"))
	if opened := append(h.logged("new "), h.logged("load ")...); len(opened) != 0 {
		t.Errorf("a conversation was opened: %q", opened)
	}
}

// On a narrow screen the list is the screen, so the board opened from it
// takes its place, and esc brings the list back.
func TestHubNarrowListOpensTheBoardInItsPlace(t *testing.T) {
	h := startHub(t, boardRoles(), 60, 24, "")
	h.waitScreen("the list", contains("News"))
	h.keys(ctrlX)
	s := h.waitScreen("the board", contains("Trip prep"))
	if strings.Contains(s, "News") {
		t.Errorf("the list is still on screen:\n%s", s)
	}
	h.keys("\x1b")
	h.waitScreen("the list again", contains("News"))
}
