package core

import (
	"reflect"
	"testing"
)

// initialize's relay/inertText reaches the connection's state, against a
// single agent and against the hub, and its absence leaves it off.
func TestInertTextIsTheConnections(t *testing.T) {
	for _, on := range []bool{true, false} {
		x, _ := ReduceSingle(Single{}, Connected{At: t0, InertText: on})
		if x.App.Links.InertText != on {
			t.Errorf("single agent, inertText %v: %v", on, x.App.Links.InertText)
		}
		h, _ := ReduceHub(NewHub(""), HubConnected{At: t0, InertText: on})
		if h.App.Links.InertText != on {
			t.Errorf("hub, inertText %v: %v", on, h.App.Links.InertText)
		}
	}
}

// A reply holds the links relay listed for every chunk of its message so
// far, each once, in the order they came, on each block it was split
// into. Any other update ends the message, and the next starts with
// none; so does a chunk of another message.
func TestAReplyHoldsTheUnionOfItsChunksLinks(t *testing.T) {
	a := Link{Address: "https://a.example/1", Shown: "hxxps://a[.]example/1"}
	b := Link{Address: "https://b.example/2", Shown: "hxxps://b[.]example/2"}
	c := Link{Address: "https://c.example/3", Shown: "hxxps://c[.]example/3"}
	d := Link{Address: "https://d.example/4", Shown: "hxxps://d[.]example/4"}
	start := []Event{Connected{At: t0}, Submit{At: t0, Text: "q"}}
	first, _ := run(append(start, TextChunk{At: t0, MessageID: "m1", Text: "one ", Links: []Link{a}})...)
	s, _ := run(append(start,
		TextChunk{At: t0, MessageID: "m1", Text: "one ", Links: []Link{a}},
		TextChunk{At: t0, MessageID: "m1", Text: "two ", Links: []Link{b, a}},
		TextChunk{At: t0, MessageID: "m1", Text: "three\n\nfour ", Links: []Link{c}},
		TextChunk{At: t0, MessageID: "m1", Text: "five "},
		ThoughtChunk{At: t0, MessageID: "m1", Text: "hm"},
		TextChunk{At: t0, MessageID: "m1", Text: "six ", Links: []Link{d}},
		TextChunk{At: t0, MessageID: "m2", Text: "seven "},
	)...)
	type got struct {
		Text  string
		Links []Link
	}
	var replies []got
	for _, e := range s.Entries {
		if e.Kind == Agent {
			replies = append(replies, got{e.Text, e.Links})
		}
	}
	want := []got{
		{"one two three\n\n", []Link{a, b, c}},
		{"four five ", []Link{a, b, c}},
		{"six ", []Link{d}},
		{"seven ", nil},
	}
	if !reflect.DeepEqual(replies, want) {
		t.Fatalf("replies:\n%+v\nwant\n%+v", replies, want)
	}
	// What an earlier state held is not changed by the chunks after it.
	for _, e := range first.Entries {
		if e.Kind == Agent && !reflect.DeepEqual(e.Links, []Link{a}) {
			t.Fatalf("an earlier state's reply changed: %+v", e.Links)
		}
	}
}
