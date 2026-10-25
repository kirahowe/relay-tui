package wire

import (
	"encoding/json"
	"testing"
	"time"

	acp "github.com/caelis-labs/acp-go-sdk"

	"github.com/kirahowe/relay-tui/internal/core"
)

const digest64 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// relay's review member is read as sent; anything else about it (no
// text, a digest that is not one) reads as no review, which fails closed.
func TestReviewMember(t *testing.T) {
	if r := Review(json.RawMessage(`{"text":"a\nb","sha256":"` + digest64 + `"}`)); r == nil || r.Text != "a\nb" || r.SHA256 != digest64 {
		t.Fatalf("review %+v", r)
	}
	for _, bad := range []string{``, `null`, `{}`, `{"text":"a"}`, `{"text":"a","sha256":"xyz"}`, `{"text":1,"sha256":"` + digest64 + `"}`, `"a"`} {
		if r := Review(json.RawMessage(bad)); r != nil {
			t.Errorf("%s read as %+v", bad, r)
		}
	}
}

// A request carries relay's review and its refusal of an earlier answer,
// and the call's kind and content for a review built here.
func TestPermissionReview(t *testing.T) {
	kind := acp.ToolKindFetch
	req := acp.RequestPermissionRequest{
		Meta: map[string]json.RawMessage{ReviewKey: json.RawMessage(`{"text":"T","sha256":"` + digest64 + `"}`),
			RefusedKey: json.RawMessage(`"the digest does not match"`)},
		ToolCall: acp.ToolCallUpdate{ToolCallId: "c", Kind: &kind, Content: []acp.ToolCallContent{
			{Content: &acp.ToolCallContentContent{Content: acp.TextBlock("does a thing")}},
			{Content: &acp.ToolCallContentContent{Content: acp.ContentBlock{Text: &acp.ContentBlockText{Text: "preview",
				Meta: map[string]json.RawMessage{previewKey: json.RawMessage("true")}}}}},
		}},
	}
	p := Permission(req, "", 1, time.Now()).Permission
	if p.Review == nil || p.Review.Text != "T" || p.Refused != "the digest does not match" || p.Kind != "fetch" ||
		len(p.Content) != 1 || p.Content[0] != "does a thing" || p.Preview != "preview" {
		t.Fatalf("%+v", p)
	}
}

// An allow gives relay's digest back in its result's _meta; a reject, a
// cancel and an allow with no review carry none. initialize declares
// relay/review.
func TestOutcomeCarriesTheDigest(t *testing.T) {
	got := Outcome(core.AnswerPermission{ID: 1, OptionID: "a", Review: digest64})
	if string(got.Meta[ReviewKey]) != `"`+digest64+`"` {
		t.Fatalf("meta %s", got.Meta)
	}
	for _, a := range []core.AnswerPermission{{OptionID: "a"}, {Cancelled: true}} {
		if got := Outcome(a); got.Meta != nil {
			t.Errorf("%+v carried %s", a, got.Meta)
		}
	}
	if string(Capabilities().Meta[ReviewKey]) != "1" {
		t.Fatalf("capabilities %s", Capabilities().Meta)
	}
}

// A card and a doc carry relay's review.
func TestCardAndItemReviews(t *testing.T) {
	c, ok := card(json.RawMessage(`{"id":1,"kind":"note","status":"active","rev":2,"title":{"text":"t","rev":2},"review":{"text":"card 1","sha256":"` + digest64 + `"}}`))
	if !ok || c.Review == nil || c.Review.Text != "card 1" {
		t.Fatalf("card %+v", c)
	}
	e := Items(json.RawMessage(`{"items":[{"kind":"shown","path":"p","version":3,"review":{"text":"doc","sha256":"`+digest64+`"}}]}`), nil, time.Now())
	if len(e.Items) != 1 || e.Items[0].Review == nil || e.Items[0].Review.Text != "doc" {
		t.Fatalf("items %+v", e)
	}
}
