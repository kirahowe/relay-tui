package a2ui

// Limits on walking a surface's tree. A component list can point back at
// itself or fan out, so neither the drawing nor Focusables follows a tree
// past these.
const (
	MaxDepth = 16
	MaxNodes = 1500
	MaxItems = 500 // template instances drawn from one bound array
)

// Child is a component to draw and the scope it draws in.
type Child struct {
	ID    string
	Scope Scope
}

// Children lists what a container draws inside it: Card and Button have
// one child, and Row, Column and List have a list of ids or a template
// that makes one instance per item of a bound array. Any other component
// has none. An id that names no component is still listed; whoever draws
// it decides what that looks like.
func (s Surface) Children(c Component, sc Scope) []Child {
	switch c.Type {
	case "Card", "Button":
		if id, ok := c.Props["child"].(string); ok {
			return []Child{{ID: id, Scope: sc}}
		}
	case "Row", "Column", "List":
		switch ch := c.Props["children"].(type) {
		case []any:
			var out []Child
			for _, e := range ch {
				if id, ok := e.(string); ok && len(out) < MaxItems {
					out = append(out, Child{ID: id, Scope: sc})
				}
			}
			return out
		case map[string]any:
			id, _ := ch["componentId"].(string)
			path, _ := ch["path"].(string)
			abs, ok := sc.Abs(path)
			if !ok || id == "" {
				return nil
			}
			items, _ := Get(s.Data, abs)
			list, _ := items.([]any)
			var out []Child
			for i := range list {
				if i >= MaxItems {
					break
				}
				out = append(out, Child{ID: id, Scope: Scope{Base: abs + "/" + itoa(i), Index: i, In: true}})
			}
			return out
		}
	}
	return nil
}

func itoa(i int) string { return Text(float64(i)) }

// Node is a component met while walking the tree.
type Node struct {
	Component Component
	Scope     Scope
	Depth     int
}

// Walk visits the tree from the root in drawing order, parents before
// children. It skips ids that name no component, and stops descending at
// MaxDepth and after MaxNodes visits.
func (s Surface) Walk(visit func(Node)) {
	budget := MaxNodes
	var walk func(ch Child, depth int)
	walk = func(ch Child, depth int) {
		c, ok := s.Components[ch.ID]
		if !ok || depth > MaxDepth || budget <= 0 {
			return
		}
		budget--
		visit(Node{Component: c, Scope: ch.Scope, Depth: depth})
		if s.Kind(c) == "" {
			return
		}
		for _, k := range s.Children(c, ch.Scope) {
			walk(k, depth+1)
		}
	}
	walk(Child{ID: "root"}, 0)
}

// Focus names one thing that can take the keys: a component, and the
// template instance it is in, since one component drawn from a template
// appears once per item.
type Focus struct {
	ID    string
	Scope Scope
}

// Focusables lists, in drawing order, what the keys can land on: every
// Button, CheckBox, TextField and ChoicePicker the root reaches. Activate
// does something for the first two only.
func (s Surface) Focusables() []Focus {
	var out []Focus
	s.Walk(func(n Node) {
		switch s.Kind(n.Component) {
		case "Button", "CheckBox", "TextField", "ChoicePicker":
			out = append(out, Focus{ID: n.Component.ID, Scope: n.Scope})
		}
	})
	return out
}
