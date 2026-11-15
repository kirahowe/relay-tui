package render

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/kirahowe/relay-tui/internal/a2ui"
	"github.com/kirahowe/relay-tui/internal/core"
)

// The board view of the workspace pane draws the cards relay sent. relay
// has already made every text safe: hidden characters made visible, and
// outside text defanged and flagged outsider. The board draws that text
// as it came, in the warning colour and under "from outside" where a card
// holds any. It still escapes anything that does not print, and breaks
// every link in outside text but relay's live links (links.go), so a
// mistake upstream cannot reach the terminal. It never fetches anything.

// cardLine is a piece of card text on one line, safe to print.
func cardLine(t string, outsider bool, live core.Links) string {
	return strings.ReplaceAll(cardText(t, outsider, live), "\n", `\n`)
}

// cardText is text relay sent, which may run over several lines, safe to
// print: a card's, a ready item's, or any other that comes with relay's
// word on whether an outsider may have written it. Outside text has its
// links broken here whatever relay did to it, all but the live links
// relay sent this connection, which become link spans (fang). Text that
// is not from outside shows as sent.
func cardText(t string, outsider bool, live core.Links) string {
	t = core.EscapeNonPrinting(strings.ReplaceAll(strings.ReplaceAll(t, "\r\n", "\n"), "\t", "    "))
	if outsider {
		t = fang(t, live)
	}
	return t
}

// cardLines is cardText wrapped to width, keeping its line breaks.
func cardLines(t string, outsider bool, live core.Links, width int) []string {
	var out []string
	for _, l := range strings.Split(cardText(t, outsider, live), "\n") {
		if l == "" {
			out = append(out, "")
			continue
		}
		out = append(out, wrap(l, width)...)
	}
	return drawn(out, outsider)
}

// drawn is the last step for rows of outside text, after everything that
// decides what they show: a row that shows something a terminal could
// follow as text is shown plain and broken (defangLines). Rows that are
// not from outside pass as they are.
func drawn(rows []string, outsider bool) []string {
	if !outsider {
		return rows
	}
	return defangLines(rows)
}

// outside colours a card text from outside, so it stands out from the
// operator's own. relay has defanged it already.
func outside(t core.CardText, s string) string {
	if t.Outsider {
		return recolour(s, warnColour)
	}
	return s
}

// recolour sets the text colour of every part of an already styled line.
func recolour(line string, colour style) string {
	var b strings.Builder
	b.WriteString(sgr(colour))
	rest := line
	for {
		loc := sgrRE.FindStringIndex(rest)
		if loc == nil {
			b.WriteString(rest)
			break
		}
		b.WriteString(rest[:loc[0]])
		cur := parseSGR(rest[loc[0]:loc[1]])
		b.WriteString(sgr(cur&^(0xff<<fgShift) | colour))
		rest = rest[loc[1]:]
	}
	b.WriteString(sgr(0))
	return b.String()
}

// kindColour is the colour of a card's title: one per kind.
func kindColour(kind string) style {
	switch kind {
	case core.CardChecklist:
		return okColour
	case core.CardTracker:
		return fg(hueMagenta)
	case core.CardNote:
		return fg(hueBlue)
	}
	return 0
}

// cardTitle is the card's title for its panel's border: bold, in its
// kind's colour or as outside text, or a stand-in when it is empty.
func cardTitle(c core.Card, live core.Links) string {
	live = live.Of(c.Links)
	if strings.TrimSpace(c.Title.Text) == "" {
		return styled(faint, "(untitled)")
	}
	t := cardLine(c.Title.Text, c.Title.Outsider, live)
	if c.Title.Outsider {
		return styled(bold|warnColour, t)
	}
	return styled(bold|kindColour(c.Kind), t)
}

// cardRight is what a card's panel says at the right of its title: how
// far along it is, or its kind. A surface card's says whose it is.
func cardRight(b core.Board, c core.Card) string {
	var w string
	switch {
	case c.Kind == core.CardSurface && c.Surface != nil:
		// The frame is the client's, so it names whose card this is, in
		// place of the kind, and it does so in every state: the owner
		// comes first and anything else after it.
		w = surfaceOwner(c)
		if b.Sending == c.ID {
			w += " · sending…"
		}
	case b.Sending == c.ID:
		w = "sending…"
	case c.Complete:
		w = "complete"
	case c.Kind == core.CardChecklist || c.Kind == core.CardTracker:
		w = fmt.Sprintf("%d of %d", c.Done, c.Total)
	default:
		w = cardLine(c.Kind, true, core.Links{})
	}
	return styled(faint, w)
}

