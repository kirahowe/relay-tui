package ui_test

// End to end for hub mode, in process: the real ui.Model and
// internal/agent against the fake hub (cmd/fakehub), which speaks the
// protocol of relay's hub. A second device is played through the fake
// hub's control file.

import (
	"encoding/json"
	"github.com/charmbracelet/colorprofile"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kirahowe/relay-tui/internal/agent"
	"github.com/kirahowe/relay-tui/internal/core"
	"github.com/kirahowe/relay-tui/internal/fakehub"
	"github.com/kirahowe/relay-tui/internal/ui"
)

type hubHarness struct {
	*harness
	control string
}

// threeRoles is a hub where Dev already has a conversation Kira has not
// read, and Mail and News have none.
func threeRoles() fakehub.Config {
	return fakehub.Config{Roles: []fakehub.RoleConfig{
		{ID: "dev", Title: "Dev", Live: &fakehub.Live{Unread: true, History: []fakehub.Said{{User: "hi"}, {Agent: "hello from dev"}}}},
		{ID: "mail", Title: "Mail", Labels: []string{"private"}},
		{ID: "news", Title: "News"},
	}}
}

func startHub(t *testing.T, cfg fakehub.Config, w, h int, open string) *hubHarness {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "hub.json")
	b, _ := json.Marshal(cfg)
	if err := os.WriteFile(cfgPath, b, 0o600); err != nil {
		t.Fatal(err)
	}
	hh := &hubHarness{
		harness: &harness{t: t, emu: newTerminal(w, h), log: filepath.Join(dir, "hub.log"), done: make(chan struct{})},
		control: filepath.Join(dir, "control"),
	}
	if err := os.WriteFile(hh.control, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	conn, err := agent.Start([]string{fakeHub, "-log", hh.log, "-control", hh.control, cfgPath}, dir, os.Stderr,
		func(e core.Event) {
			<-started
			hh.p.Send(ui.EventMsg{Event: e})
		})
	if err != nil {
		t.Fatal(err)
	}
	hh.conn = conn
	hh.p = tea.NewProgram(ui.New(conn, "").Open(open), tea.WithInput(hh.emu.in), tea.WithOutput(hh.emu),
		tea.WithWindowSize(w, h), tea.WithoutSignals(), tea.WithColorProfile(colorprofile.TrueColor), tea.WithFilter(ui.KeepWidthMethod))
	close(started)
	go func() { _, _ = hh.p.Run(); close(hh.done) }()
	t.Cleanup(func() {
		hh.p.Kill()
		select {
		case <-hh.done:
		case <-time.After(5 * time.Second):
			t.Error("the program did not stop")
		}
		conn.Shutdown()
		hh.emu.close()
	})
	lastHarness = nil
	return hh
}

