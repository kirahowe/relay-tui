package ui_test

// The saved view, end to end: the real ui.Model, internal/agent and the
// state file (internal/viewfile, wired as cmd/relay-tui wires it)
// against the fake hub. Each test closes relay-tui and opens it again
// against the same agent command with the same state directory.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/kirahowe/relay-tui/internal/agent"
	"github.com/kirahowe/relay-tui/internal/core"
	"github.com/kirahowe/relay-tui/internal/fakehub"
	"github.com/kirahowe/relay-tui/internal/ui"
	"github.com/kirahowe/relay-tui/internal/viewfile"
)

// savedRun is one run of relay-tui that keeps its view under state.
type savedRun struct {
	*hubHarness
	final chan ui.Model
}

// startSaved runs relay-tui against the fake hub configured by cfg,
// written to base, whose paths stay the same from run to run, so the
// agent command, and so the state file, is the same.
func startSaved(t *testing.T, base, state string, cfg fakehub.Config, w, h int) *savedRun {
	t.Helper()
	cfgPath := filepath.Join(base, "hub.json")
	b, _ := json.Marshal(cfg)
	if err := os.WriteFile(cfgPath, b, 0o600); err != nil {
		t.Fatal(err)
	}
	log, control := filepath.Join(base, "hub.log"), filepath.Join(base, "control")
	for _, f := range []string{log, control} {
		if err := os.WriteFile(f, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	argv := []string{fakeHub, "-log", log, "-control", control, cfgPath}
	hh := &hubHarness{
		harness: &harness{t: t, emu: newTerminal(w, h), log: log, done: make(chan struct{})},
		control: control,
	}
	run := &savedRun{hubHarness: hh, final: make(chan ui.Model, 1)}
	view, note := viewfile.Load(state, argv)
	saver := viewfile.NewSaver(viewfile.Path(state, argv), argv, 50*time.Millisecond, func(n string) { t.Errorf("saver: %s", n) })
	started := make(chan struct{})
	conn, err := agent.Start(argv, base, os.Stderr, func(e core.Event) {
		<-started
		hh.p.Send(ui.EventMsg{Event: e})
	})
	if err != nil {
		t.Fatal(err)
	}
	hh.conn = conn
	hh.p = tea.NewProgram(ui.New(conn, "").Restore(view, note).OnView(saver.Save), tea.WithInput(hh.emu.in), tea.WithOutput(hh.emu),
		tea.WithWindowSize(w, h), tea.WithoutSignals(), tea.WithColorProfile(colorprofile.TrueColor))
	close(started)
	go func() {
		m, _ := hh.p.Run()
		if mm, ok := m.(ui.Model); ok {
			saver.Close(mm.SavedView())
			run.final <- mm
		}
		close(hh.done)
	}()
	t.Cleanup(func() {
		hh.p.Kill()
		<-hh.done
		conn.Shutdown()
		hh.emu.close()
	})
	lastHarness = nil
	return run
}

// quit leaves relay-tui from the list of roles, as the operator would,
// and waits for the view to be written.
func (r *savedRun) quit() {
	r.t.Helper()
	if strings.Contains(r.screen(), "TYPING") {
		r.escape()
	}
	r.keys("hh") // from the pane or the conversation; the list is the edge
	r.waitScreen("the list", contains("enter open"))
	r.keys("\x03")
	select {
	case <-r.final:
	case <-time.After(8 * time.Second):
		r.t.Fatal("relay-tui did not quit")
	}
	<-r.done
}

// Set a view, quit, and open again: the same role, list, pane, card and
// draft, the paste in it as trusted or untrusted as it was.
func TestHubSavedViewComesBack(t *testing.T) {
	base, state := t.TempDir(), filepath.Join(t.TempDir(), "state")
	r := startSaved(t, base, state, boardRoles(), 170, 40)
	r.waitScreen("the board", contains("Board 3"))
	r.keys("3") // News, no conversation
	r.waitScreen("news", contains("No conversation with News"))
	r.keys("i")
	r.waitScreen("typing", contains("TYPING"))
	r.keys("draft for news ")
	r.keys(bracketed("an email from outside"))
	r.keys(" and ")
	r.keys(bracketed("my own words"))
	r.waitScreen("two placeholders", contains("untrusted #2]"))
	r.keys("\x19") // ctrl+y trusts the latest
	r.waitScreen("trusted", contains("trusted #2]"))
	r.escape()
	r.keys("l\rj") // the board, Trip prep, open, its first item
	r.waitScreen("the item", func(string) bool { return r.emu.selected("Renew passport") })
	r.quit()

	v, note := viewfile.Load(state, []string{fakeHub, "-log", filepath.Join(base, "hub.log"), "-control", filepath.Join(base, "control"), filepath.Join(base, "hub.json")})
	if note != "" || v.Hub == nil || v.Hub.Focus != "news" || v.Card != 14 || !v.Detail || v.Line != 1 || len(v.Drafts) != 1 {
		t.Fatalf("saved %+v %q", v, note)
	}

	r = startSaved(t, base, state, boardRoles(), 170, 40)
	r.waitScreen("the list over news", func(s string) bool {
		return strings.Contains(s, "enter open") && strings.Contains(s, "No conversation with News")
	})
	r.keys("ll") // to the conversation, then the pane with the card still open
	r.waitScreen("the open card with the keys", func(s string) bool {
		return strings.Contains(s, "x tick") && r.emu.selected("Renew passport")
	})
	s := r.screen()
	for _, want := range []string{"draft for news", "untrusted #1]", "trusted #2]"} {
		if !strings.Contains(s, want) {
			t.Errorf("%q not on screen:\n%s", want, s)
		}
	}
	if strings.Contains(s, "untrusted #2]") {
		t.Errorf("the trusted paste came back untrusted:\n%s", s)
	}
	// Sent, the untrusted paste goes as an attachment and the trusted one
	// as text, as they would have before.
	r.keys("hi")
	r.waitScreen("the input has the keys", contains("enter send"))
	r.keys("\r")
	line := r.waitLog("prompt ")
	if !strings.Contains(line, `[paste:1]`) || !strings.Contains(line, "my own words") || strings.Contains(line, "an email from outside") {
		t.Fatalf("prompt %s", line)
	}
	// A paste made now takes the next number.
	r.waitScreen("the reply", contains("draft for news"))
	r.keys(bracketed("third"))
	r.waitScreen("the next number", contains("untrusted #3]"))
}

// A role removed between runs: its focus falls back to the list, the
// draft for it is dropped with a note, and the rest comes back.
func TestHubSavedViewWithARoleRemoved(t *testing.T) {
	base, state := t.TempDir(), filepath.Join(t.TempDir(), "state")
	r := startSaved(t, base, state, boardRoles(), 170, 40)
	r.waitScreen("the board", contains("Board 3"))
	r.keys("2") // Mail
	r.waitScreen("mail", contains("No conversation with Mail"))
	r.keys("i")
	r.waitScreen("typing", contains("TYPING"))
	r.keys("for mail")
	r.keys("\x1b3") // alt+3: News, still typing
	r.waitScreen("news", contains("No conversation with News"))
	r.keys("for news")
	r.waitScreen("typed", contains("for news"))
	r.quit()

	cfg := boardRoles()
	cfg.Roles = append(cfg.Roles[:2:2], fakehub.RoleConfig{ID: "garden", Title: "Garden"}) // News is gone
	r = startSaved(t, base, state, cfg, 170, 40)
	s := r.waitScreen("the list", func(s string) bool {
		return strings.Contains(s, "Garden") && strings.Contains(s, "saved draft could not be put back")
	})
	if strings.Contains(s, "News") {
		t.Errorf("News is on screen:\n%s", s)
	}
	r.keys("\x1b2") // Mail's draft is back
	r.waitScreen("mail's draft", contains("for mail"))
}
