package ui

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kirahowe/relay-tui/internal/core"
)

// readyConn records what the ready view asks relay for.
type readyConn struct {
	nopConn
	asks    *int
	replies *[]string
}

func (c readyConn) Ready() { *c.asks++ }
func (c readyConn) Reply(m int64, text string) {
	*c.replies = append(*c.replies, fmt.Sprintf("%d:%s", m, text))
}

func readyModel(asks *int, replies *[]string) Model {
	now := time.Now()
	m := New(readyConn{asks: asks, replies: replies}, "").Open("dev")
	m.clock = func() time.Time { return now }
	items := []core.ReadyItem{
		{ID: "message:4", Kind: core.ReadyMessage, At: now.Format(time.RFC3339), Title: "Concert email is drafted",
			Message: 4, ReplyTo: 4},
		{ID: "prepared:5", Kind: core.ReadyDraft, At: now.Add(-time.Minute).Format(time.RFC3339), Title: "Concert"}}
	return update(m, tea.WindowSizeMsg{Width: 120, Height: 30},
		ev(core.HubConnected{At: now}),
		ev(core.RosterUpdate{At: now, Roles: []core.Role{{ID: "dev", Title: "Dev", Conv: &core.RosterConv{Session: "s1", Turn: core.TurnIdle}}}}),
		ev(core.Attached{At: now, Role: "dev", Session: "s1"}),
		ev(core.ReadyLoaded{At: now, Rev: 5, Items: items}))
}

func TestReadyIsAskedForWhenTheConnectionComesUp(t *testing.T) {
	asks, replies := 0, []string{}
	readyModel(&asks, &replies)
	if asks != 1 {
		t.Errorf("asked %d times", asks)
	}
}

// R opens the view from the list, the conversation while moving, and the
// pane; while typing it is a letter in the draft.
func TestRIsOneKeyAway(t *testing.T) {
	asks, replies := 0, []string{}
	moving := update(readyModel(&asks, &replies), tea.KeyPressMsg{Code: tea.KeyEscape})
	for name, setup := range map[string][]tea.Msg{
		"conversation": nil,
		"list":         {letter('h')},
		"pane":         {letter('l')},
	} {
		m := update(moving, setup...)
		m = update(m, key("R"))
		if !m.hub.Ready.Open || core.HubSurface(m.hub).Kind != core.SurfaceReady {
			t.Errorf("%s: R did not open it: %+v", name, core.HubSurface(m.hub))
		}
		m = update(m, key("R"))
		if m.hub.Ready.Open {
			t.Errorf("%s: R did not close it", name)
		}
	}
	typing := readyModel(&asks, &replies)
	typing = update(typing, key("R"))
	if typing.hub.Ready.Open || typing.input.Value() != "R" {
		t.Errorf("R while typing: open %v, draft %q", typing.hub.Ready.Open, typing.input.Value())
	}
}

func TestAReplyIsTypedInTheViewAndNowhereElse(t *testing.T) {
	asks, replies := 0, []string{}
	m := update(readyModel(&asks, &replies), key("esc"), key("R"), key("a"))
	if core.HubSurface(m.hub).Kind != core.SurfaceReadyInput {
		t.Fatalf("surface %+v", core.HubSurface(m.hub))
	}
	// Letters that mean something elsewhere are text here.
	for _, k := range []string{"X", "h", "l", "b", "q", "R", "y", "1"} {
		m = update(m, key(k))
	}
	m = update(m, key("backspace"))
	if m.hub.Ready.Input == nil || m.hub.Ready.Input.Text != "XhlbqRy" {
		t.Fatalf("typed %+v", m.hub.Ready.Input)
	}
	if m.hub.EndAsk || m.input.Value() != "" || len(replies) != 0 {
		t.Fatalf("a key reached behind the view: end %v draft %q replies %v", m.hub.EndAsk, m.input.Value(), replies)
	}
	m = update(m, key("enter"))
	if len(replies) != 1 || replies[0] != "4:XhlbqRy" {
		t.Fatalf("replies %v", replies)
	}
	// Esc, with a reply being written, cancels only the reply.
	m = update(m, ev(core.ReplyDone{At: time.Now(), Message: 4}), key("a"), key("z"), key("esc"))
	if m.hub.Ready.Input != nil || !m.hub.Ready.Open || len(replies) != 1 {
		t.Errorf("%+v replies %v", m.hub.Ready, replies)
	}
	m = update(m, key("esc"))
	if m.hub.Ready.Open {
		t.Errorf("esc did not close the view")
	}
}

func TestCtrlCInTheViewClosesItAndNeverQuits(t *testing.T) {
	asks, replies := 0, []string{}
	m := update(readyModel(&asks, &replies), key("esc"), key("R"), key("a"), key("ctrl+c"))
	if m.hub.Ready.Input != nil || !m.hub.Ready.Open || m.quitting {
		t.Fatalf("%+v quitting %v", m.hub.Ready, m.quitting)
	}
	m = update(m, key("ctrl+c"))
	if m.hub.Ready.Open || m.quitting {
		t.Errorf("%+v quitting %v", m.hub.Ready, m.quitting)
	}
}

// Against a single agent there is no view: R is an unbound letter.
func TestRDoesNothingAgainstAPlainAgent(t *testing.T) {
	var calls []string
	m := New(recConn{calls: &calls}, "")
	m = update(m, tea.WindowSizeMsg{Width: 80, Height: 24}, ev(core.Connected{At: time.Now()}), key("esc"), key("R"))
	if len(calls) != 0 || m.hub.Ready.Open {
		t.Errorf("%v %+v", calls, m.hub.Ready)
	}
}
