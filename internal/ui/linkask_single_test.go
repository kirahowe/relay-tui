package ui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kirahowe/relay-tui/internal/core"
)

// Against a single agent, a yes to a link's question counted even when
// the screen did not show the link's host: the check asked what the hub
// had on screen, and a single agent has no hub. Review round five of
// relay#110 found it.
func TestASingleAgentsHiddenLinkQuestionTakesNoYes(t *testing.T) {
	c := tallNote(1, 1)
	c.Note.Text, c.Note.Outsider = "see hxxps://two[.]example/", true
	c.Links = []core.Link{{Address: "https://two.example/", Shown: "hxxps://two[.]example/"}}
	var ops []core.CardOp
	m := tallSingle(t, &ops, 140, 30, c)
	now := m.now()
	m.clock = func() time.Time { return now }
	m = update(m, tea.KeyPressMsg{Code: 'L', Text: "L", Mod: tea.ModShift}, tea.KeyPressMsg{Code: tea.KeyEnter})
	if core.SingleSurface(m.one).Kind != core.SurfaceLinkAsk {
		t.Fatalf("no question about the link:\n%s", screen(m))
	}
	// The answer as the shell builds it, with the question off the
	// screen: a terminal that small takes no key at all now, so the
	// check is asked for the yes directly.
	m = update(m, tea.WindowSizeMsg{Width: 140, Height: 2})
	if m.paneAskShown(now) {
		t.Fatalf("the test needs the question off the screen:\n%s", screen(m))
	}
	if e, ok := m.archiveKey(letter('y'), now).(core.PaneKey); !ok || !e.Hidden {
		t.Fatalf("a yes to a hidden question is not marked hidden: %+v", e)
	}
}
