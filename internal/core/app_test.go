package core

// feedS applies events in turn to the app and its conversation, and
// returns the state and all the effects.
// feedS applies events in turn, as the shell does: after each, the
// surface is drawn at once (Drawn), as a renderer would draw it.
func feedS(x Single, events ...Event) (Single, []Effect) {
	var all []Effect
	for _, e := range events {
		var fx []Effect
		x, fx = ReduceSingle(x, e)
		all = append(all, fx...)
		x, _ = ReduceSingle(x, Drawn{At: e.at(), Surface: SingleSurfaceID(x)})
	}
	return x, all
}

// busySingle is a single agent's session with a turn running.
func busySingle() Single {
	x, _ := feedS(Single{}, Connected{At: t0}, Submit{At: t0, Text: "go"})
	return x
}
