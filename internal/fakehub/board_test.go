package fakehub

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

const surfaceCardJSON = `{"id": 1840, "kind": "surface", "status": "active", "rev": 1852, "owner": "steward",
 "title": {"text": "Pack", "outsider": false, "rev": 1840},
 "surface": {"outsider": false, "a2ui": [{"version": "1.0", "createSurface": {
  "surfaceId": "card_1840", "catalogId": "urn:relay:catalog:v1",
  "components": [
   {"id": "root", "component": "Column", "children": ["a", "b", "c"]},
   {"id": "a", "component": "CheckBox", "label": "Tent", "value": {"@path": "/items/0/done"}},
   {"id": "b", "component": "CheckBox", "label": "Bags", "value": {"@path": "/items/1/done"}},
   {"id": "c", "component": "CheckBox", "label": "Fixed", "value": true}],
  "dataModel": {"items": [{"done": true}, {"done": false}]}}}]},
 "text": "Card 1840"}`

const checklistCardJSON = `{"id": 7, "kind": "checklist", "status": "active", "rev": 5,
 "title": {"text": "List", "outsider": false, "rev": 5}, "items": [{"id": 1, "text": "x", "done": false, "outsider": false, "rev": 5}]}`

const archivedSurfaceJSON = `{"id": 1841, "kind": "surface", "status": "archived", "rev": 1860, "owner": "steward",
 "title": {"text": "Old", "outsider": false, "rev": 1841},
 "surface": {"outsider": false, "a2ui": [{"version": "1.0", "createSurface": {
  "surfaceId": "card_1841", "catalogId": "urn:relay:catalog:v1",
  "components": [{"id": "root", "component": "CheckBox", "label": "Tent", "value": {"@path": "/done"}}],
  "dataModel": {"done": false}}}]}, "text": "Card 1841"}`

func opHub(t *testing.T) (*Hub, func(params map[string]any) (map[string]any, error)) {
	t.Helper()
	h := New(Config{Cards: []json.RawMessage{json.RawMessage(surfaceCardJSON), json.RawMessage(checklistCardJSON), json.RawMessage(archivedSurfaceJSON)}}, io.Discard)
	conn, _ := connectHub(t, h)
	return h, func(params map[string]any) (map[string]any, error) {
		return callLinks(t, conn, "_relay/cards/op", params)
	}
}

// done is whether the surface card's box at index i is ticked in the data
// model the hub holds.
func done(t *testing.T, h *Hub, i int) bool {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	v, ok := getPointer(surfaceMessage(h.board.card(1840))["dataModel"], "/items/"+string(rune('0'+i))+"/done")
	if !ok {
		t.Fatalf("no value for box %d", i)
	}
	return v.(bool)
}

func TestAnInputSetsTheBoxAndBumpsTheRev(t *testing.T) {
	h, op := opHub(t)
	got, err := op(map[string]any{"card": 1840, "op": "input", "path": "/items/1/done", "value": true, "seen": 1852})
	if err != nil || got["changed"] != true {
		t.Fatalf("%v %v", got, err)
	}
	card := got["card"].(map[string]any)
	if card["rev"].(float64) != 1861 || !done(t, h, 1) || !done(t, h, 0) {
		t.Fatalf("rev %v, boxes %v %v", card["rev"], done(t, h, 0), done(t, h, 1))
	}
	// The card in the answer holds the value in its data model.
	b, _ := json.Marshal(card)
	if !strings.Contains(string(b), `"dataModel":{"items":[{"done":true},{"done":true}]}`) {
		t.Fatalf("answer %s", b)
	}
	// The same value again changes nothing and keeps the rev.
	got, err = op(map[string]any{"card": 1840, "op": "input", "path": "/items/1/done", "value": true, "seen": 1861})
	if err != nil || got["changed"] != false || got["card"].(map[string]any)["rev"].(float64) != 1861 {
		t.Fatalf("%v %v", got, err)
	}
}

func TestAnInputIsRefusedAsRelayRefusesIt(t *testing.T) {
	h, op := opHub(t)
	good := func() map[string]any {
		return map[string]any{"card": 1840, "op": "input", "path": "/items/1/done", "value": true, "seen": 1852}
	}
	without := func(k string) map[string]any { p := good(); delete(p, k); return p }
	with := func(k string, v any) map[string]any { p := good(); p[k] = v; return p }
	cases := []struct {
		name   string
		params map[string]any
		want   string
	}{
		{"no seen", without("seen"), "input needs seen: the card's rev when you read it."},
		{"stale seen", with("seen", 1800), "Card 1840 has changed since rev 1800. Read the card again before you change it."},
		{"seen past the rev", with("seen", 1900), "seen must be a rev you read. Read the card again for it."},
		{"unbound path", with("path", "/items/0/text"), "Nothing on card 1840 takes input at that path."},
		{"no path", without("path"), "Nothing on card 1840 takes input at that path."},
		{"a literal box has no path", with("path", ""), "Nothing on card 1840 takes input at that path."},
		{"not a boolean", with("value", "true"), "A checkbox takes true or false."},
		{"no value", without("value"), "A checkbox takes true or false."},
		{"another kind", map[string]any{"card": 7, "op": "input", "path": "/x", "value": true, "seen": 5}, "input is not an op on a checklist."},
		{"archived", map[string]any{"card": 1841, "op": "input", "path": "/done", "value": true, "seen": 1860}, "Card 1841 is archived. Restore it first."},
		{"no such card", with("card", 99), "There is no card 99 on the board."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := op(c.params)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %v, want %q", err, c.want)
			}
		})
	}
	if done(t, h, 1) {
		t.Fatal("a refused input changed the box")
	}
	if _, err := op(with("item", 1)); err != nil {
		t.Fatalf("item is a parameter the op takes: %v", err)
	}
	if _, err := op(with("iten", 1)); err == nil {
		t.Fatal("a typo was not refused")
	}
}

