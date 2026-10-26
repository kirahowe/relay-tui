package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/kirahowe/relay-tui/internal/render"
)

// named is every key a help row names.
func named(rows []render.KeyHelp) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r.Keys...)
	}
	return out
}

// candidates is every key worth asking a classifier about: the named
// keys, every printable character, and ctrl and alt with a letter or a
// digit.
func candidates() []string {
	out := []string{"enter", "esc", "up", "down", "left", "right", "pgup", "pgdown", "home", "end", "space",
		"shift+enter", "ctrl+enter", "alt+enter"}
	for c := '!'; c <= '~'; c++ {
		out = append(out, string(c))
	}
	for c := 'a'; c <= 'z'; c++ {
		out = append(out, "ctrl+"+string(c), "alt+"+string(c))
	}
	for c := '0'; c <= '9'; c++ {
		out = append(out, "alt+"+string(c))
	}
	return append(out, "ctrl+/", "ctrl+_")
}

// alias is the key a help row names for k, when the row stands for a
// family: "1" for 1 to 9, "alt+1" for alt+1 to alt+9, and the newline
// keys some terminals send instead.
func alias(k string, rows []render.KeyHelp) string {
	if n, ok := strings.CutPrefix(k, "alt+"); ok && digit(n) {
		if slices.Contains(named(rows), "alt+1") {
			return "alt+1"
		}
		return "1"
	}
	if digit(k) {
		return "1"
	}
	switch k {
	case "ctrl+enter", "alt+enter":
		return "shift+enter"
	}
	return k
}

// The list of keys and the classifiers agree, both ways: every key a row
// names does something in its mode, and every key that does something
// is named, so neither can drift from the other.
func TestHelpRowsAgreeWithTheClassifiers(t *testing.T) {
	typing := func(name string) bool { a, _ := Classify(press(t, name), true); return a != PassThrough }
	moving := func(name string) bool { a, _ := ClassifyMoving(press(t, name)); return a != MoveNone }
	single := func(name string) bool { // no roles to jump to
		a, _ := ClassifyMoving(press(t, name))
		return a != MoveNone && a != MoveJump && a != MoveEnd && a != MoveReady && a != MoveUp && a != MoveDown &&
			a != MovePageUp && a != MovePageDown && a != MoveTop && a != MoveBottom && a != MoveEsc
	}
	singleTyping := func(name string) bool {
		a, _ := Classify(press(t, name), true)
		return a != PassThrough && a != Jump
	}
	for _, c := range []struct {
		name string
		rows []render.KeyHelp
		does func(string) bool
	}{
		{"hub typing", render.TypingKeys, typing},
		{"hub moving", render.MovingKeys, moving},
		{"inline typing", render.InlineTypingKeys, singleTyping},
		{"inline moving", render.InlineMovingKeys, single},
	} {
		for _, r := range c.rows {
			for _, k := range r.Keys {
				if k == "pgup" || k == "pgdown" { // the hub takes them while typing, ahead of Classify
					continue
				}
				if !c.does(k) {
					t.Errorf("%s: %q (%s) does nothing", c.name, k, r.What)
				}
			}
		}
		for _, k := range candidates() {
			if c.does(k) && !slices.Contains(named(c.rows), alias(k, c.rows)) {
				t.Errorf("%s: %q does something but no row names it", c.name, k)
			}
		}
	}
	for _, r := range render.TranscriptKeys {
		for _, name := range r.Keys {
			if a, _ := ClassifyTranscript(press(t, name), true); a == PassThrough {
				t.Errorf("transcript %q (%s) does nothing", name, r.What)
			}
		}
	}
}

func TestClassifyEndOnlyYesEnds(t *testing.T) {
	for name, want := range map[string][2]bool{"y": {true, true}, "n": {true, false}, "esc": {true, false},
		"enter": {false, false}, "x": {false, false}, "ctrl+c": {true, false}} {
		a, y := ClassifyEnd(press(t, name))
		if a != want[0] || y != want[1] {
			t.Errorf("%q: %v %v, want %v", name, a, y, want)
		}
	}
}
