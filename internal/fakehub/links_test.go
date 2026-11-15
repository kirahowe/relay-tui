package fakehub

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	acp "github.com/caelis-labs/acp-go-sdk"
)

const (
	testLink   = "https://artsfund.example/grants/2027"
	testBroken = "hxxps://artsfund[.]example/grants/2027"
)

func linksConfig() Config {
	return Config{
		Links: []string{},
		Cards: []json.RawMessage{json.RawMessage(`{"id": 7, "kind": "note", "status": "active", "rev": 3,
			"title": {"text": "Grants", "outsider": true, "rev": 3},
			"note": {"text": "See ` + testBroken + `", "outsider": true, "rev": 3}, "labels": ["contaminated"]}`)},
	}
}

func callLinks(t *testing.T, conn *acp.ClientSideConnection, method string, params any) (map[string]any, error) {
	t.Helper()
	raw, err := conn.CallExtension(context.Background(), method, params)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("%s answered %s", method, raw)
	}
	return out, nil
}

func TestLinksAreKeptAndDroppedAsRelayDoes(t *testing.T) {
	conn, _ := connect(t, linksConfig())
	got, err := callLinks(t, conn, "_relay/links", map[string]any{})
	if err != nil || len(got["live"].([]any)) != 0 {
		t.Fatalf("links: %v %v", got, err)
	}
	if _, err := callLinks(t, conn, "_relay/cards", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	// Kept as the client showed it, in the broken form.
	got, err = callLinks(t, conn, "_relay/links/keep", map[string]any{"address": testBroken})
	if err != nil || got["changed"] != true {
		t.Fatalf("keep: %v %v", got, err)
	}
	got, _ = callLinks(t, conn, "_relay/links", map[string]any{})
	if live := got["live"].([]any); len(live) != 1 || live[0] != testLink {
		t.Fatalf("after keep: %v", got)
	}
	cards, _ := callLinks(t, conn, "_relay/cards", map[string]any{})
	if b, _ := json.Marshal(cards); !strings.Contains(string(b), "See \\u003c"+testLink+"\\u003e") && !strings.Contains(string(b), "See <"+testLink+">") {
		t.Fatalf("the card does not hold the link whole: %s", b)
	}
	// Kept again, nothing changes.
	if got, _ := callLinks(t, conn, "_relay/links/keep", map[string]any{"address": testLink}); got["changed"] != false {
		t.Fatalf("kept twice: %v", got)
	}
	// Dropped, it is broken again.
	if got, _ := callLinks(t, conn, "_relay/links/drop", map[string]any{"address": testLink}); got["changed"] != true {
		t.Fatalf("drop: %v", got)
	}
	cards, _ = callLinks(t, conn, "_relay/cards", map[string]any{})
	if b, _ := json.Marshal(cards); !strings.Contains(string(b), "See "+testBroken) {
		t.Fatalf("the card does not hold the link broken: %s", b)
	}
	// What a live link cannot be is refused with relay's reason, and a
	// parameter it does not take is refused as relay refuses it.
	for _, bad := range []string{"https://user@evil.test/", "https://evil.test/a(b)", "https://evil.test/a[1]", "javascript:alert(1)", "see " + testLink} {
		_, err := callLinks(t, conn, "_relay/links/keep", map[string]any{"address": bad})
		var re *acp.RequestError
		if !errors.As(err, &re) || re.Code != -32000 || re.Message != LinkRefusal {
			t.Errorf("%q: %v", bad, err)
		}
	}
	for method, params := range map[string]any{
		"_relay/links":      map[string]any{"sessionId": "s"},
		"_relay/links/keep": map[string]any{"address": testLink, "sessionId": "s"},
		"_relay/links/drop": map[string]any{},
	} {
		_, err := callLinks(t, conn, method, params)
		var re *acp.RequestError
		if !errors.As(err, &re) || re.Code != -32602 {
			t.Errorf("%s %v: %v", method, params, err)
		}
	}
}

func TestARelayWithoutLinksDoesNotHaveTheMethods(t *testing.T) {
	conn, _ := connect(t, Config{})
	for _, method := range linkMethods {
		_, err := callLinks(t, conn, method, map[string]any{"address": testLink})
		var re *acp.RequestError
		if !errors.As(err, &re) || re.Code != -32601 {
			t.Errorf("%s: %v", method, err)
		}
	}
}
