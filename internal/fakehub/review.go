package fakehub

// relay's review (relay#79): every permission request carries the text
// the operator consents to and its digest, in _meta["relay/review"], and
// so does every card's wire form, as a "review" member. For a client
// that declared clientCapabilities._meta["relay/review"]: 1, an allow
// must give the digest back in its result's _meta["relay/review"], and a
// card's trust must name it as its sha256; otherwise relay refuses. A
// refused allow is asked again, under a new id, for the same tool call,
// with _meta["relay/refused"] saying why, at most maxAsks times. The
// control file's "refuse-digest" makes the next allow be refused however
// it is sent, as if relay held another text.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// maxAsks bounds how often one request is asked of a client that keeps
// sending a wrong digest.
const maxAsks = 3

// review is relay's review member for text t.
func review(t string) map[string]any {
	sum := sha256.Sum256([]byte(t))
	return map[string]any{"text": t, "sha256": hex.EncodeToString(sum[:])}
}

// requestText is the review text of a send_mail request in session,
// long with many lines of body.
func requestText(session, name string, long bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "role: %s\ntool: %s (egress)\nwhat it does: sends a mail to a@example.com\n", session, name)
	if long {
		for i := 1; i <= 80; i++ {
			fmt.Fprintf(&b, "body line %d of the mail that goes out\n", i)
		}
	}
	b.WriteString("input:\n{\n  \"to\": \"a@example.com\"\n}\nsession labels: none\noptions:\n  allow_once: Allow once\n  reject_once: Reject")
	return b.String()
}

// cardText is the review text of a card in its wire form: its id, rev,
// title and items.
func cardText(c map[string]any) string {
	text := func(v any) string {
		m, _ := v.(map[string]any)
		s, _ := m["text"].(string)
		return s
	}
	var b strings.Builder
	fmt.Fprintf(&b, "card %v rev %v\ntitle: %s", c["id"], c["rev"], text(c["title"]))
	items, _ := c["items"].([]any)
	for _, it := range items {
		fmt.Fprintf(&b, "\n- %s", text(it))
	}
	return b.String()
}

// withReview is a copy of card c with its review member.
func withReview(c map[string]any) map[string]any {
	out := make(map[string]any, len(c)+1)
	for k, v := range c {
		out[k] = v
	}
	out["review"] = review(cardText(c))
	return out
}

// withReviews is cards, each with its review.
func withReviews(cards []map[string]any) []map[string]any {
	out := make([]map[string]any, len(cards))
	for i, c := range cards {
		out[i] = withReview(c)
	}
	return out
}
