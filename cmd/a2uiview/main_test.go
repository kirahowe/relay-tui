package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunDrawsEachSurface(t *testing.T) {
	in := `{"version":"1.0","createSurface":{"surfaceId":"one","catalogId":"urn:relay:catalog:v1","components":[{"id":"root","component":"Text","text":"hello \u001b[31mworld"}]}}

{"version":"1.0","callRendererFunction":{}}
{"version":"1.0","createSurface":{"surfaceId":"two"}}
not json
`
	var out, errs bytes.Buffer
	if run(strings.NewReader(in), &out, &errs, 30, false) {
		t.Fatal("reported success with bad lines")
	}
	got := out.String()
	for _, want := range []string{"surface one", `hello \u001b[31mworld`, "surface two", "waiting for the root"} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "\x1b") {
		t.Errorf("plain output holds an escape:\n%q", got)
	}
	for _, want := range []string{"line 3: not supported", "line 5: INVALID_MESSAGE"} {
		if !strings.Contains(errs.String(), want) {
			t.Errorf("stderr lacks %q:\n%s", want, errs.String())
		}
	}
}

// A surface's id comes from the file like every other string, so the line
// that names the surface escapes what does not print and breaks a link.
func TestRunNamesASurfaceSafely(t *testing.T) {
	in := `{"version":"1.0","createSurface":{"surfaceId":"s\u001b]8;;http://evil.test/?d=1\u0007x","catalogId":"urn:relay:catalog:v1"}}` + "\n"
	for _, color := range []bool{false, true} {
		var out, errs bytes.Buffer
		if !run(strings.NewReader(in), &out, &errs, 80, color) {
			t.Fatalf("color %v: %s", color, errs.String())
		}
		got := out.String()
		if want := `surface s\u001b]8;;hxxp://evil[.]test/?d=1\u0007x`; !strings.Contains(got, want) {
			t.Errorf("color %v: output lacks %q:\n%q", color, want, got)
		}
		if strings.Contains(got, "://evil.test") || strings.Contains(got, "\x1b]") || strings.Contains(got, "\x07") {
			t.Errorf("color %v: the id reached the output live:\n%q", color, got)
		}
	}
}
