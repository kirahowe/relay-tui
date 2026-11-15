package fakehub

// relay's live links, as its hub keeps them for every client on a
// connection: _relay/links, then _relay/links/changed once the client has
// asked, and _relay/links/keep and _relay/links/drop, the operator's own.
//
// A config with "links" (addresses, possibly none) has the set; one
// without answers these methods method-not-found, as a relay from before
// them does. Text is sent as the config has it, and relay sends outside
// text with every link broken. Beside it each card and each ready item
// carries "links", the live links its text shows, each {"address",
// "shown"} with its broken form, and "linksLeftOut". Keeping or dropping
// a link changes those lists and nothing in the text; the client is sent
// the set's change first and then each thing whose list changed, as relay
// sends them. Each call is logged as "links", "link-keep <params>" or
// "link-drop <params>". The control file's "keep-link <address>" and
// "drop-link <address>" do the same from outside, as relay link does.

import (
	"context"
	"encoding/json"
	"slices"
	"sort"
	"strings"

	acp "github.com/caelis-labs/acp-go-sdk"

	"github.com/kirahowe/relay-tui/internal/core"
)

type links struct {
	rev        int64
	live       []string
	subscribed bool
}

func newLinks(raw []string) *links {
	if raw == nil {
		return nil
	}
	l := &links{rev: 200, live: slices.Clone(raw)}
	sort.Strings(l.live)
	return l
}

// LinkRefusal is relay's reason for an address a live link cannot be.
const LinkRefusal = "A link is one http or https address in plain ASCII, with no spaces, no brackets and no user name before its host. Write a host in other letters in its xn-- form."

// mend is the address a broken link stood for (relay.text/mend), and s
// itself, trimmed, when it is not one.
func mend(s string) string {
	s = strings.TrimSpace(s)
	for broken, scheme := range map[string]string{"hxxps://": "https://", "hxxp://": "http://"} {
		if rest, ok := strings.CutPrefix(s, broken); ok {
			return scheme + strings.ReplaceAll(rest, "[.]", ".")
		}
	}
	return s
}

// broken is a link as relay shows it broken.
func broken(link string) string {
	scheme, rest, _ := strings.Cut(link, "://")
	return strings.Replace(scheme, "http", "hxxp", 1) + "://" + strings.ReplaceAll(rest, ".", "[.]")
}

// linkMethods are the methods linkMethod answers.
var linkMethods = []string{"_relay/links", "_relay/links/keep", "_relay/links/drop"}

// linkMethod answers one of the live links' methods. The caller holds no
// lock.
func (h *Hub) linkMethod(ctx context.Context, method string, raw json.RawMessage) (json.RawMessage, error) {
	var params map[string]any
	_ = json.Unmarshal(raw, &params)
	// logf takes the lock, so the call is logged before it is taken.
	if method == "_relay/links" {
		h.logf("links")
	} else {
		h.logf("%s %s", map[string]string{"_relay/links/keep": "link-keep", "_relay/links/drop": "link-drop"}[method], raw)
	}
	h.mu.Lock()
	l := h.links
	if l == nil {
		h.mu.Unlock()
		return nil, acp.NewMethodNotFound(method)
	}
	if method == "_relay/links" {
		defer h.mu.Unlock()
		if err := strict(method, params); err != nil {
			return nil, err
		}
		l.subscribed = true
		return copyOf(map[string]any{"rev": l.rev, "live": l.live}), nil
	}
	h.mu.Unlock()
	if err := strict(method, params, "address"); err != nil {
		return nil, err
	}
	address, ok := params["address"].(string)
	if !ok {
		return nil, &acp.RequestError{Code: -32602, Message: method + " needs address, the link as it is shown."}
	}
	changed, err := h.setLink(ctx, address, method == "_relay/links/keep")
	if err != nil {
		return nil, err
	}
	return copyOf(map[string]any{"changed": changed}), nil
}

// setLink keeps or drops a link and tells the client what changed: the
// set first, then each card and ready item whose own list changed.
func (h *Hub) setLink(ctx context.Context, address string, keep bool) (bool, error) {
	link := mend(address)
	if !core.LinkShape(link) || strings.ContainsAny(link, "[]") {
		return false, refused(LinkRefusal)
	}
	h.mu.Lock()
	l, conn := h.links, h.conn
	if l == nil || slices.Contains(l.live, link) == keep {
		h.mu.Unlock()
		return false, nil
	}
	l.rev++
	body := map[string]any{"rev": l.rev, "live": []string{}, "gone": []string{}}
	if keep {
		l.live = append(l.live, link)
		sort.Strings(l.live)
		body["live"] = []string{link}
	} else {
		l.live = slices.DeleteFunc(l.live, func(x string) bool { return x == link })
		body["gone"] = []string{link}
	}
	notify := l.subscribed && conn != nil
	cards, items := h.relink()
	var cardsBody, readyBody json.RawMessage
	if len(cards) > 0 && h.board.subscribed {
		cardsBody = copyOf(map[string]any{"rev": h.board.rev, "cards": cards})
	}
	if len(items) > 0 && h.ready.subscribed {
		h.ready.rev++
		readyBody = copyOf(map[string]any{"rev": h.ready.rev, "items": items, "gone": []string{}})
	}
	linksBody := copyOf(body)
	h.mu.Unlock()
	if conn == nil {
		return true, nil
	}
	if notify {
		_ = conn.NotifyExtension(ctx, "_relay/links/changed", linksBody)
	}
	if cardsBody != nil {
		_ = conn.NotifyExtension(ctx, "_relay/cards/changed", cardsBody)
	}
	if readyBody != nil {
		_ = conn.NotifyExtension(ctx, "_relay/ready/changed", readyBody)
	}
	return true, nil
}

// relink gives every card and ready item its own list of live links, and
// returns the ones whose list changed. The caller holds the lock. With
// no set, or a config that says "unlisted" (each a relay from before the
// list), nothing carries one.
func (h *Hub) relink() (cards, items []map[string]any) {
	if h.links == nil || h.unlisted {
		return nil, nil
	}
	if b := h.board; b != nil {
		for _, c := range b.cards {
			if ownLinks(c, h.links.live) {
				cards = append(cards, c)
			}
		}
	}
	if v := h.ready; v != nil {
		for _, it := range v.items {
			if ownLinks(it, h.links.live) {
				items = append(items, it)
			}
		}
	}
	return cards, items
}

// ownLinks sets a wire form's "links" to the live links whose broken form
// one of its strings holds, and "linksLeftOut" to 0. It says whether the
// list changed.
func ownLinks(v map[string]any, live []string) bool {
	own := []any{}
	for _, link := range live {
		if holds(v, broken(link)) {
			own = append(own, map[string]any{"address": link, "shown": broken(link)})
		}
	}
	old, _ := json.Marshal(v["links"])
	now, _ := json.Marshal(own)
	v["links"], v["linksLeftOut"] = own, 0
	return string(old) != string(now)
}

// holds says some string in a wire form, other than its list of links,
// holds s.
func holds(v any, s string) bool {
	switch x := v.(type) {
	case string:
		return strings.Contains(x, s)
	case map[string]any:
		for k, e := range x {
			if k != "links" && holds(e, s) {
				return true
			}
		}
	case []any:
		for _, e := range x {
			if holds(e, s) {
				return true
			}
		}
	}
	return false
}
