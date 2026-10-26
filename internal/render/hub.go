package render

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/kirahowe/relay-tui/internal/core"
)

// Hub mode draws the whole screen, from the hub's state alone: a sidebar
// of roles beside the conversation in focus. On a narrow screen it draws
// one or the other, as a phone would: the list of roles, or the
// conversation with a way back to the list.

// HubWideMin is the narrowest drawing width that gets the sidebar beside
// the conversation. The shell draws one column short of the screen, so
// this is a 73-column screen.
const HubWideMin = 72

// HubWide reports whether the sidebar stands beside the conversation.
func HubWide(width int) bool { return width >= HubWideMin }

// SidebarWidth is how wide the sidebar is at width.
func SidebarWidth(width int) int { return min(max(width/4, 22), 30) }

// HubMainWidth is the width beside the sidebar: the conversation's panel,
// and the workspace pane when there is room for it.
func HubMainWidth(width int) int {
	if HubWide(width) {
		return width - SidebarWidth(width)
	}
	return width
}

// hubColumns is how wide the conversation's panel and the pane beside it
// are, in wide hub mode, with a column between them; pane is 0 when the
// pane is not beside the conversation.
func hubColumns(h core.Hub, width int) (conv, pane int) {
	mw := HubMainWidth(width)
	if paneShown(h) && Side(mw) {
		pane = PaneWidth(mw)
		return mw - pane - 1, pane
	}
	return mw, 0
}

// paneShown says the workspace pane has something to show (see
// core.Hub.PaneShown).
func paneShown(h core.Hub) bool { return h.PaneShown() }

// focusedChat is the conversation in focus, or none.
func focusedChat(h core.Hub) core.State {
	if c := h.Focused(); c != nil {
		return c.State
	}
	return core.State{}
}

// HubFrame is the whole screen in hub mode. Max is how far the
// conversation can scroll and Page how many lines a page key moves;
// DetailMax is the same for the pane's detail view. When Cursor is set
// the input box is on screen with its first row at InputX, InputY.
type HubFrame struct {
	Content        string
	Max, Page      int
	DetailMax      int
	Cursor         bool
	InputX, InputY int
	// Active is what the frame drew as having the keys.
	Active core.SurfaceKind
	// title and right are the header's parts, for a panel's top border.
	title, right string
}

// TypingKeys is what the keys do while typing in hub mode, in the list
// of keys. Every other key is typed.
var TypingKeys = []KeyHelp{
	{[]string{"enter"}, "send"},
	{[]string{"shift+enter", "ctrl+j"}, "new line"},
	{[]string{"esc"}, "stop typing; what you typed stays"},
	{[]string{"alt+1"}, "role 1 (alt+2 for 2, up to 9), still typing"},
	{[]string{"pgup", "pgdown"}, "scroll the conversation"},
	{[]string{"ctrl+t"}, "show or hide tool calls"},
	{[]string{"ctrl+o"}, "show or hide thinking"},
	{[]string{"ctrl+c"}, "stop the turn, clear, or quit"},
	{[]string{"ctrl+d"}, "quit, from an empty input"},
	{[]string{"?"}, "these keys, from an empty input"},
}

// MovingKeys is what the keys do while moving in hub mode. Nothing is
// typed.
var MovingKeys = []KeyHelp{
	{[]string{"h", "l", "left", "right"}, "roles, conversation, pane"},
	{[]string{"k", "j", "up", "down"}, "move in the list, or scroll"},
	{[]string{"pgup", "pgdown", "g", "G", "home", "end"}, "page, top, bottom"},
	{[]string{"enter"}, "open the role and type, or type"},
	{[]string{"i"}, "type in the conversation"},
	{[]string{"1", "alt+1"}, "role 1 (2 for 2, up to 9)"},
	{[]string{"X"}, "end this conversation, after asking"},
	{[]string{"R"}, "what is ready for you"},
	{[]string{"esc"}, "back to the conversation"},
	{[]string{"ctrl+t"}, "show or hide tool calls"},
	{[]string{"ctrl+o"}, "show or hide thinking"},
	{[]string{"ctrl+c"}, "stop the turn, or quit"},
	{[]string{"ctrl+d"}, "quit, from an empty input"},
	{[]string{"?"}, "these keys"},
}

