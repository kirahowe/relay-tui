package ui_test

// End to end against the real relay, not the fake hub, so relay-tui's
// fixtures cannot drift from relay's wire: relay's hub on a scratch data
// directory (testdata/relay/setup.clj adds a role and its cards), and
// the one question relay-tui asks with no model: whether to archive a
// card. The relay checkout is the -relay flag, by default the sibling
// ../relay; the tests skip without it or without bb.

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/kirahowe/relay-tui/internal/agent"
	"github.com/kirahowe/relay-tui/internal/core"
	"github.com/kirahowe/relay-tui/internal/ui"
)

var relayDir = flag.String("relay", filepath.Join("..", "..", "..", "relay"), "a relay checkout, for the end-to-end tests against it")

// startRelay runs relay-tui against `relay acp` on a fresh data
// directory, with no role in focus.
func startRelay(t *testing.T, w, h int) (*harness, string) {
	t.Helper()
	bin, _ := filepath.Abs(filepath.Join(*relayDir, "bin", "relay"))
	if _, err := os.Stat(bin); err != nil {
		t.Skipf("no relay at %s", bin)
	}
	if _, err := exec.LookPath("bb"); err != nil {
		t.Skip("no bb")
	}
	dir := t.TempDir()
	data := filepath.Join(dir, "data")
	if err := os.MkdirAll(data, 0o700); err != nil {
		t.Fatal(err)
	}
	setup, _ := filepath.Abs(filepath.Join("..", "..", "testdata", "relay", "setup.clj"))
	relay, _ := filepath.Abs(*relayDir)
	if out, err := exec.Command("bb", setup, relay, data).CombinedOutput(); err != nil {
		t.Fatalf("setting up relay's data: %v\n%s", err, out)
	}
	h0 := &harness{t: t, emu: newTerminal(w, h), log: filepath.Join(dir, "relay.log"), done: make(chan struct{})}
	started := make(chan struct{})
	conn, err := agent.Start([]string{bin, "acp", "--data", data, "--run-dir", data}, dir, os.Stderr,
		func(e core.Event) {
			<-started
			h0.p.Send(ui.EventMsg{Event: e})
		})
	if err != nil {
		t.Fatal(err)
	}
	h0.conn = conn
	h0.p = tea.NewProgram(ui.New(conn, ""), tea.WithInput(h0.emu.in), tea.WithOutput(h0.emu),
		tea.WithWindowSize(w, h), tea.WithoutSignals(), tea.WithColorProfile(colorprofile.TrueColor), tea.WithFilter(ui.KeepWidthMethod))
	close(started)
	go func() { _, _ = h0.p.Run(); close(h0.done) }()
	t.Cleanup(func() {
		h0.p.Kill()
		select {
		case <-h0.done:
		case <-time.After(5 * time.Second):
			t.Error("the program did not stop")
		}
		conn.Shutdown()
		h0.emu.close()
	})
	lastHarness = nil
	return h0, data
}

// relayCards is `relay cards` on data.
func relayCards(t *testing.T, data string) string {
	out, err := exec.Command(filepath.Join(*relayDir, "bin", "relay"), "--data", data, "cards").CombinedOutput()
	if err != nil {
		t.Fatalf("relay cards: %v\n%s", err, out)
	}
	return string(out)
}

// A card is archived after asking, at the rev shown.
func TestRealRelayArchivesACardAfterAsking(t *testing.T) {
	h, data := startRelay(t, 140, 40)
	h.waitScreen("the roles", func(s string) bool { return strings.Contains(s, "l board") })
	h.keys("l")
	h.waitScreen("the board", func(s string) bool {
		return strings.Contains(s, "Pack for the trip") && strings.Contains(s, "X archive")
	})
	h.keys("j")
	time.Sleep(200 * time.Millisecond)
	h.keys("X")
	h.waitScreen("the question", func(s string) bool {
		return strings.Contains(s, "Archive") && strings.Contains(s, "Pack for the trip") && strings.Contains(s, "y archive")
	})
	time.Sleep(core.ArmDelay + 100*time.Millisecond)
	h.keys("y")
	h.wait("the archive", func() string { return relayCards(t, data) }, func(s string) bool { return !strings.Contains(s, "Pack for the trip") })
}
