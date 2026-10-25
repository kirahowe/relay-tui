package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"pgregory.net/rapid"
)

// unvisible undoes Visible: \\ is a backslash, \u{XXXX} a code point and
// \x{XX} a byte.
func unvisible(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			continue
		}
		switch s[i+1] {
		case '\\':
			b.WriteByte('\\')
			i++
		case 'u', 'x':
			end := strings.IndexByte(s[i:], '}')
			n, _ := strconv.ParseUint(s[i+3:i+end], 16, 32)
			if s[i+1] == 'u' {
				b.WriteRune(rune(n))
			} else {
				b.WriteByte(byte(n))
			}
			i += end
		}
	}
	return b.String()
}

// What Visible makes always passes the pager's check, a line holds no
// newline, and it reads back to the text it was made from: no two texts
// look the same.
func TestVisibleIsSafeAndUnambiguous(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := genText(t, "text")
		if rapid.Bool().Draw(t, "bytes") {
			s += string([]byte{0xff, 0xc3}) // not UTF-8
		}
		v, l := Visible(s), VisibleLine(s)
		if !ReviewTextOK(v) || !ReviewTextOK(l) || strings.Contains(l, "\n") {
			t.Fatalf("%q -> %q / %q", s, v, l)
		}
		if got := unvisible(v); got != s {
			t.Fatalf("%q -> %q -> %q", s, v, got)
		}
		if got := unvisible(l); got != s {
			t.Fatalf("line %q -> %q -> %q", s, l, got)
		}
	})
}

// The pager's check fails closed on every character that does not show
// as itself, wherever it is.
func TestReviewTextFailsClosed(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ok := Visible(genText(t, "around"))
		bad := rapid.SampledFrom([]rune{0, 0x1b, '\t', '\r', 0x7f, 0x85, 0xa0, 0xad, 0x200b, 0x200d, 0x202e, 0x2028,
			0x3000, 0xfe0f, 0xfeff, 0xe000, 0xe0001, 0x0378, 0x2800}).Draw(t, "bad")
		at := rapid.IntRange(0, len(ok)).Draw(t, "at")
		for at > 0 && at < len(ok) && !utf8.RuneStart(ok[at]) {
			at--
		}
		s := ok[:at] + string(bad) + ok[at:]
		if ReviewTextOK(s) {
			t.Fatalf("U+%04X passed in %q", bad, s)
		}
		if ReviewTextOK(ok + "\xff") {
			t.Fatal("a byte that is not UTF-8 passed")
		}
	})
	if !ReviewTextOK("plain text, 漢字 and 😀\nwith lines\n") || !ReviewTextOK("") {
		t.Fatal("plain text failed")
	}
}

// unescapeValue undoes escapeValue: \\, \", \u{XXXX} and \x{XX}.
func unescapeValue(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			continue
		}
		switch s[i+1] {
		case '\\', '"':
			b.WriteByte(s[i+1])
			i++
		case 'u', 'x':
			end := strings.IndexByte(s[i:], '}')
			n, _ := strconv.ParseUint(s[i+3:i+end], 16, 32)
			if s[i+1] == 'u' {
				b.WriteRune(rune(n))
			} else {
				b.WriteByte(byte(n))
			}
			i += end
		}
	}
	return b.String()
}

// unquote reads a quoted value from the start of s: the value, and what
// follows its closing quote.
func unquote(t *rapid.T, s string) (string, string) {
	if !strings.HasPrefix(s, `"`) {
		t.Fatalf("no quoted value at %q", s)
	}
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"':
			return unescapeValue(s[1:i]), s[i+1:]
		}
	}
	t.Fatalf("an unclosed value: %q", s)
	return "", ""
}

