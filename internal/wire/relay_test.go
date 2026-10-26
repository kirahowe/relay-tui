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

const displayUpdate = `{"sessionUpdate":"tool_call_update","toolCallId":"t","status":"completed","content":[
 {"type":"content","content":{"type":"text","text":"plain result"}},
 {"type":"content","content":{"type":"text","text":"Title\n\nbody","_meta":{"relay/display":{"title":"Title","body":"body https://a.test","labels":["contaminated","private","odd"],
   "sources":[{"title":"S","ref":"web:https://a.test"}],"ref":"shown:2026-10-18/t"}}}}]}`

func TestDisplayMetaIsParsedAndKeptOutOfOutput(t *testing.T) {
	u := Update(decode(t, displayUpdate), t0).(core.ToolUpdated)
	want := []core.Display{{Title: "Title", Body: "body https://a.test", Labels: core.Labels{Private: true, Contaminated: true},
		Sources: []core.Source{{Title: "S", Ref: "web:https://a.test"}}, Ref: "shown:2026-10-18/t"}}
	if !reflect.DeepEqual(u.Displays, want) {
		t.Errorf("displays %#v", u.Displays)
	}
	if u.Output == nil || *u.Output != "plain result" {
		t.Errorf("output %v: a display block leaked into the tool output", u.Output)
	}
	// The same on a call that arrives finished.
	started := Update(decode(t, `{"sessionUpdate":"tool_call","toolCallId":"t","title":"x","status":"completed","content":[
	 {"type":"content","content":{"type":"text","text":"out"}},
	 {"type":"content","content":{"type":"text","text":"shown","_meta":{"relay/display":{"title":"T"}}}}]}`), t0).(core.ToolStarted)
	if started.Output != "out" || len(started.Displays) != 1 || started.Displays[0].Title != "T" {
		t.Errorf("started %#v", started)
	}
}

func TestUnreadableDisplayFailsClosed(t *testing.T) {
	u := Update(decode(t, `{"sessionUpdate":"tool_call_update","toolCallId":"t","content":[
	 {"type":"content","content":{"type":"text","text":"raw words","_meta":{"relay/display":"not an object"}}}]}`), t0).(core.ToolUpdated)
	if len(u.Displays) != 1 || !u.Displays[0].Labels.Contaminated || !u.Displays[0].Labels.Private || u.Displays[0].Body != "raw words" {
		t.Errorf("displays %#v", u.Displays)
	}
	if *u.Output != "" {
		t.Errorf("output %q", *u.Output)
	}
}

// However the blocks are mixed, none that carries a display marker
// reaches a tool's output.
func TestMarkedBlocksNeverReachOutput(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		var content []acp.ToolCallContent
		var plain []string
		for range rapid.IntRange(0, 6).Draw(t, "blocks") {
			text := rapid.StringMatching(`[a-z ]{1,12}`).Draw(t, "text")
			block := &acp.ContentBlockText{Type: "text", Text: text}
			switch rapid.IntRange(0, 1).Draw(t, "marker") {
			case 0:
				plain = append(plain, text)
			case 1:
				block.Meta = map[string]json.RawMessage{"relay/display": json.RawMessage(rapid.SampledFrom([]string{`{}`, `7`, `null`, `"x"`}).Draw(t, "v"))}
			}
			content = append(content, acp.ToolCallContent{Content: &acp.ToolCallContentContent{Content: acp.ContentBlock{Text: block}}})
		}
		got := contentText(content)
		want := ""
		for i, p := range plain {
			if i > 0 {
				want += "\n"
			}
			want += p
		}
		if got != want {
			t.Fatalf("output %q, want %q", got, want)
		}
	})
}

func TestResolution(t *testing.T) {
	ok := Resolution("[[fact:1]]", json.RawMessage(`{"ref":"[[fact:1]]","kind":"fact","status":"ok","title":"Fact 1","text":"t","labels":["private"]}`), nil, t0)
	if ok.Failed || ok.Cite.Title != "Fact 1" || ok.Cite.Status != "ok" || !ok.Cite.Labels.Private {
		t.Errorf("ok %#v", ok)
	}
	ext := Resolution("[[web:u]]", json.RawMessage(`{"kind":"web","status":"external","target":"https://a.test"}`), nil, t0)
	if ext.Cite.Target != "https://a.test" || ext.Cite.Status != "external" {
		t.Errorf("external %#v", ext)
	}
	for name, r := range map[string]core.CitationResolved{
		"error":      Resolution("[[x]]", nil, errors.New("e"), t0),
		"null kind":  Resolution("[[x]]", json.RawMessage(`{"kind":null,"status":"ok"}`), nil, t0),
		"odd status": Resolution("[[x]]", json.RawMessage(`{"kind":"fact","status":"fine"}`), nil, t0),
		"garbage":    Resolution("[[x]]", json.RawMessage(`7`), nil, t0),
	} {
		if !r.Failed {
			t.Errorf("%s: not failed", name)
		}
	}
}

func TestItems(t *testing.T) {
	// An older relay also lists held calls (proposals), and trust fields on
	// a shown item; only what was shown is read.
	raw := json.RawMessage(`{"items":[
	 {"kind":"proposal","callId":"c","tool":"send","input":{"g":"Kira only"},"preview":null,"labels":["private"],"status":"waiting","at":"2026-10-18T20:02:00Z"},
	 {"kind":"shown","ref":"shown:d/x","path":"shown/d/x.md","callId":"x","title":"T","text":"body","sources":[{"title":"S","ref":"web:https://a.test"}],
	  "labels":["contaminated"],"version":412,"sha256":"ab","canTrust":true,"trustCommand":"relay trust doc shown/d/x.md"},
	 {"kind":"future","title":"unknown kind"}]}`)
	got := Items(raw, nil, t0)
	if got.Failed || got.Unavailable || len(got.Items) != 1 {
		t.Fatalf("got %#v", got)
	}
	sh := got.Items[0]
	if sh.Path != "shown/d/x.md" || sh.Title != "T" || sh.Text != "body" || !sh.Labels.Contaminated || sh.Sources[0].Ref != "web:https://a.test" {
		t.Errorf("shown %#v", sh)
	}
}

func TestItemsErrors(t *testing.T) {
	if got := Items(nil, acp.NewMethodNotFound("_relay/items"), t0); !got.Unavailable || got.Failed {
		t.Errorf("method not found: %#v", got)
	}
	if got := Items(nil, errors.New("boom"), t0); got.Unavailable || !got.Failed {
		t.Errorf("other error: %#v", got)
	}
	if got := Items(json.RawMessage(`[1]`), nil, t0); !got.Failed {
		t.Errorf("malformed: %#v", got)
	}
}
