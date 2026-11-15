package fakehub

// The board, as relay's hub keeps it for every client on a connection:
// _relay/cards and _relay/cards/op, about no conversation, and
// _relay/cards/changed once the client has asked.
//
// A config with "cards" (wire forms, as relay sends them) has a board; one
// without answers the board's methods method-not-found. An op takes tick
// and untick of a checklist item, archive (refused if "seen" is not the
// card's rev), and input on a surface card, which sets the boolean a
// CheckBox of the surface is bound to, with relay's refusals. Each call
// is logged as "cards <params>" or "card-op <params>". As relay does,
// each refuses a parameter it does not take: the board is the
// connection's, so a sessionId is refused. The control file's
// "archive-card <id>" archives a card and notifies it, and
// "bump-card <id>" moves a card's rev on without telling the client.

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

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

// num is a number of a wire form: a float64 as decoded, or the int64 a
// bump stores.
func num(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int64:
		return float64(x)
	}
	return 0
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
		if err := strict(method, params, "card", "op", "item", "row", "column", "state", "text", "path", "value", "seen"); err != nil {
			return nil, err
		}
		c := b.card(num(params["card"]))
		if c == nil {
			return nil, refused(fmt.Sprintf("There is no card %v on the board.", params["card"]))
		}
		op, _ := params["op"].(string)
		if op == "input" {
			return b.input(c, params)
		}
		if c["status"] != "active" {
			return nil, refused(fmt.Sprintf("There is no card %v on the board.", params["card"]))
		}
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

// input is the op input on card c: it sets the boolean a CheckBox of the
// card's surface is bound to, in the data model of the stored createSurface
// message. The refusals are relay's, with the card's id and revs.
func (b *board) input(c map[string]any, params map[string]any) (json.RawMessage, error) {
	id := params["card"]
	if c["kind"] != "surface" {
		return nil, refused(fmt.Sprintf("input is not an op on a %v.", c["kind"]))
	}
	if c["status"] != "active" {
		return nil, refused(fmt.Sprintf("Card %v is archived. Restore it first.", id))
	}
	seen, ok := params["seen"]
	if !ok {
		return nil, refused("input needs seen: the card's rev when you read it.")
	}
	// Neither refusal says which rev the card is at, as relay's do not:
	// the op cannot be sent again without the card being read.
	if num(seen) < num(c["rev"]) {
		return nil, refused(fmt.Sprintf("Card %v has changed since rev %v. Read the card again before you change it.", id, num(seen)))
	}
	if num(seen) > num(c["rev"]) {
		return nil, refused("seen must be a rev you read. Read the card again for it.")
	}
	create := surfaceMessage(c)
	path, _ := params["path"].(string)
	if create == nil || !boundPath(create, path) {
		return nil, refused(fmt.Sprintf("Nothing on card %v takes input at that path.", id))
	}
	value, ok := params["value"].(bool)
	if !ok {
		return nil, refused("A checkbox takes true or false.")
	}
	data, _ := create["dataModel"].(map[string]any)
	if data == nil {
		data = map[string]any{}
		create["dataModel"] = data
	}
	if now, found := getPointer(data, path); found && now == value {
		return copyOf(map[string]any{"card": c, "changed": false}), nil
	}
	setPointer(data, path, value)
	b.bump(c)
	return copyOf(map[string]any{"card": c, "changed": true}), nil
}

// surfaceMessage is the createSurface member of a surface card's first
// A2UI message, or nil.
func surfaceMessage(c map[string]any) map[string]any {
	sf, _ := c["surface"].(map[string]any)
	msgs, _ := sf["a2ui"].([]any)
	if len(msgs) == 0 {
		return nil
	}
	m, _ := msgs[0].(map[string]any)
	create, _ := m["createSurface"].(map[string]any)
	return create
}

// boundPath says a CheckBox of the createSurface message is bound to path.
func boundPath(create map[string]any, path string) bool {
	comps, _ := create["components"].([]any)
	for _, x := range comps {
		comp, _ := x.(map[string]any)
		if comp["component"] != "CheckBox" {
			continue
		}
		if v, _ := comp["value"].(map[string]any); v != nil && v["@path"] == path {
			return true
		}
	}
	return false
}

// pointerParts splits an absolute pointer into its keys.
func pointerParts(path string) []string {
	return strings.Split(strings.TrimPrefix(path, "/"), "/")
}

// getPointer reads the value at path in a data model.
func getPointer(data any, path string) (any, bool) {
	n := data
	for _, p := range pointerParts(path) {
		switch x := n.(type) {
		case map[string]any:
			v, ok := x[p]
			if !ok {
				return nil, false
			}
			n = v
		case []any:
			i, err := strconv.Atoi(p)
			if err != nil || i < 0 || i >= len(x) {
				return nil, false
			}
			n = x[i]
		default:
			return nil, false
		}
	}
	return n, true
}

// setPointer writes v at path in a data model whose steps exist.
func setPointer(data any, path string, v any) {
	parts := pointerParts(path)
	n := data
	for i, p := range parts {
		last := i == len(parts)-1
		switch x := n.(type) {
		case map[string]any:
			if last {
				x[p] = v
				return
			}
			n = x[p]
		case []any:
			j, err := strconv.Atoi(p)
			if err != nil || j < 0 || j >= len(x) {
				return
			}
			if last {
				x[j] = v
				return
			}
			n = x[j]
		default:
			return
		}
	}
}

// bumpCard moves a card's rev on, as another process changing it would,
// and does not notify the client.
func (h *Hub) bumpCard(id string) {
	n, err := strconv.ParseFloat(id, 64)
	h.mu.Lock()
	defer h.mu.Unlock()
	var c map[string]any
	if h.board != nil && err == nil {
		c = h.board.card(n)
	}
	if c == nil {
		h.logf("control: no card %q", id)
		return
	}
	h.board.bump(c)
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
