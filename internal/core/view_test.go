package core

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// The saved view: what is derived from state, and how a loaded one comes
// back as what it refers to arrives.

func viewRoster() []Role { return []Role{role("dev", "s1"), role("mail", ""), role("news", "")} }

func viewCards() []Card { return []Card{checklist(10, 10, false, false, false), tracker(20, 20)} }

// leftHub is a hub as the operator left it: Mail in focus with no
// conversation, a paste trusted and one not in its draft, the pane with
// the keys, the tracker open on its second row and second column, and
// Dev's conversation attached with a draft of its own.
func leftHub(t *testing.T) (Hub, map[string]string) {
	t.Helper()
	h, _ := apply(t, NewHub(""), HubConnected{At: t0}, RosterUpdate{At: t0, Roles: viewRoster()},
		CardsLoaded{At: t0, Rev: 30, Cards: viewCards()},
		FocusIndex{At: t0, Index: 0}, Type{At: t0}, Attached{At: t0, Role: "dev", Session: "s1"},
		Pasted{At: t0, Text: "dev paste"},
		FocusIndex{At: t0, Index: 1},
		Pasted{At: t0, Text: "an email\nfrom outside"}, Pasted{At: t0, Text: "my own notes"})
	b := h.DraftOf("mail")
	h, _ = apply(t, h, TrustPaste{At: t0.Add(ArmDelay), Draft: b.Pastes[1].Token()})
	b = h.DraftOf("mail")
	h, _ = apply(t, h, PaneKey{At: t0, Action: PaneToggle}, PaneKey{At: t0, Action: PaneDown}, PaneKey{At: t0, Action: PaneEnter},
		PaneKey{At: t0, Action: PaneDown}, PaneKey{At: t0, Action: PaneRight})
	devPaste := h.DraftOf("dev").Pastes[0]
	drafts := map[string]string{
		"mail": "see " + b.Pastes[0].Token() + " and " + b.Pastes[1].Token(),
		"dev":  "hello " + devPaste.Token(),
		"news": "  ", // nothing worth keeping
	}
	return h, drafts
}

func TestSaveHubDerivesWhereTheOperatorWas(t *testing.T) {
	h, drafts := leftHub(t)
	v := saveHub(h, drafts)
	want := HubView{Focus: "mail", Cursor: "mail"}
	if v.Hub == nil || *v.Hub != want || !v.PaneFocus || v.Items {
		t.Fatalf("layout %+v pane %v items %v", v.Hub, v.PaneFocus, v.Items)
	}
	if v.Card != 20 || !v.Detail || v.Line != 1 || v.Col != 1 {
		t.Fatalf("card %d detail %v line %d col %d", v.Card, v.Detail, v.Line, v.Col)
	}
	if len(v.Drafts) != 2 || v.Drafts[0].Role != "dev" || v.Drafts[1].Role != "mail" {
		t.Fatalf("drafts %+v", v.Drafts)
	}
	mail := v.Drafts[1]
	if len(mail.Pastes) != 2 || mail.Pastes[0].Trusted || !mail.Pastes[1].Trusted || mail.NextPaste != 2 {
		t.Fatalf("mail's pastes %+v", mail)
	}
	// A paste whose placeholder is gone from the draft is not saved.
	v = saveHub(h, map[string]string{"mail": "only " + mail.Pastes[1].Token()})
	if ps := v.Drafts[0].Pastes; len(ps) != 1 || ps[0].ID != 2 {
		t.Fatalf("pastes %+v", ps)
	}
}

// reopen is a fresh hub given v, connected, with roster and board as
// given.
func reopen(t *testing.T, v View, roles []Role, cards []Card) (Hub, []Effect) {
	t.Helper()
	return apply(t, RestoreHub(NewHub(""), v), HubConnected{At: t0}, RosterUpdate{At: t0, Roles: roles},
		CardsLoaded{At: t0, Rev: 30, Cards: cards})
}

func restoredDrafts(fx []Effect) map[string]string {
	out := map[string]string{}
	for _, d := range effectsOf[RestoreDraft](fx) {
		out[d.Role] = d.Text
	}
	return out
}

