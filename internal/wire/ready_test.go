package wire

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	acp "github.com/caelis-labs/acp-go-sdk"
	"pgregory.net/rapid"

	"github.com/kirahowe/relay-tui/internal/core"
)

// readyWire is what relay answers to _relay/ready: produced by relay's own
// relay.ready/wire for a message with a reply, a draft, an event and a
// finding, then one more item from a mail, and a file move.
const readyWire = `{"rev": 99, "items": [
 {"id": "card:7", "kind": "finding", "at": "2026-10-14T12:03:00Z", "seq": 7, "title": "Venues to call",
  "lines": ["0 of 2 done"], "from": null, "outsider": false, "card": 7},
 {"id": "event:6", "kind": "event", "at": "2026-10-14T12:02:00Z", "title": "Choir concert", "lines": ["Town hall"],
  "start": "2026-11-21T00:00:00Z", "end": "2026-11-21T01:00:00Z", "from": null, "outsider": false},
 {"id": "prepared:8", "kind": "draft", "at": "2026-10-14T12:01:30Z", "title": "Reply to the booker",
  "lines": ["Saved draft c2.", "Hello,"], "from": "from a mail by booker@example.org", "outsider": true},
 {"id": "prepared:5", "kind": "draft", "at": "2026-10-14T12:01:00Z", "title": "Concert in May",
  "lines": ["Saved draft c1.", "Hello"], "from": null, "outsider": false},
 {"id": "message:4", "kind": "message", "at": "2026-10-14T12:00:00Z", "title": "Concert email is drafted",
  "lines": ["It is in your drafts."], "from": null, "outsider": false, "message": 4, "messageKind": "note",
  "urgency": "normal", "thread": 4, "replyTo": 4,
  "replies": [{"at": "2026-10-14T12:03:00Z", "text": "Shorter please."}]},
 {"id": "move:3", "kind": "move", "at": "2026-10-14T11:00:00Z", "seq": 3, "title": "statement.pdf",
  "lines": ["1 file in f", "Downloads/statement.pdf → Bank/statement.pdf"],
  "from": "from a mail by bank@example.org", "outsider": true, "move": 3, "role": "steward", "undo": 3, "undone": false}]}`

func TestReadyReadsRelaysWireForm(t *testing.T) {
	got := Ready(json.RawMessage(readyWire), nil, t0)
	if got.Err != "" || got.Unavailable || got.Rev != 99 || len(got.Items) != 6 {
		t.Fatalf("%+v", got)
	}
	want := core.ReadyItem{ID: "message:4", Kind: "message", At: "2026-10-14T12:00:00Z", Title: "Concert email is drafted",
		Lines: []string{"It is in your drafts."}, Message: 4, MessageKind: "note", Urgency: "normal", ReplyTo: 4,
		Replies: []core.ReadyReply{{At: "2026-10-14T12:03:00Z", Text: "Shorter please."}}}
	if !reflect.DeepEqual(got.Items[4], want) {
		t.Errorf("message\n%+v\nwant\n%+v", got.Items[4], want)
	}
	if d := got.Items[2]; d.From != "from a mail by booker@example.org" || !d.Outsider || d.ReplyTo != 0 {
		t.Errorf("draft %+v", d)
	}
	if e := got.Items[1]; e.Start != "2026-11-21T00:00:00Z" || e.End != "2026-11-21T01:00:00Z" {
		t.Errorf("event %+v", e)
	}
	if f := got.Items[0]; f.Card != 7 || f.Kind != "finding" || f.Seq != 7 {
		t.Errorf("finding %+v", f)
	}
	if m := got.Items[5]; m.Kind != core.ReadyMove || m.Move != 3 || m.Undo != 3 || m.Role != "steward" || m.Undone || m.Seq != 3 {
		t.Errorf("move %+v", m)
	}
}

func TestReadyLeavesOutWhatItCannotRead(t *testing.T) {
	got := Ready(json.RawMessage(`{"rev": 1, "items": [{"kind": "draft", "title": "no id"},
	  {"id": "a", "kind": "draft", "title": "T"}]}`), nil, t0)
	if len(got.Items) != 1 || got.Items[0].ID != "a" {
		t.Fatalf("%+v", got.Items)
	}
	// A missing outsider flag reads as outside text: relay did not vouch for it.
	if !got.Items[0].Outsider {
		t.Errorf("an item with no outsider flag is shown as outside text")
	}
}

func TestReadyAnswersAndFailures(t *testing.T) {
	if got := Ready(nil, &acp.RequestError{Code: -32601, Message: "no method"}, t0); !got.Unavailable {
		t.Errorf("a plain agent has nothing ready: %+v", got)
	}
	if got := Ready(nil, errors.New("boom"), t0); got.Err == "" || got.Unavailable {
		t.Errorf("%+v", got)
	}
	for _, bad := range []string{`[1]`, `{"rev": 1}`, `{"rev": 1, "items": [], "items": []}`, `{"rev": 1, "items": []} {}`} {
		if got := Ready(json.RawMessage(bad), nil, t0); got.Err == "" {
			t.Errorf("%s was read: %+v", bad, got)
		}
	}
}

