package a2ui

import (
	"encoding/json/v2"
	"math"
	"strconv"
	"strings"
)

// Scope is where a component sits in the tree as far as paths go. At the
// root it is the zero Scope, and only absolute paths ("/a/b") resolve.
// Inside a List template instance it is the pointer of that item and the
// item's index, and a path with no leading slash is read from the item.
type Scope struct {
	Base  string
	Index int
	In    bool
}

// Abs is path as an absolute pointer. A relative path outside any
// template has no meaning, and comes back false.
func (sc Scope) Abs(path string) (string, bool) {
	if strings.HasPrefix(path, "/") {
		return path, true
	}
	if !sc.In {
		return "", false
	}
	if path == "" {
		return sc.Base, true
	}
	return sc.Base + "/" + path, true
}

// maxResolveDepth bounds how deep a literal value is walked for bindings.
const maxResolveDepth = 16

// Value resolves a dynamic value to plain JSON. {"@path": p} reads the
// data model (nil when there is nothing there), a function call is
// evaluated (see call), and strings, numbers, booleans, arrays and objects
// are returned with any bindings inside them resolved.
func (s Surface) Value(sc Scope, v any) any { return s.resolve(sc, v, 0) }

func (s Surface) resolve(sc Scope, v any, depth int) any {
	if depth > maxResolveDepth {
		return nil
	}
	switch x := v.(type) {
	case map[string]any:
		if p, ok := x["@path"]; ok {
			path, _ := p.(string)
			abs, ok := sc.Abs(path)
			if !ok {
				return nil
			}
			got, _ := Get(s.Data, abs)
			return got
		}
		if _, ok := x["@call"]; ok {
			return s.call(sc, x, depth)
		}
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = s.resolve(sc, e, depth+1)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = s.resolve(sc, e, depth+1)
		}
		return out
	}
	return v
}

// call evaluates a function call. Two functions are known: @index, the
// item's position in the template being drawn, and formatString, which
// fills ${...} spots in a string with data model values. Any other
// function is the renderer's to run in the real protocol (formatDate,
// openUrl and the rest), and here it resolves to nil.
func (s Surface) call(sc Scope, c map[string]any, depth int) any {
	name, _ := c["@call"].(string)
	args, _ := c["args"].(map[string]any)
	switch name {
	case "@index":
		if !sc.In {
			return nil
		}
		off, _ := s.resolve(sc, args["offset"], depth+1).(float64)
		return float64(sc.Index) + math.Trunc(off)
	case "formatString":
		str, ok := s.resolve(sc, args["value"], depth+1).(string)
		if !ok {
			return nil
		}
		return s.interpolate(sc, str)
	}
	return nil
}

// interpolate fills the ${...} spots of str. \${ stays a literal ${. A
// spot holds a path, an @index(offset: n) call, or a nested ${...} around
// either; a call to any other function gives an empty string.
func (s Surface) interpolate(sc Scope, str string) string {
	var b strings.Builder
	for i := 0; i < len(str); {
		switch {
		case strings.HasPrefix(str[i:], `\${`):
			b.WriteString("${")
			i += 3
		case strings.HasPrefix(str[i:], "${"):
			end := closeBrace(str, i+1)
			if end < 0 {
				b.WriteString(str[i:])
				return b.String()
			}
			b.WriteString(s.expr(sc, str[i+2:end], 0))
			i = end + 1
		default:
			b.WriteByte(str[i])
			i++
		}
	}
	return b.String()
}

// closeBrace finds the } that closes the { at str[open], or -1.
func closeBrace(str string, open int) int {
	depth := 0
	for i := open; i < len(str); i++ {
		switch str[i] {
		case '{':
			depth++
		case '}':
			if depth--; depth == 0 {
				return i
			}
		}
	}
	return -1
}

func (s Surface) expr(sc Scope, e string, depth int) string {
	e = strings.TrimSpace(e)
	if depth > maxResolveDepth {
		return ""
	}
	switch {
	case strings.HasPrefix(e, "${") && strings.HasSuffix(e, "}"):
		return s.expr(sc, e[2:len(e)-1], depth+1)
	case strings.HasPrefix(e, "@index("):
		args := map[string]any{}
		if i := strings.Index(e, "offset:"); i >= 0 {
			rest := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(e[i+len("offset:"):]), ")"))
			if n, err := strconv.ParseFloat(rest, 64); err == nil {
				args["offset"] = n
			}
		}
		return Text(s.call(sc, map[string]any{"@call": "@index", "args": args}, depth))
	case strings.Contains(e, "("):
		return ""
	}
	abs, ok := sc.Abs(e)
	if !ok {
		return ""
	}
	v, _ := Get(s.Data, abs)
	return Text(v)
}

// Text is how a value reads as text: a string as it is, a number or
// boolean in its usual form, nil as nothing, and an object or array as JSON.
func Text(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case float64:
		if math.Abs(x) < 1e21 {
			return strconv.FormatFloat(x, 'f', -1, 64)
		}
		return strconv.FormatFloat(x, 'g', -1, 64)
	}
	b, err := json.Marshal(v, json.Deterministic(true))
	if err != nil {
		return ""
	}
	return string(b)
}

// String resolves v and reads it as text.
func (s Surface) String(sc Scope, v any) string { return Text(s.Value(sc, v)) }

// Bool resolves v to a boolean; the second result is false when it is not
// one.
func (s Surface) Bool(sc Scope, v any) (bool, bool) {
	b, ok := s.Value(sc, v).(bool)
	return b, ok
}

// Strings resolves v to a list of text; nothing when it is not a list.
func (s Surface) Strings(sc Scope, v any) []string {
	list, _ := s.Value(sc, v).([]any)
	out := make([]string, len(list))
	for i, e := range list {
		out[i] = Text(e)
	}
	return out
}