func TestRestoreHubPutsTheViewBack(t *testing.T) {
	left, drafts := leftHub(t)
	v := saveHub(left, drafts)
	h, fx := reopen(t, v, viewRoster(), viewCards())
	if h.Restore != nil {
		t.Fatalf("still restoring: %+v", h.Restore)
	}
	if h.Focus != "mail" || h.Cursor != "mail" || h.List || !h.App.PaneFocus {
		t.Fatalf("focus %q cursor %q list %v pane %v", h.Focus, h.Cursor, h.List, h.App.PaneFocus)
	}
	if b := h.App.Board; b.SelID != 20 || !b.Detail || b.Line != 1 || b.Col != 1 || b.Items {
		t.Fatalf("board %+v", b)
	}
	got := restoredDrafts(fx)
	if !reflect.DeepEqual(got, map[string]string{"mail": drafts["mail"], "dev": drafts["dev"]}) {
		t.Fatalf("drafts %q", got)
	}
	if mail := h.DraftOf("mail"); !reflect.DeepEqual(mail.Pastes, unseq(left.DraftOf("mail").Pastes)) || mail.NextPaste != 2 {
		t.Fatalf("mail's pastes %+v", mail)
	}
	// Dev's pastes are its draft's, and stay so when its conversation
	// attaches.
	h, _ = apply(t, h, FocusIndex{At: t0, Index: 0}, Attached{At: t0, Role: "dev", Session: "s1"})
	if d := h.DraftOf("dev"); !reflect.DeepEqual(d.Pastes, unseq(left.DraftOf("dev").Pastes)) {
		t.Fatalf("dev's pastes %+v", d)
	}
	// Nothing restored counts as sent, asked or confirmed.
	if len(effectsOf[SendPrompt](fx))+len(effectsOf[SendCardOp](fx))+len(effectsOf[SendCardTrust](fx))+len(effectsOf[AnswerPermission](fx)) != 0 {
		t.Fatalf("effects %v", fx)
	}
}

// The parts wait for what they refer to, whichever comes first, and
// nothing is put back from the connection's first answer alone.
func TestRestoreHubWaitsForTheRosterAndTheBoard(t *testing.T) {
	left, drafts := leftHub(t)
	v := saveHub(left, drafts)
	h, fx := apply(t, RestoreHub(NewHub(""), v), HubConnected{At: t0})
	if h.Focus != "" || !h.List || len(effectsOf[RestoreDraft](fx)) != 0 || h.App.PaneFocus {
		t.Fatalf("before the roster: focus %q list %v pane %v %v", h.Focus, h.List, h.App.PaneFocus, fx)
	}
	// The board first: the card comes back; the layout still waits.
	h, _ = apply(t, h, CardsLoaded{At: t0, Rev: 30, Cards: viewCards()})
	if b := h.App.Board; b.SelID != 20 || !b.Detail || h.Focus != "" || h.Restore == nil || !h.Restore.Layout {
		t.Fatalf("after the board: %+v %+v", b, h.Restore)
	}
	// A roster that failed is not a roster.
	h, fx = apply(t, h, RosterUpdate{At: t0, Err: "down"})
	if h.Focus != "" || len(effectsOf[RestoreDraft](fx)) != 0 {
		t.Fatalf("after a failed roster: %q %v", h.Focus, fx)
	}
	h, fx = apply(t, h, RosterUpdate{At: t0, Roles: viewRoster()})
	if h.Focus != "mail" || !h.App.PaneFocus || len(effectsOf[RestoreDraft](fx)) != 2 || h.Restore != nil {
		t.Fatalf("after the roster: %q %v %v", h.Focus, h.App.PaneFocus, fx)
	}
	// The other way round: the roster first, then the board.
	h, _ = apply(t, RestoreHub(NewHub(""), v), HubConnected{At: t0}, RosterUpdate{At: t0, Roles: viewRoster()})
	if h.Focus != "mail" || h.Restore == nil || !h.Restore.Board || h.App.Board.Detail {
		t.Fatalf("roster first: %q %+v", h.Focus, h.Restore)
	}
	h, _ = apply(t, h, CardsLoaded{At: t0, Rev: 30, Cards: viewCards()})
	if b := h.App.Board; b.SelID != 20 || !b.Detail || h.Restore != nil {
		t.Fatalf("then the board: %+v", b)
	}
}

