package wire

import (
	"encoding/json"
	"slices"
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
	MethodUndo         = "_relay/undo"
	MethodMessagesAct  = "_relay/messages/act"
)

// ActParams is the parameters of _relay/messages/act: the daily note, the
// number of one of its items, the note's own, and what to do with it
// (core.OpRevert, OpDismiss or OpDecline); an empty op leaves it to what
// the item offers.
func ActParams(message int64, item int, op string) map[string]any {
	p := map[string]any{"message": message, "item": item}
	if op != "" {
		p["op"] = op
	}
	return p
}

// ReadyParams is the parameters of _relay/ready: the newest items, as
// many as relay sends by default. The method is the connection's, so it
// names no session.
func ReadyParams() map[string]any { return map[string]any{} }

// ReplyParams is the parameters of _relay/reply: the message answered and
// the operator's text.
func ReplyParams(message int64, text string) map[string]any {
	return map[string]any{"inReplyTo": message, "text": text}
}

// UndoParams is the parameters of _relay/undo: the move to undo, a move
// item's undo.
func UndoParams(move int64) map[string]any { return map[string]any{"move": move} }

type rawReply struct {
	At   string `json:"at"`
	Text string `json:"text"`
}

type rawAct struct {
	N     int     `json:"n"`
	Op    string  `json:"op"`
	Line  int     `json:"line"`
	Done  *string `json:"done"`
	Offer bool    `json:"offer"`
}

type rawSection struct {
	Heading *string  `json:"heading"`
	Lines   []string `json:"lines"`
}

type rawReadyItem struct {
	ID          string       `json:"id"`
	Kind        string       `json:"kind"`
	At          string       `json:"at"`
	Seq         int64        `json:"seq"`
	Title       string       `json:"title"`
	Lines       []string     `json:"lines"`
	From        *string      `json:"from"`
	Outsider    *bool        `json:"outsider"`
	Start       string       `json:"start"`
	End         string       `json:"end"`
	Message     int64        `json:"message"`
	MessageKind string       `json:"messageKind"`
	Urgency     string       `json:"urgency"`
	ReplyTo     *int64       `json:"replyTo"`
	Replies     []rawReply   `json:"replies"`
	Card        int64        `json:"card"`
	Move        int64        `json:"move"`
	Role        string       `json:"role"`
	Undo        *int64       `json:"undo"`
	Undone      bool         `json:"undone"`
	Acts        []rawAct     `json:"acts"`
	Sections    []rawSection `json:"sections"`
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
		Message: r.Message, MessageKind: r.MessageKind, Urgency: r.Urgency, Card: r.Card,
		Seq: r.Seq, Move: r.Move, Role: r.Role, Undone: r.Undone}
	if r.Undo != nil {
		it.Undo = *r.Undo
	}
	if r.From != nil {
		it.From = *r.From
	}
	if r.ReplyTo != nil {
		it.ReplyTo = *r.ReplyTo
	}
	for _, x := range r.Acts {
		a := core.ReadyAct{N: x.N, Op: x.Op, Line: x.Line, Offer: x.Offer}
		if x.Done != nil {
			a.Done = *x.Done
		}
		it.Acts = append(it.Acts, a)
	}
	// Sections stand in for Lines only when they hold the same lines, so
	// an act's Line still points at its own line.
	var flat []string
	var secs []core.ReadySection
	for _, x := range r.Sections {
		sec := core.ReadySection{Lines: x.Lines}
		if x.Heading != nil {
			sec.Heading = *x.Heading
		}
		secs = append(secs, sec)
		flat = append(flat, x.Lines...)
	}
	if len(secs) > 0 && slices.Equal(flat, r.Lines) {
		it.Sections = secs
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
		Rev int64 `json:"rev"`
		// Timezone is the zone name relay shows times in (relay.edn's
		// :timezone). relay does not send it yet; when it does, times
		// follow it.
		Timezone string            `json:"timezone"`
		Items    []json.RawMessage `json:"items"`
	}
	if strictjson.Unmarshal(raw, &r) != nil || r.Items == nil {
		return core.ReadyLoaded{At: at, Err: "relay's answer could not be read"}
	}
	return core.ReadyLoaded{At: at, Rev: r.Rev, Zone: r.Timezone, Items: readyItems(r.Items)}
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

// UndoOutcome translates the answer to _relay/undo on move: the lines
// relay undo prints, or relay's refusal, which says why.
func UndoOutcome(move int64, raw json.RawMessage, err error, at time.Time) core.UndoDone {
	if err != nil {
		return core.UndoDone{At: at, Move: move, Err: errText(err)}
	}
	var r struct {
		Lines   []string `json:"lines"`
		Summary string   `json:"summary"`
	}
	if strictjson.Unmarshal(raw, &r) != nil {
		return core.UndoDone{At: at, Move: move, Err: "relay's answer could not be read"}
	}
	return core.UndoDone{At: at, Move: move, Lines: r.Lines, Summary: r.Summary}
}

// ActOutcome translates the answer to _relay/messages/act on item n of
// message, which did op: a JSON-RPC error's message is relay's refusal.
func ActOutcome(message int64, n int, op string, err error, at time.Time) core.ActDone {
	if err != nil {
		return core.ActDone{At: at, Message: message, N: n, Op: op, Err: errText(err)}
	}
	return core.ActDone{At: at, Message: message, N: n, Op: op}
}