// hubLegend explains the marks beside a role.
var hubLegend = []string{
	"▶ replying   … waiting for scheduled work",
	"~ scheduled work runs   ● unread",
}

// roleName is a role's title as text that may be shown: its controls and
// invisible characters made visible.
func roleName(r core.Role) string { return paneLine(r.Title, false) }

// marks are the indicators beside a role, styled. They are in the order
// of how much they need the operator.
func marks(h core.Hub, r core.Role, now time.Time) string {
	var parts []string
	if r.Conv != nil && r.Conv.Unread {
		parts = append(parts, styled(bold, "●"))
	}
	if r.Conv != nil {
		switch r.Conv.Turn {
		case core.TurnRunning:
			parts = append(parts, "▶")
		case core.TurnQueued:
			parts = append(parts, "…")
		}
	}
	if r.Working {
		parts = append(parts, "~")
	}
	return strings.Join(parts, " ")
}

// roleRow is one role in the list: a pointer for the one in focus, the
// title and the marks at the right, width wide. The cursor's row is in
// reverse video.
func roleRow(h core.Hub, r core.Role, now time.Time, width int) string {
	lead := "  "
	if r.ID == h.Focus {
		lead = styled(bold, "› ")
	}
	m := marks(h, r, now)
	mw := Method.StringWidth(m)
	room := max(width-2-mw-1, 1)
	title := truncate(roleName(r), room)
	if r.Conv == nil {
		title = styled(faint, title)
	}
	gapN := max(width-2-Method.StringWidth(title)-mw, 0)
	line := lead + title + strings.Repeat(" ", gapN) + m
	if h.List && r.ID == h.Cursor {
		line = sgr(reverse) + ansi.Strip(line) + sgr(0)
	}
	return truncate(line, width)
}

