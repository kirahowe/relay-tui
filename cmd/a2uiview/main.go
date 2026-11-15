// Command a2uiview prints how relay-tui draws A2UI surfaces, for mocks:
//
//	a2uiview [-width n] file.jsonl
//
// The file holds one A2UI v1.0 message per line. The messages are folded in
// order, and each surface that is left is drawn at the given width. A line
// that does not decode or apply is reported on stderr and skipped, and the
// exit status is 1 when any was. Output keeps its colours when stdout is a
// terminal and is plain text when it is not.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/kirahowe/relay-tui/internal/a2ui"
	"github.com/kirahowe/relay-tui/internal/render"
)

// maxLine is the longest message line read, in bytes.
const maxLine = 4 << 20

func main() {
	width := flag.Int("width", 80, "draw this many columns wide")
	flag.Parse()
	if flag.NArg() != 1 || *width < 1 {
		fmt.Fprintln(os.Stderr, "usage: a2uiview [-width n] file.jsonl")
		os.Exit(2)
	}
	f, err := os.Open(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "a2uiview:", err)
		os.Exit(1)
	}
	defer f.Close()
	st, _ := os.Stdout.Stat()
	color := st != nil && st.Mode()&os.ModeCharDevice != 0
	if !run(f, os.Stdout, os.Stderr, *width, color) {
		os.Exit(1)
	}
}

// run folds the messages in in and writes each surface's drawing to out.
// It reports whether every line was good.
func run(in io.Reader, out, errs io.Writer, width int, color bool) bool {
	var ss a2ui.Surfaces
	ok := true
	r := bufio.NewReaderSize(in, 64<<10)
	for n := 1; ; n++ {
		line, err := readLine(r)
		if len(strings.TrimSpace(string(line))) > 0 {
			m, derr := a2ui.Decode(line)
			if derr == nil {
				ss, derr = a2ui.Apply(ss, m)
			}
			if derr != nil {
				fmt.Fprintf(errs, "line %d: %v\n", n, derr)
				ok = false
			}
		}
		if err != nil {
			if err != io.EOF {
				fmt.Fprintf(errs, "line %d: %v\n", n, err)
				ok = false
			}
			break
		}
	}
	for i, s := range ss {
		if i > 0 {
			fmt.Fprintln(out)
		}
		lines := append([]string{render.A2UIName(s, width)}, render.A2UI(s, width, nil)...)
		for _, l := range lines {
			if !color {
				l = ansi.Strip(l)
			}
			fmt.Fprintln(out, l)
		}
	}
	return ok
}

// readLine reads one line without its newline, failing on one longer than
// maxLine. The rest of such a line is read and dropped.
func readLine(r *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		part, isPrefix, err := r.ReadLine()
		if len(line)+len(part) > maxLine {
			for isPrefix && err == nil {
				_, isPrefix, err = r.ReadLine()
			}
			if err == nil {
				err = fmt.Errorf("longer than %d bytes", maxLine)
			}
			return nil, err
		}
		line = append(line, part...)
		if !isPrefix || err != nil {
			return line, err
		}
	}
}
