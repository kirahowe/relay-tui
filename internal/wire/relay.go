package wire

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	acp "github.com/caelis-labs/acp-go-sdk"

	"github.com/kirahowe/relay-tui/internal/a2ui"
	"github.com/kirahowe/relay-tui/internal/core"
	"github.com/kirahowe/relay-tui/internal/strictjson"
)

// What relay adds to ACP, in _meta and in extension methods. See relay's
// wire contract for the display channel.
const (
	displayKey = "relay/display"

	MethodItems   = "_relay/items"
	MethodResolve = "_relay/resolve"

	methodNotFound = -32601
)

// isRelayBlock says whether a text block is relay's display, which is for
// the operator and never part of a tool's output.
func isRelayBlock(t *acp.ContentBlockText) bool {
	_, display := t.Meta[displayKey]
	return display
}

// Displays reads the displays in a tool call's content: text blocks with
// _meta["relay/display"]. A display that cannot be read still shows, with
// the block's own text, marked private and contaminated: it fails closed.
func Displays(content []acp.ToolCallContent) []core.Display {
	var out []core.Display
	for _, c := range content {
		if c.Content == nil || c.Content.Content.Text == nil {
			continue
		}
		t := c.Content.Content.Text
		raw, ok := t.Meta[displayKey]
		if !ok {
			continue
		}
		var m struct {
			Title   string      `json:"title"`
			Body    string      `json:"body"`
			Labels  []string    `json:"labels"`
			Sources []rawSource `json:"sources"`
			Ref     string      `json:"ref"`
		}
		if err := strictjson.Unmarshal(raw, &m); err != nil {
			out = append(out, core.Display{Title: "(unreadable display)", Body: t.Text,
				Labels: core.Labels{Private: true, Contaminated: true}})
			continue
		}
		d := core.Display{Title: m.Title, Body: m.Body, Labels: labelNames(m.Labels), Ref: m.Ref}
		for _, src := range m.Sources {
			d.Sources = append(d.Sources, core.Source{Title: src.Title, Ref: src.Ref})
		}
		out = append(out, d)
	}
	return out
}

// ItemsParams and ResolveParams are the parameters of relay's
// extension methods.
func ItemsParams(session acp.SessionId) map[string]any {
	return map[string]any{"sessionId": session}
}

func ResolveParams(session acp.SessionId, ref string) map[string]any {
	return map[string]any{"sessionId": session, "ref": ref}
}

// notFound says whether err is the agent saying it has no such method.
func notFound(err error) bool {
	var re *acp.RequestError
	return errors.As(err, &re) && re.Code == methodNotFound
}

// errText is an error's message for the operator, with the agent's own
// data message when it adds something, as Response does.
func errText(err error) string {
	var re *acp.RequestError
	if !errors.As(err, &re) {
		return err.Error()
	}
	msg := re.Message
	var data struct {
		Message string `json:"message"`
	}
	if b, e := json.Marshal(re.Data); e == nil && strictjson.Unmarshal(b, &data) == nil &&
		data.Message != "" && !strings.Contains(msg, data.Message) {
		msg += ": " + data.Message
	}
	return msg
}

type rawSource struct {
	Title string `json:"title"`
	Ref   string `json:"ref"`
}

type rawItem struct {
	Kind    string      `json:"kind"`
	Ref     string      `json:"ref"`
	Path    string      `json:"path"`
	CallID  string      `json:"callId"`
	At      string      `json:"at"`
	Title   string      `json:"title"`
	Text    string      `json:"text"`
	Sources []rawSource `json:"sources"`
	Labels  []string    `json:"labels"`
}

// Items translates the answer to _relay/items. A plain agent answers
// method-not-found, which says relay is not there.
func Items(raw json.RawMessage, err error, at time.Time) core.ItemsLoaded {
	if err != nil {
		return core.ItemsLoaded{At: at, Unavailable: notFound(err), Failed: !notFound(err)}
	}
	var r struct {
		Items []rawItem `json:"items"`
	}
	if strictjson.Unmarshal(raw, &r) != nil {
		return core.ItemsLoaded{At: at, Failed: true}
	}
	e := core.ItemsLoaded{At: at}
	for _, it := range r.Items {
		if it.Kind != "shown" {
			continue // a kind this client does not show
		}
		out := core.Item{Ref: it.Ref, Path: it.Path, CallID: it.CallID, At: it.At, Title: it.Title, Text: it.Text,
			Labels: labelNames(it.Labels)}
		for _, src := range it.Sources {
			out.Sources = append(out.Sources, core.Source{Title: src.Title, Ref: src.Ref})
		}
		e.Items = append(e.Items, out)
	}
	return e
}

