package core

import (
	"fmt"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

func idleState() Single {
	x, _ := feedS(Single{}, Connected{At: t0})
	return x
}

func prompts(fx []Effect) []SendPrompt {
	var out []SendPrompt
	for _, f := range fx {
		if p, ok := f.(SendPrompt); ok {
			out = append(out, p)
		}
	}
	return out
}

// What Kira pastes is hers: the prompt carries the pasted text itself,
// in place of the placeholder, and no attachment. Pastes whose
// placeholder was deleted are not sent. Each paste is sent once.
func TestPasteIsSentAsPlainText(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := idleState()
		var input string            // the input box, as the shell keeps it
		held := map[string]string{} // secret -> token, for pastes still in the input
		sent := map[string]int{}    // secret -> times sent
		n := 0
		apply := func(e Event) []Effect {
			var fx []Effect
			s, fx = feedS(s, e)
			for _, f := range fx {
				if f, ok := f.(InsertInput); ok {
					input += f.Text
				}
			}
			return fx
		}
		for range rapid.IntRange(1, 30).Draw(t, "steps") {
			switch rapid.IntRange(0, 4).Draw(t, "step") {
			case 0, 1:
				n++
				secret := fmt.Sprintf("<<S%d>>", n)
				body := secret + strings.Repeat("\nmore text", rapid.IntRange(0, 3).Draw(t, "lines"))
				before := input
				apply(Pasted{At: t0, Text: body})
				tok := strings.TrimPrefix(input, before)
				if !strings.HasPrefix(tok, "[pasted ") || strings.Contains(tok, secret) {
					t.Fatalf("the input got %q for a paste", tok)
				}
				for _, w := range []string{"trusted", "untrusted", "contaminated"} {
					if strings.Contains(tok, w) {
						t.Fatalf("the placeholder %q says %q", tok, w)
					}
				}
				held[secret] = tok
			case 2:
				input += "typed "
			case 3: // delete a placeholder
				for secret, tok := range held {
					if strings.Contains(input, tok) {
						input = strings.Replace(input, tok, "", 1)
						delete(held, secret)
						break
					}
				}
			case 4:
				text := input
				if strings.TrimSpace(text) == "" {
					continue
				}
				input = ""
				ps := prompts(apply(Submit{At: t0, Text: text}))
				if len(ps) != 1 {
					t.Fatalf("%d prompts", len(ps))
				}
				for secret, tok := range held {
					inText := strings.Contains(text, tok)
					if inText != strings.Contains(ps[0].Text, secret) {
						t.Fatalf("%s: placeholder in the input %v, in the prompt %v", secret, inText, strings.Contains(ps[0].Text, secret))
					}
					if inText {
						sent[secret]++
						delete(held, secret)
					}
					if strings.Contains(ps[0].Text, tok) {
						t.Fatalf("placeholder %q left in the prompt %q", tok, ps[0].Text)
					}
				}
				apply(PromptDone{At: t0, Stop: EndTurn})
			}
		}
		for secret, c := range sent {
			if c != 1 {
				t.Fatalf("%s sent %d times", secret, c)
			}
		}
	})
}

// resolve replaces each placeholder once, in a single pass: pasted text
// that spells another placeholder stays as it is.
func TestResolveDoesNotExpandPastedPlaceholders(t *testing.T) {
	a := Paste{ID: 1, Text: "see [pasted 1 line #2]"}
	b := Paste{ID: 2, Text: "B"}
	got := resolve("x "+a.Token()+" y "+b.Token(), []Paste{a, b})
	if want := "x see [pasted 1 line #2] y B"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got := resolve("no placeholders", []Paste{a}); got != "no placeholders" {
		t.Fatalf("got %q", got)
	}
}

// A paste in a message that was queued behind a running turn is held
// until that message goes out, and is not lost if the turn fails and the
// message goes back to the input.
func TestPasteSurvivesQueue(t *testing.T) {
	s := idleState()
	s, _ = feedS(s, Submit{At: t0, Text: "first"})
	s, fx := feedS(s, Pasted{At: t0, Text: "SECRET"})
	tok := fx[0].(InsertInput).Text
	s, _ = feedS(s, Submit{At: t0, Text: "second " + tok})
	_, fx = feedS(s, PromptDone{At: t0, Stop: EndTurn})
	ps := prompts(fx)
	if len(ps) != 1 || ps[0].Text != "second SECRET" {
		t.Fatalf("prompt %+v", ps)
	}
	// Failure: the text returns to the input, and the paste is still held.
	s2, fx := feedS(s, PromptFailed{At: t0, Message: "x"})
	if len(prompts(fx)) != 0 || len(s2.Draft.Pastes) != 1 {
		t.Fatalf("effects %+v pastes %+v", fx, s2.Draft.Pastes)
	}
}

// While moving, a paste goes nowhere; while typing the input keeps the
// keys and the paste lands.
func TestPasteOnlyWhileTyping(t *testing.T) {
	x := idleState()
	if typed, fx := feedS(x, Pasted{At: t0, Text: "x"}); len(fx) != 1 || len(typed.Draft.Pastes) != 1 {
		t.Fatalf("typing: the paste did not land: %+v", fx)
	}
	x, _ = feedS(x, StopTyping{At: t0})
	if s2, fx := feedS(x, Pasted{At: t0, Text: "x"}); len(fx) != 0 || len(s2.Draft.Pastes) != 0 {
		t.Fatalf("moving: a paste got through: %+v", fx)
	}
}

// Terminals send CR, CRLF or LF for a line break in a paste. The count
// and the text sent are the same for all three.
func TestPasteLineBreaksAreAlike(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lines := rapid.SliceOfN(rapid.StringMatching(`[a-z]{1,6}`), 1, 6).Draw(t, "lines")
		var out []string
		for _, br := range []string{"\n", "\r", "\r\n"} {
			s := idleState()
			s, _ = feedS(s, Pasted{At: t0, Text: strings.Join(lines, br)})
			p := s.Draft.Pastes[0]
			if p.Lines() != len(lines) || strings.Contains(p.Text, "\r") {
				t.Fatalf("break %q: %d lines, %q", br, p.Lines(), p.Text)
			}
			out = append(out, resolve(p.Token(), s.Draft.Pastes))
		}
		if out[0] != out[1] || out[1] != out[2] || out[0] != strings.Join(lines, "\n") {
			t.Fatalf("expansions differ: %q", out)
		}
	})
}

func TestCRPasteSaysThreeLines(t *testing.T) {
	s := idleState()
	s, fx := feedS(s, Pasted{At: t0, Text: "one\rtwo\rthree"})
	if got := fx[0].(InsertInput).Text; got != "[pasted 3 lines #1]" {
		t.Fatal(got)
	}
	if r := resolve("x "+s.Draft.Pastes[0].Token(), s.Draft.Pastes); r != "x one\ntwo\nthree" {
		t.Fatalf("%q", r)
	}
}
