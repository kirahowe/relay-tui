package core

import (
	"fmt"
	"slices"
	"strconv"
)

// An item open whole on its own screen: scrolling it, picking a numbered
// item of a daily note, and acting on the one picked.

// ReadyDetail is the item open whole, and its index, or -1 when the list
// shows or the item is gone.
func ReadyDetail(r Ready) (ReadyItem, int) {
	if r.Detail == "" {
		return ReadyItem{}, -1
	}
	for i, it := range r.Items {
		if it.ID == r.Detail {
			return it, i
		}
	}
	return ReadyItem{}, -1
}

// keepReadyDetail closes the open item once it has gone, and keeps the
// picked numbered item in range.
func (r *hubReducer) keepReadyDetail() {
	rd := &r.h.Ready
	if rd.Detail == "" {
		return
	}
	it, i := ReadyDetail(*rd)
	if i < 0 {
		rd.Detail, rd.Top, rd.Follow, rd.Input = "", 0, false, nil
		return
	}
	rd.Pick = min(max(rd.Pick, 0), max(len(it.Acts)-1, 0))
}

// OpenNoteNote is what v, n and d say in the list: they act on a numbered
// item, which shows when the note is open.
const OpenNoteNote = "Open the note first (enter or its number), then v, n or d on an item."

// firstOpenAct is the index of the first numbered item not done yet, or 0.
func firstOpenAct(it ReadyItem) int {
	for i, a := range it.Acts {
		if a.Done == "" {
			return i
		}
	}
	return 0
}

// detailKey applies a key while an item is open whole.
func (r *hubReducer) detailKey(e ReadyKey) {
	rd := &r.h.Ready
	it, i := ReadyDetail(*rd)
	if i < 0 {
		rd.Detail = ""
		return
	}
	scroll := func(to int) {
		rd.Top, rd.Follow = min(max(to, 0), max(e.Max, 0)), false
	}
	pick := func(to int) {
		if len(it.Acts) == 0 {
			return
		}
		rd.Pick = min(max(to, 0), len(it.Acts)-1)
		rd.Top, rd.Follow = e.Top, true
	}
	switch e.Action {
	case ReadyUp:
		scroll(e.Top - 1)
	case ReadyDown:
		scroll(e.Top + 1)
	case ReadyPageUp:
		scroll(e.Top - max(e.Page, 1))
	case ReadyPageDown:
		scroll(e.Top + max(e.Page, 1))
	case ReadyTop:
		scroll(0)
	case ReadyBottom:
		scroll(e.Max)
	case ReadyNext:
		pick(rd.Pick + 1)
	case ReadyPrev:
		pick(rd.Pick - 1)
	case ReadyPick:
		n, err := strconv.Atoi(e.Text)
		idx := slices.IndexFunc(it.Acts, func(a ReadyAct) bool { return err == nil && a.N == n })
		if idx < 0 {
			rd.Message = fmt.Sprintf("There is no item %s here.", e.Text)
			return
		}
		pick(idx)
	case ReadyRefresh:
		rd.Asked = false
		r.askReady()
	case ReadyHelp:
		rd.Help = true
	case ReadyEsc:
		rd.Detail, rd.Top, rd.Follow = "", 0, false
	case ReadyUndoKey:
		r.undo(it, i)
	case ReadyReplyKey:
		r.reply(it, i)
	case ReadyRevert, ReadyDismiss, ReadyDecline:
		r.act(it, e.Action)
	}
}

// act sends the picked numbered item of a daily note, when the key fits
// what the item offers: v reverts a change, d drops a suggestion, and n
// says no to a suggestion's offer, keeping it.
func (r *hubReducer) act(it ReadyItem, key ReadyAction) {
	rd := &r.h.Ready
	if len(it.Acts) == 0 || rd.Pick >= len(it.Acts) {
		rd.Message = NoActNote
		return
	}
	a := it.Acts[rd.Pick]
	op := map[ReadyAction]string{ReadyRevert: OpRevert, ReadyDismiss: OpDismiss, ReadyDecline: OpDecline}[key]
	switch {
	case a.Done == DoneReverted:
		rd.Message = fmt.Sprintf("Item %d is undone already.", a.N)
	case a.Done == DoneDismissed:
		rd.Message = fmt.Sprintf("Item %d is dropped already.", a.N)
	case a.Done == DoneDeclined && op != OpDismiss:
		rd.Message = fmt.Sprintf("Item %d: its offer was turned down already. d drops it.", a.N)
	case a.Op == OpRevert && op != OpRevert:
		rd.Message = fmt.Sprintf("Item %d is a change: press v to undo it.", a.N)
	case a.Op != OpRevert && op == OpRevert:
		rd.Message = fmt.Sprintf("Item %d: press n to say no to its offer, or d to drop it.", a.N)
	case op == OpDecline && !a.Offer:
		rd.Message = fmt.Sprintf("Item %d has no offer: press d to drop it.", a.N)
	case rd.Acting != 0:
		rd.Message = ActBusyNote
	default:
		rd.Acting = it.Message
		r.emit(SendAct{Message: it.Message, N: a.N, Op: op})
	}
}
