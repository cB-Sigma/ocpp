package ocpp

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// specDir is SigmaM2M's copy of the OCA files, when the module is
// checked out inside that repository.
const specDir = "../../spec/data/ocpp"

// Covers: SIG-IPE-2105
// The OCA files are redistributed unmodified (CC BY-ND 4.0): every
// embedded file listed in SHA256SUMS has its published digest and nothing
// else is there; inside SigmaM2M the embedded copy equals spec/data/ocpp.
func TestOCASchemaFilesUnmodified(t *testing.T) {
	oca := OCAFiles()
	for _, v := range []string{"1.6", "2.0.1", "2.1"} {
		sums, err := fs.ReadFile(oca, v+"/SHA256SUMS")
		if err != nil {
			t.Fatal(err)
		}
		listed := map[string]bool{}
		sc := bufio.NewScanner(bytes.NewReader(sums))
		for sc.Scan() {
			sum, name, ok := strings.Cut(sc.Text(), "  ")
			if !ok {
				t.Fatalf("%s: bad SHA256SUMS line %q", v, sc.Text())
			}
			b, err := fs.ReadFile(oca, v+"/"+name)
			if err != nil {
				t.Fatal(err)
			}
			got := sha256.Sum256(b)
			if hex.EncodeToString(got[:]) != sum {
				t.Errorf("%s/%s differs from the file the OCA published", v, name)
			}
			listed[name] = true
		}
		for _, sub := range []string{"schemas", "appendices", "wsdl"} {
			ents, _ := fs.ReadDir(oca, v+"/"+sub)
			for _, e := range ents {
				if !listed[sub+"/"+e.Name()] {
					t.Errorf("%s/%s/%s is not in SHA256SUMS", v, sub, e.Name())
				}
			}
		}
		if _, err := os.Stat(specDir); err != nil {
			continue // outside SigmaM2M
		}
		_ = fs.WalkDir(oca, v, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			b, _ := fs.ReadFile(oca, p)
			s, err := os.ReadFile(filepath.Join(specDir, filepath.FromSlash(p)))
			if err != nil || !bytes.Equal(b, s) {
				t.Errorf("%s differs from spec/data/ocpp (run make gen)", p)
			}
			return nil
		})
		_ = filepath.WalkDir(filepath.Join(specDir, v), func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(specDir, p)
			if _, err := fs.Stat(oca, filepath.ToSlash(rel)); err != nil {
				t.Errorf("%s is in spec/data/ocpp but not embedded (run make gen)", rel)
			}
			return nil
		})
	}
}

// strip is the test's own reading of what `make gen` keeps of a schema.
func strip(v any, names bool) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, e := range x {
			switch {
			case names:
				out[k] = strip(e, false)
			case annotations[k]:
			case k == "properties" || k == "definitions":
				out[k] = strip(e, true)
			case k == "enum" || k == "required" || k == "default":
				out[k] = e
			default:
				out[k] = strip(e, false)
			}
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = strip(e, false)
		}
		return out
	}
	return v
}

// Covers: SIG-IPE-2105
// Every embedded schema is read as published and compiles with the
// validator's keyword set; strip (what is left without annotations) shows
// that the annotations are the only text the validator skips.
func TestSchemasCompile(t *testing.T) {
	for v, ver := range map[string]string{"1.6": V16, "2.0.1": V201, "2.1": V21} {
		files, err := SchemaFiles(v)
		if err != nil {
			t.Fatal(err)
		}
		ents, _ := fs.ReadDir(OCAFiles(), v+"/schemas")
		if len(files) != len(ents) || len(files) == 0 {
			t.Errorf("%s: %d schema files, %d read", v, len(ents), len(files))
		}
		for name, b := range files {
			var src any
			if err := json.Unmarshal(b, &src); err != nil {
				t.Fatalf("%s %s: %v", v, name, err)
			}
			facts, _ := json.Marshal(strip(src, false))
			if bytes.Contains(facts, []byte(`"description":"`)) {
				t.Errorf("%s %s: a description is read as a validation fact", v, name)
			}
			if _, err := compileSchema(b); err != nil {
				t.Errorf("%s %s: %v", v, name, err)
			}
		}
		if _, err := schemaSets[ver].compiled(); err != nil {
			t.Fatal(err)
		}
	}
	if err := LoadSchemas(); err != nil {
		t.Fatal(err)
	}
}

// Covers: SIG-IPE-2106
// Every 2.1 action has a request and a response schema (the SEND message
// one), and the direction tables name each action exactly once.
func TestActions21MatchSchemas(t *testing.T) {
	seen := map[string]bool{}
	schemas21 := schemaSets[V21].raw()
	for name := range schemas21 {
		a := strings.TrimSuffix(strings.TrimSuffix(name, "Request"), "Response")
		seen[a] = true
	}
	for a := range seen {
		n := 0
		for _, m := range []map[string]bool{in21, out21, sendActions21} {
			if m[a] {
				n++
			}
		}
		if n == 0 {
			t.Errorf("%s has schemas but no direction", a)
		}
		if n > 1 && a != "DataTransfer" {
			t.Errorf("%s is in %d direction tables", a, n)
		}
		if !sendActions21[a] {
			if _, ok := schemas21[a+"Request"]; !ok {
				t.Errorf("%s has no request schema", a)
			}
			if _, ok := schemas21[a+"Response"]; !ok {
				t.Errorf("%s has no response schema", a)
			}
		}
	}
	for _, m := range []map[string]bool{in21, out21, sendActions21} {
		for a := range m {
			if !seen[a] {
				t.Errorf("%s is in a direction table but has no schema", a)
			}
		}
	}
	if len(seen) != 91 {
		t.Errorf("%d actions, want 90 request/response pairs and one SEND", len(seen))
	}
}

