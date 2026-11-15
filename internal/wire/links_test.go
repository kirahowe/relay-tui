package wire

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	acp "github.com/caelis-labs/acp-go-sdk"

	"github.com/kirahowe/relay-tui/internal/core"
)

func TestLinksAnswer(t *testing.T) {
	at := time.Unix(1, 0)
	got := Links(json.RawMessage(`{"rev": 7, "live": ["https://x.example/a", "http://y.example"]}`), nil, at)
	want := core.LinksLoaded{At: at, Rev: 7, Live: []string{"https://x.example/a", "http://y.example"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if got := Links(json.RawMessage(`{"rev": 7, "live": []}`), nil, at); got.Err != "" || got.Unavailable || got.Live == nil {
		t.Fatalf("an empty set: %+v", got)
	}
	// A plain agent, or a relay from before the method: no link is live.
	if got := Links(nil, acp.NewMethodNotFound(MethodLinks), at); !got.Unavailable {
		t.Fatalf("method not found: %+v", got)
	}
	if got := Links(nil, &acp.RequestError{Code: -32000, Message: "relay has not read its links yet. Try again."}, at); got.Err == "" || got.Unavailable {
		t.Fatalf("a refusal: %+v", got)
	}
	// An answer that is not the one relay sends is not taken for a set.
	for _, bad := range []string{`{}`, `{"rev": 7}`, `{"rev": 7, "live": null}`, `{"rev": 7, "live": "https://x.example/a"}`,
		`{"rev": 7, "live": [1]}`, `{"rev": 7, "live": ["a"], "live": ["b"]}`, `[]`, `not json`} {
		if got := Links(json.RawMessage(bad), nil, at); got.Err == "" {
			t.Errorf("%s was taken: %+v", bad, got)
		}
	}
}

func TestLinksChangedNotification(t *testing.T) {
	at := time.Unix(1, 0)
	got, ok := LinksChanged(json.RawMessage(`{"rev": 8, "live": ["https://x.example/b"], "gone": ["https://x.example/a"]}`), at)
	want := core.LinksChanged{At: at, Rev: 8, Live: []string{"https://x.example/b"}, Gone: []string{"https://x.example/a"}}
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v %v, want %+v", got, ok, want)
	}
	for _, bad := range []string{`{"rev": 8, "live": "x"}`, `{"rev": "8"}`, `not json`, `{"rev": 8, "gone": [2]}`} {
		if _, ok := LinksChanged(json.RawMessage(bad), at); ok {
			t.Errorf("%s was taken", bad)
		}
	}
}

// A card and a ready item carry the live links their text shows, each
// with its broken form. An entry the client could not use is left out:
// an address out of a live link's shape, or a shown form that is not a
// broken address on one word.
func TestTheLinksOfACardAndAReadyItem(t *testing.T) {
	at := time.Unix(1, 0)
	const links = `"links": [
		{"address": "https://fund.example.org/apply?round=2027", "shown": "hxxps://fund[.]example[.]org/apply?round=2027"},
		{"address": "javascript:alert(1)", "shown": "hxxps://x[.]example"},
		{"address": "https://x.example/a\u001b]8;;https://evil.test\u0007", "shown": "hxxps://x[.]example/a"},
		{"address": "https://x.example/a", "shown": "https://x.example/a"},
		{"address": "https://x.example/a", "shown": "hxxps://x[.]example/a b"},
		{"address": "https://x.example/a", "shown": "hxxps://x[.]example/a\u001b[31m"},
		{"address": "https://x.example/a", "shown": ""},
		{"address": "http://y.example", "shown": "hxxp://y[.]example"}],
		"linksLeftOut": 3`
	want := []core.Link{
		{Address: "https://fund.example.org/apply?round=2027", Shown: "hxxps://fund[.]example[.]org/apply?round=2027"},
		{Address: "http://y.example", Shown: "hxxp://y[.]example"}}

	cards := Cards(json.RawMessage(`{"rev": 1, "cards": [{"id": 5, "kind": "note", "note": {"text": "x", "outsider": true}, `+links+`}]}`), nil, at)
	if cards.Err != "" || len(cards.Cards) != 1 {
		t.Fatalf("cards: %+v", cards)
	}
	if c := cards.Cards[0]; !reflect.DeepEqual(c.Links, want) || c.LinksLeftOut != 3 || !c.Listed {
		t.Fatalf("a card's links: %+v, %d left out, listed %v", c.Links, c.LinksLeftOut, c.Listed)
	}
	ready := Ready(json.RawMessage(`{"rev": 1, "items": [{"id": "message:4", "kind": "message", "title": "x", `+links+`}]}`), nil, at)
	if ready.Err != "" || len(ready.Items) != 1 {
		t.Fatalf("ready: %+v", ready)
	}
	if it := ready.Items[0]; !reflect.DeepEqual(it.Links, want) || it.LinksLeftOut != 3 || !it.Listed {
		t.Fatalf("a ready item's links: %+v, %d left out, listed %v", it.Links, it.LinksLeftOut, it.Listed)
	}

	// A relay from before the list sends neither key, and the card says
	// so. An empty list is a list.
	old := Cards(json.RawMessage(`{"rev": 1, "cards": [{"id": 5, "kind": "note", "note": {"text": "x"}}]}`), nil, at)
	if c := old.Cards[0]; c.Links != nil || c.LinksLeftOut != 0 || c.Listed {
		t.Fatalf("with no list: %+v", c)
	}
	none := Cards(json.RawMessage(`{"rev": 1, "cards": [{"id": 5, "kind": "note", "note": {"text": "x"}, "links": [], "linksLeftOut": 0}]}`), nil, at)
	if c := none.Cards[0]; len(c.Links) != 0 || !c.Listed {
		t.Fatalf("with an empty list: %+v", c)
	}
	// A list this client cannot read costs the card its links and nothing
	// else: the card is kept, and it counts as listed, so no link is
	// taken from its text either.
	for _, bad := range []string{`"links": {}`, `"links": "x"`, `"links": [{"address": 1}]`, `"links": null`, `"links": [], "linksLeftOut": "3"`, `"links": [], "linksLeftOut": 1.5`} {
		got := Cards(json.RawMessage(`{"rev": 1, "cards": [{"id": 5, "kind": "note", "note": {"text": "x"}, `+bad+`}]}`), nil, at)
		if got.Err != "" || len(got.Cards) != 1 {
			t.Fatalf("%s: the card is gone: %+v", bad, got)
		}
		if c := got.Cards[0]; len(c.Links) != 0 || c.LinksLeftOut != 0 || !c.Listed {
			t.Fatalf("%s: %+v", bad, c)
		}
	}
	// No more than relay's own limit are kept, and a count below zero is
	// none.
	var many []string
	for i := range core.MaxOwnLinks + 20 {
		many = append(many, fmt.Sprintf(`{"address": "https://x.example/%d", "shown": "hxxps://x[.]example/%d"}`, i, i))
	}
	big := Cards(json.RawMessage(`{"rev": 1, "cards": [{"id": 5, "kind": "note", "note": {"text": "x"}, "links": [`+strings.Join(many, ",")+`], "linksLeftOut": -4}]}`), nil, at)
	if c := big.Cards[0]; len(c.Links) != core.MaxOwnLinks || c.LinksLeftOut != 0 {
		t.Fatalf("%d links kept, %d left out", len(c.Links), c.LinksLeftOut)
	}
}
