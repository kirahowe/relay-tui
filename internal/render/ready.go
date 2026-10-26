package render

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/kirahowe/relay-tui/internal/core"
)

// The ready view draws what relay says is ready for the operator, over
// the whole screen: one panel per item, a blank row between, the newest
// first. A panel's border carries the kind and the title and, at the
// right, how long ago; its bottom border says where the thing came from,
// in words relay chose, in the warning colour when it came from outside.
// relay has made every text safe already; this draws it as sent, still
// escaping anything that does not print and defanging a link left live in
// outside text. It names no label, and it fetches nothing.

// readyMaxWidth is the widest a panel is drawn: a wide screen keeps the
// text a readable length.
const readyMaxWidth = 100

// readyBodyLines is how many rows of text a panel shows.
const readyBodyLines = 5

var labelWords = regexp.MustCompile(`(?i)contaminated|untrusted|private`)

// readyFrom is where an item came from, as relay said, safe to print. A
// word that names a label is never printed: it reads "from outside".
func readyFrom(it core.ReadyItem) string {
	from := oneLine(core.EscapeNonPrinting(strings.TrimSpace(it.From)))
	switch {
	case labelWords.MatchString(from):
		return "from outside"
	case from == "" && it.Outsider:
		return "from outside"
	}
	return from
}

// readyKindName is the word that leads a panel's title.
func readyKindName(it core.ReadyItem) string {
	switch it.Kind {
	case core.ReadyMessage:
		return "Message"
	case core.ReadyDraft:
		return "Draft"
	case core.ReadyEvent:
		return "Calendar"
	case core.ReadyFinding:
		return "Board"
	}
	return "Ready"
}

// readyColour is the colour of a kind.
func readyColour(kind string) style {
	switch kind {
	case core.ReadyMessage:
		return fg(hueBlue)
	case core.ReadyDraft:
		return okColour
	case core.ReadyEvent:
		return fg(hueMagenta)
	}
	return 0
}

// ago says how long ago at was, briefly, or the date once it is old.
func ago(at string, now time.Time) string {
	t, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return ""
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d/time.Hour))
	case d < 48*time.Hour:
		return "yesterday"
	}
	return t.In(now.Location()).Format("Jan 2")
}

// whenLine is an event's time: its start, and its end when it is later
// the same day or another day.
func whenLine(it core.ReadyItem, now time.Time) string {
	s, err := time.Parse(time.RFC3339Nano, it.Start)
	if err != nil {
		return ""
	}
	s = s.In(now.Location())
	out := s.Format("Mon Jan 2 · 15:04")
	if e, err := time.Parse(time.RFC3339Nano, it.End); err == nil && e.After(s) {
		e = e.In(now.Location())
		if e.YearDay() == s.YearDay() && e.Year() == s.Year() {
			out += "–" + e.Format("15:04")
		} else {
			out += " – " + e.Format("Mon Jan 2 · 15:04")
		}
	}
	return out
}

// readyPanel is one item as a panel of exactly width columns.
func readyPanel(it core.ReadyItem, now time.Time, width int, focus bool) []string {
	inner := framed(width)
	var lines []string
	if it.Kind == core.ReadyEvent {
		if w := whenLine(it, now); w != "" {
			lines = append(lines, styled(bold, w))
		}
	}
	for _, l := range it.Lines {
		lines = append(lines, cardLines(l, it.Outsider, inner)...)
	}
	if len(lines) > readyBodyLines {
		lines = append(lines[:readyBodyLines-1], styled(faint, "…"))
	}
	for _, r := range it.Replies[max(len(it.Replies)-1, 0):] {
		lines = append(lines, wrap(styled(faint, "↩ you replied: ")+cardLine(r.Text, false), inner)[0])
	}
	title := cardLine(it.Title, it.Outsider)
	if strings.TrimSpace(title) == "" {
		title = "(untitled)"
	}
	head := styled(bold|readyColour(it.Kind), readyKindName(it)) + styled(faint, " · ") + title
	if it.Urgency == "high" {
		head = styled(bold, "! ") + head
	}
	foot := ""
	if from := readyFrom(it); from != "" {
		foot = styled(warnColour, from)
	}
	return panel(head, styled(faint, ago(it.At, now)), foot, lines, width, len(lines)+2, focus)
}

// readyRows lays the items out as panels with a blank row between, and
// gives each one's first and last row, so the window can keep the
// selected one in view.
func readyRows(r core.Ready, now time.Time, width int) (rows []string, first, last []int) {
	for i, it := range r.Items {
		if i > 0 {
			rows = append(rows, "")
		}
		p := readyPanel(it, now, width, it.ID == r.Sel)
		first = append(first, len(rows))
		rows = append(rows, p...)
		last = append(last, len(rows)-1)
	}
	return rows, first, last
}

