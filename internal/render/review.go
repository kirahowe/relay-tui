package render

import (
	"fmt"
	"time"

	"github.com/kirahowe/relay-tui/internal/core"
)

// A review is what a key can authorize, drawn so nothing it covers is
// lost: every field of its subject, wrapped, never cut, never ended with
// an ellipsis. Only a review arms anything: the request box with its
// heading, preview and every option in full; a card's trust as a plain
// list of every text the card holds; a doc's trust with its title and
// text; the archive question with the card's title; the end question.
// The views that shorten (the tracker grid, card panels, the capped
// request box) are drawn elsewhere and when a review does not fit, and
// never arm. A review arms only if the whole of it is on screen.

// cardReview is every text of the card, as relay sent it, one after the
// other: its title, then its items, rows and columns, or its note.
func cardReview(c core.Card, width int) []string {
	var out []string
	text := func(label string, t core.CardText) {
		out = append(out, styled(faint, label))
		for _, l := range cardLines(t.Text, t.Outsider, max(width-2, 1)) {
			out = append(out, "  "+outside(t, l))
		}
	}
	text("title", c.Title)
	elems := func(label string, es []core.CardElem) {
		for _, e := range es {
			text(fmt.Sprintf("%s %d", label, e.ID), e.CardText)
		}
	}
	elems("item", c.Items)
	elems("row", c.Rows)
	elems("column", c.Columns)
	if c.Note != nil {
		text("note", *c.Note)
	}
	if c.Text != "" {
		text("text", core.CardText{Text: c.Text, Outsider: c.Labels.Contaminated})
	}
	return out
}

// requestBox is the request on offer, and whether it is drawn as its
// review, which alone arms it: heading, preview and every option whole.
// When that does not fit in fits rows the box is drawn shortened to
// room, as before, says to make the window larger, and arms nothing.
func requestBox(a core.App, s core.State, now time.Time, width, room, fits int) ([]string, string) {
	full := permissionBox(s, core.RequestArmed(a, s, now), width, room, true)
	if len(full) <= min(room, fits) {
		return full, core.RequestID(s)
	}
	return permissionBox(s, false, width, room, false), ""
}

// largerNote says a review does not fit.
const largerNote = "not all of it fits: make the window larger to answer · esc cancel"