// parseRequestText reads a review text RequestText built back into the
// request it shows, and the labels it names: the other half of the
// proof that no two requests make the same text.
func parseRequestText(t *rapid.T, text string) (Permission, Labels) {
	var p Permission
	var labels Labels
	lines := strings.Split(text, "\n")
	block := func(i int) (string, int) {
		var out []string
		for i < len(lines) && strings.HasPrefix(lines[i], Gutter) {
			out = append(out, unescapeValue(strings.TrimPrefix(lines[i], Gutter)))
			i++
		}
		return strings.Join(out, "\n"), i
	}
	words := func(s string) Labels {
		return Labels{Private: strings.Contains(s, "private"), Contaminated: strings.Contains(s, "contaminated")}
	}
	for i := 0; i < len(lines); {
		l := lines[i]
		i++
		switch {
		case strings.HasPrefix(l, "tool: "):
			p.Name, _ = unquote(t, strings.TrimPrefix(l, "tool: "))
		case strings.HasPrefix(l, "title: "):
			p.Title, _ = unquote(t, strings.TrimPrefix(l, "title: "))
		case strings.HasPrefix(l, "kind: "):
			p.Kind, _ = unquote(t, strings.TrimPrefix(l, "kind: "))
		case strings.HasPrefix(l, "session labels: "):
			labels = words(strings.TrimPrefix(l, "session labels: "))
		case strings.HasPrefix(l, "content "):
			_, rest, _ := strings.Cut(l, ", ")
			var c CallContent
			switch {
			case rest == "text:":
				c.Kind = ContentText
				c.Text, i = block(i)
			case strings.HasPrefix(rest, "diff of: "):
				c.Kind = ContentDiff
				c.Path, _ = unquote(t, strings.TrimPrefix(rest, "diff of: "))
				if lines[i] == "old text: none" {
					i++
				} else if lines[i] == "old text:" {
					var old string
					old, i = block(i + 1)
					c.OldText = &old
				} else {
					t.Fatalf("no old text after a diff: %q", lines[i])
				}
				if lines[i] != "new text:" {
					t.Fatalf("no new text after a diff: %q", lines[i])
				}
				c.NewText, i = block(i + 1)
			case strings.HasPrefix(rest, "terminal: "):
				c.Kind = ContentTerminal
				c.TerminalID, _ = unquote(t, strings.TrimPrefix(rest, "terminal: "))
			case strings.HasPrefix(rest, "resource: "):
				c.Kind = ContentResource
				var more string
				c.URI, more = unquote(t, strings.TrimPrefix(rest, "resource: "))
				c.MimeType, _ = unquote(t, strings.TrimPrefix(more, " type "))
				if b, ok := strings.CutPrefix(lines[i], "blob: "); ok {
					c.Blob = true
					if _, err := fmt.Sscanf(b, "%d bytes, sha256 %s", &c.BlobSize, &c.BlobSHA256); err != nil {
						t.Fatalf("blob line %q: %v", lines[i], err)
					}
					i++
				} else if lines[i] == "text:" {
					c.Text, i = block(i + 1)
				} else {
					t.Fatalf("no text or blob after a resource: %q", lines[i])
				}
			default:
				t.Fatalf("content of no kind: %q", l)
			}
			p.Content = append(p.Content, c)
		case strings.HasPrefix(l, "preview, labelled "):
			p.PreviewLabels = words(l)
			p.Preview, i = block(i)
		case l == "input:":
			p.Input, i = block(i)
		case l == "input: none", l == "options: none":
		case strings.HasPrefix(l, "set by your settings: "):
			rest := strings.TrimPrefix(l, "set by your settings: ")
			for rest != "" {
				var v string
				v, rest = unquote(t, rest)
				p.FixedArgs = append(p.FixedArgs, v)
				rest = strings.TrimPrefix(rest, ", ")
			}
		case strings.HasPrefix(l, "option "):
			_, rest, _ := strings.Cut(l, ": ")
			var o PermOption
			o.Name, rest = unquote(t, rest)
			kind, _ := unquote(t, strings.TrimPrefix(rest, " kind "))
			o.Kind = OptionKind(kind)
			p.Options = append(p.Options, o)
		case l == "Characters to check:":
			return p, labels
		default:
			t.Fatalf("a line of no field: %q in\n%s", l, text)
		}
	}
	t.Fatalf("no characters to check in\n%s", text)
	return p, labels
}

