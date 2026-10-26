package core

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// Ready is the view of what is ready for the operator: what relay told
// her, the drafts it prepared in her accounts, entries on the agent's
// calendar and findings on the board, each with where it came from in
// plain words. It belongs to the connection, like the board, and is
// drawn over the whole screen when it is open (R). The client holds only
// what relay last sent (_relay/ready, then _relay/ready/changed); a reply
// to a message is sent through relay (_relay/reply) and the message
// comes back changed, with the answer on it.

// Item kinds relay sends. Any other kind is drawn the same way: a title,
// its lines and where it came from.
const (
	ReadyMessage = "message"
	ReadyDraft   = "draft"
	ReadyEvent   = "event"
	ReadyFinding = "finding"
	ReadyMove    = "move"
)

// ReadyReply is an answer the operator gave to a message.
type ReadyReply struct{ At, Text string }

// ReadyItem is one thing that is ready, in relay's wire form. Every text
// is already safe to show. From says where it came from, in words, and is
// empty for her own things.
type ReadyItem struct {
	ID   string
	Kind string
	At   string
	// Seq is the log's number of the event that made the item. Items made
	// at the same time are newest first by it.
	Seq      int64
	Title    string
	Lines    []string
	From     string
	Outsider bool
	// Start and End are an event's times, as instants.
	Start, End string
	// Message is a message's number. ReplyTo is the number to answer it
	// under, or 0 when it came from relay itself and there is nobody to
	// answer.
	Message, ReplyTo int64
	MessageKind      string
	Urgency          string
	Replies          []ReadyReply
	Card             int64
	// Move is a file move's number, and Role the role that made it. Undo
	// is the number to undo it under, or 0 when nothing is left to undo;
	// Undone says all of it is back.
	Move   int64
	Role   string
	Undo   int64
	Undone bool
	// Acts are the numbered items of a daily note, each with the number
	// the note shows and what it offers (see ReadyAct).
	Acts []ReadyAct
}

// Ops a numbered item of a daily note offers.
const (
	OpRevert  = "revert"
	OpDismiss = "dismiss"
)

// ReadyAct is one numbered item of a daily note: its own number N in the
// note, what it offers (OpRevert or OpDismiss), the index in the item's
// Lines of the line that begins with the number, and Done, which is
// "reverted" or "dismissed" once it was, else empty.
type ReadyAct struct {
	N    int
	Op   string
	Line int
	Done string
}

// ReplyInput is the reply being written to a message.
type ReplyInput struct {
	Message int64
	Text    string
}

// Ready is the view's state.
type Ready struct {
	// Open says the view is on screen.
	Open bool
	// Asked says _relay/ready has been called; Loaded, that it answered.
	// Unavailable says relay has no such method.
	Asked, Loaded, Unavailable bool
	Rev                        int64
	LoadedRev                  int64 // the rev of the last load; changes up to it are in it
	// Items are newest first. Sel is the selected one, by id.
	Items []ReadyItem
	Sel   string
	// Input is the reply being written. Sending is the message a reply
	// is on its way for, and Sent the reply, so a refusal can give the
	// operator back what they typed.
	Input   *ReplyInput
	Sending int64
	Sent    *ReplyInput
	// Undoing is the move an undo is on its way for. Acting is the
	// message a revert or a dismissal is on its way for.
	Undoing int64
	Acting  int64
	// Detail is the id of the item open whole on its own screen, or empty
	// while the list shows. Top is the first row of it shown, Follow says
	// the screen is to scroll until the picked item shows, and Pick is the
	// index in the item's Acts that v and d act on.
	Detail string
	Top    int
	Follow bool
	Pick   int
	// Help says the list of keys covers the view.
	Help bool
	// Message is the outcome of the last action, and Failure relay's
	// refusal, shown escaped.
	Message string
	Failure string
}

// maxReadyItems bounds what the client holds, newest first.
const maxReadyItems = 200

// Notes the view shows.
const (
	ReplySentNote    = "Sent."
	ReplyRefusedNote = "Not sent."
	ReplyBusyNote    = "A reply is on its way: wait for it."
	NoReplyNote      = "Nothing to answer here."
	UndoneNote       = "Undone."
	UndoRefusedNote  = "Not undone."
	UndoBusyNote     = "An undo is on its way: wait for it."
	NoUndoNote       = "Nothing to undo here."
	NoActNote        = "Nothing to revert or drop here."
	ActBusyNote      = "A change is on its way: wait for it."
	ActRefusedNote   = "Not done."
)