// describe says in words where a role stands, for the list screen.
func describe(h core.Hub, r core.Role, now time.Time) string {
	var parts []string
	c := r.Conv
	if c == nil {
		parts = append(parts, "no conversation")
	} else {
		switch c.Turn {
		case core.TurnRunning:
			parts = append(parts, "replying")
		case core.TurnQueued:
			parts = append(parts, "waiting for scheduled work")
		}
		if c.Unread {
			parts = append(parts, "unread")
		}
		parts = append(parts, plural(c.Turns, "turn"))
		if !c.LastActivity.IsZero() {
			parts = append(parts, Duration(max(now.Sub(c.LastActivity), 0))+" ago")
		}
	}
	if r.Working {
		parts = append(parts, "scheduled work runs")
	}
	return strings.Join(parts, " · ")
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// EmbeddedNote is what the view says all the time when relay is not
// running as a service, so conversations end with this client.
const EmbeddedNote = "relay service not running: conversations end when you close this"

// embeddedNote is the note, or "" when relay keeps the conversations or
// did not say.
func embeddedNote(h core.Hub) string {
	if h.KeepsKnown && !h.Keeps {
		return EmbeddedNote
	}
	return ""
}

// selected is the index of the role the list shows as selected: the
// cursor when the list has the keys, else the one in focus. It is 0 for
// none.
func selected(h core.Hub) int {
	id := h.Focus
	if h.List {
		id = h.Cursor
	}
	for i, r := range h.Roles {
		if r.ID == id {
			return i
		}
	}
	return 0
}

// window is the rows that fit in room, per roles of per rows each, with
// the selected role in them: the list scrolls to keep it in sight.
func window(rows []string, per, sel, room int) []string {
	fit := max(room/per, 0)
	n := len(rows) / per
	if n <= fit {
		return rows
	}
	start := min(max(sel-fit+1, 0), n-fit)
	return rows[start*per : (start+fit)*per]
}

// sidebar is the roles beside the conversation: height rows, width wide.
func sidebar(h core.Hub, now time.Time, width, height int) []string {
	lines := []string{styled(bold, truncate("relay", width))}
	switch {
	case !h.RosterLoaded:
		lines = append(lines, styled(faint, truncate("loading roles…", width)))
	case len(h.Roles) == 0:
		lines = append(lines, styled(faint, truncate("no roles", width)))
	}
	head := len(lines)
	var rows []string
	for _, r := range h.Roles {
		rows = append(rows, roleRow(h, r, now, width))
	}
	foot := []string{styled(faint, truncate("h roles · ? keys", width))}
	room := max(height-len(foot), 0)
	lines = append(lines, window(rows, 1, selected(h), max(room-head, 0))...)
	if len(lines) > room {
		lines = lines[:room]
	}
	for len(lines) < room {
		lines = append(lines, "")
	}
	return append(lines, foot...)
}

// listScreen is the narrow screen's list of roles, with a line in words
// under each.
func listScreen(h core.Hub, now time.Time, width, height int) []string {
	lines := []string{styled(bold, truncate("roles", width))}
	switch {
	case !h.RosterLoaded:
		lines = append(lines, styled(faint, truncate("loading roles…", width)))
	case len(h.Roles) == 0:
		lines = append(lines, styled(faint, truncate("no roles", width)))
	}
	head := len(lines)
	var rows []string
	for _, r := range h.Roles {
		rows = append(rows, roleRow(h, r, now, width), styled(faint, truncate("  "+describe(h, r, now), width)))
	}
	// The note about the service keeps its rows, whatever the list is.
	var note []string
	if n := embeddedNote(h); n != "" {
		for _, l := range capLines(wrap(n, width), 2, width) {
			note = append(note, styled(bold, l))
		}
	}
	room := max(height-1, 0)
	lines = append(lines, window(rows, 2, selected(h), max(room-head-len(note), 0))...)
	if len(lines) > room-len(note) {
		lines = lines[:max(room-len(note), 0)]
	}
	for len(lines) < room-len(note) {
		lines = append(lines, "")
	}
	lines = append(lines, note...)
	foot := styled(faint, truncate(modeTag(true)+" · enter open · ↑↓ move · "+readyKey(h)+" · ? keys", width))
	if h.Notice != "" {
		foot = styled(faint, truncate("! "+oneLine(h.Notice), width))
	}
	lines = lines[:min(len(lines), room)]
	return append(lines, foot)
}

// HubInputWidth is the width of the input box that HubView draws at
// width: the text area must be the box's inside, or text typed past it
// would be out of sight.
func HubInputWidth(h core.Hub, width int) int {
	w := width
	if HubWide(width) {
		cw, _ := hubColumns(h, width)
		return framed(cw) // the prompt mark is the text area's own
	} else if h.List || h.Focus == "" {
		return InputWidth(width)
	}
	if h.Focused() != nil && Side(w) {
		w = MainWidth(w)
	}
	return InputWidth(w)
}

// HubView draws the whole screen at width, height rows. input is the text
// area as already drawn, InputWidth(HubMainWidth(width)) wide.
func HubView(h core.Hub, input, hint string, now time.Time, width, height int) HubFrame {
	height = max(height, 2)
	wide := HubWide(width)
	// What has the keys is what is drawn as active: core.HubSurface
	// decides it, and nothing here decides it again.
	switch surf := core.HubSurface(h); surf.Kind {
	case core.SurfaceReady, core.SurfaceReadyInput:
		return readyScreen(h, now, width, height)
	case core.SurfaceHelp:
		f := withHelp(h, input, hint, now, width, height, wide)
		f.Active = core.SurfaceHelp
		return f
	}
	return hubFrame(h, input, hint, now, width, height, wide, nil)
}

func withHelp(h core.Hub, input, hint string, now time.Time, width, height int, wide bool) HubFrame {
	mw := HubMainWidth(width)
	if wide {
		cw, _ := hubColumns(h, width)
		mw = framed(cw)
	}
	moving := !core.HubSurface(h).Covered().Typing()
	rows, title := TypingKeys, "typing keys"
	if moving {
		rows, title = MovingKeys, "moving keys"
	}
	help := helpBox(title, rows, mw)
	var legend []string
	if moving {
		legend = append(legend, styled(faint, truncate(paneHelp, InputWidth(mw))))
	}
	for _, l := range hubLegend {
		legend = append(legend, styled(faint, truncate(l, InputWidth(mw))))
	}
	help = append(help, legend...)
	return hubFrame(h, input, hint, now, width, height, wide, help)
}

func hubFrame(h core.Hub, input, hint string, now time.Time, width, height int, wide bool, help []string) HubFrame {
	surf := core.HubSurface(h)
	if wide {
		return panels(h, input, hint, now, width, height, help)
	}
	if surf.Column == core.ColumnPane && h.Focus == "" && help == nil {
		return paneScreen(h, now, width, height)
	}
	if surf.Column == core.ColumnList || h.Focus == "" {
		lines := listScreen(h, now, width, height)
		if help != nil {
			lines = append(lines[:max(height-len(help), 0)], help...)
			lines = lines[:min(len(lines), height)]
		}
		return HubFrame{Content: strings.Join(lines, "\n"), Active: core.SurfaceList}
	}
	return hubMain(h, input, hint, now, width, height, true, help)
}

// paneScreen is the narrow screen's pane with no role in focus: the
// list gave it the keys, and it takes the list's place.
func paneScreen(h core.Hub, now time.Time, width, height int) HubFrame {
	head := styled(faint, "‹ h roles")
	if e := elsewhere(h); e != "" {
		head += "  " + e
	}
	lines := []string{truncate(head, width)}
	var foot []string
	if h.Notice != "" {
		foot = append(foot, styled(faint, truncate("! "+oneLine(h.Notice), width)))
	}
	pane, mx := paneRender(h.App, core.State{}, now, width, max(height-1-len(foot), 0))
	lines = append(append(lines, pane...), foot...)
	return HubFrame{Content: strings.Join(lines[:min(len(lines), height)], "\n"), DetailMax: mx, Active: paneActive(h.App, core.State{})}
}

// elsewhere summarises the other roles that want the operator, for the
// narrow conversation's header.
func elsewhere(h core.Hub) string {
	unread := 0
	for _, r := range h.Roles {
		if r.ID == h.Focus {
			continue
		}
		if r.Conv != nil && r.Conv.Unread {
			unread++
		}
	}
	var parts []string
	if unread > 0 {
		parts = append(parts, styled(bold, fmt.Sprintf("● %d", unread)))
	}
	return strings.Join(parts, " ")
}

// hubMain is the conversation in focus: a header, the conversation
// bottom-aligned, then queued messages, the input box and the status
// line. help, when set, covers the
// bottom of the conversation.
func hubMain(h core.Hub, input, hint string, now time.Time, width, height int, narrow bool, help []string) HubFrame {
	// With room for it, the workspace pane stands beside the conversation,
	// as it does in the single-session view: a display is in front of
	// the operator without a key.
	if paneShown(h) && Side(width) {
		mw := MainWidth(width)
		f := hubMainAt(h, input, hint, now, mw, height, narrow, help, true)
		pane, mx := paneRender(h.App, focusedChat(h), now, PaneWidth(width), height)
		f.Content = strings.Join(joinColumns(strings.Split(f.Content, "\n"), pane, mw), "\n")
		f.DetailMax = mx
		if h.App.PaneFocus && help == nil {
			f.Active = paneActive(h.App, focusedChat(h)) // the pane is drawn with the keys
		}
		return f
	}
	return hubMainAt(h, input, hint, now, width, height, narrow, help, false)
}

// hubMainAt draws the conversation at width. beside says the pane is drawn
// beside it, so it neither covers the conversation nor needs its key named.
func hubMainAt(h core.Hub, input, hint string, now time.Time, width, height int, narrow bool, help []string, beside bool) HubFrame {
	return hubConv(h, input, hint, now, width, height, narrow, help, beside, false)
}

// hubConv draws the conversation at width. In a panel (inPanel) the header
// goes to the panel's border rather than a row of its own, and the input
// is a prompt under a rule rather than a box.
func hubConv(h core.Hub, input, hint string, now time.Time, width, height int, narrow bool, help []string, beside, inPanel bool) HubFrame {
	role := h.RoleByID(h.Focus)
	conv := h.Focused()
	var s core.State
	if conv != nil {
		s = conv.State
	}
	surf := core.HubSurface(h)
	moving := !surf.Covered().Typing()

	// The header.
	title := paneLine(h.Focus, true) // a name that came from the command line, not the config
	if role != nil {
		title = roleName(*role)
	}
	head := styled(bold, title)
	panelTitle := head
	if h.Focus == "" {
		panelTitle = "" // no role, so nothing to name: the frame goes unbroken
	}
	if narrow {
		head = styled(faint, "‹ h roles · ") + head
		if e := elsewhere(h); e != "" {
			head += "  " + e
		}
	}
	right := ""
	if conv != nil && role != nil && role.Conv != nil && role.Conv.Turns > 0 {
		head += styled(faint, " · "+plural(role.Conv.Turns, "turn"))
		right = plural(role.Conv.Turns, "turn")
	}
	scroll := 0

	// The bottom: queued messages, a note, the input or what replaces it,
	// and the status line.
	var bottom []string
	if n := embeddedNote(h); n != "" {
		for _, l := range capLines(wrap(n, width), 2, width) {
			bottom = append(bottom, styled(bold, l))
		}
	}
	for _, q := range s.Queue {
		first, _, more := strings.Cut(strings.TrimSpace(q.Text), "\n")
		first = oneLine(first)
		if more {
			first += "…"
		}
		bottom = append(bottom, styled(faint, truncate("queued › "+first, width)))
	}
	if h.Notice != "" {
		bottom = append(bottom, styled(faint, truncate("! "+oneLine(h.Notice), width)))
	}
	bottom = append(bottom, "")
	inputAt, x := 0, 0
	showInput := false
	active := core.SurfaceConversation
	switch {
	case surf.Kind == core.SurfaceEnd:
		ask := fmt.Sprintf("End the conversation with %s? y to end it, n to keep it", title)
		bottom = append(bottom, box("end conversation", wrap(ask, InputWidth(width)), width)...)
		active = core.SurfaceEnd
	case h.Focus == "":
		// No role in focus, so nothing to type into.
	default:
		if surf.Kind == core.SurfaceInput {
			active = core.SurfaceInput
		}
		if inPanel {
			bottom = append(bottom, styled(faint, strings.Repeat("─", width)))
			inputAt = len(bottom)
			bottom = append(bottom, prompt(strings.Split(input, "\n"))...)
		} else {
			inputAt = len(bottom)
			if boxed(width) {
				inputAt++
				x = 2
			}
			bottom = append(bottom, box("", strings.Split(input, "\n"), width)...)
		}
		showInput = true
	}
	var status string
	mode := ""
	if !inPanel {
		mode = modeTag(moving) + " · "
	}
	if h.Focus == "" {
		status = styled(faint, truncate("  "+mode+"no role in focus · h roles · ? keys", width))
	} else if conv == nil {
		start := "type a message to start one"
		if moving {
			start = "i to type a message and start one"
		}
		status = styled(faint, truncate("  "+mode+"no conversation · "+start+" · ? keys", width))
	} else {
		st := core.Status(h.App, s, now)
		if role != nil { // the roster may not have come yet
			if role.Conv != nil && role.Conv.Session == conv.Session {
				st.Remote = role.Conv.Turn
			}
		}
		st.PaneHint = st.PaneItems > 0 && !beside && surf.Column != core.ColumnPane
		st.ShowMode = !inPanel // the panels name it in the row of keys
		st.Moving = moving
		status = StatusText(st, hint, now, width)
	}
	bottom = append(bottom, status)

	// The conversation, or what covers it.
	headRows := 1
	if inPanel {
		headRows = 0
	}
	room := max(height-headRows-len(bottom), 0)
	if surf.Column == core.ColumnPane && !beside && showInput {
		// The pane covers the conversation and needs room for its question:
		// a tall draft gives up its top rows, down to one, as in View.
		if need := paneNeed(h.App, s, now, width); room < need {
			rows := len(strings.Split(input, "\n"))
			cut := min(need-room, rows-1)
			bottom = append(bottom[:inputAt], bottom[inputAt+cut:]...)
			room += cut
		}
	}
	var lines []string
	mx, detailMax := 0, 0
	switch {
	case surf.Column == core.ColumnPane && !beside:
		// The pane covers the conversation, or the place of one: the board
		// works with none open.
		lines, detailMax = paneRender(h.App, s, now, width, room)
		active = paneActive(h.App, s)
	case h.Focus == "":
		lines = []string{styled(faint, "No role in focus. Pick one from the roles.")}
	case conv == nil:
		lines = []string{styled(faint, "No conversation with "+title+" yet.")}
	default:
		ex := expand{tools: s.Transcript.Tools, thinking: s.Transcript.Thinking}
		lines = entriesAs(nil, s.Entries, ex, width, now)
		if conv.Truncated {
			// relay's word, not the agent's: set apart by a rule on each
			// side, in italics, where no agent text is.
			lines = append([]string{styled(faint|italic, truncate("— relay: earlier history is not shown —", width)), ""}, lines...)
		}
		mx = max(len(lines)-room, 0)
		scroll = min(s.Transcript.Scroll, mx)
		lines = lines[:len(lines)-scroll]
	}
	if scroll > 0 {
		head += styled(faint, fmt.Sprintf(" · ↓ %d more", scroll))
		right = strings.TrimPrefix(right+fmt.Sprintf(" · ↓ %d more", scroll), " · ")
	}
	if help != nil {
		lines = append(lines, help...)
		active = core.SurfaceHelp
	}
	if len(lines) > room {
		lines = lines[len(lines)-room:]
	}
	pad := make([]string, max(room-len(lines), 0))
	var top []string
	if !inPanel {
		top = []string{truncate(head, width)}
	}
	all := append(append(append(top, pad...), lines...), bottom...)
	y := headRows + len(pad) + len(lines) + inputAt
	if over := len(all) - height; over > 0 { // a screen too short for the bottom: the top goes
		all = all[over:]
		y -= over
	}
	for i := range all {
		all[i] = truncate(all[i], width)
	}
	return HubFrame{
		Content: strings.Join(all, "\n"), Max: mx, Page: max(room-1, 1), DetailMax: detailMax,
		Cursor: showInput && surf.Kind == core.SurfaceInput && y >= 0, InputX: x, InputY: max(y, 0),
		title: panelTitle, right: styled(faint, right), Active: active,
	}
}

// prompt is the input as drawn in a panel: the text area's rows, with
// its own prompt mark (two columns) in the accent colour, rather than a
// second mark beside it.
func prompt(rows []string) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		lead := "  "
		if i == 0 {
			lead = styled(accent, "›") + " "
		}
		out[i] = lead + Method.TruncateLeft(r, 2, "")
	}
	return out
}