func compactJSON(s string) string {
	var b bytes.Buffer
	if json.Compact(&b, []byte(s)) != nil {
		return s
	}
	return b.String()
}

// genValue is any text an agent might send, invalid UTF-8 and links
// included.
func genValue(t *rapid.T, label string) string {
	s := genText(t, label)
	if rapid.IntRange(0, 4).Draw(t, label+" link") == 0 {
		s += rapid.SampledFrom([]string{" https://evil.example/x?d=1 ", "pаypal.com", "12.50 ", "a@b.c", "ssh://h.x", "\xff"}).Draw(t, label+" extra")
	}
	return s
}

// genContent is any piece of content of a kind a review built here
// shows, with any text in its fields.
func genContent(t *rapid.T) CallContent {
	switch rapid.IntRange(0, 4).Draw(t, "content kind") {
	case 0:
		return CallContent{Kind: ContentText, Text: genValue(t, "text")}
	case 1:
		c := CallContent{Kind: ContentDiff, Path: genValue(t, "path"), NewText: genValue(t, "new")}
		if rapid.Bool().Draw(t, "old") {
			old := genValue(t, "old text")
			c.OldText = &old
		}
		return c
	case 2:
		return CallContent{Kind: ContentTerminal, TerminalID: genValue(t, "terminal")}
	case 3:
		return CallContent{Kind: ContentResource, URI: genValue(t, "uri"), MimeType: genValue(t, "mime"), Text: genValue(t, "resource text")}
	}
	return CallContent{Kind: ContentResource, URI: genValue(t, "uri"), MimeType: genValue(t, "mime"),
		Blob: true, BlobSize: rapid.IntRange(0, 1<<20).Draw(t, "size"), BlobSHA256: Digest(genValue(t, "blob"))}
}

func sameContent(a, b []CallContent) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := a[i], b[i]
		if (x.OldText == nil) != (y.OldText == nil) || (x.OldText != nil && *x.OldText != *y.OldText) {
			return false
		}
		x.OldText, y.OldText = nil, nil
		if x != y {
			return false
		}
	}
	return true
}

