package core

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// feed applies events in turn and returns the state and all the effects.
func feed(s Single, events ...Event) (Single, []Effect) { return feedS(s, events...) }

func effectsOf[T Effect](fx []Effect) []T {
	var out []T
	for _, f := range fx {
		if v, ok := f.(T); ok {
			out = append(out, v)
		}
	}
	return out
}

func key(at time.Time, a PaneAction) PaneKey { return PaneKey{At: at, Action: a} }

var shownItem = Item{
	Ref: "shown:2026-10-18/x", Path: "shown/2026-10-18/x.md", CallID: "x", Title: "Venues",
	Text: "see https://example.org/v", Sources: []Source{{Title: "List", Ref: "web:https://example.org/list"}},
	Labels: Labels{Private: true, Contaminated: true},
}

// focused is a connected session with relay's list loaded and the pane
// focused on the first item.
func focused(items ...Item) Single {
	s, _ := feed(Single{}, Connected{At: t0}, ItemsLoaded{At: t0, Items: items}, key(t0, PaneToggle), key(t0, PaneBoard))
	return s
}

func TestParseRefs(t *testing.T) {
	got := ParseRefs("see [[fact:12]] and [[doc:a b/c.md]], again [[fact:12]]; not [[nope:1]] or [[fact:]] or [[fact:a\nb]] or [fact:1]")
	want := []string{"[[fact:12]]", "[[doc:a b/c.md]]"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// Any id, once its brackets, newline and percent are escaped, is found
// whole in the middle of prose.
func TestParseRefsFindsEscapedIDs(t *testing.T) {
	kinds := []string{"fact", "doc", "note", "memo", "journal", "rem", "file", "web", "shown", "report"}
	rapid.Check(t, func(t *rapid.T) {
		kind := rapid.SampledFrom(kinds).Draw(t, "kind")
		raw := rapid.StringN(1, 20, -1).Draw(t, "id")
		id := strings.NewReplacer("%", "%25", "[", "%5B", "]", "%5D", "\n", "%0A").Replace(raw)
		ref := "[[" + kind + ":" + id + "]]"
		got := ParseRefs("before " + ref + " after")
		if !slices.Equal(got, []string{ref}) {
			t.Fatalf("ParseRefs found %q in %q", got, ref)
		}
	})
}

func displayEvent(id, title string, labels Labels) ToolUpdated {
	return ToolUpdated{At: t0, ID: id, Displays: []Display{{Title: title, Body: "body " + title, Labels: labels, Ref: "shown:" + id}}}
}

func TestDisplaysFillThePaneNewestFirst(t *testing.T) {
	s := busySingle()
	var fx []Effect
	s, fx = feed(s, displayEvent("a", "first", Labels{}), displayEvent("b", "second", Labels{Contaminated: true}))
	items := PaneItems(s.State)
	if len(items) != 2 || items[0].Title != "second" || items[1].Title != "first" {
		t.Fatalf("items %#v", items)
	}
	if n := len(effectsOf[RequestItems](fx)); n != 2 {
		t.Errorf("%d item requests, want one per display", n)
	}
	// The same display again replaces itself.
	s, _ = feed(s, displayEvent("a", "first", Labels{}))
	if len(PaneItems(s.State)) != 2 {
		t.Errorf("a repeated display added an item: %#v", PaneItems(s.State))
	}
}

func TestPaneNavigation(t *testing.T) {
	a, b := shownItem, shownItem
	b.Ref, b.Path, b.Title = "shown:b", "shown/b.md", "Second"
	s := focused(a, b)
	if SelectedIndex(s.State) != 0 {
		t.Fatal("not on the first item")
	}
	s, _ = feed(s, key(t0, PaneDown))
	if SelectedIndex(s.State) != 1 {
		t.Fatalf("down: %d", SelectedIndex(s.State))
	}
	s, _ = feed(s, key(t0, PaneDown)) // clamped
	if SelectedIndex(s.State) != 1 {
		t.Fatalf("down past the end: %d", SelectedIndex(s.State))
	}
	// The selection follows its item when a new one arrives on top.
	s, _ = feed(s, ItemsLoaded{At: t0, Items: []Item{{Ref: "shown:new"}, a, b}})
	if i := SelectedIndex(s.State); i != 2 {
		t.Fatalf("selection moved to %d", i)
	}
	// Enter opens, scroll is clamped to what the renderer says, esc goes back.
	s, _ = feed(s, key(t0, PaneEnter))
	if !s.Pane.Detail {
		t.Fatal("enter did not open")
	}
	s, _ = feed(s, PaneKey{At: t0, Action: PaneDown, Max: 1}, PaneKey{At: t0, Action: PaneDown, Max: 1}, PaneKey{At: t0, Action: PageNext, Max: 1})
	if s.Pane.Scroll != 1 {
		t.Fatalf("scroll %d", s.Pane.Scroll)
	}
	s, _ = feed(s, PaneKey{At: t0, Action: PagePrev, Max: 1})
	if s.Pane.Scroll != 0 {
		t.Fatalf("scroll %d", s.Pane.Scroll)
	}
	s, _ = feed(s, key(t0, PaneEsc))
	if s.Pane.Detail || !s.PaneFocus {
		t.Fatalf("esc from detail: %+v", s.Pane)
	}
	s, _ = feed(s, key(t0, PaneEsc))
	if s.PaneFocus {
		t.Fatal("esc did not return focus")
	}
	// Keys do nothing while the pane is not focused.
	s2, fx := feed(s, key(t0, PaneEnter), key(t0, PaneYes))
	if s2.Pane.Detail || len(fx) != 0 {
		t.Fatal("an unfocused pane acted on keys")
	}
}

func TestRelayListIsAuthoritative(t *testing.T) {
	s, fx := feed(Single{}, Connected{At: t0})
	if len(effectsOf[RequestItems](fx)) != 1 {
		t.Fatalf("no item request on connect: %#v", fx)
	}
	s, _ = feed(s, Submit{At: t0, Text: "go"}, displayEvent("a", "local", Labels{}))
	s, _ = feed(s, ItemsLoaded{At: t0, Items: []Item{{Ref: "shown:r", Title: "from relay"}}})
	if items := PaneItems(s.State); len(items) != 1 || items[0].Title != "from relay" {
		t.Fatalf("items %#v", items)
	}
	// A failed call leaves the list alone.
	s, _ = feed(s, ItemsLoaded{At: t0, Failed: true})
	if len(PaneItems(s.State)) != 1 {
		t.Fatalf("a failure changed the list: %#v", PaneItems(s.State))
	}
}

func TestPlainAgentFallsBackAndStopsAsking(t *testing.T) {
	s, _ := feed(busySingle(), ItemsLoaded{At: t0, Unavailable: true})
	s, fx := feed(s, displayEvent("a", "local", Labels{}), TextChunk{At: t0, Text: "see [[fact:1]]\n\n"})
	if n := len(effectsOf[RequestItems](fx)) + len(effectsOf[RequestResolve](fx)); n != 0 {
		t.Errorf("%d relay requests after method-not-found", n)
	}
	items := PaneItems(s.State)
	if len(items) != 1 || items[0].Title != "local" {
		t.Fatalf("items %#v", items)
	}
}

// Whatever keys reach the pane, it never sends a prompt, and its only
// effect on relay is a request for its list.
func TestPaneKeysSendNothingButTheListRequest(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := focused(shownItem, Item{Ref: "shown:b", Title: "b"})
		now := t0
		for range rapid.IntRange(1, 40).Draw(t, "keys") {
			now = now.Add(time.Duration(rapid.IntRange(0, 900).Draw(t, "ms")) * time.Millisecond)
			var e Event
			switch rapid.IntRange(0, 3).Draw(t, "kind") {
			case 0:
				e = PaneKey{At: now, Action: PaneAction(rapid.IntRange(0, int(PaneYes)).Draw(t, "action")), Max: 5}
			case 1:
				e = Tick{At: now}
			case 2:
				e = ItemsLoaded{At: now, Items: []Item{shownItem}}
			case 3:
				e = Submit{At: now, Text: ""}
			}
			var fx []Effect
			s, fx = ReduceSingle(s, e)
			for _, f := range fx {
				switch f.(type) {
				case RequestItems, RequestResolve, RequestCards:
				default:
					t.Fatalf("%T gave %#v", e, f)
				}
			}
		}
	})
}

