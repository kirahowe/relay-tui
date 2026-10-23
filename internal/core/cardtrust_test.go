package core

import (
	"strings"
	"testing"

	"pgregory.net/rapid"
)

const shaA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// outsideCard is a checklist with outside text, at rev, with the digest
// relay sends with it.
func outsideCard(rev int64, sha string) Card {
	c := checklist(30, rev, false)
	c.Labels = Labels{Private: true, Contaminated: true}
	c.SHA256 = sha
	return c
}

func TestTrustingACard(t *testing.T) {
	s := onBoard(outsideCard(40, shaA))
	s, fx := boardKeys(s, PaneTrust)
	if s.Pane.Board.Trust == nil || len(fx) != 0 {
		t.Fatalf("no question: %+v %v", s.Pane.Board.Trust, fx)
	}
	// y before the delay does nothing, and keeps the question.
	s, fx = feed(s, PaneKey{At: t0, Action: PaneYes})
	if s.Pane.Board.Trust == nil || len(fx) != 0 {
		t.Fatalf("y before the delay: %+v %v", s.Pane.Board.Trust, fx)
	}
	s, fx = feed(s, PaneKey{At: t0.Add(ArmDelay), Action: PaneYes})
	want := SendCardTrust{Card: 30, Rev: 40, SHA256: shaA}
	if got := effectsOf[SendCardTrust](fx); len(got) != 1 || got[0] != want || !s.Pane.Board.Trust.Sent {
		t.Fatalf("effects %v, trust %+v", fx, s.Pane.Board.Trust)
	}
	clean := checklist(30, 41, false)
	clean.Labels = Labels{Private: true}
	s, _ = feed(s, CardTrustDone{At: t0, CardID: 30, Card: &clean})
	b := s.Pane.Board
	if b.Trust != nil || b.Message != "Trusted." || b.Cards[0].Labels.Contaminated {
		t.Fatalf("after: %+v", b)
	}
}

func TestCardTrustCancelsAndRefuses(t *testing.T) {
	s, fx := boardKeys(onBoard(outsideCard(40, shaA)), PaneTrust, PaneDown)
	if s.Pane.Board.Trust != nil || s.Pane.Board.Message != "Trust cancelled." || len(fx) != 0 {
		t.Fatalf("a key other than y: %+v %v", s.Pane.Board, fx)
	}
	s, _ = boardKeys(onBoard(checklist(30, 40, false)), PaneTrust)
	if s.Pane.Board.Trust != nil || !strings.Contains(s.Pane.Board.Message, "Nothing to trust") {
		t.Fatalf("a clean card: %+v", s.Pane.Board)
	}
	s, _ = boardKeys(onBoard(outsideCard(40, "")), PaneTrust)
	if s.Pane.Board.Trust != nil || !strings.Contains(s.Pane.Board.Message, "relay trust card 30") {
		t.Fatalf("no digest: %+v", s.Pane.Board)
	}
	s, _ = boardKeys(onBoard(outsideCard(40, shaA)), PaneTrust)
	s, _ = feed(s, PaneKey{At: t0.Add(ArmDelay), Action: PaneYes}, CardTrustDone{At: t0, CardID: 30, Err: "The card 30 changed since you read it."})
	if b := s.Pane.Board; b.Trust != nil || b.Message != "Trust failed." || !strings.Contains(b.Failure, "changed") {
		t.Fatalf("a refusal: %+v", b)
	}
}

func TestACardThatChangesIsNotTrusted(t *testing.T) {
	s, _ := boardKeys(onBoard(outsideCard(40, shaA)), PaneTrust)
	s, _ = feed(s, CardsChanged{At: t0, Rev: 41, Cards: []Card{outsideCard(41, shaA)}})
	if s.Pane.Board.Trust != nil || !strings.Contains(s.Pane.Board.Message, "changed") {
		t.Fatalf("the question outlived the change: %+v", s.Pane.Board)
	}
	s, fx := feed(s, PaneKey{At: t0.Add(ArmDelay), Action: PaneYes})
	if len(effectsOf[SendCardTrust](fx)) != 0 {
		t.Fatalf("trusted a changed card: %v", fx)
	}
}

// However cards change and keys arrive, a trust is only ever sent for the
// rev and digest of the card as it is held when y is pressed, which is the
// card the operator was shown when they pressed t.
func TestATrustIsOnlyForTheCardAsShown(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := onBoard(outsideCard(40, shaA))
		rev := int64(40)
		var shown *Card
		for i := range rapid.IntRange(1, 30).Draw(t, "steps") {
			at := t0.Add(ArmDelay * 2)
			var fx []Effect
			switch rapid.IntRange(0, 3).Draw(t, "what") {
			case 0:
				rev++
				sha := rapid.SampledFrom([]string{shaA, strings.Repeat("b", 64), ""}).Draw(t, "sha")
				s, fx = Reduce(s, CardsChanged{At: at, Rev: rev, Cards: []Card{outsideCard(rev, sha)}})
			case 1:
				s, fx = Reduce(s, PaneKey{At: t0, Action: PaneTrust})
				if s.Pane.Board.Trust != nil && !s.Pane.Board.Trust.Sent {
					c := s.Pane.Board.Cards[0]
					shown = &c
				}
			case 2:
				s, fx = Reduce(s, PaneKey{At: at, Action: PaneYes})
			case 3:
				s, fx = Reduce(s, PaneKey{At: at, Action: rapid.SampledFrom([]PaneAction{PaneDown, PaneEsc, PaneOther}).Draw(t, "key")})
			}
			for _, e := range effectsOf[SendCardTrust](fx) {
				held := s.Pane.Board.Cards[0]
				if shown == nil || e.Rev != shown.Rev || e.SHA256 != shown.SHA256 || e.Rev != held.Rev || e.SHA256 != held.SHA256 {
					t.Fatalf("step %d sent %+v; shown %+v, held rev %d %q", i, e, shown, held.Rev, held.SHA256)
				}
			}
			if tr := s.Pane.Board.Trust; tr != nil && tr.Sent {
				// relay answers before the next step.
				s, _ = Reduce(s, CardTrustDone{At: at, CardID: 30, Err: "refused"})
				if !s.Pane.Focus {
					s, _ = Reduce(s, PaneKey{At: at, Action: PaneToggle})
				}
			}
			if !s.Pane.Focus {
				s, _ = Reduce(s, PaneKey{At: at, Action: PaneToggle})
			}
		}
	})
}
