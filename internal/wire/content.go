package wire

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/kirahowe/relay-tui/internal/core"
	"github.com/kirahowe/relay-tui/internal/strictjson"
)

// CallContent reads a tool call's content, as JSON, into the pieces a
// review text built here shows, each field apart: a text block, a diff
// (path, old text, new text), a terminal, or an embedded resource (its
// URI, mime type and text, or a blob's size and digest). A text block
// relay's preview marks is read with its claim (core.CallContent.Preview),
// which only the core decides on. Whatever this client does not
// understand, a content variant or a field of one, is named in unshown:
// a review that cannot show all of a request does not allow it. _meta,
// ACP's place for extensions, is not shown; it says nothing the call
// runs on.
func CallContent(raw json.RawMessage) (pieces []core.CallContent, unshown []string) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var items []map[string]json.RawMessage
	if strictjson.Unmarshal(raw, &items) != nil {
		return nil, []string{"the content (not a list of objects)"}
	}
	for i, it := range items {
		where := fmt.Sprintf("content %d", i+1)
		var kind string
		_ = strictjson.Unmarshal(it["type"], &kind)
		switch kind {
		case "content":
			var block map[string]json.RawMessage
			if bad := extra(it, "type", "content", "_meta"); bad != "" || strictjson.Unmarshal(it["content"], &block) != nil {
				unshown = append(unshown, where+" ("+orWhole(bad)+")")
				continue
			}
			p, why := blockOf(block)
			switch {
			case why != "":
				unshown = append(unshown, where+" ("+why+")")
			default:
				pieces = append(pieces, p)
			}
		case "diff":
			var d struct {
				Path    *string `json:"path"`
				OldText *string `json:"oldText"`
				NewText *string `json:"newText"`
			}
			if bad := extra(it, "type", "path", "oldText", "newText", "_meta"); bad != "" || strictjson.Unmarshal(mapJSON(it), &d) != nil ||
				d.Path == nil || d.NewText == nil {
				unshown = append(unshown, where+" (a diff: "+orWhole(bad)+")")
				continue
			}
			pieces = append(pieces, core.CallContent{Kind: core.ContentDiff, Path: *d.Path, OldText: d.OldText, NewText: *d.NewText})
		case "terminal":
			var tm struct {
				ID *string `json:"terminalId"`
			}
			if bad := extra(it, "type", "terminalId", "_meta"); bad != "" || strictjson.Unmarshal(mapJSON(it), &tm) != nil || tm.ID == nil {
				unshown = append(unshown, where+" (a terminal: "+orWhole(bad)+")")
				continue
			}
			pieces = append(pieces, core.CallContent{Kind: core.ContentTerminal, TerminalID: *tm.ID})
		default:
			unshown = append(unshown, where+" (of type "+named(kind)+")")
		}
	}
	return pieces, unshown
}

// blockOf reads a content block: a text, or an embedded resource. It
// says why it cannot show the block, or "".
func blockOf(b map[string]json.RawMessage) (core.CallContent, string) {
	var kind string
	_ = strictjson.Unmarshal(b["type"], &kind)
	switch kind {
	case "text":
		var t struct {
			Text *string `json:"text"`
		}
		if bad := extra(b, "type", "text", "_meta"); bad != "" || strictjson.Unmarshal(mapJSON(b), &t) != nil || t.Text == nil {
			return core.CallContent{}, "a text block: " + orWhole(bad)
		}
		var meta map[string]json.RawMessage
		_ = strictjson.Unmarshal(b["_meta"], &meta)
		c := core.CallContent{Kind: core.ContentText, Text: *t.Text}
		if string(meta[previewKey]) == "true" {
			// relay's preview, as claimed: the core decides (trustRelay).
			c.Preview = true
			var names []string
			if strictjson.Unmarshal(meta[previewLabelsKey], &names) == nil {
				c.PreviewLabels = labelNames(names)
			}
		}
		return c, ""
	case "resource":
		var r map[string]json.RawMessage
		if bad := extra(b, "type", "resource", "_meta"); bad != "" || strictjson.Unmarshal(b["resource"], &r) != nil {
			return core.CallContent{}, "a resource: " + orWhole(bad)
		}
		var res struct {
			URI      *string `json:"uri"`
			MimeType *string `json:"mimeType"`
			Text     *string `json:"text"`
			Blob     *string `json:"blob"`
		}
		bad := extra(r, "uri", "mimeType", "text", "blob", "_meta")
		if bad != "" || strictjson.Unmarshal(mapJSON(r), &res) != nil || res.URI == nil || (res.Text == nil) == (res.Blob == nil) {
			return core.CallContent{}, "a resource: " + orWhole(bad)
		}
		c := core.CallContent{Kind: core.ContentResource, URI: *res.URI}
		if res.MimeType != nil {
			c.MimeType = *res.MimeType
		}
		if res.Text != nil {
			c.Text = *res.Text
			return c, ""
		}
		data, err := base64.StdEncoding.DecodeString(*res.Blob)
		if err != nil {
			return core.CallContent{}, "a resource's blob that is not base64"
		}
		sum := sha256.Sum256(data)
		c.Blob, c.BlobSize, c.BlobSHA256 = true, len(data), hex.EncodeToString(sum[:])
		return c, ""
	}
	return core.CallContent{}, "a block of type " + named(kind)
}

// extra names the members of m that are not among known, or "".
func extra(m map[string]json.RawMessage, known ...string) string {
	var out []string
	for k := range m {
		if !slices.Contains(known, k) {
			out = append(out, k)
		}
	}
	if len(out) == 0 {
		return ""
	}
	sort.Strings(out)
	for i, k := range out {
		out[i] = named(k)
	}
	return "fields " + strings.Join(out, ", ")
}

// named is a name the agent chose, for a reason the review gives: quoted
// in printable ASCII, so nothing the agent wrote reaches the screen as
// itself outside the review's text.
func named(s string) string { return strconv.QuoteToASCII(s) }

func orWhole(bad string) string {
	if bad == "" {
		return "not as ACP has it"
	}
	return bad
}

func mapJSON(m map[string]json.RawMessage) json.RawMessage {
	b, _ := json.Marshal(m)
	return b
}

// ExactContent is the tool call's content in the params of a
// session/request_permission, as received.
func ExactContent(params []byte) json.RawMessage {
	var p struct {
		ToolCall struct {
			Content json.RawMessage `json:"content"`
		} `json:"toolCall"`
	}
	if strictjson.Unmarshal(params, &p) != nil {
		return nil
	}
	return p.ToolCall.Content
}
