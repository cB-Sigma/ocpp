package ocpp

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Messages are checked against hand-written Go types rather than the OCA's
// JSON schemas (see docs/architecture.md): each struct field's `ocpp` tag
// carries the schema facts that matter for interoperability.
//
//	req        the property is required
//	max=N      a string's maximum length
//	enum=A|B   the allowed values of a string
//	dt         a string holding an RFC 3339 date-time
//	min=N      a number's minimum
//	minItems=N, maxItems=N  an array's size
//	int        a number that must be integral (implied by Go integer types)
//
// A property that is not a field is a format violation; 2.0.1 allows a
// "customData" object everywhere.

// validate checks the raw payload against t (a struct type) and decodes it
// into a new value of t.
func validate(t reflect.Type, raw json.RawMessage, version string) (any, *RPCError) {
	var v any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := d.Decode(&v); err != nil {
		return nil, RPCErrorf(KindFormat, "the payload is not JSON")
	}
	if err := checkValue(t, v, "", version); err != nil {
		return nil, err
	}
	p := reflect.New(t)
	if err := json.Unmarshal(raw, p.Interface()); err != nil {
		return nil, RPCErrorf(KindType, "%v", err)
	}
	return p.Interface(), nil
}

type tag struct {
	req      bool
	max      int
	enum     []string
	dt       bool
	min      *float64
	minItems int
	maxItems int
}

func parseTag(s string) tag {
	var t tag
	for _, p := range strings.Split(s, ",") {
		k, v, _ := strings.Cut(p, "=")
		switch k {
		case "req":
			t.req = true
		case "max":
			t.max, _ = strconv.Atoi(v)
		case "enum":
			t.enum = strings.Split(v, "|")
		case "dt":
			t.dt = true
		case "min":
			f, _ := strconv.ParseFloat(v, 64)
			t.min = &f
		case "minItems":
			t.minItems, _ = strconv.Atoi(v)
		case "maxItems":
			t.maxItems, _ = strconv.Atoi(v)
		}
	}
	return t
}

var rawType = reflect.TypeFor[json.RawMessage]()

func checkValue(t reflect.Type, v any, path string, version string) *RPCError {
	return checkField(t, tag{}, v, path, version)
}

func name(path string) string {
	if path == "" {
		return "the payload"
	}
	return path
}

func checkField(t reflect.Type, tg tag, v any, path, version string) *RPCError {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == rawType || t.Kind() == reflect.Interface {
		return nil // free-form (vendor data)
	}
	switch t.Kind() {
	case reflect.Struct:
		m, ok := v.(map[string]any)
		if !ok {
			return RPCErrorf(KindType, "%s is not an object", name(path))
		}
		known := map[string]bool{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			jn, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if jn == "" || jn == "-" {
				continue
			}
			known[jn] = true
			ft := parseTag(f.Tag.Get("ocpp"))
			fv, present := m[jn]
			p := jn
			if path != "" {
				p = path + "." + jn
			}
			if !present {
				if ft.req {
					return RPCErrorf(KindOccurrence, "%s is required", p)
				}
				continue
			}
			if err := checkField(f.Type, ft, fv, p, version); err != nil {
				return err
			}
		}
		for k := range m {
			if !known[k] && !(version == V201 && k == "customData") {
				return RPCErrorf(KindFormat, "%s has no property %s", name(path), k)
			}
		}
	case reflect.Slice:
		a, ok := v.([]any)
		if !ok {
			return RPCErrorf(KindType, "%s is not an array", name(path))
		}
		if len(a) < tg.minItems {
			return RPCErrorf(KindOccurrence, "%s needs at least %d items", path, tg.minItems)
		}
		if tg.maxItems > 0 && len(a) > tg.maxItems {
			return RPCErrorf(KindOccurrence, "%s holds more than %d items", path, tg.maxItems)
		}
		et := parseTag(strings.Join(nonItemTags(tg), ","))
		for i, x := range a {
			if err := checkField(t.Elem(), et, x, path+"["+strconv.Itoa(i)+"]", version); err != nil {
				return err
			}
		}
	case reflect.String:
		s, ok := v.(string)
		if !ok {
			return RPCErrorf(KindType, "%s is not a string", path)
		}
		if tg.max > 0 && len([]rune(s)) > tg.max {
			return RPCErrorf(KindProperty, "%s is longer than %d characters", path, tg.max)
		}
		if len(tg.enum) > 0 && !contains(tg.enum, s) {
			e := RPCErrorf(KindProperty, "%s: %q is not one of %s", path, s, strings.Join(tg.enum, ", "))
			e.Enum = true
			return e
		}
		if tg.dt {
			if _, err := time.Parse(time.RFC3339Nano, s); err != nil {
				return RPCErrorf(KindProperty, "%s: %q is not an RFC 3339 date-time", path, s)
			}
		}
	case reflect.Bool:
		if _, ok := v.(bool); !ok {
			return RPCErrorf(KindType, "%s is not a boolean", path)
		}
	case reflect.Int, reflect.Int32, reflect.Int64:
		n, ok := v.(json.Number)
		if !ok {
			return RPCErrorf(KindType, "%s is not a number", path)
		}
		i, err := strconv.ParseInt(n.String(), 10, 64)
		if err != nil {
			f, ferr := n.Float64()
			if ferr != nil || f != math.Trunc(f) {
				return RPCErrorf(KindType, "%s is not an integer", path)
			}
			i = int64(f)
		}
		if t.Kind() == reflect.Int32 && (i > math.MaxInt32 || i < math.MinInt32) {
			return RPCErrorf(KindType, "%s does not fit in 32 bits", path)
		}
		if tg.min != nil && float64(i) < *tg.min {
			return RPCErrorf(KindProperty, "%s is below %v", path, *tg.min)
		}
	case reflect.Float64:
		n, ok := v.(json.Number)
		if !ok {
			return RPCErrorf(KindType, "%s is not a number", path)
		}
		f, err := n.Float64()
		if err != nil {
			return RPCErrorf(KindType, "%s is not a number", path)
		}
		if tg.min != nil && f < *tg.min {
			return RPCErrorf(KindProperty, "%s is below %v", path, *tg.min)
		}
	}
	return nil
}

// nonItemTags keeps the tags that apply to an array's items (max, enum,
// dt, min) and drops the array's own.
func nonItemTags(t tag) []string {
	var out []string
	if t.max > 0 {
		out = append(out, "max="+strconv.Itoa(t.max))
	}
	if len(t.enum) > 0 {
		out = append(out, "enum="+strings.Join(t.enum, "|"))
	}
	if t.dt {
		out = append(out, "dt")
	}
	if t.min != nil {
		out = append(out, "min="+strconv.FormatFloat(*t.min, 'g', -1, 64))
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
