package ocpp

import (
	"encoding/json"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cB-Sigma/jsonschema"
)

// Timestamps without a time zone (finding F-1412). OCPP 1.6 edition 2
// section 4.8 asks for ISO 8601 timestamps but does not say they carry a
// zone, while the OCA JSON schemas of every version need an RFC 3339
// date-time, which does. Many 1.6 chargers send local or UTC times with
// no zone. Rezone reads such a timestamp in a message from a charger (a
// CALL or a CALLRESULT) in the charger's time zone and rewrites it as an
// RFC 3339 UTC date-time, so the message can be validated. Only
// properties the schema (or, for the 1.6 security extension messages that
// have no schema, our Go type) declares a date-time are rewritten;
// free-form data is left alone.

// zonelessRE matches a date and time of day with no zone: seconds and a
// fraction are optional, the separator may be T, t or a space.
var zonelessRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[Tt ]\d{2}:\d{2}(:\d{2}(\.\d{1,9})?)?$`)

// parseZoneless reads s, a zone-less date-time, in loc.
func parseZoneless(s string, loc *time.Location) (time.Time, bool) {
	if !zonelessRE.MatchString(s) {
		return time.Time{}, false
	}
	v := strings.ToUpper(s[:10]) + "T" + s[11:]
	for _, layout := range []string{"2006-01-02T15:04:05.999999999", "2006-01-02T15:04"} {
		if t, err := time.ParseInLocation(layout, v, loc); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// Rezone rewrites the zone-less date-times of a message from a charger
// (a request when response is false, else the answer to a CALL), reading
// them in loc, as RFC 3339 UTC date-times. Only properties the OCA schema
// (or, for the 1.6 security extension messages that have no schema, our Go
// type) declares a date-time are rewritten. It returns the payload and the
// values it rewrote; the payload is returned unchanged when it holds none
// or is not JSON.
func Rezone(version, action string, response bool, raw json.RawMessage, loc *time.Location) (json.RawMessage, []string) {
	if len(raw) == 0 {
		return raw, nil
	}
	v, err := jsonschema.Decode(raw)
	if err != nil {
		return raw, nil
	}
	var seen []string
	fix := func(s string) (string, bool) {
		t, ok := parseZoneless(s, loc)
		if !ok {
			return "", false
		}
		seen = append(seen, s)
		return t.UTC().Format(time.RFC3339Nano), true
	}
	if !zoneBySchema(version, action, response, v, fix) {
		if version != V16 {
			return raw, nil
		}
		t := goType16(action, response)
		if t == nil {
			return raw, nil
		}
		v = fixByType(t, v, fix)
	}
	if len(seen) == 0 {
		return raw, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return raw, nil
	}
	return b, seen
}

// zoneBySchema rewrites, in v, each value the OCA schema of action
// refuses for its date-time format and fix accepts. It reports false when
// version has no schema for the message.
func zoneBySchema(version, action string, response bool, v any, fix func(string) (string, bool)) bool {
	ss := schemaSets[version]
	if ss == nil {
		return false
	}
	name := schemaName(version, action, response)
	if !ss.has(name) {
		return false
	}
	set, err := ss.compiled()
	if err != nil {
		return true
	}
	verr := set.Validate(name, v)
	var ve jsonschema.ValidationError
	if verr == nil || !errorsAs(verr, &ve) {
		return true
	}
	for _, e := range ve {
		if e.Keyword != "format" {
			continue
		}
		setAt(v, e.Path, fix)
	}
	return true
}

// setAt replaces the string at JSON pointer ptr in v with fix's result.
func setAt(v any, ptr string, fix func(string) (string, bool)) {
	toks := strings.Split(strings.TrimPrefix(ptr, "/"), "/")
	if ptr == "" || len(toks) == 0 {
		return
	}
	cur := v
	for i, tok := range toks {
		tok = strings.ReplaceAll(strings.ReplaceAll(tok, "~1", "/"), "~0", "~")
		last := i == len(toks)-1
		switch c := cur.(type) {
		case map[string]any:
			if last {
				if s, ok := c[tok].(string); ok {
					if n, ok := fix(s); ok {
						c[tok] = n
					}
				}
				return
			}
			cur = c[tok]
		case []any:
			n, err := strconv.Atoi(tok)
			if err != nil || n < 0 || n >= len(c) {
				return
			}
			if last {
				if s, ok := c[n].(string); ok {
					if f, ok := fix(s); ok {
						c[n] = f
					}
				}
				return
			}
			cur = c[n]
		default:
			return
		}
	}
}

// goType16 is our Go type of a 1.6 message, or nil.
func goType16(action string, response bool) reflect.Type {
	if response {
		if md, ok := v16Out[action]; ok {
			return md.Conf
		}
		return nil
	}
	if md, ok := v16In[action]; ok {
		return md.Req
	}
	return nil
}

// fixByType rewrites, in v, the zone-less values of fields t tags dt.
func fixByType(t reflect.Type, v any, fix func(string) (string, bool)) any {
	return fixField(t, tag{}, v, fix)
}

func fixField(t reflect.Type, tg tag, v any, fix func(string) (string, bool)) any {
	if t == nil {
		return v
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Struct:
		m, ok := v.(map[string]any)
		if !ok {
			return v
		}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			jn, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if fv, ok := m[jn]; ok && jn != "" && jn != "-" {
				m[jn] = fixField(f.Type, parseTag(f.Tag.Get("ocpp")), fv, fix)
			}
		}
	case reflect.Slice:
		a, ok := v.([]any)
		if !ok || t == rawType {
			return v
		}
		et := parseTag(strings.Join(nonItemTags(tg), ","))
		for i := range a {
			a[i] = fixField(t.Elem(), et, a[i], fix)
		}
	case reflect.String:
		if s, ok := v.(string); ok && tg.dt {
			if n, ok := fix(s); ok {
				return n
			}
		}
	}
	return v
}