func TestBumpCardMovesTheRevWithoutTellingTheClient(t *testing.T) {
	h, op := opHub(t)
	h.control(context.Background(), "bump-card 1840")
	_, err := op(map[string]any{"card": 1840, "op": "input", "path": "/items/1/done", "value": true, "seen": 1852})
	if err == nil || !strings.Contains(err.Error(), "Card 1840 has changed since rev 1852.") {
		t.Fatalf("error %v", err)
	}
	if done(t, h, 1) {
		t.Fatal("a stale input changed the box")
	}
}

// buttonsCard is the shared fixture of a card with buttons.
func buttonsCard(t *testing.T) json.RawMessage {
	t.Helper()
	b, err := os.ReadFile("../../testdata/surfaces/buttons.card.json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func buttonHub(t *testing.T) (*Hub, func(params map[string]any) (map[string]any, error)) {
	t.Helper()
	h := New(Config{Cards: []json.RawMessage{buttonsCard(t), json.RawMessage(archivedSurfaceJSON)}}, io.Discard)
	conn, _ := connectHub(t, h)
	return h, func(params map[string]any) (map[string]any, error) {
		return callLinks(t, conn, "_relay/cards/op", params)
	}
}

func TestAPressOfASetInputButtonActsAsThatBoxsInput(t *testing.T) {
	h, op := buttonHub(t)
	got, err := op(map[string]any{"card": 1840, "op": "press", "component": "done__0", "seen": 1840})
	if err != nil || got["changed"] != true || got["card"].(map[string]any)["rev"].(float64) != 1861 {
		t.Fatalf("%v %v", got, err)
	}
	h.mu.Lock()
	v, _ := getPointer(surfaceMessage(h.board.card(1840))["dataModel"], "/items/0/done")
	h.mu.Unlock()
	if v != true {
		t.Fatalf("box is %v", v)
	}
	// The box already holds the value: nothing changes.
	got, err = op(map[string]any{"card": 1840, "op": "press", "component": "done__1", "seen": 1861})
	if err != nil || got["changed"] != false || got["card"].(map[string]any)["rev"].(float64) != 1861 {
		t.Fatalf("%v %v", got, err)
	}
}

func TestAPressOfAnEventButtonChangesNothingAndIsRefusedTwice(t *testing.T) {
	h, op := buttonHub(t)
	press := map[string]any{"card": 1840, "op": "press", "component": "remove__1", "seen": 1840}
	got, err := op(press)
	if err != nil || got["changed"] != true || got["card"].(map[string]any)["rev"].(float64) != 1840 {
		t.Fatalf("%v %v", got, err)
	}
	if _, err = op(press); err == nil || !strings.Contains(err.Error(), "steward has not yet taken up your last press of that button.") {
		t.Fatalf("error %v", err)
	}
	// Another button of the card is not held.
	if _, err = op(map[string]any{"card": 1840, "op": "press", "component": "go", "seen": 1840}); err != nil {
		t.Fatal(err)
	}
	h.control(context.Background(), "take-up 1840")
	if _, err = op(press); err != nil {
		t.Fatalf("after take-up: %v", err)
	}
}

func TestAPressIsRefusedAsRelayRefusesIt(t *testing.T) {
	_, op := buttonHub(t)
	good := func() map[string]any {
		return map[string]any{"card": 1840, "op": "press", "component": "go", "seen": 1840}
	}
	without := func(k string) map[string]any { p := good(); delete(p, k); return p }
	with := func(k string, v any) map[string]any { p := good(); p[k] = v; return p }
	cases := []struct {
		name   string
		params map[string]any
		want   string
	}{
		{"no seen", without("seen"), "press needs seen: the card's rev when you read it."},
		{"stale seen", with("seen", 1800), "Card 1840 has changed since rev 1800. Read the card again before you change it."},
		{"seen past the rev", with("seen", 1900), "seen must be a rev you read. Read the card again for it."},
		{"no such button", with("component", "ghost"), "Nothing on card 1840 is a button with that id."},
		{"a box is no button", with("component", "box__0"), "Nothing on card 1840 is a button with that id."},
		{"no component", without("component"), "Nothing on card 1840 is a button with that id."},
		{"archived", map[string]any{"card": 1841, "op": "press", "component": "go", "seen": 1860}, "Card 1841 is archived. Restore it first."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := op(c.params)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %v, want %q", err, c.want)
			}
		})
	}
}