// A key before the roster comes means the operator has moved on: the
// layout and the card are not put back over it, but the drafts are.
func TestRestoreHubKeyBeforeTheRosterKeepsOnlyTheDrafts(t *testing.T) {
	left, drafts := leftHub(t)
	v := saveHub(left, drafts)
	h, _ := apply(t, RestoreHub(NewHub(""), v), HubConnected{At: t0}, ListMove{At: t0, Delta: 1})
	h, fx := apply(t, h, RosterUpdate{At: t0, Roles: viewRoster()}, CardsLoaded{At: t0, Rev: 30, Cards: viewCards()})
	if h.Focus != "" || !h.List || h.App.PaneFocus || h.App.Board.SelID != 10 || h.App.Board.Detail {
		t.Fatalf("focus %q list %v pane %v board %+v", h.Focus, h.List, h.App.PaneFocus, h.App.Board)
	}
	if len(effectsOf[RestoreDraft](fx)) != 2 {
		t.Fatalf("drafts %v", fx)
	}
}

// What has gone since falls back quietly: a role removed, a card
// archived, a conversation ended.
func TestRestoreHubFallsBackForWhatHasGone(t *testing.T) {
	left, drafts := leftHub(t)
	v := saveHub(left, drafts)
	h, fx := reopen(t, v, []Role{role("dev", "s1"), role("news", "")}, []Card{checklist(10, 10)})
	if h.Focus != "" || !h.List || h.Cursor != "dev" || h.App.PaneFocus {
		t.Fatalf("mail removed: focus %q list %v cursor %q pane %v", h.Focus, h.List, h.Cursor, h.App.PaneFocus)
	}
	if b := h.App.Board; b.SelID != 10 || b.Detail {
		t.Fatalf("card archived: %+v", b)
	}
	if got := restoredDrafts(fx); len(got) != 1 || got["dev"] == "" || h.Notice != "a saved draft could not be put back" {
		t.Fatalf("drafts %q notice %q", got, h.Notice)
	}
	if len(h.DraftOf("dev").Pastes) != 1 || len(h.DraftOf("mail").Pastes) != 0 {
		t.Fatalf("drafts %+v", h.Drafts)
	}
	// Dev in focus, its conversation since ended: Dev shows none, and none
	// is opened.
	v.Hub = &HubView{Focus: "dev", Cursor: "dev"}
	h, fx = reopen(t, v, []Role{role("dev", ""), role("mail", "")}, viewCards())
	if h.Focus != "dev" || h.Focused() != nil || len(attachesIn(fx)) != 0 {
		t.Fatalf("ended: focus %q conv %v effects %v", h.Focus, h.Focused(), fx)
	}
	// With it still live, it attaches.
	_, fx = reopen(t, v, viewRoster(), viewCards())
	if a := attachesIn(fx); len(a) != 1 || a[0] != (Attach{Role: "dev", Session: "s1"}) {
		t.Fatalf("live: %v", fx)
	}
}

// --open names the role to focus, so the saved focus, list and pane give
// way to it; the card and the drafts still come back.
func TestRestoreHubOpenFlagWins(t *testing.T) {
	left, drafts := leftHub(t)
	v := saveHub(left, drafts)
	h, fx := apply(t, RestoreHub(NewHub("news"), v), HubConnected{At: t0}, RosterUpdate{At: t0, Roles: viewRoster()},
		CardsLoaded{At: t0, Rev: 30, Cards: viewCards()})
	if h.Focus != "news" || h.List || h.App.PaneFocus || h.App.Board.SelID != 20 || len(effectsOf[RestoreDraft](fx)) != 2 {
		t.Fatalf("focus %q list %v pane %v board %d %v", h.Focus, h.List, h.App.PaneFocus, h.App.Board.SelID, fx)
	}
}

// The board with no role in focus comes back with the keys, and esc
// still goes back to the list.
func TestRestoreHubBoardWithNoRole(t *testing.T) {
	h, _ := apply(t, NewHub(""), HubConnected{At: t0}, RosterUpdate{At: t0, Roles: viewRoster()},
		CardsLoaded{At: t0, Rev: 30, Cards: viewCards()}, ListMove{At: t0, Delta: 2}, Move{At: t0, Delta: 1})
	v := saveHub(h, nil)
	g, _ := reopen(t, v, viewRoster(), viewCards())
	if g.Focus != "" || g.List || !g.App.PaneFocus || g.Cursor != "news" || !g.App.Moving {
		t.Fatalf("focus %q list %v pane %v cursor %q moving %v", g.Focus, g.List, g.App.PaneFocus, g.Cursor, g.App.Moving)
	}
	g, _ = apply(t, g, PaneKey{At: t0, Action: PaneEsc})
	if !g.List || g.Cursor != "news" {
		t.Fatalf("esc: list %v cursor %q", g.List, g.Cursor)
	}
}

