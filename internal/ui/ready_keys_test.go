package ui

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kirahowe/relay-tui/internal/core"
	"github.com/kirahowe/relay-tui/internal/render"
)

// readyConn records what the ready view asks relay for.
type readyConn struct {
	nopConn
	asks    *int
	replies *[]string
	undos   *[]int64
	acts    *[]string
}

func (c readyConn) Act(m int64, n int, op string) {
	if c.acts != nil {
		*c.acts = append(*c.acts, fmt.Sprintf("%d:%d:%s", m, n, op))
	}
}

func (c readyConn) Undo(m int64) {
	if c.undos != nil {
		*c.undos = append(*c.undos, m)
	}
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

func TestUOnAMoveAsksRelayToUndoIt(t *testing.T) {
	asks, replies, undos := 0, []string{}, []int64{}
	now := time.Now()
	m := New(readyConn{asks: &asks, replies: &replies, undos: &undos}, "").Open("dev")
	m.clock = func() time.Time { return now }
	items := []core.ReadyItem{
		{ID: "move:7", Kind: core.ReadyMove, At: now.Format(time.RFC3339), Seq: 7, Title: "statement.pdf",
			Lines: []string{"By steward.", "/f/Downloads/statement.pdf → /f/Bank/statement.pdf"}, Move: 7, Role: "steward", Undo: 7},
		{ID: "message:4", Kind: core.ReadyMessage, At: now.Add(-time.Minute).Format(time.RFC3339), Title: "Hi", Message: 4, ReplyTo: 4}}
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 30},
		ev(core.HubConnected{At: now}),
		ev(core.RosterUpdate{At: now, Roles: []core.Role{{ID: "dev", Title: "Dev", Conv: &core.RosterConv{Session: "s1", Turn: core.TurnIdle}}}}),
		ev(core.Attached{At: now, Role: "dev", Session: "s1"}),
		ev(core.ReadyLoaded{At: now, Rev: 5, Items: items}),
		key("esc"), key("R"), key("u"))
	if len(undos) != 1 || undos[0] != 7 {
		t.Fatalf("undos %v", undos)
	}
	m = update(m, ev(core.UndoDone{At: now, Move: 7, Err: "Move 7 was not undone, and nothing changed: /f/Bank/statement.pdf has changed since it was moved."}))
	if m.hub.Ready.Message != core.UndoRefusedNote || !strings.Contains(m.hub.Ready.Failure, "changed since") {
		t.Fatalf("%+v", m.hub.Ready)
	}
	// u on a message does nothing but say so.
	m = update(m, key("j"), key("u"))
	if len(undos) != 1 || m.hub.Ready.Message != core.NoUndoNote {
		t.Errorf("undos %v %+v", undos, m.hub.Ready)
	}
}

func noteModel(acts *[]string) Model {
	now := time.Now()
	asks, replies := 0, []string{}
	m := New(readyConn{asks: &asks, replies: &replies, acts: acts}, "").Open("dev")
	m.clock = func() time.Time { return now }
	note := core.ReadyItem{ID: "message:9", Kind: core.ReadyMessage, At: now.Format(time.RFC3339), Title: "Daily note", Message: 9, ReplyTo: 9,
		Lines: []string{"1. A change", "And 5 more changes like these", "7. A suggestion"},
		Acts:  []core.ReadyAct{{N: 1, Op: core.OpRevert, Line: 0}, {N: 7, Op: core.OpDismiss, Line: 2}}}
	return update(m, tea.WindowSizeMsg{Width: 60, Height: 30},
		ev(core.HubConnected{At: now}),
		ev(core.RosterUpdate{At: now, Roles: []core.Role{{ID: "dev", Title: "Dev", Conv: &core.RosterConv{Session: "s1", Turn: core.TurnIdle}}}}),
		ev(core.Attached{At: now, Role: "dev", Session: "s1"}),
		ev(core.ReadyLoaded{At: now, Rev: 5, Items: []core.ReadyItem{note}}))
}

// Enter opens the note whole, v reverts the picked change and d drops a
// suggestion, each with the note's own number.
func TestKeysOnANotesNumberedItemsCallRelay(t *testing.T) {
	var acts []string
	m := update(noteModel(&acts), key("esc"), key("R"), key("enter"))
	if m.hub.Ready.Detail != "message:9" || m.hub.Ready.Input != nil {
		t.Fatalf("enter should open it: %+v", m.hub.Ready)
	}
	m = update(m, key("d")) // item 1 is a change: d is the wrong key
	if len(acts) != 0 {
		t.Fatalf("%v", acts)
	}
	m = update(m, key("v"))
	m = update(m, ev(core.ActDone{At: time.Now(), Message: 9, N: 1, Op: core.OpRevert}), key("7"), key("d"))
	if want := []string{"9:1:revert", "9:7:dismiss"}; !slices.Equal(acts, want) {
		t.Errorf("%v want %v", acts, want)
	}
	if got := ansi.Strip(m.View().Content); !strings.Contains(got, "(undone)") {
		t.Errorf("the screen does not show item 1 done:\n%s", got)
	}
}

// ? works in the view, in the list and in an open item.
func TestQuestionMarkWorksInTheReadyView(t *testing.T) {
	m := update(noteModel(nil), key("esc"), key("R"), key("?"))
	if !m.hub.Ready.Help || !strings.Contains(ansi.Strip(m.View().Content), "close this view") {
		t.Fatalf("%+v", m.hub.Ready)
	}
	m = update(m, key("R")) // any key closes the list, and does only that
	if m.hub.Ready.Help || !m.hub.Ready.Open {
		t.Fatalf("%+v", m.hub.Ready)
	}
	m = update(m, key("enter"), key("?"))
	if !m.hub.Ready.Help || !strings.Contains(ansi.Strip(m.View().Content), "numbered item") {
		t.Errorf("%+v", m.hub.Ready)
	}
}

// The keys list for the view and the classifier agree, both ways, as
// they do for the rest.
func TestReadyHelpRowsAgreeWithTheClassifier(t *testing.T) {
	rows := append(slices.Clone(render.ReadyListKeys), render.ReadyDetailKeys...)
	does := func(name string) bool { a, _ := ClassifyReady(press(t, name)); return a != core.ReadyOther }
	for _, r := range rows {
		for _, k := range r.Keys {
			if k == "R" { // the view's own toggle, taken before the classifier
				continue
			}
			if !does(k) {
				t.Errorf("%q (%s) does nothing", k, r.What)
			}
		}
	}
	for _, k := range candidates() {
		if k == "ctrl+c" || k == "R" || !does(k) {
			continue
		}
		if !slices.Contains(named(rows), alias(k, rows)) {
			t.Errorf("%q does something but no row names it", k)
		}
	}
}
