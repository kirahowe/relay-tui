package a2ui

import (
	"fmt"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// relaySurface folds a surface in the relay catalog from a component list
// and a data model, both JSON.
func relaySurface(t *testing.T, components, data string) Surface {
	t.Helper()
	ss := mustApply(t, nil, `{"version":"1.0","createSurface":{"surfaceId":"s","catalogId":"`+RelayCatalogID+`","dataModel":`+data+`,"components":`+components+`}}`)
	return ss[0]
}

const goodComponents = `[
 {"id":"root","component":"Column","children":["note","row","card","rule","list"]},
 {"id":"note","component":"Text","variant":"caption","text":"2 of 3 done"},
 {"id":"row","component":"Row","children":["a","b"]},
 {"id":"a","component":"CheckBox","label":"Tent","value":{"@path":"/items/0/done"}},
 {"id":"b","component":"CheckBox","label":"Stakes","value":true},
 {"id":"card","component":"Card","child":"t"},
 {"id":"t","component":"Text","text":"hello"},
 {"id":"rule","component":"Divider","axis":"horizontal"},
 {"id":"list","component":"List","direction":"horizontal","children":["c"]},
 {"id":"c","component":"CheckBox","label":"Map","value":{"@path":"/items/1/done"}}]`

const goodData = `{"items":[{"done":true},{"done":false}]}`

func TestResolvedAcceptsASurfaceAsRelaySendsIt(t *testing.T) {
	if err := Resolved(relaySurface(t, goodComponents, goodData)); err != nil {
		t.Fatal(err)
	}
}

func TestResolvedRejects(t *testing.T) {
	good := relaySurface(t, goodComponents, goodData)
	with := func(edit func(*Surface)) Surface {
		s := relaySurface(t, goodComponents, goodData)
		edit(&s)
		return s
	}
	prop := func(id, key string, v any) func(*Surface) {
		return func(s *Surface) { s.Components[id].Props[key] = v }
	}
	cases := []struct {
		name string
		s    Surface
		want string
	}{
		{"basic catalog", func() Surface { s := good; s.CatalogID = BasicCatalogID; return s }(), "catalog"},
		{"no catalog", Surface{Components: good.Components, Data: good.Data}, "catalog"},
		{"no root", func() Surface {
			s := relaySurface(t, `[{"id":"x","component":"Text","text":"a"}]`, `{}`)
			return s
		}(), "root"},
		{"unknown type", with(func(s *Surface) { s.Components["t"] = Component{ID: "t", Type: "Image", Props: map[string]any{}} }), `"Image"`},
		{"basic only type", with(func(s *Surface) { s.Components["t"] = Component{ID: "t", Type: "Button", Props: map[string]any{}} }), `"Button"`},
		{"extra property", with(prop("t", "weight", 1.0)), `"weight"`},
		{"catalog id", with(prop("t", "@catalogId", RelayCatalogID)), `"@catalogId"`},
		{"at property", with(prop("t", "@call", "x")), `"@call"`},
		{"property of another type", with(prop("rule", "text", "x")), `"text"`},
		{"template children", with(prop("row", "children", map[string]any{"componentId": "a", "path": "/items"})), "children"},
		{"child not an id", with(prop("row", "children", []any{"a", 1.0})), "child"},
		{"children missing", with(func(s *Surface) { delete(s.Components["row"].Props, "children") }), "children"},
		{"card child list", with(prop("card", "child", []any{"t"})), "child"},
		{"missing child", with(prop("row", "children", []any{"a", "ghost"})), `"ghost"`},
		{"cycle", with(prop("card", "child", "root")), "more than once"},
		{"child listed twice", with(prop("row", "children", []any{"a", "a"})), `"a"`},
		{"two parents list one child", with(prop("list", "children", []any{"a"})), `"a"`},
		{"too deep", with(func(s *Surface) {
			prev := "t"
			for i := range MaxDepth + 2 {
				id := fmt.Sprintf("d%d", i)
				s.Components[id] = Component{ID: id, Type: "Card", Props: map[string]any{"child": prev}}
				prev = id
			}
			s.Components["card"].Props["child"] = prev
		}), "deeper"},
		{"text is a call", with(prop("t", "text", map[string]any{"@call": "formatString", "args": map[string]any{"value": "x"}})), "text"},
		{"text is a path", with(prop("t", "text", map[string]any{"@path": "/a"})), "text"},
		{"text is a number", with(prop("t", "text", 1.0)), "text"},
		{"variant", with(prop("note", "variant", "h1")), "variant"},
		{"variant not a string", with(prop("note", "variant", true)), "variant"},
		{"direction", with(prop("list", "direction", "diagonal")), "direction"},
		{"axis", with(prop("rule", "axis", "x")), "axis"},
		{"label is a path", with(prop("a", "label", map[string]any{"@path": "/x"})), "label"},
		{"label missing", with(func(s *Surface) { delete(s.Components["a"].Props, "label") }), "label"},
		{"value is a string", with(prop("b", "value", "true")), "value"},
		{"value is a call", with(prop("b", "value", map[string]any{"@call": "x"})), "value"},
		{"value path and more", with(prop("a", "value", map[string]any{"@path": "/items/0/done", "x": 1.0})), "value"},
		{"relative path", with(prop("a", "value", map[string]any{"@path": "items/0/done"})), "absolute"},
		{"path is not a string", with(prop("a", "value", map[string]any{"@path": 3.0})), "value"},
		{"path to nothing", with(prop("a", "value", map[string]any{"@path": "/items/7/done"})), "lacks"},
		{"path to a string", with(func(s *Surface) {
			s.Data = map[string]any{"items": []any{map[string]any{"done": "yes"}, map[string]any{"done": false}}}
		}), "not a boolean"},
		{"too many nodes", with(func(s *Surface) {
			kids := make([]any, 0, MaxNodes)
			for i := range MaxNodes {
				id := fmt.Sprintf("n%d", i)
				s.Components[id] = Component{ID: id, Type: "Text", Props: map[string]any{"text": "x"}}
				kids = append(kids, id)
			}
			s.Components["row"].Props["children"] = kids
		}), "more than"},
	}
	if err := Resolved(good); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Resolved(c.s)
			if err == nil {
				t.Fatal("accepted")
			}
			if _, ok := err.(*Error); !ok {
				t.Fatalf("error is %T", err)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q lacks %q", err, c.want)
			}
		})
	}
}

