// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package gen21

import (
	"fmt"
	"strings"
)

// Instance builds a payload valid against schema, for tests: with full
// false only the required properties, with full true every property (and
// one unnamed property in each object the schema leaves open).
func Instance(schema []byte, full bool) (any, error) {
	root, err := parse(schema)
	if err != nil {
		return nil, err
	}
	return instance(root, root, full, 0)
}

func instance(root, n *node, full bool, depth int) (any, error) {
	if depth > 40 {
		return nil, fmt.Errorf("the schema nests too deep")
	}
	if n.Ref != "" {
		def := root.Definitions[strings.TrimPrefix(n.Ref, "#/definitions/")]
		if def == nil {
			return nil, fmt.Errorf("unknown reference %s", n.Ref)
		}
		return instance(root, def, full, depth+1)
	}
	switch n.Type {
	case "string":
		if len(n.Enum) > 0 {
			return n.Enum[len(n.Enum)-1], nil
		}
		switch n.Format {
		case "date-time":
			return "2026-10-11T12:30:45Z", nil
		case "uri":
			return "https://example.test/a", nil
		}
		l := 3
		if n.MinLength != nil && *n.MinLength > l {
			l = *n.MinLength
		}
		if n.MaxLength != nil && *n.MaxLength < l {
			l = *n.MaxLength
		}
		return strings.Repeat("a", l), nil
	case "integer":
		v := 2.0
		if n.Minimum != nil && *n.Minimum > v {
			v = *n.Minimum
		}
		if n.Maximum != nil && *n.Maximum < v {
			v = *n.Maximum
		}
		return v, nil
	case "number":
		v := 2.5
		if n.Minimum != nil && *n.Minimum > v {
			v = *n.Minimum
		}
		if n.Maximum != nil && *n.Maximum < v {
			v = *n.Maximum
		}
		return v, nil
	case "boolean":
		return true, nil
	case "array":
		k := 1
		if n.MinItems != nil && *n.MinItems > k {
			k = *n.MinItems
		}
		out := make([]any, 0, k)
		for i := 0; i < k; i++ {
			if n.Items == nil {
				out = append(out, map[string]any{})
				continue
			}
			v, err := instance(root, n.Items, full, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	case "object", "":
		req := map[string]bool{}
		for _, r := range n.Required {
			req[r] = true
		}
		m := map[string]any{}
		for _, p := range n.props {
			if !full && !req[p.name] {
				continue
			}
			v, err := instance(root, p.n, full, depth+1)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", p.name, err)
			}
			m[p.name] = v
		}
		if full && (n.AdditionalProperties == nil || *n.AdditionalProperties) && len(n.props) > 0 {
			m["sigmaExtra"] = map[string]any{"x": 1.0}
		}
		return m, nil
	}
	return nil, fmt.Errorf("type %q", n.Type)
}
