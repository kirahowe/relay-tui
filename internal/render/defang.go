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
//
// A word is first judged with its characters that take no cell skipped
// (unhide), so a mark inside an address does not hide it.
func Defang(s string) string { return breakLinks(unhide(s)) }

// unhide is s with each word whose characters that take no cell
// (core.NoAdvance) hide an address made that address, broken (relay#130).
// Defang finds a link by its letters, so a mark or a jamo after "https:"
// or inside www hides the link from it, while a finder that skips such
// characters follows it. Where the word as skipped holds a link that the
// word as written does not, the skipped form is broken and shown in its
// place. Every other word keeps its marks, so ordinary text in a script
// that uses them is left as written.
func unhide(s string) string {
	if !holdsNoAdvance(s) {
		return s
	}
	return wordRE.ReplaceAllStringFunc(s, func(w string) string {
		sk := withoutNoAdvance(w)
		if sk == w {
			return w
		}
		broken := breakLinks(sk)
		if broken == sk || withoutNoAdvance(breakLinks(w)) == broken {
			return w
		}
		return broken
	})
}

// holdsNoAdvance says s holds a character that takes no cell. ASCII
// holds none, and most text is ASCII.
func holdsNoAdvance(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return strings.IndexFunc(s[i:], core.NoAdvance) >= 0
		}
	}
	return false
}

// withoutNoAdvance is s with every character that takes no cell taken out.
func withoutNoAdvance(s string) string {
	return strings.Map(func(r rune) rune {
		if core.NoAdvance(r) {
			return -1
		}
		return r
	}, s)
}

// breakLinks is Defang on text whose words are judged as written.
func breakLinks(s string) string {
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
	out := schemeSepRE.ReplaceAllStringFunc(b.String(), func(m string) string {
		if len(m) > 3 {
			return m
		}
		return "[:]//"
	})
	return breakColonSchemes(out)
}

// A link finder follows some schemes with a colon and no "//" after it,
// and Defang breaks those as relay.text/defang does (relay#126). A web
// scheme written short (https:/x.example, https:x.example,
// https:\\x.example) becomes hxxp:, hxxps: or fxp:. One a terminal's
// finder takes with no slashes at all (Ghostty's takes each of these)
// has its colon bracketed: ssh[:]user@host. mailto:, tel: and sms: are
// left alone: whether those are broken is relay#121's question.

// colonNames are those schemes' names. A name that ends another comes
// after it, so sftp is found before ftp.
var colonNames = []string{"https", "http", "sftp", "ftp", "ssh", "file", "magnet", "news", "git"}

// colonScheme is one scheme in a string that a link finder would follow
// with no "//" after its colon: where its name starts, where its colon
// is, and whether it is a web scheme.
type colonScheme struct {
	start, colon int
	web          bool
}

// colonSchemes finds each such scheme in s. Its name must not run on from
// an ASCII letter or digit before it (xssh: and Makefile:12 are left, as
// relay leaves them); after anything else it is a scheme, a dot, a hyphen
// or a broken dot's bracket included, since a terminal's finder (Ghostty's)
// looks for one with no boundary before it. "//" must
// not follow the colon (that form has its own rule). A web scheme counts
// when a slash, a backslash or an ASCII letter or digit follows, the
// others when anything but a space does. Defang breaks what this finds,
// and the check on rows as drawn (linkable) asks the same function, so
// the two cannot come to differ. It goes from colon to colon by hand:
// every row drawn passes through it.
func colonSchemes(s string) []colonScheme {
	var out []colonScheme
	for at := 0; ; {
		i := strings.IndexByte(s[at:], ':')
		if i < 0 {
			return out
		}
		colon := at + i
		at = colon + 1
		name := ""
		for _, n := range colonNames {
			if colon >= len(n) && strings.EqualFold(s[colon-len(n):colon], n) {
				name = n
				break
			}
		}
		start := colon - len(name)
		rest := s[colon+1:]
		if name == "" || (start > 0 && alnum(s[start-1])) || rest == "" || strings.HasPrefix(rest, "//") {
			continue
		}
		c := rest[0]
		// relay breaks before it escapes a hidden character, so a scheme
		// it left before one (https:\u{0080}//) is no short scheme: the
		// escape is the hidden character's, shown.
		if strings.HasPrefix(rest, `\u{`) {
			continue
		}
		switch name {
		case "http", "https", "ftp":
			if c == '/' || c == '\\' || (c >= '0' && c <= '9') || (c|0x20 >= 'a' && c|0x20 <= 'z') {
				out = append(out, colonScheme{start, colon, true})
			}
		default:
			if !strings.ContainsRune(" \t\n\v\f\r", rune(c)) {
				out = append(out, colonScheme{start, colon, false})
			}
		}
	}
}