// The review text built for a plain agent's request always passes the
// pager's check and reads back to exactly the request: no two requests
// show the same. With a contaminated source no link in it is live.
func TestRequestTextReadsBackToTheRequest(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		p := Permission{ID: 1, Name: genValue(t, "name"), Title: genValue(t, "title"), Kind: genValue(t, "kind"),
			Labels: genLabels(t), PreviewLabels: genLabels(t)}
		for range rapid.IntRange(0, 3).Draw(t, "contents") {
			p.Content = append(p.Content, genContent(t))
		}
		if rapid.Bool().Draw(t, "preview") {
			p.Preview = genValue(t, "preview")
		}
		p.Input = rapid.SampledFrom([]string{"", " ", `{"to":"a‮b","n":9007199254740993}`, `{"u":"https://evil.example/x","s":"a\\\"b"}`,
			"not json \x1b[2J", `["x",{"y":null}]`}).Draw(t, "input")
		for range rapid.IntRange(0, 2).Draw(t, "fixed") {
			p.FixedArgs = append(p.FixedArgs, genValue(t, "fixed"))
		}
		for i := range rapid.IntRange(0, 3).Draw(t, "options") {
			p.Options = append(p.Options, PermOption{ID: fmt.Sprint(i), Name: genValue(t, "option"),
				Kind: OptionKind(rapid.SampledFrom([]string{"allow_once", "reject_once", `x"y`}).Draw(t, "okind"))})
		}
		session := genLabels(t)
		text := RequestText(p, session)
		if !ReviewTextOK(text) {
			t.Fatalf("not showable: %q", text)
		}
		got, labels := parseRequestText(t, text)
		if labels != p.Labels.Union(session) {
			t.Fatalf("labels %+v, want %+v", labels, p.Labels.Union(session))
		}
		if got.Name != p.Name || got.Title != p.Title || got.Kind != p.Kind || !sameContent(got.Content, p.Content) ||
			!slices.Equal(got.FixedArgs, p.FixedArgs) || got.Preview != p.Preview || (p.Preview != "" && got.PreviewLabels != p.PreviewLabels) {
			t.Fatalf("read back %+v\nwant %+v\nfrom\n%s", got, p, text)
		}
		if strings.TrimSpace(p.Input) == "" {
			if got.Input != "" {
				t.Fatalf("input %q from none", got.Input)
			}
		} else if compactJSON(got.Input) != compactJSON(p.Input) {
			t.Fatalf("input %q, want %q", got.Input, p.Input)
		}
		if len(got.Options) != len(p.Options) {
			t.Fatalf("options %+v, want %+v", got.Options, p.Options)
		}
		for i, o := range p.Options {
			if got.Options[i].Name != o.Name || got.Options[i].Kind != o.Kind {
				t.Fatalf("option %d %+v, want %+v", i, got.Options[i], o)
			}
		}
		if p.Shown().Union(session).Contaminated {
			if m := liveLink.FindString(text); m != "" {
				t.Fatalf("a live link %q in\n%s", m, text)
			}
			if m := liveDot.FindString(text); m != "" {
				t.Fatalf("a live host dot %q in\n%s", m, text)
			}
		}
	})
}

var (
	liveLink = regexp.MustCompile(`(?i)[a-z][a-z0-9+.\-]*://`)
	liveDot  = regexp.MustCompile(`[\pL][.。．｡][\pL]`)
)

// Words that mix scripts, hold a lookalike, or are like an address with
// characters outside ASCII are named, with their code points and field.
func TestCharactersToCheck(t *testing.T) {
	text := RequestText(Permission{Name: "send", Input: `{"to":"bob@exаmple.com","note":"Ηello"}`,
		Options: []PermOption{{Name: "Allоw", Kind: AllowOnce}}}, Labels{})
	_, checks, _ := strings.Cut(text, "Characters to check:\n")
	for _, want := range []string{
		`input: "\"bob@exаmple.com\","`, "U+0430", `input: "\"Ηello\""`, "mixes Greek and Latin", "U+0397",
		`option 1: "Allоw" mixes Cyrillic and Latin; holds a lookalike: U+043E`,
	} {
		if !strings.Contains(checks, want) {
			t.Errorf("%q missing from\n%s", want, checks)
		}
	}
	plain := RequestText(Permission{Name: "send", Input: `{"to":"bob@example.com"}`}, Labels{})
	if !strings.HasSuffix(plain, "Characters to check:\n  none") {
		t.Errorf("plain text has something to check:\n%s", plain)
	}
	// A contaminated session's links are broken by escapes, and amounts
	// keep their dots.
	def := RequestText(Permission{Name: "fetch", Input: `{"url":"https://evil.example/x","amount":"12.50"}`}, Labels{Contaminated: true})
	for _, want := range []string{`"https\u{003a}//evil\u{002e}example/x"`, `"12.50"`} {
		if !strings.Contains(def, want) {
			t.Errorf("%q missing from\n%s", want, def)
		}
	}
}

// askedSingle is a single agent's session moving, with a request on
// offer whose review is not open. A relay review is given when rv is
// set.
func askedSingle(rv *Review, input string) Single {
	x := busySingle()
	x, _ = feedS(x, StopTyping{At: t0}, PermissionAsked{At: t0, Permission: Permission{ID: 1, CallID: "c1", Name: "send_mail",
		Input: input, Review: rv, Options: []PermOption{{ID: "a", Kind: AllowOnce}, {ID: "r", Kind: RejectOnce}}}})
	return x
}