// goVsSchema lists where a hand-written Go type disagrees with a schema:
// a schema property the Go type lacks that our handlers would need, a Go
// field the schema does not define, or a JSON type that cannot decode.
func goVsSchema(t reflect.Type, n *jsNode, path string, out *[]string) {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	n = n.resolve()
	if t == rawType || t.Kind() == reflect.Interface {
		return
	}
	switch t.Kind() {
	case reflect.Struct:
		fields := map[string]reflect.StructField{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			jn, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if jn != "" && jn != "-" {
				fields[jn] = f
			}
		}
		for name, f := range fields {
			p, ok := n.props[name]
			if !ok {
				*out = append(*out, path+"."+name+": not in the schema")
				continue
			}
			goVsSchema(f.Type, p, path+"."+name, out)
			tg := parseTag(f.Tag.Get("ocpp"))
			if tg.req != contains(n.required, name) {
				*out = append(*out, path+"."+name+": required differs")
			}
		}
		for _, r := range n.required {
			if _, ok := fields[r]; !ok {
				*out = append(*out, path+"."+r+": required by the schema, missing in Go")
			}
		}
	case reflect.Slice:
		if n.items != nil {
			goVsSchema(t.Elem(), n.items, path+"[]", out)
		}
	case reflect.Int, reflect.Int32, reflect.Int64:
		if !contains(n.types, "integer") {
			*out = append(*out, path+": Go integer, schema "+strings.Join(n.types, "|"))
		}
	case reflect.Float64:
		if !contains(n.types, "number") && !contains(n.types, "integer") {
			*out = append(*out, path+": Go number, schema "+strings.Join(n.types, "|"))
		}
	case reflect.String:
		if !contains(n.types, "string") {
			*out = append(*out, path+": Go string, schema "+strings.Join(n.types, "|"))
		}
	}
}

// Covers: SIG-IPE-2107
// The Go types our 2.0.1 and 2.1 handlers decode into (the 2.0.1 ones,
// also used for the 2.1 messages both versions share) agree with the OCA
// schemas: each property our code reads exists with a JSON type that
// decodes into its Go field.
func TestGoTypesAgreeWithSchemas(t *testing.T) {
	var diffs []string
	raw16 := schemaSets[V16].raw()
	for _, m := range []map[string]msgDef{v16In, v16Out} {
		for action, md := range m {
			for _, side := range []struct {
				t    reflect.Type
				resp bool
			}{{md.Req, false}, {md.Conf, true}} {
				r, ok := raw16[schemaName(V16, action, side.resp)]
				if !ok {
					continue // the 1.6 security extension messages have no OCA schema
				}
				n, err := compileSchema(r)
				if err != nil {
					t.Fatal(err)
				}
				var d []string
				goVsSchema(side.t, n, "", &d)
				for _, x := range d {
					diffs = append(diffs, V16+" "+schemaName(V16, action, side.resp)+x)
				}
			}
		}
	}
	for _, ver := range []string{V201, V21} {
		raw := schemaSets[ver].raw()
		for _, dir := range []struct {
			m        map[string]msgDef
			incoming bool
		}{{v201In, true}, {v201Out, false}} {
			for action, md := range dir.m {
				for _, side := range []struct {
					t    reflect.Type
					resp bool
				}{{md.Req, false}, {md.Conf, true}} {
					var n *jsNode
					if r, ok := raw[schemaName(ver, action, side.resp)]; ok {
						n, _ = compileSchema(r)
					}
					if n == nil {
						diffs = append(diffs, ver+" "+action+": no schema")
						continue
					}
					var d []string
					goVsSchema(side.t, n, "", &d)
					for _, x := range d {
						diffs = append(diffs, ver+" "+schemaName(ver, action, side.resp)+x)
					}
				}
			}
		}
	}
	sort.Strings(diffs)
	var buf bytes.Buffer
	for _, d := range diffs {
		buf.WriteString(d + "\n")
	}
	if os.Getenv("OCPP_SCHEMA_DIFF") != "" {
		_ = os.WriteFile(os.Getenv("OCPP_SCHEMA_DIFF"), buf.Bytes(), 0o644)
	}
	for _, d := range diffs {
		if !knownTypeDiff(d) {
			t.Error(d)
		}
	}
}

// knownTypeDiff accepts the differences that do not affect decoding: a
// Go type that marks fewer properties required (the schema decides now),
// or a 2.1 property the shared 2.0.1 type does not carry.
func knownTypeDiff(d string) bool {
	return strings.HasSuffix(d, ": required differs") ||
		(strings.HasPrefix(d, "ocpp2.1 ") && strings.HasSuffix(d, ": required by the schema, missing in Go"))
}
