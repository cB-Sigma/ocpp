package ocpp

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"

	"github.com/cB-Sigma/jsonschema"
)

// Chargers in the field often deviate from the OCA schemas in ways that
// do not stop a message from being understood: a vendor property the
// schema does not define, a text longer (or shorter) than its limit, or a
// value outside an enumeration (newer or vendor firmware). Deviations
// sorts a message's failures so that an application can take such a
// message anyway (and warn) rather than answer it with a CALLERROR;
// DecodeLoose then decodes it into the Go type. A missing required
// property or a value of the wrong JSON type cannot be acted on and is
// never tolerable.

// tolerableKeywords are the schema keywords whose failure is tolerated.
var tolerableKeywords = map[string]bool{"additionalProperties": true, "maxLength": true, "minLength": true, "enum": true}

// deviations sorts a message's validation failures: ok when every one is
// tolerable, enum when one is a value outside an enumeration, other when
// one is of another tolerable kind. When the schema passes (or there is
// none, as for the 1.6 security extension), verr came from our Go type:
// an undefined property or an enumeration value is tolerated.
func deviations(version, action string, response bool, raw json.RawMessage, verr *RPCError) (ok, enum, other bool) {
	goType := func() (bool, bool, bool) {
		switch {
		case verr.Enum:
			return true, true, false
		case verr.Kind == KindFormat:
			return true, false, true
		}
		return false, false, false
	}
	ss := schemaSets[version]
	if ss == nil {
		return goType()
	}
	name := schemaName(version, action, response)
	if !ss.has(name) {
		return goType()
	}
	set, err := ss.compiled()
	if err != nil {
		return false, false, false
	}
	v, err := jsonschema.Decode(raw)
	if err != nil {
		return false, false, false
	}
	serr := set.Validate(name, v)
	if serr == nil {
		return goType()
	}
	var ve jsonschema.ValidationError
	if !errorsAs(serr, &ve) {
		return false, false, false
	}
	// A property sent as null where the schema wants an object, array,
	// string or number (ocpp-go's 2.0.1 Reset and SetNetworkProfile
	// answers carry "statusInfo": null) is read as absent: the message is
	// checked again without it, so a required property sent as null still
	// fails.
	if nullTyped(v, ve) {
		other = true
		serr = set.Validate(name, dropNulls(v))
		if serr == nil {
			return true, false, true
		}
		ve = nil
		if !errorsAs(serr, &ve) {
			return false, false, false
		}
	}
	for _, e := range ve {
		if !tolerableKeywords[e.Keyword] {
			return false, false, false
		}
		if e.Keyword == "enum" {
			enum = true
		} else {
			other = true
		}
	}
	return true, enum, other
}

// nullTyped reports whether one of the failures is a type failure of a
// property whose value is null.
func nullTyped(v any, ve jsonschema.ValidationError) bool {
	for _, e := range ve {
		if e.Keyword == "type" && e.Path != "" && valueAt(v, e.Path) == nil && present(v, e.Path) {
			return true
		}
	}
	return false
}

// present reports whether the JSON pointer ptr names a member or element
// that exists in v (its value may be null).
func present(v any, ptr string) bool {
	parent, last := "", ptr
	if i := strings.LastIndex(ptr, "/"); i >= 0 {
		parent, last = ptr[:i], ptr[i+1:]
	}
	last = strings.ReplaceAll(strings.ReplaceAll(last, "~1", "/"), "~0", "~")
	switch c := valueAt(v, parent).(type) {
	case map[string]any:
		_, ok := c[last]
		return ok
	case []any:
		n, err := strconv.Atoi(last)
		return err == nil && n >= 0 && n < len(c)
	}
	return false
}

// dropNulls returns v without the object members whose value is null.
func dropNulls(v any) any {
	switch c := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(c))
		for k, x := range c {
			if x != nil {
				out[k] = dropNulls(x)
			}
		}
		return out
	case []any:
		out := make([]any, len(c))
		for i, x := range c {
			out[i] = dropNulls(x)
		}
		return out
	}
	return v
}

// decodeLoose decodes a payload into a new t without the schema facts.
func decodeLoose(t reflect.Type, payload json.RawMessage) (any, *RPCError) {
	p := reflect.New(t)
	if err := json.Unmarshal(payload, p.Interface()); err != nil {
		return nil, RPCErrorf(KindType, "%v", err)
	}
	return p.Interface(), nil
}

// echoesOnly reports whether every schema failure of our answer b is a
// value outside an enumeration that the charger's request carries.
func echoesOnly(version, action string, b, request json.RawMessage) bool {
	ss := schemaSets[version]
	if ss == nil {
		return false
	}
	set, err := ss.compiled()
	if err != nil {
		return false
	}
	v, err := jsonschema.Decode(b)
	if err != nil {
		return false
	}
	serr := set.Validate(schemaName(version, action, true), v)
	var ve jsonschema.ValidationError
	if serr == nil || !errorsAs(serr, &ve) {
		return false
	}
	for _, e := range ve {
		x, ok := valueAt(v, e.Path).(string)
		q, _ := json.Marshal(x)
		if e.Keyword != "enum" || !ok || !bytes.Contains(request, q) {
			return false
		}
	}
	return true
}

// valueAt is the value at JSON pointer ptr in v, or nil.
func valueAt(v any, ptr string) any {
	if ptr == "" {
		return v
	}
	for _, tok := range strings.Split(strings.TrimPrefix(ptr, "/"), "/") {
		tok = strings.ReplaceAll(strings.ReplaceAll(tok, "~1", "/"), "~0", "~")
		switch c := v.(type) {
		case map[string]any:
			v = c[tok]
		case []any:
			n, err := strconv.Atoi(tok)
			if err != nil || n < 0 || n >= len(c) {
				return nil
			}
			v = c[n]
		default:
			return nil
		}
	}
	return v
}

// Deviations sorts the validation failures of a charger's message (verr,
// from DecodeIn or ValidateJSON): ok when every one is tolerable, enum when
// one is a value outside an enumeration, other when one is a property the
// schema does not define or a text of the wrong length.
func Deviations(version, action string, response bool, raw json.RawMessage, verr *RPCError) (ok, enum, other bool) {
	return deviations(version, action, response, raw, verr)
}

// DecodeLoose decodes a payload into a new t without the schema facts.
func DecodeLoose(t reflect.Type, payload json.RawMessage) (any, *RPCError) {
	return decodeLoose(t, payload)
}

// EchoesOnly reports whether every schema failure of our answer b is a
// value outside an enumeration that the charger's request carries.
func EchoesOnly(version, action string, b, request json.RawMessage) bool {
	return echoesOnly(version, action, b, request)
}
