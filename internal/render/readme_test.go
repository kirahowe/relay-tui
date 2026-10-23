package render

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The README's tables of keys in "Typing and moving" and the lists of
// keys the help shows name the same keys. The help rows are checked
// against the classifiers in package ui, so the three agree.
func TestReadmeKeysMatchTheHelp(t *testing.T) {
	b, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(b)
	start := strings.Index(doc, "## Typing and moving")
	end := strings.Index(doc[start+1:], "\n## ")
	if start < 0 || end < 0 {
		t.Fatal("no section Typing and moving")
	}
	section := doc[start : start+1+end]
	tables := map[string][]KeyHelp{
		"Typing:": append(slices.Clone(TypingKeys), InlineTypingKeys...),
		"Moving:": append(slices.Clone(MovingKeys), InlineMovingKeys...),
	}
	tick := regexp.MustCompile("`([^`]+)`")
	for head, rows := range tables {
		i := strings.Index(section, "\n"+head+"\n")
		if i < 0 {
			t.Fatalf("no %q table", head)
		}
		table := section[i+len(head)+2:]
		table = table[:strings.Index(table, "\n\n")]
		var keys []string
		for _, line := range strings.Split(table, "\n") {
			cells := strings.Split(line, "|")
			if len(cells) < 3 {
				continue
			}
			for _, m := range tick.FindAllStringSubmatch(cells[1], -1) {
				keys = append(keys, m[1])
			}
		}
		var named []string
		for _, r := range rows {
			named = append(named, r.Keys...)
		}
		for _, k := range named {
			if !slices.Contains(keys, k) {
				t.Errorf("%s the help names %q and the README does not", head, k)
			}
		}
		for _, k := range keys {
			// The README names the ends of a range, and the newline keys
			// only some terminals send, in the row's words.
			if slices.Contains(named, k) || k == "9" || k == "alt+9" {
				continue
			}
			t.Errorf("%s the README names %q and the help does not", head, k)
		}
	}
}
