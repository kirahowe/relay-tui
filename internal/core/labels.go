package core

import "time"

// Labels is what the session has seen, as the agent reports it. They only
// grow: Union is all the core ever does to them. relay-tui never shows a
// label by its name. Contaminated only decides whether links in text are
// broken so a click cannot carry data out, and where a thing came from
// is told in plain words (see Provenance).
type Labels struct{ Private, Contaminated bool }

// Union is the labels of l and o together.
func (l Labels) Union(o Labels) Labels {
	return Labels{Private: l.Private || o.Private, Contaminated: l.Contaminated || o.Contaminated}
}

// Empty reports whether there is no label.
func (l Labels) Empty() bool { return l == Labels{} }

// LabelsSeen is the session's labels as of an update. It can only add to
// what the core already knows.
type LabelsSeen struct {
	At     time.Time
	Labels Labels
}

func (e LabelsSeen) at() time.Time { return e.At }
