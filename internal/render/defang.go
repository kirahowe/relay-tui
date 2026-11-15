package render

import (
	"regexp"
	"strings"

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

// Defang defangs every URL in s. Every dot in it becomes [.], http and
// https become hxxp and hxxps, and any other scheme's :// becomes [:]//,
// so no "://" is left. s should be sanitized first.
func Defang(s string) string {
	dots := func(t string) string {
		return strings.ReplaceAll(strings.ReplaceAll(t, "[.]", "."), ".", "[.]")
	}
	out := urlRE.ReplaceAllStringFunc(s, func(u string) string {
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
			case "hxxp", "hxxps": // defanged already
				scheme += "://"
			default:
				// A scheme may hold dots (www.x.io://y), and so read as
				// a www. host by itself.
				scheme = dots(scheme) + "[:]//"
			}
		}
		// Every dot loses its power, in the path and query too, so a
		// second host riding in the URL (?next=www.evil.test/?d=1) is
		// broken as well as the first, as relay's own defang does.
		return scheme + dots(rest)
	})
	// A "://" can be left that the match did not own (it came from the
	// middle of another URL's host, or after a www. host's path); none
	// survives but hxxp(s)://.
	return schemeSepRE.ReplaceAllStringFunc(out, func(m string) string {
		if len(m) > 3 {
			return m
		}
		return "[:]//"
	})
}

// schemeSepRE finds "://" with a defanged http scheme before it, which
// stays, or without, which does not.
var schemeSepRE = regexp.MustCompile("(?i)hxxps?://|://")

// defangLines is a last check on rendered lines: markdown can put a
// "://" together from pieces (an emphasis mark between the slashes), which
// Defang on the source did not see. A line that shows one anywhere but in
// a whole live link's span is shown plain and defanged.
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
