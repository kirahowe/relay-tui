package a2ui

import (
	"strconv"
	"strings"
)

// The data model is plain decoded JSON: map[string]any, []any, string,
// float64, bool and nil. Get and Set address it by JSON Pointer (RFC 6901).
// Neither changes its input: Set copies the nodes along the path and
// shares the rest.

// parsePointer splits a pointer into its unescaped parts. "" and "/" both
// mean the whole model.
func parsePointer(p string) ([]string, bool) {
	if p == "" || p == "/" {
		return nil, true
	}
	if p[0] != '/' {
		return nil, false
	}
	parts := strings.Split(p[1:], "/")
	if len(parts) > MaxPathParts {
		return nil, false
	}
	for i, s := range parts {
		if strings.Contains(s, "~") {
			var b strings.Builder
			for j := 0; j < len(s); j++ {
				if s[j] != '~' {
					b.WriteByte(s[j])
					continue
				}
				if j+1 >= len(s) || (s[j+1] != '0' && s[j+1] != '1') {
					return nil, false
				}
				if s[j+1] == '0' {
					b.WriteByte('~')
				} else {
					b.WriteByte('/')
				}
				j++
			}
			parts[i] = b.String()
		}
	}
	return parts, true
}

// EscapePart escapes one key for use in a pointer.
func EscapePart(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}

// index reads an array index: digits, no leading zero.
func index(s string) (int, bool) {
	if s == "" || len(s) > 9 || (len(s) > 1 && s[0] == '0') {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	return n, err == nil && n >= 0
}

// Get is the value at ptr, and false when there is none.
func Get(root any, ptr string) (any, bool) {
	parts, ok := parsePointer(ptr)
	if !ok {
		return nil, false
	}
	n := root
	for _, p := range parts {
		switch x := n.(type) {
		case map[string]any:
			v, ok := x[p]
			if !ok {
				return nil, false
			}
			n = v
		case []any:
			i, ok := index(p)
			if !ok || i >= len(x) {
				return nil, false
			}
			n = x[i]
		default:
			return nil, false
		}
	}
	return n, true
}

// Set returns root with the value at ptr replaced by v, creating objects
// for missing steps. A nil v removes the key instead, and does nothing
// when it is not there. From an array it takes the element out and the
// rest move down, as JSON Patch's remove does, since A2UI says only that
// null removes a key. Setting the whole model to nil leaves an empty
// object. It fails on a bad pointer, on a step through a value that is
// neither object nor array, and on an array index past the end ("-" or
// the length appends).
func Set(root any, ptr string, v any) (any, error) {
	parts, ok := parsePointer(ptr)
	if !ok {
		return root, errorf(CodeInvalid, "bad pointer %s", escapeKind(ptr))
	}
	if len(parts) == 0 {
		if v == nil {
			return map[string]any{}, nil
		}
		return clone(v), nil
	}
	if v == nil {
		return del(root, parts), nil
	}
	return set(root, parts, clone(v))
}

func set(n any, parts []string, v any) (any, error) {
	if len(parts) == 0 {
		return v, nil
	}
	switch x := n.(type) {
	case nil:
		child, err := set(nil, parts[1:], v)
		if err != nil {
			return n, err
		}
		return map[string]any{parts[0]: child}, nil
	case map[string]any:
		child, err := set(x[parts[0]], parts[1:], v)
		if err != nil {
			return n, err
		}
		out := make(map[string]any, len(x)+1)
		for k, e := range x {
			out[k] = e
		}
		out[parts[0]] = child
		return out, nil
	case []any:
		if parts[0] == "-" || len(parts) == 1 && parts[0] == strconv.Itoa(len(x)) {
			child, err := set(nil, parts[1:], v)
			if err != nil {
				return n, err
			}
			return append(append([]any(nil), x...), child), nil
		}
		i, ok := index(parts[0])
		if !ok || i >= len(x) {
			return n, errorf(CodeInvalid, "array index %s is out of range", escapeKind(parts[0]))
		}
		child, err := set(x[i], parts[1:], v)
		if err != nil {
			return n, err
		}
		out := append([]any(nil), x...)
		out[i] = child
		return out, nil
	}
	return n, errorf(CodeInvalid, "cannot step into a value that is not an object or array")
}

// del is n with the value at parts removed; n itself when it is not there.
func del(n any, parts []string) any {
	switch x := n.(type) {
	case map[string]any:
		child, ok := x[parts[0]]
		if !ok {
			return n
		}
		out := make(map[string]any, len(x))
		for k, e := range x {
			if k != parts[0] {
				out[k] = e
			}
		}
		if len(parts) > 1 {
			out[parts[0]] = del(child, parts[1:])
		}
		return out
	case []any:
		i, ok := index(parts[0])
		if !ok || i >= len(x) {
			return n
		}
		if len(parts) == 1 {
			return append(append([]any(nil), x[:i]...), x[i+1:]...)
		}
		out := append([]any(nil), x...)
		out[i] = del(x[i], parts[1:])
		return out
	}
	return n
}

// clone is a deep copy of decoded JSON.
func clone(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = clone(e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = clone(e)
		}
		return out
	}
	return v
}
