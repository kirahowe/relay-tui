package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"
)

// This file is the one display function for what the operator consents
// to, and the check the review pager trusts. It follows relay.text:
// Visible shows every character that does not print as \u{XXXX}, braced
// so the escape cannot run into the text after it, and a backslash as
// two, so text that spells out an escape cannot pass for one. No two
// texts look the same. A review text built here (RequestText) escapes
// the same way, and the pager shows a text only when ReviewTextOK says
// every character in it is one the screen shows as itself.

// Shows says r is one the review screen shows as itself: a printable
// character (no control, format, separator other than space, private
// use, surrogate, unassigned code point, or one that draws as nothing),
// or a space.
func Shows(r rune) bool { return r == ' ' || printable(r) }

// ReviewTextOK says t may be shown as a review: valid UTF-8 whose every
// character Shows, or is a newline. Anything else and the pager fails
// closed: it says why and arms nothing.
func ReviewTextOK(t string) bool { return reviewTextProblem(t) == "" }

// reviewTextProblem is why t cannot be shown as a review, or "".
func reviewTextProblem(t string) string {
	for i := 0; i < len(t); {
		r, n := utf8.DecodeRuneInString(t[i:])
		switch {
		case r == utf8.RuneError && n == 1:
			return fmt.Sprintf("byte %d is not UTF-8", i)
		case r != '\n' && !Shows(r):
			return fmt.Sprintf("it holds U+%04X, which does not show as itself", r)
		}
		i += n
	}
	return ""
}

// Digest is the sha256 hex of t's UTF-8 bytes, as relay sends H.
func Digest(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}

// visible is s for the screen: every character that does not show as
// itself as \u{XXXX} (a newline too when line is set, so nothing in it
// can start another line), a byte that is not UTF-8 as \x{XX}, and a
// backslash as two.
func visible(s string, line bool) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && n == 1:
			fmt.Fprintf(&b, `\x{%02x}`, s[i])
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n' && !line:
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

// Visible is s for the screen, newlines kept (see visible). What it
// returns always passes ReviewTextOK.
func Visible(s string) string { return visible(s, false) }

// VisibleLine is Visible for one line: a newline shows as \u{000a}.
func VisibleLine(s string) string { return visible(s, true) }
