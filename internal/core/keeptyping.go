package core

import (
	"fmt"
	"slices"
)

// Only the operator moves the keys away from what they are typing in. An
// event that is not theirs (a card changed, archived or gone, the roster
// changing or a role removed, a conversation ending, a request arriving,
// a replay or an attach) never turns a typing surface into one that is
// not, so the next letters are never taken as commands. ReduceHub and
// ReduceSingle check it after every such event and, if the event moved
// the keys, put back where they were: the column, the mode, and the
// board's editor with its text. What the editor or the input was for may
// have gone; they say so and keep the text until the operator sends or
// discards it.

// editorGoneNote says the editor's card has left the board.
func editorGoneNote(card int64) string {
	return fmt.Sprintf("card %d left the board · enter does nothing · esc to discard", card)
}

// keepEditor puts back the board's editor in a, as it was in was, and
// says if its card has gone.
func keepEditor(a *App, was App) {
	b := &a.Board
	b.Items = false
	if b.Input == nil {
		b.Input = was.Board.Input
	}
	if in := b.Input; in != nil && !slices.ContainsFunc(b.Cards, func(c Card) bool { return c.ID == in.Card }) {
		b.Message, b.Failure = editorGoneNote(in.Card), ""
	}
}

// keepTypingHub puts the keys back where they were in was, a hub with a
// typing surface, after an event that was not the operator's took them.
func keepTypingHub(h *Hub, was Hub, surf Surface) {
	h.Focus, h.List, h.Help, h.EndAsk = was.Focus, was.List, false, false
	h.App.PaneFocus, h.App.Moving = was.App.PaneFocus, was.App.Moving
	if surf.Kind == SurfaceEditor {
		keepEditor(&h.App, was.App)
	}
	if surf.Kind == SurfaceInput && h.RoleByID(h.Focus) == nil && h.RosterLoaded {
		h.Notice = "no role " + oneLineText(h.Focus) + " any more: what you typed stays · esc to move"
	}
}

// keepTypingSingle is keepTypingHub against a single agent.
func keepTypingSingle(x *Single, was Single, surf Surface) {
	x.App.PaneFocus, x.App.Moving = was.App.PaneFocus, was.App.Moving
	x.State.Help = false
	if surf.Kind == SurfaceEditor {
		keepEditor(&x.App, was.App)
	}
}
