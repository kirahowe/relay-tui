package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/kirahowe/relay-tui/internal/core"
)

// Caps is what the terminal told us about its keyboard.
type Caps struct {
	// Disambiguation means the kitty keyboard protocol is on, so
	// shift+enter reaches us as itself.
	Disambiguation bool
}

// Action is what a key press does while typing.
type Action int

const (
	PassThrough  Action = iota // to the input box
	Send                       // submit the input (the core queues it when busy)
	Newline                    // insert a newline into the input
	Interrupt                  // Ctrl-C
	EOF                        // Ctrl-D on an empty input
	ShowTools                  // Ctrl-T: tool calls in full
	ShowThinking               // Ctrl-O: thinking in full
	Help                       // ? on an empty input: the list of keys
	StopTyping                 // Esc: start moving; what was typed stays
	Jump                       // alt+1 to alt+9: that role, still typing
	Move                       // in the transcript: scroll, or leave
)

// Classify decides what a key press does while typing. It is pure, and
// the index is set for Jump. Only these keys are taken from the input:
// everything else is typed.
//
// Enter always sends. Four keys insert a newline, so one of them works in
// any terminal: shift+enter and ctrl+enter need the kitty protocol or
// xterm's modifyOtherKeys (Bubble Tea asks for both), alt+enter works in
// most terminals, and ctrl+j (a bare line feed) works everywhere.
func Classify(k tea.KeyPressMsg, inputEmpty bool) (Action, int) {
	switch s := k.String(); s {
	case "enter":
		return Send, 0
	case "shift+enter", "ctrl+enter", "alt+enter", "ctrl+j":
		return Newline, 0
	case "esc":
		return StopTyping, 0
	case "ctrl+c":
		return Interrupt, 0
	case "ctrl+t":
		return ShowTools, 0
	case "ctrl+o":
		return ShowThinking, 0
	case "?":
		if inputEmpty {
			return Help, 0
		}
	case "ctrl+d":
		if inputEmpty {
			return EOF, 0
		}
	default:
		if n, ok := strings.CutPrefix(s, "alt+"); ok && digit(n) {
			return Jump, int(n[0] - '1')
		}
	}
	return PassThrough, 0
}

// digit says s is one of 1 to 9.
func digit(s string) bool { return len(s) == 1 && s[0] >= '1' && s[0] <= '9' }

// MoveAction is what a key press does while moving.
type MoveAction int

const (
	MoveNone      MoveAction = iota // nothing: a letter says "i to type"
	MoveLeft                        // h, left: the column to the left
	MoveRight                       // l, right: the column to the right
	MoveUp                          // k, up
	MoveDown                        // j, down
	MovePageUp                      // pgup
	MovePageDown                    // pgdown
	MoveTop                         // g, home
	MoveBottom                      // G, end
	MoveEnter                       // enter: open, type, or act
	MoveType                        // i: type in the conversation
	MoveJump                        // 1 to 9, alt+1 to alt+9: that role
	MoveEsc                         // esc: back out of what is open
	MoveEnd                         // X on the conversation: end it, after asking
	MoveTools                       // ctrl+t
	MoveThinking                    // ctrl+o
	MoveHelp                        // ?
	MoveReady                       // R: what is ready for you (hub)
	MoveInterrupt                   // ctrl+c: stop the turn, or quit
	MoveEOF                         // ctrl+d: quit
)

// Global says the key acts whichever column has the keys; the others
// mean something in the list or the conversation, and give way to the
// pane's own keys when the pane has them.
func (a MoveAction) Global() bool {
	switch a {
	case MoveLeft, MoveRight, MoveType, MoveJump, MoveHelp, MoveReady, MoveInterrupt, MoveEOF:
		return true
	}
	return false
}

// ClassifyMoving decides what a key press does while moving. It is
// pure, and the index is set for MoveJump. Nothing here types.
func ClassifyMoving(k tea.KeyPressMsg) (MoveAction, int) {
	switch s := k.String(); s {
	case "h", "left":
		return MoveLeft, 0
	case "l", "right":
		return MoveRight, 0
	case "k", "up":
		return MoveUp, 0
	case "j", "down":
		return MoveDown, 0
	case "pgup":
		return MovePageUp, 0
	case "pgdown":
		return MovePageDown, 0
	case "g", "home":
		return MoveTop, 0
	case "G", "end":
		return MoveBottom, 0
	case "enter":
		return MoveEnter, 0
	case "i":
		return MoveType, 0
	case "esc":
		return MoveEsc, 0
	case "X":
		return MoveEnd, 0
	case "R":
		return MoveReady, 0
	case "ctrl+t":
		return MoveTools, 0
	case "ctrl+o":
		return MoveThinking, 0
	case "?":
		return MoveHelp, 0
	case "ctrl+c":
		return MoveInterrupt, 0
	case "ctrl+d":
		return MoveEOF, 0
	default:
		n, _ := strings.CutPrefix(s, "alt+")
		if digit(n) {
			return MoveJump, int(n[0] - '1')
		}
	}
	return MoveNone, 0
}

// printable says the key would type text: an unbound one says "i to
// type" rather than doing nothing without a word.
func printable(k tea.KeyPressMsg) bool { return k.Text != "" && k.Mod&^tea.ModShift == 0 }

