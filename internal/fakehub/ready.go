package fakehub

// What is ready for the operator, as relay's hub keeps it for every
// client on a connection: _relay/ready, _relay/reply, and
// _relay/ready/changed once the client has asked, and _relay/undo, which
// puts a move back.
//
// A config with "ready" (wire items, as relay sends them) has a view; one
// without answers both methods method-not-found. A reply to a message in
// the view puts the answer on the item, as relay does, and notifies the
// client. Each call is logged as "ready <params>" or "reply <params>", and
// each refuses a parameter it does not take.

import (
	"context"
	"encoding/json"
	"fmt"

	acp "github.com/caelis-labs/acp-go-sdk"
)

type readyView struct {
	rev        int64
	items      []map[string]any // wire forms, as JSON objects
	subscribed bool
}

func newReady(raw []json.RawMessage) (*readyView, error) {
	if raw == nil {
		return nil, nil
	}
	v := &readyView{rev: 100}
	for _, r := range raw {
		var it map[string]any
		if err := json.Unmarshal(r, &it); err != nil {
			return nil, err
		}
		v.items = append(v.items, it)
	}
	return v, nil
}

// readyMethods are the methods readyMethod answers.
var readyMethods = []string{"_relay/ready", "_relay/reply", "_relay/undo"}

// readyMethod answers one of the view's methods. The caller holds no lock.
func (h *Hub) readyMethod(ctx context.Context, method string, raw json.RawMessage) (json.RawMessage, error) {
	var params map[string]any
	_ = json.Unmarshal(raw, &params)
	h.logf("%s %s", map[string]string{"_relay/ready": "ready", "_relay/reply": "reply", "_relay/undo": "undo"}[method], raw)
	h.mu.Lock()
	v, conn := h.ready, h.conn
	if v == nil {
		h.mu.Unlock()
		return nil, acp.NewMethodNotFound(method)
	}
	switch method {
	case "_relay/ready":
		defer h.mu.Unlock()
		if err := strict(method, params, "limit"); err != nil {
			return nil, err
		}
		v.subscribed = true
		return copyOf(map[string]any{"rev": v.rev, "items": v.items}), nil
	case "_relay/reply":
		if err := strict(method, params, "inReplyTo", "text"); err != nil {
			h.mu.Unlock()
			return nil, err
		}
		id, text := num(params["inReplyTo"]), fmt.Sprint(params["text"])
		var item map[string]any
		for _, it := range v.items {
			if num(it["message"]) == id && num(it["replyTo"]) == id {
				item = it
			}
		}
		if item == nil || text == "" {
			h.mu.Unlock()
			return nil, refused(fmt.Sprintf("There is no message %v to answer.", params["inReplyTo"]))
		}
		replies, _ := item["replies"].([]any)
		item["replies"] = append(replies, map[string]any{"at": "2026-10-15T09:30:00Z", "text": text})
		v.rev++
		body := copyOf(map[string]any{"rev": v.rev, "items": []any{item}, "gone": []string{}})
		notify := v.subscribed
		h.mu.Unlock()
		if notify && conn != nil {
			_ = conn.NotifyExtension(ctx, "_relay/ready/changed", body)
		}
		return copyOf(map[string]any{}), nil
	case "_relay/undo":
		if err := strict(method, params, "move"); err != nil {
			h.mu.Unlock()
			return nil, err
		}
		id := num(params["move"])
		var item map[string]any
		for _, it := range v.items {
			if num(it["move"]) == id {
				item = it
			}
		}
		// A move whose undo is null has nothing left to undo, as relay
		// says when it refuses.
		if item == nil || num(item["undo"]) != id {
			h.mu.Unlock()
			return nil, refused(fmt.Sprintf("Move %v was not undone, and nothing changed: there is nothing left to undo.", params["move"]))
		}
		item["undo"], item["undone"] = nil, true
		v.rev++
		body := copyOf(map[string]any{"rev": v.rev, "items": []any{item}, "gone": []string{}})
		notify := v.subscribed
		h.mu.Unlock()
		if notify && conn != nil {
			_ = conn.NotifyExtension(ctx, "_relay/ready/changed", body)
		}
		return copyOf(map[string]any{"lines": []string{fmt.Sprintf("Undid move %v: put back 1 of 1.", params["move"])}}), nil
	}
	h.mu.Unlock()
	return nil, acp.NewMethodNotFound(method)
}
