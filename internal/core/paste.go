package core

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// A pasted block of text is the operator's own: what Kira pastes is hers,
// and relay cleans it on arrival. It is not typed into the prompt, though.
// It is held, the input shows a placeholder so a long paste does not fill
// the box, and on send the placeholder is replaced by the text itself.
// Delete the placeholder and the paste is not sent.

// Paste is a block of pasted text held until its placeholder is sent.
type Paste struct {
	ID   int
	Text string
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
	return fmt.Sprintf("[pasted %d %s #%d]", n, unit, p.ID)
}

// NormalizeLineBreaks makes every line break LF. Terminals send CR, CRLF
// or LF for a line break in a bracketed paste.
func NormalizeLineBreaks(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
}

func pasteLines(text string) int {
	text = strings.TrimRight(NormalizeLineBreaks(text), "\n")
	return strings.Count(text, "\n") + 1
}

// Events and effects for pastes.
type (
	// Pasted is a bracketed paste into the input.
	Pasted struct {
		At   time.Time
		Text string
	}
)

func (e Pasted) at() time.Time { return e.At }

// InsertInput asks the shell to insert text at the cursor of Role's
// input ("" against a single agent).
type InsertInput struct{ Role, Text string }

func (InsertInput) effect() {}

// Message is a short word for the status line that goes by itself.
type Message struct {
	Text string
	At   time.Time
}

// MessageFor is how long a Message shows.
const MessageFor = 6 * time.Second

// paste holds text pasted into the draft, numbered after every paste the
// draft has had (numbers are never used twice).
func (d *Draft) paste(text string) Paste {
	d.Pastes = slices.Clone(d.Pastes)
	for _, q := range d.Pastes {
		d.NextPaste = max(d.NextPaste, q.ID)
	}
	d.NextPaste++
	p := Paste{ID: d.NextPaste, Text: NormalizeLineBreaks(text)}
	d.Pastes = append(d.Pastes, p)
	return p
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

// resolve turns the text of a prompt into what is sent: each placeholder
// it still holds becomes the pasted text. Pastes whose placeholder was
// deleted are not sent.
func resolve(text string, pastes []Paste) string {
	if len(pastes) == 0 {
		return text
	}
	// One pass, so pasted text that happens to spell another placeholder
	// is not expanded again.
	pairs := make([]string, 0, 2*len(pastes))
	for _, p := range pastes {
		pairs = append(pairs, p.Token(), p.Text)
	}
	return strings.NewReplacer(pairs...).Replace(text)
}