// surfaceOwner is what a surface card's corner calls its owner. The owner
// is a string from relay with no mark of its source, so it is drawn as
// outside text on one line. One that draws as nothing names nobody, and
// the kind stands in for it.
func surfaceOwner(c core.Card) string {
	if o := cardLine(c.Owner, true, core.Links{}); Method.StringWidth(o) > 0 {
		return o
	}
	return "surface"
}

// cardFoot is what the bottom border of a card's panel says: that some
// of its text came from outside. A surface card says it too when its
// surface or its title may have been written there, whatever its labels.
func cardFoot(c core.Card) string {
	if p := core.Provenance(c.Labels, nil); p != "" {
		return styled(warnColour, p)
	}
	if c.Kind == core.CardSurface && c.Surface != nil && (c.Surface.Outsider || c.Title.Outsider) {
		return styled(warnColour, core.Provenance(core.Labels{Contaminated: true}, nil))
	}
	return ""
}

// listItems is how many of a checklist's items the board's list shows,
// and how many of a note's lines.
const listItems, listNoteLines = 6, 12

// checkLines is a checklist item: a green tick and struck-through text
// when it is done, a ring when it is not, the text hanging beside it.
func checkLines(it core.CardElem, live core.Links, width int) []string {
	if it.Done {
		return drawn(hang(styled(okColour, "✓")+" ", outside(it.CardText, styled(faint|strike, cardLine(it.Text, it.Outsider, live))), width), it.Outsider)
	}
	return drawn(hang(styled(faint, "○")+" ", outside(it.CardText, cardLine(it.Text, it.Outsider, live)), width), it.Outsider)
}

var (
	noteHeading = regexp.MustCompile(`^#{1,6}\s+(.*)$`)
	noteItem    = regexp.MustCompile(`^(\s*)([-*•]|[0-9]{1,3}[.)])\s+(.*)$`)
	noteBold    = regexp.MustCompile(`\*\*([^*]+)\*\*`)
)

// noteLines draws a note's text, width wide, line by line as it was
// written: every line break stays.
//
// The operator's own note is dressed: a # heading is bold, **text** is
// bold, and a list line's marker is faint with its text hanging beside
// it. Text an outsider may have written is never dressed. It is drawn as
// relay sent it, mark for mark (cardLines), because dressing takes
// characters away, and what is left can read as an address relay never
// saw: "ev**il.**example/x" would show as evil.example/x. What relay
// checked is then what is on the screen.
func noteLines(text string, outsider bool, live core.Links, width int) []string {
	if outsider {
		return cardLines(text, true, live, width)
	}
	inline := func(s string) string {
		return noteBold.ReplaceAllStringFunc(s, func(m string) string {
			return styled(bold, noteBold.FindStringSubmatch(m)[1])
		})
	}
	var out []string
	for _, l := range strings.Split(cardText(text, false, live), "\n") {
		switch {
		case strings.TrimSpace(l) == "":
			out = append(out, "")
		case noteHeading.MatchString(l):
			out = append(out, wrap(styled(bold, noteHeading.FindStringSubmatch(l)[1]), width)...)
		case noteItem.MatchString(l):
			m := noteItem.FindStringSubmatch(l)
			lead := strings.Repeat(" ", min(len(m[1]), 8)) + styled(faint, m[2]) + " "
			out = append(out, hang(lead, inline(m[3]), width)...)
		default:
			out = append(out, wrap(inline(l), width)...)
		}
	}
	return out
}

// cellGlyph is how a tracker cell shows: a dot when done, a cross when
// skipped, a ring when open.
func cellGlyph(state string) string {
	switch state {
	case core.CellDone:
		return styled(fg(hueMagenta), "●")
	case core.CellSkip:
		return styled(faint, "×")
	case core.CellOpen:
		return styled(faint, "○")
	}
	return "?"
}

