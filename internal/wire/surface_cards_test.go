package wire

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/kirahowe/relay-tui/internal/core"
)

// surfaceCardWire is a card of kind surface with the given surface member,
// in relay's wire form.
func surfaceCardWire(surface string) string {
	s := `{"id": 1840, "kind": "surface", "status": "active", "complete": false, "rev": 1852, "owner": "steward",
 "addedAt": "2026-11-14T15:03:00Z", "title": {"text": "Pack for the lake weekend", "outsider": false, "rev": 1840},
 "labels": ["private"]`
	if surface != "" {
		s += `, "surface": ` + surface
	}
	return s + `, "text": "Card 1840: the text form"}`
}

func createMessage(surfaceID string, components, data string) string {
	return `{"version": "1.0", "createSurface": {"surfaceId": "` + surfaceID + `", "catalogId": "urn:relay:catalog:v1",
 "components": ` + components + `, "dataModel": ` + data + `}}`
}

const goodComponents = `[
 {"id": "root", "component": "Column", "children": ["count", "items"]},
 {"id": "count", "component": "Text", "variant": "caption", "text": "1 of 2 done"},
 {"id": "items", "component": "List", "children": ["item__0", "item__1"]},
 {"id": "item__0", "component": "CheckBox", "label": "Tent and stakes", "value": {"@path": "/items/0/done"}},
 {"id": "item__1", "component": "CheckBox", "label": "Sleeping bags", "value": {"@path": "/items/1/done"}}]`

const goodData = `{"items": [{"done": true}, {"done": false}]}`

func oneCard(t *testing.T, card string) core.Card {
	t.Helper()
	got := Cards(json.RawMessage(`{"rev": 1900, "cards": [`+card+`]}`), nil, t0)
	if got.Err != "" || len(got.Cards) != 1 {
		t.Fatalf("%+v", got)
	}
	return got.Cards[0]
}

func TestTheContractsSurfaceCardIsNative(t *testing.T) {
	c := oneCard(t, surfaceCardWire(`{"outsider": false, "a2ui": [`+createMessage("card_1840", goodComponents, goodData)+`]}`))
	if c.Kind != core.CardSurface || !c.Native() || c.Surface == nil || c.Surface.Outsider || c.Text != "Card 1840: the text form" {
		t.Fatalf("%+v", c)
	}
	ins := c.Inputs()
	if len(ins) != 2 || ins[0].Path != "/items/0/done" || ins[0].Label != "Tent and stakes" || !ins[0].On ||
		ins[1].Path != "/items/1/done" || ins[1].On {
		t.Fatalf("inputs %+v", ins)
	}
}

func TestABrokenSurfaceCardShowsItsText(t *testing.T) {
	update := `{"version": "1.0", "updateDataModel": {"surfaceId": "card_1840", "path": "/items/1/done", "value": true}}`
	many := createMessage("card_1840", goodComponents, goodData) + strings.Repeat(","+update, 64)
	cases := map[string]string{
		"no surface member":  surfaceCardWire(""),
		"empty a2ui":         surfaceCardWire(`{"outsider": false, "a2ui": []}`),
		"no a2ui":            surfaceCardWire(`{"outsider": false}`),
		"does not decode":    surfaceCardWire(`{"outsider": false, "a2ui": [{"version": "1.0"}]}`),
		"does not apply":     surfaceCardWire(`{"outsider": false, "a2ui": [{"version": "1.0", "deleteSurface": {"surfaceId": "card_1840"}}]}`),
		"two surfaces":       surfaceCardWire(`{"outsider": false, "a2ui": [` + createMessage("card_1840", goodComponents, goodData) + `,` + createMessage("card_1841", goodComponents, goodData) + `]}`),
		"wrong surface id":   surfaceCardWire(`{"outsider": false, "a2ui": [` + createMessage("card_7", goodComponents, goodData) + `]}`),
		"unknown component":  surfaceCardWire(`{"outsider": false, "a2ui": [` + createMessage("card_1840", `[{"id": "root", "component": "Image", "url": "https://x.test/a.png"}]`, `{}`) + `]}`),
		"template":           surfaceCardWire(`{"outsider": false, "a2ui": [` + createMessage("card_1840", `[{"id": "root", "component": "Column", "children": {"componentId": "x", "path": "/items"}}]`, `{}`) + `]}`),
		"too many messages":  surfaceCardWire(`{"outsider": false, "a2ui": [` + many + `]}`),
		"basic catalog":      surfaceCardWire(`{"outsider": false, "a2ui": [` + strings.Replace(createMessage("card_1840", goodComponents, goodData), "urn:relay:catalog:v1", "https://a2ui.org/specification/v1_0/catalogs/basic/catalog.json", 1) + `]}`),
		"path to a non-bool": surfaceCardWire(`{"outsider": false, "a2ui": [` + createMessage("card_1840", goodComponents, `{"items": [{"done": "yes"}, {"done": false}]}`) + `]}`),
	}
	for name, card := range cases {
		t.Run(name, func(t *testing.T) {
			c := oneCard(t, card)
			if c.Kind != "surface?" || c.Native() || c.Surface != nil || c.Text != "Card 1840: the text form" {
				t.Fatalf("%+v", c)
			}
		})
	}
}

func TestAMissingOutsiderFlagReadsAsTrue(t *testing.T) {
	c := oneCard(t, surfaceCardWire(`{"a2ui": [`+createMessage("card_1840", goodComponents, goodData)+`]}`))
	if c.Surface == nil || !c.Surface.Outsider {
		t.Fatalf("%+v", c.Surface)
	}
}