// A string from the surface is quoted in the error, so it cannot carry a
// control character into whatever prints the error.
func TestResolvedQuotesWhatItQuotes(t *testing.T) {
	s := relaySurface(t, `[{"id":"root","component":"Column","children":["x\u001b[31m"]}]`, `{}`)
	err := Resolved(s)
	if err == nil || strings.ContainsAny(err.Error(), "\x1b") || !strings.Contains(err.Error(), `\u001b`) {
		t.Fatalf("error %q", err)
	}
}

func TestInputs(t *testing.T) {
	got := relaySurface(t, goodComponents, goodData).Inputs()
	want := []Input{{"a", "/items/0/done", "Tent", true}, {"c", "/items/1/done", "Map", false}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// genRelaySurface builds a surface relay could send: a random tree of the
// seven types with literal and bound CheckBoxes and the data they read.
func genRelaySurface(t *rapid.T) Surface {
	comps := map[string]Component{}
	data := map[string]any{}
	n := 0
	next := func() string { n++; return fmt.Sprintf("c%d", n) }
	var build func(id string, depth int)
	build = func(id string, depth int) {
		kinds := []string{"Text", "Divider", "CheckBox"}
		if depth < 5 && n < 40 {
			kinds = append(kinds, "Row", "Column", "List", "Card")
		}
		kind := rapid.SampledFrom(kinds).Draw(t, "kind")
		props := map[string]any{}
		switch kind {
		case "Text":
			props["text"] = rapid.String().Draw(t, "text")
			if rapid.Bool().Draw(t, "hasVariant") {
				props["variant"] = rapid.SampledFrom(textVariants).Draw(t, "variant")
			}
		case "Divider":
			if rapid.Bool().Draw(t, "hasAxis") {
				props["axis"] = rapid.SampledFrom(dividerAxes).Draw(t, "axis")
			}
		case "CheckBox":
			props["label"] = rapid.String().Draw(t, "label")
			if rapid.Bool().Draw(t, "bound") {
				key := fmt.Sprintf("k%d", n)
				data[key] = rapid.Bool().Draw(t, "on")
				props["value"] = map[string]any{"@path": "/" + key}
			} else {
				props["value"] = rapid.Bool().Draw(t, "literal")
			}
		case "Card":
			child := next()
			props["child"] = child
			build(child, depth+1)
		default:
			if kind == "List" && rapid.Bool().Draw(t, "hasDirection") {
				props["direction"] = rapid.SampledFrom(listDirections).Draw(t, "direction")
			}
			kids := []any{}
			for range rapid.IntRange(0, 4).Draw(t, "kids") {
				child := next()
				kids = append(kids, child)
				build(child, depth+1)
			}
			props["children"] = kids
		}
		comps[id] = Component{ID: id, Type: kind, Props: props}
	}
	build("root", 0)
	return Surface{ID: "s", CatalogID: RelayCatalogID, Components: comps, Data: data}
}

// Every generated surface passes, and its Inputs are exactly the bound
// CheckBoxes, each reading a boolean.
func TestResolvedAcceptsEverySurfaceRelayCouldSend(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := genRelaySurface(t)
		if err := Resolved(s); err != nil {
			t.Fatal(err)
		}
		for _, in := range s.Inputs() {
			v, ok := Get(s.Data, in.Path)
			if b, isBool := v.(bool); !ok || !isBool || b != in.On {
				t.Fatalf("input %+v reads %v", in, v)
			}
		}
	})
}