// trackerGrid is a tracker as a grid: the column names across the top,
// one row per line with its cells under them. Each cell is as wide as its
// column's name, up to colMax. It is nil when the grid is wider than
// width, and the caller lists the tracker another way. sel and col pick
// the selected row (1 on) and column (0 on), when focus is on the card.
func trackerGrid(c core.Card, live core.Links, width int, line, col int, focus bool) (out []string, selA, selB int, ok bool) {
	live = live.Of(c.Links)
	const colMax = 6
	nameW := 0
	for _, r := range c.Rows {
		nameW = max(nameW, Method.StringWidth(cardLine(r.Text, r.Outsider, live)))
	}
	nameW = min(nameW, max(width/3, 8))
	cols := make([]int, len(c.Columns))
	total := nameW
	for i, k := range c.Columns {
		cols[i] = min(max(Method.StringWidth(cardLine(k.Text, k.Outsider, live)), 1), colMax)
		total += 1 + cols[i]
	}
	if total > width {
		return nil, 0, 0, false
	}
	head := strings.Repeat(" ", nameW)
	for i, k := range c.Columns {
		// A column of the operator's own is named in capitals. An
		// outsider's is drawn as it was sent, like all outside text.
		text := k.Text
		if !k.Outsider {
			text = strings.ToUpper(text)
		}
		name := truncate(cardLine(text, k.Outsider, live), cols[i])
		name += strings.Repeat(" ", cols[i]-Method.StringWidth(name))
		st := faint
		if focus && i == col {
			st = accent | bold
		}
		head += " " + outside(k.CardText, styled(st, name))
	}
	out = append(out, head)
	for i, r := range c.Rows {
		name := truncate(cardLine(r.Text, r.Outsider, live), nameW)
		l := outside(r.CardText, name) + strings.Repeat(" ", nameW-Method.StringWidth(name))
		for j, k := range c.Columns {
			g := cellGlyph(c.CellState(r.ID, k.ID))
			if focus && i+1 == line && j == col {
				g = styled(reverse, ansi.Strip(g))
			}
			l += " " + g + strings.Repeat(" ", cols[j]-1)
		}
		if focus && i+1 == line {
			selA, selB = len(out), len(out)+1
			l = barRow(l, width)
		}
		out = append(out, l)
	}
	return out, selA, selB, true
}

// cardLinesFor is the inside of a card's panel, width wide. In the
// board's list (full false) a long checklist or note is cut short. In a
// card's detail (full true) everything shows, and with focus the selected
// line is on a tinted bar; selA and selB are its rows.
func cardLinesFor(b core.Board, c core.Card, live core.Links, width int, full, focus bool) (out []string, selA, selB int) {
	live = live.Of(c.Links)
	line := core.BoardLine(b, c)
	col := core.BoardCol(b, c)
	add := func(i int, lines []string) {
		if full && focus && i == line {
			selA = len(out)
			for k := range lines {
				lines[k] = barRow(lines[k], width)
			}
		}
		out = append(out, lines...)
		if full && focus && i == line {
			selB = len(out)
		}
	}
	switch c.Kind {
	case core.CardChecklist:
		if len(c.Items) == 0 {
			out = append(out, styled(faint, "no items yet"))
		}
		for i, it := range c.Items {
			if !full && i == listItems && len(c.Items) > listItems+1 {
				out = append(out, styled(faint, fmt.Sprintf("+%d more", len(c.Items)-listItems)))
				break
			}
			add(i+1, checkLines(it, live, width))
		}
	case core.CardTracker:
		if len(c.Rows) == 0 || len(c.Columns) == 0 {
			out = append(out, styled(faint, "no rows or columns yet"))
			break
		}
		if grid, a, z, ok := trackerGrid(c, live, width, line, col, full && focus); ok {
			if full && focus && line > 0 {
				selA, selB = len(out)+a, len(out)+z
			}
			out = append(out, grid...)
			break
		}
		// Too wide for a grid: each row's name, then its cells by column.
		for i, r := range c.Rows {
			var cells []string
			for _, k := range c.Columns {
				cells = append(cells, outside(k.CardText, cardLine(k.Text, k.Outsider, live))+" "+cellGlyph(c.CellState(r.ID, k.ID)))
			}
			name := hang("", outside(r.CardText, cardLine(r.Text, r.Outsider, live)), width)
			add(i+1, append(name, hang("  ", strings.Join(cells, styled(faint, " · ")), width)...))
		}
	case core.CardNote:
		var lines []string
		if c.Note != nil {
			lines = noteLines(c.Note.Text, c.Note.Outsider, live, width)
			if c.Note.Outsider {
				for i := range lines {
					lines[i] = recolour(lines[i], warnColour)
				}
			}
		}
		if !full && len(lines) > listNoteLines {
			lines = append(lines[:listNoteLines-1], styled(faint, fmt.Sprintf("+%d more lines", len(lines)-listNoteLines+1)))
		}
		out = append(out, lines...)
	case core.CardSurface:
		if c.Surface == nil {
			break
		}
		var f *a2ui.Focus
		if full && focus && line > 0 {
			f = &a2ui.Focus{ID: c.Inputs()[line-1].ID}
		}
		lines, a, z := surfaceRows(c.Surface.A2UI, c.Surface.Outsider, live, width, f)
		if f != nil {
			selA, selB = len(out)+a, len(out)+z
		}
		if !full && len(lines) > listNoteLines {
			lines = append(lines[:listNoteLines-1], styled(faint, fmt.Sprintf("+%d more lines", len(lines)-listNoteLines+1)))
		}
		out = append(out, lines...)
	default:
		// A kind this client does not know shows relay's text form.
		lines := cardLines(c.Text, c.Labels.Contaminated, live, width)
		if !full && len(lines) > listNoteLines {
			lines = lines[:listNoteLines]
		}
		out = append(out, lines...)
	}
	return out, selA, selB
}

