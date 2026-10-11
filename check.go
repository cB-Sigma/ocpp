// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package ocpp

import (
	"encoding/json"
	"strings"
)

// ValidateRequest checks the request of action in version, whichever side
// sends it, and returns the problem as an *RPCError carrying the kind of
// CALLERROR to answer (nil when the payload is valid).
//
// 2.0.1 and 2.1 requests are checked against the OCA schema. 1.6 requests
// are checked against the OCA 1.6 schema when the action has one and then
// against this package's Go type when it has one (the 1.6 security
// extension messages have only the Go type). An action that neither knows
// is reported with KindNotImplemented. The payload of a signed action
// (an action name ending in SignedSuffix) is a JWS and is not checked here.
func ValidateRequest(version, action string, payload json.RawMessage) *RPCError {
	return validateEither(version, action, false, payload)
}

// ValidateResponse is ValidateRequest for the CALLRESULT payload of action.
func ValidateResponse(version, action string, payload json.RawMessage) *RPCError {
	return validateEither(version, action, true, payload)
}

// Known reports whether version has a schema or a Go type for action, sent
// by either side.
func Known(version, action string) bool {
	if version == V16 {
		_, in := v16In[action]
		_, out := v16Out[action]
		return in || out || hasSchema(V16, action)
	}
	return hasSchema(version, action)
}

func validateEither(version, action string, response bool, payload json.RawMessage) *RPCError {
	if strings.HasSuffix(action, SignedSuffix) {
		return nil
	}
	switch version {
	case V201, V21:
		_, verr := validateJSON(version, action, response, payload)
		return verr
	case V16:
		checked := false
		if hasSchema(V16, action) {
			checked = true
			if _, verr := validateJSON(V16, action, response, payload); verr != nil {
				return verr
			}
		}
		md, ok := v16In[action]
		if !ok {
			md, ok = v16Out[action]
		}
		if ok {
			t := md.Req
			if response {
				t = md.Conf
			}
			if _, verr := validate(t, payload, version); verr != nil {
				return verr
			}
			return nil
		}
		if !checked {
			return RPCErrorf(KindNotImplemented, "action %s is not a %s message", action, version)
		}
		return nil
	}
	return RPCErrorf(KindInternal, "version %q is not supported", version)
}