// A client that closes before the roster or the board arrived saves what
// it had been given, not the defaults it showed meanwhile.
func TestSaveHubKeepsWhatHasNotComeBackYet(t *testing.T) {
	left, drafts := leftHub(t)
	v := saveHub(left, drafts)
	h, _ := apply(t, RestoreHub(NewHub(""), v), HubConnected{At: t0})
	if got := saveHub(h, nil); !reflect.DeepEqual(got, v) {
		t.Fatalf("saved\n%+v\nwant\n%+v", got, v)
	}
}

// A paste made since the start keeps its number, and the saved draft's
// pastes take the numbers after it, placeholders and trust with them, so
// both come back.
func TestRestoreHubRenumbersSavedPastes(t *testing.T) {
	left, drafts := leftHub(t)
	v := saveHub(left, drafts)
	v.Hub = nil
	h, _ := apply(t, RestoreHub(NewHub("mail"), v), HubConnected{At: t0}, Pasted{At: t0, Text: "new"})
	h, fx := apply(t, h, RosterUpdate{At: t0, Roles: viewRoster()})
	got := restoredDrafts(fx)
	saved := left.DraftOf("mail").Pastes // #1 untrusted, #2 trusted
	moved := []Paste{{ID: 2, Text: saved[0].Text}, {ID: 3, Text: saved[1].Text, Trusted: true}}
	if want := "see " + moved[0].Token() + " and " + moved[1].Token(); got["mail"] != want || h.Notice != "" {
		t.Fatalf("mail %q want %q notice %q", got["mail"], want, h.Notice)
	}
	d := h.DraftOf("mail")
	if !reflect.DeepEqual(d.Pastes, append([]Paste{{ID: 1, Text: "new", Seq: 1}}, moved...)) || d.NextPaste != 3 {
		t.Fatalf("mail %+v", d)
	}
}

// A draft typed before the restore came back is saved with the one
// still waiting, the saved one ahead, so closing in between loses
// neither: not the text, not the pastes, not their trust.
func TestSaveKeepsADraftTypedBeforeTheRestore(t *testing.T) {
	left, drafts := leftHub(t)
	v := saveHub(left, drafts)
	v.Hub = nil
	saved := left.DraftOf("mail").Pastes
	h, _ := apply(t, RestoreHub(NewHub("mail"), v), HubConnected{At: t0}, Pasted{At: t0, Text: "new"})
	newer := h.DraftOf("mail").Pastes[0]
	got := saveHub(h, map[string]string{"mail": "n " + newer.Token()})
	i := slices.IndexFunc(got.Drafts, func(d Draft) bool { return d.Role == "mail" })
	moved := []Paste{{ID: 2, Text: saved[0].Text}, {ID: 3, Text: saved[1].Text, Trusted: true}}
	want := Draft{Role: "mail", Text: "see " + moved[0].Token() + " and " + moved[1].Token() + "\nn " + newer.Token(),
		Pastes: append([]Paste{newer}, moved...), NextPaste: 3}
	if i < 0 || !reflect.DeepEqual(got.Drafts[i], want) {
		t.Fatalf("saved %+v\nwant %+v", got.Drafts, want)
	}
	// Reopened from that, it all comes back.
	g, fx := reopen(t, got, viewRoster(), viewCards())
	if restoredDrafts(fx)["mail"] != want.Text || !reflect.DeepEqual(g.DraftOf("mail").Pastes, unseq(want.Pastes)) {
		t.Fatalf("reopened %v %+v", fx, g.Drafts)
	}
	// Against a single agent, typed text with no pastes.
	x := RestoreSingle(Single{}, View{Drafts: []Draft{{Text: "old draft"}}})
	if d := saveSingle(x, "n").Drafts; len(d) != 1 || d[0].Text != "old draft\nn" {
		t.Fatalf("single %+v", d)
	}
}