// Resolution translates the answer to _relay/resolve for ref. Anything
// but a clear answer leaves the citation unchecked.
func Resolution(ref string, raw json.RawMessage, err error, at time.Time) core.CitationResolved {
	failed := core.CitationResolved{At: at, Cite: core.Citation{Ref: ref}, Failed: true}
	if err != nil {
		return failed
	}
	var r struct {
		Kind   *string  `json:"kind"`
		Status string   `json:"status"`
		Title  string   `json:"title"`
		Text   string   `json:"text"`
		Target string   `json:"target"`
		Labels []string `json:"labels"`
	}
	if strictjson.Unmarshal(raw, &r) != nil || r.Kind == nil {
		return failed
	}
	switch r.Status {
	case core.CiteOK, core.CiteMissing, core.CiteExternal:
	default:
		return failed
	}
	return core.CitationResolved{At: at, Cite: core.Citation{Ref: ref, Kind: *r.Kind, Status: r.Status,
		Title: r.Title, Text: r.Text, Target: r.Target, Labels: labelNames(r.Labels)}}
}

// The board: relay's cards, read with _relay/cards, changed with
// _relay/cards/op, and pushed with _relay/cards/changed once the client
// has read them. relay makes every text safe before it sends it.
const (
	MethodCards        = "_relay/cards"
	MethodCardOp       = "_relay/cards/op"
	MethodCardsChanged = "_relay/cards/changed"
)

// CardsParams is the parameters of _relay/cards: the active cards only.
// The board's methods are the connection's, so none names a session;
// relay refuses a sessionId on them.
func CardsParams() map[string]any { return map[string]any{} }

// CardOpParams is the parameters of _relay/cards/op: the card and the
// op's fields, leaving out the ones not set.
func CardOpParams(card int64, op core.CardOp) map[string]any {
	m := map[string]any{"card": card, "op": op.Op}
	if op.Item != nil {
		m["item"] = *op.Item
	}
	if op.Row != nil {
		m["row"] = *op.Row
	}
	if op.Column != nil {
		m["column"] = *op.Column
	}
	if op.State != nil {
		m["state"] = *op.State
	}
	if op.Text != nil {
		m["text"] = *op.Text
	}
	if op.Path != nil {
		m["path"] = *op.Path
	}
	if op.Value != nil {
		m["value"] = *op.Value
	}
	if op.Seen != nil {
		m["seen"] = *op.Seen
	}
	return m
}

// rawText is a piece of a card's text. A missing outsider flag reads as
// true: text relay did not vouch for is shown as outside text.
type rawText struct {
	ID       *int   `json:"id"`
	Text     string `json:"text"`
	Outsider *bool  `json:"outsider"`
	Rev      int64  `json:"rev"`
	Done     bool   `json:"done"`
}

func (t rawText) text() core.CardText {
	return core.CardText{Text: t.Text, Outsider: t.Outsider == nil || *t.Outsider, Rev: t.Rev}
}

type rawCard struct {
	ID       *int64   `json:"id"`
	Kind     string   `json:"kind"`
	Status   string   `json:"status"`
	Complete bool     `json:"complete"`
	Rev      int64    `json:"rev"`
	Owner    string   `json:"owner"`
	AddedAt  string   `json:"addedAt"`
	Title    *rawText `json:"title"`
	Progress *struct {
		Done  int `json:"done"`
		Total int `json:"total"`
	} `json:"progress"`
	Labels  []string  `json:"labels"`
	Items   []rawText `json:"items"`
	Rows    []rawText `json:"rows"`
	Columns []rawText `json:"columns"`
	Cells   []struct {
		Row    int    `json:"row"`
		Column int    `json:"column"`
		State  string `json:"state"`
	} `json:"cells"`
	Note    *rawText        `json:"note"`
	Surface json.RawMessage `json:"surface"` // read apart, so a wrong type breaks the surface and not the card
	Text    string          `json:"text"`
	rawLinks
}

