package render

import "github.com/charmbracelet/x/ansi"

// Method is the one way relay-tui measures how wide text is on the
// terminal: by code point (wcwidth), which is Bubble Tea's own unless a
// terminal negotiates Unicode mode 2027. The shell keeps Bubble Tea on it
// (ui.KeepWidthMethod drops that negotiation), and everything here lays
// out by it, so what the renderer means to draw is what Bubble Tea puts
// in its buffer. The review fails closed on any grapheme whose width the
// other method would measure differently (see pager.go), since a terminal
// may draw it either way.
var Method = ansi.WcWidth

// Width is how many cells s takes, by Method.
func Width(s string) int { return Method.StringWidth(s) }
