package core

import (
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// What the operator is told about where something came from never names
// a label, whatever the labels and sources are.
func TestProvenanceNeverNamesALabel(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		l := Labels{Private: rapid.Bool().Draw(t, "private"), Contaminated: rapid.Bool().Draw(t, "contaminated")}
		var srcs []Source
		for range rapid.IntRange(0, 3).Draw(t, "sources") {
			srcs = append(srcs, Source{Title: rapid.String().Draw(t, "title"), Ref: rapid.String().Draw(t, "ref")})
		}
		got := Provenance(l, srcs)
		for _, w := range []string{"contaminated", "untrusted", "private", "trust"} {
			if strings.Contains(got, w) {
				t.Fatalf("%v %v gave %q", l, srcs, got)
			}
		}
		if (got != "") != l.Contaminated {
			t.Fatalf("%v %v gave %q", l, srcs, got)
		}
	})
}

func TestProvenanceSaysWhereItCameFrom(t *testing.T) {
	web := []Source{{Title: "Venue list", Ref: "web:https://example.org/list"}}
	doc := []Source{{Title: "Notes", Ref: "doc:notes/venues.md"}}
	for name, c := range map[string]struct {
		l    Labels
		srcs []Source
		want string
	}{
		"from the web":    {Labels{Contaminated: true}, web, "from the web"},
		"from outside":    {Labels{Contaminated: true}, doc, "from outside"},
		"no sources":      {Labels{Contaminated: true}, nil, "from outside"},
		"private is home": {Labels{Private: true}, web, ""},
		"clean":           {Labels{}, web, ""},
	} {
		if got := Provenance(c.l, c.srcs); got != c.want {
			t.Errorf("%s: %q, want %q", name, got, c.want)
		}
	}
}