// rawLinks is the live links relay lists beside a card's or a ready
// item's text. Both members are read apart from the thing that holds
// them, so a list this client cannot read costs the thing its links and
// nothing else.
type rawLinks struct {
	Links        json.RawMessage `json:"links"`
	LinksLeftOut json.RawMessage `json:"linksLeftOut"`
}

// links reads the list. An entry is kept only when its address has a
// live link's shape and its shown form is a broken address with no space
// or control character in it: nothing else can be written into a
// hyperlink or found in a word. No more than relay's own limit are kept.
// A list that cannot be read is no links.
func (r rawLinks) links() (own []core.Link, leftOut int) {
	var raw []struct {
		Address string `json:"address"`
		Shown   string `json:"shown"`
	}
	if r.Links == nil || strictjson.Unmarshal(r.Links, &raw) != nil {
		return nil, 0
	}
	for _, l := range raw {
		if len(own) == core.MaxOwnLinks {
			break
		}
		if !core.LinkShape(l.Address) || !strings.HasPrefix(l.Shown, "hxxp") || len(l.Shown) > 3*core.MaxLink ||
			strings.ContainsFunc(l.Shown, func(c rune) bool { return c <= ' ' || c > 0x7e }) {
			continue
		}
		own = append(own, core.Link{Address: l.Address, Shown: l.Shown})
	}
	if r.LinksLeftOut != nil && strictjson.Unmarshal(r.LinksLeftOut, &leftOut) != nil || leftOut < 0 {
		leftOut = 0
	}
	return own, leftOut
}

// maxSurfaceMessages is the most A2UI messages one surface card may hold.
const maxSurfaceMessages = 64

// readSurface reads the surface member of a surface card. Any failure,
// of the JSON or of the surface, leaves the card to its text form. A
// missing outsider flag reads as true, as it does for a text.
func readSurface(id int64, raw json.RawMessage) (core.RoleSurface, bool) {
	var m struct {
		Outsider *bool             `json:"outsider"`
		A2UI     []json.RawMessage `json:"a2ui"`
	}
	if raw == nil || strictjson.Unmarshal(raw, &m) != nil {
		return core.RoleSurface{}, false
	}
	surface, ok := roleSurface(id, m.A2UI)
	if !ok {
		return core.RoleSurface{}, false
	}
	return core.RoleSurface{Outsider: m.Outsider == nil || *m.Outsider, A2UI: surface}, true
}

// roleSurface folds the messages of a surface card from nothing. The
// surface is good when there are between one and maxSurfaceMessages
// messages, each decodes and applies, exactly one surface results, its id
// is "card_" and the card's id, and it is a surface as relay sends one.
func roleSurface(id int64, msgs []json.RawMessage) (a2ui.Surface, bool) {
	if len(msgs) < 1 || len(msgs) > maxSurfaceMessages {
		return a2ui.Surface{}, false
	}
	var ss a2ui.Surfaces
	for _, raw := range msgs {
		m, err := a2ui.Decode(raw)
		if err != nil {
			return a2ui.Surface{}, false
		}
		if ss, err = a2ui.Apply(ss, m); err != nil {
			return a2ui.Surface{}, false
		}
	}
	if len(ss) != 1 || ss[0].ID != "card_"+strconv.FormatInt(id, 10) || a2ui.Resolved(ss[0]) != nil {
		return a2ui.Surface{}, false
	}
	return ss[0], true
}

func elems(raw []rawText) ([]core.CardElem, bool) {
	var out []core.CardElem
	for _, t := range raw {
		if t.ID == nil {
			return nil, false
		}
		out = append(out, core.CardElem{ID: *t.ID, CardText: t.text(), Done: t.Done})
	}
	return out, true
}

