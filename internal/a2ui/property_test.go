package a2ui

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"encoding/json/v2"

	"pgregory.net/rapid"
)

// genScalar is a JSON scalar, with strings that hold awkward characters.
func genScalar() *rapid.Generator[any] {
	return rapid.OneOf(
		rapid.Map(rapid.String(), func(s string) any { return s }),
		rapid.Map(rapid.Float64Range(-1e6, 1e6), func(f float64) any { return f }),
		rapid.Map(rapid.Bool(), func(b bool) any { return b }),
		rapid.Just[any](nil),
	)
}

// genJSON is a JSON value at most depth levels deep.
func genJSON(depth int) *rapid.Generator[any] {
	if depth <= 0 {
		return genScalar()
	}
	return rapid.OneOf(
		genScalar(),
		rapid.Map(rapid.SliceOfN(rapid.Deferred(func() *rapid.Generator[any] { return genJSON(depth - 1) }), 0, 4), func(x []any) any {
			if x == nil {
				x = []any{}
			}
			return x
		}),
		rapid.Map(rapid.MapOfN(rapid.StringMatching(`[a-c/~]{0,3}`), rapid.Deferred(func() *rapid.Generator[any] { return genJSON(depth - 1) }), 0, 4), func(m map[string]any) any { return m }),
	)
}

var pathPart = rapid.StringMatching(`[a-c0-2~/]{0,3}`)

func genPointer() *rapid.Generator[string] {
	return rapid.Map(rapid.SliceOfN(pathPart, 0, 4), func(parts []string) string {
		var b strings.Builder
		for _, p := range parts {
			b.WriteString("/" + EscapePart(p))
		}
		return b.String()
	})
}

// A write to the data model reads back, a null write removes, and neither
// changes the model it started from.
func TestSetReadsBack(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		root := genJSON(3).Draw(t, "root")
		ptr := genPointer().Draw(t, "ptr")
		v := genJSON(2).Draw(t, "value")
		before, _ := json.Marshal(root, json.Deterministic(true))
		out, err := Set(root, ptr, v)
		after, _ := json.Marshal(root, json.Deterministic(true))
		if string(before) != string(after) {
			t.Fatalf("Set changed its input")
		}
		if err != nil {
			return
		}
		got, ok := Get(out, ptr)
		if v == nil {
			if ptr != "" && ptr != "/" && ok {
				t.Fatalf("null write left %v at %q", got, ptr)
			}
			return
		}
		if !ok || !reflect.DeepEqual(got, v) {
			t.Fatalf("wrote %v at %q, read %v (found %v)", v, ptr, got, ok)
		}
		// Writing the same value again changes nothing.
		again, err := Set(out, ptr, v)
		if err != nil || !reflect.DeepEqual(again, out) {
			t.Fatalf("second write differs: %v %v", again, err)
		}
	})
}

// The same through messages: updateDataModel, then a read.
func TestDataModelMessageReadsBack(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ss := mustApplyQuiet(t, nil, `{"version":"1.0","createSurface":{"surfaceId":"s","dataModel":{"a":{"b":1}}}}`)
		ptr := genPointer().Draw(t, "ptr")
		if ptr == "" {
			ptr = "/k"
		}
		v := genJSON(2).Draw(t, "value")
		if v == nil {
			return
		}
		line, _ := json.Marshal(map[string]any{"version": "1.0", "updateDataModel": map[string]any{"surfaceId": "s", "path": ptr, "value": v}}, json.Deterministic(true))
		m, err := Decode(line)
		if err != nil {
			t.Fatalf("Decode(%s): %v", line, err)
		}
		out, err := Apply(ss, m)
		if err != nil {
			return
		}
		s, _ := out.Get("s")
		if got, ok := Get(s.Data, ptr); ptr != "/" && (!ok || !reflect.DeepEqual(got, v)) {
			t.Fatalf("wrote %v at %q, read %v", v, ptr, got)
		}
	})
}

func mustApplyQuiet(t *rapid.T, ss Surfaces, line string) Surfaces {
	m, err := Decode([]byte(line))
	if err != nil {
		t.Fatalf("Decode(%s): %v", line, err)
	}
	out, err := Apply(ss, m)
	if err != nil {
		t.Fatalf("Apply(%s): %v", line, err)
	}
	return out
}

// fixtureLines are real messages, which mutations start from.
func fixtureLines(t testing.TB) [][]byte {
	var out [][]byte
	for _, f := range []string{"checklist", "tracker", "note"} {
		raw, err := os.ReadFile("testdata/" + f + ".jsonl")
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
			out = append(out, []byte(l))
		}
	}
	return out
}

