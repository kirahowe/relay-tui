// Package a2uitest makes surfaces as relay sends them, for tests: a
// generator of random ones, and the JSON of one createSurface message.
package a2uitest

import (
	"encoding/json/v2"
	"fmt"
	"maps"
	"slices"
	"strings"

	"pgregory.net/rapid"

	"github.com/kirahowe/relay-tui/internal/a2ui"
)

// GenResolved draws a surface that passes a2ui.Resolved: a random tree of
// the eight components of relay's catalog, its text and labels from text,
// with CheckBoxes that are literal or bound to a boolean the data model
// holds, and Buttons whose child is a Text and whose action is an event
// or sets such a boolean. Its id is "s".
func GenResolved(t *rapid.T, text *rapid.Generator[string]) a2ui.Surface {
	comps := map[string]a2ui.Component{}
	data := map[string]any{}
	n := 0
	var build func(id string, depth int)
	build = func(id string, depth int) {
		kinds := []string{"Text", "Divider", "CheckBox", "CheckBox", "Button", "Button"}
		if depth < 6 && n < 40 {
			kinds = append(kinds, "Row", "Column", "List", "Card")
		}
		kind := rapid.SampledFrom(kinds).Draw(t, "kind")
		props := map[string]any{}
		child := func() string {
			n++
			k := fmt.Sprintf("c%d", n)
			build(k, depth+1)
			return k
		}
		switch kind {
		case "Text":
			props["text"] = text.Draw(t, "text")
			if rapid.Bool().Draw(t, "hasVariant") {
				props["variant"] = rapid.SampledFrom([]string{"body", "caption"}).Draw(t, "variant")
			}
		case "Divider":
			if rapid.Bool().Draw(t, "hasAxis") {
				props["axis"] = rapid.SampledFrom([]string{"horizontal", "vertical"}).Draw(t, "axis")
			}
		case "CheckBox":
			props["label"] = text.Draw(t, "label")
			if rapid.Bool().Draw(t, "bound") {
				n++
				key := fmt.Sprintf("k%d", n)
				data[key] = rapid.Bool().Draw(t, "on")
				props["value"] = map[string]any{"@path": "/" + key}
			} else {
				props["value"] = rapid.Bool().Draw(t, "literal")
			}
		case "Button":
			n++
			label := fmt.Sprintf("b%d", n)
			comps[label] = a2ui.Component{ID: label, Type: "Text", Props: map[string]any{"text": strings.NewReplacer("\n", " ", "\r", " ").Replace(text.Draw(t, "buttonLabel"))}}
			btn := map[string]any{"child": label}
			if rapid.Bool().Draw(t, "primary") {
				btn["variant"] = "primary"
			}
			if !rapid.Bool().Draw(t, "setsInput") {
				btn["action"] = map[string]any{"event": map[string]any{"name": rapid.StringMatching(`[A-Za-z][A-Za-z0-9_]{0,15}`).Draw(t, "event")}}
				props = btn
				break
			}
			// A button that sets a box sits beside a CheckBox bound to
			// the path it sets, in a Row that takes this component's id.
			key := fmt.Sprintf("k%d", n)
			data[key] = rapid.Bool().Draw(t, "on")
			btn["action"] = map[string]any{"functionCall": map[string]any{"@call": "relay:setInput",
				"args": map[string]any{"path": "/" + key, "value": rapid.Bool().Draw(t, "to")}}}
			btnID, boxID := fmt.Sprintf("bb%d", n), fmt.Sprintf("bx%d", n)
			comps[btnID] = a2ui.Component{ID: btnID, Type: "Button", Props: btn}
			comps[boxID] = a2ui.Component{ID: boxID, Type: "CheckBox", Props: map[string]any{"label": "box", "value": map[string]any{"@path": "/" + key}}}
			kind, props = "Row", map[string]any{"children": []any{btnID, boxID}}
		case "Card":
			props["child"] = child()
		default:
			if kind == "List" && rapid.Bool().Draw(t, "hasDirection") {
				props["direction"] = rapid.SampledFrom([]string{"vertical", "horizontal"}).Draw(t, "direction")
			}
			kids := []any{}
			for range rapid.IntRange(0, 4).Draw(t, "kids") {
				kids = append(kids, child())
			}
			props["children"] = kids
		}
		comps[id] = a2ui.Component{ID: id, Type: kind, Props: props}
	}
	build("root", 0)
	return a2ui.Surface{ID: "s", CatalogID: a2ui.RelayCatalogID, Components: comps, Data: data}
}

// Create is the JSON of the one createSurface message that builds s from
// nothing, with its components and data model inline.
func Create(s a2ui.Surface) []byte {
	ids := slices.Sorted(maps.Keys(s.Components))
	comps := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		c := s.Components[id]
		m := map[string]any{"id": c.ID, "component": c.Type}
		for k, v := range c.Props {
			m[k] = v
		}
		comps = append(comps, m)
	}
	cs := map[string]any{"surfaceId": s.ID, "catalogId": s.CatalogID, "components": comps}
	if s.Data != nil {
		cs["dataModel"] = s.Data
	}
	b, err := json.Marshal(map[string]any{"version": "1.0", "createSurface": cs}, json.Deterministic(true))
	if err != nil {
		panic("a2uitest: " + err.Error())
	}
	return b
}
