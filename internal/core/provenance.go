package core

import "strings"

// Provenance says in plain words where something came from, for text
// relay-tui shows the operator: "from the web" when one of its sources is
// a page, "from outside" for anything else that arrived through a tool,
// and nothing for what the operator or the house wrote. It never names a
// label: the operator is told where a thing came from, not how it was
// marked.
func Provenance(l Labels, sources []Source) string {
	if !l.Contaminated {
		return ""
	}
	for _, s := range sources {
		if webRef(s.Ref) {
			return "from the web"
		}
	}
	return "from outside"
}

// webRef says a source reference points at a page: relay's web kind
// ("web:...", or as a citation "[[web:...]]"), or a link.
func webRef(ref string) bool {
	ref = strings.ToLower(strings.TrimSpace(ref))
	for _, p := range []string{"web:", "[[web:", "http://", "https://"} {
		if strings.HasPrefix(ref, p) {
			return true
		}
	}
	return false
}