// panels is wide hub mode: a top bar, the roles and the conversation in
// panels, the workspace pane beside them when there is room, and a row of
// key hints along the bottom.
func panels(h core.Hub, input, hint string, now time.Time, width, height int, help []string) HubFrame {
	mid := max(height-2, 2)
	sw := SidebarWidth(width)
	cw, pw := hubColumns(h, width)
	conv := h.Focused()
	var s core.State
	if conv != nil {
		s = conv.State
	}
	surf := core.HubSurface(h)
	convKeys := surf.Column == core.ColumnConversation

	f := hubConv(h, input, hint, now, framed(cw), mid-2, false, help, pw > 0, true)
	main := panel(f.title, f.right, "", strings.Split(f.Content, "\n"), cw, mid, convKeys)
	side := rolesPanel(h, now, sw, mid)
	keys := ""
	if pw > 0 {
		pane, mx, pk := paneRenderKeys(h.App, s, now, pw, mid)
		for i := range main {
			main[i] += " " + pane[i]
		}
		f.DetailMax = mx
		keys = pk
		if surf.Column == core.ColumnPane && help == nil {
			f.Active = paneActive(h.App, s) // the pane is drawn with the keys
		}
	}
	if surf.Column == core.ColumnList && help == nil {
		f.Active = core.SurfaceList
	}
	right := ""
	switch {
	case h.Focus != "":
		right = " · l conversation"
	case paneShown(h):
		right = " · l " + paneView(h.App)
	}
	pane := ""
	if paneShown(h) {
		pane = " · l " + paneView(h.App)
	}
	switch under := surf.Covered().Kind; {
	case under == core.SurfaceList:
		keys = "enter open · ↑↓ move" + right + " · 1 role · " + readyKey(h) + " · ? keys"
	case surf.Column == core.ColumnPane && pw > 0:
	case surf.Column == core.ColumnPane:
		keys = "" // the pane, in the conversation's place, lists its own
	case under == core.SurfaceEnd:
		keys = "y end it · n keep it"
	case under == core.SurfaceConversation && conv != nil:
		keys = "i type · h roles" + pane + " · ↑↓ scroll · X end · " + readyKey(h) + " · ? keys"
	case under == core.SurfaceConversation:
		keys = "i type · h roles" + pane + " · " + readyKey(h) + " · ? keys"
	default:
		keys = "enter send · esc move · alt+1 role · ctrl+t tools · ? keys"
	}
	rows := []string{topBar(h, now, width)}
	for i := 0; i < mid; i++ {
		rows = append(rows, side[i]+sgr(0)+main[i])
	}
	rows = append(rows, modeRow(!surf.Covered().Typing(), keys, width))
	for i := range rows {
		rows[i] = truncate(rows[i], width)
	}
	f.Content = strings.Join(rows[:min(len(rows), height)], "\n")
	f.InputX += sw + 2
	f.InputY += 2 // the top bar and the panel's top border
	return f
}

