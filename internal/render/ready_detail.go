package render

import (
	"slices"
	"strings"
	"time"

	"github.com/kirahowe/relay-tui/internal/core"
)

// An item of the ready view open whole: its rows, the panel that scrolls
// them, and the keys that work there.

// readyDetailRows is an item whole, as rows of inner width: its title
// when the border cannot hold it, an event's time, every line wrapped, and
// every reply. For a daily note each numbered item has a two-column gutter
// that shows the picked one with a marker. first and last are the rows of
// the picked numbered item, or -1.
func readyDetailRows(r core.Ready, it core.ReadyItem, now time.Time, width int) (rows []string, first, last int, head, age string) {
	inner := framed(width)
	head, age, titleRows := readyHead(it, now, width, 0)
	first, last = -1, -1
	rows = append(rows, titleRows...)
	if len(titleRows) > 0 {
		rows = append(rows, "")
	}
	if it.Kind == core.ReadyEvent {
		if w := whenLine(it, now); w != "" {
			rows = append(rows, styled(bold, w), "")
		}
	}
	gutter := 0
	if len(it.Acts) > 0 {
		gutter = 2
	}
	// A message with headings shows each above its lines, after a blank
	// row. heads maps a line's index to the heading before it.
	heads := map[int]string{}
	li := 0
	for _, sec := range it.Sections {
		if sec.Heading != "" {
			heads[li] = sec.Heading
		}
		li += len(sec.Lines)
	}
	for li, l := range it.Lines {
		if h, ok := heads[li]; ok {
			if len(rows) > 0 {
				rows = append(rows, "")
			}
			rows = append(rows, wrap(styled(bold, cardLine(h, it.Outsider)), inner)...)
		}
		ai := slices.IndexFunc(it.Acts, func(a core.ReadyAct) bool { return a.Line == li })
		text := l
		if ai >= 0 {
			text = doneMark(text, it.Acts[ai].Done)
		}
		parts := cardLines(text, it.Outsider, max(inner-gutter, 1))
		if ai >= 0 && ai == r.Pick {
			first = len(rows)
		}
		for pi, part := range parts {
			g := strings.Repeat(" ", gutter)
			if ai >= 0 && ai == r.Pick {
				if pi == 0 {
					g = styled(accent, "▸ ")
				}
				part = styled(bold, part)
			}
			rows = append(rows, g+part)
		}
		if ai >= 0 && ai == r.Pick {
			last = len(rows) - 1
		}
	}
	for _, rp := range it.Replies {
		rows = append(rows, "")
		rows = append(rows, wrap(styled(faint, "↩ you replied: ")+cardLine(rp.Text, false), inner)...)
	}
	return rows, first, last, head, age
}

// doneMarks is how a numbered item done since reads, after its text.
var doneMarks = map[string]string{
	core.DoneReverted:  " (undone)",
	core.DoneDismissed: " (dropped)",
	core.DoneDeclined:  " (offer declined)",
}

// doneMark is a numbered item's line with the mark of what was done with
// it, once: relay marks its lines too, and an act done here shows before
// relay sends the note again.
func doneMark(text, done string) string {
	m := doneMarks[done]
	if m == "" || strings.HasSuffix(text, m) {
		return text
	}
	for _, other := range doneMarks {
		text = strings.TrimSuffix(text, other)
	}
	return text + m
}

// readyDetail draws the open item as one framed panel of room rows,
// scrolled to r.Top (moved to show the picked numbered item when r.Follow
// says so). It returns the rows, the first row of the item shown, how far
// it can scroll and how many rows a page is.
func readyDetail(r core.Ready, it core.ReadyItem, now time.Time, width, room int) (out []string, top, maxTop, page int) {
	all, first, last, head, age := readyDetailRows(r, it, now, width)
	// The panel is as tall as what it holds, up to the room.
	h := max(min(room-2, len(all)), 1)
	maxTop = max(len(all)-h, 0)
	top = min(max(r.Top, 0), maxTop)
	if r.Follow && first >= 0 {
		switch {
		case first < top:
			top = first
		case last >= top+h:
			top = min(last-h+1, first)
		}
		top = min(max(top, 0), maxTop)
	}
	view := all[top:min(len(all), top+h)]
	right := styled(faint, age)
	if maxTop > 0 {
		arrows := ""
		if top > 0 {
			arrows += "↑"
		}
		if top < maxTop {
			arrows += "↓"
		}
		right = styled(faint, strings.TrimSpace(age+" "+arrows))
	}
	foot := ""
	if from := readyFrom(it); from != "" {
		foot = styled(warnColour, from)
	}
	return panel(head, right, foot, view, width, h+2, true), top, maxTop, max(h-1, 1)
}

// ReadyDetailKeys is what the keys do in an item open whole.
var ReadyDetailKeys = []KeyHelp{
	{[]string{"up", "down", "k", "j"}, "scroll"},
	{[]string{"pgup", "pgdown", "space", "g", "G", "home", "end"}, "page, top, bottom"},
	{[]string{"1-9"}, "pick a numbered item of a daily note"},
	{[]string{"tab", "]", "shift+tab", "["}, "the next or the previous numbered item"},
	{[]string{"v"}, "undo the picked change"},
	{[]string{"n"}, "say no to the picked suggestion's offer; it stays"},
	{[]string{"d"}, "drop the picked suggestion for good"},
	{[]string{"a"}, "reply to a message"},
	{[]string{"u"}, "undo a file move"},
	{[]string{"r"}, "ask relay again"},
	{[]string{"R"}, "close this view"},
	{[]string{"esc", "q", "h", "left"}, "back to the list"},
	{[]string{"?"}, "these keys"},
}
