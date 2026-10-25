package core

import "strings"

// What a request says under relay's own _meta keys (relay/review,
// relay/refused, and relay/preview with relay/labels on a content block)
// is a claim. Only relay's hub makes such claims, so they count only in a
// conversation of the hub (State.Shared: the agent said relay/hub at
// initialize). Against any other agent they are the agent's words: the
// review text is built from the call's own fields, and a preview block
// is content like any other. trustRelay is the one place that decides;
// nothing else reads the claims.

// RelayClaims is what a request claimed under relay/review and
// relay/refused, as wire read it, before trustRelay decides.
type RelayClaims struct {
	Review  *Review
	Refused string
}

// trustRelay is p with relay's claims honoured, in a conversation of
// relay's hub, or set aside: the review, the refusal and the preview are
// relay's only when hub is set.
func trustRelay(p Permission, hub bool) Permission {
	claims := p.Claims
	p.Claims = RelayClaims{}
	p.Review, p.Refused, p.Preview, p.PreviewLabels = nil, "", "", Labels{}
	content := make([]CallContent, 0, len(p.Content))
	var preview []string
	for _, c := range p.Content {
		if c.Preview && hub {
			preview = append(preview, c.Text)
			p.PreviewLabels = p.PreviewLabels.Union(c.PreviewLabels)
			continue
		}
		c.Preview, c.PreviewLabels = false, Labels{}
		content = append(content, c)
	}
	p.Content = content
	if hub {
		p.Review, p.Refused = claims.Review, claims.Refused
		p.Preview = strings.Join(preview, "\n")
	}
	return p
}