// alnum says c is an ASCII letter or digit: what a scheme's name may run
// on from and still be part of another word, as relay.text has it.
func alnum(c byte) bool {
	return (c|0x20 >= 'a' && c|0x20 <= 'z') || (c >= '0' && c <= '9')
}

// schemeChar says c can stand in a scheme's name.
func schemeChar(c byte) bool {
	return (c|0x20 >= 'a' && c|0x20 <= 'z') || (c >= '0' && c <= '9') || c == '+' || c == '.' || c == '-'
}

// The names as they stand once their colon form is broken: a web
// scheme's, before a colon, and the others', before a bracketed one.
var (
	brokenWebNames = []string{"hxxps", "hxxp", "fxp"}
	slashlessNames = []string{"sftp", "ssh", "file", "magnet", "news", "git"}
)

// brokenColon says a word holds a scheme whose colon form is broken, with
// something after it: a web scheme written short (hxxps:/x), or a
// bracketed colon after one of the others (ssh[:]x). A name that runs on
// from another is no scheme, and a "//" after it is the other rule's.
func brokenColon(w string) bool {
	for at := 0; ; {
		i := strings.IndexByte(w[at:], ':')
		if i < 0 {
			return false
		}
		colon := at + i
		at = colon + 1
		// Where the name ends, which names it may be, and what follows.
		end, names, after := colon, brokenWebNames, w[colon+1:]
		if colon > 0 && w[colon-1] == '[' && strings.HasPrefix(after, "]") {
			end, names, after = colon-1, slashlessNames, after[1:]
		}
		if after == "" || strings.HasPrefix(after, "//") {
			continue
		}
		for _, n := range names {
			if end >= len(n) && strings.EqualFold(w[end-len(n):end], n) {
				if start := end - len(n); start == 0 || !alnum(w[start-1]) {
					return true
				}
				break
			}
		}
	}
}

// breakColonSchemes breaks the schemes colonSchemes finds, and then every
// dot in a word that holds one, broken here or before: with the scheme
// gone a finder could still take the host for a bare name. A dot broken
// that way can lay bare a scheme that ran on from it (x.ssh:u@host reads
// x[.]ssh:u@host), so it goes round until nothing changes.
func breakColonSchemes(s string) string {
	for range 8 {
		next := breakColonWords(breakColons(s))
		if next == s {
			break
		}
		s = next
	}
	return s
}

// breakColons breaks each scheme colonSchemes finds in s.
func breakColons(s string) string {
	found := colonSchemes(s)
	if len(found) == 0 {
		return s
	}
	var b strings.Builder
	at := 0
	for _, c := range found {
		b.WriteString(s[at:c.start])
		if c.web {
			switch strings.ToLower(s[c.start:c.colon]) {
			case "http":
				b.WriteString("hxxp:")
			case "https":
				b.WriteString("hxxps:")
			default:
				b.WriteString("fxp:")
			}
		} else {
			b.WriteString(s[c.start:c.colon] + "[:]")
		}
		at = c.colon + 1
	}
	return b.String() + s[at:]
}

// breakColonWords breaks every dot in a word that holds a scheme whose
// colon form is broken.
func breakColonWords(s string) string {
	// Looked at whole first, which finds any word that will count and
	// costs no pattern: most text holds none.
	if !strings.Contains(s, ".") || !brokenColon(s) {
		return s
	}
	return wordRE.ReplaceAllStringFunc(s, func(w string) string {
		if strings.Contains(w, ".") && brokenColon(w) {
			return breakDots(w, false)
		}
		return w
	})
}

