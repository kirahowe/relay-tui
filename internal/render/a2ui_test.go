package render

import (
	"encoding/json/v2"
	"regexp"
	"strings"
	"testing"
	"unicode"

	"pgregory.net/rapid"

	"github.com/kirahowe/relay-tui/internal/a2ui"
)

// hostileText is text a surface might carry to reach the terminal.
var hostileText = []string{
	"\x1b[38;5;196mred\x1b[0m", "\x1b]8;;http://evil.test\x07link\x1b]8;;\x07", "\x1b[2J\x1b[H",
	"bell\x07", "a\rb", "tab\there", "‮gnp.exe", "a​b", "\x9b31m", "\x00nul", "\x7f",
	"https://evil.test/?d=secret", "www.evil.test/x", "世界宽字符", "\U0001F600 emoji", "é́́",
	"line one\nline two", "# heading", "**bold** and `code`", "[a link](https://evil.test/x)",
}

func genSurfaceText() *rapid.Generator[string] {
	return rapid.Map(rapid.SliceOfN(rapid.OneOf(rapid.SampledFrom(hostileText), rapid.StringMatching(`[a-z ]{0,10}`), rapid.String()), 0, 4),
		func(p []string) string { return strings.Join(p, " ") })
}

var surfaceTypes = []string{"Text", "Column", "Row", "List", "Card", "Divider", "CheckBox", "Button", "TextField", "ChoicePicker", "Tracker", "Image", "Video", "AudioPlayer", "Mystery\x1b[31m"}

func genSurfaceValue(ids []string, depth int) *rapid.Generator[any] {
	str := rapid.Map(genSurfaceText(), func(s string) any { return s })
	opts := []*rapid.Generator[any]{
		str,
		rapid.Map(rapid.SampledFrom([]string{"/a", "/items", "/items/0/x", "x", "", "/nope"}), func(p string) any { return map[string]any{"@path": p} }),
		rapid.Map(genSurfaceText(), func(s string) any {
			return map[string]any{"@call": "formatString", "args": map[string]any{"value": "n ${/a} ${x} ${@index(offset: 1)} " + s}}
		}),
		rapid.Map(rapid.Bool(), func(b bool) any { return b }),
		rapid.Map(rapid.Float64Range(-5, 5), func(f float64) any { return f }),
	}
	if depth > 0 {
		inner := rapid.Deferred(func() *rapid.Generator[any] { return genSurfaceValue(ids, depth-1) })
		opts = append(opts,
			rapid.Map(rapid.SliceOfN(inner, 0, 4), func(x []any) any {
				if x == nil {
					x = []any{}
				}
				return x
			}),
			rapid.Map(rapid.MapOfN(rapid.SampledFrom([]string{"id", "label", "value", "row", "column", "state"}), inner, 0, 4), func(m map[string]any) any { return m }))
	}
	return rapid.OneOf(opts...)
}

func genSurface(t *rapid.T) a2ui.Surface {
	ids := []string{"root"}
	n := rapid.IntRange(1, 10).Draw(t, "n")
	for i := 1; i < n; i++ {
		ids = append(ids, "c"+string(rune('a'+i)))
	}
	ids = append(ids, "gone")
	childRefs := rapid.OneOf(
		rapid.Map(rapid.SliceOfN(rapid.SampledFrom(ids), 0, 4), func(x []string) any {
			out := make([]any, len(x))
			for i, s := range x {
				out[i] = s
			}
			return out
		}),
		rapid.Map(rapid.SampledFrom(ids), func(id string) any { return map[string]any{"componentId": id, "path": "/items"} }),
	)
	props := []string{"text", "label", "value", "variant", "direction", "axis", "options", "action", "rows", "columns", "cells", "weight", "placeholder", "displayStyle"}
	var comps []map[string]any
	for i := 0; i < n; i++ {
		c := map[string]any{"id": ids[i], "component": rapid.SampledFrom(surfaceTypes).Draw(t, "type")}
		for range rapid.IntRange(0, 5).Draw(t, "props") {
			c[rapid.SampledFrom(props).Draw(t, "prop")] = genSurfaceValue(ids, 2).Draw(t, "val")
		}
		switch rapid.IntRange(0, 2).Draw(t, "kids") {
		case 0:
			c["children"] = childRefs.Draw(t, "children")
		case 1:
			c["child"] = rapid.SampledFrom(ids).Draw(t, "child")
		}
		if rapid.IntRange(0, 5).Draw(t, "cat") == 0 {
			c["@catalogId"] = rapid.SampledFrom([]string{a2ui.BasicCatalogID, a2ui.RelayCatalogID, "x"}).Draw(t, "own")
		}
		comps = append(comps, c)
	}
	data := map[string]any{
		"a":     genSurfaceText().Draw(t, "a"),
		"items": rapid.SliceOfN(rapid.Map(genSurfaceText(), func(s string) any { return map[string]any{"x": s, "done": true} }), 0, 5).Draw(t, "items"),
	}
	catalog := rapid.SampledFrom([]string{a2ui.BasicCatalogID, a2ui.RelayCatalogID, a2ui.RelayCatalogID}).Draw(t, "catalog")
	var ss a2ui.Surfaces
	for _, m := range []map[string]any{
		{"version": "1.0", "createSurface": map[string]any{"surfaceId": "s", "catalogId": catalog, "dataModel": data}},
		{"version": "1.0", "updateComponents": map[string]any{"surfaceId": "s", "components": comps}},
	} {
		line, err := json.Marshal(m, json.Deterministic(true))
		if err != nil {
			t.Skip()
		}
		dm, err := a2ui.Decode(line)
		if err != nil {
			t.Fatalf("Decode(%s): %v", line, err)
		}
		if ss, err = a2ui.Apply(ss, dm); err != nil {
			t.Fatalf("Apply(%s): %v", line, err)
		}
	}
	s, _ := ss.Get("s")
	return s
}

