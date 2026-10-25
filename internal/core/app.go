package core

import (
	"slices"
	"strings"
	"time"
)

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
	// Moving says the operator is moving about the screen; otherwise
	// they are typing in the conversation in focus. Only the
	// conversation can be typed in, so the pane, the list and having no
	// role in focus all mean moving (see settleMode). Letters move and
	// act while moving, and go into the input while typing.
	Moving bool
	// PasteSeq counts the pastes of this run, in every conversation, so
	// the latest can be told by when it came (Paste.Seq).
	PasteSeq int
	// Shown is the surface the shell last drew as active, while it still
	// is; KeyReplayed says the key being handled was held while
	// connecting and is replayed (see authority.go).
	Shown       Shown
	KeyReplayed bool
	// Pressed says the operator has pressed a key. Until then the start
	// (the saved view, --open, the roster) may move the keys as it likes;
	// after, only the operator does (see keeptyping.go).
	Pressed bool
}

// Events that move between typing and moving, and between the columns:
// the list of roles (hub mode only), the conversation, and the pane.
type (
	// Move goes Delta columns to the right (h is -1, l is +1). It stops
	// at the edge, and skips a column with nothing in it.
	Move struct {
		At    time.Time
		Delta int
	}
	// Type starts typing in the conversation in focus: i, or enter on
	// the conversation.
	Type struct{ At time.Time }
	// StopTyping is esc while typing: what was typed stays.
	StopTyping struct{ At time.Time }
	// Unbound is a letter that does nothing where the operator is: it
	// says how to type, so the letter is not lost without a word.
	Unbound struct{ At time.Time }
)

func (e Move) at() time.Time       { return e.At }
func (e Type) at() time.Time       { return e.At }
func (e StopTyping) at() time.Time { return e.At }
func (e Unbound) at() time.Time    { return e.At }

// UnboundNote is what the status line says for an Unbound key.
const UnboundNote = "i to type"

// EditorPasteNote is what the status line says for a paste while the
// board's editor is open. A card's text is the operator's own words, so
// a paste, which may be anyone's, does not go into it.
const EditorPasteNote = "a card's editor takes no paste: type the text"

// settleMode keeps the mode true to the column: anywhere but the
// conversation is moving.
func settleMode(a *App) {
	if a.PaneFocus {
		a.Moving = true
	}
}

// Single is the whole client state against one agent that is not relay's
// hub: the app, and the agent's one conversation. Their fields have no
// names in common, so each is reached as if it were Single's own.
type Single struct {
	App
	State
	// Draft is what is being written to the agent: the input's text, as
	// the shell last wrote it back, and the pastes it names. It is the
	// operator's, apart from the conversation.
	Draft Draft
	// Restore is a saved view still on its way back (see view.go).
	Restore *Restore
}

// ReduceSingle applies one event. It never mutates x's slices in place,
// so a caller may keep the old state. An operator's event that the
// surface does not take (see SingleSurface) is dropped whole.
func ReduceSingle(x Single, e Event) (Single, []Effect) {
	surf := SingleSurface(x)
	id := SingleSurfaceID(x)
	if !surf.takes(e) {
		return x, nil
	}
	if need := authorityOf(e, surf, x.State); !x.App.mayTake(need, id, e.at()) {
		x.State.Message = Message{Text: x.App.refusal(need, id), At: e.at()}
		return x, nil
	}
	if isAllow(e, x.State) && !inputArmed(x.State, e.at()) {
		x.State.Message = Message{Text: inputRefusal(x.State), At: e.at()}
		return x, nil
	}
	switch e := e.(type) {
	case Drawn:
		x.App = x.App.shown(e, id)
		if !e.OnlyForget {
			for _, d := range e.Input {
				seeInput(&x.State, d, e.At)
			}
		}
		return x, nil
	case KeyPressed:
		x.App.KeyReplayed, x.App.Pressed = e.Replayed, true
	case Pasted:
		if x.State.Phase == Exited || e.Text == "" {
			return x, nil
		}
		p := x.Draft.paste(&x.App, e.Text)
		return x, []Effect{InsertInput{Text: p.Token()}}
	case TrustPaste:
		if x.State.Phase == Exited {
			return x, nil
		}
		old, new, note := x.Draft.trust(e.Draft)
		x.State.Message = Message{Text: note, At: e.At}
		if note != "" {
			return x, nil
		}
		return x, []Effect{ReplaceInput{Old: old, New: new}}
	case Submit:
		if strings.TrimSpace(e.Text) != "" {
			e.Pastes = x.Draft.take(e.Text)
		}
		return reduceSingle(x, e, surf, id)
	}
	return reduceSingle(x, e, surf, id)
}

func reduceSingle(x Single, e Event, surf Surface, id string) (Single, []Effect) {
	if _, ok := e.(Unbound); ok && surf.Kind == SurfaceEditor {
		x.State.Message = Message{Text: EditorPasteNote, At: e.at()} // only a paste reaches the editor as Unbound
		return x, nil
	}
	rs := pending(x.Restore, e)
	a, s, fx := reduce(x.App, x.State, e, true)
	out := Single{App: a, State: s, Draft: x.Draft, Restore: rs}
	for _, f := range fx {
		if ri, ok := f.(RestoreInput); ok {
			out.Draft.giveBack(ri.Pastes) // a message not sent: its pastes are the draft's again
		}
	}
	if rs != nil {
		out, fx = restoreSingle(out, e.at(), fx)
	}
	settleMode(&out.App)
	if !fromOperator(e) && surf.Typing() && !SingleSurface(out).Typing() && x.App.Pressed {
		keepTypingSingle(&out, x, surf)
	}
	if SingleSurfaceID(out) != id {
		out.App.Shown = Shown{} // a new surface is armed only once drawn
	}
	if surf.Kind != SurfaceRequest && SingleSurface(out).Kind == SurfaceRequest {
		// The request has just become the surface: its delay starts now.
		out.State.Asks = append([]Permission(nil), out.State.Asks...)
		out.State.Asks[0].Since = e.at()
		out.State.Pick = DefaultPick(out.State.Asks[0].Options)
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
	s.Queue = slices.Clone(s.Queue)
	s.Asks = append([]Permission(nil), s.Asks...)
	s.Pane.Server = append([]Item(nil), s.Pane.Server...)
	s.Pane.Local = append([]Item(nil), s.Pane.Local...)
	s.Pane.Cites = append([]Citation(nil), s.Pane.Cites...)
	a.Board.Cards = slices.Clone(a.Board.Cards)
	r := &reducer{s: s, a: a, shown: shown}
	r.apply(e)
	return r.a, r.s, r.fx
}
