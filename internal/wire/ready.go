package wire

import (
	"encoding/json"
	"time"

	"github.com/kirahowe/relay-tui/internal/core"
	"github.com/kirahowe/relay-tui/internal/strictjson"
)

// What is ready for the operator: relay's _relay/ready, read once the
// connection is up and then pushed with _relay/ready/changed, and
// _relay/reply, which answers a message. relay has made every text safe
// before it sends it.
const (
	MethodReady        = "_relay/ready"
	MethodReadyChanged = "_relay/ready/changed"
	MethodReply        = "_relay/reply"
)

// ReadyParams is the parameters of _relay/ready: the newest items, as
// many as relay sends by default. The method is the connection's, so it
// names no session.
func ReadyParams() map[string]any { return map[string]any{} }

// ReplyParams is the parameters of _relay/reply: the message answered and
// the operator's text.
func ReplyParams(message int64, text string) map[string]any {
	return map[string]any{"inReplyTo": message, "text": text}
}

type rawReply struct {
	At   string `json:"at"`
	Text string `json:"text"`
}

type rawReadyItem struct {
	ID          string     `json:"id"`
	Kind        string     `json:"kind"`
	At          string     `json:"at"`
	Title       string     `json:"title"`
	Lines       []string   `json:"lines"`
	From        *string    `json:"from"`
	Outsider    *bool      `json:"outsider"`
	Start       string     `json:"start"`
	End         string     `json:"end"`
	Message     int64      `json:"message"`
	MessageKind string     `json:"messageKind"`
	Urgency     string     `json:"urgency"`
	ReplyTo     *int64     `json:"replyTo"`
	Replies     []rawReply `json:"replies"`
	Card        int64      `json:"card"`
}

// readyItem reads one item. An item without an id is left out. A missing
// outsider flag reads as true, and so does a "from" that is present but
// empty while the flag says outsider: text relay did not vouch for is
// shown as outside text.
func readyItem(raw json.RawMessage) (core.ReadyItem, bool) {
	var r rawReadyItem
	if strictjson.Unmarshal(raw, &r) != nil || r.ID == "" {
		return core.ReadyItem{}, false
	}
	it := core.ReadyItem{ID: r.ID, Kind: r.Kind, At: r.At, Title: r.Title, Lines: r.Lines,
		Outsider: r.Outsider == nil || *r.Outsider, Start: r.Start, End: r.End,
		Message: r.Message, MessageKind: r.MessageKind, Urgency: r.Urgency, Card: r.Card}
	if r.From != nil {
		it.From = *r.From
	}
	if r.ReplyTo != nil {
		it.ReplyTo = *r.ReplyTo
	}
	for _, x := range r.Replies {
		it.Replies = append(it.Replies, core.ReadyReply{At: x.At, Text: x.Text})
	}
	return it, true
}

func readyItems(raw []json.RawMessage) []core.ReadyItem {
	var out []core.ReadyItem
	for _, x := range raw {
		if it, ok := readyItem(x); ok {
			out = append(out, it)
		}
	}
	return out
}

// Ready translates the answer to _relay/ready. A plain agent, or a relay
// from before it, answers method-not-found: there is nothing ready to
// show.
func Ready(raw json.RawMessage, err error, at time.Time) core.ReadyLoaded {
	if err != nil {
		if notFound(err) {
			return core.ReadyLoaded{At: at, Unavailable: true}
		}
		return core.ReadyLoaded{At: at, Err: errText(err)}
	}
	var r struct {
		Rev   int64             `json:"rev"`
		Items []json.RawMessage `json:"items"`
	}
	if strictjson.Unmarshal(raw, &r) != nil || r.Items == nil {
		return core.ReadyLoaded{At: at, Err: "relay's answer could not be read"}
	}
	return core.ReadyLoaded{At: at, Rev: r.Rev, Items: readyItems(r.Items)}
}

// ReadyChanged translates a _relay/ready/changed notification. ok is
// false when it cannot be read.
func ReadyChanged(params json.RawMessage, at time.Time) (core.ReadyChanged, bool) {
	var r struct {
		Rev   int64             `json:"rev"`
		Items []json.RawMessage `json:"items"`
		Gone  []string          `json:"gone"`
	}
	if strictjson.Unmarshal(params, &r) != nil {
		return core.ReadyChanged{}, false
	}
	return core.ReadyChanged{At: at, Rev: r.Rev, Items: readyItems(r.Items), Gone: r.Gone}, true
}

// ReplyOutcome translates the answer to _relay/reply on message: a
// JSON-RPC error's message is relay's refusal.
func ReplyOutcome(message int64, err error, at time.Time) core.ReplyDone {
	if err != nil {
		return core.ReplyDone{At: at, Message: message, Err: errText(err)}
	}
	return core.ReplyDone{At: at, Message: message}
}
