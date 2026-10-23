package core

import (
	"slices"
	"strconv"
	"strings"
	"time"
)

// The saved view is where the operator was when the client closed, so
// that opening it again against the same agent looks the same: the role
// in focus, the list and its cursor, whether the pane has the keys and
// which view it shows, the selected card and whether it is open, and the
// unsent drafts with their pastes. It is presentation, kept by the shell
// in a file of its own (package viewfile), never sent to the agent.
//
// SaveHub and SaveSingle derive it from state. RestoreHub and
// RestoreSingle hand a loaded one to the state at the start, and the
// reducers put it back part by part as what each part refers to arrives:
// the roster for the focus, the list and the drafts (against a single
// agent, the session), the board for the card. Anything that has gone
// meanwhile, a role, a card, a conversation, falls back to the default.
// A key the operator presses before a part is back drops that part (but
// not the drafts): the operator has moved on. Any key counts, a letter
// typed into the input as much as one that moves, so the shell sends
// KeyPressed for each.
//
// Nothing saved has been sent, answered or confirmed. No permission
// answer, trust of a card or doc, end question or queued message is
// saved. A paste keeps its trust exactly as the operator left it, and
// can only come back less trusted (see restoredDraft).

// View is a saved view.
type View struct {
	// Hub is the hub's part, nil against a single agent.
	Hub *HubView
	// PaneFocus says the pane had the keys; Items, that it showed the
	// workspace items rather than the board.
	PaneFocus, Items bool
	// Card is the selected card (0 for none); Detail says it was open,
	// with Line and Col the selected line and tracker column.
	Card      int64
	Detail    bool
	Line, Col int
	// Drafts are the unsent inputs: one per role in hub mode, one with no
	// role against a single agent.
	Drafts []Draft
}

// HubView is the hub's part of a saved view (see Hub). The column with
// the keys is saved, not the mode: the mode follows from the column
// when the view comes back (typing in a conversation, moving anywhere
// else).
type HubView struct {
	Focus, Cursor string
	List          bool
}

// Draft is an unsent input: its text, with the placeholders of the
// pastes it holds, and those pastes.
type Draft struct {
	Role      string
	Text      string
	Pastes    []Paste
	NextPaste int
}

// Restore is a saved view on its way back. Layout, Board and Drafts say
// which parts still wait.
type Restore struct {
	View                  View
	Layout, Board, Drafts bool
	// Moved says the operator pressed a key while it waited.
	Moved bool
}

// RestoreDraft asks the shell to put a saved draft's text in Role's
// input, ahead of anything typed there since the start, or, with After,
// after what is there.
type RestoreDraft struct {
	Role, Text string
	After      bool
}

func (RestoreDraft) effect() {}

// fromOperator says the event is a key, a paste or a click of the
// operator's.
func fromOperator(e Event) bool {
	switch e.(type) {
	case KeyPressed, Submit, Interrupt, EOF, Pasted, TrustPaste, PaneKey, ShowTools, ShowThinking, TranscriptKey, ToggleHelp,
		PermissionMove, PermissionConfirm, PermissionQuick, PermissionCancel, PermissionShowAll,
		FocusIndex, Move, Type, StopTyping, Unbound, ListMove, ListOpen, StartConversation, AskEnd, ConfirmEnd:
		return true
	}
	return false
}

// pending is what of a restore is still to come after event e: a key of
// the operator's drops the layout and the card, which the operator has
// moved away from, but not the drafts. It never changes rs in place.
func pending(rs *Restore, e Event) *Restore {
	if rs == nil {
		return nil
	}
	out := *rs
	if fromOperator(e) {
		out.Layout, out.Board, out.Moved = false, false, true
	}
	return &out
}

func (rs *Restore) done() bool { return !rs.Layout && !rs.Board && !rs.Drafts }

