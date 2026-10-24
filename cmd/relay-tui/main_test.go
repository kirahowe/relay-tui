package main

// The built binary on a real pseudo-terminal: raw mode, Ctrl-C as a key
// rather than SIGINT, exit codes, and putting the terminal back.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"

	"github.com/kirahowe/relay-tui/internal/core"
	"github.com/kirahowe/relay-tui/internal/viewfile"
)

var relaytui, fakeAgent string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "relay-tui-pty")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	relaytui, fakeAgent = filepath.Join(dir, "relay-tui"), filepath.Join(dir, "fakeagent")
	for bin, pkg := range map[string]string{relaytui: ".", fakeAgent: "../fakeagent"} {
		if out, err := exec.Command("go", "build", "-o", bin, pkg).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "building %s: %v\n%s", pkg, err, out)
			os.Exit(1)
		}
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// session is relay-tui running on a pty, its output fed to an emulator.
type session struct {
	t    *testing.T
	cmd  *exec.Cmd
	pty  *os.File
	mu   sync.Mutex // guards emu's screen and raw
	emu  *vt.Emulator
	raw  bytes.Buffer // everything relay-tui wrote
	done chan error
}

func runPTY(t *testing.T, args ...string) *session {
	t.Helper()
	if !slices.Contains(args, "--state-dir") {
		// Never the operator's own state directory.
		args = append([]string{"--state-dir", t.TempDir()}, args...)
	}
	s := &session{t: t, cmd: exec.Command(relaytui, args...), emu: vt.NewEmulator(80, 24), done: make(chan error, 1)}
	s.cmd.Dir = t.TempDir()
	f, err := pty.StartWithSize(s.cmd, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		t.Fatal(err)
	}
	s.pty = f
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := f.Read(buf)
			if n > 0 {
				s.mu.Lock()
				s.raw.Write(buf[:n])
				_, _ = s.emu.Write(buf[:n])
				s.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	go func() { _, _ = io.Copy(f, s.emu) }() // the emulator's answers to queries
	go func() { s.done <- s.cmd.Wait() }()
	t.Cleanup(func() {
		select {
		case <-s.done:
		default:
			_ = s.cmd.Process.Kill() // our own child, by PID
			<-s.done
		}
		_ = f.Close()
		_ = s.emu.InputPipe().(io.Closer).Close()
	})
	return s
}

func (s *session) keys(k string) { _, _ = s.pty.Write([]byte(k)) }

func (s *session) screen() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.emu.String()
}

