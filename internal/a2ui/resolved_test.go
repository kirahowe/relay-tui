package a2ui

import (
	"fmt"
	"strings"
	"testing"
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
