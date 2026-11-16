package ui_test

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/kirahowe/relay-tui/internal/fakehub"
)

// Against a relay that makes conversation text inert itself
// (relay/inertText), a contaminated reply arrives with every link broken
// and its live links listed beside the chunk. The terminal holds its text
// as relay sent it, and a hyperlink to the listed live link under its
// broken form, and nowhere else.
func TestARelayInertReplyIsDrawnAsSentWithItsListedLink(t *testing.T) {
	const said = "Arts fund: " + e2eLinkBroken + " and " + e2eOtherBroken + " and h\\u{200b}ttps[:]//evil[.]test today."
	cfg := threeRoles()
	cfg.InertText = true
	cfg.Links = []string{e2eLink}
	cfg.Roles[0].Labels = []string{"contaminated"}
	cfg.Roles[0].Live.History = append(cfg.Roles[0].Live.History, fakehub.Said{User: "and the grants?"}, fakehub.Said{Agent: said})
	h := startHub(t, cfg, 140, 30, "dev")
	h.waitLog("links")
	// Each word as relay sent it: the reply may wrap at a space.
	h.waitScreen("the reply as sent", func(screen string) bool {
		for _, w := range strings.Fields(said) {
			if !strings.Contains(screen, w) {
				return false
			}
		}
		return true
	})
	h.wait("the listed link as a hyperlink", h.screen, func(string) bool { return h.linked(e2eLinkBroken, e2eLink) })
	if !h.unlinked(e2eOtherBroken) {
		t.Fatalf("a link that is not live is a hyperlink:\n%s", h.screen())
	}
	if got := slices.Sorted(maps.Keys(h.targets())); !slices.Equal(got, []string{e2eLink}) {
		t.Fatalf("hyperlinks written: %v", got)
	}
	h.noAddressAsText(t)
}
