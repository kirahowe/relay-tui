package ui

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kirahowe/relay-tui/internal/core"
)

func hubModel(t *testing.T) Model {
	t.Helper()
	step := func(m Model, msg tea.Msg) Model {
		next, _ := m.Update(msg)
		return next.(Model)
	}
	m := New(nopConn{}, "").Open("dev")
	m = step(m, tea.WindowSizeMsg{Width: 100, Height: 24})
	now := time.Now()
	m = step(m, EventMsg{Event: core.HubConnected{At: now}})
	m = step(m, EventMsg{Event: core.RosterUpdate{At: now, Roles: []core.Role{{ID: "dev", Title: "Dev", Conv: &core.RosterConv{Session: "s1", Turn: core.TurnIdle}}}}})
	m = step(m, EventMsg{Event: core.Attached{At: now, Role: "dev", Session: "s1"}})
	for i := 0; i < 60; i++ {
		m = step(m, EventMsg{Event: core.Routed{Session: "s1", Event: core.UserChunk{At: now, MessageID: fmt.Sprint("u", i), Text: fmt.Sprint("question ", i)}}})
	}
	return m
}
