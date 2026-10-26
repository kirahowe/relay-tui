package core

import (
	"regexp"
	"slices"
	"strings"
	"time"
)

// The workspace pane shows what a tool showed the operator (a display) and
// what the latest reply cites. Everything in it comes from data relay
// sent. Model text is only searched for reference strings.

// Source is a document a shown item draws on.
type Source struct{ Title, Ref string }

// Item is one thing the pane shows: what a tool showed the operator.
type Item struct {
	Ref     string // shown:2026-10-18/toolu_01abc, when relay knows it
	Path    string // the doc's path in relay's data, when relay says
	CallID  string
	At      string
	Title   string
	Text    string
	Sources []Source
	// Labels are where its text came from (see Provenance).
	Labels Labels
}

// Key names an item so a selection can follow it when the list changes.
func (it Item) Key() string {
	switch {
	case it.Ref != "":
		return "s|" + it.Ref
	case it.Path != "":
		return "s|" + it.Path
	}
	return "s|" + it.CallID + "|" + it.Title
}

// Display is what a tool call carried for the operator in
// _meta["relay/display"].
type Display struct {
	Title, Body string
	Labels      Labels
	Sources     []Source
	Ref         string
}

// Citation is a reference string from the latest reply and what relay
// answered when asked about it. An empty Status means unchecked: the
// answer is pending, or relay cannot be asked.
type Citation struct {
	Ref    string // [[kind:id]] as written
	Kind   string
	Status string // "", "ok", "missing" or "external"
	Title  string
	Text   string
	Target string
	Labels Labels
}

// Citation statuses.
const (
	CiteOK       = "ok"
	CiteMissing  = "missing"
	CiteExternal = "external"
)

// Pane is a conversation's part of the workspace pane: its items, and
// where the operator is among them. The board, which view the pane shows
// and whether it has the keys are the app's (App).
type Pane struct {
	// Relay says relay answered _relay/items: Server is then the whole
	// list. Unavailable says it did not, so the pane falls back to Local,
	// which holds what the client saw itself.
	Relay       bool
	Unavailable bool
	Server      []Item
	Local       []Item // oldest first
	Cites       []Citation

	Detail bool // the selected item fills the pane
	Sel    int
	SelKey string
	Scroll int // first line shown of the detail view
}

// PaneItems is the items the pane lists, newest first.
func PaneItems(s State) []Item {
	p := s.Pane
	if p.Relay {
		return p.Server
	}
	out := make([]Item, len(p.Local))
	for i, it := range p.Local {
		out[len(out)-1-i] = it
	}
	return out
}

// SelectedIndex is the index in PaneItems of the selected item, or -1.
func SelectedIndex(s State) int {
	items := PaneItems(s)
	if len(items) == 0 {
		return -1
	}
	if k := s.Pane.SelKey; k != "" {
		for i, it := range items {
			if it.Key() == k {
				return i
			}
		}
	}
	return min(max(s.Pane.Sel, 0), len(items)-1)
}

// Events about the pane.
type (
	// ItemsLoaded is relay's answer to _relay/items. Unavailable says the
	// agent does not have the method; Failed, that the call failed some
	// other way and the list should stay as it is.
	ItemsLoaded struct {
		At                  time.Time
		Items               []Item
		Unavailable, Failed bool
	}
	// CitationResolved is relay's answer to _relay/resolve for Cite.Ref.
	// Failed leaves the citation unchecked.
	CitationResolved struct {
		At     time.Time
		Cite   Citation
		Failed bool
	}
	// PaneKey is a key press for the pane. Max is, for scroll keys, the
	// furthest the detail view can scroll, which the shell takes from the
	// renderer.
	PaneKey struct {
		At     time.Time
		Action PaneAction
		Max    int
		Text   string // what PaneChar types
	}
)

func (e ItemsLoaded) at() time.Time      { return e.At }
func (e CitationResolved) at() time.Time { return e.At }
func (e PaneKey) at() time.Time          { return e.At }

// PaneAction is what a key does in the pane.
type PaneAction int

