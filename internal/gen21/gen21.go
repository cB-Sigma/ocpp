// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

// Package gen21 writes the Go types of package v21 from the OCA's OCPP 2.1
// JSON schemas, which the ocpp module embeds unmodified. Only the
// structure is taken (property names, types, required properties,
// enumeration values); descriptions are not copied.
package gen21

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"sort"
	"strings"
	"unicode"
)

// node is the part of a JSON schema the generator reads.
type node struct {
	Ref                  string           `json:"$ref"`
	Type                 string           `json:"type"`
	Format               string           `json:"format"`
	Enum                 []string         `json:"enum"`
	Properties           json.RawMessage  `json:"properties"`
	Required             []string         `json:"required"`
	Items                *node            `json:"items"`
	AdditionalProperties *bool            `json:"additionalProperties"`
	Definitions          map[string]*node `json:"definitions"`
	MinLength            *int             `json:"minLength"`
	MaxLength            *int             `json:"maxLength"`
	Minimum              *float64         `json:"minimum"`
	Maximum              *float64         `json:"maximum"`
	MinItems             *int             `json:"minItems"`

	props []prop // Properties in the order written
}

type prop struct {
	name string
	n    *node
}

func parse(b []byte) (*node, error) {
	var n node
	if err := json.Unmarshal(b, &n); err != nil {
		return nil, err
	}
	if err := n.order(); err != nil {
		return nil, err
	}
	return &n, nil
}

// order reads Properties keeping the schema's order, recursively.
func (n *node) order() error {
	if n == nil {
		return nil
	}
	if len(n.Properties) > 0 {
		keys, err := orderedKeys(n.Properties)
		if err != nil {
			return err
		}
		var m map[string]*node
		if err := json.Unmarshal(n.Properties, &m); err != nil {
			return err
		}
		for _, k := range keys {
			n.props = append(n.props, prop{k, m[k]})
		}
	}
	for _, p := range n.props {
		if err := p.n.order(); err != nil {
			return err
		}
	}
	if err := n.Items.order(); err != nil {
		return err
	}
	for _, d := range n.Definitions {
		if err := d.order(); err != nil {
			return err
		}
	}
	return nil
}

func orderedKeys(raw json.RawMessage) ([]string, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	var keys []string
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, err
		}
		keys = append(keys, t.(string))
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return nil, err
		}
	}
	return keys, nil
}

// shape is a definition reduced to what the generated type depends on, to
// check that a name defined in many files means the same thing in all.
func shape(n *node) string {
	var b strings.Builder
	var walk func(n *node)
	walk = func(n *node) {
		if n == nil {
			b.WriteString("nil")
			return
		}
		fmt.Fprintf(&b, "{ref=%s type=%s fmt=%s enum=%v req=%v open=%v", n.Ref, n.Type, n.Format, n.Enum, n.Required, n.AdditionalProperties == nil || *n.AdditionalProperties)
		for _, p := range n.props {
			b.WriteString(" " + p.name + ":")
			walk(p.n)
		}
		b.WriteString(" items:")
		walk(n.Items)
		b.WriteString("}")
	}
	walk(n)
	return b.String()
}

// typeName is the Go name of a schema definition.
func (g *gen) typeName(def string) string {
	if s, ok := strings.CutSuffix(def, "EnumType"); ok {
		if g.objects[s] {
			return goName(s) + "Enum" // an object of the same name exists
		}
		return goName(s)
	}
	if s, ok := strings.CutSuffix(def, "Type"); ok && s != "" {
		return goName(s)
	}
	return goName(def)
}

// goName makes an exported Go identifier of a JSON name: the first letter
// upper case, separators dropped with the next letter upper case, and "Id"
// at the end of a word written ID.
func goName(s string) string {
	var out []rune
	up := true
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			up = true
			continue
		}
		if up {
			r = unicode.ToUpper(r)
			up = false
		}
		out = append(out, r)
	}
	n := string(out)
	n = fixID(n)
	if n == "" || unicode.IsDigit(rune(n[0])) {
		n = "V" + n
	}
	return n
}

func fixID(s string) string {
	r := []rune(s)
	var out []rune
	for i := 0; i < len(r); i++ {
		if r[i] == 'I' && i+1 < len(r) && r[i+1] == 'd' && (i+2 == len(r) || unicode.IsUpper(r[i+2]) || unicode.IsDigit(r[i+2])) {
			out = append(out, 'I', 'D')
			i++
			continue
		}
		out = append(out, r[i])
	}
	return string(out)
}

