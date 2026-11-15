package a2ui

// Surface is one UI the agent describes: its components by id (the entry
// point is "root") and the data model they bind to.
type Surface struct {
	ID            string
	CatalogID     string
	SendDataModel bool
	Components    map[string]Component
	Data          any
}

// Surfaces is every surface that exists, in the order they were created.
type Surfaces []Surface

// Get finds a surface by id.
func (ss Surfaces) Get(id string) (Surface, bool) {
	for _, s := range ss {
		if s.ID == id {
			return s, true
		}
	}
	return Surface{}, false
}

// With is ss with the surface of the same id replaced by s, or ss
// unchanged when there is none.
func (ss Surfaces) With(s Surface) Surfaces {
	out := append(Surfaces(nil), ss...)
	for i := range out {
		if out[i].ID == s.ID {
			out[i] = s
		}
	}
	return out
}

// Root is the component with id "root".
func (s Surface) Root() (Component, bool) {
	c, ok := s.Components["root"]
	return c, ok
}

// Apply folds one message into the surfaces. It returns new surfaces and
// leaves ss and the message as they were; on an error it returns ss
// itself. A surface may not be created twice while it exists, and the
// other messages need the surface to exist.
func Apply(ss Surfaces, m Message) (Surfaces, error) {
	n := 0
	for _, set := range []bool{m.Create != nil, m.Components != nil, m.Data != nil, m.Delete != nil} {
		if set {
			n++
		}
	}
	if n != 1 {
		return ss, errorf(CodeInvalid, "a message has exactly one kind, this one has %d", n)
	}
	switch {
	case m.Create != nil:
		c := m.Create
		if _, ok := ss.Get(c.SurfaceID); ok {
			return ss, errorf(CodeIntegrity, "surface %s exists; delete it before creating it again", escapeKind(c.SurfaceID))
		}
		if len(ss) >= MaxSurfaces {
			return ss, errorf(CodeIntegrity, "more than %d surfaces", MaxSurfaces)
		}
		if len(c.Components) > MaxComponents {
			return ss, errorf(CodeIntegrity, "more than %d components", MaxComponents)
		}
		s := Surface{ID: c.SurfaceID, CatalogID: c.CatalogID, SendDataModel: c.SendDataModel,
			Components: map[string]Component{}, Data: map[string]any{}}
		if c.DataModel != nil {
			s.Data = clone(c.DataModel)
		}
		for _, k := range c.Components {
			s.Components[k.ID] = cloneComponent(k)
		}
		return append(append(Surfaces(nil), ss...), s), nil
	case m.Components != nil:
		u := m.Components
		s, ok := ss.Get(u.SurfaceID)
		if !ok {
			return ss, errorf(CodeNoSurface, "no surface %s", escapeKind(u.SurfaceID))
		}
		comps := make(map[string]Component, len(s.Components)+len(u.Components))
		for id, k := range s.Components {
			comps[id] = k
		}
		for _, k := range u.Components {
			comps[k.ID] = cloneComponent(k)
		}
		if len(comps) > MaxComponents {
			return ss, errorf(CodeIntegrity, "more than %d components", MaxComponents)
		}
		s.Components = comps
		return ss.With(s), nil
	case m.Data != nil:
		u := m.Data
		s, ok := ss.Get(u.SurfaceID)
		if !ok {
			return ss, errorf(CodeNoSurface, "no surface %s", escapeKind(u.SurfaceID))
		}
		data, err := Set(s.Data, u.Path, u.Value)
		if err != nil {
			return ss, err
		}
		s.Data = data
		return ss.With(s), nil
	default:
		id := m.Delete.SurfaceID
		if _, ok := ss.Get(id); !ok {
			return ss, errorf(CodeNoSurface, "no surface %s", escapeKind(id))
		}
		out := make(Surfaces, 0, len(ss))
		for _, s := range ss {
			if s.ID != id {
				out = append(out, s)
			}
		}
		return out, nil
	}
}

func cloneComponent(c Component) Component {
	props := make(map[string]any, len(c.Props))
	for k, v := range c.Props {
		props[k] = clone(v)
	}
	return Component{ID: c.ID, Type: c.Type, Props: props}
}
