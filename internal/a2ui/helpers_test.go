package a2ui

import "encoding/json/v2"

// snapshot is a deep, comparable copy of surfaces: their JSON form.
func snapshot(ss Surfaces) string {
	type view struct {
		ID, Catalog string
		Comps       map[string]Component
		Data        any
	}
	var vs []view
	for _, s := range ss {
		vs = append(vs, view{s.ID, s.CatalogID, s.Components, s.Data})
	}
	b, _ := json.Marshal(vs, json.Deterministic(true))
	return string(b)
}

func snapshotMessage(m Message) string {
	b, _ := json.Marshal(m, json.Deterministic(true))
	return string(b)
}