func allowed(fx []Effect) []AnswerPermission {
	var out []AnswerPermission
	for _, a := range answers(fx) {
		if a.OptionID == "a" {
			out = append(out, a)
		}
	}
	return out
}

// The core property of the review: a request's allow goes out only when
// every part of its text was drawn, by the pages the pager reported, and
// at least ArmDelay after the last part not drawn before, by a live key
// with the review the surface. A reject or a cancel is taken at once,
// from a live key. Replayed keys answer nothing.
func TestAnAllowNeedsEveryPageReadAndTheDelay(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		input := fmt.Sprintf(`{"body":%q}`, strings.Repeat("word ", rapid.IntRange(0, 200).Draw(t, "words")))
		x := askedSingle(nil, input)
		x, _ = ReduceSingle(x, OpenReview{At: t0})
		sub := ReviewSubject(SingleSurface(x), x.App, x.State)
		n := len(sub.Text)
		covered := make([]bool, n)
		var lastNew time.Time
		read := func() bool {
			for _, c := range covered {
				if !c {
					return false
				}
			}
			return true
		}
		at := t0
		for step := range rapid.IntRange(1, 60).Draw(t, "steps") {
			at = at.Add(time.Duration(rapid.IntRange(0, 400).Draw(t, "ms")) * time.Millisecond)
			if len(x.State.Asks) == 0 {
				return
			}
			switch rapid.IntRange(0, 3).Draw(t, "what") {
			case 0, 1: // a page drawn
				from := rapid.IntRange(0, n).Draw(t, "from")
				to := rapid.IntRange(from, min(n, from+rapid.IntRange(0, 300).Draw(t, "len"))).Draw(t, "to")
				id := SingleSurfaceID(x)
				x, _ = ReduceSingle(x, Drawn{At: at, Surfaces: []string{id}, Review: &ReviewDrawn{ID: id, From: from, To: to, Pick: x.State.Pick}})
				grew := false
				for i := from; i < to; i++ {
					grew = grew || !covered[i]
					covered[i] = true
				}
				if grew || (lastNew.IsZero() && read()) {
					lastNew = at
				}
			default:
				replayed := rapid.IntRange(0, 3).Draw(t, "replayed") == 0
				x, _ = ReduceSingle(x, KeyPressed{At: at, Replayed: replayed})
				var e Event = PermissionQuick{At: at, Kind: AllowOnce}
				next, fx := ReduceSingle(x, e)
				if a := allowed(fx); len(a) > 0 {
					if replayed || !read() || at.Sub(lastNew) < ArmDelay {
						t.Fatalf("step %d: allowed (replayed %v, read %v, %v after the last new part)", step, replayed, read(), at.Sub(lastNew))
					}
					return
				}
				if !replayed && read() && !lastNew.IsZero() && at.Sub(lastNew) >= ArmDelay && x.State.Pick >= 0 &&
					x.State.Asks[0].Options[x.State.Pick].Kind == AllowOnce {
					t.Fatalf("step %d: a live allow was refused after reading it all: %q", step, next.State.Message.Text)
				}
				x = next
			}
		}
	})
}

// Reject and cancel are taken at once from the review, with a live key;
// a replayed one does nothing.
func TestRejectAndCancelAtOnce(t *testing.T) {
	for _, e := range []Event{PermissionQuick{At: t0, Kind: RejectOnce}, PermissionCancel{At: t0}} {
		x := askedSingle(nil, `{"a":1}`)
		x, _ = ReduceSingle(x, OpenReview{At: t0})
		if _, fx := ReduceSingle(x, e); len(answers(fx)) != 1 || len(allowed(fx)) != 0 {
			t.Fatalf("%T not taken at once: %v", e, fx)
		}
		y, _ := ReduceSingle(x, KeyPressed{At: t0, Replayed: true})
		if _, fx := ReduceSingle(y, e); len(fx) != 0 {
			t.Fatalf("a replayed %T answered: %v", e, fx)
		}
	}
}

