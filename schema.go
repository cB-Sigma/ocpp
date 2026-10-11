package ocpp

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/cB-Sigma/jsonschema"
)

// Messages of every version are validated against the OCA's part 3 JSON
// schemas (1.6 draft-04, 2.x draft-06), read from the unmodified OCA files
// embedded in oca/ (see oca.go) when a version is first used, by
// libs/jsonschema (santhosh-tekuri/jsonschema/v6 underneath) with the
// jsonschema.OCA dialect: a schema that uses a keyword outside the dialect
// is refused when the set is loaded (CheckSchemas), so no schema is checked
// only halfway.
//
// Failures map to the OCPP-J 2.x RPC error codes (part 4 clause 4.3), by
// the keyword of the first failed check: a missing required property or an
// array of the wrong size is an OccurrenceConstraintViolation, a value of
// the wrong JSON type a TypeConstraintViolation, a property the schema
// does not define a FormatViolation, and a value outside an enumeration,
// length, bound or format a PropertyConstraintViolation.

// annotations are keywords without validation effect that the original
// files carry.
var annotations = map[string]bool{"description": true, "javaType": true, "comment": true, "$comment": true, "$id": true, "$schema": true,
	"id": true, "title": true}

func kindOf(keyword string) ErrKind {
	switch keyword {
	case "required", "minItems", "maxItems", "minProperties", "maxProperties":
		return KindOccurrence
	case "type":
		return KindType
	case "additionalProperties", "json":
		return KindFormat
	}
	return KindProperty
}

// schemaSets are the schemas of each version, by the folder under oca/.
var schemaSets = map[string]*schemaSet{V16: {dir: "1.6"}, V201: {dir: "2.0.1"}, V21: {dir: "2.1"}}

type schemaSet struct {
	dir string

	loadOnce sync.Once
	files    map[string][]byte // file name without .json -> the file as published
	loadErr  error

	once sync.Once
	set  *jsonschema.Set
	err  error
}

// raw is the version's schema files, read once from the embedded copy.
func (ss *schemaSet) raw() map[string][]byte {
	ss.loadOnce.Do(func() { ss.files, ss.loadErr = SchemaFiles(ss.dir) })
	return ss.files
}

// has reports whether the version has a schema file name (without .json).
func (ss *schemaSet) has(name string) bool {
	_, ok := ss.raw()[name]
	return ok
}

// compiled loads every schema of the set once.
func (ss *schemaSet) compiled() (*jsonschema.Set, error) {
	ss.once.Do(func() {
		files := ss.raw()
		if ss.loadErr != nil {
			ss.err = ss.loadErr
			return
		}
		// format is asserted whatever draft a schema names (the OCA's
		// schemas are draft-04 and draft-06, which assert it anyway).
		set := jsonschema.NewSetWith(jsonschema.Options{Dialect: jsonschema.OCA(), AssertFormat: true})
		for name, raw := range files {
			if err := set.Add(name, raw); err != nil {
				ss.err = fmt.Errorf("OCPP schema %s: %w", name, err)
				return
			}
		}
		for name := range files {
			if _, err := set.CheckSchemas(name, ""); err != nil {
				ss.err = fmt.Errorf("OCPP schema %s: %w", name, err)
				return
			}
			if _, err := set.Compile(name); err != nil {
				ss.err = fmt.Errorf("OCPP schema %s: %w", name, err)
				return
			}
		}
		ss.set = set
	})
	return ss.set, ss.err
}

// LoadSchemas reads and compiles the schemas of every version now rather
// than on first use, and reports a problem with them.
func LoadSchemas() error {
	for _, v := range []string{V16, V201, V21} {
		if _, err := schemaSets[v].compiled(); err != nil {
			return err
		}
	}
	return nil
}

// schemaName is the part 3 file name of an action's request or response
// (a 2.1 SEND message has no suffix).
func schemaName(version, action string, response bool) string {
	if version == V21 && sendActions21[action] {
		return action
	}
	if version == V16 { // 1.6 files are <Action>.json and <Action>Response.json
		if response {
			return action + "Response"
		}
		return action
	}
	if response {
		return action + "Response"
	}
	return action + "Request"
}

// hasSchema reports whether version (2.0.1 or 2.1) has a schema for the
// request of action.
func hasSchema(version, action string) bool {
	ss := schemaSets[version]
	if ss == nil {
		return false
	}
	return ss.has(schemaName(version, action, false))
}

// validateJSON checks raw against the OCA schema of action's request or
// response in version (2.0.1 or 2.1) and returns it decoded (numbers as
// json.Number).
func validateJSON(version, action string, response bool, raw json.RawMessage) (map[string]any, *RPCError) {
	ss := schemaSets[version]
	if ss == nil {
		return nil, RPCErrorf(KindInternal, "no schemas for %s", version)
	}
	set, err := ss.compiled()
	if err != nil {
		return nil, RPCErrorf(KindInternal, "%v", err)
	}
	name := schemaName(version, action, response)
	if !ss.has(name) {
		return nil, RPCErrorf(KindNotImplemented, "action %s is not a %s message", action, version)
	}
	v, err := jsonschema.Decode(raw)
	if err != nil {
		return nil, RPCErrorf(KindFormat, "the payload is not JSON")
	}
	if err := set.Validate(name, v); err != nil {
		var ve jsonschema.ValidationError
		if errorsAs(err, &ve) && len(ve) > 0 {
			e := ve[0]
			return nil, RPCErrorf(kindOf(e.Keyword), "%s", strings.TrimPrefix(e.String(), "/"))
		}
		return nil, RPCErrorf(KindInternal, "%v", err)
	}
	m, _ := v.(map[string]any)
	return m, nil
}

// validate21 is validateJSON for OCPP 2.1.
func validate21(action string, response bool, raw json.RawMessage) (map[string]any, *RPCError) {
	return validateJSON(V21, action, response, raw)
}

// ValidateMessage checks a 2.0.1 or 2.1 payload against the OCA schema of
// action's request (or response): nil when it is valid. For the
// simulator and tests.
func ValidateMessage(version, action string, response bool, raw json.RawMessage) error {
	if _, verr := validateJSON(version, action, response, raw); verr != nil {
		return &SchemaError{Code: verr.Kind.code(version), Description: verr.Desc, Schema: version + " " + schemaName(version, action, response)}
	}
	return nil
}

// SchemaError is a payload that does not match its OCA schema, with the
// RPC error code it maps to.
type SchemaError struct {
	Code, Description, Schema string
}

func (e *SchemaError) Error() string { return e.Schema + ": " + e.Code + ": " + e.Description }

func errorsAs(err error, ve *jsonschema.ValidationError) bool {
	x, ok := err.(jsonschema.ValidationError)
	if ok {
		*ve = x
	}
	return ok
}

// HasSchema reports whether the OCA publishes a schema for the request of
// action in version.
func HasSchema(version, action string) bool { return hasSchema(version, action) }
