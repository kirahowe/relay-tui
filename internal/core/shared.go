package core

import "time"

// A State is Shared when it is one conversation among several on one
// connection (hub mode). Another device may be driving it, and an
// attach is followed by a replay of what already happened, so its
// updates are taken in any phase, not only inside a prompt of our own.
// Its transcript is always the full-screen view, which the hub draws,
// so nothing goes to scrollback and the view never closes.

// Events about a conversation that the hub tells it.
type (
	// UserChunk is text the agent says the user wrote: relay replays
	// each prompt that way, and sends the prompts of other devices as
	// they happen. A plain agent has no reason to send one, but one that
	// does is shown the same.
	UserChunk struct {
		At              time.Time
		MessageID, Text string
	}
	// ConvAttached means the attach to the conversation has been
	// answered, so it can take prompts.
	ConvAttached struct{ At time.Time }
	// RemoteIdle says relay reports no turn running in the conversation,
	// so whatever is still open in it will not change.
	RemoteIdle struct{ At time.Time }
	// ReplayEnd says relay has sent all of the replay (_relay/replayed).
	// Calls the replay left open will not finish, so they stop.
	ReplayEnd struct{ At time.Time }
)

func (e UserChunk) at() time.Time    { return e.At }
func (e ConvAttached) at() time.Time { return e.At }
func (e RemoteIdle) at() time.Time   { return e.At }
func (e ReplayEnd) at() time.Time    { return e.At }

func (r *reducer) shared(e Event) {
	s := &r.s
	switch e := e.(type) {
	case UserChunk:
		if s.Phase == Exited {
			return
		}
		if n := len(s.Entries); n > 0 && e.MessageID != "" && s.Entries[n-1].Kind == User && s.Entries[n-1].ID == e.MessageID {
			s.Entries[n-1].Text += e.Text
			return
		}
		// The text came down the agent's channel, so its URLs show
		// defanged once the session is contaminated, as the agent's own
		// words do.
		r.add(Entry{Kind: User, ID: e.MessageID, Text: e.Text, Final: true})
		s.Entries[len(s.Entries)-1].Defang = s.Labels.Contaminated
	case ConvAttached:
		if s.Phase == Starting {
			s.Phase = Idle
			r.refresh()
			r.sendQueue(e.At)
		}
	case RemoteIdle:
		if s.Phase != Busy && s.Phase != Exited {
			r.closeAll()
		}
	case ReplayEnd:
		// An open reply or thought stays open: the turn may still be
		// running and its next chunk joins it. The roster going idle
		// closes those.
		if s.Phase != Exited {
			for i := range s.Entries {
				en := &s.Entries[i]
				if en.Kind != Tool || en.Final {
					continue
				}
				// Finality is left to the next entry: a run with every call
				// done still takes the next call.
				for j := range en.Calls {
					if !done(en.Calls[j].Status) {
						en.Calls[j].Status = Stopped
					}
				}
			}
		}
	}
}