// The dashboard's summary answers nothing: every answering key is
// refused there, however late, and only enter opens the review.
func TestTheSummaryAuthorizesNothing(t *testing.T) {
	x := askedSingle(nil, `{"a":1}`)
	if SingleSurface(x).Kind != SurfaceRequest {
		t.Fatalf("surface %+v", SingleSurface(x))
	}
	late := t0.Add(time.Hour)
	for _, e := range []Event{PermissionQuick{At: late, Kind: AllowOnce}, PermissionQuick{At: late, Kind: RejectOnce},
		PermissionConfirm{At: late}, PermissionCancel{At: late}, PaneKey{At: late, Action: PaneYes}} {
		if _, fx := feedS(x, e); len(fx) != 0 {
			t.Fatalf("%T on the summary gave %v", e, fx)
		}
	}
	x, _ = feedS(x, OpenReview{At: late})
	if s := SingleSurface(x); s.Kind != SurfaceReview || s.Subject != SubjectRequest {
		t.Fatalf("enter did not open the review: %+v", s)
	}
	if _, fx := feedS(x, PermissionQuick{At: late.Add(ArmDelay), Kind: AllowOnce}); len(allowed(fx)) != 1 {
		t.Fatalf("not allowed from the read review: %v", fx)
	}
	// q goes back, answering nothing.
	x, fx := feedS(x, ReviewKey{At: late, Action: ReviewBack})
	if len(fx) != 0 || SingleSurface(x).Kind != SurfaceRequest {
		t.Fatalf("back: %v %+v", fx, SingleSurface(x))
	}
}

// relay's review is shown as sent, and the allow gives its digest back.
// A review whose digest does not match, or that holds a character that
// does not show, fails closed: nothing can be allowed.
func TestRelayReviewDigest(t *testing.T) {
	good := reviewOf("tool call: send_mail\ninput: {}")
	x := askedSingle(good, `{}`)
	x, _ = feedS(x, OpenReview{At: t0})
	if sub := ReviewSubject(SingleSurface(x), x.App, x.State); sub.Text != good.Text || sub.Digest != good.SHA256 || sub.Problem != "" {
		t.Fatalf("subject %+v", sub)
	}
	_, fx := feedS(x, PermissionQuick{At: t0.Add(ArmDelay), Kind: AllowOnce})
	if a := allowed(fx); len(a) != 1 || a[0].Review != good.SHA256 {
		t.Fatalf("allow %+v", fx)
	}
	// A reject carries no digest.
	_, fx = feedS(x, PermissionQuick{At: t0, Kind: RejectOnce})
	if a := answers(fx); len(a) != 1 || a[0].Review != "" {
		t.Fatalf("reject %+v", a)
	}
	for name, rv := range map[string]*Review{
		"wrong digest": {Text: good.Text, SHA256: Digest("other")},
		"a control":    reviewOf("send\x1b[2J"),
		"a bidi mark":  reviewOf("pay‮evil"),
		"a tab":        reviewOf("a\tb"),
	} {
		x := askedSingle(rv, `{}`)
		x, _ = feedS(x, OpenReview{At: t0})
		sub := ReviewSubject(SingleSurface(x), x.App, x.State)
		x, fx := feedS(x, PermissionQuick{At: t0.Add(time.Hour), Kind: AllowOnce})
		if sub.Problem == "" || len(allowed(fx)) != 0 || !strings.Contains(x.State.Message.Text, "nothing can be allowed") {
			t.Fatalf("%s: problem %q, effects %v, note %q", name, sub.Problem, fx, x.State.Message.Text)
		}
		if _, fx := feedS(x, PermissionQuick{At: t0.Add(time.Hour), Kind: RejectOnce}); len(answers(fx)) != 1 {
			t.Fatalf("%s: rejecting must still work", name)
		}
	}
}