func TestSixtyFourMessagesAreTheMost(t *testing.T) {
	create := createMessage("card_1840", goodComponents, goodData)
	update := `{"version": "1.0", "updateDataModel": {"surfaceId": "card_1840", "path": "/items/1/done", "value": true}}`
	msgs := create + strings.Repeat(","+update, 63)
	c := oneCard(t, surfaceCardWire(`{"outsider": false, "a2ui": [`+msgs+`]}`))
	if !c.Native() || !c.Inputs()[1].On {
		t.Fatalf("%+v", c)
	}
}

func TestAnInputOpsParamsAreExactlyTheContractsFive(t *testing.T) {
	path, on, seen := "/items/1/done", true, int64(1852)
	got, _ := json.Marshal(CardOpParams(1840, core.CardOp{Op: "input", Path: &path, Value: &on, Seen: &seen}))
	if string(got) != `{"card":1840,"op":"input","path":"/items/1/done","seen":1852,"value":true}` {
		t.Errorf("%s", got)
	}
	off := false
	got, _ = json.Marshal(CardOpParams(1840, core.CardOp{Op: "input", Path: &path, Value: &off, Seen: &seen}))
	if !strings.Contains(string(got), `"value":false`) {
		t.Errorf("a false value is left out: %s", got)
	}
}

// relay's three fixture cards read as native surface cards through the
// function that reads an answer to _relay/cards, each with the list of
// links relay sends beside it.
func TestRelaysSurfaceFixtureCardsAreNative(t *testing.T) {
	for _, c := range []struct {
		name     string
		inputs   int
		outsider bool
		links    int
	}{{"checklist", 4, false, 0}, {"note", 0, false, 0}, {"hostile", 3, true, 1}} {
		t.Run(c.name, func(t *testing.T) {
			got := oneCard(t, readSurfaceFixture(t, c.name))
			if got.Kind != core.CardSurface || !got.Native() || got.Surface.Outsider != c.outsider || len(got.Inputs()) != c.inputs {
				t.Fatalf("kind %q, outsider %v, %d inputs", got.Kind, got.Surface != nil && got.Surface.Outsider, len(got.Inputs()))
			}
			if !got.Listed || len(got.Links) != c.links || got.LinksLeftOut != 0 {
				t.Fatalf("listed %v, links %+v, %d left out", got.Listed, got.Links, got.LinksLeftOut)
			}
		})
	}
}

func readSurfaceFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "surfaces", name+".card.json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

var hyperlinkRE = regexp.MustCompile(`\x1b\]8;[^;]*;([^\x07\x1b]*)(?:\x07|\x1b\\)`)

// relay's hostile fixture card, read as an answer to _relay/cards and
// drawn on the board, gives the terminal one address to open: the link
// relay lists beside the card, and only while that address is one of the
// connection's live links. The address is never on screen as text.
func TestRelaysHostileSurfaceCardKeepsItsListedLink(t *testing.T) {
	const kept = "https://kept.example/fund"
	raw := readSurfaceFixture(t, "hostile")
	c := oneCard(t, raw)
	if len(c.Links) != 1 || c.Links[0] != (core.Link{Address: kept, Shown: "hxxps://kept[.]example/fund"}) {
		t.Fatalf("links %+v", c.Links)
	}
	s := core.Single{App: core.App{Live: true, PaneFocus: true, Board: core.Board{Asked: true}},
		State: core.State{Phase: core.Idle, Ready: true}}
	s, _ = core.ReduceSingle(s, Cards(json.RawMessage(`{"rev": 1900, "cards": [`+raw+`]}`), nil, t0))
	for name, view := range map[string]core.Single{"the list": s, "the open card": showCard(s, c.ID)} {
		for _, live := range []core.Links{core.LinksOf(kept), {}} {
			view.App.Links = live
			lines := paneRaw(view)
			var targets []string
			for _, l := range lines {
				for _, m := range hyperlinkRE.FindAllStringSubmatch(l, -1) {
					if m[1] != "" {
						targets = append(targets, m[1])
					}
				}
			}
			if live.Empty() && len(targets) != 0 {
				t.Errorf("%s, no live link: hyperlinks %q", name, targets)
			}
			if !live.Empty() && (len(targets) != 1 || targets[0] != kept) {
				t.Errorf("%s: hyperlinks %q, want the one to %s", name, targets, kept)
			}
			text := ansi.Strip(strings.Join(lines, "\n"))
			if !strings.Contains(text, "hxxps://kept[.]example/fund") {
				t.Errorf("%s: the link's broken form is not on screen:\n%s", name, text)
			}
			noLiveLink(t, name, text)
		}
	}
}

// A surface member of the wrong JSON type breaks the surface, not the
// card: the card stays, in its text form.
func TestAWronglyTypedSurfaceMemberKeepsTheCardInItsTextForm(t *testing.T) {
	for name, surface := range map[string]string{
		"a2ui is an object":    `{"outsider": false, "a2ui": {}}`,
		"surface is a number":  `5`,
		"outsider is a string": `{"outsider": "yes", "a2ui": [` + createMessage("card_1840", goodComponents, goodData) + `]}`,
		"a2ui holds a number":  `{"outsider": false, "a2ui": [1]}`,
		"surface is null":      `null`,
	} {
		card := surfaceCardWire(surface)
		t.Run(name, func(t *testing.T) {
			c := oneCard(t, card)
			if c.Kind != "surface?" || c.Surface != nil || c.Text != "Card 1840: the text form" {
				t.Fatalf("answer: %+v", c)
			}
			got, ok := CardsChanged(json.RawMessage(`{"rev": 1950, "cards": [`+card+`]}`), t0)
			if !ok || len(got.Cards) != 1 || got.Cards[0].Kind != "surface?" || got.Cards[0].Surface != nil || got.Cards[0].Text != "Card 1840: the text form" {
				t.Fatalf("notification: %+v", got)
			}
		})
	}
}