type genType struct {
	name   string
	shape  string
	from   string // file it was first read from
	enum   []string
	fields []field
	open   bool // more properties allowed: kept in Extra
	alias  string
}

type field struct {
	goName, json, goType string
	required             bool
}

type gen struct {
	objects  map[string]bool // "X" of every definition "XType" that is not an enumeration
	types    map[string]*genType
	messages []message
}

type message struct {
	action, file string
	response     bool
	send         bool
}

// Generate returns the source of package v21 from the 2.1 schema files (by
// file name without .json). send names the SEND actions, whose single
// schema has no Request suffix.
func Generate(files map[string][]byte, send map[string]bool) ([]byte, error) {
	g := &gen{types: map[string]*genType{}, objects: map[string]bool{}}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, file := range names {
		root, err := parse(files[file])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		for d := range root.Definitions {
			if s, ok := strings.CutSuffix(d, "Type"); ok && !strings.HasSuffix(d, "EnumType") {
				g.objects[s] = true
			}
		}
	}
	for _, file := range names {
		root, err := parse(files[file])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		defs := make([]string, 0, len(root.Definitions))
		for d := range root.Definitions {
			defs = append(defs, d)
		}
		sort.Strings(defs)
		for _, d := range defs {
			if err := g.define(g.typeName(d), root.Definitions[d], file); err != nil {
				return nil, err
			}
		}
		var m message
		switch {
		case send[file]:
			m = message{action: file, send: true}
		case strings.HasSuffix(file, "Request"):
			m = message{action: strings.TrimSuffix(file, "Request")}
		case strings.HasSuffix(file, "Response"):
			m = message{action: strings.TrimSuffix(file, "Response"), response: true}
		default:
			return nil, fmt.Errorf("%s: neither a request, a response nor a SEND", file)
		}
		m.file = file
		tn := m.action + "Request"
		if m.response {
			tn = m.action + "Response"
		}
		if err := g.define(tn, root, file); err != nil {
			return nil, err
		}
		g.messages = append(g.messages, m)
	}
	return g.source()
}

func (g *gen) define(name string, n *node, file string) error {
	sh := shape(n)
	if t, ok := g.types[name]; ok {
		if t.shape != sh {
			return fmt.Errorf("%s is defined differently in %s and %s", name, t.from, file)
		}
		return nil
	}
	t := &genType{name: name, shape: sh, from: file}
	g.types[name] = t
	switch {
	case n.Type == "string" && len(n.Enum) > 0:
		t.enum = n.Enum
	case n.Type == "object" || len(n.props) > 0:
		t.open = n.AdditionalProperties == nil || *n.AdditionalProperties
		req := map[string]bool{}
		for _, r := range n.Required {
			req[r] = true
		}
		for _, p := range n.props {
			gt, err := g.goType(name, p.name, p.n, file)
			if err != nil {
				return fmt.Errorf("%s.%s: %w", name, p.name, err)
			}
			if !req[p.name] && !strings.HasPrefix(gt, "[]") && gt != "json.RawMessage" {
				gt = "*" + gt
			}
			t.fields = append(t.fields, field{goName: goName(p.name), json: p.name, goType: gt, required: req[p.name]})
		}
	default:
		al, err := g.goType(name, "", n, file)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		t.alias = al
	}
	return nil
}

func (g *gen) goType(parent, prop string, n *node, file string) (string, error) {
	if n.Ref != "" {
		def, ok := strings.CutPrefix(n.Ref, "#/definitions/")
		if !ok {
			return "", fmt.Errorf("reference %s outside the file", n.Ref)
		}
		return g.typeName(def), nil
	}
	switch n.Type {
	case "string":
		if len(n.Enum) > 0 {
			name := parent + goName(prop)
			return name, g.define(name, n, file)
		}
		return "string", nil
	case "integer":
		return "int", nil
	case "number":
		return "float64", nil
	case "boolean":
		return "bool", nil
	case "array":
		if n.Items == nil {
			return "[]json.RawMessage", nil
		}
		it, err := g.goType(parent, prop+"Item", n.Items, file)
		if err != nil {
			return "", err
		}
		return "[]" + it, nil
	case "object":
		if len(n.props) == 0 {
			return "json.RawMessage", nil
		}
		name := parent + goName(prop)
		return name, g.define(name, n, file)
	case "":
		return "json.RawMessage", nil
	}
	return "", fmt.Errorf("type %q is not handled", n.Type)
}