// No drawn line is wider than the width asked for, at any width, with or
// without a focus.
func TestA2UINeverDrawsPastTheWidth(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := genSurface(t)
		width := rapid.IntRange(1, 120).Draw(t, "width")
		var focus *a2ui.Focus
		if fs := s.Focusables(); len(fs) > 0 && rapid.Bool().Draw(t, "focused") {
			focus = &fs[rapid.IntRange(0, len(fs)-1).Draw(t, "which")]
		}
		for _, l := range A2UI(s, width, focus) {
			if w := Method.StringWidth(l); w > width {
				t.Fatalf("width %d: line is %d wide: %q", width, w, l)
			}
		}
	})
}

// Whatever a surface holds in its strings, the drawing has no control
// character and no escape sequence but the styles this package writes.
func TestA2UIDrawsNoControlBytesFromTheSurface(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := genSurface(t)
		width := rapid.IntRange(1, 120).Draw(t, "width")
		for _, l := range A2UI(s, width, nil) {
			if strings.Contains(l, "\x1b[38;5;196") || strings.Contains(l, "\x1b]8;;") {
				t.Fatalf("a sequence from the surface got through: %q", l)
			}
			rest := sgrRE.ReplaceAllString(l, "")
			for _, r := range rest {
				if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) || unicode.Is(unicode.Cf, r) && r != 0x200d {
					t.Fatalf("control or format character %U in %q", r, l)
				}
			}
			for _, sgr := range sgrRE.FindAllString(l, -1) {
				if !regexp.MustCompile(`^\x1b\[[0-9;]*m$`).MatchString(sgr) {
					t.Fatalf("odd sequence %q", sgr)
				}
			}
		}
	})
}

// A link in a surface's text never shows live.
func TestA2UIDefangsLinks(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := genSurface(t)
		for _, l := range A2UI(s, 80, nil) {
			if live(ansiPlain(l)) {
				t.Fatalf("live link in %q", l)
			}
		}
	})
}

const placeholderSurface = `{"version":"1.0","updateComponents":{"surfaceId":"s","components":[` +
	`{"id":"root","component":"Column","children":["img","vid","aud","odd","gone","txt"]},` +
	`{"id":"img","component":"Image","url":"https://evil.test/a.png?d=secret"},` +
	`{"id":"vid","component":"Video","url":"https://evil.test/v.mp4","posterUrl":"https://evil.test/p.png"},` +
	`{"id":"aud","component":"AudioPlayer","url":"https://evil.test/a.mp3"},` +
	`{"id":"odd","component":"Hologram","url":"https://evil.test/h"},` +
	`{"id":"txt","component":"Text","text":"see https://evil.test/x?q=1 now"}` +
	`]}}`

func TestA2UIPlaceholdersAndLinks(t *testing.T) {
	create := `{"version":"1.0","createSurface":{"surfaceId":"s","catalogId":"` + a2ui.BasicCatalogID + `"}}`
	var ss a2ui.Surfaces
	for _, l := range []string{create, placeholderSurface} {
		m, err := a2ui.Decode([]byte(l))
		if err != nil {
			t.Fatal(err)
		}
		if ss, err = a2ui.Apply(ss, m); err != nil {
			t.Fatal(err)
		}
	}
	out := ansiPlain(strings.Join(A2UI(ss[0], 60, nil), "\n"))
	for _, want := range []string{"[image not shown]", "[video not shown]", "[audioplayer not shown]", "[hologram not shown]", "[missing gone]", "hxxps://evil[.]test/x?q=1"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "a.png") || strings.Contains(out, "evil.test/") {
		t.Errorf("a URL shows:\n%s", out)
	}
}

func TestA2UIWaitsForRoot(t *testing.T) {
	out := A2UI(a2ui.Surface{ID: "s", CatalogID: a2ui.BasicCatalogID}, 40, nil)
	if len(out) != 1 || !strings.Contains(out[0], "waiting") {
		t.Fatalf("got %q", out)
	}
}