func TestRestoreSingle(t *testing.T) {
	x, _ := feedS(Single{}, Connected{At: t0, Embedded: true}, CardsLoaded{At: t0, Rev: 30, Cards: viewCards()},
		Pasted{At: t0, Text: "outside"}, PaneKey{At: t0, Action: PaneToggle}, PaneKey{At: t0, Action: PaneEnter})
	draft := "read " + x.Draft.Pastes[0].Token()
	v := saveSingle(x, draft)
	if v.Hub != nil || !v.PaneFocus || v.Card != 10 || !v.Detail || len(v.Drafts) != 1 || len(v.Drafts[0].Pastes) != 1 {
		t.Fatalf("saved %+v", v)
	}
	y := RestoreSingle(Single{}, v)
	if got := saveSingle(y, ""); !reflect.DeepEqual(got, v) {
		t.Fatalf("before the session: %+v", got)
	}
	y, fx := feedS(y, Connected{At: t0, Embedded: true})
	if d := effectsOf[RestoreDraft](fx); len(d) != 1 || d[0] != (RestoreDraft{Text: draft}) || !y.PaneFocus {
		t.Fatalf("after the session: pane %v %v", y.PaneFocus, fx)
	}
	if !reflect.DeepEqual(y.Draft.Pastes, unseq(x.Draft.Pastes)) || y.Draft.NextPaste != 1 {
		t.Fatalf("pastes %+v", y.Draft.Pastes)
	}
	y, _ = feedS(y, CardsLoaded{At: t0, Rev: 30, Cards: viewCards()})
	if y.Board.SelID != 10 || !y.Board.Detail || y.Restore != nil {
		t.Fatalf("board %+v", y.Board)
	}
	// The restored draft sends its paste as it was: untrusted, attached.
	y, fx = feedS(y, Type{At: t0}, Submit{At: t0, Text: draft})
	if p := effectsOf[SendPrompt](fx); len(p) != 1 || len(p[0].Attachments) != 1 || p[0].Attachments[0].Text != "outside" {
		t.Fatalf("sent %v", fx)
	}
}

// genDraft is a draft with pastes, some trusted, and the text holding
// some of their placeholders in either form.
func genDraft(t *rapid.T) Draft {
	n := rapid.IntRange(0, 5).Draw(t, "pastes")
	var d Draft
	var parts []string
	for i := 1; i <= n; i++ {
		p := Paste{ID: i, Text: rapid.StringMatching(`[a-z\n]{1,12}`).Draw(t, "text"), Trusted: rapid.Bool().Draw(t, "trusted")}
		d.Pastes = append(d.Pastes, p)
		if rapid.Bool().Draw(t, "in text") {
			parts = append(parts, p.Token())
		}
		parts = append(parts, rapid.StringMatching(`[a-z ]{0,6}`).Draw(t, "typed"))
	}
	d.Text = strings.Join(parts, " ")
	d.NextPaste = n
	return d
}

// mangle changes a draft the way damage might: a paste's trust lost,
// its number duplicated or zeroed, its text changed, a placeholder in
// the text swapped to the other form. A paste's trust is never gained
// here: in the file it is sealed to the paste's number and text, which
// package viewfile tests.
func mangle(t *rapid.T, d Draft) Draft {
	d.Pastes = slices.Clone(d.Pastes)
	for i := rapid.IntRange(0, 4).Draw(t, "damage"); i > 0 && len(d.Pastes) > 0; i-- {
		j := rapid.IntRange(0, len(d.Pastes)-1).Draw(t, "which")
		p := &d.Pastes[j]
		switch rapid.IntRange(0, 4).Draw(t, "how") {
		case 0:
			p.Trusted = false
		case 1:
			// A paste's trust is sealed to its number and text, so damage
			// to either loses the trust (package viewfile tests the seal).
			p.ID, p.Trusted = d.Pastes[rapid.IntRange(0, len(d.Pastes)-1).Draw(t, "dup")].ID, false
		case 2:
			p.ID, p.Trusted = rapid.IntRange(-1, 0).Draw(t, "id"), false
		case 3:
			p.Text, p.Trusted = p.Text+"\nmore", false
		case 4:
			q := *p
			q.Trusted = !q.Trusted
			d.Text = strings.ReplaceAll(d.Text, p.Token(), q.Token())
		}
	}
	return d
}

