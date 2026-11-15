package a2ui

import (
	_ "embed"
	"encoding/json/v2"
)

// The catalogs a surface can name. A catalog id is an arbitrary string in
// A2UI, not a URL; nothing here fetches it.
const (
	BasicCatalogID = "https://a2ui.org/specification/v1_0/catalogs/basic/catalog.json"
	RelayCatalogID = "urn:relay:catalog:v1"
)

//go:embed relay_catalog.json
var relayCatalogJSON []byte

// basicTypes are the components of A2UI's basic catalog that relay-tui
// knows by name. The basic catalog also has Icon, Tabs, Modal, Slider and
// DateTimeInput; a surface that uses them gets the same placeholder as a
// component no catalog has.
var basicTypes = []string{"Text", "Image", "Video", "AudioPlayer", "Row", "Column", "List", "Card", "Divider", "Button", "TextField", "CheckBox", "ChoicePicker"}

// catalogs maps a catalog id to the component types it defines.
var catalogs = func() map[string]map[string]bool {
	var rc struct {
		CatalogID  string                    `json:"catalogId"`
		Components map[string]map[string]any `json:"components"`
	}
	if err := json.Unmarshal(relayCatalogJSON, &rc); err != nil || rc.CatalogID != RelayCatalogID {
		panic("a2ui: relay_catalog.json is not the relay catalog")
	}
	relay := map[string]bool{}
	for name := range rc.Components {
		relay[name] = true
	}
	basic := map[string]bool{}
	for _, name := range basicTypes {
		basic[name] = true
	}
	return map[string]map[string]bool{BasicCatalogID: basic, RelayCatalogID: relay}
}()

// CatalogOf is the id of the catalog c resolves against: its own
// @catalogId, else the surface's. A2UI has no further fallback.
func (s Surface) CatalogOf(c Component) string {
	if id, ok := c.Props["@catalogId"].(string); ok && id != "" {
		return id
	}
	return s.CatalogID
}

// Kind is the type of c when the catalog it resolves against defines that
// type, and empty when it does not (an unknown catalog, no catalog, or a
// type the catalog lacks).
func (s Surface) Kind(c Component) string {
	if catalogs[s.CatalogOf(c)][c.Type] {
		return c.Type
	}
	return ""
}