func TestReadyChangedReadsItemsAndGone(t *testing.T) {
	got, ok := ReadyChanged(json.RawMessage(`{"rev": 100, "items": [{"id": "a", "kind": "draft", "title": "T", "outsider": false}],
	  "gone": ["event:6"]}`), t0)
	if !ok || got.Rev != 100 || len(got.Items) != 1 || !reflect.DeepEqual(got.Gone, []string{"event:6"}) {
		t.Fatalf("%+v %v", got, ok)
	}
	if _, ok := ReadyChanged(json.RawMessage(`[1]`), t0); ok {
		t.Errorf("read a list")
	}
	if _, ok := ReadyChanged(json.RawMessage(`{"rev": 1} {}`), t0); ok {
		t.Errorf("read two values")
	}
}

func TestReplyParamsAndOutcome(t *testing.T) {
	if p := ReplyParams(4, "Thanks"); !reflect.DeepEqual(p, map[string]any{"inReplyTo": int64(4), "text": "Thanks"}) {
		t.Errorf("%v", p)
	}
	if len(ReadyParams()) != 0 {
		t.Errorf("_relay/ready takes no parameters here")
	}
	if got := ReplyOutcome(4, nil, t0); got.Err != "" || got.Message != 4 {
		t.Errorf("%+v", got)
	}
	err := &acp.RequestError{Code: -32000, Message: "The one who told you that is no longer here."}
	if got := ReplyOutcome(4, err, t0); got.Err != err.Message {
		t.Errorf("%+v", got)
	}
}

func TestUndoParamsAndOutcome(t *testing.T) {
	if p := UndoParams(3); !reflect.DeepEqual(p, map[string]any{"move": int64(3)}) {
		t.Errorf("%v", p)
	}
	got := UndoOutcome(3, json.RawMessage(`{"lines": ["Undid move 3: put back 1 of 1."]}`), nil, t0)
	if got.Err != "" || got.Move != 3 || !reflect.DeepEqual(got.Lines, []string{"Undid move 3: put back 1 of 1."}) {
		t.Errorf("%+v", got)
	}
	err := &acp.RequestError{Code: -32000, Message: "Move 3 was not undone, and nothing changed: /f/Bank/statement.pdf has changed since it was moved."}
	if got := UndoOutcome(3, nil, err, t0); got.Err != err.Message {
		t.Errorf("%+v", got)
	}
	if got := UndoOutcome(3, json.RawMessage(`{"lines": 1}`), nil, t0); got.Err == "" {
		t.Errorf("an answer it cannot read is a failure: %+v", got)
	}
}

// Whatever relay sends, reading it never panics and never invents an id.
func TestReadyReadingIsTotal(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		raw := rapid.StringMatching(`[\[\]{}",:a-z0-9 .\-]{0,80}`).Draw(t, "raw")
		got := Ready(json.RawMessage(raw), nil, t0)
		for _, it := range got.Items {
			if it.ID == "" {
				t.Fatalf("an item with no id: %+v", it)
			}
		}
		ReadyChanged(json.RawMessage(raw), t0)
	})
}

func TestAnActsOfADailyNoteAreRead(t *testing.T) {
	raw := json.RawMessage(`{"rev": 3, "items": [{"id": "message:9", "kind": "message", "at": "2026-10-14T12:00:00Z", "seq": 9,
 "title": "Daily note", "lines": ["1. A change", "And 2 more changes like these", "2. Dentist", "3. A suggestion"], "from": null, "outsider": false,
 "message": 9, "messageKind": "digest", "urgency": "normal", "thread": 9, "replyTo": 9, "replies": [],
 "acts": [{"n": 1, "op": "revert", "line": 0, "done": null}, {"n": 3, "op": "dismiss", "line": 3, "done": "dismissed"}]}]}`)
	got := Ready(raw, nil, t0)
	want := []core.ReadyAct{{N: 1, Op: "revert", Line: 0}, {N: 3, Op: "dismiss", Line: 3, Done: "dismissed"}}
	if got.Err != "" || len(got.Items) != 1 || !reflect.DeepEqual(got.Items[0].Acts, want) {
		t.Fatalf("%+v", got)
	}
}

func TestActParamsAndOutcome(t *testing.T) {
	if p := ActParams(9, 3); !reflect.DeepEqual(p, map[string]any{"message": int64(9), "item": 3}) {
		t.Errorf("%v", p)
	}
	if got := ActOutcome(9, 3, "dismiss", nil, t0); got != (core.ActDone{At: t0, Message: 9, N: 3, Op: "dismiss"}) {
		t.Errorf("%+v", got)
	}
	err := &acp.RequestError{Code: -32000, Message: "Item 3 was dropped already."}
	if got := ActOutcome(9, 3, "dismiss", err, t0); got.Err != err.Message {
		t.Errorf("%+v", got)
	}
}

func TestUndoSummaryIsRead(t *testing.T) {
	got := UndoOutcome(3, json.RawMessage(`{"lines": ["Undid move 3: put back 3 of 3."], "summary": "Put back 3 files."}`), nil, t0)
	if got.Summary != "Put back 3 files." {
		t.Errorf("%+v", got)
	}
}
