package ui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// KeepWidthMethod is the program's message filter (tea.WithFilter) that
// keeps Bubble Tea measuring text by render.Method. Bubble Tea switches
// to grapheme widths when the terminal reports Unicode mode 2027; this
// drops that report, so the renderer and Bubble Tea's own buffer always
// agree on how wide a line is.
func KeepWidthMethod(_ tea.Model, msg tea.Msg) tea.Msg {
	if m, ok := msg.(tea.ModeReportMsg); ok && m.Mode == ansi.ModeUnicodeCore {
		return nil
	}
	return msg
}
