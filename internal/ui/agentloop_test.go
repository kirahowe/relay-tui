package ui_test

// An end-to-end check against the real agentloop binary, with a fake
// Messages API on localhost standing in for the model. No real model is
// called and nothing leaves the machine. The binary is not built here: its
// path is the -agentloop flag, which defaults to the sibling checkout's
// bin/agentloop, and the tests skip when it is absent.

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/charmbracelet/colorprofile"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kirahowe/relay-tui/internal/agent"
	"github.com/kirahowe/relay-tui/internal/core"
	"github.com/kirahowe/relay-tui/internal/ui"
)

var agentloopBin = flag.String("agentloop", filepath.Join("..", "..", "..", "agentloop", "bin", "agentloop"),
	"path to an agentloop binary for the end-to-end tests with it")

// fakeMessagesAPI serves Anthropic's Models API and a scripted sequence of
// streamed Messages replies.
type fakeMessagesAPI struct {
	*httptest.Server
	mu      sync.Mutex
	replies []func(http.ResponseWriter)
	n       int
	bodies  []string // the request bodies, in order
}

// requests is the Messages request bodies the model was sent so far.
func (f *fakeMessagesAPI) requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.bodies...)
}

func newFakeMessagesAPI(t *testing.T, replies ...func(http.ResponseWriter)) *fakeMessagesAPI {
	f := &fakeMessagesAPI{replies: replies}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id, ok := strings.CutPrefix(r.URL.Path, "/v1/models/"); ok && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"type":"model","id":%q,"display_name":"Claude Test","created_at":"2026-01-01T00:00:00Z","max_input_tokens":200000,"max_tokens":64000,"capabilities":{}}`, id)
			return
		}
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		n := f.n
		f.n++
		f.bodies = append(f.bodies, string(body))
		f.mu.Unlock()
		if n >= len(f.replies) {
			http.Error(w, `{"type":"error","error":{"type":"invalid_request_error","message":"unexpected request"}}`, 400)
			return
		}
		f.replies[n](w)
	}))
	t.Cleanup(f.Close)
	return f
}

func sseReply(blockStart, delta, stop string) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, e := range [][2]string{
			{"message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-test","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":1}}}`},
			{"content_block_start", blockStart},
			{"content_block_delta", delta},
			{"content_block_stop", `{"type":"content_block_stop","index":0}`},
			{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"` + stop + `","stop_sequence":null,"stop_details":null},"usage":{"output_tokens":7}}`},
			{"message_stop", `{"type":"message_stop"}`},
		} {
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e[0], e[1])
		}
	}
}

func apiText(text string) func(http.ResponseWriter) {
	b, _ := json.Marshal(text)
	return sseReply(`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":`+string(b)+`}}`, "end_turn")
}

func apiTool(id, name, input string) func(http.ResponseWriter) {
	b, _ := json.Marshal(input)
	return sseReply(`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"`+id+`","name":"`+name+`","input":{}}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":`+string(b)+`}}`, "tool_use")
}

// startAgentloop runs relay-tui against the real agentloop, with no
// tools, so the request needs no policy and any agentloop takes it.
func startAgentloop(t *testing.T, api *fakeMessagesAPI) *harness {
	t.Helper()
	bin, err := filepath.Abs(*agentloopBin)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(bin); err != nil {
		t.Skipf("no agentloop binary at %s (build it, or pass -agentloop): %v", bin, err)
	}
	dir := t.TempDir()
	req := map[string]any{
		"provider":        "anthropic",
		"model":           "claude-test",
		"base_url":        api.URL,
		"system":          "be brief",
		"api_key_command": []string{"/usr/bin/printf", "%s", "test-key"},
		"session_dir":     filepath.Join(dir, "sessions"),
		"retry":           map[string]any{"retries": 0},
	}
	b, _ := json.Marshal(req)
	reqPath := filepath.Join(dir, "request.json")
	if err := os.WriteFile(reqPath, b, 0o644); err != nil {
		t.Fatal(err)
	}

	w, hgt := 100, 40
	h0 := &harness{t: t, emu: newTerminal(w, hgt), done: make(chan struct{})}
	started := make(chan struct{})
	conn, err := agent.Start([]string{bin, "acp", reqPath}, dir, io.Discard, func(e core.Event) {
		<-started
		h0.p.Send(ui.EventMsg{Event: e})
	})
	if err != nil {
		t.Fatal(err)
	}
	h0.conn = conn
	h0.p = tea.NewProgram(ui.New(conn, ""), tea.WithInput(h0.emu.in), tea.WithOutput(h0.emu),
		tea.WithWindowSize(w, hgt), tea.WithoutSignals(), tea.WithColorProfile(colorprofile.TrueColor), tea.WithFilter(ui.KeepWidthMethod))
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
	return h0
}

func (h *harness) pasteAndSend(text string) {
	h.keys("post it ")
	h.keys(bracketed(text))
	h.waitScreen("the placeholder", func(s string) bool { return strings.Contains(s, "[pasted 1 line #1]") })
	h.keys("\r")
}

// What Kira pastes reaches the model as her own text, in the prompt: no
// resource block, no label, and nothing is held or asked.
func TestAgentloopPasteIsPlainText(t *testing.T) {
	api := newFakeMessagesAPI(t, apiText("all posted"))
	h := startAgentloop(t, api)
	const pasted = "Please post this to the group."
	h.pasteAndSend(pasted)
	h.waitAll("all posted")
	h.waitIdle()
	reqs := api.requests()
	if len(reqs) == 0 || !strings.Contains(reqs[0], pasted) || strings.Contains(reqs[0], "paste:1") {
		t.Fatalf("the first request to the model did not carry the pasted text as text: %v", reqs)
	}
	all := h.all()
	for _, not := range []string{"permission needed", "allowed:", "rejected:", "contaminated", "private", "untrusted"} {
		if strings.Contains(all, not) {
			t.Errorf("%q on screen:\n%s", not, all)
		}
	}
}
