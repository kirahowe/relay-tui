package wire

import (
	"encoding/json"
	"reflect"
	"testing"

	acp "github.com/caelis-labs/acp-go-sdk"

	"github.com/kirahowe/relay-tui/internal/core"
)

// relay says in initialize's _meta that it makes conversation text inert
// itself, as a positive version number. Anything else is a relay from
// before it, or plain ACP, in single-agent and hub mode alike.
func TestInertTextIsReadFromInitialize(t *testing.T) {
	for raw, want := range map[string]bool{
		"1": true, "2": true,
		"": false, "0": false, "-1": false, "1.5": false, "true": false, `"1"`: false, "null": false, "[1]": false,
	} {
		init := acp.InitializeResponse{Meta: map[string]json.RawMessage{"relay/hub": json.RawMessage("1")}}
		if raw != "" {
			init.Meta["relay/inertText"] = json.RawMessage(raw)
		}
		if got := HubConnected(init, t0).InertText; got != want {
			t.Errorf("hub, relay/inertText %q: %v, want %v", raw, got, want)
		}
		if got := Connected(init, acp.NewSessionResponse{}, t0).(core.Connected).InertText; got != want {
			t.Errorf("single agent, relay/inertText %q: %v, want %v", raw, got, want)
		}
	}
	if HubConnected(acp.InitializeResponse{}, t0).InertText {
		t.Error("an initialize with no _meta says relay/inertText")
	}
}

func chunkWith(u acp.SessionUpdate, meta string) acp.SessionNotification {
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(meta), &m); err != nil {
		panic(err)
	}
	switch {
	case u.AgentMessageChunk != nil:
		u.AgentMessageChunk.Meta = m
	case u.AgentThoughtChunk != nil:
		u.AgentThoughtChunk.Meta = m
	case u.ToolCall != nil:
		u.ToolCall.Meta = m
	}
	return acp.SessionNotification{SessionId: "s", Update: u}
}

// An agent message chunk carries the live links its text shows. They are
// read as a card's are: an entry the client could not use is dropped, and
// a list that cannot be read costs the chunk its links, never its text.
func TestAMessageChunkCarriesTheLinksRelayListed(t *testing.T) {
	const text = "Apply at hxxps://fund[.]example[.]org/apply today."
	list := `"relay/links": [
		{"address": "https://fund.example.org/apply", "shown": "hxxps://fund[.]example[.]org/apply"},
		{"address": "javascript:alert(1)", "shown": "hxxps://x[.]example"},
		{"address": "https://x.example/a", "shown": "https://x.example/a"},
		{"address": "https://x.example/a", "shown": "hxxps://x[.]example/a b"},
		{"address": "https://x.example/a", "shown": "hxxps://x[.]example/é"},
		{"address": "https://x.example/a", "shown": ""},
		{"address": "http://y.example", "shown": "hxxp://y[.]example"}]`
	want := []core.Link{
		{Address: "https://fund.example.org/apply", Shown: "hxxps://fund[.]example[.]org/apply"},
		{Address: "http://y.example", Shown: "hxxp://y[.]example"}}

	got := Update(chunkWith(acp.UpdateAgentMessageText(text), `{`+list+`, "relay/linksLeftOut": 3}`), t0)
	c, ok := got.(core.TextChunk)
	if !ok || c.Text != text || !reflect.DeepEqual(c.Links, want) || c.LinksLeftOut != 3 {
		t.Fatalf("got %+v", got)
	}

	// No more than relay's own limit are kept.
	var many []map[string]string
	for range core.MaxOwnLinks + 5 {
		many = append(many, map[string]string{"address": "https://x.example/a", "shown": "hxxps://x[.]example/a"})
	}
	raw, _ := json.Marshal(map[string]any{"relay/links": many})
	if c := Update(chunkWith(acp.UpdateAgentMessageText(text), string(raw)), t0).(core.TextChunk); len(c.Links) != core.MaxOwnLinks {
		t.Errorf("%d links kept of %d", len(c.Links), len(many))
	}

	// A list of the wrong type, or a count that is not one, costs only the
	// links.
	for _, meta := range []string{
		`{"relay/links": {"address": "https://fund.example.org/apply", "shown": "hxxps://fund[.]example[.]org/apply"}}`,
		`{"relay/links": "hxxps://fund[.]example[.]org/apply"}`,
		`{"relay/links": [1, 2]}`,
		`{"relay/links": null}`,
		`{}`,
	} {
		c, ok := Update(chunkWith(acp.UpdateAgentMessageText(text), meta), t0).(core.TextChunk)
		if !ok || c.Text != text || c.Links != nil || c.LinksLeftOut != 0 {
			t.Errorf("%s: %+v", meta, c)
		}
	}
	for _, left := range []string{`"3"`, `-1`, `1.5`} {
		c := Update(chunkWith(acp.UpdateAgentMessageText(text), `{`+list+`, "relay/linksLeftOut": `+left+`}`), t0).(core.TextChunk)
		if c.LinksLeftOut != 0 || !reflect.DeepEqual(c.Links, want) {
			t.Errorf("linksLeftOut %s: %+v", left, c)
		}
	}

	// A thought or a tool call never carries them: the keys change
	// nothing in what it is read as.
	for name, u := range map[string]func() acp.SessionUpdate{
		"a thought":   func() acp.SessionUpdate { return acp.UpdateAgentThoughtText(text) },
		"a tool call": func() acp.SessionUpdate { return acp.StartToolCall("c1", text) },
	} {
		with := Update(chunkWith(u(), `{`+list+`}`), t0)
		without := Update(acp.SessionNotification{SessionId: "s", Update: u()}, t0)
		if !reflect.DeepEqual(with, without) {
			t.Errorf("%s read relay/links: %+v", name, with)
		}
	}
}
