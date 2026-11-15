package render

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"

	"github.com/kirahowe/relay-tui/internal/core"
)

// The list of the links in one thing, a ready item or a card: each link
// in full, a filled dot before a live one and a ring before a broken
// one. The ready view and the board draw the same list. Every link reads
// in its broken form. A live one is a hyperlink to the address relay
// listed for it, here as where it was read. A broken one is text and
// nothing else.

// linkRows is the list's rows, inner cells wide, and the rows the
// selected link takes: from selA up to, not including, selB. The
// selected link sits on the tinted bar.
func linkRows(links []core.ShownLink, sel int, live core.Links, inner int) (rows []string, selA, selB int) {
	if len(links) == 0 {
		return []string{styled(faint, truncate(core.NoLinksNote, inner))}, 0, 0
	}
	for i, l := range links {
		lead := styled(faint, "○") + " "
		// A row is drawn as every piece of a thing's text is, with a
		// list of its own: the one link it shows when it is live, and
		// none when it is broken. So a broken row is never a hyperlink,
		// whatever its text is, and a live row's target is the address
		// relay listed, which fang and gate hold to the set again.
		var own []core.Link
		if l.Live {
			lead = styled(okColour, "●") + " "
			own = []core.Link{{Address: l.Address, Shown: l.Text}}
		}
		text := cardLine(l.Text, true, live.Of(own))
		part := hang(lead, text, inner)
		if i == sel {
			selA = len(rows)
			for k := range part {
				part[k] = barRow(part[k], inner)
			}
			selB = selA + len(part)
		}
		rows = append(rows, part...)
	}
	return rows, selA, selB
}

// linkLegend counts the list: "2 live · 1 broken", a part left out when
// it is none. leftOut is how many more live links relay has for the thing
// than it listed, and they are said last: "3 more not listed".
func linkLegend(links []core.ShownLink, leftOut int) string {
	live := 0
	for _, l := range links {
		if l.Live {
			live++
		}
	}
	var parts []string
	if live > 0 {
		parts = append(parts, fmt.Sprintf("%d live", live))
	}
	if broken := len(links) - live; broken > 0 {
		parts = append(parts, fmt.Sprintf("%d broken", broken))
	}
	if leftOut > 0 {
		parts = append(parts, fmt.Sprintf("%d more not listed", leftOut))
	}
	return strings.Join(parts, " · ")
}

// linkHead is the words in the list's top border: Links, and what the
// links are in when the border has room for it beside the count.
func linkHead(title, legend string, width int) string {
	head := styled(bold, "Links")
	full := head + styled(faint, " · ") + title
	if strings.TrimSpace(title) != "" && Method.StringWidth(full)+Method.StringWidth(legend)+10 <= width {
		return full
	}
	return head
}

// askedRows is the link a question is about, as the list draws it, on
// rows of its own under the question: wherever the list is cut, the link
// a yes would send is on screen beside the question. A link longer than
// limit rows shows its first rows and ends in an ellipsis, but its host
// is never cut: the rows go on until the whole host is shown, however
// many that takes. The list above has the link whole.
func askedRows(ask core.LinkAsk, live core.Links, width, limit int) []string {
	var own []core.Link
	if !ask.Keep {
		own = []core.Link{{Address: ask.Address, Shown: ask.Text}}
	}
	rows := hang("  ", cardLine(ask.Text, true, live.Of(own)), width)
	host := core.LinkHost(ask.Text)
	for k := 1; k <= len(rows); k++ {
		if strings.Contains(solidText(rows[:k]), host) {
			limit = max(limit, k)
			break
		}
	}
	if n := max(limit, 1); len(rows) > n {
		rows = rows[:n]
		// A cut row ends in an ellipsis where its last cell was.
		rows[n-1] = truncate(rows[n-1]+"  ", Width(rows[n-1]))
	}
	return rows
}

// solidText is rows as a reader sees them run together: styles and
// hyperlinks gone, and every space and box-drawing mark between them
// taken out, so a word wrapped over rows or framed in a panel reads
// whole.
func solidText(rows []string) string {
	plain := ansi.Strip(strings.Join(rows, "\n"))
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || (r >= 0x2500 && r <= 0x259F) || r == '…' {
			return -1
		}
		return r
	}, plain)
}

// askShown says rows, as drawn, show the words of the question that ask
// and the whole host of the link it is about: what the operator answers, and
// where the link goes. A yes to a question not shown so is refused
// (core's Hidden).
func askShown(ask *core.LinkAsk, rows []string) bool {
	if ask == nil {
		return false
	}
	// The words that ask, whatever else of the question a short screen
	// leaves out.
	asks := "Dropthislink?"
	if ask.Keep {
		asks = "Keepit?"
	}
	solid := solidText(rows)
	return strings.Contains(solid, asks) && strings.Contains(solid, core.LinkHost(ask.Text))
}

// boardAsk is the question the board's list of links asks, or nil.
func boardAsk(a core.App) *core.LinkAsk {
	if l := a.Board.Links; l != nil {
		return l.Ask
	}
	return nil
}

// linkQuestion is the question a list asks, in words.
func linkQuestion(ask core.LinkAsk) string {
	if ask.Keep {
		return core.KeepQuestion
	}
	return core.DropQuestion
}

// linkAskHint is the keys that answer the question.
func linkAskHint(ask core.LinkAsk) string {
	if ask.Keep {
		return "y keep · any other key leaves it broken"
	}
	return "y drop · any other key keeps it"
}

// linkListHint is the keys of the list, for the link selected: a broken
// one can be kept, a live one dropped.
func linkListHint(ll core.LinkList, links []core.ShownLink, back string) string {
	keys := []string{"↑↓ select"}
	if ll.Sel >= 0 && ll.Sel < len(links) {
		if links[ll.Sel].Live {
			keys = append(keys, "d drop")
		} else {
			keys = append(keys, "enter keep")
		}
	} else {
		keys = nil
	}
	return strings.Join(append(keys, back), " · ")
}

// linkWindow is the rows of a list to show in room rows, with the
// selected link in view.
func linkWindow(rows []string, selA, selB, room int) []string {
	if room <= 0 {
		return nil
	}
	if len(rows) <= room {
		return rows
	}
	start := max(selB-room, 0)
	if selA < start {
		start = selA
	}
	start = min(start, len(rows)-room)
	return rows[start : start+room]
}