// RestoreHub hands a saved view to the hub at the start. Which view the
// pane shows is put back at once; the rest waits (see ReduceHub). A role
// named by --open wins over the saved focus, list and pane.
func RestoreHub(h Hub, v View) Hub {
	h.App = restoreItems(h.App, v)
	// A draft with no role was typed before any role was in focus; it
	// goes to the first one that is (see placeLoose).
	for _, d := range v.Drafts {
		if d.Role == "" {
			if text, pastes, next := restoredDraft(d); strings.TrimSpace(text) != "" || len(pastes) > 0 {
				h.Loose = &Draft{Text: text, Pastes: pastes, NextPaste: next}
			}
		}
	}
	h.Restore = &Restore{View: v, Layout: v.Hub != nil && h.Open == "", Board: v.Card != 0, Drafts: len(v.Drafts) > 0}
	if h.Restore.done() {
		h.Restore = nil
	}
	return h
}

// HubFromSingle is the hub's state when the agent turns out to be
// relay's hub, after the client started as if against a single agent:
// x is that state, and draft the text typed into its input meanwhile.
// The restore x was given carries over, less the layout and the card if
// a key was pressed since. What was typed and pasted carries over too,
// pastes trusted or not exactly as they were, as a draft for the first
// role in focus (Hub.Loose), after any saved one with no role.
func HubFromSingle(h Hub, x Single, draft string) Hub {
	h.App.PasteSeq = max(h.App.PasteSeq, x.App.PasteSeq)
	h = hubRestore(h, x.Restore)
	if pastes := referenced(x.State.Pastes, draft); strings.TrimSpace(draft) != "" || len(pastes) > 0 {
		d := Draft{Text: draft, Pastes: pastes, NextPaste: x.State.NextPaste}
		if l := h.Loose; l != nil {
			text, ps, next := afterPastes(d.Text, d.Pastes, d.NextPaste, l.Pastes, l.NextPaste)
			d = Draft{Text: aheadOf(l.Text, text), Pastes: append(slices.Clone(l.Pastes), ps...), NextPaste: max(next, l.NextPaste)}
		}
		h.Loose = &d
	}
	return h
}

func hubRestore(h Hub, rs *Restore) Hub {
	if rs == nil {
		return h
	}
	h = RestoreHub(h, rs.View)
	if rs.Moved && h.Restore != nil {
		h.Restore.Layout, h.Restore.Board, h.Restore.Moved = false, false, true
		if h.Restore.done() {
			h.Restore = nil
		}
	}
	return h
}

// RestoreSingle hands a saved view to the state at the start, as
// RestoreHub does. Every client starts here, before it knows whether the
// agent is relay's hub, so a hub's layout waits too: HubFromSingle takes
// it over.
func RestoreSingle(x Single, v View) Single {
	x.App = restoreItems(x.App, v)
	x.Restore = &Restore{View: v, Layout: v.PaneFocus || v.Hub != nil, Board: v.Card != 0, Drafts: len(v.Drafts) > 0}
	if x.Restore.done() {
		x.Restore = nil
	}
	return x
}

func restoreItems(a App, v View) App {
	if !a.Board.Unavailable {
		a.Board.Items = v.Items
	}
	return a
}

// restoreHub puts back what of the saved view can be put back now.
func (r *hubReducer) restoreHub(at time.Time) {
	h := &r.h
	rs := h.Restore
	if rs == nil {
		return
	}
	if rs.Board {
		if b := &h.App.Board; b.Loaded || b.Unavailable {
			rs.Board = false
			restoreCard(b, rs.View)
		}
	}
	if h.RosterLoaded {
		if rs.Drafts {
			rs.Drafts = false
			r.restoreDrafts(rs.View.Drafts)
		}
		if rs.Layout {
			rs.Layout = false
			r.restoreLayout(rs.View, at)
		}
	}
	if rs.done() {
		h.Restore = nil
	}
}

