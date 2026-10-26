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

// OpenNoteNote is what v and d say in the list: they act on a numbered
// item, which shows when the note is open.
const OpenNoteNote = "Open the note first (enter), then v or d on an item."

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
	case ReadyRevert, ReadyDismiss:
		r.act(it, e.Action)
	}
}

// act sends the picked numbered item of a daily note, when the key is the
// one for what the item offers.
func (r *hubReducer) act(it ReadyItem, key ReadyAction) {
	rd := &r.h.Ready
	if len(it.Acts) == 0 || rd.Pick >= len(it.Acts) {
		rd.Message = NoActNote
		return
	}
	a := it.Acts[rd.Pick]
	want, other := OpRevert, "d"
	if key == ReadyDismiss {
		want, other = OpDismiss, "v"
	}
	switch {
	case a.Done == "reverted":
		rd.Message = fmt.Sprintf("Item %d is undone already.", a.N)
	case a.Done != "":
		rd.Message = fmt.Sprintf("Item %d is dropped already.", a.N)
	case a.Op != want:
		rd.Message = fmt.Sprintf("Item %d: press %s.", a.N, map[string]string{"v": "v to undo it", "d": "d to drop it"}[other])
	case rd.Acting != 0:
		rd.Message = ActBusyNote
	default:
		rd.Acting = it.Message
		r.emit(SendAct{Message: it.Message, N: a.N, Op: a.Op})
	}
}
