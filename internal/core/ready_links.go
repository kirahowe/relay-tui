package core

// The list of the links in a ready item's outside text, open over the
// ready view: L opens it for the item open whole, or else the selected
// one. A broken link can be kept and a live one dropped, each after one
// question. relay does both (SendLink), and sends the item again as it
// then reads.

// ReadyLinks is the item the list of links is open for and its links, as
// they stand. ok is false when no list is open or its item has gone.
func ReadyLinks(r Ready, live Links) (it ReadyItem, links []ShownLink, ok bool) {
	if r.Links == nil {
		return ReadyItem{}, nil, false
	}
	for _, it := range r.Items {
		if it.ID == r.Links.Item {
			return it, ReadyItemLinks(it, live), true
		}
	}
	return ReadyItem{}, nil, false
}

// ReadyLinksItem is the item L would list the links of: the one open
// whole, or else the selected one. ok is false when there is none.
func ReadyLinksItem(r Ready) (ReadyItem, bool) {
	if it, i := ReadyDetail(r); i >= 0 {
		return it, true
	}
	if r.Detail != "" {
		return ReadyItem{}, false
	}
	it, i := ReadySelected(r)
	return it, i >= 0
}

// openReadyLinks opens the list for the item in front of the operator,
// on its first broken link, or says it has no link from outside.
func (r *hubReducer) openReadyLinks() {
	rd := &r.h.Ready
	it, ok := ReadyLinksItem(*rd)
	links := ReadyItemLinks(it, r.h.App.Links)
	if !ok || len(links) == 0 {
		rd.Message = NoLinksNote
		return
	}
	rd.Links = &LinkList{Item: it.ID, Sel: firstBroken(links), Sending: rd.LinkOut}
	rd.LinkOut = nil
}

// closeLinks closes the list of links. A keep or a drop still on its way
// to relay is kept, so relay's answer is said when it comes.
func (rd *Ready) closeLinks() {
	if out := rd.Links.closed(); out != nil {
		rd.LinkOut = out
	}
	rd.Links = nil
}

// readyLinksKey applies a key while the list of links is open.
func (r *hubReducer) readyLinksKey(e ReadyKey) {
	rd := &r.h.Ready
	_, links, ok := ReadyLinks(*rd, r.h.App.Links)
	if !ok {
		rd.closeLinks()
		return
	}
	act, delta := linkNothing, 0
	asked := rd.Links.Ask != nil
	switch e.Action {
	case ReadyUp, ReadyPrev:
		act, delta = linkMove, -1
	case ReadyDown, ReadyNext:
		act, delta = linkMove, 1
	case ReadyPageUp:
		act, delta = linkMove, -scrollPage
	case ReadyPageDown:
		act, delta = linkMove, scrollPage
	case ReadyTop:
		act = linkTop
	case ReadyBottom:
		act = linkBottom
	case ReadyOpenKey, ReadyEnter:
		act = linkKeep
	case ReadyDismiss:
		act = linkDrop
	case ReadyYes:
		act = linkYes
	case ReadyEsc, ReadyLinksKey:
		if !asked {
			rd.closeLinks()
			return
		}
	case ReadyHelp:
		if !asked {
			rd.Help = true
			return
		}
	case ReadyRefresh:
		if !asked {
			rd.Asked = false
			r.withApp(func(a *reducer) { a.askLinks() })
			r.askReady()
			return
		}
	}
	if asked && act != linkYes {
		act = linkNo
	}
	next, msg, send := rd.Links.step(links, act, delta)
	rd.Links = &next
	rd.Message = msg
	if send != nil {
		r.emit(SendLink{Address: send.Sends(), Keep: send.Keep})
	}
}

// keepReadyLinks keeps the list in step with the view: it closes when
// its item has gone, and its question is dropped when the link it was
// asked about is no longer there as it was.
func (r *hubReducer) keepReadyLinks() {
	rd := &r.h.Ready
	if rd.Links == nil {
		return
	}
	_, links, ok := ReadyLinks(*rd, r.h.App.Links)
	if !ok {
		rd.closeLinks()
		return
	}
	next, cancelled := rd.Links.kept(links)
	rd.Links = &next
	if cancelled {
		rd.Message, rd.Failure = LinkChangedNote, ""
	}
}
