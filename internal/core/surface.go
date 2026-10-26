package core

// The surface is what has the keys: one thing on the screen, drawn as
// the active one, that the next key goes to. HubSurface and
// SingleSurface are the only place that decides it. The shell classifies
// a key against the surface's own keys and nothing else, the renderer
// draws that surface as the active one (an overlay only when it is the
// surface, or lies under the one that is), and the reducers drop an
// operator's event the surface does not take, so a key routed wrongly
// still cannot act anywhere else.
//
// Typing (the conversation's input, or the board's one-line editor) is a
// surface of its own, so nothing that arrives moves the keys or closes
// an editor.

// SurfaceKind is the kind of thing that has the keys.
type SurfaceKind int

const (
	// SurfaceConversation is the conversation column, moving: the keys
	// move about, scroll and open things.
	SurfaceConversation SurfaceKind = iota
	// SurfaceInput is the conversation's input, typing.
	SurfaceInput
	// SurfaceEditor is the board's one-line editor (a card's title, an
	// item's text), typing.
	SurfaceEditor
	// SurfacePane is the workspace pane (the board or the items), moving.
	SurfacePane
	// SurfaceList is the list of roles (hub mode only).
	SurfaceList
	// SurfaceEnd is the question whether to end the conversation.
	SurfaceEnd
	// SurfaceArchive is the question whether to archive the selected
	// card, asked in the pane.
	SurfaceArchive
	// SurfaceHelp is the list of keys, an overlay. Any key closes it.
	SurfaceHelp
	// SurfaceTranscript is the full-screen transcript, an overlay
	// (against a single agent; in hub mode the conversation is always
	// drawn whole).
	SurfaceTranscript
)

// Column is the column of the screen the keys are in: the list of
// roles, the conversation, or the pane.
type Column int

const (
	ColumnList Column = iota
	ColumnConversation
	ColumnPane
)

// Surface is what has the keys, and where. Under is the surface the help
// covers, when Kind is SurfaceHelp (and the same as Kind otherwise), so
// the help can list the keys of what it covers. Column is where the keys
// are, under any overlay.
type Surface struct {
	Kind, Under SurfaceKind
	Column      Column
}

// Covered is the surface the help covers, or s itself.
func (s Surface) Covered() Surface {
	return Surface{Kind: s.Under, Under: s.Under, Column: s.Column}
}

// Typing says the keys type text: into the conversation's input or the
// board's editor. Nothing typed answers anything.
func (s Surface) Typing() bool { return s.Kind == SurfaceInput || s.Kind == SurfaceEditor }

// HubSurface is what has the keys in hub mode.
func HubSurface(h Hub) Surface {
	col := h.col()
	under := SurfaceConversation
	switch {
	case col == ColumnList:
		under = SurfaceList
	case col == ColumnPane:
		under = paneSurface(h.App)
	case !h.App.Moving:
		under = SurfaceInput
	}
	s := Surface{Kind: under, Under: under, Column: col}
	switch {
	case h.Help:
		s.Kind = SurfaceHelp
	case h.EndAsk:
		s.Kind, s.Under = SurfaceEnd, SurfaceEnd
	}
	return s
}

// SingleSurface is what has the keys against a single agent: there is
// no list, and the transcript is an overlay.
func SingleSurface(x Single) Surface {
	col := ColumnConversation
	under := SurfaceConversation
	switch {
	case x.State.Transcript.Open && !x.State.Shared:
		under = SurfaceTranscript
	case x.App.PaneFocus:
		col = ColumnPane
		under = paneSurface(x.App)
	case !x.App.Moving:
		under = SurfaceInput
	}
	s := Surface{Kind: under, Under: under, Column: col}
	if x.State.Help {
		s.Kind = SurfaceHelp
	}
	return s
}

// paneSurface is what has the keys in the pane: its editor, the archive
// question, or the pane itself, in the order the pane takes a key (see
// paneKey and boardKey).
func paneSurface(a App) SurfaceKind {
	b := a.Board
	switch {
	case b.Items:
		return SurfacePane
	case b.Input != nil:
		return SurfaceEditor
	case b.Confirm != 0:
		return SurfaceArchive
	}
	return SurfacePane
}

// takes says the surface takes an event of the operator's. Every other
// event (the agent's, the shell's) is taken whatever has the keys. Any
// key counts as one (KeyPressed), and Unbound only puts a note on the
// status line.
func (s Surface) takes(e Event) bool {
	k := s.Kind
	in := func(kinds ...SurfaceKind) bool {
		for _, x := range kinds {
			if x == k {
				return true
			}
		}
		return false
	}
	switch e := e.(type) {
	case KeyPressed, Unbound:
		return true
	case ToggleHelp:
		return in(SurfaceHelp, SurfaceInput, SurfaceConversation, SurfaceList, SurfacePane, SurfaceTranscript)
	case Submit, Pasted, StartConversation, StopTyping:
		return k == SurfaceInput
	case PaneKey:
		if e.Action == PaneToggle {
			// Into or out of the pane, as a column: the shell sends Move,
			// which is this from the conversation. It types nothing and
			// answers nothing, and the question or editor in the pane
			// keeps the keys.
			return in(SurfaceConversation, SurfaceInput, SurfaceList, SurfacePane)
		}
		// The archive question answers y, and cancels on any other key.
		return in(SurfacePane, SurfaceEditor, SurfaceArchive)
	case ListMove, ListOpen:
		return k == SurfaceList
	case AskEnd:
		return k == SurfaceConversation
	case ConfirmEnd:
		return k == SurfaceEnd
	case Move:
		return in(SurfaceConversation, SurfaceList, SurfacePane)
	case Type:
		return in(SurfaceConversation, SurfaceList, SurfacePane)
	case FocusIndex:
		return in(SurfaceConversation, SurfaceInput, SurfaceList, SurfacePane)
	case ShowTools, ShowThinking:
		return in(SurfaceConversation, SurfaceInput, SurfaceTranscript)
	case TranscriptKey:
		if e.Action == TranscriptClose {
			return k == SurfaceTranscript
		}
		// The wheel scrolls the conversation wherever the keys are, but
		// under the questions and the editor, which have the keys.
		return !in(SurfaceHelp, SurfaceEnd, SurfaceEditor, SurfaceArchive)
	case Interrupt:
		return !in(SurfaceHelp, SurfaceEnd)
	case EOF:
		return !in(SurfaceHelp, SurfaceEnd, SurfaceEditor, SurfaceArchive)
	}
	return true
}
