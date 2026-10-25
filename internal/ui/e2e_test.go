package ui_test

// End to end, in process: the real ui.Model and internal/agent, talking
// to the scripted fake agent, drawing into an x/vt emulator. Keys go in
// as the raw bytes a terminal would send.

import (
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/colorprofile"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kirahowe/relay-tui/internal/agent"
	"github.com/kirahowe/relay-tui/internal/core"
	"github.com/kirahowe/relay-tui/internal/ui"
)

var fakeAgent, fakeHub string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "relay-tui-e2e")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fakeAgent = filepath.Join(dir, "fakeagent")
	if out, err := exec.Command("go", "build", "-o", fakeAgent, "../../cmd/fakeagent").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "building fakeagent: %v\n%s", err, out)
		os.Exit(1)
	}
	fakeHub = filepath.Join(dir, "fakehub")
	if out, err := exec.Command("go", "build", "-o", fakeHub, "../../cmd/fakehub").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "building fakehub: %v\n%s", err, out)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type harness struct {
	t    *testing.T
	emu  *terminal
	p    *tea.Program
	conn *agent.Agent
	log  string
	done chan struct{}
}

// start runs relay-tui against the fake agent playing script, at w×h.
func start(t *testing.T, script string, w, h int) *harness {
	t.Helper()
	return startWith(t, script, w, h, "")
}

// startWith also sets relay-tui's --title and passes flags to the fake agent.
func startWith(t *testing.T, script string, w, h int, title string, agentFlags ...string) *harness {
	t.Helper()
	h0 := &harness{t: t, emu: newTerminal(w, h), log: filepath.Join(t.TempDir(), "agent.log"), done: make(chan struct{})}
	started := make(chan struct{})
	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "testdata", "scripts", script))
	if err != nil {
		t.Fatal(err)
	}
	command := append(append([]string{fakeAgent, "-log", h0.log}, agentFlags...), scriptPath)
	conn, err := agent.Start(command,
		t.TempDir(), io.Discard, func(e core.Event) {
			<-started
			h0.p.Send(ui.EventMsg{Event: e})
		})
	if err != nil {
		t.Fatal(err)
	}
	h0.conn = conn
	h0.p = tea.NewProgram(ui.New(conn, title), tea.WithInput(h0.emu.in), tea.WithOutput(h0.emu),
		tea.WithWindowSize(w, h), tea.WithoutSignals(), tea.WithColorProfile(colorprofile.TrueColor))
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
	h0.waitScreen("the first frame", func(s string) bool { return strings.Contains(s, "›") })
	lastHarness = h0
	return h0
}

// keys sends raw bytes, as a terminal would.
func (h *harness) keys(s string) { h.emu.SendText(s) }

func (h *harness) screen() string { return h.emu.String() }

