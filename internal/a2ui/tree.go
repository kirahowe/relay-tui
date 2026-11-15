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

// Texts is every string the surface draws, in drawing order, as the
// drawing resolves it: a Text's text, a CheckBox's label, a TextField's
// label, its value unless it is obscured, and its placeholder when it has
// no value, and a ChoicePicker's label and its options' labels. A Button
// draws its child, which is walked as any component is. It is what a
// reader of the drawn surface sees, so what is looked for in it agrees
// with what is on screen, and an obscured value is never in it.
func (s Surface) Texts() []string {
	var out []string
	add := func(v string) {
		if v != "" {
			out = append(out, v)
		}
	}
	s.Walk(func(n Node) {
		c, sc := n.Component, n.Scope
		switch s.Kind(c) {
		case "Text":
			add(s.String(sc, c.Props["text"]))
		case "CheckBox":
			add(s.String(sc, c.Props["label"]))
		case "TextField":
			add(s.String(sc, c.Props["label"]))
			value := s.String(sc, c.Props["value"])
			if variant, _ := c.Props["variant"].(string); variant != "obscured" {
				add(value)
			}
			if value == "" {
				add(s.String(sc, c.Props["placeholder"]))
			}
		case "ChoicePicker":
			add(s.String(sc, c.Props["label"]))
			opts, _ := c.Props["options"].([]any)
			for i, o := range opts {
				if i >= MaxItems {
					break
				}
				m, _ := o.(map[string]any)
				add(s.String(sc, m["label"]))
			}
		}
	})
	return out
}