// readyKey is how the keys row names the ready view: R ready, with how
// many things are in it.
func readyKey(h core.Hub) string {
	if n := len(h.Ready.Items); n > 0 {
		return fmt.Sprintf("R ready %d", n)
	}
	return "R ready"
}

// modeRow is the row of keys along the bottom, led by the mode.
func modeRow(moving bool, keys string, width int) string {
	tag := " " + styled(bold, Mode(moving)) + " "
	if keys == "" {
		return truncate(tag, width)
	}
	return tag + hintRow(keys, max(width-Method.StringWidth(tag), 1))
}

// paneView names what l shows: the board, or the workspace items.
func paneView(a core.App) string {
	if a.Board.Items {
		return "workspace"
	}
	return "board"
}

// topBar is the screen's first row: relay, the role in focus and what
// the other roles want, and the time at the right.
func topBar(h core.Hub, now time.Time, width int) string {
	left := styled(accent, " ◆ ") + styled(bold, "relay")
	if r := h.RoleByID(h.Focus); r != nil {
		left += "   " + styled(faint, roleName(*r))
	}
	if e := elsewhere(h); e != "" {
		left += "   " + e
	}
	right := styled(faint, now.Format("Mon Jan 2 · 15:04")+" ")
	n := width - Method.StringWidth(left) - Method.StringWidth(right)
	if n < 1 {
		return truncate(left, width)
	}
	return left + strings.Repeat(" ", n) + right
}

