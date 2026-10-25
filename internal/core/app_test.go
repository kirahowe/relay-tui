package core

import "time"

// wholeReview is what a renderer that draws the whole review at once
// reports, for the surface surf over a and s: the review and all of its
// text, or nothing when no review has the keys or it fails closed.
func wholeReview(surf Surface, a App, s State, at time.Time) Drawn {
	if surf.Kind != SurfaceReview {
		return Drawn{At: at}
	}
	sub := ReviewSubject(surf, a, s)
	if sub.Problem != "" {
		return Drawn{At: at}
	}
	var opts []int
	if surf.Subject == SubjectRequest && len(s.Asks) > 0 {
		for i := range s.Asks[0].Options {
			opts = append(opts, i) // every option, on a row with room
		}
	}
	return Drawn{At: at, Surfaces: []string{sub.ID}, Review: &ReviewDrawn{ID: sub.ID, From: 0, To: len(sub.Text), Options: opts}}
}

// feedS applies events in turn, as the shell does: after each, the
// review, if one has the keys, is drawn whole at once (Drawn), as a
// renderer with room for all of it would draw it.
func feedS(x Single, events ...Event) (Single, []Effect) {
	var all []Effect
	for _, e := range events {
		var fx []Effect
		x, fx = ReduceSingle(x, e)
		all = append(all, fx...)
		x, _ = ReduceSingle(x, wholeReview(SingleSurface(x), x.App, x.State, e.at()))
	}
	return x, all
}

// busySingle is a single agent's session with a turn running.
func busySingle() Single {
	x, _ := feedS(Single{}, Connected{At: t0}, Submit{At: t0, Text: "go"})
	return x
}