// readyWindow is the rows to show in room rows with the selected item in
// view: as much of what is above it as fits.
func readyWindow(rows []string, first, last []int, sel, room int) []string {
	if len(rows) == 0 || room <= 0 {
		return nil
	}
	start := 0
	if sel >= 0 && sel < len(first) && last[sel] >= room {
		start = last[sel] - room + 1
		if first[sel] < start && last[sel]-first[sel]+1 > room {
			start = first[sel] // taller than the room: show its top
		}
		// Begin at a panel, not in the middle of one, when that still
		// shows the selected one whole.
		for i := range first {
			if first[i] >= start {
				if last[sel]-first[i]+1 <= room {
					start = first[i]
				}
				break
			}
		}
	}
	return rows[start:min(len(rows), start+room)]
}

// readyHint is the row of keys at the bottom.
func readyHint(r core.Ready) string {
	switch {
	case r.Input != nil:
		return "enter send · esc cancel"
	}
	keys := "↑↓ move"
	if it, i := core.ReadySelected(r); i >= 0 && it.ReplyTo != 0 {
		keys += " · a reply"
	}
	return keys + " · r refresh · R close"
}

// readyStatus is the row above the keys: the reply being written, or the
// outcome of the last thing, or nothing.
func readyStatus(r core.Ready, width int) string {
	switch {
	case r.Input != nil:
		subject := ""
		for _, it := range r.Items {
			if it.ReplyTo == r.Input.Message {
				subject = it.Title
				break
			}
		}
		lead := styled(faint, "Reply to “") + truncate(cardLine(subject, true), max(width/3, 8)) + styled(faint, "”: ")
		room := max(width-Method.StringWidth(lead)-2, 1)
		text := core.EscapeNonPrinting(strings.ReplaceAll(r.Input.Text, "\n", " "))
		if Method.StringWidth(text) > room {
			text = "…" + string([]rune(text)[max(len([]rune(text))-room+1, 0):])
		}
		return truncate(" "+lead+text+styled(accent, "▌"), width)
	case r.Failure != "":
		return truncate(" "+styled(warnColour, r.Message)+" "+styled(faint, oneLine(core.EscapeNonPrinting(r.Failure))), width)
	case r.Message != "":
		return truncate(" "+styled(faint, r.Message), width)
	}
	return ""
}

// readyScreen draws the ready view over the whole screen at width, height
// rows: a top bar, the panels, the status row and the keys.
func readyScreen(h core.Hub, now time.Time, width, height int) HubFrame {
	r := h.Ready
	left := styled(accent, " ◆ ") + styled(bold, "relay") + "   " + styled(bold, "Ready")
	right := ""
	if n := len(r.Items); n > 0 {
		right = styled(faint, fmt.Sprintf("%d ", n))
	}
	bar := left
	if gap := width - Method.StringWidth(left) - Method.StringWidth(right); gap > 0 {
		bar += strings.Repeat(" ", gap) + right
	}
	rows := []string{truncate(bar, width)}

	room := max(height-3, 0)
	// A refusal of the reply being written has a row of its own above it:
	// the words come back, and the reason must not hide behind them.
	note := ""
	if r.Input != nil && r.Failure != "" && room > 0 {
		note = truncate(" "+styled(warnColour, r.Message)+" "+styled(faint, oneLine(core.EscapeNonPrinting(r.Failure))), width)
		room--
	}
	var body []string
	switch {
	case r.Unavailable:
		body = []string{"", " " + styled(faint, "This relay has nothing ready to show.")}
	case !r.Loaded && len(r.Items) == 0:
		body = []string{"", " " + styled(faint, "loading…")}
	case len(r.Items) == 0:
		body = []string{"", " " + styled(faint, "Nothing is ready.")}
	default:
		pw := min(width, readyMaxWidth)
		all, first, last := readyRows(r, now, pw)
		_, sel := core.ReadySelected(r)
		body = readyWindow(all, first, last, sel, room)
	}
	if len(body) > room {
		body = body[:room]
	}
	rows = append(rows, body...)
	for len(rows) < height-2-btoi(note != "") {
		rows = append(rows, "")
	}
	if note != "" {
		rows = append(rows, note)
	}
	rows = append(rows, readyStatus(r, width))
	typing := r.Input != nil
	rows = append(rows, modeRow(!typing, readyHint(r), width))
	for i := range rows {
		rows[i] = truncate(rows[i], width)
	}
	active := core.SurfaceReady
	if typing {
		active = core.SurfaceReadyInput
	}
	return HubFrame{Content: strings.Join(rows[:min(len(rows), height)], "\n"), Active: active}
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}