func (g *gen) source() ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("// Code generated by internal/gen21 from the OCA's OCPP 2.1 JSON schemas; DO NOT EDIT.\n\n")
	b.WriteString("package v21\n\nimport (\n\t\"encoding/json\"\n\t\"reflect\"\n)\n\n")
	names := make([]string, 0, len(g.types))
	for n := range g.types {
		names = append(names, n)
	}
	sort.Strings(names)
	used := map[string]string{}
	for _, n := range names {
		t := g.types[n]
		switch {
		case t.enum != nil:
			fmt.Fprintf(&b, "// %s is an enumeration of the 2.1 schemas.\ntype %s string\n\n", n, n)
			b.WriteString("const (\n")
			for _, v := range t.enum {
				c := n + goName(v)
				if prev, ok := used[c]; ok {
					return nil, fmt.Errorf("constant %s stands for %s and %s", c, prev, v)
				}
				used[c] = v
				fmt.Fprintf(&b, "\t%s %s = %q\n", c, n, v)
			}
			b.WriteString(")\n\n")
		case t.alias != "":
			fmt.Fprintf(&b, "// %s is a type of the 2.1 schemas.\ntype %s = %s\n\n", n, n, t.alias)
		default:
			fmt.Fprintf(&b, "// %s is an object of the 2.1 schemas.\ntype %s struct {\n", n, n)
			for _, f := range t.fields {
				tag := f.json
				if !f.required {
					tag += ",omitempty"
				}
				fmt.Fprintf(&b, "\t%s %s `json:%q`\n", f.goName, f.goType, tag)
			}
			if t.open {
				b.WriteString("\t// Extra holds properties the schema does not name (it allows them).\n\tExtra map[string]json.RawMessage `json:\"-\"`\n")
			}
			b.WriteString("}\n\n")
			if t.open {
				keys := make([]string, len(t.fields))
				for i, f := range t.fields {
					keys[i] = fmt.Sprintf("%q", f.json)
				}
				fmt.Fprintf(&b, "// MarshalJSON writes the named properties and Extra.\nfunc (v %s) MarshalJSON() ([]byte, error) {\n\ttype plain %s\n\treturn marshalOpen(plain(v), v.Extra)\n}\n\n", n, n)
				fmt.Fprintf(&b, "// UnmarshalJSON reads the named properties and keeps the others in Extra.\nfunc (v *%s) UnmarshalJSON(b []byte) error {\n\ttype plain %s\n\tvar p plain\n\tif err := json.Unmarshal(b, &p); err != nil {\n\t\treturn err\n\t}\n\textra, err := unmarshalOpen(b, %s)\n\tif err != nil {\n\t\treturn err\n\t}\n\t*v = %s(p)\n\tv.Extra = extra\n\treturn nil\n}\n\n", n, n, strings.Join(keys, ", "), n)
			}
		}
	}
	sort.Slice(g.messages, func(i, j int) bool {
		if g.messages[i].action != g.messages[j].action {
			return g.messages[i].action < g.messages[j].action
		}
		return !g.messages[i].response
	})
	for _, m := range g.messages {
		tn := m.action + "Request"
		if m.response {
			tn = m.action + "Response"
		}
		fmt.Fprintf(&b, "// ActionName is %q.\nfunc (%s) ActionName() string { return %q }\n\n", m.action, tn, m.action)
		fmt.Fprintf(&b, "// IsResponse reports whether this is a response (%v).\nfunc (%s) IsResponse() bool { return %v }\n\n", m.response, tn, m.response)
	}
	b.WriteString("// Messages are the Go types of every 2.1 action.\nvar Messages = map[string]Message{\n")
	for _, m := range g.messages {
		if m.response {
			continue
		}
		if m.send {
			fmt.Fprintf(&b, "\t%q: {Request: reflect.TypeFor[%sRequest](), Send: true},\n", m.action, m.action)
			continue
		}
		fmt.Fprintf(&b, "\t%q: {Request: reflect.TypeFor[%sRequest](), Response: reflect.TypeFor[%sResponse]()},\n", m.action, m.action, m.action)
	}
	b.WriteString("}\n")
	src, err := format.Source(b.Bytes())
	if err != nil {
		return b.Bytes(), err
	}
	return src, nil
}
