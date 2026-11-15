package a2ui

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

func mustDecode(t *testing.T, line string) Message {
	t.Helper()
	m, err := Decode([]byte(line))
	if err != nil {
		t.Fatalf("Decode(%s): %v", line, err)
	}
	return m
}

func mustApply(t *testing.T, ss Surfaces, line string) Surfaces {
	t.Helper()
	out, err := Apply(ss, mustDecode(t, line))
	if err != nil {
		t.Fatalf("Apply(%s): %v", line, err)
	}
	return out
}

func TestDecodeRejects(t *testing.T) {
	for name, line := range map[string]string{
		"not an object":      `[1]`,
		"no version":         `{"deleteSurface":{"surfaceId":"s"}}`,
		"old version":        `{"version":"0.9","deleteSurface":{"surfaceId":"s"}}`,
		"version a number":   `{"version":1.0,"deleteSurface":{"surfaceId":"s"}}`,
		"no message key":     `{"version":"1.0"}`,
		"two message keys":   `{"version":"1.0","deleteSurface":{"surfaceId":"s"},"createSurface":{"surfaceId":"s"}}`,
		"duplicate key":      `{"version":"1.0","deleteSurface":{"surfaceId":"s","surfaceId":"t"}}`,
		"no surface id":      `{"version":"1.0","deleteSurface":{}}`,
		"empty surface id":   `{"version":"1.0","createSurface":{"surfaceId":""}}`,
		"unknown member":     `{"version":"1.0","deleteSurface":{"surfaceId":"s","extra":1}}`,
		"create empty list":  `{"version":"1.0","createSurface":{"surfaceId":"s","components":[]}}`,
		"create data array":  `{"version":"1.0","createSurface":{"surfaceId":"s","dataModel":[]}}`,
		"update no value":    `{"version":"1.0","updateDataModel":{"surfaceId":"s","path":"/a"}}`,
		"update no comps":    `{"version":"1.0","updateComponents":{"surfaceId":"s","components":[]}}`,
		"component no id":    `{"version":"1.0","updateComponents":{"surfaceId":"s","components":[{"component":"Text"}]}}`,
		"component bad id":   `{"version":"1.0","updateComponents":{"surfaceId":"s","components":[{"id":"a-b","component":"Text"}]}}`,
		"component no type":  `{"version":"1.0","updateComponents":{"surfaceId":"s","components":[{"id":"a"}]}}`,
		"component Surface":  `{"version":"1.0","updateComponents":{"surfaceId":"s","components":[{"id":"a","component":"Surface"}]}}`,
		"component id twice": `{"version":"1.0","updateComponents":{"surfaceId":"s","components":[{"id":"a","component":"Text"},{"id":"a","component":"Text"}]}}`,
		"trailing value":     `{"version":"1.0","deleteSurface":{"surfaceId":"s"}} {}`,
		"bad metadata":       `{"version":"1.0","createSurface":{"surfaceId":"s","metadata":{"other":1}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Decode([]byte(line))
			var e *Error
			if !errors.As(err, &e) {
				t.Fatalf("err = %v, want *Error", err)
			}
		})
	}
}

func TestDecodeNotSupported(t *testing.T) {
	for _, line := range []string{
		`{"version":"1.0","callRendererFunction":{"functionCallId":"x","callFunction":{"@call":"f","catalogId":"c"}}}`,
		`{"version":"1.0","agentFunctionResponse":{"functionCallId":"x","value":1}}`,
		`{"version":"1.0","somethingNew":{}}`,
	} {
		_, err := Decode([]byte(line))
		var ns *NotSupportedError
		if !errors.As(err, &ns) {
			t.Fatalf("%s: err = %v, want *NotSupportedError", line, err)
		}
	}
}

func TestDecodeAccepts(t *testing.T) {
	m := mustDecode(t, `{"version":"1.0","createSurface":{"surfaceId":"s","catalogId":"c","sendDataModel":true,"dataModel":{"a":1},"metadata":{"extensions":{"x":1}},"components":[{"id":"root","component":"Text","text":"hi","@catalogId":"d"}]}}`)
	c := m.Create
	if c == nil || c.SurfaceID != "s" || c.CatalogID != "c" || !c.SendDataModel || len(c.Components) != 1 {
		t.Fatalf("decoded %+v", c)
	}
	if k := c.Components[0]; k.ID != "root" || k.Type != "Text" || k.Props["text"] != "hi" || k.Props["@catalogId"] != "d" {
		t.Fatalf("component %+v", k)
	}
	m = mustDecode(t, `{"version":"1.0","updateDataModel":{"surfaceId":"s","value":null}}`)
	if m.Data == nil || m.Data.Path != "/" || m.Data.Value != nil {
		t.Fatalf("decoded %+v", m.Data)
	}
}

func TestFoldLifecycle(t *testing.T) {
	var ss Surfaces
	ss = mustApply(t, ss, `{"version":"1.0","createSurface":{"surfaceId":"s","catalogId":"`+BasicCatalogID+`","dataModel":{"n":"one"}}}`)
	ss = mustApply(t, ss, `{"version":"1.0","updateComponents":{"surfaceId":"s","components":[{"id":"root","component":"Text","text":{"@path":"/n"}}]}}`)
	ss = mustApply(t, ss, `{"version":"1.0","updateDataModel":{"surfaceId":"s","path":"/n","value":"two"}}`)
	s, ok := ss.Get("s")
	if !ok {
		t.Fatal("no surface")
	}
	root, _ := s.Root()
	if got := s.String(Scope{}, root.Props["text"]); got != "two" {
		t.Fatalf("text = %q", got)
	}
	if _, err := Apply(ss, mustDecode(t, `{"version":"1.0","createSurface":{"surfaceId":"s"}}`)); err == nil {
		t.Fatal("created a surface twice")
	}
	for _, line := range []string{
		`{"version":"1.0","updateComponents":{"surfaceId":"x","components":[{"id":"a","component":"Text"}]}}`,
		`{"version":"1.0","updateDataModel":{"surfaceId":"x","value":1}}`,
		`{"version":"1.0","deleteSurface":{"surfaceId":"x"}}`,
	} {
		_, err := Apply(ss, mustDecode(t, line))
		var e *Error
		if !errors.As(err, &e) || e.Code != CodeNoSurface {
			t.Fatalf("%s: err = %v", line, err)
		}
	}
	ss = mustApply(t, ss, `{"version":"1.0","deleteSurface":{"surfaceId":"s"}}`)
	if len(ss) != 0 {
		t.Fatalf("left %d surfaces", len(ss))
	}
	// Once deleted, the id may be created again.
	mustApply(t, ss, `{"version":"1.0","createSurface":{"surfaceId":"s"}}`)
}

func TestApplyKeepsItsInput(t *testing.T) {
	ss := mustApply(t, nil, `{"version":"1.0","createSurface":{"surfaceId":"s","dataModel":{"a":{"b":[1,2]}},"components":[{"id":"root","component":"Text","text":"x"}]}}`)
	before := snapshot(ss)
	for _, line := range []string{
		`{"version":"1.0","updateDataModel":{"surfaceId":"s","path":"/a/b/0","value":9}}`,
		`{"version":"1.0","updateDataModel":{"surfaceId":"s","path":"/a","value":null}}`,
		`{"version":"1.0","updateComponents":{"surfaceId":"s","components":[{"id":"root","component":"Text","text":"y"}]}}`,
		`{"version":"1.0","deleteSurface":{"surfaceId":"s"}}`,
		`{"version":"1.0","createSurface":{"surfaceId":"t"}}`,
	} {
		m := mustDecode(t, line)
		mb := snapshotMessage(m)
		if _, err := Apply(ss, m); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before, snapshot(ss)) {
			t.Fatalf("%s changed the surfaces", line)
		}
		if !reflect.DeepEqual(mb, snapshotMessage(m)) {
			t.Fatalf("%s changed the message", line)
		}
	}
}

func TestPointer(t *testing.T) {
	root := any(map[string]any{"a": map[string]any{"b": []any{"x", "y"}}, "k/~": 1.0})
	for ptr, want := range map[string]any{"/a/b/1": "y", "/k~1~0": 1.0} {
		if got, ok := Get(root, ptr); !ok || got != want {
			t.Errorf("Get(%q) = %v, %v", ptr, got, ok)
		}
	}
	for _, ptr := range []string{"/nope", "/a/b/2", "/a/b/01", "/a/b/-", "a/b", "/k~2", "/a/b/x/y"} {
		if _, ok := Get(root, ptr); ok {
			t.Errorf("Get(%q) found something", ptr)
		}
	}
	got, err := Set(root, "/a/b/2", "z") // appends
	if err != nil || !reflect.DeepEqual(got.(map[string]any)["a"].(map[string]any)["b"], []any{"x", "y", "z"}) {
		t.Fatalf("append: %v %v", got, err)
	}
	if _, err := Set(root, "/a/b/5", "z"); err == nil {
		t.Fatal("set past the end")
	}
	if _, err := Set(root, "/a/b/0/c", "z"); err == nil {
		t.Fatal("set through a string")
	}
	got, _ = Set(root, "/new/deep/key", true)
	if v, _ := Get(got, "/new/deep/key"); v != true {
		t.Fatal("did not create the path")
	}
	got, _ = Set(root, "/a/b/0", nil) // null removes an element
	if v, _ := Get(got, "/a/b"); !reflect.DeepEqual(v, []any{"y"}) {
		t.Fatalf("removal gave %v", v)
	}
	got, _ = Set(root, "/a", nil)
	if _, ok := Get(got, "/a"); ok {
		t.Fatal("null did not remove the key")
	}
	if got, _ = Set(root, "/", nil); !reflect.DeepEqual(got, map[string]any{}) {
		t.Fatalf("null at the root gave %v", got)
	}
	if got, _ = Set(root, "", map[string]any{"q": 1.0}); !reflect.DeepEqual(got, map[string]any{"q": 1.0}) {
		t.Fatalf("replace at the root gave %v", got)
	}
}

func TestBindings(t *testing.T) {
	ss := mustApply(t, nil, `{"version":"1.0","createSurface":{"surfaceId":"s","catalogId":"`+BasicCatalogID+`","dataModel":{"company":"Acme","staff":[{"name":"Alice","tags":["a","b"]},{"name":"Bob","tags":[]}],"n":3,"on":true}}}`)
	s, _ := ss.Get("s")
	item1 := Scope{Base: "/staff/1", Index: 1, In: true}
	cases := []struct {
		name  string
		scope Scope
		v     any
		want  any
	}{
		{"absolute", Scope{}, map[string]any{"@path": "/company"}, "Acme"},
		{"relative in a template", item1, map[string]any{"@path": "name"}, "Bob"},
		{"absolute in a template", item1, map[string]any{"@path": "/company"}, "Acme"},
		{"relative outside a template", Scope{}, map[string]any{"@path": "name"}, nil},
		{"missing", Scope{}, map[string]any{"@path": "/nope"}, nil},
		{"literal", Scope{}, "plain", "plain"},
		{"index", item1, map[string]any{"@call": "@index", "args": map[string]any{"offset": 1.0}}, 2.0},
		{"index outside", Scope{}, map[string]any{"@call": "@index"}, nil},
		{"unknown function", Scope{}, map[string]any{"@call": "formatDate"}, nil},
		{"nested", Scope{}, []any{map[string]any{"@path": "/n"}, "k"}, []any{3.0, "k"}},
		{"format", item1, map[string]any{"@call": "formatString", "args": map[string]any{"value": "#${@index(offset: 1)} ${name} of ${/company}, \\${x}, ${/n} ${/on} ${/nope}|"}}, "#2 Bob of Acme, ${x}, 3 true |"},
		{"format function", Scope{}, map[string]any{"@call": "formatString", "args": map[string]any{"value": "a ${now()} b ${unclosed"}}, "a  b ${unclosed"},
	}
	for _, c := range cases {
		if got := s.Value(c.scope, c.v); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %#v, want %#v", c.name, got, c.want)
		}
	}
	if got := s.String(Scope{}, map[string]any{"@path": "/staff/0/tags"}); got != `["a","b"]` {
		t.Errorf("array as text: %q", got)
	}
}

const listCreate = `{"version":"1.0","createSurface":{"surfaceId":"s","catalogId":"` + BasicCatalogID + `","dataModel":{"id":"c1","items":[{"t":"a","done":false},{"t":"b","done":true}],"note":"n"}}}`

const listComponents = `{"version":"1.0","updateComponents":{"surfaceId":"s","components":[` +
	`{"id":"root","component":"Column","children":["list","go","fn","field","pick","txt"]},` +
	`{"id":"list","component":"List","children":{"componentId":"row","path":"/items"}},` +
	`{"id":"row","component":"CheckBox","label":{"@path":"t"},"value":{"@path":"done"}},` +
	`{"id":"go","component":"Button","child":"txt","action":{"event":{"name":"go","userMessage":{"@call":"formatString","args":{"value":"went ${/id}"}},"context":{"id":{"@path":"/id"},"n":{"@path":"/note"},"lit":[1,{"@path":"/id"}]}}}},` +
	`{"id":"fn","component":"Button","child":"txt","action":{"functionCall":{"@call":"openUrl","args":{"url":{"@path":"/id"}}}}},` +
	`{"id":"field","component":"TextField","label":"f","value":{"@path":"/note"}},` +
	`{"id":"pick","component":"ChoicePicker","options":[],"value":[]},` +
	`{"id":"txt","component":"Text","text":"go"},` +
	`{"id":"hidden","component":"Button","child":"txt","action":{"event":{"name":"unreached"}}}` +
	`]}}`

func listSurfaces(t *testing.T) Surface {
	t.Helper()
	ss := mustApply(t, mustApply(t, nil, listCreate), listComponents)
	s, ok := ss.Get("s")
	if !ok {
		t.Fatal("no surface")
	}
	return s
}

func TestChildrenAndFocus(t *testing.T) {
	s := listSurfaces(t)
	root, _ := s.Root()
	kids := s.Children(root, Scope{})
	if len(kids) != 6 || kids[0].ID != "list" {
		t.Fatalf("children %v", kids)
	}
	list, _ := s.Components["list"]
	inst := s.Children(list, Scope{})
	if len(inst) != 2 || inst[1].Scope != (Scope{Base: "/items/1", Index: 1, In: true}) {
		t.Fatalf("template instances %v", inst)
	}
	var ids []string
	for _, f := range s.Focusables() {
		ids = append(ids, f.ID)
	}
	// "hidden" is a Button too, but the root does not reach it.
	want := []string{"row", "row", "go", "fn", "field", "pick"}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("focusables %v, want %v", ids, want)
	}
}

func TestActivateCheckBox(t *testing.T) {
	s := listSurfaces(t)
	before := snapshot(Surfaces{s})
	f := s.Focusables()
	out, act, err := Activate(s, f[0])
	if err != nil || act != nil {
		t.Fatalf("act %v err %v", act, err)
	}
	if v, _ := Get(out.Data, "/items/0/done"); v != true {
		t.Fatalf("first item is %v", v)
	}
	if v, _ := Get(out.Data, "/items/1/done"); v != true {
		t.Fatalf("second item changed to %v", v)
	}
	if !reflect.DeepEqual(before, snapshot(Surfaces{s})) {
		t.Fatal("Activate changed its input")
	}
	again, _, _ := Activate(out, f[0])
	if v, _ := Get(again.Data, "/items/0/done"); v != false {
		t.Fatal("did not toggle back")
	}
}

func TestActivateButton(t *testing.T) {
	s := listSurfaces(t)
	f := s.Focusables()
	_, act, err := Activate(s, f[2])
	if err != nil {
		t.Fatal(err)
	}
	want := &Action{Name: "go", SurfaceID: "s", SourceComponentID: "go", UserMessage: "went c1",
		Context: map[string]any{"id": "c1", "n": "n", "lit": []any{1.0, "c1"}}}
	if !reflect.DeepEqual(act, want) {
		t.Fatalf("action %+v, want %+v", act, want)
	}
	_, act, err = Activate(s, f[3])
	if err != nil {
		t.Fatal(err)
	}
	want = &Action{Function: true, Name: "openUrl", SurfaceID: "s", SourceComponentID: "fn", Args: map[string]any{"url": "c1"}}
	if !reflect.DeepEqual(act, want) {
		t.Fatalf("action %+v, want %+v", act, want)
	}
	for _, i := range []int{4, 5} {
		if _, _, err := Activate(s, f[i]); !errors.Is(err, ErrNotActivatable) {
			t.Fatalf("focusable %d: err = %v", i, err)
		}
	}
	if _, _, err := Activate(s, Focus{ID: "nope"}); !errors.Is(err, ErrNotActivatable) {
		t.Fatalf("err = %v", err)
	}
}

func TestCatalogs(t *testing.T) {
	if !catalogs[RelayCatalogID]["Tracker"] || catalogs[BasicCatalogID]["Tracker"] {
		t.Fatal("Tracker belongs to the relay catalog only")
	}
	// The relay catalog leaves out what a client would fetch from a URL the
	// moment it drew it.
	media := map[string]bool{"Image": true, "Video": true, "AudioPlayer": true}
	for _, name := range basicTypes {
		if !catalogs[BasicCatalogID][name] {
			t.Errorf("%s missing from the basic catalog", name)
		}
		if catalogs[RelayCatalogID][name] == media[name] {
			t.Errorf("%s: in the relay catalog is %v, want %v", name, catalogs[RelayCatalogID][name], !media[name])
		}
	}
	relayImage := Surface{CatalogID: RelayCatalogID}
	if relayImage.Kind(Component{Type: "Image"}) != "" {
		t.Fatal("the relay catalog must not define Image")
	}
	s := Surface{CatalogID: BasicCatalogID}
	own := Component{Type: "Tracker", Props: map[string]any{"@catalogId": RelayCatalogID}}
	if s.Kind(own) != "Tracker" || s.Kind(Component{Type: "Tracker"}) != "" || (Surface{}).Kind(Component{Type: "Text"}) != "" {
		t.Fatal("catalog resolution is wrong")
	}
	raw, err := os.ReadFile("relay_catalog.json")
	if err != nil || !strings.Contains(string(raw), `"$id": "`+RelayCatalogID+`"`) {
		t.Fatalf("catalog file: %v", err)
	}
}

func TestTracker(t *testing.T) {
	ss := mustApply(t, nil, `{"version":"1.0","createSurface":{"surfaceId":"s","catalogId":"`+RelayCatalogID+`","dataModel":{"cols":[{"id":"a","label":"A"}],"st":"done"},"components":[
 {"id":"root","component":"Tracker","rows":[{"id":"r1","label":{"@path":"/cols/0/label"}},{"id":"r1","label":"dup"},{"id":"r2","label":"two"}],"columns":{"@path":"/cols"},"cells":[{"row":"r1","column":"a","state":{"@path":"/st"}}]}]}}`)
	s, _ := ss.Get("s")
	root, _ := s.Root()
	got := s.Tracker(root, Scope{})
	want := TrackerData{
		Rows:    []TrackerLine{{"r1", "A"}, {"r2", "two"}},
		Columns: []TrackerLine{{"a", "A"}},
		Cells:   []TrackerCell{{"r1", "a", "done"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