// Events about the view.
type (
	// ReadyLoaded is relay's answer to _relay/ready. Unavailable says
	// relay has no such method; Err, that the call failed another way.
	ReadyLoaded struct {
		At          time.Time
		Rev         int64
		Items       []ReadyItem
		Unavailable bool
		Err         string
	}
	// ReadyChanged is a _relay/ready/changed notification: the items that
	// are new or differ, and the ids of those gone.
	ReadyChanged struct {
		At    time.Time
		Rev   int64
		Items []ReadyItem
		Gone  []string
	}
	// ReplyDone is relay's answer to _relay/reply on Message; Err is
	// relay's refusal.
	ReplyDone struct {
		At      time.Time
		Message int64
		Err     string
	}
	// UndoDone is relay's answer to _relay/undo on Move: the lines relay
	// undo prints and a short Summary for the status line, or Err,
	// relay's refusal with its reason.
	UndoDone struct {
		At      time.Time
		Move    int64
		Lines   []string
		Summary string
		Err     string
	}
	// ActDone is relay's answer to _relay/messages/act on item N of
	// Message, which did Op; Err is relay's refusal.
	ActDone struct {
		At      time.Time
		Message int64
		N       int
		Op      string
		Err     string
	}
	// ReadyOpen is R: the view opens, or closes when it is open.
	ReadyOpen struct{ At time.Time }
	// ReadyKey is a key press in the view.
	ReadyKey struct {
		At     time.Time
		Action ReadyAction
		Text   string // what ReadyChar types, or the digit ReadyPick names
		// Top, Max and Page are the open item as drawn: the first row
		// shown, how far it can scroll and how many rows a page is.
		Top, Max, Page int
	}
)

func (e ReadyLoaded) at() time.Time  { return e.At }
func (e ReadyChanged) at() time.Time { return e.At }
func (e ReplyDone) at() time.Time    { return e.At }
func (e UndoDone) at() time.Time     { return e.At }
func (e ActDone) at() time.Time      { return e.At }
func (e ReadyOpen) at() time.Time    { return e.At }
func (e ReadyKey) at() time.Time     { return e.At }

// ReadyAction is what a key does in the view.
type ReadyAction int

const (
	ReadyOther     ReadyAction = iota // any other key: nothing
	ReadyUp                           // k, up
	ReadyDown                         // j, down
	ReadyTop                          // g, home
	ReadyBottom                       // G, end
	ReadyReplyKey                     // a: write a reply to the selected message
	ReadyEnter                        // enter: write a reply, or send the one written
	ReadyEsc                          // esc: cancel the reply, or close the view
	ReadyRefresh                      // r: ask relay again
	ReadyChar                         // typed into the reply: ReadyKey.Text
	ReadyBackspace                    // delete the reply's last character
	ReadyUndoKey                      // u: undo the selected move
	ReadyOpenKey                      // enter, o: open the selected item whole
	ReadyPageUp                       // page up
	ReadyPageDown                     // page down
	ReadyNext                         // ], tab in an open item: the next numbered item
	ReadyPrev                         // [, shift+tab in an open item: the one before
	ReadyPick                         // a digit: that numbered item (ReadyKey.Text)
	ReadyRevert                       // v: revert the picked item of a daily note
	ReadyDismiss                      // d: drop the picked item of a daily note
	ReadyHelp                         // ?: the list of keys
)

// Effects asking the shell to call relay.
type (
	RequestReady struct{}
	SendReply    struct {
		Message int64
		Text    string
	}
	// SendUndo asks relay to undo a file move (_relay/undo), as relay
	// undo does.
	SendUndo struct{ Move int64 }
	// SendAct asks relay to do what item N of daily note Message offers
	// (_relay/messages/act): revert a change, or drop a suggestion. Op is
	// what the item offered, kept for the answer's wording.
	SendAct struct {
		Message int64
		N       int
		Op      string
	}
)

func (RequestReady) effect() {}
func (SendReply) effect()    {}
func (SendUndo) effect()     {}
func (SendAct) effect()      {}

// ReadyCount is how many items the view holds.
func ReadyCount(h Hub) int { return len(h.Ready.Items) }

// ReadySelected is the selected item and its index, or -1.
func ReadySelected(r Ready) (ReadyItem, int) {
	if len(r.Items) == 0 {
		return ReadyItem{}, -1
	}
	for i, it := range r.Items {
		if it.ID == r.Sel {
			return it, i
		}
	}
	return r.Items[0], 0
}