// restoreCard selects the saved card, and opens it, if it is still on
// the board.
func restoreCard(b *Board, v View) {
	i := slices.IndexFunc(b.Cards, func(c Card) bool { return c.ID == v.Card })
	if !b.Loaded || i < 0 {
		return
	}
	c := b.Cards[i]
	b.SelID, b.Detail = c.ID, v.Detail
	b.Line = min(max(v.Line, 0), c.Lines()-1)
	b.Col = max(min(v.Col, len(c.Columns)-1), 0)
}

// restoreLayout puts back the focus, the list and the pane, for what of
// them is still there.
func (r *hubReducer) restoreLayout(v View, at time.Time) {
	h := &r.h
	hv := v.Hub
	if hv == nil {
		return
	}
	if hv.Focus != "" && h.RoleByID(hv.Focus) != nil {
		// As if picked from the list: a live conversation attaches, and a
		// role whose conversation has ended shows none.
		r.focus(hv.Focus, false, at)
	}
	if h.RoleByID(hv.Cursor) != nil {
		h.Cursor = hv.Cursor
	} else {
		r.clampCursor()
	}
	h.List = hv.List || h.Focus == ""
	// Back in a conversation is typing in it; anywhere else is moving.
	defer func() { h.App.Moving = h.List || h.App.PaneFocus || h.Focus == "" }()
	if c := h.Focused(); !v.PaneFocus || h.Focus != hv.Focus || !h.PaneShown() || (c != nil && len(c.State.Asks) > 0) {
		// The pane had the keys over a role that has gone, or has
		// nothing to show, or a request has the keys.
		return
	}
	h.App.PaneFocus, h.List = true, false
}

// restoreDrafts puts back the drafts of the roles still in the roster:
// their pastes into the role's conversation, or into its blank when it
// has none yet, and their text into the shell's inputs.
func (r *hubReducer) restoreDrafts(ds []Draft) {
	h := &r.h
	lost := 0
	for _, d := range ds {
		if d.Role == "" {
			continue
		}
		if h.RoleByID(d.Role) == nil {
			lost++
			continue
		}
		text, pastes, next := restoredDraft(d)
		if strings.TrimSpace(text) == "" && len(pastes) == 0 {
			continue
		}
		if len(pastes) > 0 {
			var s *State
			if i := r.convIndex(d.Role); i >= 0 {
				s = &h.Convs[i].State
			} else {
				s = &r.blank(d.Role).State
			}
			// Pasted into since the start: the saved pastes take the
			// numbers after the new ones.
			text, pastes, next = afterPastes(text, pastes, next, s.Pastes, s.NextPaste)
			s.Pastes, s.NextPaste = append(slices.Clone(s.Pastes), pastes...), max(next, s.NextPaste)
		}
		r.emit(RestoreDraft{Role: d.Role, Text: text})
	}
	if lost > 0 {
		h.Notice = "a saved draft could not be put back"
		if lost > 1 {
			h.Notice = strconv.Itoa(lost) + " saved drafts could not be put back"
		}
	}
}

// restoreSingle is restoreHub against a single agent: the drafts and
// the pane wait for the session, the card for the board.
func restoreSingle(x Single, at time.Time, fx []Effect) (Single, []Effect) {
	rs := x.Restore
	if rs.Board {
		if b := &x.App.Board; b.Loaded || b.Unavailable {
			rs.Board = false
			restoreCard(b, rs.View)
		}
	}
	if x.App.Live && x.State.Phase != Starting {
		if rs.Drafts {
			rs.Drafts = false
			for _, d := range rs.View.Drafts {
				if d.Role != "" {
					continue
				}
				text, pastes, next := restoredDraft(d)
				if strings.TrimSpace(text) == "" && len(pastes) == 0 {
					continue
				}
				if len(pastes) > 0 {
					text, pastes, next = afterPastes(text, pastes, next, x.State.Pastes, x.State.NextPaste)
					x.State.Pastes, x.State.NextPaste = append(slices.Clone(x.State.Pastes), pastes...), max(next, x.State.NextPaste)
				}
				fx = append(fx, RestoreDraft{Text: text})
			}
		}
		if rs.Layout {
			rs.Layout = false
			if rs.View.PaneFocus && len(x.State.Asks) == 0 && x.State.Phase != Exited {
				x.App.PaneFocus, x.App.Moving = true, true
			}
		}
	}
	if rs.done() {
		x.Restore = nil
	}
	return x, fx
}

