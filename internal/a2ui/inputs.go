package a2ui

// Input is a CheckBox the operator can change.
type Input struct {
	ID    string // the component
	Path  string // the absolute path its value is bound to
	Label string // its label, resolved
	On    bool   // its value in the data model
}

// boundPath is the path a CheckBox's value is bound to, when it is bound
// to one.
func boundPath(value any) (string, bool) {
	m, ok := value.(map[string]any)
	if !ok || len(m) != 1 {
		return "", false
	}
	p, ok := m["@path"].(string)
	return p, ok
}

// Inputs lists, in drawing order, the CheckBoxes the root reaches whose
// value is bound to a path.
func (s Surface) Inputs() []Input {
	var out []Input
	s.Walk(func(n Node) {
		if s.Kind(n.Component) != "CheckBox" {
			return
		}
		path, ok := boundPath(n.Component.Props["value"])
		if !ok {
			return
		}
		abs, ok := n.Scope.Abs(path)
		if !ok {
			return
		}
		on, _ := s.Bool(n.Scope, n.Component.Props["value"])
		out = append(out, Input{
			ID:    n.Component.ID,
			Path:  abs,
			Label: s.String(n.Scope, n.Component.Props["label"]),
			On:    on,
		})
	})
	return out
}
