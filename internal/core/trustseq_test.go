package core

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// Ctrl-Y trusts the latest paste made in this run, by when it was
// pasted: never one a saved view brought back, however a restore or a
// save has numbered and ordered them.

// mailRun is a hub with Mail in focus and the input as the shell would
// hold it.
type mailRun struct {
	h     Hub
	input string
}

func (m *mailRun) apply(t interface{ Helper() }, es ...Event) []Effect {
	var all []Effect
	for _, e := range es {
		var fx []Effect
		m.h, fx = ReduceHub(m.h, e)
		for _, f := range fx {
			switch f := f.(type) {
			case InsertInput:
				m.input += f.Text
			case ReplaceInput:
				m.input = strings.Replace(m.input, f.Old, f.New, 1)
			case RestoreDraft:
				if f.Role == "mail" {
					if m.input != "" {
						m.input = f.Text + "\n" + m.input
					} else {
						m.input = f.Text
					}
				}
			}
		}
		all = append(all, fx...)
		// Drawn at once, as a renderer would draw it.
		m.h, _ = ReduceHub(m.h, Drawn{At: e.at(), Surfaces: []string{HubSurfaceID(m.h)}})
	}
	return all
}

func (m *mailRun) pastes() []Paste {
	if c := m.h.ConvOf("mail"); c != nil {
		return c.State.Pastes
	}
	for _, b := range m.h.Blanks {
		if b.Role == "mail" {
			return b.State.Pastes
		}
	}
	return nil
}

func openMail(t interface{ Helper() }, v *View) *mailRun {
	h := NewHub("mail")
	if v != nil {
		h = RestoreHub(h, *v)
	}
	m := &mailRun{h: h}
	m.apply(t, HubConnected{At: t0})
	return m
}

// The case Codex found: a saved untrusted email, a restart, a paste of
// your own before the roster: Ctrl-Y trusts yours, not the email.
func TestCtrlYAfterARestoreTrustsThisRunsPaste(t *testing.T) {
	m := openMail(t, nil)
	m.apply(t, RosterUpdate{At: t0, Roles: viewRoster()})
	m.input = "read "
	m.apply(t, Pasted{At: t0, Text: "an email from outside"})
	v := SaveHub(m.h, map[string]string{"mail": m.input})

	m = openMail(t, &v)
	m.input = "mine: "
	m.apply(t, Pasted{At: t0, Text: "my own words"})
	m.apply(t, RosterUpdate{At: t0, Roles: viewRoster()})
	if !strings.Contains(m.input, "an email") && !strings.Contains(m.input, "untrusted #2]") {
		t.Fatalf("the saved draft is not back: %q", m.input)
	}
	m.apply(t, TrustPaste{At: t0.Add(ArmDelay), Draft: m.input})
	for _, p := range m.pastes() {
		if p.Trusted != (p.Text == "my own words") {
			t.Fatalf("trusted the wrong paste: %+v (input %q)", m.pastes(), m.input)
		}
	}
	// Again: the email was not pasted in this run, so nothing is trusted,
	// and the status line says why.
	m.apply(t, TrustPaste{At: t0.Add(ArmDelay), Draft: m.input})
	for _, p := range m.pastes() {
		if p.Text != "my own words" && p.Trusted {
			t.Fatalf("a restored paste was trusted: %+v", m.pastes())
		}
	}
	if c := m.h.ConvOf("mail"); c == nil || !strings.Contains(c.State.Message.Text, "paste it again") {
		t.Fatalf("no word on why: %+v", c)
	}
}

func TestCtrlYOnlyEverTrustsTheLatestPasteOfThisRun(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		m := openMail(t, nil)
		var order []string // this run's pastes, oldest first
		n := 0
		for range rapid.IntRange(1, 30).Draw(rt, "steps") {
			switch rapid.IntRange(0, 5).Draw(rt, "op") {
			case 0, 1:
				n++
				text := fmt.Sprintf("paste %d", n)
				m.apply(t, Pasted{At: t0, Text: text})
				order = append(order, text)
			case 2:
				m.input += rapid.SampledFrom([]string{"a", " ", "\n"}).Draw(rt, "typed")
			case 3:
				if !m.h.RosterLoaded {
					m.apply(t, RosterUpdate{At: t0, Roles: viewRoster()})
				}
			case 4:
				// Close and open again: what was saved comes back, and is
				// no longer this run's.
				v := SaveHub(m.h, map[string]string{"mail": m.input})
				m, order = openMail(t, &v), nil
			case 5:
				before := slices.Clone(m.pastes())
				want := ""
				for i := len(order) - 1; i >= 0 && want == ""; i-- {
					for _, p := range before {
						if p.Text == order[i] && !p.Trusted && strings.Contains(m.input, p.Token()) {
							want = p.Text
						}
					}
				}
				m.apply(t, TrustPaste{At: t0.Add(ArmDelay), Draft: m.input})
				got := ""
				for _, p := range m.pastes() {
					if p.Trusted && !slices.ContainsFunc(before, func(q Paste) bool { return q.ID == p.ID && q.Trusted }) {
						got = p.Text
					}
				}
				if got != want {
					rt.Fatalf("ctrl+y trusted %q, want %q (this run %q)\ninput %q\npastes %+v", got, want, order, m.input, before)
				}
			}
		}
	})
}