const (
	PaneOther  PaneAction = iota // any key without a meaning cancels the archive question
	PaneToggle                   // h or l into or out of the pane (Move): focus it, or leave it
	PaneUp
	PaneDown
	PagePrev
	PageNext
	PaneEnter
	PaneEsc
	PaneYes // y
	// The board's keys.
	PaneBoard      // b: switch between the items and the board
	PaneRefresh    // r: ask relay for the board again
	PaneLeft       // the previous tracker column
	PaneRight      // the next tracker column
	PaneMark       // tick an item, or move a tracker cell on
	PaneAdd        // add an item or a row
	PaneAddColumn  // add a tracker column
	PaneEdit       // edit the selected line's text
	PaneEditColumn // rename the selected tracker column
	PaneArchive    // archive the selected card, once confirmed
	PaneChar       // typed into the board's input: PaneKey.Text
	PaneBackspace  // delete the input's last character
)

// scrollPage is how far a page key scrolls the detail view.
const scrollPage = 10

// Effects asking the shell to call relay.
type (
	RequestItems   struct{}
	RequestResolve struct{ Ref string }
)

func (RequestItems) effect()   {}
func (RequestResolve) effect() {}

// refRE finds a reference in model text: [[kind:id]].
var refRE = regexp.MustCompile(`\[\[(fact|doc|note|memo|journal|rem|file|web|shown|report):([^\[\]\n]+)\]\]`)

// ParseRefs is the distinct references in text, in order of first
// appearance, as written.
func ParseRefs(text string) []string {
	var out []string
	for _, m := range refRE.FindAllString(text, -1) {
		if !slices.Contains(out, m) {
			out = append(out, m)
		}
	}
	return out
}

// latestRefs is the references in the latest assistant message: the
// entries of the last agent entry's message id, or, without an id, the
// run of id-less agent entries it ends.
func latestRefs(entries []Entry) []string {
	j := -1
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Kind == Agent {
			j = i
			break
		}
	}
	if j < 0 {
		return nil
	}
	id := entries[j].ID
	var parts []string
	for i := j; i >= 0; i-- {
		e := entries[i]
		if id == "" && (e.Kind != Agent || e.ID != "") {
			break
		}
		if e.Kind == Agent && e.ID == id {
			parts = append(parts, e.Text)
		}
	}
	slices.Reverse(parts)
	return ParseRefs(strings.Join(parts, "\n"))
}

// LatestRefs is the references in the latest assistant message of s.
func LatestRefs(s State) []string { return latestRefs(s.Entries) }

// Cite is what the pane knows about ref: its citation, or an unchecked one.
func (p Pane) Cite(ref string) Citation {
	for _, c := range p.Cites {
		if c.Ref == ref {
			return c
		}
	}
	return Citation{Ref: ref}
}

// refresh asks relay for the item list, unless it is known not to have one.
func (r *reducer) refresh() {
	if !r.s.Pane.Unavailable {
		r.emit(RequestItems{})
	}
}

// displays records what a tool call showed the operator.
func (r *reducer) displays(callID string, ds []Display) {
	if len(ds) == 0 {
		return
	}
	for _, d := range ds {
		it := Item{CallID: callID, Ref: d.Ref, Title: d.Title, Text: d.Body,
			Sources: d.Sources, Labels: d.Labels}
		r.putLocal(it)
	}
	r.showItems()
	r.refresh()
}

// putLocal adds an item to the client's own list, or replaces the one
// with its key.
func (r *reducer) putLocal(it Item) {
	p := &r.s.Pane
	for i := range p.Local {
		if p.Local[i].Key() == it.Key() {
			p.Local[i] = it
			return
		}
	}
	p.Local = append(p.Local, it)
}

// showItems brings the workspace items into view for something relay
// just put there, unless the operator is working in the pane.
func (r *reducer) showItems() {
	p := &r.s.Pane
	if !r.shown || r.a.PaneFocus || r.a.Board.Items {
		return
	}
	r.a.Board.Items, p.Detail, p.Scroll = true, false, 0
}