// rolesPanel is the sidebar as a panel: one row per role, the one in
// focus (or under the cursor, while the list has the keys) on a tinted
// bar, and the marks at the right.
func rolesPanel(h core.Hub, now time.Time, width, height int) []string {
	inner := framed(width)
	var lines []string
	switch {
	case !h.RosterLoaded:
		lines = append(lines, styled(faint, "loading roles…"))
	case len(h.Roles) == 0:
		lines = append(lines, styled(faint, "no roles"))
	}
	var rows []string
	for _, r := range h.Roles {
		m := marks(h, r, now)
		title := truncate(roleName(r), max(inner-Method.StringWidth(m)-1, 1))
		switch {
		case r.ID == h.Focus:
			title = styled(bold, title)
		case r.Conv == nil:
			title = styled(faint, title)
		}
		l := title + strings.Repeat(" ", max(inner-Method.StringWidth(title)-Method.StringWidth(m), 1)) + m
		if (h.List && r.ID == h.Cursor) || (!h.List && r.ID == h.Focus) {
			l = barRow(l, inner)
		}
		rows = append(rows, l)
	}
	lines = append(lines, window(rows, 1, selected(h), max(height-2-len(lines), 0))...)
	count := ""
	if len(h.Roles) > 0 {
		count = styled(faint, fmt.Sprint(len(h.Roles)))
	}
	return panel(styled(bold, "Roles"), count, "", lines, width, height, core.HubSurface(h).Column == core.ColumnList)
}
