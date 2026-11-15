package a2ui_test

import (
	"reflect"
	"testing"

	"pgregory.net/rapid"

	"github.com/kirahowe/relay-tui/internal/a2ui"
	"github.com/kirahowe/relay-tui/internal/a2ui/a2uitest"
)

// Every generated surface passes, and its Inputs are exactly the bound
// CheckBoxes, each reading a boolean.
func TestResolvedAcceptsEverySurfaceRelayCouldSend(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := a2uitest.GenResolved(t, rapid.String())
		if err := a2ui.Resolved(s); err != nil {
			t.Fatal(err)
		}
		for _, in := range s.Inputs() {
			v, ok := a2ui.Get(s.Data, in.Path)
			if b, isBool := v.(bool); !ok || !isBool || b != in.On {
				t.Fatalf("input %+v reads %v", in, v)
			}
		}
	})
}

// wrapRoot is s with a new root Column over the old root and the extra
// components, which are named by id in the order given.
func wrapRoot(s a2ui.Surface, extra ...a2ui.Component) a2ui.Surface {
	comps := map[string]a2ui.Component{}
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
	comps["root"] = a2ui.Component{ID: "root", Type: "Column", Props: map[string]any{"children": kids}}
	s.Components = comps
	return s
}

// One change from the list below makes any such surface fail the check.
func TestResolvedRejectsEveryMutationOfASurfaceRelayCouldSend(t *testing.T) {
	data := map[string]any{"n": "not a boolean", "ok": true}
	text := func(props map[string]any) a2ui.Component { return a2ui.Component{ID: "x", Type: "Text", Props: props} }
	mutations := map[string]func(s a2ui.Surface) a2ui.Surface{
		"unknown type": func(s a2ui.Surface) a2ui.Surface {
			return wrapRoot(s, a2ui.Component{ID: "x", Type: "Image", Props: map[string]any{}})
		},
		"extra property": func(s a2ui.Surface) a2ui.Surface {
			return wrapRoot(s, text(map[string]any{"text": "a", "extra": 1.0}))
		},
		"catalog id": func(s a2ui.Surface) a2ui.Surface {
			return wrapRoot(s, text(map[string]any{"text": "a", "@catalogId": a2ui.RelayCatalogID}))
		},
		"template children": func(s a2ui.Surface) a2ui.Surface {
			return wrapRoot(s, a2ui.Component{ID: "x", Type: "Row", Props: map[string]any{"children": map[string]any{"componentId": "root", "path": "/items"}}})
		},
		"missing child": func(s a2ui.Surface) a2ui.Surface {
			return wrapRoot(s, a2ui.Component{ID: "x", Type: "Card", Props: map[string]any{"child": "ghost"}})
		},
		"call in text": func(s a2ui.Surface) a2ui.Surface {
			return wrapRoot(s, text(map[string]any{"text": map[string]any{"@call": "formatString", "args": map[string]any{"value": "a"}}}))
		},
		"relative path": func(s a2ui.Surface) a2ui.Surface {
			return wrapRoot(s, a2ui.Component{ID: "x", Type: "CheckBox", Props: map[string]any{"label": "a", "value": map[string]any{"@path": "ok"}}})
		},
		"path to a non-boolean": func(s a2ui.Surface) a2ui.Surface {
			return wrapRoot(s, a2ui.Component{ID: "x", Type: "CheckBox", Props: map[string]any{"label": "a", "value": map[string]any{"@path": "/n"}}})
		},
		"child listed twice": func(s a2ui.Surface) a2ui.Surface {
			return wrapRoot(s, a2ui.Component{ID: "x", Type: "Row", Props: map[string]any{"children": []any{"old_root", "old_root"}}})
		},
		"two parents list one child": func(s a2ui.Surface) a2ui.Surface {
			return wrapRoot(s, a2ui.Component{ID: "x", Type: "Card", Props: map[string]any{"child": "old_root"}})
		},
		"cycle": func(s a2ui.Surface) a2ui.Surface {
			return wrapRoot(s, a2ui.Component{ID: "x", Type: "Card", Props: map[string]any{"child": "x"}})
		},
	}
	rapid.Check(t, func(t *rapid.T) {
		s := a2uitest.GenResolved(t, rapid.String())
		for k, v := range data {
			s.Data.(map[string]any)[k] = v
		}
		name := rapid.SampledFrom([]string{"unknown type", "extra property", "catalog id", "template children", "missing child", "call in text", "relative path", "path to a non-boolean", "cycle", "child listed twice", "two parents list one child"}).Draw(t, "mutation")
		bad := mutations[name](s)
		if err := a2ui.Resolved(bad); err == nil {
			t.Fatalf("%s: accepted", name)
		}
	})
}

// The message a2uitest.Create writes folds back into the surface it was
// written from.
func TestCreateFoldsBackToTheSurface(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := a2uitest.GenResolved(t, rapid.String())
		m, err := a2ui.Decode(a2uitest.Create(s))
		if err != nil {
			t.Fatal(err)
		}
		ss, err := a2ui.Apply(nil, m)
		if err != nil || len(ss) != 1 {
			t.Fatalf("%v, %d surfaces", err, len(ss))
		}
		if !reflect.DeepEqual(ss[0], s) {
			t.Fatalf("got\n%+v\nwant\n%+v", ss[0], s)
		}
	})
}