// citeLatest asks relay about each reference in the latest reply that it
// has not been asked about.
func (r *reducer) citeLatest() {
	p := &r.s.Pane
	if p.Unavailable {
		return
	}
	for _, ref := range latestRefs(r.s.Entries) {
		if slices.ContainsFunc(p.Cites, func(c Citation) bool { return c.Ref == ref }) {
			continue
		}
		p.Cites = append(p.Cites, Citation{Ref: ref})
		r.emit(RequestResolve{Ref: ref})
	}
}

func (r *reducer) pane(e Event) {
	s := &r.s
	p := &s.Pane
	switch e := e.(type) {
	case ItemsLoaded:
		switch {
		case e.Unavailable:
			p.Unavailable, p.Relay, p.Server = true, false, nil
		case e.Failed:
			return
		default:
			p.Relay = true
			p.Server = append([]Item(nil), e.Items...)
		}
		r.keepSelection()
	case CitationResolved:
		if e.Failed {
			return
		}
		for i := range p.Cites {
			if p.Cites[i].Ref == e.Cite.Ref {
				c := e.Cite
				c.Ref = p.Cites[i].Ref
				p.Cites[i] = c
			}
		}
	case PaneKey:
		r.paneKey(e)
	case CardsLoaded, CardsChanged, CardOpDone:
		r.board(e)
	}
}

// keepSelection keeps the same item selected when the list changes.
func (r *reducer) keepSelection() {
	p := &r.s.Pane
	items := PaneItems(r.s)
	i := SelectedIndex(r.s)
	if i < 0 {
		p.Sel, p.SelKey, p.Detail, p.Scroll = 0, "", false, 0
		return
	}
	p.Sel = i
	if p.SelKey != "" && items[i].Key() != p.SelKey {
		p.SelKey = items[i].Key() // it went away
	}
}

func (r *reducer) selectIndex(i int) {
	p := &r.s.Pane
	items := PaneItems(r.s)
	if len(items) == 0 {
		p.Sel, p.SelKey = 0, ""
		return
	}
	i = min(max(i, 0), len(items)-1)
	p.Sel, p.SelKey = i, items[i].Key()
}

func (r *reducer) paneKey(e PaneKey) {
	s := &r.s
	p := &s.Pane
	if e.Action == PaneToggle {
		b := &r.a.Board
		b.Confirm, b.Message, b.Failure = 0, "", ""
		r.a.PaneFocus = !r.a.PaneFocus
		if r.a.PaneFocus {
			r.a.Moving = true // the pane is never typed in
			r.selectIndex(max(SelectedIndex(*s), 0))
		} else {
			p.Detail, p.Scroll = false, 0
		}
		return
	}
	if !r.a.PaneFocus {
		return
	}
	if !r.a.Board.Items {
		r.boardKey(e)
		return
	}
	if e.Action == PaneBoard {
		r.toggleBoard()
		return
	}
	i := SelectedIndex(*s)
	switch e.Action {
	case PaneUp, PaneDown:
		d := 1
		if e.Action == PaneUp {
			d = -1
		}
		if p.Detail {
			p.Scroll = min(max(p.Scroll+d, 0), max(e.Max, 0))
		} else if i >= 0 {
			r.selectIndex(i + d)
		}
	case PagePrev, PageNext:
		if p.Detail {
			d := scrollPage
			if e.Action == PagePrev {
				d = -scrollPage
			}
			p.Scroll = min(max(p.Scroll+d, 0), max(e.Max, 0))
		}
	case PaneEnter:
		if i >= 0 && !p.Detail {
			p.Detail, p.Scroll = true, 0
		}
	case PaneEsc:
		if p.Detail {
			p.Detail, p.Scroll = false, 0
		} else {
			r.a.PaneFocus = false
		}
	}
}

// PaneSideKeys says the pane takes left and right (h and l) for itself:
// a tracker card is open on the board, and they move between its
// columns. Esc closes the card, and they move between columns again.
func PaneSideKeys(a App) bool {
	b := a.Board
	if b.Items || !b.Detail {
		return false
	}
	c, i := BoardCard(b)
	return i >= 0 && c.Kind == CardTracker && len(c.Columns) > 0
}