func TestCitationsAreResolvedOnce(t *testing.T) {
	s := busySingle()
	var all []Effect
	var fx []Effect
	for _, chunk := range []string{"The hall [[fa", "ct:12]] seats 900, ", "[[fact:12]] and [[web:https://x.test/a]].\n\n", "More [[fact:12]]."} {
		s, fx = feed(s, TextChunk{At: t0, MessageID: "m", Text: chunk})
		all = append(all, fx...)
	}
	var refs []string
	for _, r := range effectsOf[RequestResolve](all) {
		refs = append(refs, r.Ref)
	}
	if want := []string{"[[fact:12]]", "[[web:https://x.test/a]]"}; !slices.Equal(refs, want) {
		t.Fatalf("resolve requests %q, want %q", refs, want)
	}
	if c := s.Pane.Cite("[[fact:12]]"); c.Status != "" {
		t.Errorf("a pending citation reads %+v", c)
	}
	s, _ = feed(s, CitationResolved{At: t0, Cite: Citation{Ref: "[[fact:12]]", Kind: "fact", Status: CiteOK, Title: "Fact 12"}})
	if c := s.Pane.Cite("[[fact:12]]"); c.Title != "Fact 12" || c.Status != CiteOK {
		t.Errorf("resolved citation %+v", c)
	}
	// Only the latest message's references are the current ones.
	s, _ = feed(s, TextChunk{At: t0, MessageID: "m2", Text: "Now [[doc:b]]."})
	if got := LatestRefs(s.State); !slices.Equal(got, []string{"[[doc:b]]"}) {
		t.Errorf("latest refs %q", got)
	}
}

func TestStatusCountsPaneItems(t *testing.T) {
	s := focused(shownItem, shownItem)
	if got := Status(s.App, s.State, t0).PaneItems; got != 2 {
		t.Errorf("PaneItems %d", got)
	}
}

// Without relay's list the pane shows what it saw, newest first, however
// many displays arrive.
func TestLocalItemsNewestFirst(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := busySingle()
		n := rapid.IntRange(0, 12).Draw(t, "n")
		for i := range n {
			s, _ = ReduceSingle(s, displayEvent(fmt.Sprintf("c%d", i), fmt.Sprintf("title %d", i), Labels{}))
		}
		items := PaneItems(s.State)
		if len(items) != n {
			t.Fatalf("%d items, want %d", len(items), n)
		}
		for i, it := range items {
			if want := fmt.Sprintf("title %d", n-1-i); it.Title != want {
				t.Fatalf("item %d is %q, want %q", i, it.Title, want)
			}
		}
	})
}
