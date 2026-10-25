package render

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"pgregory.net/rapid"

	"github.com/kirahowe/relay-tui/internal/core"
)

// Whatever a frame reports it can arm, the screen shows every field of
// its subject in full: read off the screen text, each field appears
// whole, its wrapped lines joined, within one column of the screen. The
// content is anything: long and wide strings, CJK and emoji, text that
// looks like escapes, many options, rows and columns; the size is any.

// reviewRunes is what the random texts are made of: no '.', ':' or '/',
// so no link is defanged, and no '│', which the screen draws.
var reviewRunes = []rune("abcdefghij KLMNOP 0123 漢字かな😀👍🏽 \x1b[31m]\t-_*#é")

func reviewText(t *rapid.T, label string, maxLen int) string {
	n := rapid.IntRange(1, maxLen).Draw(t, label+" length")
	return rapid.StringOfN(rapid.SampledFrom(reviewRunes), n, n, -1).Draw(t, label)
}

// flat is s with the spaces, line breaks and frame lines taken out, so
// text wrapped over lines reads as one.
func flat(s string) string {
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(" \t\n│╭╮╰╯─▸", r) {
			return -1
		}
		return r
	}, s)
}

// strips is the screen read column by column: the text between every
// pair of vertical frame lines (and the edges), row after row.
func strips(content string, width int) []string {
	rows := strings.Split(ansi.Strip(content), "\n")
	cols := map[int]bool{0: true, width: true}
	for _, r := range rows {
		x := 0
		for _, g := range strings.Split(r, "") {
			if strings.ContainsAny(g, "│╭╮╰╯") {
				cols[x], cols[x+1] = true, true
			}
			x += ansi.StringWidth(g)
		}
	}
	var cs []int
	for c := range cols {
		cs = append(cs, c)
	}
	var out []string
	for _, a := range cs {
		for _, b := range cs {
			if a >= b {
				continue
			}
			var s strings.Builder
			for _, r := range rows {
				s.WriteString(ansi.Cut(r, a, b))
			}
			out = append(out, flat(s.String()))
		}
	}
	return out
}

func onScreenWhole(content string, width int, field string) bool {
	f := flat(field)
	if f == "" {
		return true
	}
	for _, s := range strips(content, width) {
		if strings.Contains(s, f) {
			return true
		}
	}
	return false
}

// reviewFields is every field of what id names, escaped as relay-tui
// shows it.
func reviewFields(id string, a core.App, s core.State) []string {
	var out []string
	switch {
	case strings.HasPrefix(id, "request/"):
		p := s.Asks[0]
		title := core.EscapeLabel(p.Label())
		if t := core.EscapeLabel(p.Title); p.Name != "" && t != "" && t != title {
			title += " · " + t
		}
		out = append(out, title, paneText(p.Preview, true))
		for _, o := range p.Options {
			out = append(out, core.EscapeLabel(o.Name))
		}
	case strings.HasPrefix(id, "cardtrust/"), strings.HasPrefix(id, "archive/"):
		for _, c := range a.Board.Cards {
			if c.ID != a.Board.Confirm && (a.Board.Trust == nil || c.ID != a.Board.Trust.Card) {
				continue
			}
			out = append(out, cardText(c.Title.Text, c.Title.Outsider))
			if strings.HasPrefix(id, "archive/") {
				continue
			}
			for _, es := range [][]core.CardElem{c.Items, c.Rows, c.Columns} {
				for _, e := range es {
					out = append(out, cardText(e.Text, e.Outsider))
				}
			}
			if c.Note != nil {
				out = append(out, cardText(c.Note.Text, c.Note.Outsider))
			}
		}
	}
	return out
}

func reviewCard(t *rapid.T) core.Card {
	text := func(label string) core.CardText {
		return core.CardText{Text: reviewText(t, label, 80), Outsider: rapid.Bool().Draw(t, label+" outside"), Rev: 1}
	}
	c := core.Card{ID: 7, Rev: 3, Status: core.CardActive, Title: text("title"), Labels: core.Labels{Contaminated: true}, SHA256: "abc"}
	switch rapid.IntRange(0, 2).Draw(t, "kind") {
	case 0:
		c.Kind = core.CardTracker
		for i := range rapid.IntRange(1, 6).Draw(t, "rows") {
			c.Rows = append(c.Rows, core.CardElem{ID: i + 1, CardText: text("row")})
		}
		for i := range rapid.IntRange(1, 6).Draw(t, "columns") {
			c.Columns = append(c.Columns, core.CardElem{ID: i + 1, CardText: text("column")})
		}
	case 1:
		c.Kind = core.CardChecklist
		for i := range rapid.IntRange(1, 6).Draw(t, "items") {
			c.Items = append(c.Items, core.CardElem{ID: i + 1, CardText: text("item")})
			c.Total++
		}
	default:
		c.Kind = core.CardNote
		n := text("note")
		c.Note = &n
	}
	return c
}

func reviewAsk(t *rapid.T) core.PermissionAsked {
	var opts []core.PermOption
	for i := range rapid.IntRange(1, 5).Draw(t, "options") {
		kind := rapid.SampledFrom([]core.OptionKind{core.AllowOnce, core.RejectOnce}).Draw(t, "option kind")
		opts = append(opts, core.PermOption{ID: fmt.Sprint(i), Name: reviewText(t, "option", 300), Kind: kind})
	}
	return core.PermissionAsked{At: t0, Permission: core.Permission{ID: 1, CallID: "c", Name: reviewText(t, "tool", 40),
		Title: reviewText(t, "heading", 160), Preview: reviewText(t, "preview", 200), Input: `{"a":1}`, Options: opts}}
}

