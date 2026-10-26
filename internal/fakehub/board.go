package fakehub

// The board, as relay's hub keeps it for every client on a connection:
// _relay/cards and _relay/cards/op, about no conversation, and
// _relay/cards/changed once the client has asked.
//
// A config with "cards" (wire forms, as relay sends them) has a board; one
// without answers the board's methods method-not-found. An op takes tick
// and untick of a checklist item, and archive (refused if "seen" is not
// the card's rev). Each call is logged as "cards <params>" or "card-op
// <params>". As relay does, each refuses a parameter it does not take: the board is the
// connection's, so a sessionId is refused. The
// control file's "archive-card <id>" archives a card and notifies it.

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strconv"

	acp "github.com/caelis-labs/acp-go-sdk"
)

type board struct {
	rev        int64
	cards      []map[string]any // wire forms, as JSON objects
	subscribed bool
}

func newBoard(raw []json.RawMessage) (*board, error) {
	if raw == nil {
		return nil, nil
	}
	b := &board{}
	for _, r := range raw {
		var c map[string]any
		if err := json.Unmarshal(r, &c); err != nil {
			return nil, err
		}
		b.cards = append(b.cards, c)
		b.rev = max(b.rev, int64(num(c["rev"])))
	}
	return b, nil
}

func num(v any) float64 {
	f, _ := v.(float64)
	return f
}

func (b *board) card(id float64) map[string]any {
	for _, c := range b.cards {
		if num(c["id"]) == id {
			return c
		}
	}
	return nil
}

func (b *board) active() []map[string]any {
	out := []map[string]any{}
	for _, c := range b.cards {
		if c["status"] == "active" {
			out = append(out, c)
		}
	}
	return out
}

// bump gives card c the board's next rev.
func (b *board) bump(c map[string]any) {
	b.rev++
	c["rev"] = b.rev
}

// strict refuses a parameter the method does not take, as relay does.
func strict(method string, params map[string]any, takes ...string) error {
	var unknown []string
	for k := range params {
		if !slices.Contains(takes, k) {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return &acp.RequestError{Code: -32602, Message: fmt.Sprintf("%s has no parameter %v.", method, unknown)}
	}
	return nil
}

func refused(msg string) error { return &acp.RequestError{Code: -32000, Message: msg} }

// boardMethods are the methods boardMethod answers.
var boardMethods = []string{"_relay/cards", "_relay/cards/op"}

// boardMethod answers one of the board's methods, with a copy of what it
// holds. The caller holds no lock.
func (h *Hub) boardMethod(method string, raw json.RawMessage) (json.RawMessage, error) {
	var params map[string]any
	_ = json.Unmarshal(raw, &params)
	h.logf("%s %s", map[string]string{"_relay/cards": "cards", "_relay/cards/op": "card-op"}[method], raw)
	h.mu.Lock()
	defer h.mu.Unlock()
	b := h.board
	if b == nil {
		return nil, acp.NewMethodNotFound(method)
	}
	switch method {
	case "_relay/cards":
		if err := strict(method, params, "archived"); err != nil {
			return nil, err
		}
		b.subscribed = true
		return copyOf(map[string]any{"rev": b.rev, "cards": b.active()}), nil
	case "_relay/cards/op":
		if err := strict(method, params, "card", "op", "item", "row", "column", "state", "text", "seen"); err != nil {
			return nil, err
		}
		c := b.card(num(params["card"]))
		if c == nil || c["status"] != "active" {
			return nil, refused(fmt.Sprintf("There is no card %v on the board.", params["card"]))
		}
		op, _ := params["op"].(string)
		if op == "archive" {
			if seen, ok := params["seen"]; ok && num(seen) != num(c["rev"]) {
				return nil, refused(fmt.Sprintf("The card %v changed since you saw it.", params["card"]))
			}
			c["status"] = "archived"
			b.bump(c)
			return copyOf(map[string]any{"card": c, "changed": true}), nil
		}
		if op != "tick" && op != "untick" {
			return nil, refused("fakehub takes tick, untick and archive only.")
		}
		items, _ := c["items"].([]any)
		for _, it := range items {
			if m, _ := it.(map[string]any); m != nil && num(m["id"]) == num(params["item"]) {
				m["done"] = op == "tick"
				b.bump(c)
				return copyOf(map[string]any{"card": c, "changed": true}), nil
			}
		}
		return nil, refused(fmt.Sprintf("Item %v is not on card %v.", params["item"], params["card"]))
	default:
		return nil, acp.NewMethodNotFound(method)
	}
}

// archiveCard archives a card, as another process would, and notifies
// the client if it has asked for the board.
func (h *Hub) archiveCard(ctx context.Context, id string) {
	n, err := strconv.ParseFloat(id, 64)
	h.mu.Lock()
	b, conn := h.board, h.conn
	var c map[string]any
	if b != nil && err == nil {
		c = b.card(n)
	}
	if c == nil {
		h.mu.Unlock()
		h.logf("control: no card %q", id)
		return
	}
	c["status"] = "archived"
	b.bump(c)
	body := copyOf(map[string]any{"rev": b.rev, "cards": []any{c}})
	notify := b.subscribed
	h.mu.Unlock()
	if notify && conn != nil {
		_ = conn.NotifyExtension(ctx, "_relay/cards/changed", body)
	}
}

// copyOf is v as JSON, so what is sent is what the board held when it was
// answered.
func copyOf(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
