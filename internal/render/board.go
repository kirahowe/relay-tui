package render

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/kirahowe/relay-tui/internal/core"
)

// The board view of the workspace pane draws the cards relay sent. relay
// has already made every text safe: hidden characters made visible, and
// outside text defanged and flagged outsider. The board draws that text
// as it came, under an "untrusted" mark where it is outside text. It
// still escapes anything that does not print, and defangs a link left
// live in outside text, so a mistake upstream cannot reach the terminal. It never writes a hyperlink and never
// fetches anything.

// cardLine is a piece of card text on one line, safe to print.
func cardLine(t string, outsider bool) string {
	return strings.ReplaceAll(cardText(t, outsider), "\n", `\n`)
}

// cardText is card text that may run over several lines, safe to print.
// relay has defanged outside text already, so it shows as sent; only a
// link still live in it, which relay would never send, is defanged here.
func cardText(t string, outsider bool) string {
	t = core.EscapeNonPrinting(strings.ReplaceAll(strings.ReplaceAll(t, "\r\n", "\n"), "\t", "    "))
	if outsider && live(t) {
		t = Defang(t)
	}
	return t
}

// cardLines is cardText wrapped to width, keeping its line breaks.
func cardLines(t string, outsider bool, width int) []string {
	var out []string
	for _, l := range strings.Split(cardText(t, outsider), "\n") {
		if l == "" {
			out = append(out, "")
			continue
		}
		out = append(out, wrap(l, width)...)
	}
	return out
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
func cardTitle(c core.Card) string {
	if strings.TrimSpace(c.Title.Text) == "" {
		return styled(faint, "(untitled)")
	}
	t := cardLine(c.Title.Text, c.Title.Outsider)
	if c.Title.Outsider {
		return styled(bold|warnColour, t)
	}
	return styled(bold|kindColour(c.Kind), t)
}

// cardRight is what a card's panel says at the right of its title: how
// far along it is, or its kind.
func cardRight(b core.Board, c core.Card) string {
	var w string
	switch {
	case b.Sending == c.ID:
		w = "sending…"
	case c.Complete:
		w = "complete"
	case c.Kind == core.CardChecklist || c.Kind == core.CardTracker:
		w = fmt.Sprintf("%d of %d", c.Done, c.Total)
	default:
		w = cardLine(c.Kind, true)
	}
	return styled(faint, w)
}

// cardFoot is what the bottom border of a card's panel says: that some
// of its text came from outside, and, on the card that has the keys, that
// t trusts it.
func cardFoot(c core.Card, focus bool) string {
	if !c.Labels.Contaminated {
		return ""
	}
	f := styled(warnColour, "outside text")
	if focus && c.SHA256 != "" {
		f += styled(faint, " · t trust")
	}
	return f
}

// trustKey is the hint for t on card c, when t trusts it from here.
func trustKey(c core.Card) string {
	if c.Labels.Contaminated && c.SHA256 != "" {
		return "t trust · "
	}
	return ""
}

// listItems is how many of a checklist's items the board's list shows,
// and how many of a note's lines.
const listItems, listNoteLines = 6, 12

// checkLines is a checklist item: a green tick and struck-through text
// when it is done, a ring when it is not, the text hanging beside it.
func checkLines(it core.CardElem, width int) []string {
	if it.Done {
		return hang(styled(okColour, "✓")+" ", outside(it.CardText, styled(faint|strike, cardLine(it.Text, it.Outsider))), width)
	}
	return hang(styled(faint, "○")+" ", outside(it.CardText, cardLine(it.Text, it.Outsider)), width)
}

var (
	noteHeading = regexp.MustCompile(`^#{1,6}\s+(.*)$`)
	noteItem    = regexp.MustCompile(`^(\s*)([-*•]|[0-9]{1,3}[.)])\s+(.*)$`)
	noteBold    = regexp.MustCompile(`\*\*([^*]+)\*\*`)
)

// noteLines draws a note's text, width wide, line by line as it was
// written: every line break stays. A # heading is bold, **text** is bold,
// and a list line's marker is faint with its text hanging beside it.
// text is safe to print already (cardText).
func noteLines(text string, width int) []string {
	inline := func(s string) string {
		return noteBold.ReplaceAllStringFunc(s, func(m string) string {
			return styled(bold, noteBold.FindStringSubmatch(m)[1])
		})
	}
	var out []string
	for _, l := range strings.Split(text, "\n") {
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
func trackerGrid(c core.Card, width int, line, col int, focus bool) (out []string, selA, selB int, ok bool) {
	const colMax = 6
	nameW := 0
	for _, r := range c.Rows {
		nameW = max(nameW, ansi.StringWidth(cardLine(r.Text, r.Outsider)))
	}
	nameW = min(nameW, max(width/3, 8))
	cols := make([]int, len(c.Columns))
	total := nameW
	for i, k := range c.Columns {
		cols[i] = min(max(ansi.StringWidth(cardLine(k.Text, k.Outsider)), 1), colMax)
		total += 1 + cols[i]
	}
	if total > width {
		return nil, 0, 0, false
	}
	head := strings.Repeat(" ", nameW)
	for i, k := range c.Columns {
		name := truncate(strings.ToUpper(cardLine(k.Text, k.Outsider)), cols[i])
		name += strings.Repeat(" ", cols[i]-ansi.StringWidth(name))
		st := faint
		if focus && i == col {
			st = accent | bold
		}
		head += " " + outside(k.CardText, styled(st, name))
	}
	out = append(out, head)
	for i, r := range c.Rows {
		name := truncate(cardLine(r.Text, r.Outsider), nameW)
		l := outside(r.CardText, name) + strings.Repeat(" ", nameW-ansi.StringWidth(name))
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
func cardLinesFor(b core.Board, c core.Card, width int, full, focus bool) (out []string, selA, selB int) {
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
			add(i+1, checkLines(it, width))
		}
	case core.CardTracker:
		if len(c.Rows) == 0 || len(c.Columns) == 0 {
			out = append(out, styled(faint, "no rows or columns yet"))
			break
		}
		if grid, a, z, ok := trackerGrid(c, width, line, col, full && focus); ok {
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
				cells = append(cells, outside(k.CardText, cardLine(k.Text, k.Outsider))+" "+cellGlyph(c.CellState(r.ID, k.ID)))
			}
			name := hang("", outside(r.CardText, cardLine(r.Text, r.Outsider)), width)
			add(i+1, append(name, hang("  ", strings.Join(cells, styled(faint, " · ")), width)...))
		}
	case core.CardNote:
		var lines []string
		if c.Note != nil {
			lines = noteLines(cardText(c.Note.Text, c.Note.Outsider), width)
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
	default:
		// A kind this client does not know shows relay's text form.
		lines := cardLines(c.Text, c.Labels.Contaminated, width)
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
func cardPanel(b core.Board, c core.Card, width int, full, focus bool) (out []string, selA, selB int) {
	body, a, z := cardLinesFor(b, c, framed(width), full, focus)
	title := cardTitle(c)
	if full && focus && core.BoardLine(b, c) == 0 {
		title = tint(title, selBar)
	}
	out = panel(title, cardRight(b, c), cardFoot(c, focus), body, width, len(body)+2, focus)
	if full && focus {
		if core.BoardLine(b, c) == 0 {
			return out, 0, 1
		}
		return out, a + 1, z + 1
	}
	return out, 0, len(out)
}

// cellLine names the selected tracker cell and its state.
func cellLine(b core.Board, c core.Card, width int) []string {
	line, col := core.BoardLine(b, c), core.BoardCol(b, c)
	if c.Kind != core.CardTracker || line == 0 || col < 0 {
		return nil
	}
	r, k := c.Rows[line-1], c.Columns[col]
	st := c.CellState(r.ID, k.ID)
	return hang("", styled(faint, "cell: ")+outside(r.CardText, cardLine(r.Text, r.Outsider))+styled(faint, " × ")+
		outside(k.CardText, cardLine(k.Text, k.Outsider))+styled(faint, " · ")+cardLine(st, true), width)
}

// cardName is how a prompt names a card: its title, quoted.
func cardName(b core.Board, id int64) string {
	for _, c := range b.Cards {
		if c.ID == id && strings.TrimSpace(c.Title.Text) != "" {
			return "“" + cardLine(c.Title.Text, c.Title.Outsider) + "”"
		}
	}
	return "card " + strconv.FormatInt(id, 10)
}

// boardParts lays out the board view.
func boardParts(a core.App, s core.State, now time.Time, width int) parts {
	b := a.Board
	focus := a.PaneFocus
	var pr parts
	count := -1
	if b.Loaded {
		count = len(b.Cards)
	}
	pr.head = paneHead("Board", count, fmt.Sprintf("workspace %d", len(core.PaneItems(s))), focus, width)
	hint := func(s string) []string { return wrap(styled(faint, s), width) }

	c, sel := core.BoardCard(b)
	switch {
	case b.Unavailable:
		pr.body = []string{styled(faint, "This agent has no board.")}
	case !b.Loaded && (b.Asked || !a.Live):
		pr.body = []string{styled(faint, "loading…")}
	case !b.Loaded:
		pr.body = []string{styled(faint, "not loaded: r to load")}
	case b.Trust != nil:
		// What the operator vouches for: the whole card, as it is now.
		if i := slices.IndexFunc(b.Cards, func(x core.Card) bool { return x.ID == b.Trust.Card }); i >= 0 {
			pr.body, _, _ = cardPanel(b, b.Cards[i], width, true, false)
		}
	case b.Detail && sel >= 0:
		pr.body, pr.selA, pr.selB = cardPanel(b, c, width, true, focus)
	case len(b.Cards) == 0:
		pr.body = []string{styled(faint, "no cards")}
	default:
		for i, x := range b.Cards {
			if i > 0 {
				pr.body = append(pr.body, "")
			}
			lines, _, _ := cardPanel(b, x, width, false, focus && i == sel)
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
		pr.foot = hang("", styled(bold, cardLine(in.Prompt, false)+" › ")+cardLine(in.Text, false)+styled(reverse, " "), width)
		pr.foot = append(pr.foot, hint("enter send · esc cancel")...)
	case b.Confirm != 0:
		pr.foot = wrap(styled(bold, "Archive "+cardName(b, b.Confirm)+"?")+styled(faint, " y archive · any other key cancels"), width)
	case b.Trust != nil:
		pr.foot = wrap(styled(bold, "Trust "+cardName(b, b.Trust.Card)+"? It clears outside text from what it says now."), width)
		switch {
		case b.Trust.Sent:
			pr.foot = append(pr.foot, hint("trusting…")...)
		case now.Sub(b.Trust.Since) >= core.ArmDelay:
			pr.foot = append(pr.foot, wrap(styled(bold|accent, "y trust")+styled(faint, " · any other key cancels"), width)...)
		default:
			pr.foot = append(pr.foot, hint("y trust (wait a moment) · any other key cancels")...)
		}
	case b.Detail && sel >= 0:
		pr.foot = cellLine(b, c, width)
		switch c.Kind {
		case core.CardChecklist:
			pr.keys = "↑↓ select · x tick · a add · e edit · " + trustKey(c) + "X archive · r reload · b workspace · esc back"
		case core.CardTracker:
			pr.keys = "↑↓←→ select · x mark · a row · A column · e rename · E rename column · " + trustKey(c) + "X archive · r reload · b workspace · esc back"
		default:
			pr.keys = "e retitle · " + trustKey(c) + "X archive · r reload · b workspace · esc back"
		}
	case len(b.Cards) > 0:
		pr.keys = "↑↓ select · enter open · e retitle · " + trustKey(c) + "X archive · r reload · b workspace · esc back"
	case b.Unavailable:
		// r would not ask again: the agent has no such method.
		pr.keys = "b workspace · esc back"
	default:
		pr.keys = "r reload · b workspace · esc back"
	}
	if b.Message != "" {
		msg := wrap(styled(bold, cardLine(b.Message, false)), width)
		if b.Failure != "" {
			msg = append(msg, hang("  ", cardLine(b.Failure, true), width)...)
		}
		pr.foot = append(msg, pr.foot...)
	}
	return pr
}
