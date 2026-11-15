package a2ui

// Limits on one Tracker.
const (
	MaxTrackerLines = 200
	MaxTrackerCells = 5000
)

// TrackerLine is a row or a column of a Tracker.
type TrackerLine struct{ ID, Label string }

// TrackerCell is a Tracker cell that is not open, by row id and column id.
type TrackerCell struct{ Row, Column, State string }

// TrackerData is a Tracker with its bindings resolved.
type TrackerData struct {
	Rows, Columns []TrackerLine
	Cells         []TrackerCell
}

// Tracker reads the Tracker component c in scope sc. Entries that are not
// objects with the right members are skipped, and so are repeated ids.
func (s Surface) Tracker(c Component, sc Scope) TrackerData {
	lines := func(v any) []TrackerLine {
		list, _ := s.Value(sc, v).([]any)
		var out []TrackerLine
		seen := map[string]bool{}
		for _, e := range list {
			m, ok := e.(map[string]any)
			if !ok || len(out) >= MaxTrackerLines {
				continue
			}
			id := Text(m["id"])
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, TrackerLine{ID: id, Label: Text(m["label"])})
		}
		return out
	}
	d := TrackerData{Rows: lines(c.Props["rows"]), Columns: lines(c.Props["columns"])}
	cells, _ := s.Value(sc, c.Props["cells"]).([]any)
	for _, e := range cells {
		m, ok := e.(map[string]any)
		if !ok || len(d.Cells) >= MaxTrackerCells {
			continue
		}
		d.Cells = append(d.Cells, TrackerCell{Row: Text(m["row"]), Column: Text(m["column"]), State: Text(m["state"])})
	}
	return d
}
