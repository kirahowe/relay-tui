package core

// feedS applies events in turn, as the shell does, and returns the state
// and every effect.
func feedS(x Single, events ...Event) (Single, []Effect) {
	var all []Effect
	for _, e := range events {
		var fx []Effect
		x, fx = ReduceSingle(x, e)
		all = append(all, fx...)
	}
	return x, all
}

// busySingle is a single agent's session with a turn running.
func busySingle() Single {
	x, _ := feedS(Single{}, Connected{At: t0}, Submit{At: t0, Text: "go"})
	return x
}