// cardPanel is a card in its panel, width wide, as tall as it needs. Its
// frame is in the accent colour when it has the keys; in a card's detail,
// with the title the selected line, the title sits on a tinted bar.
func cardPanel(b core.Board, c core.Card, live core.Links, width int, full, focus bool) (out []string, selA, selB int) {
	live = live.Of(c.Links)
	body, a, z := cardLinesFor(b, c, live, framed(width), full, focus)
	title := cardTitle(c, live)
	right := cardRight(b, c)
	if c.Kind == core.CardSurface && c.Surface != nil {
		// The corner names whose card this is, so the title gives way to
		// it, down to a few columns. With too little room for the rest
		// of the corner as well, the name is what stays.
		const least = 8
		if width-10-Method.StringWidth(right) < least {
			right = styled(faint, surfaceOwner(c))
		}
		title = truncate(title, max(width-10-Method.StringWidth(right), least))
	}
	if full && focus && core.BoardLine(b, c) == 0 {
		title = tint(title, selBar)
	}
	out = panel(title, right, cardFoot(c), body, width, len(body)+2, focus)
	if full && focus {
		if core.BoardLine(b, c) == 0 {
			return out, 0, 1
		}
		return out, a + 1, z + 1
	}
	return out, 0, len(out)
}

// cellLine names the selected tracker cell and its state.
func cellLine(b core.Board, c core.Card, live core.Links, width int) []string {
	live = live.Of(c.Links)
	line, col := core.BoardLine(b, c), core.BoardCol(b, c)
	if c.Kind != core.CardTracker || line == 0 || col < 0 {
		return nil
	}
	r, k := c.Rows[line-1], c.Columns[col]
	st := c.CellState(r.ID, k.ID)
	return hang("", styled(faint, "cell: ")+outside(r.CardText, cardLine(r.Text, r.Outsider, live))+styled(faint, " × ")+
		outside(k.CardText, cardLine(k.Text, k.Outsider, live))+styled(faint, " · ")+cardLine(st, true, core.Links{}), width)
}

// cardName is how a prompt names a card: its title, quoted.
func cardName(b core.Board, id int64) string {
	for _, c := range b.Cards {
		if c.ID == id && strings.TrimSpace(c.Title.Text) != "" {
			return "“" + cardLine(c.Title.Text, c.Title.Outsider, core.Links{}) + "”"
		}
	}
	return "card " + strconv.FormatInt(id, 10)
}