// sentAs is how a paste goes out when the text is sent: "trusted" when
// as plain text the operator vouched for, "untrusted" when attached.
func sentAs(text string, ps []Paste) map[string]string {
	out := map[string]string{}
	for _, p := range ps {
		switch {
		case strings.Contains(text, p.untrustedToken()):
			out[p.Text] = "untrusted"
		case p.Trusted && strings.Contains(text, p.Token()):
			if out[p.Text] == "" {
				out[p.Text] = "trusted"
			}
		}
	}
	return out
}

// trustedWords is the words of the pastes that go as trusted text when
// text is sent.
func trustedWords(text string, ps []Paste) map[string]bool {
	out := map[string]bool{}
	for _, p := range ps {
		if p.Trusted && strings.Contains(text, p.Token()) {
			out[p.Text] = true
		}
	}
	return out
}

// A saved draft comes back exactly, and a damaged one can only come back
// less trusted: every paste that would go as trusted text went as
// trusted text, with the same words, before.
func TestRestoredDraftIsNeverMoreTrusted(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		d := genDraft(t)
		text, pastes, next := restoredDraft(d)
		if text != d.Text || next != d.NextPaste || !reflect.DeepEqual(pastes, referenced(d.Pastes, d.Text)) {
			t.Fatalf("not faithful:\n%+v\n%q %+v %d", d, text, pastes, next)
		}
		m := mangle(t, d)
		text, pastes, next = restoredDraft(m)
		// By the words: two pastes may hold the same words, one trusted
		// and one not, so what counts is whether those words went as
		// trusted text before at all.
		before := trustedWords(d.Text, d.Pastes)
		for words := range trustedWords(text, pastes) {
			if !before[words] {
				t.Fatalf("%q came back trusted, was not\nsaved %+v\ndamaged %+v\nrestored %q %+v", words, d, m, text, pastes)
			}
		}
		for _, p := range pastes {
			if p.ID <= 0 || p.ID > next {
				t.Fatalf("paste %d, next %d", p.ID, next)
			}
			if n := slices.IndexFunc(pastes, func(q Paste) bool { return q.ID == p.ID }); pastes[n] != p {
				t.Fatalf("paste %d twice", p.ID)
			}
		}
	})
}

func TestRestoredDraftExamples(t *testing.T) {
	trusted := Paste{ID: 3, Text: "mine", Trusted: true}
	untrusted := Paste{ID: 3, Text: "theirs"}
	cases := []struct {
		name   string
		d      Draft
		text   string
		pastes []Paste
	}{
		{"faithful", Draft{Text: "a " + trusted.Token(), Pastes: []Paste{trusted}}, "a " + trusted.Token(), []Paste{trusted}},
		{"untrusted with a trusted placeholder",
			Draft{Text: "a " + Paste{ID: 3, Text: "theirs", Trusted: true}.Token(), Pastes: []Paste{untrusted}},
			"a " + untrusted.Token(), []Paste{untrusted}},
		{"trusted with an untrusted placeholder",
			Draft{Text: "a " + untrusted.Token(), Pastes: []Paste{{ID: 3, Text: "theirs", Trusted: true}}},
			"a " + untrusted.Token(), []Paste{untrusted}},
		{"two with one number", Draft{Text: trusted.Token(), Pastes: []Paste{trusted, untrusted}}, trusted.Token(), nil},
		{"not in the text", Draft{Text: "a", Pastes: []Paste{trusted}}, "a", nil},
	}
	for _, c := range cases {
		text, pastes, _ := restoredDraft(c.d)
		if text != c.text || !reflect.DeepEqual(pastes, c.pastes) {
			t.Errorf("%s: %q %+v", c.name, text, pastes)
		}
	}
	_ = fmt.Sprint
}

// A key before the roster has come: the drafts still come back, the
// layout and the card do not.
func TestHubRestoreAfterAKey(t *testing.T) {
	left, drafts := leftHub(t)
	v := saveHub(left, drafts)
	h, fx := apply(t, RestoreHub(NewHub(""), v), HubConnected{At: t0}, KeyPressed{At: t0}, RosterUpdate{At: t0, Roles: viewRoster()},
		CardsLoaded{At: t0, Rev: 30, Cards: viewCards()})
	if h.Focus != "" || h.App.Board.Detail || len(effectsOf[RestoreDraft](fx)) != 2 {
		t.Fatalf("focus %q board %+v %v", h.Focus, h.App.Board, fx)
	}
	h, _ = apply(t, RestoreHub(NewHub(""), v), HubConnected{At: t0}, RosterUpdate{At: t0, Roles: viewRoster()})
	if h.Focus != "mail" {
		t.Fatalf("no key: focus %q", h.Focus)
	}
}

