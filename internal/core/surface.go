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
// Two rules follow from deciding it in one place. Typing (the
// conversation's input, or the board's one-line editor) is a surface of
// its own, so a request never takes keys while text is being entered,
// and a request arriving does not move the keys or close an editor: it
// waits, and says so, until the operator moves to its conversation. A
// request is answered only while it is the surface, and its delay starts
// when it becomes the surface (see ReduceHub and ReduceSingle).

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
	// SurfaceRequest is the permission request on offer in the
	// conversation on screen, while moving.
	SurfaceRequest
	// SurfaceConfirm is a question the pane asks: archive a card, trust a
	// card or a doc. Its next key answers or cancels it.
	SurfaceConfirm
	// SurfacePane is the workspace pane (the board or the items), moving.
	SurfacePane
	// SurfaceList is the list of roles (hub mode only).
	SurfaceList
	// SurfaceEnd is the question whether to end the conversation.
	SurfaceEnd
	// SurfaceHelp is the list of keys, an overlay. Any key closes it.
	SurfaceHelp
	// SurfaceTranscript is the full-screen transcript, an overlay
	// (against a single agent; in hub mode the conversation is always
	// drawn whole).
	SurfaceTranscript
	// SurfaceInputView is the input of the request on offer, opened with
	// v in the request's place, to read all of it.
	SurfaceInputView
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
func (s Surface) Covered() Surface { return Surface{Kind: s.Under, Under: s.Under, Column: s.Column} }

// Typing says the keys type text: into the conversation's input or the
// board's editor. Nothing typed answers, confirms or trusts anything.
func (s Surface) Typing() bool { return s.Kind == SurfaceInput || s.Kind == SurfaceEditor }

// HubSurface is what has the keys in hub mode.
func HubSurface(h Hub) Surface {
	col := h.col()
	var st State
	if c := h.Focused(); c != nil && col != ColumnList {
		st = c.State
	}
	under := SurfaceConversation
	switch {
	case col == ColumnList:
		under = SurfaceList
	case col == ColumnPane:
		under = paneSurface(h.App, st)
	case !h.App.Moving:
		under = SurfaceInput
	case st.viewing():
		under = SurfaceInputView
	case len(st.Asks) > 0:
		under = SurfaceRequest
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
		col, under = ColumnPane, paneSurface(x.App, x.State)
	case !x.App.Moving:
		under = SurfaceInput
	case x.State.viewing():
		under = SurfaceInputView
	case len(x.State.Asks) > 0:
		under = SurfaceRequest
	}
	s := Surface{Kind: under, Under: under, Column: col}
	if x.State.Help {
		s.Kind = SurfaceHelp
	}
	return s
}

// paneSurface is what has the keys in the pane: its editor, a question
// it asks, or the pane itself, in the order the pane takes a key (see
// paneKey and boardKey).
func paneSurface(a App, s State) SurfaceKind {
	b := a.Board
	switch {
	case s.Pane.Trust != nil && !s.Pane.Trust.Sent:
		return SurfaceConfirm
	case b.Items:
		return SurfacePane
	case b.Input != nil:
		return SurfaceEditor
	case b.Confirm != 0 || (b.Trust != nil && !b.Trust.Sent):
		return SurfaceConfirm
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
	case Submit, Pasted, TrustPaste, StartConversation, StopTyping:
		return k == SurfaceInput
	case PermissionMove, PermissionConfirm, PermissionQuick, PermissionCancel, PermissionShowAll:
		return k == SurfaceRequest
	case PaneKey:
		if e.Action == PaneToggle {
			// Into or out of the pane, as a column: the shell sends Move,
			// which is this from the conversation. It types nothing and
			// answers nothing, and the question or editor in the pane
			// keeps the keys.
			return in(SurfaceConversation, SurfaceInput, SurfaceList, SurfacePane, SurfaceRequest)
		}
		return in(SurfacePane, SurfaceEditor, SurfaceConfirm)
	case ListMove, ListOpen:
		return k == SurfaceList
	case InputViewKey:
		return k == SurfaceInputView
	case AskEnd:
		return k == SurfaceConversation
	case ConfirmEnd:
		return k == SurfaceEnd
	case Move:
		return in(SurfaceConversation, SurfaceList, SurfacePane, SurfaceRequest)
	case Type:
		return in(SurfaceConversation, SurfaceList, SurfacePane)
	case FocusIndex:
		return in(SurfaceConversation, SurfaceInput, SurfaceList, SurfacePane, SurfaceRequest)
	case ShowTools, ShowThinking:
		return in(SurfaceConversation, SurfaceInput, SurfaceTranscript)
	case TranscriptKey:
		if e.Action == TranscriptClose {
			return k == SurfaceTranscript
		}
		// The wheel scrolls the conversation wherever the keys are.
		return !in(SurfaceHelp, SurfaceEnd, SurfaceEditor, SurfaceConfirm)
	case Interrupt:
		return !in(SurfaceHelp, SurfaceEnd)
	case EOF:
		return !in(SurfaceHelp, SurfaceEnd, SurfaceEditor, SurfaceConfirm)
	}
	return true
}