// restoredDraft is the draft as it may be put back. A paste can only
// come back as trusted as it was saved, or less, never more:
//
//   - a paste with no number, no text, or a number another paste has is
//     dropped (its placeholder stays as plain text, matching nothing);
//   - a paste whose placeholder is not in the text is dropped, since it
//     could not be sent;
//   - an untrusted paste whose placeholder reads "trusted" gets its
//     untrusted placeholder back, and a trusted one whose placeholder
//     reads "untrusted" becomes untrusted, since what the placeholder
//     says is what is sent.
//
// next is the number the next paste takes, past every paste kept.
func restoredDraft(d Draft) (text string, pastes []Paste, next int) {
	text = d.Text
	next = max(d.NextPaste, 0)
	count := map[int]int{}
	for _, p := range d.Pastes {
		count[p.ID]++
	}
	for _, p := range d.Pastes {
		if p.ID <= 0 || p.Text == "" || count[p.ID] > 1 {
			continue
		}
		next = max(next, p.ID)
		trusted := p
		trusted.Trusted = true
		switch {
		case strings.Contains(text, p.untrustedToken()):
			p.Trusted = false
		case strings.Contains(text, trusted.Token()):
			if !p.Trusted {
				text = strings.ReplaceAll(text, trusted.Token(), p.untrustedToken())
			}
		default:
			continue
		}
		pastes = append(pastes, Paste{ID: p.ID, Text: p.Text, Trusted: p.Trusted})
	}
	return text, pastes, next
}

// referenced is the pastes whose placeholder, either way, is in text:
// the ones a draft needs.
func referenced(ps []Paste, text string) []Paste {
	var out []Paste
	for _, p := range ps {
		t := p
		t.Trusted = true
		if strings.Contains(text, p.untrustedToken()) || strings.Contains(text, t.Token()) {
			out = append(out, p)
		}
	}
	return out
}

// appView is the app's part of a saved view.
func appView(a App) View {
	b := a.Board
	v := View{PaneFocus: a.PaneFocus, Items: b.Items && !b.Unavailable}
	if c, i := BoardCard(b); b.Loaded && i >= 0 {
		v.Card, v.Detail, v.Line, v.Col = c.ID, b.Detail, BoardLine(b, c), max(BoardCol(b, c), 0)
	}
	return v
}

// carry keeps in v the parts of a restore that have not come back yet,
// so a client that closes before the roster or the board arrives does
// not forget them.
func carry(v View, rs *Restore) View {
	if rs == nil {
		return v
	}
	if rs.Layout {
		v.Hub, v.PaneFocus = rs.View.Hub, rs.View.PaneFocus
	}
	if rs.Board {
		v.Card, v.Detail, v.Line, v.Col = rs.View.Card, rs.View.Detail, rs.View.Line, rs.View.Col
	}
	if rs.Drafts {
		for _, d := range rs.View.Drafts {
			if d.Role == "" && v.Hub != nil {
				continue // the hub holds it as Loose, which SaveHub saves
			}
			i := slices.IndexFunc(v.Drafts, func(x Draft) bool { return x.Role == d.Role })
			if i < 0 {
				v.Drafts = append(v.Drafts, d)
				continue
			}
			// Typed since the start: the saved draft goes ahead of it, as
			// it will when it comes back, and neither is lost.
			typed := v.Drafts[i]
			text, pastes, next := restoredDraft(d)
			text, pastes, next = afterPastes(text, pastes, next, typed.Pastes, typed.NextPaste)
			v.Drafts = slices.Clone(v.Drafts)
			v.Drafts[i] = Draft{Role: d.Role, Text: aheadOf(text, typed.Text),
				Pastes: append(slices.Clone(typed.Pastes), pastes...), NextPaste: max(next, typed.NextPaste)}
		}
	}
	return v
}

