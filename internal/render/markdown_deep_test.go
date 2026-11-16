package render

import (
	"strings"
	"testing"
)

// Markdown nested deeper than the screen is wide draws, row by row
// within the width, and does not panic (relay#127). Quotes, lists and
// code each take columns from the width.
func TestMarkdownNestedDeeperThanTheWidthDraws(t *testing.T) {
	for _, md := range []string{
		strings.Repeat("> ", 45) + "---",
		strings.Repeat("> ", 45) + "deep words here",
		strings.Repeat("> ", 30) + "```\ncode\n```",
		strings.Repeat("> ", 30) + "    indented code",
		strings.Repeat("> ", 30) + "# heading",
		strings.Repeat("> ", 20) + strings.Repeat("- ", 20) + "item",
		strings.Repeat("  ", 0) + strings.Repeat("1. ", 30) + "numbered",
		strings.Repeat("> ", 40) + "| a | b |\n" + strings.Repeat("> ", 40) + "|---|---|",
	} {
		for width := 1; width <= 80; width++ {
			lines := Markdown(md, width)
			for _, l := range lines {
				if w := Width(l); w > width {
					t.Fatalf("%q at %d: a row %d wide: %q", md[:min(len(md), 30)], width, w, l)
				}
			}
		}
	}
}
