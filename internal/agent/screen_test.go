package agent

import (
	"bytes"
	"context"
	"io"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	acp "github.com/caelis-labs/acp-go-sdk"

	"github.com/kirahowe/relay-tui/internal/core"
	"github.com/kirahowe/relay-tui/internal/jsoncases"
)

// The shared table (relay#58) through the real reader: the screen, then
// the SDK's connection and relay-tui's client. Only the accepted lines whose
// exact method is _relay/cards/changed reach the board; the capitalised
// and folded ones, which encoding/json v1 alone would read as that method,
// do not. Every rejected line is dropped, counted and logged.
func TestSharedTableThroughTheConnection(t *testing.T) {
	cases := jsoncases.Read(t)
	var stream bytes.Buffer
	wantChanged, wantDropped := 0, 0
	for _, c := range cases {
		stream.Write(c.Line)
		stream.WriteByte('\n')
		if c.Reject {
			wantDropped++
		} else if c.Method != nil && *c.Method == "_relay/cards/changed" {
			wantChanged++
		}
	}
	stream.WriteString("\n   \n") // blank lines are not counted
	// Notifications run in order, so when this one arrives every line
	// before it has been handled.
	stream.WriteString(`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"end"}}}}` + "\n")

	var mu sync.Mutex
	var events []core.Event
	end := make(chan struct{})
	a := &Agent{emit: func(e core.Event) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, e)
		if c, ok := e.(core.TextChunk); ok && c.Text == "end" {
			close(end)
		}
	}}
	var log bytes.Buffer
	pr, pw := io.Pipe()
	src := newScreened(pr, &log)
	conn, err := acp.NewClientSideConnectionWithOptions(client{a}, io.Discard, src, acp.ConnectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	go func() { _, _ = pw.Write(stream.Bytes()) }()
	select {
	case <-end:
	case <-time.After(10 * time.Second):
		t.Fatal("the last line never arrived")
	}
	_ = pw.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = conn.Wait(ctx)

	mu.Lock()
	defer mu.Unlock()
	changed := 0
	for _, e := range events {
		if _, ok := e.(core.CardsChanged); ok {
			changed++
		}
	}
	if changed != wantChanged {
		t.Errorf("%d _relay/cards/changed reached the board, want %d", changed, wantChanged)
	}
	if got := int(src.dropped.Load()); got != wantDropped {
		t.Errorf("dropped %d lines, want %d", got, wantDropped)
	}
	if n := strings.Count(log.String(), "dropped a line from the agent"); n != wantDropped {
		t.Errorf("logged %d drops, want %d:\n%s", n, wantDropped, log.String())
	}
}

// A line longer than the SDK takes ends the stream with the error the SDK
// recognises, rather than being cut or skipped.
func TestScreenedLineTooLong(t *testing.T) {
	long := `{"jsonrpc":"2.0","method":"x","params":"` + strings.Repeat("a", maxLine) + `"}` + "\n"
	src := newScreened(strings.NewReader(long), io.Discard)
	_, err := io.ReadAll(src)
	if err == nil || !strings.Contains(err.Error(), "token too long") {
		t.Fatalf("err = %v", err)
	}
}

// Valid lines pass in order and whole, through reads of any size.
func TestScreenedPassesLines(t *testing.T) {
	in := "{\"jsonrpc\":\"2.0\",\"method\":\"a\"}\r\n{\"a\":1,\"a\":2}\n\n {\"jsonrpc\":\"2.0\",\"method\":\"b\"} \n{\"jsonrpc\":\"2.0\",\"method\":\"c\"}"
	src := newScreened(strings.NewReader(in), io.Discard)
	var out bytes.Buffer
	buf := make([]byte, 3)
	for {
		n, err := src.Read(buf)
		out.Write(buf[:n])
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	want := "{\"jsonrpc\":\"2.0\",\"method\":\"a\"}\n {\"jsonrpc\":\"2.0\",\"method\":\"b\"} \n{\"jsonrpc\":\"2.0\",\"method\":\"c\"}\n"
	if out.String() != want {
		t.Fatalf("got %q\nwant %q", out.String(), want)
	}
}

// relay-tui asks nothing: a session/request_permission is answered at once
// with a plain cancel, whatever it holds, and nothing reaches the screen.
// _relay/permission/settled is ignored.
func TestPermissionRequestsAreRefusedWithoutAWord(t *testing.T) {
	var mu sync.Mutex
	var events []core.Event
	a := &Agent{emit: func(e core.Event) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, e)
	}}
	pr, pw := io.Pipe()
	out, outw := io.Pipe()
	conn, err := acp.NewClientSideConnectionWithOptions(client{a}, outw, newScreened(pr, io.Discard), acp.ConnectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	replies := make(chan string, 8)
	go func() {
		buf := make([]byte, 4096)
		var acc string
		for {
			n, err := out.Read(buf)
			acc += string(buf[:n])
			for {
				i := strings.IndexByte(acc, '\n')
				if i < 0 {
					break
				}
				replies <- acc[:i]
				acc = acc[i+1:]
			}
			if err != nil {
				return
			}
		}
	}()
	ask := func(id int, input, options string) string {
		return `{"jsonrpc":"2.0","id":` + strconv.Itoa(id) + `,"method":"session/request_permission","params":{"sessionId":"s","toolCall":{"toolCallId":"c","name":"send","rawInput":` +
			input + `,"_meta":{"agentloop/labels":["contaminated"]}},"options":` + options +
			`,"_meta":{"relay/review":{"text":"t","sha256":"` + strings.Repeat("0", 64) + `"}}}}` + "\n"
	}
	const allow = `[{"optionId":"allow","name":"Allow","kind":"allow_once"},{"optionId":"no","name":"No","kind":"reject_once"}]`
	write := func(line string) { go func() { _, _ = pw.Write([]byte(line)) }() }
	for id, line := range []string{
		ask(1, `{"to":"a@example.com"}`, allow),
		ask(2, `{"n":9007199254740993}`, `[{"optionId":"a","name":"Always","kind":"allow_always"}]`),
		`{"jsonrpc":"2.0","method":"_relay/permission/settled","params":{"sessionId":"s","toolCallId":"c","outcome":{"outcome":"selected"}}}` + "\n",
	} {
		write(line)
		if id == 2 {
			continue // a notification has no reply
		}
		select {
		case r := <-replies:
			if !strings.Contains(r, `"outcome":"cancelled"`) || strings.Contains(r, "selected") || strings.Contains(r, "relay/review") {
				t.Errorf("request %d answered %s", id+1, r)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("request %d: no answer", id+1)
		}
	}
	// A request the SDK cannot decode (options of the wrong type) never
	// reaches the handler: the SDK itself answers with an error.
	write(ask(9, `{"n":1}`, `"x"`))
	select {
	case r := <-replies:
		if !strings.Contains(r, `"error"`) || strings.Contains(r, "selected") {
			t.Fatalf("an unreadable request was answered %s", r)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no answer to an unreadable request")
	}
	_ = pw.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = conn.Wait(ctx)
	_ = outw.Close()
	mu.Lock()
	defer mu.Unlock()
	if len(events) != 0 {
		t.Errorf("events reached the screen: %#v", events)
	}
}