func (s *session) waitScreen(text string) {
	s.t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for !strings.Contains(s.screen(), text) {
		if time.Now().After(deadline) {
			s.t.Fatalf("timed out waiting for %q; screen:\n%s", text, s.screen())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// exit waits for relay-tui to exit and returns its exit code.
func (s *session) exit() int {
	s.t.Helper()
	select {
	case err := <-s.done:
		s.done <- err
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode()
		}
		if err != nil {
			s.t.Fatal(err)
		}
		return 0
	case <-time.After(8 * time.Second):
		s.t.Fatalf("relay-tui did not exit; screen:\n%s", s.screen())
		return -1
	}
}

func script(name string) string {
	p, _ := filepath.Abs(filepath.Join("..", "..", "testdata", "scripts", name))
	return p
}

func TestPTYReplyThenCtrlD(t *testing.T) {
	s := runPTY(t, "--", fakeAgent, script("simple.jsonl"))
	s.waitScreen("ready")
	s.keys("hi\r")
	s.waitScreen("Hello there.")
	s.keys("\x04")
	if code := s.exit(); code != 0 {
		t.Fatalf("exit %d", code)
	}
	s.mu.Lock()
	raw := s.raw.String()
	s.mu.Unlock()
	// The terminal is put back: modifyOtherKeys reset at the end.
	if i := strings.LastIndex(raw, "\x1b[>4m"); i < 0 || i < strings.LastIndex(raw, "\x1b[>4;2m") {
		t.Fatalf("modifyOtherKeys was not reset; output tail %q", raw[max(0, len(raw)-200):])
	}
}

// Ctrl-C is a key in raw mode, not a signal: it cancels the turn and
// relay-tui keeps running.
func TestPTYCtrlCCancels(t *testing.T) {
	log := filepath.Join(t.TempDir(), "agent.log")
	s := runPTY(t, "--", fakeAgent, "-log", log, script("cancel.jsonl"))
	s.waitScreen("ready")
	s.keys("go\r")
	s.waitScreen("running sleep 60")
	s.keys("\x03")
	s.waitScreen("[cancelled]")
	s.keys("again\r")
	s.waitScreen("Still here.")
	if b, _ := os.ReadFile(log); !strings.Contains(string(b), "cancel\n") {
		t.Fatalf("no cancel in the agent log:\n%s", b)
	}
	s.keys("\x03") // idle, empty input: quit
	if code := s.exit(); code != 0 {
		t.Fatalf("exit %d", code)
	}
}

func TestPTYAgentExitsOnItsOwn(t *testing.T) {
	s := runPTY(t, "--", "false")
	if code := s.exit(); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
}

func TestUsage(t *testing.T) {
	err := exec.Command(relaytui).Run()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 2 {
		t.Fatalf("no command: %v, want exit 2", err)
	}
}

// The saved view outlives a kill: the draft is written soon after it is
// typed, to the --state-dir given, in a private file, and the next run
// against the same agent puts it back.
func TestPTYDraftSurvivesAKill(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	agent := []string{fakeAgent, script("simple.jsonl")}
	s := runPTY(t, append([]string{"--state-dir", dir, "--"}, agent...)...)
	s.waitScreen("ready")
	s.keys("keep these words")
	deadline := time.Now().Add(8 * time.Second)
	for {
		if v, _ := viewfile.Load(dir, agent); len(v.Drafts) == 1 && v.Drafts[0].Text == "keep these words" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the draft was never saved")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if fi, err := os.Stat(viewfile.Path(dir, agent)); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("file: %v %v", fi, err)
	}
	if fi, err := os.Stat(dir); err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("dir: %v %v", fi, err)
	}
	_ = s.cmd.Process.Kill() // our own child, by PID
	<-s.done
	s.done <- nil

	s = runPTY(t, append([]string{"--state-dir", dir, "--"}, agent...)...)
	s.waitScreen("keep these words")
	// Another agent command has a view of its own.
	other := runPTY(t, "--state-dir", dir, "--", fakeAgent, "-log", filepath.Join(t.TempDir(), "log"), script("simple.jsonl"))
	other.waitScreen("ready")
	if strings.Contains(other.screen(), "keep these words") {
		t.Fatal("another command got the draft")
	}
}

// Codex round 2, finding 2: an agent that exits before initialize
// answers leaves the saved view exactly as it was, whatever was typed
// meanwhile: nothing was handled, so there is nothing new to save.
func TestPTYAFailedStartLeavesTheSavedView(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	agent := []string{"false"}
	v := core.View{Hub: &core.HubView{Focus: "mail", Cursor: "mail"},
		Drafts: []core.Draft{{Text: "typed before the hub was known"}, {Role: "mail", Text: "a draft"}}}
	b, _, err := viewfile.Encode(agent, v)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := viewfile.Write(viewfile.Path(dir, agent), b); err != nil {
		t.Fatal(err)
	}
	s := runPTY(t, append([]string{"--state-dir", dir, "--"}, agent...)...)
	s.keys("more words")
	if code := s.exit(); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if got, err := os.ReadFile(viewfile.Path(dir, agent)); err != nil || !bytes.Equal(got, b) {
		t.Fatalf("the saved view changed (%v):\n%s\nwas\n%s", err, got, b)
	}
}
