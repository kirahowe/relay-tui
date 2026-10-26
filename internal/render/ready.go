package render

import (
	"fmt"
	"regexp"
	"slices"
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
	case core.ReadyMove:
		if it.Undone {
			return "Moved back"
		}
		return "Moved"
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
	return t.In(now.Location()).Format("2 Jan")
}

// ZoneTime is now in the zone relay named for the operator (an IANA name
// such as America/Halifax, from the ready answer), so every time in the
// view reads as it does in relay's own output. A zone that is empty or
// unknown leaves it in the machine's zone.
func ZoneTime(now time.Time, zone string) time.Time {
	if zone == "" {
		return now
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return now
	}
	return now.In(loc)
}

// whenLine is an event's time, as "Mon 2 Jan · 19:00": its start, and
// its end when it is later the same day or another day. The zone is
// now's.
func whenLine(it core.ReadyItem, now time.Time) string {
	s, err := time.Parse(time.RFC3339Nano, it.Start)
	if err != nil {
		return ""
	}
	s = s.In(now.Location())
	out := s.Format("Mon 2 Jan · 15:04")
	if e, err := time.Parse(time.RFC3339Nano, it.End); err == nil && e.After(s) {
		e = e.In(now.Location())
		if e.YearDay() == s.YearDay() && e.Year() == s.Year() {
			out += "–" + e.Format("15:04")
		} else {
			out += " – " + e.Format("Mon 2 Jan · 15:04")
		}
	}
	return out
}

// readyHead is the words in a panel's top border and the age at its
// right. When the kind, the title and the age do not all fit, the border
// keeps the kind and the age and the title goes to rows of its own
// (titleRows, wrapped, at most maxRows of them), so the age never drops
// out and a long title is not cut mid-word.
func readyHead(it core.ReadyItem, now time.Time, width, maxRows int) (head, age string, titleRows []string) {
	age = ago(it.At, now)
	title := cardLine(it.Title, it.Outsider)
	if strings.TrimSpace(title) == "" {
		title = "(untitled)"
	}
	lead := ""
	if it.Urgency == "high" {
		lead = styled(bold, "! ")
	}
	kind := lead + styled(bold|readyColour(it.Kind), readyKindName(it))
	full := kind + styled(faint, " · ") + title
	if Method.StringWidth(full)+Method.StringWidth(age)+10 <= width {
		return full, age, nil
	}
	rows := wrap(title, framed(width))
	if maxRows > 0 {
		rows = capLines(rows, maxRows, framed(width))
	}
	for i := range rows {
		rows[i] = styled(bold, rows[i])
	}
	return kind, age, rows
}

// readyListLines is the rows of an item's lines in the list: a move's
// lines one row each, the rest wrapped; cut to readyBodyLines rows, the
// last one saying how to read the rest.
func readyListLines(it core.ReadyItem, inner int) []string {
	var rows []string
	for _, l := range it.Lines {
		if it.Kind == core.ReadyMove {
			rows = append(rows, truncate(cardLine(l, it.Outsider), inner))
			continue
		}
		rows = append(rows, cardLines(l, it.Outsider, inner)...)
	}
	return capReady(rows, it, inner)
}

func capReady(rows []string, it core.ReadyItem, inner int) []string {
	if len(rows) <= readyBodyLines {
		return rows
	}
	keep := readyBodyLines - 1
	more := styled(faint, "… enter reads it all")
	if it.Kind == core.ReadyMove {
		more = styled(faint, fmt.Sprintf("… and %d more · enter lists all", len(rows)-keep))
	}
	return append(slices.Clone(rows[:keep]), truncate(more, inner))
}

// readyPanel is one item as a panel of exactly width columns.
func readyPanel(it core.ReadyItem, now time.Time, width int, focus bool) []string {
	inner := framed(width)
	head, age, lines := readyHead(it, now, width, 3)
	if it.Kind == core.ReadyEvent {
		if w := whenLine(it, now); w != "" {
			lines = append(lines, styled(bold, w))
		}
	}
	lines = append(lines, readyListLines(it, inner)...)
	for _, r := range it.Replies[max(len(it.Replies)-1, 0):] {
		lines = append(lines, wrap(styled(faint, "↩ you replied: ")+cardLine(r.Text, false), inner)[0])
	}
	foot := ""
	if from := readyFrom(it); from != "" {
		foot = styled(warnColour, from)
	}
	return panel(head, styled(faint, age), foot, lines, width, len(lines)+2, focus)
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
	out, _ := readyWindowAt(rows, first, last, sel, room)
	return out
}

