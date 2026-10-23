package render

import (
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/kirahowe/relay-tui/internal/core"
)

func txtc(s string, out bool) core.CardText { return core.CardText{Text: s, Outsider: out} }

// weekBoard is a board like the one the look was designed with: a note, a
// checklist and a tracker with a row name from outside.
func weekBoard() []core.Card {
	note := core.Card{ID: 12, Kind: core.CardNote, Status: core.CardActive, Rev: 12, Owner: "hub",
		Title: txtc("This week", false), Labels: core.Labels{Private: true},
		Note: &core.CardText{Text: "## Work\n1. Ship the parser rewrite ← doing now\n2. Review the onboarding doc\n\n## Home\n1. Book the furnace service\n\nThe rest can wait until **Monday**."}}
	check := core.Card{ID: 14, Kind: core.CardChecklist, Status: core.CardActive, Rev: 14, Owner: "clerk",
		Title: txtc("Trip prep", false), Done: 2, Total: 4, Labels: core.Labels{Private: true},
		Items: []core.CardElem{{ID: 1, CardText: txtc("Renew passport", false), Done: true},
			{ID: 2, CardText: txtc("Book the kennel", false), Done: true},
			{ID: 3, CardText: txtc("Pack the chargers", false)}, {ID: 4, CardText: txtc("Print the tickets", false)}}}
	bills := core.Card{ID: 15, Kind: core.CardTracker, Status: core.CardActive, Rev: 15, Owner: "clerk",
		Title: txtc("Monthly bills", false), Done: 7, Total: 9, Labels: core.Labels{Private: true, Contaminated: true},
		Rows: []core.CardElem{{ID: 1, CardText: txtc("Hydro", false)}, {ID: 2, CardText: txtc("Internet", false)},
			{ID: 3, CardText: txtc("Phone", true)}},
		Columns: []core.CardElem{{ID: 1, CardText: txtc("Aug", false)}, {ID: 2, CardText: txtc("Sep", false)}, {ID: 3, CardText: txtc("Oct", false)}},
		Cells: []core.Cell{{Row: 1, Column: 1, State: core.CellDone}, {Row: 1, Column: 2, State: core.CellDone},
			{Row: 2, Column: 1, State: core.CellDone}, {Row: 2, Column: 2, State: core.CellDone},
			{Row: 3, Column: 1, State: core.CellDone}, {Row: 3, Column: 2, State: core.CellSkip}}}
	return []core.Card{note, check, bills}
}

// tintedHub is the hub fixture with tools run, labels on the session and
// the board loaded, as a person would see it on a wide screen.
func tintedHub() core.Hub {
	h := hubFixture()
	return hubStep(h,
		core.Routed{Session: "s1", Event: core.ToolStarted{At: t0, ID: "t1", Name: "facts", Status: core.Running}},
		core.Routed{Session: "s1", Event: core.ToolStarted{At: t0, ID: "t2", Name: "card_add", Status: core.Running}},
		core.Routed{Session: "s1", Event: core.ToolUpdated{At: t0, ID: "t1", Status: ptr(core.OK)}},
		core.Routed{Session: "s1", Event: core.ToolUpdated{At: t0, ID: "t2", Status: ptr(core.OK)}},
		core.Routed{Session: "s1", Event: core.TextChunk{At: t0, Text: " I put **This week** on your board."}},
		core.CardsLoaded{At: t0, Rev: 15, Cards: weekBoard()},
		// The request in Mail brought the workspace items into view; the
		// operator went back to the board.
		core.PaneKey{At: t0, Action: core.PaneToggle}, core.PaneKey{At: t0, Action: core.PaneBoard},
		core.PaneKey{At: t0, Action: core.PaneToggle})
}

// The look with the terminal's colours known: tints behind the role in
// focus, the operator's words, the tool chips and the label pills.
func TestTintedGolden(t *testing.T) {
	SetTerminalColors(color.RGBA{0x1d, 0x1f, 0x23, 0xff}, color.RGBA{0xc8, 0xcc, 0xd4, 0xff})
	defer SetTerminalColors(nil, nil)
	h := tintedHub()
	for _, c := range []struct {
		name string
		h    core.Hub
	}{
		{"conversation", h},
		{"board_focused", hubStep(h, core.PaneKey{At: t0, Action: core.PaneToggle},
			core.PaneKey{At: t0, Action: core.PaneDown}, core.PaneKey{At: t0, Action: core.PaneDown})},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := HubView(c.h, "", "shift+enter for a new line", t0.Add(time.Minute), 170, 40)
			requireGolden(t, f.Content)
			if !strings.Contains(f.Content, "48;2;") {
				t.Error("no tint drawn")
			}
		})
	}
}
