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
		{"basic only type", with(func(s *Surface) { s.Components["t"] = Component{ID: "t", Type: "TextField", Props: map[string]any{}} }), `"TextField"`},
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
		{"component nothing lists", with(func(s *Surface) {
			s.Components["orphan"] = Component{ID: "orphan", Type: "Text", Props: map[string]any{"text": "x"}}
		}), `"orphan"`},
		{"too many children", with(func(s *Surface) {
			kids := make([]any, 0, MaxItems+1)
			for i := range MaxItems + 1 {
				id := fmt.Sprintf("m%d", i)
				s.Components[id] = Component{ID: id, Type: "Text", Props: map[string]any{"text": "x"}}
				kids = append(kids, id)
			}
			s.Components["row"].Props["children"] = kids
		}), "more than 500 children"},
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
	want := []Input{{ID: "a", Path: "/items/0/done", Label: "Tent", On: true}, {ID: "c", Path: "/items/1/done", Label: "Map"}}
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

const buttonComponents = `[
 {"id":"root","component":"Column","children":["go","set","box"]},
 {"id":"box","component":"CheckBox","label":"Packed","value":{"@path":"/a"}},
 {"id":"go","component":"Button","variant":"primary","child":"t1","action":{"event":{"name":"plan_route"}}},
 {"id":"set","component":"Button","child":"t2","action":{"functionCall":{"@call":"relay:setInput","args":{"path":"/a","value":true}}}},
 {"id":"t1","component":"Text","text":"Plan"},
 {"id":"t2","component":"Text","text":"Pack"}]`

func TestResolvedAcceptsAnEventNameAtRelaysLimits(t *testing.T) {
	for _, name := range []string{"a", "Plan_route2", "a" + strings.Repeat("b", 63)} {
		s := relaySurface(t, buttonComponents, `{"a":false}`)
		s.Components["go"].Props["action"] = map[string]any{"event": map[string]any{"name": name}}
		if err := Resolved(s); err != nil {
			t.Errorf("%q: %v", name, err)
		}
	}
}

func TestResolvedButtons(t *testing.T) {
	data := `{"a":false}`
	if err := Resolved(relaySurface(t, buttonComponents, data)); err != nil {
		t.Fatal(err)
	}
	got := relaySurface(t, buttonComponents, data).Inputs()
	want := []Input{{ID: "go", Label: "Plan", Button: true, Event: "plan_route"}, {ID: "set", Label: "Pack", Button: true}, {ID: "box", Path: "/a", Label: "Packed"}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	set := func(id string, v any) func(*Surface) {
		return func(s *Surface) { s.Components["go"].Props[id] = v }
	}
	fn := func(call string, args map[string]any) map[string]any {
		return map[string]any{"functionCall": map[string]any{"@call": call, "args": args}}
	}
	cases := []struct {
		name string
		edit func(*Surface)
	}{
		{"borderless", set("variant", "borderless")},
		{"variant not a string", set("variant", true)},
		{"no action", func(s *Surface) { delete(s.Components["go"].Props, "action") }},
		{"empty action", set("action", map[string]any{})},
		{"event and function", set("action", map[string]any{"event": map[string]any{"name": "x"}, "functionCall": map[string]any{}})},
		{"event without a name", set("action", map[string]any{"event": map[string]any{}})},
		{"event with an empty name", set("action", map[string]any{"event": map[string]any{"name": ""}})},
		{"event with context", set("action", map[string]any{"event": map[string]any{"name": "x", "context": map[string]any{}}})},
		{"event with userMessage", set("action", map[string]any{"event": map[string]any{"name": "x", "userMessage": "m"}})},
		{"openUrl", set("action", fn("openUrl", map[string]any{"url": "https://example.com"}))},
		{"another function", set("action", fn("formatString", map[string]any{"path": "/a", "value": true}))},
		{"relative path", set("action", fn("relay:setInput", map[string]any{"path": "a", "value": true}))},
		{"path is a binding", set("action", fn("relay:setInput", map[string]any{"path": map[string]any{"@path": "/a"}, "value": true}))},
		{"value is a string", set("action", fn("relay:setInput", map[string]any{"path": "/a", "value": "true"}))},
		{"value missing", set("action", fn("relay:setInput", map[string]any{"path": "/a"}))},
		{"extra argument", set("action", fn("relay:setInput", map[string]any{"path": "/a", "value": true, "x": 1.0}))},
		{"function with returnType", set("action", map[string]any{"functionCall": map[string]any{"@call": "relay:setInput", "returnType": "void", "args": map[string]any{"path": "/a", "value": true}}})},
		{"child is a Button", set("child", "set")},
		{"child is missing", set("child", "ghost")},
		{"child is a list", set("child", []any{"t1"})},
		{"no child", func(s *Surface) { delete(s.Components["go"].Props, "child") }},
		{"extra property", set("weight", 1.0)},
		{"setInput to a path no box is bound to", set("action", fn("relay:setInput", map[string]any{"path": "/b", "value": true}))},
		{"event name starting with a digit", set("action", map[string]any{"event": map[string]any{"name": "1go"}})},
		{"event name with a dash", set("action", map[string]any{"event": map[string]any{"name": "plan-route"}})},
		{"event name with a space", set("action", map[string]any{"event": map[string]any{"name": "plan route"}})},
		{"event name of 65 characters", set("action", map[string]any{"event": map[string]any{"name": "a" + strings.Repeat("b", 64)}})},
		{"label with a line break", func(s *Surface) { s.Components["t1"].Props["text"] = "Plan\nthe route" }},
		{"label with a carriage return", func(s *Surface) { s.Components["t1"].Props["text"] = "Plan\rthe route" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := relaySurface(t, buttonComponents, data)
			c.edit(&s)
			if err := Resolved(s); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}
