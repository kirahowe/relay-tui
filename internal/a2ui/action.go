package a2ui

import "errors"

// ErrNotActivatable says Activate was asked about something that has no
// action: a component that is gone, a TextField or ChoicePicker, or a
// CheckBox whose value is not bound to the data model.
var ErrNotActivatable = errors.New("a2ui: not activatable")

// Action is what a Button stands for, with every binding in it resolved.
// Nothing sends it: the caller decides what to do with it.
type Action struct {
	// Function is true for a functionCall action, which names a function
	// the renderer would run; false for an event for the agent.
	Function bool
	// Name is the event's name, or the function's.
	Name string
	// SurfaceID and SourceComponentID say where it came from.
	SurfaceID         string
	SourceComponentID string
	// UserMessage is an event's human-readable message, if it has one.
	UserMessage string
	// Context is an event's context, resolved.
	Context map[string]any
	// Args is a function's arguments, resolved.
	Args map[string]any
}

// Activate does what pressing f does. For a CheckBox it returns the
// surface with the toggled value written to its data model, and no
// action. For a Button it returns the surface as it was and the action
// the button carries. It does not change s.
func Activate(s Surface, f Focus) (Surface, *Action, error) {
	c, ok := s.Components[f.ID]
	if !ok {
		return s, nil, ErrNotActivatable
	}
	switch s.Kind(c) {
	case "CheckBox":
		bind, _ := c.Props["value"].(map[string]any)
		p, _ := bind["@path"].(string)
		abs, ok := f.Scope.Abs(p)
		if !ok || bind == nil || bind["@path"] == nil {
			return s, nil, ErrNotActivatable
		}
		cur, _ := s.Bool(f.Scope, c.Props["value"])
		data, err := Set(s.Data, abs, !cur)
		if err != nil {
			return s, nil, err
		}
		s.Data = data
		return s, nil, nil
	case "Button":
		act, _ := c.Props["action"].(map[string]any)
		a := &Action{SurfaceID: s.ID, SourceComponentID: c.ID}
		if ev, ok := act["event"].(map[string]any); ok {
			name, _ := ev["name"].(string)
			if name == "" {
				return s, nil, ErrNotActivatable
			}
			a.Name = name
			if _, has := ev["userMessage"]; has {
				a.UserMessage = s.String(f.Scope, ev["userMessage"])
			}
			a.Context = map[string]any{}
			if ctx, ok := ev["context"].(map[string]any); ok {
				for k, v := range ctx {
					a.Context[k] = s.Value(f.Scope, v)
				}
			}
			return s, a, nil
		}
		if fc, ok := act["functionCall"].(map[string]any); ok {
			name, _ := fc["@call"].(string)
			if name == "" {
				return s, nil, ErrNotActivatable
			}
			a.Function, a.Name = true, name
			a.Args = map[string]any{}
			if args, ok := fc["args"].(map[string]any); ok {
				for k, v := range args {
					a.Args[k] = s.Value(f.Scope, v)
				}
			}
			return s, a, nil
		}
	}
	return s, nil, ErrNotActivatable
}