// all is the scrollback, then the screen.
func (h *harness) all() string {
	var b strings.Builder
	for _, l := range h.emu.scrollback() {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	b.WriteString(h.screen())
	return b.String()
}

func (h *harness) wait(what string, get func() string, cond func(string) bool) string {
	h.t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for {
		s := get()
		if cond(s) {
			return s
		}
		if time.Now().After(deadline) {
			h.t.Fatalf("timed out waiting for %s; screen:\n%s\nscrollback and screen:\n%s", what, h.screen(), h.all())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (h *harness) waitScreen(what string, cond func(string) bool) string {
	h.t.Helper()
	return h.wait(what, h.screen, cond)
}

func (h *harness) waitAll(text string) string {
	h.t.Helper()
	return h.wait(fmt.Sprintf("%q", text), h.all, func(s string) bool { return strings.Contains(s, text) })
}

func (h *harness) waitIdle() {
	h.t.Helper()
	h.waitScreen("idle", func(s string) bool { return strings.Contains(s, "ready") })
}

// prompts are the prompts the fake agent received, in order.
func (h *harness) prompts() []string {
	b, _ := os.ReadFile(h.log)
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if rest, ok := strings.CutPrefix(l, "prompt "); ok {
			var p string
			_ = json.Unmarshal([]byte(rest), &p)
			out = append(out, p)
		}
	}
	return out
}

func (h *harness) agentLog() string { b, _ := os.ReadFile(h.log); return string(b) }

func (h *harness) waitDone() {
	h.t.Helper()
	select {
	case <-h.done:
	case <-time.After(8 * time.Second):
		h.t.Fatal("relay-tui did not quit")
	}
}

// statusLine is the screen row that holds the status.
func statusLine(screen string) string {
	for _, l := range strings.Split(screen, "\n") {
		for _, w := range []string{"waiting", "thinking", "writing", "running", "retrying", "cancelling", "waiting for you", "ready", "starting"} {
			if strings.Contains(l, w+" ") || strings.HasSuffix(strings.TrimSpace(l), w) {
				return strings.TrimSpace(l)
			}
		}
	}
	return ""
}

func orderedIn(t *testing.T, s string, parts ...string) {
	t.Helper()
	at := 0
	for _, p := range parts {
		i := strings.Index(s[at:], p)
		if i < 0 {
			t.Fatalf("%q missing or out of order in:\n%s", p, s)
		}
		at += i + len(p)
	}
}

func count(s, sub string) int { return strings.Count(s, sub) }

// bothSizes runs a scenario at 80×24 and at 40×10, where the boxes are
// narrow and the live view is short.
func bothSizes(t *testing.T, test func(t *testing.T, cols, rows int)) {
	for _, size := range [][2]int{{80, 24}, {40, 10}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			test(t, size[0], size[1])
			checkBoxes(t, size[0]-1, t.Name())
		})
	}
}

var lastHarness *harness

// checkBoxes asserts every box row on the terminal is whole: width wide,
// with matching ends.
func checkBoxes(t *testing.T, width int, name string) {
	t.Helper()
	if lastHarness == nil {
		return
	}
	for _, l := range strings.Split(lastHarness.all(), "\n") {
		l = strings.TrimRight(l, " ")
		r := []rune(l)
		if len(r) == 0 {
			continue
		}
		ends := map[rune]rune{'╭': '╮', '│': '│', '╰': '╯'}
		if end, ok := ends[r[0]]; ok {
			if r[len(r)-1] != end || ansi.StringWidth(l) != width {
				t.Errorf("%s: broken box row %q (%d wide, want %d)", name, l, ansi.StringWidth(l), width)
			}
		}
	}
}

func TestSimpleReply(t *testing.T) {
	h := start(t, "simple.jsonl", 80, 24)
	h.keys("hi\r")
	h.waitAll("Hello there.")
	h.waitIdle()
	all := h.all()
	orderedIn(t, all, "› hi", "Hello there.")
	if strings.Contains(all, "**") {
		t.Fatalf("markdown markers on screen:\n%s", all)
	}
	// Bubble Tea #1740: an early print must not scroll a screen of blank
	// lines into the scrollback.
	if n := len(h.emu.scrollback()); n != 0 {
		t.Fatalf("%d lines scrolled off a nearly empty screen:\n%s", n, all)
	}
	// The input box and the status line sit at the bottom of the screen,
	// with the transcript just above them, once the first print has moved
	// the frame down (it waits readyDelay after the first frame, which a
	// warm run can beat to idle).
	var rows []string
	h.waitScreen("the input box at the bottom", func(scr string) bool {
		rows = strings.Split(strings.TrimRight(scr, "\n"), "\n")
		n := len(rows)
		return n == 24 && strings.Contains(rows[n-1], "ready") && strings.HasPrefix(rows[n-2], "╰")
	})
	var shown []string
	for _, r := range rows {
		if strings.TrimSpace(r) != "" {
			shown = append(shown, r)
		}
	}
	if !strings.HasPrefix(shown[0], "╭─ fakeagent ─") {
		t.Fatalf("first row shown %q, want the banner", shown[0])
	}
}

// toolRun is how tool_loop.jsonl's run of two calls shows, as chips.
const toolRun = `✓  echo {"text":"hi"}   ls -la`

// privateBadge says the session's private label is on screen, as a pill
// or, without tints, as [private].
func privateBadge(s string) bool {
	return strings.Contains(s, "private")
}

// Tool calls show as one line each, apart from the prose, and the status
// line names the running tool.
func TestToolLoop(t *testing.T) { bothSizes(t, testToolLoop) }

func testToolLoop(t *testing.T, cols, rows int) {
	h := start(t, "tool_loop.jsonl", cols, rows)
	h.keys("look\r")
	mid := h.waitScreen("a running tool", func(s string) bool { return strings.Contains(s, `running echo {"text":"hi"}`) })
	_ = mid
	time.Sleep(300 * time.Millisecond) // let the prints in flight land
	t.Logf("screen mid-turn:\n%s", strings.TrimRight(h.screen(), "\n "))
	h.waitAll("the directory is fine")
	h.waitIdle()
	all := h.all()
	// With the terminal's colours known each tool is a chip.
	orderedIn(t, all, "› look", "Let me look.", toolRun, "It said hi, and the directory is fine.")
	if st := statusLine(h.screen()); !strings.Contains(st, "12% context · $0.03") {
		t.Fatalf("status %q, want the context use and the spend", st)
	}
	if n := count(all, toolRun); n != 1 {
		t.Fatalf("the run appears %d times:\n%s", n, all)
	}
	for _, l := range strings.Split(all, "\n") {
		if strings.Contains(l, "✓") && strings.Contains(l, "Let me") {
			t.Fatalf("a tool line shares a row with prose: %q", l)
		}
	}
}

// A burst of fast tool calls must not flash. Bubble Tea erases and
// redraws the whole frame whenever its height changes, and each tool
// line used to grow the frame and then shrink it again as it moved to
// scrollback. The frame now never changes height, so nothing is erased.
func TestToolBurstDoesNotRedraw(t *testing.T) { bothSizes(t, testToolBurstDoesNotRedraw) }

func testToolBurstDoesNotRedraw(t *testing.T, cols, rows int) {
	h := start(t, "tool_burst.jsonl", cols, rows)
	h.waitAll("ready")
	before := len(h.emu.written())
	h.keys("go\r")
	h.waitAll("All thirty read.")
	h.waitIdle()
	if out := h.emu.written()[before:]; strings.Contains(out, ansi.EraseScreenBelow) || strings.Contains(out, ansi.EraseEntireScreen) {
		t.Fatalf("the frame was erased and redrawn during the turn")
	}
	if n := count(h.all(), "✓  read file1.go   read file2.go"); n != 1 {
		t.Fatalf("the run shows %d times:\n%s", n, h.all())
	}
	// ctrl+t shows the calls in full in the transcript view, and
	// leaving it adds nothing to scrollback.
	scroll := strings.Join(h.emu.scrollback(), "\n")
	h.keys("\x14")
	h.waitScreen("the last call in full", func(s string) bool {
		return strings.Contains(s, "✓ read file30.go\n") && strings.Contains(s, "transcript")
	})
	h.keys("g")
	h.waitScreen("the top", func(s string) bool { return strings.Contains(s, "› go") })
	h.keys("\x1b")
	h.waitIdle()
	if again := strings.Join(h.emu.scrollback(), "\n"); again != scroll {
		t.Fatalf("scrollback changed:\n%s\n---\n%s", scroll, again)
	}
}

// The original complaint: a silent agent looked frozen. The elapsed time
// and the spinner keep moving while nothing arrives.
func TestStatusMovesWhileSilent(t *testing.T) { bothSizes(t, testStatusMovesWhileSilent) }

func testStatusMovesWhileSilent(t *testing.T, cols, rows int) {
	h := start(t, "silent.jsonl", cols, rows)
	h.keys("go\r")
	first := statusLine(h.waitScreen("waiting 0s", func(s string) bool { return strings.Contains(statusLine(s), "waiting · 0s") }))
	time.Sleep(250 * time.Millisecond)
	if again := statusLine(h.screen()); again == first {
		t.Fatalf("the spinner did not move in 250ms: %q", again)
	}
	h.waitScreen("waiting 1s", func(s string) bool { return strings.Contains(statusLine(s), "waiting · 1s") })
	h.waitScreen("waiting 2s", func(s string) bool { return strings.Contains(statusLine(s), "waiting · 2s") })
	h.waitAll("Done waiting.")
}

func TestRetryCountdown(t *testing.T) { bothSizes(t, testRetryCountdown) }

func testRetryCountdown(t *testing.T, cols, rows int) {
	h := start(t, "retry.jsonl", cols, rows)
	h.keys("go\r")
	// At 40 columns the mode takes the room the reason would have.
	h.waitScreen("retrying in 3s", func(s string) bool {
		return strings.Contains(s, "retrying in 3s · rate limit exceeded") || cols < 80 && strings.Contains(s, "retrying in 3s ·") && strings.Contains(s, "TYPING")
	})
	h.waitScreen("retrying in 2s", func(s string) bool { return strings.Contains(s, "retrying in 2s") })
	h.waitAll("Sorry for the wait.")
	if strings.Contains(h.all(), "Rate limited") {
		t.Fatal("the retry notice was printed")
	}
}

// Enter pressed again and again while the agent works queues one joined
// prompt, sent when the turn ends, and never several.
func TestEnterWhileBusyQueuesOnePrompt(t *testing.T) { bothSizes(t, testEnterWhileBusyQueuesOnePrompt) }

func testEnterWhileBusyQueuesOnePrompt(t *testing.T, cols, rows int) {
	h := start(t, "queue.jsonl", cols, rows)
	h.keys("a\r")
	h.waitScreen("busy", func(s string) bool { return strings.Contains(s, "waiting") })
	h.keys("b\r")
	h.keys("\r\r")
	h.keys("c\r")
	h.keys("\r")
	h.waitScreen("two queued", func(s string) bool { return strings.Contains(s, "2 queued") })
	h.waitAll("Second reply.")
	h.waitIdle()
	if got := h.prompts(); len(got) != 2 || got[0] != "a" || got[1] != "b\n\nc" {
		t.Fatalf("prompts %q, want a then b\\n\\nc", got)
	}
	orderedIn(t, h.all(), "› a", "First reply.", "› b", "  c", "Second reply.")
}

// Ctrl-C cancels the turn, not the agent: queued text comes back to the
// input, and the agent answers the next prompt.
func TestCtrlCCancelsAndAgentSurvives(t *testing.T) { bothSizes(t, testCtrlCCancelsAndAgentSurvives) }

func testCtrlCCancelsAndAgentSurvives(t *testing.T, cols, rows int) {
	h := start(t, "cancel.jsonl", cols, rows)
	h.keys("go\r")
	h.waitScreen("running", func(s string) bool { return strings.Contains(s, "running sleep 60") })
	h.keys("later\r")
	h.waitScreen("queued", func(s string) bool { return strings.Contains(s, "queued › later") })
	h.keys("\x03")
	h.waitAll("[cancelled]")
	h.waitIdle()
	if !strings.Contains(h.agentLog(), "cancel\n") {
		t.Fatalf("the agent never saw session/cancel:\n%s", h.agentLog())
	}
	all := h.all()
	orderedIn(t, all, "Working on it.", "sleep 60 · stopped", "[cancelled]")
	if !strings.Contains(h.screen(), "› later") {
		t.Fatalf("queued text not restored to the input:\n%s", h.screen())
	}
	h.keys("\r")
	h.waitAll("Still here.")
	if got := h.prompts(); len(got) != 2 || got[1] != "later" {
		t.Fatalf("prompts %q", got)
	}
}

func TestCtrlCTwiceQuits(t *testing.T) {
	h := start(t, "cancel.jsonl", 80, 24)
	h.keys("go\r")
	h.waitScreen("running", func(s string) bool { return strings.Contains(s, "running sleep 60") })
	h.keys("\x03")
	h.waitIdle()
	h.keys("\x03")
	h.waitDone()
	h.wait("the agent to exit", h.agentLog, func(s string) bool { return strings.Contains(s, "exit\n") })
}

func TestCtrlDQuits(t *testing.T) {
	h := start(t, "simple.jsonl", 80, 24)
	h.waitIdle()
	h.keys("x\x04") // Ctrl-D with text deletes forward; nothing happens
	time.Sleep(100 * time.Millisecond)
	select {
	case <-h.done:
		t.Fatal("Ctrl-D quit with text in the input")
	default:
	}
	h.keys("\x15\x04") // Ctrl-U clears the line, then Ctrl-D quits
	h.waitDone()
	h.wait("the agent to exit", h.agentLog, func(s string) bool { return strings.Contains(s, "exit\n") })
}

// Every newline key adds a newline to the input and sends nothing, and no
// escape sequence leaks as text. The original complaint was Ctrl+Enter
// showing up as "[27;5;13~".
func TestNewlineKeys(t *testing.T) { bothSizes(t, testNewlineKeys) }

func testNewlineKeys(t *testing.T, cols, rows int) {
	h := start(t, "simple.jsonl", cols, rows)
	h.waitIdle()
	h.keys("\x1b[?1u") // claim kitty keyboard support, as Ghostty would
	if cols >= 80 {    // at 40 columns the mode takes the hint's room
		h.waitScreen("the kitty hint", func(s string) bool { return strings.Contains(s, "shift+enter for a new line") })
	}
	keys := map[string]string{
		"ctrl+enter (modifyOtherKeys)": "\x1b[27;5;13~",
		"ctrl+enter (kitty)":           "\x1b[13;5u",
		"shift+enter (kitty)":          "\x1b[13;2u",
		"alt+enter":                    "\x1b\r",
		"ctrl+j":                       "\n",
	}
	sent := 0
	for name, seq := range keys {
		h.keys("a" + seq + "b")
		h.waitScreen(name+" newline", func(s string) bool {
			return strings.Contains(s, "│ › a ") && strings.Contains(s, "│   b ")
		})
		if got := h.prompts(); len(got) != sent {
			t.Fatalf("%s sent a prompt: %q", name, got)
		}
		for _, leak := range []string{"27;5;13", "13;5u", "13;2u", "[2", "\x1b"} {
			if strings.Contains(h.all(), leak) {
				t.Fatalf("%s leaked %q:\n%s", name, leak, h.all())
			}
		}
		h.keys("\r")
		sent++
		h.wait(name+" prompt", func() string { return fmt.Sprint(len(h.prompts())) }, func(s string) bool { return s == fmt.Sprint(sent) })
		h.waitIdle()
		if got := h.prompts()[sent-1]; got != "a\nb" {
			t.Fatalf("%s: prompt %q, want a\\nb", name, got)
		}
	}
}

// The input box grows with what is typed, to about half the screen,
// and only then scrolls; the whole message is sent.
func TestInputGrows(t *testing.T) {
	h := start(t, "simple.jsonl", 80, 40)
	h.waitIdle()
	var lines []string
	for i := 1; i <= 12; i++ {
		lines = append(lines, fmt.Sprintf("item %d", i))
	}
	h.keys(strings.Join(lines, "\n"))
	screen := h.waitScreen("twelve rows of input", func(s string) bool { return strings.Contains(s, "│   item 12") })
	for _, l := range lines {
		if !strings.Contains(screen, l+" ") {
			t.Fatalf("%q is not on screen:\n%s", l, screen)
		}
	}
	h.keys("\r")
	h.wait("the prompt", func() string { return strings.Join(h.prompts(), "|") }, func(s string) bool { return s == strings.Join(lines, "\n") })
}

// Bubble Tea #1822 and #1736: a reply three screens long, printed at
// 40×10, keeps the view on screen and puts each line in scrollback once.
func TestLongReplyKeepsView(t *testing.T) {
	h := start(t, "long.jsonl", 40, 10)
	h.keys("go\r")
	h.waitAll("Paragraph 15 has")
	h.waitIdle()
	// While the last prints are in flight the view still shows their text,
	// so wait for each paragraph to be on the terminal exactly once.
	once := func(s string) bool {
		for i := 1; i <= 15; i++ {
			if count(s, fmt.Sprintf("Paragraph %d has", i)) != 1 {
				return false
			}
		}
		return true
	}
	all := h.wait("each paragraph once", h.all, once)
	screen := h.screen()
	if !strings.Contains(screen, "ready") || !strings.Contains(screen, "›") {
		t.Fatalf("the view is gone:\n%s", screen)
	}
	// Regression: a chunk that was one blank line was dropped, so the
	// paragraphs ran together.
	orderedIn(t, all, "› go", "\n\nParagraph 1 has", "in it.\n\nParagraph 2 has")
}

// The banner: --title wins, then agentInfo.title, then the agent's name;
// the body has the agent, its version, and the model when the session
// offers one.
func TestBanner(t *testing.T) {
	cases := []struct {
		name, flag, top, body string
		agentFlags            []string
	}{
		{"agent title and model", "", "╭─ relay · hub ─", "│ fakeagent 0 · claude-opus-5-5 ", []string{"-title", "relay · hub", "-model", "claude-opus-5-5"}},
		{"flag wins", "mine", "╭─ mine ─", "│ fakeagent 0 ", []string{"-title", "relay · hub"}},
		{"name", "", "╭─ fakeagent ─", "│ fakeagent 0 ", nil},
	}
	for _, c := range cases {
		for _, size := range [][2]int{{80, 24}, {40, 10}} {
			t.Run(fmt.Sprintf("%s %dx%d", c.name, size[0], size[1]), func(t *testing.T) {
				h := startWith(t, "simple.jsonl", size[0], size[1], c.flag, c.agentFlags...)
				h.waitAll(c.top)
				all := h.waitAll(c.body)
				orderedIn(t, all, c.top, c.body, "ctrl-c cancel · ctrl-d quit", "╰─")
				if n := count(all, "╰─"); n != 2 { // the banner, and the input box
					t.Fatalf("%d box bottoms:\n%s", n, all)
				}
				checkBoxes(t, size[0]-1, t.Name())
			})
		}
	}
}

// permit starts a turn that stops on a permission request and waits for
// the request's summary to have the keys.
func permit(t *testing.T, cols, rows int) *harness {
	h := start(t, "permission.jsonl", cols, rows)
	h.keys("go\r")
	h.waitScreen("the request", func(s string) bool { return strings.Contains(s, "permission needed") })
	h.answer() // esc: while typing a request takes no keys
	return h
}

// review opens the review of the request on offer from its summary.
func (h *harness) review() {
	h.t.Helper()
	h.keys("\r")
	h.waitScreen("the review", func(s string) bool { return strings.Contains(s, "Allow this tool call?") })
}

// readAll pages through the review to its end, then waits out the arm
// delay: allowing waits for both.
func (h *harness) readAll() {
	h.t.Helper()
	// Each page stays a moment: one replaced within a frame or two never
	// counts as drawn.
	time.Sleep(150 * time.Millisecond)
	for strings.Contains(h.screen(), "more below") {
		was := pageOf(h.screen())
		h.keys(" ")
		h.waitScreen("the next page", func(s string) bool { return pageOf(s) != was })
		time.Sleep(150 * time.Millisecond)
	}
	time.Sleep(core.ArmDelay + 100*time.Millisecond)
}

// pageOf is the review's "page N of M", or "".
func pageOf(screen string) string {
	i := strings.Index(screen, "page ")
	if i < 0 {
		return ""
	}
	end := strings.IndexAny(screen[i:], "·\n")
	if end < 0 {
		return screen[i:]
	}
	return screen[i : i+end]
}

// answer stops typing so the request's summary takes the keys, and
// waits until it does: esc on its own, since an escape sent with the
// next key reads as alt and that key.
func (h *harness) answer() {
	h.t.Helper()
	h.keys("\x1b")
	h.waitScreen("the request with the keys", func(s string) bool {
		return strings.Contains(s, "enter to review") && !strings.Contains(s, "esc, then enter to review")
	})
}

// A request that arrives while you type takes no keys: a y typed long
// after it is text, and the request says how to review it. Esc gives its
// summary the keys, which answers nothing; enter opens the review, and
// only once it is read, after its delay, does y allow.
func TestPermissionWaitsWhileTyping(t *testing.T) {
	h := start(t, "permission.jsonl", 80, 24)
	h.keys("go\r")
	h.waitScreen("the request, waiting", func(s string) bool {
		return strings.Contains(s, "permission needed") && strings.Contains(s, "esc, then enter to review")
	})
	time.Sleep(core.ArmDelay + 100*time.Millisecond)
	h.keys("yes y")
	h.waitScreen("typed", func(s string) bool { return strings.Contains(s, "› yes y") })
	time.Sleep(100 * time.Millisecond)
	if o := h.permissionOutcome(); o != "" {
		t.Fatalf("typing answered the request: %s", o)
	}
	h.answer()
	h.keys("y") // on the summary: nothing
	time.Sleep(core.ArmDelay + 100*time.Millisecond)
	if o := h.permissionOutcome(); o != "" {
		t.Fatalf("a y on the summary answered: %s", o)
	}
	h.review()
	h.keys("y") // right behind enter: too soon
	time.Sleep(100 * time.Millisecond)
	if o := h.permissionOutcome(); o != "" {
		t.Fatalf("a y right behind enter answered: %s", o)
	}
	h.readAll()
	h.keys("y")
	h.waitOutcome(`{"optionId":"allow","outcome":"selected"}`)
	h.waitScreen("the draft", func(s string) bool { return strings.Contains(s, "› yes y") })
}

func (h *harness) permissionOutcome() string {
	for _, l := range strings.Split(h.agentLog(), "\n") {
		if rest, ok := strings.CutPrefix(l, "permission "); ok {
			return rest
		}
	}
	return ""
}

func (h *harness) waitOutcome(want string) {
	h.t.Helper()
	h.wait("the outcome", func() string { return h.permissionOutcome() }, func(s string) bool { return s != "" })
	if got := h.permissionOutcome(); got != want {
		h.t.Fatalf("outcome %s, want %s", got, want)
	}
}

// y allows: the request shows the tool and its input, the status line
// shows the labels as they grow, and the turn carries on.
func TestPermissionAllow(t *testing.T) { bothSizes(t, testPermissionAllow) }

func testPermissionAllow(t *testing.T, cols, rows int) {
	h := permit(t, cols, rows)
	if st := h.waitScreen("the private badge", func(s string) bool { return privateBadge(s) }); !strings.Contains(st, "waiting for you") && cols >= 80 {
		t.Errorf("status does not say it is waiting:\n%s", st)
	}
	h.review()
	scr := h.screen()
	for _, want := range []string{"web_fetch", "https://example.com/page", "Allow once (y)", "Reject (n)"} {
		if cols >= 80 && !strings.Contains(scr, want) {
			t.Errorf("%q not on screen:\n%s", want, scr)
		}
	}
	h.readAll()
	h.keys("y")
	h.waitOutcome(`{"optionId":"allow","outcome":"selected"}`)
	h.waitAll("Fetched.")
	h.waitIdle()
	h.waitScreen("both badges", func(s string) bool {
		st := statusLine(s)
		return privateBadge(st) && strings.Contains(st, "contaminated")
	})
	h.waitAll("allowed: web_fetch")
	if p := h.prompts(); len(p) != 1 || p[0] != "go" {
		t.Fatalf("prompts %q: the y reached the input", p)
	}
	// A plain agent sent no review: relay-tui built the text itself, and
	// the allow carries no digest.
	if strings.Contains(h.agentLog(), "permission-meta") {
		t.Errorf("an allow to a plain agent carried _meta:\n%s", h.agentLog())
	}
}

// Keys before the review is read answer nothing but a refusal, and
// nothing typed while it waits reaches the input box or sends a prompt.
func TestPermissionNoAccidents(t *testing.T) { bothSizes(t, testPermissionNoAccidents) }

func testPermissionNoAccidents(t *testing.T, cols, rows int) {
	h := permit(t, cols, rows)
	h.keys("y\rhey") // y on the summary, enter opens the review, the rest is swallowed
	time.Sleep(core.ArmDelay + 100*time.Millisecond)
	if o := h.permissionOutcome(); o != "" {
		t.Fatalf("answered before it was armed: %s", o)
	}
	h.keys("more text") // swallowed
	h.keys("\r")        // enter takes the highlighted option, which is Reject
	h.waitOutcome(`{"optionId":"reject","outcome":"selected"}`)
	h.waitAll("Fetched.")
	h.waitIdle()
	if p := h.prompts(); len(p) != 1 {
		t.Fatalf("prompts %q", p)
	}
	if all := h.all(); strings.Contains(all, "hello") || strings.Contains(all, "more text") {
		t.Fatalf("typing got into the input:\n%s", all)
	}
	h.waitAll("rejected: web_fetch")
}

func TestPermissionEscCancels(t *testing.T) {
	h := permit(t, 80, 24)
	h.review()
	h.keys("\x1b")
	h.waitOutcome(`{"outcome":"cancelled"}`)
	h.waitAll("cancelled: web_fetch")
}

// Ctrl-C stops the turn and answers the request cancelled, as ACP wants.
func TestPermissionCtrlCCancels(t *testing.T) {
	h := permit(t, 80, 24)
	h.keys("\x03")
	h.waitOutcome(`{"outcome":"cancelled"}`)
	h.wait("the cancel", h.agentLog, func(s string) bool { return strings.Contains(s, "cancel\n") })
}

// q leaves the review for the dashboard and answers nothing; enter
// opens it again, read afresh.
func TestPermissionReviewBack(t *testing.T) {
	h := permit(t, 80, 24)
	h.review()
	h.keys("q")
	h.waitScreen("the summary again", func(s string) bool { return strings.Contains(s, "enter to review") })
	if h.permissionOutcome() != "" {
		t.Fatal("going back answered it")
	}
	h.review()
	h.keys("y")
	time.Sleep(100 * time.Millisecond)
	if h.permissionOutcome() != "" {
		t.Fatal("a y right behind reopening answered it")
	}
	h.readAll()
	h.keys("y")
	h.waitOutcome(`{"optionId":"allow","outcome":"selected"}`)
}

// bracketed is a paste as a terminal sends it.
func bracketed(s string) string { return "\x1b[200~" + s + "\x1b[201~" }

func (h *harness) resources() []string {
	var out []string
	for _, l := range strings.Split(h.agentLog(), "\n") {
		if rest, ok := strings.CutPrefix(l, "resource "); ok {
			out = append(out, rest)
		}
	}
	return out
}

// A paste is held and shown as a placeholder; on send the prompt is the
// typed text plus the paste as a resource block.
func TestPasteGoesAsResource(t *testing.T) {
	h := startWith(t, "simple.jsonl", 80, 24, "", "-embedded")
	h.waitIdle()
	h.keys("read ")
	h.keys(bracketed("line1\nline2\nline3"))
	h.waitScreen("the placeholder", func(s string) bool { return strings.Contains(s, "[pasted 3 lines · untrusted #1]") })
	if strings.Contains(h.screen(), "line2") {
		t.Fatalf("the paste is in the input:\n%s", h.screen())
	}
	h.keys("\r")
	h.waitAll("Hello there.")
	h.waitIdle()
	if p := h.prompts(); len(p) != 1 || p[0] != "read [paste:1]" {
		t.Fatalf("prompts %q", p)
	}
	want := `{"mimeType":"text/plain","text":"line1\nline2\nline3","uri":"paste:1"}`
	if r := h.resources(); len(r) != 1 || r[0] != want {
		t.Fatalf("resources %q, want %s", r, want)
	}
}

// A paste is untrusted when it arrives. Ctrl-Y marks the latest one in
// the input as trusted: the placeholder says so, and it goes as plain
// text, with a note in the transcript.
func TestTrustedPaste(t *testing.T) {
	h := startWith(t, "simple.jsonl", 80, 24, "", "-embedded")
	h.waitIdle()
	time.Sleep(core.ArmDelay + 100*time.Millisecond) // trusting is armed a moment after the screen is drawn
	h.keys("\x19")                                   // nothing to trust yet
	h.waitScreen("the word", func(s string) bool { return strings.Contains(s, "no untrusted paste in the input to trust") })
	h.keys(bracketed("trusted text"))
	h.waitScreen("the placeholder", func(s string) bool { return strings.Contains(s, "untrusted #1]") })
	time.Sleep(core.ArmDelay + 100*time.Millisecond) // trusting is armed a moment after the screen is drawn
	h.keys("\x19")
	h.waitScreen("trusted", func(s string) bool {
		return strings.Contains(s, "[pasted 1 line · trusted #1]") && !strings.Contains(s, "untrusted #1]")
	})
	h.keys("\r")
	h.waitAll("Hello there.")
	h.waitIdle()
	if p := h.prompts(); len(p) != 1 || p[0] != "trusted text" {
		t.Fatalf("prompts %q", p)
	}
	if r := h.resources(); len(r) != 0 {
		t.Fatalf("resources %q", r)
	}
	h.waitAll("as trusted text, not labelled")
}

// An agent that takes no embedded context gets the text in the prompt,
// and the transcript says it was not labelled.
func TestPasteWithoutEmbeddedContext(t *testing.T) {
	h := start(t, "simple.jsonl", 80, 24)
	h.waitIdle()
	h.keys(bracketed("SECRET"))
	h.waitScreen("the placeholder", func(s string) bool { return strings.Contains(s, "[pasted 1 line · untrusted #1]") })
	h.keys("\r")
	h.waitAll("Hello there.")
	h.waitIdle()
	if p := h.prompts(); len(p) != 1 || p[0] != "SECRET" {
		t.Fatalf("prompts %q", p)
	}
	h.waitAll("not labelled untrusted")
}
