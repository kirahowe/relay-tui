package render

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"

	"github.com/kirahowe/relay-tui/internal/core"
)

// When the session is contaminated, text the agent wrote may have been
// shaped by an outsider, and a link in it is a way to carry data out: a
// click sends the URL, query string and all. So such text shows its URLs
// defanged, https://evil.com/x.html?d=1 as hxxps://evil[.]com/x[.]html?d=1,
// which no terminal turns into a link. The one exception is a live link
// of relay's (links.go), the only thing here that is ever written as an
// OSC 8 hyperlink; sanitize drops the ESC that would start any other.

// urlRE finds a URL: a scheme and "://", or "www.", then everything up to
// a space or a character that usually closes a URL in prose or markdown.
// A dot that is already broken, [.], is part of it, so a URL broken once
// is found whole and Defang leaves it as it is.
var urlRE = regexp.MustCompile("(?i)(?:[a-z][a-z0-9+.\\-]*://|www\\.)(?:\\[\\.\\]|[^\\s<>\"'`)\\]\\x1b])*")

// schemeName is a scheme by itself, as it stands before "://".
var schemeName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.\-]*$`)

// Defang defangs every URL in s. A dot in it becomes [.], http and https
// become hxxp and hxxps, and any other scheme's :// becomes [:]//, so no
// "://" is left. s should be sanitized first.
//
// It changes nothing in text it or relay broke already. A dot is broken
// as relay breaks one (relay.text/break-every-dot): only with a
// character either side of it in its word, and never one that stands in
// [.] already. So the full stop that ends a sentence after a link stays
// a full stop, as relay sent it.
func Defang(s string) string {
	var b strings.Builder
	at := 0
	for _, m := range urlRE.FindAllStringIndex(s, -1) {
		u := s[m[0]:m[1]]
		scheme, rest := "", u
		// What stands before the first "://" is the scheme only when it
		// is one. In www.evil.test/?u=http://x it is a host and a path,
		// and its dots are broken with the rest.
		if i := strings.Index(u, "://"); i >= 0 && schemeName.MatchString(u[:i]) {
			scheme, rest = u[:i], u[i+3:]
			switch strings.ToLower(scheme) {
			case "http":
				scheme = "hxxp" + "://"
			case "https":
				scheme = "hxxps" + "://"
			case "hxxp", "hxxps", "fxp": // broken already, by this or by relay
				scheme += "://"
			default:
				// A scheme may hold dots (www.x.io://y), and so read as
				// a www. host by itself.
				scheme = breakDots(scheme, true) + "[:]//"
			}
		}
		// Every dot loses its power, in the path and query too, so a
		// second host riding in the URL (?next=www.evil.test/?d=1) is
		// broken as well as the first, as relay's own defang does. The
		// URL's last dot counts as inside the word when more of the word
		// follows it.
		more := m[1] < len(s) && !unicode.IsSpace(rune(s[m[1]])) && s[m[1]] != ']'
		b.WriteString(s[at:m[0]] + scheme + breakDots(rest, more))
		at = m[1]
	}
	b.WriteString(s[at:])
	// A "://" can be left that the match did not own: it came from the
	// middle of another URL's host, or after a www. host's path, or it
	// has something before it that is no scheme's letter (a hidden
	// character, a letter of another script) and a terminal's finder may
	// read as one. None survives but a broken scheme's.
	return schemeSepRE.ReplaceAllStringFunc(b.String(), func(m string) string {
		if len(m) > 3 {
			return m
		}
		return "[:]//"
	})
}

// breakDots breaks the dots in t, part of a word, that have a character
// either side: every dot but the first character, one in [.] or beside a
// bracket that would make it look so, and the last character unless more
// says the word goes on after t.
func breakDots(t string, more bool) string {
	if !strings.Contains(t, ".") {
		return t
	}
	var b strings.Builder
	for i := 0; i < len(t); i++ {
		c := t[i]
		last := i == len(t)-1
		if c != '.' || i == 0 || t[i-1] == '[' || (last && !more) || (!last && t[i+1] == ']') {
			b.WriteByte(c)
			continue
		}
		b.WriteString("[.]")
	}
	return b.String()
}

// schemeSepRE finds "://" with a broken scheme before it, which stays, or
// without, which does not. hxxp and hxxps are the broken http schemes,
// and fxp is how relay writes a broken ftp.
var schemeSepRE = regexp.MustCompile("(?i)(?:hxxps?|fxp)://|://")

// defangLines is a last check on rendered lines: markdown can put a
// "://" together from pieces (an emphasis mark between the slashes), which
// Defang on the source did not see. A line that shows one is shown plain
// and defanged, its hyperlinks gone with its styles.
func defangLines(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		if unsafeRow(l) {
			l = styled(faint, Defang(ansi.Strip(l)))
		}
		out[i] = l
	}
	return out
}

// defanged is e with its agent-written text defanged. A reply keeps its
// live links, for the Markdown renderer to draw (fangSource); a call's
// name, arguments and output, and the agent's thinking, keep none.
func defanged(e core.Entry, live core.Links) core.Entry {
	d := func(s string) string { return Defang(sanitize(s)) }
	if e.Kind == core.Agent {
		e.Text = fangSource(sanitize(e.Text), live)
	} else {
		e.Text = d(e.Text)
	}
	e.Detail = d(e.Detail)
	calls := make([]core.Call, len(e.Calls))
	for i, c := range e.Calls {
		c.Name, c.Title, c.Input, c.Output = d(c.Name), d(c.Title), d(c.Input), d(c.Output)
		calls[i] = c
	}
	e.Calls = calls
	return e
}