// readyBefore orders items newest first; at the same time, the later in
// relay's log first (Seq); then by id.
func readyBefore(a, b ReadyItem) int {
	ta, ea := time.Parse(time.RFC3339Nano, a.At)
	tb, eb := time.Parse(time.RFC3339Nano, b.At)
	switch {
	case ea == nil && eb == nil && !ta.Equal(tb):
		if ta.After(tb) {
			return -1
		}
		return 1
	case ea == nil && eb != nil:
		return -1
	case ea != nil && eb == nil:
		return 1
	case a.Seq != b.Seq:
		if a.Seq > b.Seq {
			return -1
		}
		return 1
	}
	return strings.Compare(a.ID, b.ID)
}

// sortReady puts items newest first and drops the oldest past the bound.
func sortReady(items []ReadyItem) []ReadyItem {
	items = slices.Clone(items)
	slices.SortStableFunc(items, readyBefore)
	if len(items) > maxReadyItems {
		items = items[:maxReadyItems]
	}
	return items
}

// readyOnly keeps the first item of each id.
func readyOnly(items []ReadyItem) []ReadyItem {
	var out []ReadyItem
	for _, it := range items {
		if it.ID != "" && !slices.ContainsFunc(out, func(x ReadyItem) bool { return x.ID == it.ID }) {
			out = append(out, it)
		}
	}
	return out
}

// keepReadySelection keeps the selected item selected, or moves to the
// nearest one when it has gone. was is where it was.
func (r *hubReducer) keepReadySelection(was int) {
	rd := &r.h.Ready
	if len(rd.Items) == 0 {
		rd.Sel = ""
		return
	}
	if slices.ContainsFunc(rd.Items, func(it ReadyItem) bool { return it.ID == rd.Sel }) {
		return
	}
	rd.Sel = rd.Items[min(max(was, 0), len(rd.Items)-1)].ID
}

// askReady asks relay for the view once the connection can take it, so
// that relay sends the changes after. It asks again only after a failure
// or a refresh.
func (r *hubReducer) askReady() {
	rd := &r.h.Ready
	if !rd.Asked && !rd.Unavailable && r.h.App.Live {
		rd.Asked = true
		r.emit(RequestReady{})
	}
}

func (r *hubReducer) ready(e Event) {
	h := &r.h
	rd := &h.Ready
	rd.Items = slices.Clone(rd.Items)
	_, was := ReadySelected(*rd)
	switch e := e.(type) {
	case ReadyLoaded:
		switch {
		case e.Unavailable:
			rd.Unavailable, rd.Loaded, rd.Items = true, false, nil
		case e.Err != "":
			rd.Asked = false // opening it asks again
			rd.Message, rd.Failure = "What is ready did not load.", e.Err
			return
		case e.Rev < rd.Rev:
			// A change newer than this answer got here first, and relay
			// counts it as sent. Taking the answer would undo it, so ask
			// again for one that is current.
			rd.Asked = false
			r.askReady()
			return
		default:
			rd.Loaded, rd.Unavailable = true, false
			rd.Items = sortReady(readyOnly(e.Items))
			rd.Rev = max(rd.Rev, e.Rev)
			rd.LoadedRev = e.Rev
		}
		r.keepReadySelection(was)
		r.keepReadyDetail()
	case ReadyChanged:
		if rd.Loaded && e.Rev <= rd.LoadedRev {
			return // the load already holds it
		}
		items := slices.DeleteFunc(slices.Clone(rd.Items), func(it ReadyItem) bool { return slices.Contains(e.Gone, it.ID) })
		for _, n := range readyOnly(e.Items) {
			if i := slices.IndexFunc(items, func(it ReadyItem) bool { return it.ID == n.ID }); i >= 0 {
				items[i] = n
			} else {
				items = append(items, n)
			}
		}
		rd.Items = sortReady(items)
		rd.Rev = max(rd.Rev, e.Rev)
		r.keepReadySelection(was)
		r.keepReadyDetail()
	case ReplyDone:
		sent := rd.Sent
		if rd.Sending == e.Message {
			rd.Sending, rd.Sent = 0, nil
		}
		if e.Err != "" {
			rd.Message, rd.Failure = ReplyRefusedNote, e.Err
			// What the operator typed comes back, unless they have since
			// begun another reply.
			if sent != nil && rd.Input == nil {
				in := *sent
				rd.Input = &in
			}
			return
		}
		rd.Message, rd.Failure = ReplySentNote, ""
	case UndoDone:
		if rd.Undoing == e.Move {
			rd.Undoing = 0
		}
		if e.Err != "" {
			rd.Message, rd.Failure = UndoRefusedNote, e.Err
			return
		}
		rd.Message, rd.Failure = UndoneNote, ""
		switch {
		case e.Summary != "":
			rd.Message = e.Summary
		case len(e.Lines) > 0:
			rd.Message = e.Lines[0]
		}
	case ActDone:
		if rd.Acting == e.Message {
			rd.Acting = 0
		}
		if e.Err != "" {
			rd.Message, rd.Failure = ActRefusedNote, e.Err
			return
		}
		done := "reverted"
		rd.Message, rd.Failure = fmt.Sprintf("Item %d undone.", e.N), ""
		if e.Op == OpDismiss {
			done = "dismissed"
			rd.Message = fmt.Sprintf("Item %d dropped.", e.N)
		}
		// Relay sends the note changed too; this shows it at once.
		for i, it := range rd.Items {
			if it.Message != e.Message {
				continue
			}
			acts := slices.Clone(it.Acts)
			for j := range acts {
				if acts[j].N == e.N {
					acts[j].Done = done
				}
			}
			rd.Items[i].Acts = acts
		}
	case ReadyOpen:
		if rd.Open {
			rd.Open, rd.Input, rd.Help = false, nil, false
			rd.Detail, rd.Top, rd.Follow = "", 0, false
			rd.Message, rd.Failure = "", ""
			return
		}
		rd.Open = true
		r.askReady()
		if _, i := ReadySelected(*rd); i >= 0 {
			rd.Sel = rd.Items[i].ID
		}
	case ReadyKey:
		r.readyKey(e)
	}
}

