package core

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// A pasted block of text is the usual way untrusted text (an email, a web
// page) reaches the agent. So a paste is not typed into the prompt: it is
// held, the input shows a placeholder, and on send the prompt carries it
// as an embedded resource, which the agent labels contaminated. The
// person can then mark one block as trusted text (TrustPaste), after the
// fact and on a placeholder in view: a paste is always untrusted when it
// arrives, so nothing is armed ahead of it for a later paste to inherit.
// Where the agent takes no embedded context, the text goes in the prompt
// and the transcript says it was not labelled.

// Paste is a block of pasted text held until its placeholder is sent.
type Paste struct {
	ID   int
	Text string
	// Trusted says the person marked it trusted; its placeholder then
	// reads "trusted", and it is sent as plain text, not an attachment.
	Trusted bool
	// Seq is when it was pasted in this run, counted per connection
	// (App.PasteSeq): later pastes have higher numbers, whatever their ID
	// says. 0 means it was not pasted in this run but restored from a
	// saved view, and Ctrl-Y never trusts it. Seq is never saved.
	Seq int
}

// Lines is how many lines the paste has.
func (p Paste) Lines() int { return pasteLines(p.Text) }

// Token is the placeholder the input shows for the paste. The id makes
// it unique, so deleting it from the input drops the paste.
func (p Paste) Token() string {
	n := p.Lines()
	unit := "lines"
	if n == 1 {
		unit = "line"
	}
	word := "untrusted"
	if p.Trusted {
		word = "trusted"
	}
	return fmt.Sprintf("[pasted %d %s · %s #%d]", n, unit, word, p.ID)
}

// untrustedToken is the placeholder the paste had when it arrived.
func (p Paste) untrustedToken() string {
	q := p
	q.Trusted = false
	return q.Token()
}

// Ref is how the prompt's text refers to the paste's resource.
func (p Paste) Ref() string { return fmt.Sprintf("[%s]", p.URI()) }

// URI is the paste's resource uri.
func (p Paste) URI() string { return fmt.Sprintf("paste:%d", p.ID) }

func pasteLines(text string) int {
	text = strings.TrimRight(strings.ReplaceAll(text, "\r\n", "\n"), "\r\n")
	return strings.Count(text, "\n") + 1
}

// Attachment is a paste sent as an embedded text resource.
type Attachment struct{ URI, Text string }

// Events and effects for pastes.
type (
	// Pasted is a bracketed paste into the input.
	Pasted struct {
		At   time.Time
		Text string
	}
	// TrustPaste marks the latest paste whose untrusted placeholder is
	// still in the input as trusted. Draft is the input's text.
	TrustPaste struct {
		At    time.Time
		Draft string
	}
)

func (e Pasted) at() time.Time     { return e.At }
func (e TrustPaste) at() time.Time { return e.At }

// InsertInput asks the shell to insert text at the cursor of Role's
// input ("" against a single agent).
type InsertInput struct{ Role, Text string }

// ReplaceInput asks the shell to replace the first Old in Role's input
// with New, where it stands.
type ReplaceInput struct{ Role, Old, New string }

func (InsertInput) effect()  {}
func (ReplaceInput) effect() {}

// Message is a short word for the status line that goes by itself.
type Message struct {
	Text string
	At   time.Time
}

// MessageFor is how long a Message shows.
const MessageFor = 6 * time.Second

// paste holds text pasted into the draft, numbered after every paste the
// draft has had (numbers are never used twice), and counted on the app,
// so the latest paste of the run can be told (Paste.Seq).
func (d *Draft) paste(a *App, text string) Paste {
	d.Pastes = slices.Clone(d.Pastes)
	for _, q := range d.Pastes {
		d.NextPaste = max(d.NextPaste, q.ID)
		a.PasteSeq = max(a.PasteSeq, q.Seq)
	}
	d.NextPaste++
	a.PasteSeq++
	p := Paste{ID: d.NextPaste, Text: text, Seq: a.PasteSeq}
	d.Pastes = append(d.Pastes, p)
	return p
}

// trust marks the latest paste still in the input, whose text is input,
// as trusted, and gives the placeholder before and after. Latest is by
// when it was pasted (Seq), not by number or place in the list, which a
// restore can reorder. Only a paste made in this run can be trusted: one
// restored from a saved view is never marked, so trusting it means
// pasting it again. With none to mark, note says why.
func (d *Draft) trust(input string) (old, new, note string) {
	best, restored := -1, false
	for i, p := range d.Pastes {
		if p.Trusted || !strings.Contains(input, p.Token()) {
			continue
		}
		if p.Seq == 0 {
			restored = true
			continue
		}
		if best < 0 || p.Seq > d.Pastes[best].Seq {
			best = i
		}
	}
	if best < 0 {
		if restored {
			return "", "", "a paste from before cannot be trusted: paste it again"
		}
		return "", "", "no untrusted paste in the input to trust"
	}
	d.Pastes = slices.Clone(d.Pastes)
	old = d.Pastes[best].Token()
	d.Pastes[best].Trusted = true
	return old, d.Pastes[best].Token(), ""
}

// take removes from the draft, and gives back, the pastes text names: a
// message sent or queued carries its pastes with it.
func (d *Draft) take(text string) []Paste {
	took := referenced(d.Pastes, text)
	d.Pastes = slices.DeleteFunc(slices.Clone(d.Pastes), func(p Paste) bool {
		return slices.ContainsFunc(took, func(q Paste) bool { return q.ID == p.ID })
	})
	return took
}

// giveBack puts pastes a message carried back in the draft, with the text
// that names them (the shell puts the text in the input): numbers are
// the draft's own, so none clashes.
func (d *Draft) giveBack(ps []Paste) {
	d.Pastes = slices.Clone(d.Pastes)
	for _, p := range ps {
		if !slices.ContainsFunc(d.Pastes, func(q Paste) bool { return q.ID == p.ID }) {
			d.Pastes = append(d.Pastes, p)
		}
		d.NextPaste = max(d.NextPaste, p.ID)
	}
}

// Queued is a message submitted while the conversation could not take it
// yet: its text, and the pastes it names, which it carries until sent.
type Queued struct {
	Text   string
	Pastes []Paste
}

// joinQueued is queued messages as one: their texts, and all their
// pastes.
func joinQueued(qs []Queued) Queued {
	var texts []string
	var pastes []Paste
	for _, q := range qs {
		texts = append(texts, q.Text)
		pastes = append(pastes, q.Pastes...)
	}
	return Queued{Text: strings.Join(texts, "\n\n"), Pastes: pastes}
}

// resolve turns the text of a prompt into what is sent: the placeholders
// it still holds become references to attachments, or, where the agent
// takes no embedded context, the pasted text itself. Pastes whose
// placeholder was deleted are not sent. inlined counts the pastes that
// went in as plain text.
func resolve(text string, pastes []Paste, embedded bool) (out string, atts []Attachment, inlined, trusted int) {
	for _, p := range pastes {
		// What the placeholder says decides, so a draft that still shows
		// "untrusted" is sent as an attachment whatever was marked.
		switch {
		case strings.Contains(text, p.untrustedToken()):
			tok := p.untrustedToken()
			if embedded {
				text = strings.ReplaceAll(text, tok, p.Ref())
				atts = append(atts, Attachment{URI: p.URI(), Text: p.Text})
			} else {
				text = strings.ReplaceAll(text, tok, p.Text)
				inlined++
			}
		case p.Trusted && strings.Contains(text, p.Token()):
			text = strings.ReplaceAll(text, p.Token(), p.Text)
			trusted++
		}
	}
	return text, atts, inlined, trusted
}