// When relay refuses an allow and asks again for the same tool call,
// the review opens on it again, with relay's word, and arms afresh.
func TestARefusedAllowStaysInTheReview(t *testing.T) {
	rv := reviewOf("tool call: send_mail")
	x := askedSingle(rv, `{}`)
	x, _ = feedS(x, OpenReview{At: t0}, PermissionQuick{At: t0.Add(ArmDelay), Kind: AllowOnce})
	at := t0.Add(time.Second)
	x, _ = feedS(x, PermissionAsked{At: at, Permission: Permission{ID: 2, CallID: "c1", Name: "send_mail", Review: rv,
		Refused: "the review digest does not match", Options: []PermOption{{ID: "a", Kind: AllowOnce}, {ID: "r", Kind: RejectOnce}}}})
	surf := SingleSurface(x)
	sub := ReviewSubject(surf, x.App, x.State)
	if surf.Kind != SurfaceReview || !strings.Contains(sub.Note, "does not match") {
		t.Fatalf("surface %+v, note %q", surf, sub.Note)
	}
	if _, fx := feedS(x, PermissionQuick{At: at.Add(ArmDelay - time.Millisecond), Kind: AllowOnce}); len(fx) != 0 {
		t.Fatalf("the re-asked request was armed at once: %v", fx)
	}
	if _, fx := feedS(x, PermissionQuick{At: at.Add(ArmDelay), Kind: AllowOnce}); len(allowed(fx)) != 1 {
		t.Fatalf("not allowed once read: %v", fx)
	}
	// Another request for another call does not open the review.
	y := askedSingle(rv, `{}`)
	y, _ = feedS(y, OpenReview{At: t0}, PermissionQuick{At: t0.Add(ArmDelay), Kind: AllowOnce},
		PermissionAsked{At: at, Permission: Permission{ID: 3, CallID: "other", Refused: "x", Options: []PermOption{{ID: "a", Kind: AllowOnce}}}})
	if SingleSurface(y).Kind != SurfaceRequest {
		t.Fatalf("another call opened the review: %+v", SingleSurface(y))
	}
}

// A review whose text changes (the session became contaminated, so a
// plain agent's request is defanged now) is read again from the start.
func TestAChangedReviewIsReadAgain(t *testing.T) {
	x := askedSingle(nil, `{"url":"https://example.com/x"}`)
	x, _ = feedS(x, OpenReview{At: t0})
	before := SingleSurfaceID(x)
	x, _ = ReduceSingle(x, LabelsSeen{At: t0.Add(time.Second), Labels: Labels{Contaminated: true}})
	if SingleSurfaceID(x) == before || x.App.Pager.Read() {
		t.Fatalf("the review kept its reading: %q %+v", SingleSurfaceID(x), x.App.Pager)
	}
	if _, fx := ReduceSingle(x, PermissionQuick{At: t0.Add(time.Hour), Kind: AllowOnce}); len(fx) != 0 {
		t.Fatalf("allowed a review not read since it changed: %v", fx)
	}
	if sub := ReviewSubject(SingleSurface(x), x.App, x.State); !strings.Contains(sub.Text, `https\u{003a}//example\u{002e}com/x`) {
		t.Fatalf("not defanged: %q", sub.Text)
	}
}