// readyKey applies a key in the view.
func (r *hubReducer) readyKey(e ReadyKey) {
	h := &r.h
	rd := &h.Ready
	if rd.Input != nil {
		in := *rd.Input
		// A refusal stays while the words are on screen, and goes with
		// the next key.
		if e.Action == ReadyChar || e.Action == ReadyBackspace || e.Action == ReadyEsc {
			rd.Message, rd.Failure = "", ""
		}
		switch e.Action {
		case ReadyChar:
			in.Text += e.Text
			rd.Input = &in
		case ReadyBackspace:
			if rs := []rune(in.Text); len(rs) > 0 {
				in.Text = string(rs[:len(rs)-1])
			}
			rd.Input = &in
		case ReadyEsc:
			rd.Input = nil
		case ReadyEnter:
			text := strings.TrimSpace(in.Text)
			if text == "" {
				return
			}
			if rd.Sending != 0 {
				rd.Message, rd.Failure = ReplyBusyNote, ""
				return
			}
			rd.Input, rd.Sending = nil, in.Message
			rd.Sent = &ReplyInput{Message: in.Message, Text: in.Text}
			rd.Message, rd.Failure = "", ""
			r.emit(SendReply{Message: in.Message, Text: text})
		}
		return
	}
	// The list of keys covers the view; any key closes it and does only that.
	if rd.Help {
		rd.Help = false
		return
	}
	rd.Message, rd.Failure = "", ""
	if rd.Detail != "" {
		r.detailKey(e)
		return
	}
	it, i := ReadySelected(*rd)
	move := func(to int) {
		if len(rd.Items) > 0 {
			rd.Sel = rd.Items[min(max(to, 0), len(rd.Items)-1)].ID
		}
	}
	switch e.Action {
	case ReadyUp, ReadyPrev:
		move(i - 1)
	case ReadyDown, ReadyNext:
		move(i + 1)
	case ReadyTop:
		move(0)
	case ReadyBottom:
		move(len(rd.Items) - 1)
	case ReadyRefresh:
		rd.Asked = false
		r.askReady()
	case ReadyHelp:
		rd.Help = true
	case ReadyEsc:
		rd.Open = false
	case ReadyOpenKey, ReadyEnter:
		if i >= 0 {
			rd.Detail, rd.Top, rd.Follow, rd.Pick = it.ID, 0, false, firstOpenAct(it)
		}
	case ReadyUndoKey:
		r.undo(it, i)
	case ReadyReplyKey:
		r.reply(it, i)
	case ReadyRevert, ReadyDismiss:
		rd.Message = OpenNoteNote
	}
}

func (r *hubReducer) undo(it ReadyItem, i int) {
	rd := &r.h.Ready
	switch {
	case i < 0 || it.Undo == 0:
		rd.Message = NoUndoNote
	case rd.Undoing != 0:
		rd.Message = UndoBusyNote
	default:
		rd.Undoing = it.Undo
		r.emit(SendUndo{Move: it.Undo})
	}
}

func (r *hubReducer) reply(it ReadyItem, i int) {
	rd := &r.h.Ready
	switch {
	case i < 0 || it.ReplyTo == 0:
		rd.Message = NoReplyNote
	case rd.Sending != 0:
		rd.Message = ReplyBusyNote
	default:
		rd.Input = &ReplyInput{Message: it.ReplyTo}
	}
}