// readyWindowAt is readyWindow and the index of the first row it shows.
func readyWindowAt(rows []string, first, last []int, sel, room int) ([]string, int) {
	if len(rows) == 0 || room <= 0 {
		return nil, 0
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
	return rows[start:min(len(rows), start+room)], start
}

// hiddenNew is how many of the new items are wholly above the window that
// starts at row start.
func hiddenNew(r core.Ready, last []int, start int) int {
	n := 0
	for _, id := range r.New {
		i := slices.IndexFunc(r.Items, func(it core.ReadyItem) bool { return it.ID == id })
		if i >= 0 && i < len(last) && last[i] < start {
			n++
		}
	}
	return n
}

// ReadyListKeys is what the keys do in the ready view's list, in the list
// of keys.
var ReadyListKeys = []KeyHelp{
	{[]string{"up", "down", "k", "j", "tab", "shift+tab"}, "select"},
	{[]string{"g", "G", "home", "end"}, "top, bottom"},
	{[]string{"enter", "o"}, "open the one selected, whole"},
	{[]string{"1-9"}, "open a daily note at that numbered item"},
	{[]string{"a"}, "reply to a message"},
	{[]string{"u"}, "undo a file move"},
	{[]string{"r"}, "ask relay again"},
	{[]string{"R", "esc", "q", "h", "left"}, "close this view"},
	{[]string{"?"}, "these keys"},
}

// readyHint is the row of keys at the bottom: the ones that matter here
// first, so a narrow screen cuts the least useful.
func readyHint(r core.Ready) string {
	if r.Input != nil {
		return "enter send · esc cancel"
	}
	if r.Help {
		return "any key closes the list"
	}
	it, i := core.ReadySelected(r)
	if r.Detail != "" {
		it, i = core.ReadyDetail(r)
		var keys []string
		if i >= 0 && len(it.Acts) > 0 && r.Pick < len(it.Acts) {
			a := it.Acts[r.Pick]
			switch {
			case a.Done == core.DoneDeclined:
				keys = append(keys, "d drop")
			case a.Done != "":
			case a.Op == core.OpRevert:
				keys = append(keys, "v revert")
			case a.Offer:
				keys = append(keys, "n no thanks", "d drop")
			default:
				keys = append(keys, "d drop")
			}
		}
		if i >= 0 && it.ReplyTo != 0 {
			keys = append(keys, "a reply")
		}
		if i >= 0 && it.Undo != 0 {
			keys = append(keys, "u undo")
		}
		keys = append(keys, "esc back", "↑↓ scroll")
		if i >= 0 && len(it.Acts) > 1 {
			keys = append(keys, "tab item")
		}
		return strings.Join(append(keys, "? keys"), " · ")
	}
	keys := []string{"↑↓ move"}
	if i >= 0 {
		keys = append(keys, "enter open")
	}
	switch {
	case i >= 0 && it.ReplyTo != 0:
		keys = append(keys, "a reply")
	case i >= 0 && it.Undo != 0:
		keys = append(keys, "u undo")
	}
	return strings.Join(append(keys, "? keys", "R close"), " · ")
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
	now = ZoneTime(now, r.Zone)
	left := styled(accent, " ◆ ") + styled(bold, "relay") + "   " + styled(bold, "Ready")
	count := ""
	if n := len(r.Items); n > 0 {
		count = fmt.Sprintf("%d ", n)
	}
	rows := []string{""} // the bar, once the body says what is out of sight

	// A refusal with no reply open may take up to a third of the screen,
	// at least three rows, so its reason (which files an undo left, and
	// why) is read whole on a phone.
	status := []string{readyStatus(r, width)}
	if r.Input == nil && r.Failure != "" {
		status = readyFailureRows(r, width, max(min(max(3, height/3), height-3), 1))
	}
	room := max(height-2-len(status), 0)
	// A refusal of the reply being written has a row of its own above it:
	// the words come back, and the reason must not hide behind them.
	note := ""
	if r.Input != nil && r.Failure != "" && room > 0 {
		note = truncate(" "+styled(warnColour, r.Message)+" "+styled(faint, oneLine(core.EscapeNonPrinting(r.Failure))), width)
		room--
	}
	var body []string
	var top, maxTop, page int
	detail, di := core.ReadyDetail(r)
	switch {
	case r.Help:
		rows := ReadyListKeys
		title := "ready keys"
		if di >= 0 {
			rows, title = ReadyDetailKeys, "keys for this one"
		}
		body = helpBox(title, rows, min(width, readyMaxWidth))
	case di >= 0:
		body, top, maxTop, page = readyDetail(r, detail, now, min(width, readyMaxWidth), room)
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
		var start int
		body, start = readyWindowAt(all, first, last, sel, room)
		if n := hiddenNew(r, last, start); n > 0 {
			count = fmt.Sprintf("↑ %d new · ", n) + count
		}
	}
	if len(body) > room {
		body = body[:room]
	}
	// The bar carries the count at its right, and, when new items sit above
	// the window, how many.
	right := styled(faint, count)
	bar := left
	if gap := width - Method.StringWidth(left) - Method.StringWidth(right); gap > 0 {
		bar += strings.Repeat(" ", gap) + right
	}
	rows[0] = truncate(bar, width)
	rows = append(rows, body...)
	for len(rows) < height-1-len(status)-btoi(note != "") {
		rows = append(rows, "")
	}
	if note != "" {
		rows = append(rows, note)
	}
	rows = append(rows, status...)
	typing := r.Input != nil
	rows = append(rows, modeRow(!typing, readyHint(r), width))
	for i := range rows {
		rows[i] = truncate(rows[i], width)
	}
	active := core.SurfaceReady
	if typing {
		active = core.SurfaceReadyInput
	}
	return HubFrame{Content: strings.Join(rows[:min(len(rows), height)], "\n"), Active: active,
		Top: top, Max: maxTop, Page: page}
}

// readyFailureRows is a refusal over at most n rows, wrapped, the last
// row cut when it is longer still.
func readyFailureRows(r core.Ready, width, n int) []string {
	text := styled(warnColour, r.Message) + " " + styled(faint, oneLine(core.EscapeNonPrinting(r.Failure)))
	rows := wrap(text, max(width-1, 1))
	if len(rows) > n {
		rows = rows[:n]
		rows[n-1] = truncate(rows[n-1]+" …", max(width-1, 1))
	}
	for i := range rows {
		rows[i] = truncate(" "+rows[i], width)
	}
	return rows
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}
