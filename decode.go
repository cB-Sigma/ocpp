package ocpp

import (
	"encoding/json"
	"reflect"
)

// DecodeIn validates a received request (1.6: our Go types; 2.x: the OCA
// JSON schema) and decodes it into its Go type.
func DecodeIn(version, action string, md MsgDef, payload json.RawMessage) (any, *RPCError) {
	if version == V16 {
		// the OCA 1.6 schema when the action has one (the 1.6 security
		// extension messages have none), then our Go type
		if hasSchema(V16, action) {
			if _, verr := validateJSON(V16, action, false, payload); verr != nil {
				return nil, verr
			}
		}
		return validate(md.Req, payload, version)
	}
	if _, verr := validateJSON(version, action, false, payload); verr != nil {
		return nil, verr
	}
	p := reflect.New(md.Req)
	if err := json.Unmarshal(payload, p.Interface()); err != nil {
		return nil, RPCErrorf(KindType, "%v", err)
	}
	return p.Interface(), nil
}

// CheckAnswer validates our own answer to a 2.x CALL against its response
// schema: a failure is our defect, answered with an InternalError rather
// than an invalid CALLRESULT.
//
// An answer that echoes a value the charger sent outside an enumeration
// (a newer or vendor value the application tolerated) is sent as it
// is: the charger knows its own value. request is the charger's payload.
func CheckAnswer(version, action string, res any, request ...json.RawMessage) (any, error) {
	if res == nil {
		res = struct{}{}
	}
	b, err := json.Marshal(res)
	if err != nil {
		return nil, RPCErrorf(KindInternal, "the answer cannot be encoded")
	}
	if _, verr := validateJSON(version, action, true, b); verr != nil && !(len(request) > 0 && echoesOnly(version, action, b, request[0])) {
		return nil, RPCErrorf(KindInternal, "our %s answer does not match the %s schema: %s", action, version, verr.Desc)
	}
	return json.RawMessage(b), nil
}

func decodeIn(version, action string, md MsgDef, payload json.RawMessage) (any, *RPCError) {
	return DecodeIn(version, action, md, payload)
}

// Validate checks a raw payload against t (a struct type of this
// package, by its `ocpp` tags) and decodes it into a new value of t.
func Validate(t reflect.Type, raw json.RawMessage, version string) (any, *RPCError) {
	return validate(t, raw, version)
}

// ValidateJSON checks raw against the OCA schema of action's request or
// response in version and returns it decoded (numbers as json.Number).
func ValidateJSON(version, action string, response bool, raw json.RawMessage) (map[string]any, *RPCError) {
	return validateJSON(version, action, response, raw)
}
