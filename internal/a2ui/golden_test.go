package a2ui_test

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/kirahowe/relay-tui/internal/a2ui"
	"github.com/kirahowe/relay-tui/internal/render"
)

// fold reads a fixture and folds all its messages.
func fold(t *testing.T, name string) a2ui.Surfaces {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name + ".jsonl")
	if err != nil {
		t.Fatal(err)
	}
	msgs, err := a2ui.ParseLines(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	var ss a2ui.Surfaces
	for _, m := range msgs {
		if ss, err = a2ui.Apply(ss, m); err != nil {
			t.Fatal(err)
		}
	}
	return ss
}

// Each fixture is one of relay's card kinds as a surface. The goldens show
// ESC as ␛ so they can be read.
func TestFixturesGolden(t *testing.T) {
	for _, name := range []string{"checklist", "tracker", "note"} {
		for _, w := range []int{60, 100} {
			t.Run(name+"_"+strconv.Itoa(w), func(t *testing.T) {
				ss := fold(t, name)
				if len(ss) != 1 {
					t.Fatalf("%d surfaces", len(ss))
				}
				out := strings.Join(render.A2UI(ss[0], w, nil), "\n")
				golden.RequireEqual(t, strings.ReplaceAll(out, "\x1b", "␛")+"\n")
			})
		}
	}
}

// A narrow terminal still gets a drawing that fits.
func TestFixturesFitNarrow(t *testing.T) {
	for _, name := range []string{"checklist", "tracker", "note"} {
		for _, w := range []int{1, 5, 12, 30} {
			for _, l := range render.A2UI(fold(t, name)[0], w, nil) {
				if render.Width(l) > w {
					t.Errorf("%s at %d: %q is %d wide", name, w, l, render.Width(l))
				}
			}
		}
	}
}

// Pressing keys on the checklist: focus lands on the items in order, and
// toggling one shows in the next drawing.
func TestChecklistFocusAndToggle(t *testing.T) {
	s := fold(t, "checklist")[0]
	f := s.Focusables()
	if len(f) != 7 || f[0].ID != "item" || f[6].ID != "archive" {
		t.Fatalf("focusables %v", f)
	}
	out, act, err := a2ui.Activate(s, f[2])
	if err != nil || act != nil {
		t.Fatalf("act %v err %v", act, err)
	}
	if v, _ := a2ui.Get(out.Data, "/items/2/done"); v != true {
		t.Fatalf("third item is %v", v)
	}
	drawn := strings.Join(render.A2UI(out, 60, &f[2]), "\n")
	if !strings.Contains(drawn, "✓") || strings.Count(drawn, "✓") != 3 {
		t.Fatalf("want three ticks:\n%s", drawn)
	}
	_, act, err = a2ui.Activate(s, f[6])
	if err != nil || act == nil || act.Name != "archiveCard" || act.Context["cardId"] != "card-17" || act.SourceComponentID != "archive" {
		t.Fatalf("archive: %+v %v", act, err)
	}
}