// No bytes make Decode or Apply panic, whether random or a real message
// with a few bytes changed.
func TestNoInputPanicsTheFold(t *testing.T) {
	lines := fixtureLines(t)
	base, err := Decode(lines[0])
	if err != nil {
		t.Fatal(err)
	}
	seed, _ := Apply(nil, base)
	rapid.Check(t, func(t *rapid.T) {
		var line []byte
		if rapid.Bool().Draw(t, "mutate") {
			line = append([]byte(nil), rapid.SampledFrom(lines).Draw(t, "base")...)
			for range rapid.IntRange(1, 4).Draw(t, "edits") {
				if len(line) == 0 {
					break
				}
				i := rapid.IntRange(0, len(line)-1).Draw(t, "at")
				switch rapid.IntRange(0, 2).Draw(t, "edit") {
				case 0:
					line[i] = rapid.Byte().Draw(t, "byte")
				case 1:
					line = append(line[:i], line[i+1:]...)
				default:
					line = append(line[:i], append([]byte{rapid.Byte().Draw(t, "byte")}, line[i:]...)...)
				}
			}
		} else {
			line = rapid.SliceOf(rapid.Byte()).Draw(t, "bytes")
		}
		m, err := Decode(line)
		if err != nil {
			return
		}
		before := snapshot(seed)
		out, _ := Apply(seed, m)
		if snapshot(seed) != before {
			t.Fatal("Apply changed its input")
		}
		for _, s := range out {
			s.Focusables()
			s.Walk(func(Node) {})
		}
	})
}

var componentTypes = []string{"Text", "Column", "Row", "List", "Card", "Divider", "CheckBox", "Button", "TextField", "ChoicePicker", "Tracker", "Image", "Video", "AudioPlayer", "Icon", "Mystery"}
var bindPaths = []string{"/a", "/items", "/items/0/x", "x", "", "/", "/nope", "items/0"}

// genProp is a property value: a literal, a binding or a function call.
func genProp(ids []string) *rapid.Generator[any] {
	return rapid.OneOf(
		genJSON(1),
		rapid.Map(rapid.SampledFrom(bindPaths), func(p string) any { return map[string]any{"@path": p} }),
		rapid.Map(rapid.String(), func(s string) any {
			return map[string]any{"@call": "formatString", "args": map[string]any{"value": "x ${/a} ${@index()} " + s}}
		}),
		rapid.SampledFrom(ids).AsAny(),
		rapid.Map(rapid.SliceOfN(rapid.SampledFrom(ids), 0, 3), func(x []string) any {
			out := make([]any, len(x))
			for i, s := range x {
				out[i] = s
			}
			return out
		}),
		rapid.Map(rapid.SampledFrom(ids), func(id string) any {
			return map[string]any{"componentId": id, "path": "/items"}
		}),
	)
}

// genComponents is a component list with links between its parts that
// may loop or dangle. The first is the root.
func genComponents(t *rapid.T) []map[string]any {
	n := rapid.IntRange(1, 8).Draw(t, "n")
	ids := []string{"root"}
	for i := 1; i < n; i++ {
		ids = append(ids, "c"+string(rune('a'+i)))
	}
	ids = append(ids, "missing")
	props := []string{"text", "children", "child", "label", "value", "variant", "direction", "axis", "options", "action", "rows", "columns", "cells", "weight", "placeholder", "displayStyle"}
	var out []map[string]any
	for i := 0; i < n; i++ {
		c := map[string]any{"id": ids[i], "component": rapid.SampledFrom(componentTypes).Draw(t, "type")}
		for range rapid.IntRange(0, 5).Draw(t, "props") {
			c[rapid.SampledFrom(props).Draw(t, "prop")] = genProp(ids).Draw(t, "val")
		}
		out = append(out, c)
	}
	return out
}

// GenSurfaceMessages returns well-formed messages that build a surface from
// generated components and data. Both packages' tests use the same shape.
func genMessages(t *rapid.T) []map[string]any {
	catalog := rapid.SampledFrom([]string{BasicCatalogID, RelayCatalogID, "other"}).Draw(t, "catalog")
	data := map[string]any{"a": genJSON(2).Draw(t, "a"), "items": rapid.SliceOfN(genJSON(1), 0, 4).Draw(t, "items")}
	comps := genComponents(t)
	return []map[string]any{
		{"version": "1.0", "createSurface": map[string]any{"surfaceId": "s", "catalogId": catalog, "dataModel": data}},
		{"version": "1.0", "updateComponents": map[string]any{"surfaceId": "s", "components": comps}},
	}
}

// Well-formed messages always decode and apply; focus and activation over
// the result never panic, and Activate leaves its input alone.
func TestWellFormedMessagesFold(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		var ss Surfaces
		for _, m := range genMessages(t) {
			line, err := json.Marshal(m, json.Deterministic(true))
			if err != nil {
				t.Skip()
			}
			dm, err := Decode(line)
			if err != nil {
				t.Fatalf("Decode(%s): %v", line, err)
			}
			if ss, err = Apply(ss, dm); err != nil {
				t.Fatalf("Apply(%s): %v", line, err)
			}
		}
		s, _ := ss.Get("s")
		before := snapshot(ss)
		for _, f := range s.Focusables() {
			Activate(s, f)
		}
		if snapshot(ss) != before {
			t.Fatal("Activate changed its input")
		}
	})
}