// A draft typed while a saved one waits is saved with it: every paste of
// both goes out as it would have, no two share a number, and the result
// comes back as saved.
func TestCarryKeepsBothDrafts(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// A prefix keeps each paste's words its own; the line count, and so
		// the placeholder, is the same.
		saved, typed := genDraft(t), genDraft(t)
		saved.Role, typed.Role = "r", "r"
		for i := range saved.Pastes {
			saved.Pastes[i].Text = "s" + saved.Pastes[i].Text
		}
		for i := range typed.Pastes {
			typed.Pastes[i].Text = "t" + typed.Pastes[i].Text
		}
		typed.Pastes = referenced(typed.Pastes, typed.Text) // as SaveHub saves it
		got := carry(View{Drafts: []Draft{typed}}, &Restore{View: View{Drafts: []Draft{saved}}, Drafts: true})
		if len(got.Drafts) != 1 {
			t.Fatalf("drafts %+v", got.Drafts)
		}
		m := got.Drafts[0]
		want := sentAs(saved.Text, saved.Pastes)
		for words, how := range sentAs(typed.Text, typed.Pastes) {
			want[words] = how
		}
		if out := sentAs(m.Text, m.Pastes); !reflect.DeepEqual(out, want) {
			t.Fatalf("sent as %v, want %v\nsaved %+v\ntyped %+v\nmerged %+v", out, want, saved, typed, m)
		}
		seen := map[int]bool{}
		for _, p := range m.Pastes {
			if seen[p.ID] || p.ID <= 0 || p.ID > m.NextPaste {
				t.Fatalf("paste %d in %+v", p.ID, m)
			}
			seen[p.ID] = true
		}
		if text, pastes, _ := restoredDraft(m); text != m.Text || len(pastes) != len(m.Pastes) {
			t.Fatalf("comes back as %q %+v from %+v", text, pastes, m)
		}
	})
}

// A view saved against the other kind of agent: its layout means
// nothing here and goes quietly, and a draft with nowhere to go (a role's
// against a single agent, one with no role against the hub) is not put
// back, and the status line says so.
func TestAViewOfTheOtherKind(t *testing.T) {
	hubView := View{Hub: &HubView{Focus: "mail", Cursor: "mail"}, Drafts: []Draft{{Role: "mail", Text: "for mail"}}}
	x, fx := feedS(RestoreSingle(Single{}, hubView), Connected{At: t0})
	if x.Restore != nil || x.App.PaneFocus || len(effectsOf[RestoreDraft](fx)) != 0 || x.State.Message.Text != "a saved draft could not be put back" {
		t.Fatalf("single: %+v pane %v %v %q", x.Restore, x.App.PaneFocus, fx, x.State.Message.Text)
	}
	singleView := View{Drafts: []Draft{{Text: "for the agent"}}}
	h, fx := apply(t, RestoreHub(NewHub(""), singleView), HubConnected{At: t0}, RosterUpdate{At: t0, Roles: viewRoster()})
	if len(effectsOf[RestoreDraft](fx)) != 0 || h.Notice != "a saved draft could not be put back" {
		t.Fatalf("hub: %v %q", fx, h.Notice)
	}
}

// unseq is pastes as a restore gives them back: not from this run.
func unseq(ps []Paste) []Paste {
	out := slices.Clone(ps)
	for i := range out {
		out[i].Seq = 0
	}
	return out
}

// saveHub is SaveHub with each role's input text as the shell would have
// written it back.
func saveHub(h Hub, texts map[string]string) View {
	for role, text := range texts {
		h = SetDraftText(h, role, text)
	}
	return SaveHub(h)
}

// saveSingle is SaveSingle with the input's text as the shell would have
// written it back.
func saveSingle(x Single, text string) View {
	x.Draft.Text = text
	return SaveSingle(x)
}
