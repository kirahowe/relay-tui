package wire

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	acp "github.com/caelis-labs/acp-go-sdk"

	"github.com/kirahowe/relay-tui/internal/core"
)

func TestLinksAnswer(t *testing.T) {
	at := time.Unix(1, 0)
	got := Links(json.RawMessage(`{"rev": 7, "live": ["https://x.example/a", "http://y.example"]}`), nil, at)
	want := core.LinksLoaded{At: at, Rev: 7, Live: []string{"https://x.example/a", "http://y.example"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if got := Links(json.RawMessage(`{"rev": 7, "live": []}`), nil, at); got.Err != "" || got.Unavailable || got.Live == nil {
		t.Fatalf("an empty set: %+v", got)
	}
	// A plain agent, or a relay from before the method: no link is live.
	if got := Links(nil, acp.NewMethodNotFound(MethodLinks), at); !got.Unavailable {
		t.Fatalf("method not found: %+v", got)
	}
	if got := Links(nil, &acp.RequestError{Code: -32000, Message: "relay has not read its links yet. Try again."}, at); got.Err == "" || got.Unavailable {
		t.Fatalf("a refusal: %+v", got)
	}
	// An answer that is not the one relay sends is not taken for a set.
	for _, bad := range []string{`{}`, `{"rev": 7}`, `{"rev": 7, "live": null}`, `{"rev": 7, "live": "https://x.example/a"}`,
		`{"rev": 7, "live": [1]}`, `{"rev": 7, "live": ["a"], "live": ["b"]}`, `[]`, `not json`} {
		if got := Links(json.RawMessage(bad), nil, at); got.Err == "" {
			t.Errorf("%s was taken: %+v", bad, got)
		}
	}
}

func TestLinksChangedNotification(t *testing.T) {
	at := time.Unix(1, 0)
	got, ok := LinksChanged(json.RawMessage(`{"rev": 8, "live": ["https://x.example/b"], "gone": ["https://x.example/a"]}`), at)
	want := core.LinksChanged{At: at, Rev: 8, Live: []string{"https://x.example/b"}, Gone: []string{"https://x.example/a"}}
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v %v, want %+v", got, ok, want)
	}
	for _, bad := range []string{`{"rev": 8, "live": "x"}`, `{"rev": "8"}`, `not json`, `{"rev": 8, "gone": [2]}`} {
		if _, ok := LinksChanged(json.RawMessage(bad), at); ok {
			t.Errorf("%s was taken", bad)
		}
	}
}