// wrapRoot is s with a new root Column over the old root and the extra
// components, which are named by id in the order given.
func wrapRoot(s Surface, extra ...Component) Surface {
	comps := map[string]Component{}
	for id, c := range s.Components {
		comps[id] = c
	}
	old := comps["root"]
	old.ID = "old_root"
	comps["old_root"] = old
	kids := []any{"old_root"}
	for _, c := range extra {
		comps[c.ID] = c
		kids = append(kids, c.ID)
	}
	comps["root"] = Component{ID: "root", Type: "Column", Props: map[string]any{"children": kids}}
	s.Components = comps
	return s
}

// One change from the list below makes any such surface fail the check.
func TestResolvedRejectsEveryMutationOfASurfaceRelayCouldSend(t *testing.T) {
	data := map[string]any{"n": "not a boolean", "ok": true}
	text := func(props map[string]any) Component { return Component{ID: "x", Type: "Text", Props: props} }
	mutations := map[string]func(s Surface) Surface{
		"unknown type": func(s Surface) Surface {
			return wrapRoot(s, Component{ID: "x", Type: "Image", Props: map[string]any{}})
		},
		"extra property": func(s Surface) Surface {
			return wrapRoot(s, text(map[string]any{"text": "a", "extra": 1.0}))
		},
		"catalog id": func(s Surface) Surface {
			return wrapRoot(s, text(map[string]any{"text": "a", "@catalogId": RelayCatalogID}))
		},
		"template children": func(s Surface) Surface {
			return wrapRoot(s, Component{ID: "x", Type: "Row", Props: map[string]any{"children": map[string]any{"componentId": "root", "path": "/items"}}})
		},
		"missing child": func(s Surface) Surface {
			return wrapRoot(s, Component{ID: "x", Type: "Card", Props: map[string]any{"child": "ghost"}})
		},
		"call in text": func(s Surface) Surface {
			return wrapRoot(s, text(map[string]any{"text": map[string]any{"@call": "formatString", "args": map[string]any{"value": "a"}}}))
		},
		"relative path": func(s Surface) Surface {
			return wrapRoot(s, Component{ID: "x", Type: "CheckBox", Props: map[string]any{"label": "a", "value": map[string]any{"@path": "ok"}}})
		},
		"path to a non-boolean": func(s Surface) Surface {
			return wrapRoot(s, Component{ID: "x", Type: "CheckBox", Props: map[string]any{"label": "a", "value": map[string]any{"@path": "/n"}}})
		},
		"child listed twice": func(s Surface) Surface {
			return wrapRoot(s, Component{ID: "x", Type: "Row", Props: map[string]any{"children": []any{"old_root", "old_root"}}})
		},
		"two parents list one child": func(s Surface) Surface {
			return wrapRoot(s, Component{ID: "x", Type: "Card", Props: map[string]any{"child": "old_root"}})
		},
		"cycle": func(s Surface) Surface {
			return wrapRoot(s, Component{ID: "x", Type: "Card", Props: map[string]any{"child": "x"}})
		},
	}
	rapid.Check(t, func(t *rapid.T) {
		s := genRelaySurface(t)
		for k, v := range data {
			s.Data.(map[string]any)[k] = v
		}
		name := rapid.SampledFrom([]string{"unknown type", "extra property", "catalog id", "template children", "missing child", "call in text", "relative path", "path to a non-boolean", "cycle", "child listed twice", "two parents list one child"}).Draw(t, "mutation")
		bad := mutations[name](s)
		if err := Resolved(bad); err == nil {
			t.Fatalf("%s: accepted", name)
		}
	})
}

// A type the catalog gains but this client does not draw fails, instead of
// being checked as some other type.
func TestResolvedRefusesATypeTheClientDoesNotDraw(t *testing.T) {
	relayProps["Gizmo"] = map[string]bool{"component": true, "text": true}
	defer delete(relayProps, "Gizmo")
	s := relaySurface(t, `[{"id":"root","component":"Column","children":["g"]},{"id":"g","component":"Text","text":"x"}]`, `{}`)
	s.Components["g"] = Component{ID: "g", Type: "Gizmo", Props: map[string]any{"text": "x"}}
	err := Resolved(s)
	if err == nil || !strings.Contains(err.Error(), "does not draw") || !strings.Contains(err.Error(), `"Gizmo"`) {
		t.Fatalf("error %v", err)
	}
}
