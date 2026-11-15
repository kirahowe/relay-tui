package a2ui

import (
	"maps"
	"slices"
	"strings"
)

// textVariants and the other enums are the values the relay catalog
// allows for the properties that take one.
var (
	textVariants   = []string{"body", "caption"}
	listDirections = []string{"vertical", "horizontal"}
	dividerAxes    = []string{"horizontal", "vertical"}
)

// drawnTypes are the component types this client draws. A type the
// catalog gains is refused until the client draws it.
var drawnTypes = map[string]bool{"Text": true, "Row": true, "Column": true, "List": true, "Card": true, "Divider": true, "CheckBox": true}

// Resolved reports why s is not a surface as relay sends one, or nil. A
// surface relay sends names the relay catalog, holds only components of
// that catalog with only the properties it lists, and has every string
// final: children are lists of ids, and the only binding left is the
// path of a CheckBox's value, which reads a boolean in the data model.
// The check covers what the root reaches, within the limits of Walk, and
// reaches each component once, so a surface that passes is one this
// package draws in full and in which a component id names one drawn
// component.
func Resolved(s Surface) error {
	if s.CatalogID != RelayCatalogID {
		return errorf(CodeInvalid, "catalog is %s, not %s", escapeKind(s.CatalogID), RelayCatalogID)
	}
	if _, ok := s.Root(); !ok {
		return errorf(CodeInvalid, "there is no root component")
	}
	nodes := 0
	reached := map[string]bool{}
	var visit func(id string, depth int) error
	visit = func(id string, depth int) error {
		if depth > MaxDepth {
			return errorf(CodeInvalid, "deeper than %d", MaxDepth)
		}
		if nodes++; nodes > MaxNodes {
			return errorf(CodeInvalid, "more than %d components reached", MaxNodes)
		}
		c, ok := s.Components[id]
		if !ok {
			return errorf(CodeInvalid, "component %s does not exist", escapeKind(id))
		}
		if reached[id] {
			return errorf(CodeInvalid, "component %s is reached more than once", escapeKind(id))
		}
		reached[id] = true
		kids, err := s.resolvedComponent(c)
		if err != nil {
			return err
		}
		for _, k := range kids {
			if err := visit(k, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit("root", 0); err != nil {
		return err
	}
	for _, id := range slices.Sorted(maps.Keys(s.Components)) {
		if !reached[id] {
			return errorf(CodeInvalid, "component %s is not reached from the root", escapeKind(id))
		}
	}
	return nil
}

// resolvedComponent checks one component and returns the ids it names.
func (s Surface) resolvedComponent(c Component) ([]string, error) {
	id := escapeKind(c.ID)
	allowed, ok := relayProps[c.Type]
	if !ok {
		return nil, errorf(CodeInvalid, "component %s has type %s, which the catalog lacks", id, escapeKind(c.Type))
	}
	if !drawnTypes[c.Type] {
		return nil, errorf(CodeInvalid, "component %s has type %s, which this client does not draw", id, escapeKind(c.Type))
	}
	for k := range c.Props {
		if !allowed[k] {
			return nil, errorf(CodeInvalid, "component %s has property %s, which %s lacks", id, escapeKind(k), c.Type)
		}
	}
	str := func(prop string) (string, bool) {
		v, ok := c.Props[prop].(string)
		return v, ok
	}
	oneOf := func(prop string, values []string) error {
		v, present := c.Props[prop]
		if !present {
			return nil
		}
		if got, ok := v.(string); ok {
			for _, want := range values {
				if got == want {
					return nil
				}
			}
		}
		return errorf(CodeInvalid, "component %s has %s %s, not one of %s", id, prop, escapeKind(Text(v)), strings.Join(values, ", "))
	}
	switch c.Type {
	case "Text":
		if _, ok := str("text"); !ok {
			return nil, errorf(CodeInvalid, "component %s has no text string", id)
		}
		return nil, oneOf("variant", textVariants)
	case "Divider":
		return nil, oneOf("axis", dividerAxes)
	case "Card":
		child, ok := str("child")
		if !ok {
			return nil, errorf(CodeInvalid, "component %s has no child id", id)
		}
		return []string{child}, nil
	case "Row", "Column", "List":
		if c.Type == "List" {
			if err := oneOf("direction", listDirections); err != nil {
				return nil, err
			}
		}
		list, ok := c.Props["children"].([]any)
		if !ok {
			return nil, errorf(CodeInvalid, "component %s has children that are not a list of ids", id)
		}
		if len(list) > MaxItems {
			return nil, errorf(CodeInvalid, "component %s has more than %d children", id, MaxItems)
		}
		out := make([]string, len(list))
		for i, e := range list {
			if out[i], ok = e.(string); !ok {
				return nil, errorf(CodeInvalid, "component %s has a child that is not an id", id)
			}
		}
		return out, nil
	case "CheckBox":
		if _, ok := str("label"); !ok {
			return nil, errorf(CodeInvalid, "component %s has no label string", id)
		}
		switch v := c.Props["value"].(type) {
		case bool:
		case map[string]any:
			path, ok := boundPath(v)
			if !ok || !strings.HasPrefix(path, "/") {
				return nil, errorf(CodeInvalid, "component %s has a value that is neither a boolean nor an absolute path", id)
			}
			if got, found := Get(s.Data, path); !found {
				return nil, errorf(CodeInvalid, "component %s reads %s, which the data model lacks", id, escapeKind(path))
			} else if _, isBool := got.(bool); !isBool {
				return nil, errorf(CodeInvalid, "component %s reads %s, which is not a boolean", id, escapeKind(path))
			}
		default:
			return nil, errorf(CodeInvalid, "component %s has a value that is neither a boolean nor an absolute path", id)
		}
		return nil, nil
	}
	return nil, errorf(CodeInvalid, "component %s has type %s, which this client does not check", id, escapeKind(c.Type))
}
