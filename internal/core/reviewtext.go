package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Review is the text an operator consents to, T, and its digest, H, as
// relay sends them (_meta["relay/review"] on a permission request, and a
// "review" member on a card's wire form and a doc item). T is relay's,
// made safe by relay; H is the sha256 hex of its UTF-8 bytes, which an
// allow or a trust gives back so relay knows what was shown.
type Review struct {
	Text, SHA256 string
}

// RequestText is the review text relay-tui builds for a request that
// came with none: a plain ACP agent's. It follows the rules relay's own
// review text does:
//
//   - The text's own lines are its structure: labels and words of this
//     client's. A value the agent wrote never stands unquoted in one. A
//     one-line value is a quoted string on a labelled line (tool, title,
//     kind, each option and its kind, each fixed argument); a value of
//     many lines (each piece of content, relay's preview, the whole
//     input pretty-printed) is a block whose every line starts with the
//     gutter "  │ ".
//   - Inside a quoted string a backslash shows as \\, a quote as \", and
//     every character that does not show as itself, a newline included,
//     as \u{XXXX}; in a block the same, but newlines end lines. So the
//     text reads back to exactly the request (parseRequestText in the
//     tests), and passes ReviewTextOK.
//   - Text from a contaminated source has its links broken by the same
//     escapes: the ":" of a scheme's "://" as \u{003a}, and every dot (or
//     full stop) inside a word as \u{002e}, but in a plain amount such as
//     12.50. Nothing is replaced by a token, so it still reads back.
//   - "Characters to check:" names every word that mixes scripts or holds
//     a lookalike character, and every non-ASCII character in a word that
//     looks like a host or an address, with their code points and the
//     field each came from.
//
// Nothing is left out for length.
func RequestText(p Permission, session Labels) string {
	def := p.Shown().Union(session).Contaminated
	var b strings.Builder
	line := func(parts ...string) {
		for _, s := range parts {
			b.WriteString(s)
		}
		b.WriteByte('\n')
	}
	q := func(s string) string { return quoteValue(s, def) }
	var fields []field
	one := func(name, label, v string) {
		line(label, ": ", q(v))
		fields = append(fields, field{name, v})
	}
	block := func(name, head, v string, defang bool) {
		line(head)
		for _, l := range blockLines(v, defang) {
			line(Gutter, l)
		}
		fields = append(fields, field{name, v})
	}
	one("tool", "tool", p.Name)
	one("title", "title", p.Title)
	one("kind", "kind", p.Kind)
	line("session labels: ", labelWords(p.Labels.Union(session)))
	for i, c := range p.Content {
		block(fmt.Sprintf("content %d", i+1), fmt.Sprintf("content %d:", i+1), c, def)
	}
	if p.Preview != "" {
		// A service's words: always defanged, whatever the labels.
		block("preview", "preview, labelled "+labelWords(p.PreviewLabels)+":", p.Preview, true)
	}
	if in := prettyInput(p.Input); in == "" {
		line("input: none")
	} else {
		block("input", "input:", in, def)
	}
	if len(p.FixedArgs) > 0 {
		names := make([]string, len(p.FixedArgs))
		for i, n := range p.FixedArgs {
			names[i] = q(n)
			fields = append(fields, field{fmt.Sprintf("setting %d", i+1), n})
		}
		line("set by your settings: ", strings.Join(names, ", "))
	}
	for i, o := range p.Options {
		line(fmt.Sprintf("option %d: ", p.Number(i)), q(o.Name), " kind ", q(string(o.Kind)))
		fields = append(fields, field{fmt.Sprintf("option %d", p.Number(i)), o.Name})
	}
	if len(p.Options) == 0 {
		line("options: none")
	}
	line("Characters to check:")
	checks := charactersToCheck(fields, def)
	for _, c := range checks {
		line("  ", c)
	}
	if len(checks) == 0 {
		line("  none")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// Gutter is what every line of a block of the agent's text starts with
// in a review text built here, as in relay's: nothing else does, so no
// line of the agent's can pass for one of the text's own.
const Gutter = "  │ "

// field is a value of the agent's in a review text, by the name the
// characters to check give it.
type field struct{ name, value string }

// quoteValue is s as a quoted string for a review text's own line.
func quoteValue(s string, defang bool) string {
	return `"` + escapeValue(s, true, defang) + `"`
}

// blockLines is s as the lines of a block.
func blockLines(s string, defang bool) []string {
	return strings.Split(escapeValue(s, false, defang), "\n")
}

// escapeValue is s with a backslash as \\, every character that does not
// show as itself as \u{XXXX} (a byte that is not UTF-8 as \x{XX}), and,
// in a quoted string, a quote as \" and a newline as \u{000a}. With
// defang, the links in it are broken by the same escapes (see
// RequestText).
func escapeValue(s string, quoted, defang bool) string {
	var marks map[int]bool
	if defang {
		marks = linkMarks(s)
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && n == 1:
			fmt.Fprintf(&b, `\x{%02x}`, s[i])
		case marks[i]:
			fmt.Fprintf(&b, `\u{%04x}`, r)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '"' && quoted:
			b.WriteString(`\"`)
		case r == '\n' && !quoted:
			b.WriteByte('\n')
		case Shows(r):
			b.WriteRune(r)
		default:
			fmt.Fprintf(&b, `\u{%04x}`, r)
		}
		i += n
	}
	return b.String()
}

// schemeRE finds a URL scheme and its "://".
var schemeRE = regexp.MustCompile(`(?i)[a-z][a-z0-9+.\-]*://`)

// plainNumberRE is a number with dots that is not an address: an amount
// or a version (12.50, 1.5.2), with what can stand around it in prose or
// in JSON (quotes, a comma).
var plainNumberRE = regexp.MustCompile(`^["'$€£¥(+\-]*\d+(\.\d+){1,2}["'),.;:%]*$`)

// quadRE is a dotted quad: an address, whose dots break.
var quadRE = regexp.MustCompile(`\d+\.\d+\.\d+\.\d+`)

// isStop is a dot, or a full stop a host reads as one.
func isStop(r rune) bool { return r == '.' || r == '。' || r == '．' || r == '｡' }

// linkMarks is where in s a link breaks, by byte offset: the ":" of each
// scheme's "://", and each dot inside a word but a plain number's.
func linkMarks(s string) map[int]bool {
	marks := map[int]bool{}
	for _, m := range schemeRE.FindAllStringIndex(s, -1) {
		marks[m[1]-3] = true
	}
	for _, w := range words(s) {
		if plainNumberRE.MatchString(s[w[0]:w[1]]) && !quadRE.MatchString(s[w[0]:w[1]]) {
			continue
		}
		first, last := w[0], w[1]
		_, lastSize := utf8.DecodeLastRuneInString(s[w[0]:w[1]])
		for i, r := range s[w[0]:w[1]] {
			at := w[0] + i
			if isStop(r) && at != first && at != last-lastSize {
				marks[at] = true
			}
		}
	}
	return marks
}

// words is where the words of s are, as [start, end) byte offsets: runs
// of what is not white space.
func words(s string) [][2]int {
	var out [][2]int
	start := -1
	for i, r := range s {
		if unicode.IsSpace(r) {
			if start >= 0 {
				out = append(out, [2]int{start, i})
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		out = append(out, [2]int{start, len(s)})
	}
	return out
}

// labelWords names labels for a review text.
func labelWords(l Labels) string {
	var w []string
	if l.Private {
		w = append(w, "private")
	}
	if l.Contaminated {
		w = append(w, "contaminated")
	}
	if len(w) == 0 {
		return "none"
	}
	return strings.Join(w, ", ")
}

// prettyInput is a call's input indented, one member per line, with its
// numbers as written; text that is not JSON as it is; "" for none.
func prettyInput(input string) string {
	if strings.TrimSpace(input) == "" {
		return ""
	}
	var b bytes.Buffer
	if json.Valid([]byte(input)) && json.Indent(&b, []byte(input), "", "  ") == nil {
		return b.String()
	}
	return input
}

// scripts are the scripts whose letters most often pass for one
// another's, as relay.text names them.
var scripts = []struct {
	name  string
	table *unicode.RangeTable
}{
	{"Latin", unicode.Latin}, {"Cyrillic", unicode.Cyrillic}, {"Greek", unicode.Greek}, {"Armenian", unicode.Armenian},
	{"Cherokee", unicode.Cherokee}, {"Hebrew", unicode.Hebrew}, {"Arabic", unicode.Arabic}, {"Han", unicode.Han},
	{"Hiragana", unicode.Hiragana}, {"Katakana", unicode.Katakana}, {"Hangul", unicode.Hangul},
	{"Georgian", unicode.Georgian}, {"Thai", unicode.Thai}, {"Devanagari", unicode.Devanagari},
}

func scriptOf(r rune) string {
	for _, s := range scripts {
		if unicode.Is(s.table, r) {
			return s.name
		}
	}
	return ""
}

// lookalikes are letters of other scripts that pass for Latin ones.
var lookalikes = map[rune]bool{}

func init() {
	for _, r := range "авекмнорстухѕіјԁӏԛԝүһАВЕКМНОРСТХЅІЈαβεικνορτυχΑΒΕΖΗΙΚΜΝΟΡΤΥΧıɑɡℓ" {
		lookalikes[r] = true
	}
}

// lookalike says r passes for an ASCII character it is not: a letter of
// another script that looks Latin, a fullwidth form, or a mathematical
// letter or digit.
func lookalike(r rune) bool {
	return lookalikes[r] || (r >= 0xFF01 && r <= 0xFF5E) || (r >= 0x1D400 && r <= 0x1D7FF)
}

// hostLike says a word looks like a host or an address: a dot between
// two letters or digits, an @, or a "://".
func hostLike(w string) bool {
	if strings.Contains(w, "@") || strings.Contains(w, "://") {
		return true
	}
	rs := []rune(w)
	for i := 1; i+1 < len(rs); i++ {
		if isStop(rs[i]) && (unicode.IsLetter(rs[i-1]) || unicode.IsDigit(rs[i-1])) && (unicode.IsLetter(rs[i+1]) || unicode.IsDigit(rs[i+1])) {
			return true
		}
	}
	return false
}

// charactersToCheck is what a review text says to check in the agent's
// values: each word that mixes scripts or holds a lookalike, and each
// word like a host or an address with a character outside ASCII, named
// by its field, quoted, with the code points that make it so.
func charactersToCheck(fields []field, defang bool) []string {
	var out []string
	for _, f := range fields {
		seen := map[string]bool{}
		for _, at := range words(f.value) {
			w := f.value[at[0]:at[1]]
			if seen[w] {
				continue
			}
			seen[w] = true
			var found []string
			var names []string
			var cps []string
			for _, r := range w {
				if s := scriptOf(r); s != "" && unicode.IsLetter(r) && !slices.Contains(names, s) {
					names = append(names, s)
				}
			}
			slices.Sort(names)
			if len(names) > 1 {
				found = append(found, "mixes "+strings.Join(names, " and "))
			}
			host := hostLike(w)
			for _, r := range w {
				if lookalike(r) || (host && r > unicode.MaxASCII) {
					cp := fmt.Sprintf("U+%04X", r)
					if !slices.Contains(cps, cp) {
						cps = append(cps, cp)
					}
				}
			}
			if len(cps) > 0 {
				what := "holds a lookalike"
				if host {
					what = "is like an address, with characters outside ASCII"
				}
				found = append(found, what+": "+strings.Join(cps, " "))
			}
			if len(found) > 0 {
				out = append(out, f.name+": "+quoteValue(w, defang)+" "+strings.Join(found, "; "))
			}
		}
	}
	return out
}