// say appends a command for the fake hub, as a second device would act.
func (h *hubHarness) say(cmd string) {
	h.t.Helper()
	f, err := os.OpenFile(h.control, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		h.t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(cmd + "\n"); err != nil {
		h.t.Fatal(err)
	}
}

// logged is the fake hub's log lines that start with prefix.
func (h *hubHarness) logged(prefix string) []string {
	var out []string
	for _, l := range strings.Split(h.agentLog(), "\n") {
		if strings.HasPrefix(l, prefix) {
			out = append(out, l)
		}
	}
	return out
}

func (h *hubHarness) waitLog(prefix string) string {
	h.t.Helper()
	var got string
	h.wait("log "+prefix, h.agentLog, func(s string) bool {
		for _, l := range strings.Split(s, "\n") {
			if strings.HasPrefix(l, prefix) {
				got = l
				return true
			}
		}
		return false
	})
	return got
}

const (
	alt1 = "\x1b1"
	alt2 = "\x1b2"
)

// escape stops typing. It goes on its own and waits for the mode to
// change: an escape sent together with the next key reads as alt and
// that key.
func (h *harness) escape() {
	h.t.Helper()
	if s := h.screen(); strings.Contains(s, "MOVING") && !strings.Contains(s, "TYPING") {
		return // moving already, as after answering a request
	}
	h.keys("\x1b")
	h.waitScreen("moving", contains("MOVING"))
}

func contains(sub string) func(string) bool {
	return func(s string) bool { return strings.Contains(s, sub) }
}

func TestHubListThenOpenReplaysTheConversation(t *testing.T) {
	h := startHub(t, threeRoles(), 100, 24, "")
	s := h.waitScreen("the roster", contains("News"))
	if !strings.Contains(s, "●") { // Dev is unread
		t.Errorf("no unread mark:\n%s", s)
	}
	h.keys("\r") // the list starts on the first role
	s = h.waitScreen("the replay", contains("hello from dev"))
	if !strings.Contains(s, "› hi") {
		t.Errorf("Kira's own prompt is not replayed:\n%s", s)
	}
	// Looking at the latest turn tells relay.
	h.waitLog("seen fh-dev-1 1")
	h.waitScreen("no unread mark", func(s string) bool { return !strings.Contains(s, "●") })
	if n := len(h.logged("load ")); n != 1 {
		t.Errorf("%d loads", n)
	}
}

func TestHubOpenFlagAttachesAndPromptsEcho(t *testing.T) {
	h := startHub(t, threeRoles(), 100, 24, "dev")
	h.waitScreen("the replay", contains("hello from dev"))
	if l := h.waitLog("new dev"); l != "new dev attached=true" {
		t.Errorf("%q", l)
	}
	h.keys("what next\r")
	h.waitScreen("the echo", contains("echo: what next"))
	h.waitScreen("ready", contains("ready"))
	if got := h.waitLog("prompt fh-dev-1"); got != `prompt fh-dev-1 "what next"` {
		t.Errorf("%q", got)
	}
}

func TestHubSwitchingKeepsEveryConversationIntact(t *testing.T) {
	h := startHub(t, threeRoles(), 100, 24, "dev")
	h.waitScreen("dev", contains("hello from dev"))
	h.keys(alt2) // Mail has no conversation yet; still typing
	h.waitScreen("empty", contains("No conversation with Mail"))
	if n := len(h.logged("new mail")); n != 0 {
		t.Fatalf("moving to a role must not start a conversation: %v", h.logged("new"))
	}
	h.keys("to mail\r")
	h.waitScreen("mail echo", contains("echo: to mail"))
	if l := h.waitLog("new mail"); l != "new mail attached=false" {
		t.Errorf("%q", l)
	}
	h.keys(alt1)
	s := h.waitScreen("dev again", contains("hello from dev"))
	if strings.Contains(s, "echo: to mail") {
		t.Errorf("Mail's reply leaked into Dev:\n%s", s)
	}
	if count(s, "hello from dev") != 1 {
		t.Errorf("history shown twice:\n%s", s)
	}
	h.keys("\x1b2") // alt+2: Mail
	s = h.waitScreen("mail", contains("echo: to mail"))
	if strings.Contains(s, "hello from dev") {
		t.Errorf("Dev's history leaked into Mail:\n%s", s)
	}
	if n := len(h.logged("load ")) + len(h.logged("new dev")); n != 1 {
		t.Errorf("switching back and forth re-attached: %v", h.agentLog())
	}
}

func TestHubDraftsStayWithTheirRole(t *testing.T) {
	h := startHub(t, threeRoles(), 100, 24, "dev")
	h.waitScreen("dev", contains("hello from dev"))
	h.keys("half a thought")
	h.waitScreen("typed", contains("half a thought"))
	h.keys(alt2)
	s := h.waitScreen("mail", contains("No conversation with Mail"))
	if strings.Contains(s, "half a thought") {
		t.Errorf("the draft followed to the next role:\n%s", s)
	}
	h.keys(alt1)
	h.waitScreen("draft back", contains("half a thought"))
}

func TestHubQueuedTurnWaitsForScheduledWork(t *testing.T) {
	cfg := threeRoles()
	cfg.Roles[0].Busy = true
	h := startHub(t, cfg, 100, 24, "dev")
	h.waitScreen("dev", contains("hello from dev"))
	h.keys("please\r")
	h.waitScreen("queued", contains("waiting for the role's scheduled work"))
	h.waitScreen("the mark", contains("…"))
	h.say("unbusy dev")
	h.waitScreen("the reply", contains("echo: please"))
	h.waitScreen("no longer waiting", func(s string) bool { return !strings.Contains(s, "scheduled work") })
}

func TestHubAnotherDevicesPromptAppears(t *testing.T) {
	h := startHub(t, threeRoles(), 100, 24, "dev")
	h.waitScreen("dev", contains("hello from dev"))
	h.say("other-prompt dev from the phone")
	s := h.waitScreen("the prompt", contains("› from the phone"))
	h.waitScreen("the reply", contains("echo: from the phone"))
	if strings.Contains(s, "hello from dev\n\nhello from dev") {
		t.Errorf("duplicated")
	}
}

func TestHubEndAsksFirstThenEnds(t *testing.T) {
	h := startHub(t, threeRoles(), 100, 24, "dev")
	h.waitScreen("dev", contains("hello from dev"))
	h.escape()
	h.keys("X")
	h.waitScreen("the question", contains("End the conversation with Dev?"))
	h.keys("n")
	h.waitScreen("asked no", func(s string) bool { return !strings.Contains(s, "End the conversation") })
	if len(h.logged("end ")) != 0 {
		t.Fatal("ended after no")
	}
	h.keys("X")
	h.waitScreen("the question", contains("End the conversation with Dev?"))
	time.Sleep(core.ArmDelay + 100*time.Millisecond) // ending is armed a moment after the question is drawn
	h.keys("y")
	h.waitLog("end fh-dev-1")
	h.waitScreen("gone", contains("No conversation with Dev"))
	// The role is still there, and typing starts a new conversation.
	h.keys("i")
	h.waitScreen("typing", contains("TYPING"))
	h.keys("again\r")
	h.waitScreen("new conversation", contains("echo: again"))
	h.waitLog("new dev attached=false")
}

func TestHubConversationEndedElsewhereIsDropped(t *testing.T) {
	h := startHub(t, threeRoles(), 100, 24, "dev")
	h.waitScreen("dev", contains("hello from dev"))
	h.say("end dev")
	h.waitScreen("gone", contains("No conversation with Dev"))
}

func TestHubNarrowScreenShowsTheListOrTheConversation(t *testing.T) {
	h := startHub(t, threeRoles(), 44, 16, "")
	s := h.waitScreen("the list", contains("News"))
	if strings.Contains(s, "│ ") && strings.Contains(s, "relay") {
		t.Errorf("a sidebar on a narrow screen:\n%s", s)
	}
	h.keys("\r")
	s = h.waitScreen("the conversation", contains("hello from dev"))
	if strings.Contains(s, "News") || !strings.Contains(s, "h roles") {
		t.Errorf("narrow conversation:\n%s", s)
	}
	h.escape()
	h.keys("h")
	h.waitScreen("the list again", contains("News"))
	h.keys("l")
	h.waitScreen("the conversation again", contains("hello from dev"))
	h.keys("h")
	h.waitScreen("the list again", contains("News"))
	h.keys("\x1b") // esc goes back to the conversation
	h.waitScreen("the conversation again", contains("hello from dev"))
}

func TestHubWorkingFlagShowsInTheSidebar(t *testing.T) {
	cfg := threeRoles()
	h := startHub(t, cfg, 100, 24, "dev")
	h.waitScreen("dev", contains("hello from dev"))
	h.say("working news on")
	h.waitScreen("the mark", contains("~"))
	h.say("working news off")
	h.waitScreen("no mark", func(s string) bool { return !strings.Contains(s, "~") })
}

func TestHubHelpListsTheKeys(t *testing.T) {
	h := startHub(t, threeRoles(), 100, 30, "dev")
	h.waitScreen("dev", contains("hello from dev"))
	h.keys("?")
	h.waitScreen("the help", func(s string) bool { return strings.Contains(s, "typing keys") && strings.Contains(s, "alt+1") })
	h.keys("x")
	h.waitScreen("closed", func(s string) bool { return !strings.Contains(s, "any key closes") })
	h.escape()
	h.keys("x") // nothing while moving, and the input stays empty
	h.waitScreen("the note", contains("i to type"))
	h.keys("?")
	h.waitScreen("the moving keys", func(s string) bool { return strings.Contains(s, "moving keys") && strings.Contains(s, "X") })
}

func TestHubCtrlCWithAnEmptyInputQuits(t *testing.T) {
	h := startHub(t, threeRoles(), 100, 24, "dev")
	h.waitScreen("dev", contains("hello from dev"))
	h.keys("\x03")
	h.waitDone()
}

func TestHubCancelsASlowTurn(t *testing.T) {
	h := startHub(t, threeRoles(), 100, 24, "dev")
	h.waitScreen("dev", contains("hello from dev"))
	h.keys("slow\r")
	h.waitScreen("running", contains("▶"))
	h.keys("\x03")
	h.waitLog("cancel fh-dev-1")
	h.waitScreen("idle", contains("ready"))
}

// Against a hub with no board the pane falls back to the conversation's
// items, as against a plain agent.
func TestHubWorkspacePaneAndBoardWorkForTheFocusedConversation(t *testing.T) {
	h := startHub(t, threeRoles(), 100, 24, "dev")
	h.waitScreen("dev", contains("hello from dev"))
	h.escape()
	h.keys("l")
	h.waitScreen("the pane", contains("nothing yet"))
	h.keys("b")
	h.waitScreen("the board", contains("This agent has no board."))
	h.keys("h")
	h.waitScreen("the conversation", func(s string) bool { return !strings.Contains(s, "nothing yet") })
	// Moving to another role gives the keys to its conversation, and its
	// pane lists its own items.
	h.keys("2")
	s := h.waitScreen("mail", contains("No conversation with Mail"))
	if strings.Contains(s, "nothing yet") {
		t.Errorf("the pane followed to another role:\n%s", s)
	}
}

func TestHubScrollsTheConversation(t *testing.T) {
	cfg := threeRoles()
	var hist []fakehub.Said
	for i := 0; i < 30; i++ {
		hist = append(hist, fakehub.Said{User: "q" + string(rune('a'+i%26)) + string(rune('a'+i/26)), Agent: "reply number " + string(rune('A'+i%26)) + string(rune('a'+i/26))})
	}
	cfg.Roles[0].Live = &fakehub.Live{Unread: true, History: hist}
	h := startHub(t, cfg, 100, 24, "dev")
	h.waitScreen("the end", contains("reply number Db"))
	h.waitLog("seen fh-dev-1 15")
	h.keys("\x1b[5~") // page up
	s := h.waitScreen("scrolled", contains("more"))
	if strings.Contains(s, "reply number Db") {
		t.Errorf("the end is still showing:\n%s", s)
	}
	h.keys("\x1b[6~") // page down
	h.waitScreen("back at the end", contains("reply number Db"))
}

// With the pane beside the conversation (160 columns) the text area is as
// wide as the box drawn, so a long draft wraps and every character shows.
func TestHubLongDraftIsAllVisibleWithThePaneBeside(t *testing.T) {
	h := startHub(t, threeRoles(), 160, 24, "dev")
	h.waitScreen("dev", contains("hello from dev"))
	h.keys(strings.Repeat("z", 100))
	h.waitScreen("the draft", func(s string) bool { return count(s, "z") == 100 })
}

func manyTurns() fakehub.Config {
	cfg := threeRoles()
	var hist []fakehub.Said
	for i := 0; i < 30; i++ {
		hist = append(hist, fakehub.Said{User: "q" + string(rune('a'+i%26)) + string(rune('a'+i/26)), Agent: "reply number " + string(rune('A'+i%26)) + string(rune('a'+i/26))})
	}
	cfg.Roles[0].Live = &fakehub.Live{Unread: true, History: hist}
	return cfg
}

// --- Independent review

func TestHubEmbeddedModeIsSaidAllTheTime(t *testing.T) {
	cfg := threeRoles()
	cfg.Embedded = true
	h := startHub(t, cfg, 100, 24, "dev")
	h.waitScreen("the note", func(s string) bool {
		return strings.Contains(strings.Join(strings.Fields(s), " "), "relay service not running")
	})
	h.keys(alt2)
	h.waitScreen("still there", func(s string) bool {
		return strings.Contains(strings.Join(strings.Fields(s), " "), "relay service not running")
	})
}

func TestHubTruncatedReplayShowsRelaysNotice(t *testing.T) {
	cfg := threeRoles()
	cfg.Roles[0].Live.Truncated = true
	h := startHub(t, cfg, 100, 24, "dev")
	h.waitScreen("the notice", contains("earlier history is not shown"))
}

// The roster learns a turn ended a moment after the answer: a prompt sent
// in between is queued, not refused, and the next one still goes.
func TestHubPromptsAcrossTheRosterLagAreNotRefused(t *testing.T) {
	h := startHub(t, threeRoles(), 100, 24, "dev")
	h.waitScreen("dev", contains("hello from dev"))
	h.keys("one\r")
	h.waitScreen("first echo", contains("echo: one"))
	h.keys("two\r")
	h.waitScreen("second echo", contains("echo: two"))
	if got := h.logged("refused"); len(got) != 0 {
		t.Fatalf("the hub refused a prompt: %v", got)
	}
}

// A relay from before it dropped its holds still asks permission for a
// call, in whichever conversation, and tells the client when another
// device answered. relay-tui refuses every request with a plain cancel
// and says nothing about it: no mark in the sidebar, no summary, no note.
func TestHubAnOlderRelaysRequestsAreRefusedWithoutAWord(t *testing.T) {
	cfg := threeRoles()
	cfg.Roles[1].Live = &fakehub.Live{History: []fakehub.Said{{User: "a"}, {Agent: "b"}}}
	h := startHub(t, cfg, 100, 24, "dev")
	h.waitScreen("dev", contains("hello from dev"))
	if got := h.logged("initialize "); len(got) != 1 || got[0] != "initialize review=false" {
		t.Fatalf("initialize %q", got)
	}
	h.say("ask mail") // in the background
	if out := h.waitLog("permission fh-mail"); !strings.Contains(out, `"outcome":"cancelled"`) {
		t.Errorf("the background request was answered %q", out)
	}
	h.say("ask dev") // on screen
	if out := h.waitLog("permission fh-dev"); !strings.Contains(out, `"outcome":"cancelled"`) {
		t.Errorf("the request on screen was answered %q", out)
	}
	h.say("settle dev")
	h.keys("after\r")
	s := h.waitScreen("echo", contains("echo: after"))
	for _, not := range []string{"permission", "?1", "!1", "answered elsewhere", "allowed", "rejected", "review"} {
		if strings.Contains(s, not) {
			t.Errorf("%q on screen:\n%s", not, s)
		}
	}
}

// A paste in a hub conversation becomes a placeholder, with no word
// about trust.
func TestHubPasteBecomesAPlaceholder(t *testing.T) {
	h := startHub(t, threeRoles(), 100, 24, "dev")
	h.waitScreen("dev", contains("hello from dev"))
	h.keys("\x1b[200~line one\nline two\x1b[201~")
	s := h.waitScreen("the placeholder", contains("[pasted 2 lines #1]"))
	if strings.Contains(s, "trusted") {
		t.Errorf("the placeholder speaks of trust:\n%s", s)
	}
}
