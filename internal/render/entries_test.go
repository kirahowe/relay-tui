package render

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"pgregory.net/rapid"

	"github.com/kirahowe/relay-tui/internal/core"
)

// A run of tool calls names no tool in the chat, whatever the tools are
// called and however the run went; the names are in the transcript.
func TestRunLineNamesNoTool(t *testing.T) {
	name := rapid.StringMatching(`zq[a-z_]{4,10}`)
	rapid.Check(t, func(t *rapid.T) {
		var calls []core.Call
		for i := 0; i < rapid.IntRange(1, 6).Draw(t, "n"); i++ {
			n := name.Draw(t, "name")
			calls = append(calls, core.Call{ID: fmt.Sprint(i), Name: n, Title: n + " args",
				Status: rapid.SampledFrom([]core.ToolStatus{core.OK, core.Failed, core.Running, core.Pending, core.Stopped}).Draw(t, "status")})
		}
		out := ansi.Strip(runLine(calls, 80, t0))
		for _, c := range calls {
			if strings.Contains(out, c.Name) {
				t.Fatalf("%q names %q", out, c.Name)
			}
		}
	})
}
