package core

import "slices"

// The client's state comes in two parts. The app is one per connection:
// what relay's own methods answer about the connection as a whole, such
// as the board, and the workspace pane that shows it. A conversation's
// State is one per conversation: its transcript, input, pastes,
// permission requests and its own pane items (what it showed and
// proposed, the docs it cited). The app is there before any conversation
// opens and stays when they end, so the board shows with no conversation
// open, and is the same board, with the same selection, whichever role is
// in focus.
//
// Hub mode holds the app in Hub, beside the roster and the conversations.
// Against a single agent, Single holds it beside the one conversation.

// App is the state of the connection, apart from its conversations.
type App struct {
	// Live says the connection can take relay's methods: initialize has
	// answered and, against a single agent, the session exists, since a
	// plain ACP agent may need one before it answers anything. It goes
	// false when the agent exits.
	Live bool
	// Board is relay's board, as this connection was last sent it, and
	// where the operator is on it. An agent with no board (a plain ACP
	// agent answers _relay/cards method-not-found) leaves it Unavailable,
	// and the pane shows the conversation's items instead.
	Board Board
	// PaneFocus says the workspace pane has the keys. On a screen too
	// narrow for the pane beside the conversation, it is then the screen.
	PaneFocus bool
}

// Single is the whole client state against one agent that is not relay's
// hub: the app, and the agent's one conversation. Their fields have no
// names in common, so each is reached as if it were Single's own.
type Single struct {
	App
	State
	// Restore is a saved view still on its way back (see view.go).
	Restore *Restore
}

// ReduceSingle applies one event. It never mutates x's slices in place,
// so a caller may keep the old state.
func ReduceSingle(x Single, e Event) (Single, []Effect) {
	rs := pending(x.Restore, e)
	a, s, fx := reduce(x.App, x.State, e, true)
	out := Single{App: a, State: s, Restore: rs}
	if rs != nil {
		out, fx = restoreSingle(out, e.at(), fx)
	}
	return out, fx
}

// Reduce applies one event to a conversation on its own, with no app
// around it: whatever the event does to the board or the pane is
// dropped. The shells use ReduceSingle and ReduceHub; this is for looking
// at a conversation alone.
func Reduce(s State, e Event) (State, []Effect) {
	_, s, fx := reduce(App{}, s, e, true)
	return s, fx
}

// reduce applies one event to a conversation and the app around it.
// shown says the conversation is the one on screen: only that one may
// move the pane between the board and its items, or take the keys from
// the pane.
func reduce(a App, s State, e Event, shown bool) (App, State, []Effect) {
	s.Entries = append([]Entry(nil), s.Entries...)
	for i := range s.Entries {
		if !s.Entries[i].Final { // a final entry's calls never change
			s.Entries[i].Calls = append([]Call(nil), s.Entries[i].Calls...)
		}
	}
	s.Queue = append([]string(nil), s.Queue...)
	s.Asks = append([]Permission(nil), s.Asks...)
	s.Pastes = append([]Paste(nil), s.Pastes...)
	s.Pane.Server = append([]Item(nil), s.Pane.Server...)
	s.Pane.Local = append([]Item(nil), s.Pane.Local...)
	s.Pane.Cites = append([]Citation(nil), s.Pane.Cites...)
	a.Board.Cards = slices.Clone(a.Board.Cards)
	r := &reducer{s: s, a: a, shown: shown}
	r.apply(e)
	return r.a, r.s, r.fx
}
