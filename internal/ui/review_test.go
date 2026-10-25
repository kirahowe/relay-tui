package ui

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"pgregory.net/rapid"

	"github.com/kirahowe/relay-tui/internal/core"
	"github.com/kirahowe/relay-tui/internal/render"
)

// The review through the real model, read off the screen: whatever the
// window's size, the keys and the waits, an allow, a card's trust or an
// archive goes out only once the screen has shown every byte of that
// exact review text, as the pager's lines of it, and ArmDelay after the
// last part not shown before. A refusal needs no reading. (Ending a
// conversation is an inline question, not a review; see the surface
// rules.)

// longRequest is a request whose input runs to many lines.
func longRequest(rt *rapid.T, at time.Time) core.PermissionAsked {
	n := rapid.IntRange(1, 40).Draw(rt, "input lines")
	var kv []string
	for i := range n {
		kv = append(kv, fmt.Sprintf(`"k%d":"%s"`, i, strings.Repeat("w", rapid.IntRange(1, 120).Draw(rt, "value"))))
	}
	e := askEvent(at, 1)
	e.Input = "{" + strings.Join(kv, ",") + "}"
	return e
}

// reviewOnScreen is the review that has the keys and what the screen
// shows of its text, read off the screen against the pager's lines: the
// bytes from and to of the text, or ok false when the screen shows no
// rows of it.
func reviewOnScreen(rt *rapid.T, m Model, now time.Time) (id string, from, to, n int, ok bool) {
	surf := core.HubSurface(m.hub)
	a, s := m.hub.App, m.focusedState()
	content := ""
	if m.kind == kindHub {
		content = m.hubFrame(now).Content
	} else {
		surf, a, s = core.SingleSurface(m.one), m.one.App, m.one.State
		content = m.frame(now).Content
	}
	if surf.Kind != core.SurfaceReview {
		return "", 0, 0, 0, false
	}
	sub := core.ReviewSubject(surf, a, s)
	lines, fits := render.PagerLines(sub.Text, m.width()-len(render.Margin))
	if sub.Problem != "" || !fits {
		return sub.ID, 0, 0, len(sub.Text), false
	}
	rows := strings.Split(ansi.Strip(content), "\n")
	d := m.pagerFrame(now).Drawn
	if d == nil {
		return sub.ID, 0, 0, len(sub.Text), false
	}
	// What the frame says it drew must be what the screen shows: the
	// body, below the heading, is the text's lines from From, row by
	// row, to To.
	off, i := 0, 0
	for off < d.From && i < len(lines) {
		off += len(lines[i])
		i++
	}
	if off != d.From {
		rt.Fatalf("the frame says it drew from byte %d, where no line starts", d.From)
	}
	for _, r := range rows[1:] {
		if off >= d.To {
			break
		}
		lead := render.Margin
		if render.Continues(lines, i) {
			lead = render.ContinuationMark
		}
		if strings.TrimRight(r, " ") != strings.TrimRight(lead+strings.TrimSuffix(lines[i], "\n"), " ") {
			rt.Fatalf("the screen row %q is not the line %q the frame says it drew:\n%s", r, lines[i], ansi.Strip(content))
		}
		off += len(lines[i])
		i++
	}
	if off != d.To {
		rt.Fatalf("the screen shows to byte %d, the frame says %d", off, d.To)
	}
	return sub.ID, d.From, d.To, len(sub.Text), true
}