// Codex round 1, finding 2: a diff's fields stay apart, so a diff from
// "safe" to "danger" and a new file whose text spells that out are two
// texts; and content the review cannot show makes it fail closed for an
// allow, while a refusal still works.
func TestContentFieldsStayApart(t *testing.T) {
	safe := "safe"
	a := RequestText(Permission{Name: "edit", Content: []CallContent{{Kind: ContentDiff, Path: "f", OldText: &safe, NewText: "danger"}}}, Labels{})
	b := RequestText(Permission{Name: "edit", Content: []CallContent{{Kind: ContentDiff, Path: "f", NewText: "was:\nsafe\nnow:\ndanger"}}}, Labels{})
	if a == b {
		t.Fatalf("two diffs read the same:\n%s", a)
	}
	x := askedSingle(nil, `{}`)
	x.State.Asks = append([]Permission(nil), x.State.Asks...)
	x.State.Asks[0].Unshown = []string{`content 2 (of type "image")`}
	x, _ = feedS(x, OpenReview{At: t0})
	sub := ReviewSubject(SingleSurface(x), x.App, x.State)
	if !strings.Contains(sub.Problem, `content 2 (of type "image")`) {
		t.Fatalf("problem %q", sub.Problem)
	}
	if _, fx := feedS(x, PermissionQuick{At: t0.Add(time.Hour), Kind: AllowOnce}); len(allowed(fx)) != 0 {
		t.Fatalf("allowed content it could not show: %v", fx)
	}
	if _, fx := feedS(x, PermissionQuick{At: t0, Kind: RejectOnce}); len(answers(fx)) != 1 {
		t.Fatal("rejecting must still work")
	}
}

// Codex round 1, finding 3: enter sends the highlighted option, so it
// allows only once a frame has drawn that option whole, ArmDelay after
// the highlight last moved; y, which names its option, needs only the
// review read. Frames here draw the highlight whole or not at random (a
// narrow row may not hold it).
func TestEnterNeedsTheHighlightOnScreen(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		x := busySingle()
		x, _ = feedS(x, StopTyping{At: t0}, PermissionAsked{At: t0, Permission: Permission{ID: 1, CallID: "c1", Name: "send",
			Options: []PermOption{{ID: "a", Kind: AllowOnce}, {ID: "r", Kind: RejectOnce}, {ID: "b", Kind: AllowOnce}}}})
		x, _ = ReduceSingle(x, OpenReview{At: t0})
		id := SingleSurfaceID(x)
		n := len(ReviewSubject(SingleSurface(x), x.App, x.State).Text)
		x, _ = ReduceSingle(x, Drawn{At: t0, Surfaces: []string{id}, Review: &ReviewDrawn{ID: id, To: n, Pick: -1}}) // read, highlight cut
		var pickAt time.Time                                                                                         // when the highlight, as it is now, was first drawn whole
		at := t0
		for range rapid.IntRange(1, 40).Draw(t, "steps") {
			at = at.Add(time.Duration(rapid.IntRange(0, 500).Draw(t, "ms")) * time.Millisecond)
			if len(x.State.Asks) == 0 {
				return
			}
			switch rapid.IntRange(0, 2).Draw(t, "what") {
			case 0:
				x, _ = ReduceSingle(x, PermissionMove{At: at, Delta: rapid.SampledFrom([]int{-1, 1}).Draw(t, "delta")})
				pickAt = time.Time{}
			case 1:
				whole := rapid.Bool().Draw(t, "whole")
				pick := -1
				if whole {
					pick = x.State.Pick
					if pickAt.IsZero() {
						pickAt = at
					}
				} else {
					pickAt = time.Time{}
				}
				x, _ = ReduceSingle(x, Drawn{At: at, Surfaces: []string{id}, Review: &ReviewDrawn{ID: id, To: n, Pick: pick}})
			default:
				allow := x.State.Asks[0].Options[x.State.Pick].Kind == AllowOnce
				next, fx := ReduceSingle(x, PermissionConfirm{At: at})
				ok := !pickAt.IsZero() && at.Sub(pickAt) >= ArmDelay
				switch {
				case allow && len(allowed(fx)) > 0 && !ok:
					t.Fatalf("enter allowed with the highlight drawn whole since %v (%v)", pickAt, at.Sub(pickAt))
				case allow && len(allowed(fx)) == 0 && ok:
					t.Fatalf("enter refused with the highlight on screen %v: %q", at.Sub(pickAt), next.State.Message.Text)
				case !allow && len(answers(fx)) != 1:
					t.Fatalf("enter on a reject did not answer at once: %v", fx)
				}
				x = next
			}
		}
	})
}