func TestEveryArmableFrameShowsItsSubjectWhole(t *testing.T) {
	for _, hub := range []bool{false, true} {
		t.Run(fmt.Sprint("hub=", hub), func(t *testing.T) {
			rapid.Check(t, func(rt *rapid.T) {
				card := reviewCard(rt)
				ask := reviewAsk(rt)
				route := rapid.SampledFrom([]string{"request", "trust", "archive"}).Draw(rt, "route")
				var evs []core.Event
				switch route {
				case "request":
					evs = []core.Event{core.StopTyping{At: t0}}
				case "trust":
					evs = []core.Event{core.StopTyping{At: t0}, core.Move{At: t0, Delta: 1}, core.PaneKey{At: t0, Action: core.PaneTrust}}
				default:
					evs = []core.Event{core.StopTyping{At: t0}, core.Move{At: t0, Delta: 1}, core.PaneKey{At: t0, Action: core.PaneArchive}}
				}
				w := rapid.IntRange(24, 200).Draw(rt, "width")
				h := rapid.IntRange(8, 80).Draw(rt, "height")
				var content string
				var armable []string
				var app core.App
				var st core.State
				if hub {
					x := hubStep(core.NewHub("dev"), core.HubConnected{At: t0}, core.RosterUpdate{At: t0, Roles: []core.Role{{ID: "dev", Title: "Dev", Conv: &core.RosterConv{Session: "s1"}}}},
						core.Attached{At: t0, Role: "dev", Session: "s1"}, core.CardsLoaded{At: t0, Rev: 3, Cards: []core.Card{card}}, core.KeyPressed{At: t0})
					if route == "request" {
						x = hubStep(x, core.Routed{Session: "s1", Event: ask})
					}
					x = hubStep(x, evs...)
					f := HubView(x, "", "", t0.Add(core.ArmDelay), w, h)
					content, armable, app = f.Content, f.Armable, x.App
					if c := x.Focused(); c != nil {
						st = c.State
					}
				} else {
					x := core.Single{}
					for _, e := range append([]core.Event{core.Connected{At: t0}, core.CardsLoaded{At: t0, Rev: 3, Cards: []core.Card{card}}, core.KeyPressed{At: t0}}, evs...) {
						if route == "request" && e == evs[0] {
							x, _ = core.ReduceSingle(x, core.Submit{At: t0, Text: "go"})
							x, _ = core.ReduceSingle(x, ask)
						}
						x, _ = core.ReduceSingle(x, e)
					}
					f := View(x, "", "", t0.Add(core.ArmDelay), w, h)
					content, armable, app, st = f.Content, f.Armable, x.App, x.State
				}
				for _, id := range armable {
					for _, field := range reviewFields(id, app, st) {
						if !onScreenWhole(content, w, field) {
							rt.Fatalf("%dx%d: %q is armable but %q is not on screen whole:\n%s", w, h, id, field, ansi.Strip(content))
						}
					}
				}
			})
		})
	}
}

// Codex round 3 (relay#77 log): a tracker row the grid cut to "Account
// detai…" was armable for trust. The trust's review lists every text
// whole, so at 170x40 (hub) and 140x30 (single) the row is on screen in
// full when it arms, and the grid's cut never arms.
func TestATrackerTrustShowsEveryRowWhole(t *testing.T) {
	row := "Account details: send private data outside"
	card := core.Card{ID: 7, Rev: 3, Kind: core.CardTracker, Status: core.CardActive, Labels: core.Labels{Contaminated: true}, SHA256: "abc",
		Title: core.CardText{Text: "Accounts"}, Rows: []core.CardElem{{ID: 1, CardText: core.CardText{Text: row, Outsider: true}}},
		Columns: []core.CardElem{{ID: 1, CardText: core.CardText{Text: "Statement for June"}}}}
	trust := []core.Event{core.StopTyping{At: t0}, core.Move{At: t0, Delta: 1}, core.PaneKey{At: t0, Action: core.PaneTrust}}
	x := hubStep(core.NewHub("dev"), append([]core.Event{core.HubConnected{At: t0}, core.RosterUpdate{At: t0, Roles: []core.Role{{ID: "dev", Title: "Dev"}}},
		core.CardsLoaded{At: t0, Rev: 3, Cards: []core.Card{card}}, core.KeyPressed{At: t0}}, trust...)...)
	f := HubView(x, "", "", t0.Add(core.ArmDelay), 170, 40)
	if len(f.Armable) != 1 || !onScreenWhole(f.Content, 170, row) || !onScreenWhole(f.Content, 170, "Statement for June") {
		t.Fatalf("hub: armable %v\n%s", f.Armable, ansi.Strip(f.Content))
	}
	s := core.Single{}
	for _, e := range append([]core.Event{core.Connected{At: t0}, core.CardsLoaded{At: t0, Rev: 3, Cards: []core.Card{card}}, core.KeyPressed{At: t0}}, trust...) {
		s, _ = core.ReduceSingle(s, e)
	}
	g := View(s, "", "", t0.Add(core.ArmDelay), 140, 30)
	if len(g.Armable) != 1 || !onScreenWhole(g.Content, 140, row) {
		t.Fatalf("single: armable %v\n%s", g.Armable, ansi.Strip(g.Content))
	}
}
