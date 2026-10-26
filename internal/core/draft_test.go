package core

import (
	"fmt"
	"regexp"
	"strconv"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// A role owns its draft: its text, its pastes and their counter, apart
// from its conversations. However conversations start, end, attach,
// replay first or fail, and roles come and go, every placeholder in
// every draft names exactly the paste it was given for, and no paste
// number is ever used twice in a role.

var tokenRE = regexp.MustCompile(`\[pasted \d+ lines? #(\d+)\]`)

// draftShell plays the shell for the hub: an input per role, edited by
// the core's effects and written back with SetDraftText.
type draftShell struct {
	h Hub
	// payload is what each paste number of each role was given for.
	payload map[string]map[int]string
	at      time.Time
}

func (s *draftShell) text(role string) string { return s.h.DraftOf(role).Text }

func (s *draftShell) apply(t *rapid.T, e Event) {
	var fx []Effect
	s.h, fx = ReduceHub(s.h, e)
	for _, f := range fx {
		switch f := f.(type) {
		case InsertInput:
			s.h = SetDraftText(s.h, f.Role, s.text(f.Role)+f.Text)
			m := tokenRE.FindStringSubmatch(f.Text)
			n, _ := strconv.Atoi(m[1])
			if s.payload[f.Role] == nil {
				s.payload[f.Role] = map[int]string{}
			}
			if _, used := s.payload[f.Role][n]; used {
				t.Fatalf("paste number %d used twice in %s", n, f.Role)
			}
			p, ok := e.(Pasted)
			if !ok {
				t.Fatalf("%T inserted a placeholder", e)
			}
			s.payload[f.Role][n] = p.Text
		case RestoreInput:
			s.h = SetDraftText(s.h, f.Role, aheadOf(f.Text, s.text(f.Role)))
		case RestoreDraft:
			s.h = SetDraftText(s.h, f.Role, aheadOf(f.Text, s.text(f.Role)))
		}
	}
}

// check is the rule, for every role's draft.
func (s *draftShell) check(t *rapid.T, step int, e Event) {
	for _, d := range s.h.Drafts {
		ids := map[int]int{}
		for _, p := range d.Pastes {
			ids[p.ID]++
			if ids[p.ID] > 1 {
				t.Fatalf("step %d (%T): %s holds paste %d twice", step, e, d.Role, p.ID)
			}
			if p.ID > d.NextPaste {
				t.Fatalf("step %d (%T): %s's counter %d is behind paste %d", step, e, d.Role, d.NextPaste, p.ID)
			}
		}
		for _, m := range tokenRE.FindAllStringSubmatch(d.Text, -1) {
			n, _ := strconv.Atoi(m[1])
			var found *Paste
			for i := range d.Pastes {
				if d.Pastes[i].ID == n {
					found = &d.Pastes[i]
				}
			}
			switch {
			case found == nil:
				t.Fatalf("step %d (%T): %s's placeholder #%d names no paste (%+v)", step, e, d.Role, n, d.Pastes)
			case found.Text != s.payload[d.Role][n]:
				t.Fatalf("step %d (%T): %s's #%d holds %q, was given %q", step, e, d.Role, n, found.Text, s.payload[d.Role][n])
			}
		}
	}
}

// A paste in an unsent draft, then the conversation ended from another
// device while the role stays. The draft keeps the paste and its counter;
// a later paste takes a new number, and both placeholders send what they
// were given.
func TestEndingAConversationLeavesItsRolesDraft(t *testing.T) {
	h, _ := apply(t, NewHub("dev"), HubConnected{At: t0}, RosterUpdate{At: t0, Roles: []Role{role("dev", "s1")}},
		Attached{At: t0, Role: "dev", Session: "s1"}, KeyPressed{At: t0})
	h, fx := apply(t, h, Pasted{At: t0, Text: "first"})
	first, _ := insertText(fx)
	h = SetDraftText(h, "dev", "see "+first)
	h, _ = apply(t, h, RosterUpdate{At: t0, Roles: []Role{role("dev", "")}}) // ended elsewhere
	if h.ConvOf("dev") != nil {
		t.Fatal("the conversation is still there")
	}
	h, fx = apply(t, h, Pasted{At: t0, Text: "second"})
	second, _ := insertText(fx)
	if second == first {
		t.Fatalf("the number was used again: %q", second)
	}
	if v := SaveHub(SetDraftText(h, "dev", "see "+first+" "+second)); len(v.Drafts) != 1 || len(v.Drafts[0].Pastes) != 2 || v.Drafts[0].NextPaste != 2 {
		t.Fatalf("saved %+v", v.Drafts)
	}
	_, fx = apply(t, h, Submit{At: t0, Text: "see " + first + " " + second}, Attached{At: t0, Role: "dev", Session: "s2"})
	var sent []string
	for _, f := range fx {
		if fs, ok := f.(ForSession); ok {
			if sp, ok := fs.Effect.(SendPrompt); ok {
				sent = append(sent, sp.Text)
			}
		}
	}
	if fmt.Sprint(sent) != "[see first second]" {
		t.Fatalf("sent %q", sent)
	}
}

func TestADraftsPastesSurviveItsConversations(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := &draftShell{payload: map[string]map[int]string{}, at: t0}
		roles := []Role{role("dev", ""), role("mail", "")}
		s.apply(t, HubConnected{At: t0})
		s.apply(t, RosterUpdate{At: t0, Roles: roles})
		s.apply(t, KeyPressed{At: t0})
		session := 0
		for step := range rapid.IntRange(1, 60).Draw(t, "steps") {
			s.at = s.at.Add(time.Duration(rapid.IntRange(0, 900).Draw(t, "ms")) * time.Millisecond)
			at := s.at
			focus := s.h.Focus
			var e Event
			switch rapid.IntRange(0, 12).Draw(t, "what") {
			case 0, 1:
				e = Pasted{At: at, Text: fmt.Sprint("payload ", step)}
			case 2: // typing between placeholders
				if focus != "" {
					s.h = SetDraftText(s.h, focus, s.text(focus)+" words ")
				}
				continue
			case 3: // send what is typed, as the shell does: the input empties
				if focus == "" {
					continue
				}
				text := s.text(focus)
				s.h = SetDraftText(s.h, focus, "")
				e = Submit{At: at, Text: text}
			case 4:
				e = FocusIndex{At: at, Index: rapid.IntRange(0, 1).Draw(t, "role")}
			case 5:
				e = Type{At: at}
			case 6: // the attach answered, or the replay came first
				session++
				id := fmt.Sprint("s", session)
				if rapid.Bool().Draw(t, "replay first") {
					s.apply(t, Routed{Session: id, Event: TextChunk{At: at, Text: "earlier"}})
				}
				e = Attached{At: at, Role: rapid.SampledFrom([]string{"dev", "mail"}).Draw(t, "attached"), Session: id}
			case 7:
				e = Attached{At: at, Role: rapid.SampledFrom([]string{"dev", "mail"}).Draw(t, "failed"), Err: "no"}
			case 8: // ended here or on another device
				if len(s.h.Convs) == 0 {
					continue
				}
				c := s.h.Convs[rapid.IntRange(0, len(s.h.Convs)-1).Draw(t, "conv")]
				if rapid.Bool().Draw(t, "elsewhere") {
					var rs []Role
					for _, r := range roles {
						if r.ID != c.Role {
							rs = append(rs, r)
						}
					}
					rs = append(rs, role(c.Role, ""))
					e = RosterUpdate{At: at, Roles: rs}
				} else {
					e = ConversationEnded{At: at, Session: c.Session}
				}
			case 9: // a role removed, and back
				rs := []Role{role("mail", "")}
				if rapid.Bool().Draw(t, "dev back") {
					rs = append(rs, role("dev", ""))
				}
				e = RosterUpdate{At: at, Roles: rs}
			case 10:
				if len(s.h.Convs) == 0 {
					continue
				}
				c := s.h.Convs[rapid.IntRange(0, len(s.h.Convs)-1).Draw(t, "conv")]
				e = Routed{Session: c.Session, Event: rapid.SampledFrom([]Event{PromptDone{At: at, Stop: EndTurn}, PromptFailed{At: at, Message: "x"}}).Draw(t, "done")}
			case 11:
				e = StopTyping{At: at}
			default:
				e = StartConversation{At: at}
			}
			if fromOperator(e) {
				s.apply(t, KeyPressed{At: at})
			}
			s.apply(t, e)
			s.check(t, step, e)
		}
	})
}