// aheadOf is saved text put ahead of what has been typed since, as the
// shell puts a restored draft back.
func aheadOf(saved, typed string) string {
	if typed != "" {
		saved += "\n" + typed
	}
	return saved
}

// afterPastes renumbers a saved draft's pastes to follow the ones made
// since (have, numbered up to haveNext), placeholders and all, so the
// two never share a number. Each paste keeps its trust.
func afterPastes(text string, pastes []Paste, next int, have []Paste, haveNext int) (string, []Paste, int) {
	off := haveNext
	for _, p := range have {
		off = max(off, p.ID)
	}
	if off == 0 || len(pastes) == 0 {
		return text, pastes, next
	}
	out := slices.Clone(pastes)
	// Highest first: a paste's new number is above every number not yet
	// moved, so no placeholder is moved twice.
	slices.SortFunc(out, func(a, b Paste) int { return b.ID - a.ID })
	for i, p := range out {
		q := p
		q.ID += off
		for _, trusted := range []bool{false, true} {
			p.Trusted, q.Trusted = trusted, trusted
			text = strings.ReplaceAll(text, p.Token(), q.Token())
		}
		out[i].ID = q.ID
	}
	slices.Reverse(out)
	return text, out, next + off
}

// SaveHub is the saved view of a hub. drafts is the text of each role's
// input, the one in focus included.
func SaveHub(h Hub, drafts map[string]string) View {
	v := appView(h.App)
	v.Hub = &HubView{Focus: h.Focus, Cursor: h.Cursor, List: h.List}
	roles := make([]string, 0, len(drafts))
	for role := range drafts {
		roles = append(roles, role)
	}
	slices.Sort(roles)
	for _, role := range roles {
		text := drafts[role]
		if role == "" || strings.TrimSpace(text) == "" {
			continue
		}
		d := Draft{Role: role, Text: text}
		var s *State
		if c := h.ConvOf(role); c != nil {
			s = &c.State
		} else if i := slices.IndexFunc(h.Blanks, func(b Blank) bool { return b.Role == role }); i >= 0 {
			s = &h.Blanks[i].State
		}
		if s != nil {
			d.Pastes, d.NextPaste = referenced(s.Pastes, text), s.NextPaste
		}
		v.Drafts = append(v.Drafts, d)
	}
	if l := h.Loose; l != nil {
		v.Drafts = append(v.Drafts, Draft{Text: l.Text, Pastes: l.Pastes, NextPaste: l.NextPaste})
	}
	return carry(v, h.Restore)
}

// placeLoose gives a draft typed before any role was in focus to the
// first role that is: its pastes, numbered after the role's own, and its
// text after what the role's input holds.
func (r *hubReducer) placeLoose() {
	h := &r.h
	l := h.Loose
	if l == nil || h.Focus == "" {
		return
	}
	var s *State
	if i := r.convIndex(h.Focus); i >= 0 {
		s = &h.Convs[i].State
	} else {
		s = &r.blank(h.Focus).State
	}
	text, pastes, next := afterPastes(l.Text, l.Pastes, l.NextPaste, s.Pastes, s.NextPaste)
	s.Pastes, s.NextPaste = append(slices.Clone(s.Pastes), pastes...), max(next, s.NextPaste)
	h.Loose = nil
	r.emit(RestoreDraft{Role: h.Focus, Text: text, After: true})
}

// SaveSingle is the saved view against a single agent. draft is the
// input's text.
func SaveSingle(x Single, draft string) View {
	v := appView(x.App)
	if strings.TrimSpace(draft) != "" {
		v.Drafts = []Draft{{Text: draft, Pastes: referenced(x.State.Pastes, draft), NextPaste: x.State.NextPaste}}
	}
	return carry(v, x.Restore)
}
