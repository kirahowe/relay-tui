package render

import (
	"fmt"
	"strings"

	"github.com/kirahowe/relay-tui/internal/core"
)

// The dashboard only summarises a waiting request: what it is called,
// shortened to the room there is, and how to get to its review, which
// alone answers it (see pager.go). Nothing here arms or answers anything.

// wrapAll wraps each line, keeping its leading spaces on the lines it
// breaks into.
func wrapAll(lines []string, width int) []string {
	var out []string
	for _, l := range lines {
		lead := len(l) - len(strings.TrimLeft(l, " "))
		for i, w := range hardwrap(l, max(width-lead, 1)) {
			if i > 0 || lead > 0 {
				w = strings.Repeat(" ", lead) + w
			}
			out = append(out, w)
		}
	}
	return out
}

// optionKey is the key that picks an option outright, or "".
func optionKey(k core.OptionKind) string {
	switch k {
	case core.AllowOnce:
		return "y"
	case core.RejectOnce:
		return "n"
	}
	return ""
}

// requestTitle is the request on offer's name, on one line, safe, and
// defanged when what it shows is contaminated.
func requestTitle(s core.State) string {
	p := s.Asks[0]
	title := core.VisibleLine(p.Label())
	if p.Shown().Union(s.Labels).Contaminated {
		title = Defang(title)
	}
	return title
}

// requestNote is the line that says a request waits while the operator
// types, and how to get to it.
func requestNote(s core.State, surf core.Surface, width int) string {
	more := ""
	if n := len(s.Asks); n > 1 {
		more = fmt.Sprintf(" (%d)", n)
	}
	return truncate(styled(bold|reverse, " permission needed"+more+" ")+" "+styled(bold, requestTitle(s))+styled(faint, " · "+howToAnswer(surf)), width)
}

// requestSummary is the request on offer where the input box was, while
// its conversation has the keys: its name and "enter to review".
func requestSummary(s core.State, width int) []string {
	more := ""
	if n := len(s.Asks) - 1; n > 0 {
		more = fmt.Sprintf(" · %d more waiting", n)
	}
	line := styled(bold, requestTitle(s)) + styled(faint, " · ") + styled(bold|accent, "enter") + styled(faint, " to review"+more)
	return box("permission needed", []string{truncate(line, InputWidth(width))}, width)
}

// howToAnswer is how to get to the request from what has the keys: its
// review opens from its summary.
func howToAnswer(surf core.Surface) string {
	switch surf.Covered().Kind {
	case core.SurfaceInput:
		return "esc, then enter to review"
	case core.SurfaceEditor, core.SurfaceReview:
		return "esc, then h and enter to review"
	case core.SurfacePane:
		return "h, then enter to review"
	}
	return "to review it, go to its conversation"
}

// capLines is lines cut to n, with the last one ending in an ellipsis when
// something was cut.
func capLines(lines []string, n, width int) []string {
	if len(lines) <= n {
		return lines
	}
	out := append([]string(nil), lines[:n]...)
	out[n-1] = truncate(out[n-1], max(width-2, 1)) + " …"
	return out
}

// fixedLines says which arguments of the input the operator set, not the
// model, or nothing when there are none.
func fixedLines(names []string, def bool, width int) []string {
	if len(names) == 0 {
		return nil
	}
	esc := make([]string, len(names))
	for i, n := range names {
		esc[i] = paneLine(n, def)
	}
	return hang("", styled(faint, "set by your settings: ")+strings.Join(esc, ", "), width)
}
