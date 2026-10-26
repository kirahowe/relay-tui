package render

import (
	"fmt"
	"math"
	"strconv"

	"time"

	"github.com/kirahowe/relay-tui/internal/core"
)

// StatusText formats the status line: faint, one row, at most width wide.
// While the agent works it starts with a spinner, so a moving spinner
// means the TUI is alive, and "no output for" means the agent is quiet.
// hint says how to type a new line; it is shown when idle. The tool
// running, context use and spend show only with the detail toggle on
// (StatusLine.Detail), and once the agent has reported them.
func StatusText(l core.StatusLine, hint string, now time.Time, width int) string {
	var parts []string
	add := func(s string) {
		if s != "" {
			parts = append(parts, s)
		}
	}
	lead := spinner(now)
	switch l.Kind {
	case core.StatusStarting:
		add("starting")
	case core.StatusIdle:
		lead = " "
		add("ready")
		if l.Remote == core.TurnRunning {
			lead = spinner(now)
			parts = []string{"replying"}
		}
	case core.StatusExited:
		lead = " "
		add("the agent exited")
	case core.StatusWaiting:
		add("waiting")
	case core.StatusThinking:
		add("thinking")
	case core.StatusWriting:
		add("writing")
	case core.StatusRunning:
		switch {
		case !l.Detail:
			add("working")
		case l.Tools == 1:
			add("running " + oneLine(l.Tool))
		default:
			add(fmt.Sprintf("running %d tools", l.Tools))
		}
	case core.StatusRetrying:
		add("retrying in " + Duration((l.RetryIn + time.Second - 1).Truncate(time.Second))) // round up: never "0s" with time left
		add(oneLine(l.Reason))
	case core.StatusCancelling:
		lead = " "
		add("cancelling")
	}
	add(oneLine(l.Message))
	if l.Remote == core.TurnQueued {
		add("waiting for the role's scheduled work")
	}
	if l.PaneHint {
		add(fmt.Sprintf("%s %s (%d)", PaneKey, l.PaneView, l.PaneItems))
	}
	if l.Kind != core.StatusIdle && l.Kind != core.StatusStarting && l.Kind != core.StatusExited && !l.NoClock {
		add(Duration(l.Elapsed))
	}
	if l.Quiet > 0 {
		add("no output for " + Duration(l.Quiet))
	}
	if l.Queued > 0 {
		add(strconv.Itoa(l.Queued) + " queued")
	}
	if l.Detail && l.HasContext {
		add(strconv.Itoa(l.Context) + "% context")
	}
	if l.Detail && l.Spend != nil {
		add(Money(*l.Spend))
	}
	idle := l.Kind == core.StatusIdle && l.Remote != core.TurnRunning
	if idle {
		if l.Moving {
			add(core.UnboundNote)
		} else {
			add(hint)
		}
	}
	mode := -1
	if l.ShowMode {
		// On the line after what the agent is doing and how to type, but
		// kept ahead of them when the line is short (see below).
		mode = len(parts)
		add(modeTag(l.Moving))
	}
	if idle {
		add("? keys")
	}
	// The first part always shows, cut if it must be. The mode comes next: it shows whole whenever it fits
	// after it, whatever else would have, since what a key does depends
	// on it. Each other part shows only if it fits whole, so a narrow
	// terminal loses the least important ones instead of showing half a
	// word. The parts keep their order on the line.
	if len(parts) == 0 {
		parts = []string{""}
	}
	sep := Method.StringWidth(" · ")
	first := truncate(lead+" "+parts[0], width)
	used := Method.StringWidth(first)
	keep := make([]bool, len(parts))
	if mode > 0 && used+sep+Method.StringWidth(parts[mode]) <= width {
		keep[mode] = true
		used += sep + Method.StringWidth(parts[mode])
	}
	for i := 1; i < len(parts); i++ {
		if w := sep + Method.StringWidth(parts[i]); !keep[i] && i != mode && used+w <= width {
			keep[i] = true
			used += w
		}
	}
	line := first
	for i := 1; i < len(parts); i++ {
		if keep[i] {
			line += " · " + parts[i]
		}
	}
	return styled(faint, truncate(line, width))
}

// Mode names a mode as the screen shows it.
func Mode(moving bool) string {
	if moving {
		return "MOVING"
	}
	return "TYPING"
}

// modeTag names the mode in the status line, in bold, and returns to
// faint, the style the whole line is in.
func modeTag(moving bool) string { return sgr(bold) + Mode(moving) + sgr(faint) }

// Money formats a cost to the cent: $1.23 in US dollars, else the
// amount and the currency code. It is empty for an amount that is not a
// number.
func Money(c core.Cost) string {
	if math.IsNaN(c.Amount) || math.IsInf(c.Amount, 0) {
		return ""
	}
	switch cur := oneLine(c.Currency); cur {
	case "USD", "":
		if c.Amount < 0 {
			return fmt.Sprintf("-$%.2f", -c.Amount)
		}
		return fmt.Sprintf("$%.2f", c.Amount)
	default:
		return fmt.Sprintf("%.2f %s", c.Amount, cur)
	}
}

// Duration formats d as 9s, 1m02s or 1h04m, rounding down.
func Duration(d time.Duration) string {
	d = max(d, 0)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d/time.Second))
	case d < time.Hour:
		return fmt.Sprintf("%dm%02ds", int(d/time.Minute), int(d%time.Minute/time.Second))
	}
	return fmt.Sprintf("%dh%02dm", int(d/time.Hour), int(d%time.Hour/time.Minute))
}
