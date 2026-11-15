// Package a2ui reads A2UI v1.0 (https://a2ui.org) server-to-client
// messages and folds them into surfaces. It is pure: bytes and state in,
// state out, no IO and no clock. Drawing is in internal/render, which
// imports this package.
//
// The package covers the four messages that build and tear down a
// surface (createSurface, updateComponents, updateDataModel and
// deleteSurface). Any other message kind is a NotSupportedError. A surface
// holds a flat map of components and a JSON data model. Components bind to
// the data model with {"@path": ...} (absolute, or relative inside a List
// template), and a Button or CheckBox is activated by Activate, which
// returns the action it stands for and sends nothing.
package a2ui

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/kirahowe/relay-tui/internal/strictjson"
)

// Version is the one protocol version this package reads.
const Version = "1.0"

// Error is a message that cannot be read or applied, with the A2UI error
// code that names why.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Error codes. A2UI names INTEGRITY_ERROR; the other two are this
// package's own.
const (
	CodeInvalid   = "INVALID_MESSAGE"
	CodeIntegrity = "INTEGRITY_ERROR"
	CodeNoSurface = "UNKNOWN_SURFACE"
)

func errorf(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// NotSupportedError says a message is well formed but of a kind this
// renderer does not take: callRendererFunction, agentFunctionResponse or
// a key A2UI does not define.
type NotSupportedError struct{ Kind string }

func (e *NotSupportedError) Error() string {
	return "not supported: message kind " + escapeKind(e.Kind)
}

// escapeKind quotes a message key for an error text, since it came from
// the wire.
func escapeKind(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// Limits keep a hostile surface from growing without bound.
const (
	MaxSurfaces   = 64
	MaxComponents = 2000
	MaxPathParts  = 32
)

// Component is one entry of a surface's flat component list. Props holds
// every member of the JSON object but id and component, undecoded beyond
// JSON's own types.
type Component struct {
	ID    string
	Type  string
	Props map[string]any
}

// Message is one decoded server-to-client message; exactly one field is
// set.
type Message struct {
	Create     *CreateSurface
	Components *UpdateComponents
	Data       *UpdateDataModel
	Delete     *DeleteSurface
}

// CreateSurface is the createSurface message.
type CreateSurface struct {
	SurfaceID     string
	CatalogID     string
	SendDataModel bool
	Components    []Component
	DataModel     map[string]any
}

// UpdateComponents is the updateComponents message.
type UpdateComponents struct {
	SurfaceID  string
	Components []Component
}

// UpdateDataModel is the updateDataModel message. A nil Value deletes the
// key at Path.
type UpdateDataModel struct {
	SurfaceID string
	Path      string
	Value     any
}

// DeleteSurface is the deleteSurface message.
type DeleteSurface struct{ SurfaceID string }

// Decode reads one message: one JSON object in strictjson's dialect, with
// "version": "1.0" and exactly one message key. A key that is not one of
// the four this package takes gives a *NotSupportedError; anything else
// wrong gives an *Error. Unknown members inside a message body are
// refused, as the v1.0 schemas close them.
func Decode(line []byte) (Message, error) {
	if err := strictjson.Message(line); err != nil {
		return Message{}, errorf(CodeInvalid, "%v", err)
	}
	var top map[string]jsontext.Value
	if err := json.Unmarshal(line, &top); err != nil {
		return Message{}, errorf(CodeInvalid, "%v", err)
	}
	var version string
	if v, ok := top["version"]; !ok {
		return Message{}, errorf(CodeInvalid, "no version")
	} else if json.Unmarshal(v, &version) != nil || version != Version {
		return Message{}, errorf(CodeInvalid, "version must be the string %q", Version)
	}
	var keys []string
	for k := range top {
		if k != "version" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	if len(keys) != 1 {
		return Message{}, errorf(CodeInvalid, "want exactly one message key, got %d", len(keys))
	}
	body := top[keys[0]]
	strict := json.RejectUnknownMembers(true)
	switch keys[0] {
	case "createSurface":
		var r struct {
			SurfaceID     *string           `json:"surfaceId"`
			CatalogID     *string           `json:"catalogId"`
			SendDataModel *bool             `json:"sendDataModel"`
			Components    *[]map[string]any `json:"components"`
			DataModel     map[string]any    `json:"dataModel"`
			Metadata      *struct {
				Extensions map[string]any `json:"extensions"`
			} `json:"metadata"`
		}
		if err := json.Unmarshal(body, &r, strict); err != nil {
			return Message{}, errorf(CodeInvalid, "createSurface: %v", err)
		}
		if r.SurfaceID == nil || *r.SurfaceID == "" {
			return Message{}, errorf(CodeInvalid, "createSurface: surfaceId is required")
		}
		m := &CreateSurface{SurfaceID: *r.SurfaceID, DataModel: r.DataModel}
		if r.CatalogID != nil {
			m.CatalogID = *r.CatalogID
		}
		m.SendDataModel = r.SendDataModel != nil && *r.SendDataModel
		if r.Components != nil {
			if len(*r.Components) == 0 {
				return Message{}, errorf(CodeInvalid, "createSurface: components needs at least one entry")
			}
			cs, err := toComponents(*r.Components)
			if err != nil {
				return Message{}, err
			}
			m.Components = cs
		}
		return Message{Create: m}, nil
	case "updateComponents":
		var r struct {
			SurfaceID  *string          `json:"surfaceId"`
			Components []map[string]any `json:"components"`
		}
		if err := json.Unmarshal(body, &r, strict); err != nil {
			return Message{}, errorf(CodeInvalid, "updateComponents: %v", err)
		}
		if r.SurfaceID == nil || *r.SurfaceID == "" || len(r.Components) == 0 {
			return Message{}, errorf(CodeInvalid, "updateComponents: surfaceId and at least one component are required")
		}
		cs, err := toComponents(r.Components)
		if err != nil {
			return Message{}, err
		}
		return Message{Components: &UpdateComponents{SurfaceID: *r.SurfaceID, Components: cs}}, nil
	case "updateDataModel":
		var r struct {
			SurfaceID *string        `json:"surfaceId"`
			Path      *string        `json:"path"`
			Value     jsontext.Value `json:"value"`
		}
		if err := json.Unmarshal(body, &r, strict); err != nil {
			return Message{}, errorf(CodeInvalid, "updateDataModel: %v", err)
		}
		if r.SurfaceID == nil || *r.SurfaceID == "" {
			return Message{}, errorf(CodeInvalid, "updateDataModel: surfaceId is required")
		}
		if len(r.Value) == 0 {
			return Message{}, errorf(CodeInvalid, "updateDataModel: value is required (null deletes)")
		}
		m := &UpdateDataModel{SurfaceID: *r.SurfaceID, Path: "/"}
		if r.Path != nil {
			m.Path = *r.Path
		}
		if err := json.Unmarshal(r.Value, &m.Value); err != nil {
			return Message{}, errorf(CodeInvalid, "updateDataModel: %v", err)
		}
		return Message{Data: m}, nil
	case "deleteSurface":
		var r struct {
			SurfaceID *string `json:"surfaceId"`
		}
		if err := json.Unmarshal(body, &r, strict); err != nil {
			return Message{}, errorf(CodeInvalid, "deleteSurface: %v", err)
		}
		if r.SurfaceID == nil || *r.SurfaceID == "" {
			return Message{}, errorf(CodeInvalid, "deleteSurface: surfaceId is required")
		}
		return Message{Delete: &DeleteSurface{SurfaceID: *r.SurfaceID}}, nil
	}
	return Message{}, &NotSupportedError{Kind: keys[0]}
}

// toComponents checks the envelope of each component: a valid id, a
// component type that is not the reserved Surface, and no repeated id.
// Everything else about a component is left to the drawing, which meets
// it as the catalog's schema describes.
func toComponents(in []map[string]any) ([]Component, error) {
	out := make([]Component, 0, len(in))
	seen := map[string]bool{}
	for _, m := range in {
		id, _ := m["id"].(string)
		if !validID(id) {
			return nil, errorf(CodeInvalid, "component id %s is not a valid identifier", escapeKind(id))
		}
		typ, _ := m["component"].(string)
		if typ == "" {
			return nil, errorf(CodeInvalid, "component %s has no component type", escapeKind(id))
		}
		if typ == "Surface" {
			return nil, errorf(CodeInvalid, "component %s: Surface is reserved", escapeKind(id))
		}
		if seen[id] {
			return nil, errorf(CodeInvalid, "component id %s appears twice in one list", escapeKind(id))
		}
		seen[id] = true
		props := make(map[string]any, len(m))
		for k, v := range m {
			if k != "id" && k != "component" {
				props[k] = v
			}
		}
		out = append(out, Component{ID: id, Type: typ, Props: props})
	}
	return out, nil
}

// validID approximates UAX #31: a letter or underscore, then letters,
// marks, digits or underscores. The spec's XID classes are not in Go's
// standard library, so this is a little wider than they are.
func validID(id string) bool {
	if id == "" {
		return false
	}
	for i, r := range id {
		switch {
		case r == '_', unicode.IsLetter(r):
		case i > 0 && (unicode.IsDigit(r) || unicode.IsMark(r)):
		default:
			return false
		}
	}
	return true
}

// ParseLines decodes a JSONL document. It stops at the first line that
// does not decode, and returns what it has with the error naming the line.
// Blank lines are skipped.
func ParseLines(doc string) ([]Message, error) {
	var out []Message
	for i, l := range strings.Split(doc, "\n") {
		if strings.TrimSpace(l) == "" {
			continue
		}
		m, err := Decode([]byte(l))
		if err != nil {
			return out, fmt.Errorf("line %d: %w", i+1, err)
		}
		out = append(out, m)
	}
	return out, nil
}
