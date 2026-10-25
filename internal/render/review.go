package render

import (
	"fmt"
	"slices"
	"strings"
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
func requestBox(a core.App, s core.State, now time.Time, width, room, fits int) ([]string, string, []core.InputDrawn) {
	full, in := permissionBox(s, core.RequestArmed(a, s, now), width, room, true)
	if len(full) <= min(room, fits) {
		return full, core.RequestID(s), in
	}
	short, in := permissionBox(s, false, width, room, false)
	return short, "", in
}

// largerNote says a review does not fit.
const largerNote = "not all of it fits: make the window larger to answer · esc cancel"

// inputRows is a request's input as shown, wrapped to width and never
// cut: its rows, the part of the input each row holds, and the length of
// each line of the input as shown (see core.InputDrawn).
func inputRows(p core.Permission, def bool, width int) (rows []string, spans []core.Span, lens []int) {
	for i, l := range defangIf(def, core.FaithfulLines(p.Input)) {
		lens = append(lens, len([]rune(l)))
		lead := len(l) - len(strings.TrimLeft(l, " "))
		at := 0
		for j, w := range hardwrap(l, max(width-lead, 1)) {
			n := len([]rune(w))
			spans = append(spans, core.Span{Line: i, From: at, To: at + n})
			at += n
			if j > 0 || lead > 0 {
				w = strings.Repeat(" ", lead) + w // as wrapAll indents
			}
			rows = append(rows, w)
		}
	}
	return rows, spans, lens
}

// drawnInput is what of the request's input rows from to to are.
func drawnInput(s core.State, spans []core.Span, lens []int, from, to int) []core.InputDrawn {
	if len(lens) == 0 || from >= to {
		return nil
	}
	return []core.InputDrawn{{Request: core.RequestID(s), Lens: lens, Spans: slices.Clone(spans[from:to])}}
}

// inputView is the input view (v): the request's input whole, a screen
// at a time, wrapped and never cut, height rows. It gives what it drew,
// how far it scrolls and a page.
func inputView(s core.State, width, height int) ([]string, []core.InputDrawn, int, int) {
	p := s.Asks[0]
	def := p.Shown().Union(s.Labels).Contaminated
	rows, spans, lens := inputRows(p, def, max(width, 1))
	head := styled(bold, truncate("input of "+core.EscapeLabel(p.Label()), width))
	foot := styled(faint, truncate("↑↓ pgup pgdown g G scroll · v back", width))
	room := max(height-2, 1)
	mx := max(len(rows)-room, 0)
	scroll := min(max(s.InputView.Scroll, 0), mx)
	end := min(scroll+room, len(rows))
	out := []string{head}
	for _, r := range rows[scroll:end] {
		out = append(out, styled(faint, r))
	}
	if len(rows) == 0 {
		out = append(out, styled(faint, "(no input)"))
	}
	for len(out) < height-1 {
		out = append(out, "")
	}
	out = append(out[:max(height-1, 0)], foot)
	if height-2 < 1 {
		return out[:max(height, 0)], nil, mx, room
	}
	return out, drawnInput(s, spans, lens, scroll, end), mx, max(room-1, 1)
}