// boardParts lays out the board view.
func boardParts(a core.App, s core.State, now time.Time, width int) parts {
	b := a.Board
	live := a.Links
	focus := a.PaneFocus
	var pr parts
	count := -1
	if b.Loaded {
		count = len(b.Cards)
	}
	pr.head = paneHead("Board", count, fmt.Sprintf("workspace %d", len(core.PaneItems(s))), focus, width)
	hint := func(s string) []string { return wrap(styled(faint, s), width) }

	c, sel := core.BoardCard(b)
	// The list of a card's links takes the pane's body while it is open.
	linksCard, links, listing := core.BoardLinks(b, live)
	// L lists the links of a card whose outside text holds any.
	linksKey := ""
	if sel >= 0 && len(core.CardLinks(c, live)) > 0 {
		linksKey = "L links · "
	}
	switch {
	case b.Unavailable:
		pr.body = []string{styled(faint, "This agent has no board.")}
	case !b.Loaded && (b.Asked || !a.Live):
		pr.body = []string{styled(faint, "loading…")}
	case !b.Loaded:
		pr.body = []string{styled(faint, "not loaded: r to load")}
	case listing:
		rows, selA, selB := linkRows(links, b.Links.Sel, live, framed(width))
		legend := linkLegend(links, linksCard.LinksLeftOut)
		pr.body = panel(linkHead(cardLine(linksCard.Title.Text, linksCard.Title.Outsider, live.Of(linksCard.Links)), legend, width), styled(faint, legend),
			cardFoot(linksCard), rows, width, len(rows)+2, focus)
		pr.selA, pr.selB = selA+1, selB+1
	case b.Detail && sel >= 0:
		pr.body, pr.selA, pr.selB = cardPanel(b, c, live, width, true, focus)
	case len(b.Cards) == 0:
		pr.body = []string{styled(faint, "no cards")}
	default:
		for i, x := range b.Cards {
			if i > 0 {
				pr.body = append(pr.body, "")
			}
			lines, _, _ := cardPanel(b, x, live, width, false, focus && i == sel)
			if i == sel {
				pr.selA, pr.selB = len(pr.body), len(pr.body)+len(lines)
			}
			pr.body = append(pr.body, lines...)
		}
	}

	switch {
	case !focus:
		pr.keys = PaneKey + " to focus, then b for the workspace"
	case b.Input != nil:
		in := b.Input
		pr.foot = hang("", styled(bold, cardLine(in.Prompt, false, live)+" › ")+cardLine(in.Text, false, live)+styled(reverse, " "), width)
		pr.foot = append(pr.foot, hint("enter send · esc cancel")...)
	case b.Confirm != 0:
		pr.foot = hang("", styled(bold, "Archive "+cardName(b, b.Confirm)+"?"), width)
		pr.foot = append(pr.foot, hint("y archive · any other key keeps it")...)
	case listing && b.Links.Ask != nil:
		pr.foot = hang("", styled(bold, linkQuestion(*b.Links.Ask)), width)
		pr.foot = append(pr.foot, askedRows(*b.Links.Ask, live, width, 2)...)
		pr.foot = append(pr.foot, hint(linkAskHint(*b.Links.Ask))...)
	case listing:
		pr.keys = linkListHint(*b.Links, links, "esc back")
	case b.Detail && sel >= 0:
		pr.foot = cellLine(b, c, live, width)
		switch c.Kind {
		case core.CardChecklist:
			pr.keys = "↑↓ select · x tick · a add · e edit · " + linksKey + "X archive · r reload · b workspace · esc back"
		case core.CardTracker:
			pr.keys = "↑↓←→ select · x mark · a row · A column · e rename · E rename column · " + linksKey + "X archive · r reload · b workspace · esc back"
		case core.CardSurface:
			if len(c.Inputs()) > 0 {
				pr.keys = "↑↓ select · x tick · e retitle · " + linksKey + "X archive · r reload · b workspace · esc back"
			} else {
				pr.keys = "e retitle · " + linksKey + "X archive · r reload · b workspace · esc back"
			}
		default:
			pr.keys = "e retitle · " + linksKey + "X archive · r reload · b workspace · esc back"
		}
	case len(b.Cards) > 0:
		pr.keys = "↑↓ select · enter open · e retitle · " + linksKey + "X archive · r reload · b workspace · esc back"
	case b.Unavailable:
		// r would not ask again: the agent has no such method.
		pr.keys = "b workspace · esc back"
	default:
		pr.keys = "r reload · b workspace · esc back"
	}
	if b.Message != "" {
		msg := wrap(styled(bold, cardLine(b.Message, false, live)), width)
		if b.Failure != "" {
			msg = append(msg, hang("  ", cardLine(b.Failure, true, core.Links{}), width)...)
		}
		pr.foot = append(msg, pr.foot...)
	}
	return pr
}