// card reads one card in relay's wire form. A card without an id, or a
// native kind whose elements cannot be read, is left out; a card of a
// kind this client does not know keeps its text form.
func card(raw json.RawMessage) (core.Card, bool) {
	var r rawCard
	if strictjson.Unmarshal(raw, &r) != nil || r.ID == nil {
		return core.Card{}, false
	}
	c := core.Card{ID: *r.ID, Kind: r.Kind, Status: r.Status, Complete: r.Complete, Rev: r.Rev, Owner: r.Owner,
		AddedAt: r.AddedAt, Labels: labelNames(r.Labels), Text: r.Text}
	if c.Status == "" {
		c.Status = core.CardActive
	}
	c.Links, c.LinksLeftOut = r.links()
	if r.Title != nil {
		c.Title = r.Title.text()
	}
	if r.Progress != nil {
		c.Done, c.Total = r.Progress.Done, r.Progress.Total
	}
	var ok bool
	switch c.Kind {
	case core.CardChecklist:
		if c.Items, ok = elems(r.Items); !ok {
			c.Kind = "checklist?" // shows its text form
		}
	case core.CardTracker:
		rows, ok1 := elems(r.Rows)
		cols, ok2 := elems(r.Columns)
		if !ok1 || !ok2 {
			c.Kind = "tracker?"
			break
		}
		c.Rows, c.Columns = rows, cols
		for _, x := range r.Cells {
			if x.State != "" && x.State != core.CellOpen {
				c.Cells = append(c.Cells, core.Cell{Row: x.Row, Column: x.Column, State: x.State})
			}
		}
	case core.CardNote:
		if r.Note == nil {
			c.Kind = "note?"
			break
		}
		n := r.Note.text()
		c.Note = &n
	case core.CardSurface:
		role, ok := readSurface(c.ID, r.Surface)
		if !ok {
			c.Kind = "surface?"
			break
		}
		c.Surface = &role
	}
	return c, true
}

func cardList(raw []json.RawMessage) []core.Card {
	var out []core.Card
	for _, x := range raw {
		if c, ok := card(x); ok {
			out = append(out, c)
		}
	}
	return out
}

// Cards translates the answer to _relay/cards. A plain agent answers
// method-not-found: it has no board.
func Cards(raw json.RawMessage, err error, at time.Time) core.CardsLoaded {
	if err != nil {
		if notFound(err) {
			return core.CardsLoaded{At: at, Unavailable: true}
		}
		return core.CardsLoaded{At: at, Err: errText(err)}
	}
	var r struct {
		Rev   int64             `json:"rev"`
		Cards []json.RawMessage `json:"cards"`
	}
	if strictjson.Unmarshal(raw, &r) != nil || r.Cards == nil {
		return core.CardsLoaded{At: at, Err: "relay's answer could not be read"}
	}
	return core.CardsLoaded{At: at, Rev: r.Rev, Cards: cardList(r.Cards)}
}

// CardOpOutcome translates the answer to _relay/cards/op on card id. A
// JSON-RPC error's message is relay's refusal, and "changed": false says
// the op changed nothing.
func CardOpOutcome(id int64, raw json.RawMessage, err error, at time.Time) core.CardOpDone {
	if err != nil {
		return core.CardOpDone{At: at, CardID: id, Err: errText(err)}
	}
	var r struct {
		Card    json.RawMessage `json:"card"`
		Changed *bool           `json:"changed"`
	}
	if strictjson.Unmarshal(raw, &r) != nil || r.Card == nil {
		return core.CardOpDone{At: at, CardID: id, Err: "relay's answer could not be read"}
	}
	c, ok := card(r.Card)
	if !ok || c.ID != id {
		return core.CardOpDone{At: at, CardID: id, Err: "relay's answer could not be read"}
	}
	// An answer without the field is taken as a change.
	return core.CardOpDone{At: at, CardID: id, Card: &c, Unchanged: r.Changed != nil && !*r.Changed}
}

// CardsChanged translates a _relay/cards/changed notification. ok is
// false when it cannot be read.
func CardsChanged(params json.RawMessage, at time.Time) (core.CardsChanged, bool) {
	var r struct {
		Rev   int64             `json:"rev"`
		Cards []json.RawMessage `json:"cards"`
	}
	if strictjson.Unmarshal(params, &r) != nil {
		return core.CardsChanged{}, false
	}
	return core.CardsChanged{At: at, Rev: r.Rev, Cards: cardList(r.Cards)}, true
}
