package main

import (
	"bytes"
	"strings"
	"testing"
	"unicode"
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

// An error line can hold a string from the file. It prints as outside text
// on one line: no control or format character, and no address a terminal
// would follow.
func TestRunPrintsErrorsAsOutsideText(t *testing.T) {
	in := `{"version":"1.0","createSurface":{"surfaceId":"s","catalogId":"urn:relay:catalog:v1","components":[` +
		`{"id":"bad\u009b31mhttps://evil.test/leak?d=private\u202e","component":"Text","text":"x"},` +
		`{"id":"bad\u009b31mhttps://evil.test/leak?d=private\u202e","component":"Text","text":"y"}]}}` + "\n" +
		`{"version":"1.0","deleteSurface":{"surfaceId":"nope\u009b31mhttps://evil.test/leak"}}` + "\n"
	var out, errs bytes.Buffer
	if run(strings.NewReader(in), &out, &errs, 60, false) {
		t.Fatal("reported success")
	}
	got := errs.String()
	if got == "" {
		t.Fatal("no error reported")
	}
	for _, r := range strings.TrimSuffix(got, "\n") {
		if r == '\n' {
			continue
		}
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) || unicode.Is(unicode.Cf, r) {
			t.Errorf("control or format character %U in stderr:\n%q", r, got)
		}
	}
	if strings.Contains(got, "://evil.test") {
		t.Errorf("a followable address on stderr:\n%q", got)
	}
	if n := strings.Count(got, "\n"); n != strings.Count(got, "line ") {
		t.Errorf("an error spans more than one line:\n%q", got)
	}
}