// ClassifyTranscript decides what a key press does in the transcript
// view: Move, with the move, or one of ShowTools, ShowThinking, Help,
// Interrupt and EOF. Every other key is swallowed (PassThrough), so
// nothing typed reaches the input box. It is pure.
func ClassifyTranscript(k tea.KeyPressMsg, inputEmpty bool) (Action, core.TranscriptAction) {
	switch k.String() {
	case "up", "k":
		return Move, core.TranscriptUp
	case "down", "j":
		return Move, core.TranscriptDown
	case "pgup", "b":
		return Move, core.TranscriptPageUp
	case "pgdown", "space", "f":
		return Move, core.TranscriptPageDown
	case "home", "g":
		return Move, core.TranscriptTop
	case "end", "G":
		return Move, core.TranscriptBottom
	case "esc", "q":
		return Move, core.TranscriptClose
	case "ctrl+t", "t":
		return ShowTools, 0
	case "ctrl+o", "o":
		return ShowThinking, 0
	case "?", "ctrl+/", "ctrl+_":
		return Help, 0
	case "ctrl+c":
		return Interrupt, 0
	case "ctrl+d":
		if inputEmpty {
			return EOF, 0
		}
	}
	return PassThrough, 0
}

// NewlineHint tells the user how to type a new line.
func NewlineHint(c Caps) string {
	if c.Disambiguation {
		return "shift+enter for a new line"
	}
	return "ctrl+j for a new line"
}

// ClassifyPane decides what a key press does while the workspace pane has
// focus. Every key is swallowed, so nothing typed reaches the input box. It
// is pure.
func ClassifyPane(k tea.KeyPressMsg) core.PaneAction {
	switch k.String() {
	case "up", "k", "shift+tab":
		return core.PaneUp
	case "down", "j", "tab":
		return core.PaneDown
	case "pgup":
		return core.PagePrev
	case "pgdown", "space":
		return core.PageNext
	case "enter":
		return core.PaneEnter
	case "esc":
		return core.PaneEsc
	case "y", "Y":
		return core.PaneYes
	case "b":
		return core.PaneBoard
	}
	return core.PaneOther
}

// ClassifyBoard decides what a key press does while the pane shows the
// board. Every key is swallowed. It is pure.
func ClassifyBoard(k tea.KeyPressMsg) core.PaneAction {
	switch k.String() {
	case "up", "k", "shift+tab":
		return core.PaneUp
	case "down", "j", "tab":
		return core.PaneDown
	case "left", "h":
		return core.PaneLeft
	case "right", "l":
		return core.PaneRight
	case "pgup":
		return core.PagePrev
	case "pgdown":
		return core.PageNext
	case "enter":
		return core.PaneEnter
	case "esc":
		return core.PaneEsc
	case "b":
		return core.PaneBoard
	case "r":
		return core.PaneRefresh
	case "space", "x":
		return core.PaneMark
	case "a":
		return core.PaneAdd
	case "A":
		return core.PaneAddColumn
	case "e":
		return core.PaneEdit
	case "E":
		return core.PaneEditColumn
	case "X":
		return core.PaneArchive
	case "y", "Y":
		return core.PaneYes
	}
	return core.PaneOther
}

// ClassifyBoardInput decides what a key press does while the board's
// one-line input is open: enter sends, esc cancels, backspace deletes,
// and a key that types text types it. It is pure.
func ClassifyBoardInput(k tea.KeyPressMsg) (core.PaneAction, string) {
	switch k.String() {
	case "enter":
		return core.PaneEnter, ""
	case "esc":
		return core.PaneEsc, ""
	case "backspace", "ctrl+h":
		return core.PaneBackspace, ""
	}
	if printable(k) {
		return core.PaneChar, k.Text
	}
	return core.PaneOther, ""
}

// ClassifyEnd decides what a key press answers when the view asks
// whether to end a conversation: yes only for y, no for n, Esc and
// Ctrl-C, and nothing for any other key. It is pure.
func ClassifyEnd(k tea.KeyPressMsg) (answered, yes bool) {
	switch k.String() {
	case "y", "Y":
		return true, true
	case "n", "N", "esc", "ctrl+c":
		return true, false
	}
	return false, false
}

// ClassifyReady decides what a key press does in the ready view while no
// reply is being written. R opens and closes the view, so it is handled
// by the caller. It is pure.
func ClassifyReady(k tea.KeyPressMsg) core.ReadyAction {
	switch k.String() {
	case "up", "k", "shift+tab":
		return core.ReadyUp
	case "down", "j", "tab":
		return core.ReadyDown
	case "g", "home":
		return core.ReadyTop
	case "G", "end":
		return core.ReadyBottom
	case "a":
		return core.ReadyReplyKey
	case "u":
		return core.ReadyUndoKey
	case "enter":
		return core.ReadyEnter
	case "esc", "q", "h", "left", "ctrl+c":
		return core.ReadyEsc
	case "r":
		return core.ReadyRefresh
	}
	return core.ReadyOther
}

// ClassifyReadyInput decides what a key press does while a reply is being
// written: enter sends, esc cancels, backspace deletes, and a key that
// types text types it. It is pure.
func ClassifyReadyInput(k tea.KeyPressMsg) (core.ReadyAction, string) {
	switch k.String() {
	case "enter":
		return core.ReadyEnter, ""
	case "esc", "ctrl+c":
		return core.ReadyEsc, ""
	case "backspace", "ctrl+h":
		return core.ReadyBackspace, ""
	}
	if printable(k) {
		return core.ReadyChar, k.Text
	}
	return core.ReadyOther, ""
}
