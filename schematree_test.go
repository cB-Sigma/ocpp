package ocpp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// A read-only view of a schema's tree for tests that compare the Go types
// with the schemas (validation itself is libs/jsonschema's).

type jsNode struct {
	ref        string
	types      []string
	props      map[string]*jsNode
	required   []string
	closed     bool // additionalProperties: false
	enum       []string
	maxLen     int
	min, max   *float64
	items      *jsNode
	minItems   int
	maxItems   int
	dateTime   bool
	defs       map[string]*jsNode // the root's definitions
	root       *jsNode
	hasMaxLen  bool
	hasMaxItem bool
}

func compileSchema(raw []byte) (*jsNode, error) {
	var v map[string]any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := d.Decode(&v); err != nil {
		return nil, err
	}
	root := &jsNode{defs: map[string]*jsNode{}}
	root.root = root
	if defs, ok := v["definitions"].(map[string]any); ok {
		for k := range defs {
			root.defs[k] = &jsNode{root: root}
		}
		for k, s := range defs {
			m, ok := s.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("definition %s is not an object", k)
			}
			if err := root.defs[k].fill(m, root); err != nil {
				return nil, fmt.Errorf("definition %s: %w", k, err)
			}
		}
	}
	if err := root.fill(v, root); err != nil {
		return nil, err
	}
	return root, nil
}

func num(v any) (float64, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	f, err := n.Float64()
	return f, err == nil
}

func (n *jsNode) fill(m map[string]any, root *jsNode) error {
	n.root = root
	for k, v := range m {
		switch k {
		case "$ref":
			s, _ := v.(string)
			name, ok := strings.CutPrefix(s, "#/definitions/")
			if !ok || root.defs[name] == nil {
				return fmt.Errorf("$ref %q does not name a definition", s)
			}
			n.ref = name
		case "type":
			switch t := v.(type) {
			case string:
				n.types = []string{t}
			case []any:
				for _, x := range t {
					s, _ := x.(string)
					n.types = append(n.types, s)
				}
			}
			for _, t := range n.types {
				switch t {
				case "object", "array", "string", "integer", "number", "boolean", "null":
				default:
					return fmt.Errorf("type %q is not supported", t)
				}
			}
		case "properties":
			pm, ok := v.(map[string]any)
			if !ok {
				return fmt.Errorf("properties is not an object")
			}
			n.props = map[string]*jsNode{}
			for name, s := range pm {
				sm, ok := s.(map[string]any)
				if !ok {
					return fmt.Errorf("property %s is not a schema", name)
				}
				c := &jsNode{}
				if err := c.fill(sm, root); err != nil {
					return fmt.Errorf("%s: %w", name, err)
				}
				n.props[name] = c
			}
		case "required":
			a, _ := v.([]any)
			for _, x := range a {
				s, _ := x.(string)
				n.required = append(n.required, s)
			}
		case "additionalProperties":
			b, ok := v.(bool)
			if !ok {
				return fmt.Errorf("additionalProperties is not a boolean")
			}
			n.closed = !b
		case "enum":
			a, _ := v.([]any)
			for _, x := range a {
				s, ok := x.(string)
				if !ok {
					return fmt.Errorf("enum holds a value that is not a string")
				}
				n.enum = append(n.enum, s)
			}
		case "maxLength":
			f, ok := num(v)
			if !ok {
				return fmt.Errorf("maxLength is not a number")
			}
			n.maxLen, n.hasMaxLen = int(f), true
		case "minimum":
			f, ok := num(v)
			if !ok {
				return fmt.Errorf("minimum is not a number")
			}
			n.min = &f
		case "maximum":
			f, ok := num(v)
			if !ok {
				return fmt.Errorf("maximum is not a number")
			}
			n.max = &f
		case "minItems":
			f, _ := num(v)
			n.minItems = int(f)
		case "maxItems":
			f, _ := num(v)
			n.maxItems, n.hasMaxItem = int(f), true
		case "items":
			sm, ok := v.(map[string]any)
			if !ok {
				return fmt.Errorf("items is not a single schema")
			}
			c := &jsNode{}
			if err := c.fill(sm, root); err != nil {
				return fmt.Errorf("items: %w", err)
			}
			n.items = c
		case "format":
			if v != "date-time" && v != "uri" {
				return fmt.Errorf("format %v is not supported", v)
			}
			n.dateTime = true
		}
	}
	return nil
}

func (n *jsNode) resolve() *jsNode {
	for i := 0; n.ref != "" && i < 16; i++ {
		n = n.root.defs[n.ref]
	}
	return n
}