// cutSchemes looks at the rows text was wrapped into for a scheme a cut
// laid bare. Defang leaves a name that runs on from another, xssh: or
// long-https:/, since it is no scheme. A row that begins in the middle of
// such a word, after a hard cut or a break at a hyphen, can begin with
// ssh: or https:/ all the same, and there a link finder reads it as one.
// Such a row has that scheme broken, as Defang would have broken it at a
// word's start, and is cut to width if the brackets made it longer. Only
// a row that starts mid-word is touched, so text that was wrapped at a
// space reads as it was written. s is the text before it was wrapped.
func cutSchemes(s string, rows []string, width int) []string {
	if len(rows) < 2 || !strings.Contains(s, ":") {
		return rows
	}
	// Most text has no such row: only then is the text gone through for
	// where each row began.
	any := false
	for _, r := range rows[1:] {
		if strings.Contains(r, ":") {
			if found := colonSchemes(ansi.Strip(r)); len(found) > 0 && found[0].start == 0 && found[0].web {
				any = true
				break
			}
		}
	}
	if !any {
		return rows
	}
	plain := ansi.Strip(s)
	pos := 0
	for i, r := range rows {
		row := ansi.Strip(r)
		if row == "" {
			continue
		}
		k := strings.Index(plain[pos:], row)
		// Not found is a row the wrapper changed: it is looked at, to be
		// safe.
		mid := i > 0 && (k < 0 || (k == 0 && pos > 0 && !strings.ContainsRune(" \t\n\v\f\r", rune(plain[pos-1]))))
		if k >= 0 {
			pos += k + len(row)
		}
		if !mid {
			continue
		}
		// Only a web scheme: its broken name is as long as its name, so
		// the row keeps every character. A slashless one can start a row
		// only where a cut parted it from a letter or digit, in a word
		// relay leaves as written, and is left with it.
		if found := colonSchemes(row); len(found) > 0 && found[0].start == 0 && found[0].web {
			rows[i] = breakRowStart(r, found[0], width)
		}
	}
	return rows
}

// escRE finds an escape sequence a row may hold: a style, or the start or
// end of a hyperlink.
var escRE = regexp.MustCompile("^(?:\x1b\\[[0-9;]*m|\x1b\\]8;;[^\x07\x1b]*\x07)")

// breakRowStart breaks the scheme c, which starts a styled row's text:
// a web scheme's letters, or the colon of one of the others. The row's
// styles and hyperlinks stay where they are.
func breakRowStart(row string, c colonScheme, width int) string {
	name := ""
	if c.web {
		name = map[int]string{4: "hxxp", 5: "hxxps", 3: "fxp"}[c.colon-c.start]
	}
	var b strings.Builder
	for at := 0; len(row) > 0; {
		if loc := escRE.FindStringIndex(row); loc != nil {
			b.WriteString(row[:loc[1]])
			row = row[loc[1]:]
			continue
		}
		switch {
		case name != "" && at >= c.start && at < c.colon:
			b.WriteByte(name[at-c.start])
		case name == "" && at == c.colon:
			b.WriteString("[:]")
		default:
			b.WriteByte(row[0])
		}
		row = row[1:]
		at++
	}
	return truncate(b.String(), width)
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

// seen is text whose links are about to be broken, sanitized, with every
// hidden character gone first but the ones that hold an emoji together
// (core.DropHidden). Defang finds a link by its letters, so a character
// nobody sees inside "://" or "www." would hide the link from it and may
// not hide it from a terminal. The order matters: taken out first, the
// pieces come together and Defang sees the word a terminal would.
func seen(s string) string { return sanitize(core.DropHidden(s)) }

// defanged is e with its agent-written text defanged. A reply keeps its
// live links, for the Markdown renderer to draw (fangSource); a call's
// name, arguments and output, and the agent's thinking, keep none.
func defanged(e core.Entry, live core.Links) core.Entry {
	d := func(s string) string { return Defang(seen(s)) }
	if e.Kind == core.Agent {
		e.Text = fangSource(seen(e.Text), live)
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

// Against a relay that makes a contaminated session's text inert before
// any client sees it (relay/inertText), that text is drawn as relay sent
// it: every link broken, hidden characters shown as escapes, and
// Markdown's link and HTML syntax escaped. A breaker of the client's own
// would add nothing, and it would read relay's escapes as text to break,
// so none runs. A reply's hyperlinks are the links relay listed for its
// message (anchoredSource). The checks on the rows drawn and on the frame
// (defangLines, gate) stay where they are.

// anchoredSource is a reply's Markdown source with each link relay listed
// for it made a Markdown autolink, where its shown form stands in a word
// as relay wrote it (core.Anchored) and its address is a live link: the
// renderer then draws the hyperlink over the address's broken form, which
// is what relay showed (mdRenderer.link). Every other word is left as it
// came, and no address is read out of the text.
//
// anchored is the set the renderer may draw as links: the addresses
// written here, and only those the text did not hold whole already. An
// autolink the text wrote itself, which relay escapes and so never sends,
// is then drawn as text.
func anchoredSource(t string, own []core.Link, live core.Links) (src string, anchored core.Links) {
	if len(own) == 0 || !strings.Contains(t, "hxxp") {
		return t, core.Links{}
	}
	var made []string
	src = wordRE.ReplaceAllStringFunc(t, func(w string) string {
		before, l, after, ok := core.Anchored(w, own)
		if !ok || !live.Has(l.Address) || strings.Contains(t, l.Address) {
			return w
		}
		made = append(made, l.Address)
		return before + autolink(l.Address) + after
	})
	return src, core.LinksOf(made...)
}