func TestConsentOnlyFromAReviewReadWhole(t *testing.T) {
	sizes := [][2]int{{40, 5}, {40, 10}, {60, 14}, {80, 24}, {170, 40}}
	for _, hub := range []bool{true, false} {
		t.Run(fmt.Sprint("hub=", hub), func(t *testing.T) {
			rapid.Check(t, func(rt *rapid.T) {
				clock := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
				var calls []call
				var m Model
				ask := longRequest(rt, clock)
				if hub {
					m = surfaceHub(&calls, &clock)
				} else {
					m = surfaceSingle(&calls, &clock)
				}
				sz := rapid.SampledFrom(sizes).Draw(rt, "size")
				m = update(m, tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
				if hub {
					m = update(m, ev(core.Routed{Session: "s1", Event: ask}))
				} else {
					m = update(m, ev(ask))
				}
				m = update(m, key("esc"))
				seen := map[string][]bool{}
				lastNew := map[string]time.Time{}
				look := func() {
					id, from, to, n, ok := reviewOnScreen(rt, m, clock)
					if !ok {
						return
					}
					if seen[id] == nil {
						seen[id] = make([]bool, n)
					}
					for i := from; i < to; i++ {
						if !seen[id][i] {
							seen[id][i] = true
							lastNew[id] = clock
						}
					}
					if n == 0 && lastNew[id].IsZero() {
						lastNew[id] = clock
					}
				}
				read := func(id string) bool {
					if _, ok := lastNew[id]; !ok {
						return false
					}
					for _, b := range seen[id] {
						if !b {
							return false
						}
					}
					return true
				}
				look()
				for range rapid.IntRange(1, 40).Draw(rt, "steps") {
					clock = clock.Add(time.Duration(rapid.IntRange(0, 900).Draw(rt, "ms")) * time.Millisecond)
					if rapid.IntRange(0, 9).Draw(rt, "resize") == 0 {
						sz := rapid.SampledFrom(sizes).Draw(rt, "size")
						m = update(m, tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
						look()
						continue
					}
					id := m.surfaceID()
					n := len(calls)
					k := rapid.SampledFrom([]string{"enter", "enter", "y", "y", "y", " ", " ", "j", "k", "G", "g", "q", "n", "tab", "l", "t", "X", "h", "esc"}).Draw(rt, "key")
					m = update(m, key(k))
					for _, c := range calls[n:] {
						if authorized(c) && c.what != "end" && (!read(id) || c.at.Sub(lastNew[id]) < core.ArmDelay) {
							rt.Fatalf("key %q made %+v on %q: read whole %v, the last new part %v before", k, c, id, read(id), c.at.Sub(lastNew[id]))
						}
					}
					look()
				}
			})
		})
	}
}

// Round 3: moving the highlight changes only its mark, so a short review
// drew the same report after the move and no report armed the new
// highlight: tab then enter never allowed, and a second allow_once could
// not be sent at all. Each highlight is now a report of its own, and a
// tab with one option, which moves nothing, disarms nothing (round 4).
func TestTheMovedHighlightCanAllow(t *testing.T) {
	two := surfaceAsk
	two.Options = []core.PermOption{{ID: "allow", Kind: core.AllowOnce}, {ID: "allow-edit", Kind: core.AllowOnce}, {ID: "reject", Kind: core.RejectOnce}}
	one := surfaceAsk
	one.Options = []core.PermOption{{ID: "allow", Kind: core.AllowOnce}}
	cases := []struct {
		name string
		ask  core.Permission
		tabs int
		want string
	}{
		{"tab to the allow", surfaceAsk, 1, "allow"},
		{"tab to the second allow", two, 2, "allow-edit"},
		{"tab on a lone allow", one, 1, "allow"},
	}
	for _, c := range cases {
		for _, hub := range []bool{false, true} {
			clock := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
			var calls []call
			ask := c.ask
			ask.ID, ask.CallID = 1, "c1"
			asked := core.PermissionAsked{At: clock, Permission: ask}
			var m Model
			if hub {
				m = surfaceHub(&calls, &clock)
				m = update(m, ev(core.Routed{Session: "s1", Event: asked}))
			} else {
				m = surfaceSingle(&calls, &clock)
				m = update(m, ev(asked))
			}
			m = update(m, key("esc"))
			clock = clock.Add(time.Second)
			m = update(m, key("enter")) // open the review
			clock = clock.Add(2 * time.Second)
			for range c.tabs {
				m = update(m, key("tab"))
			}
			clock = clock.Add(2 * time.Second)
			m = update(m, key("enter"))
			var sent []string
			for _, cl := range calls {
				if cl.what == "answer" {
					sent = append(sent, cl.option)
				}
			}
			if !slices.Equal(sent, []string{c.want}) {
				t.Errorf("%s (hub %v): sent %v, want [%s]", c.name, hub, sent, c.want)
			}
		}
	}
}
