package a2ui

// Input is a CheckBox the operator can change, or a Button they can
// press.
type Input struct {
	ID    string // the component, as relay sent it
	Path  string // a CheckBox: the absolute path its value is bound to
	Label string // its label, resolved; a Button's is its child Text's
	On    bool   // a CheckBox: its value in the data model
	// Button says the input is a Button. Event is the name of the event
	// it tells the role, or empty when it sets a box (relay:setInput).
	Button bool
	Event  string
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
// value is bound to a path, and the Buttons.
func (s Surface) Inputs() []Input {
	var out []Input
	s.Walk(func(n Node) {
		if s.Kind(n.Component) == "Button" {
			in := Input{ID: n.Component.ID, Button: true}
			if kids := s.Children(n.Component, n.Scope); len(kids) == 1 {
				if t, ok := s.Components[kids[0].ID]; ok {
					in.Label = s.String(kids[0].Scope, t.Props["text"])
				}
			}
			act, _ := n.Component.Props["action"].(map[string]any)
			if ev, ok := act["event"].(map[string]any); ok {
				in.Event, _ = ev["name"].(string)
			}
			out = append(out, in)
			return
		}
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
