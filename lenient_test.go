// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package ocpp_test

import (
	"encoding/json"
	"testing"

	"github.com/cB-Sigma/ocpp"
)

// A property sent as null is read as absent by Deviations: tolerable when
// the property is optional, never when it is required, and a value of
// another wrong type stays intolerable. Found running lorenzodonini/ocpp-go
// v0.19.0's 2.0.1 charging station against this module: its Reset answer
// carries "statusInfo": null, which the schema refuses (statusInfo is an
// optional object) and Deviations had counted as a type error.
func TestDeviationsNullProperty(t *testing.T) {
	cases := []struct {
		version, action string
		response        bool
		raw             string
		ok, enum, other bool
	}{
		{ocpp.V201, "Reset", true, `{"status":"Accepted","statusInfo":null}`, true, false, true},
		{ocpp.V21, "SetNetworkProfile", true, `{"status":"Accepted","statusInfo":null}`, true, false, true},
		{ocpp.V201, "Reset", true, `{"status":"Sometimes","statusInfo":null}`, true, true, true},
		{ocpp.V201, "Reset", true, `{"status":null}`, false, false, false},
		{ocpp.V201, "Reset", true, `{"status":"Accepted","statusInfo":"none"}`, false, false, false},
		{ocpp.V201, "Reset", true, `{"status":"Accepted","statusInfo":{"reasonCode":null}}`, false, false, false},
		{ocpp.V16, "StopTransaction", false, `{"transactionId":1,"meterStop":2,"timestamp":"2026-10-11T10:00:00Z","idTag":null}`, true, false, true},
	}
	for _, c := range cases {
		raw := json.RawMessage(c.raw)
		_, verr := ocpp.ValidateJSON(c.version, c.action, c.response, raw)
		if verr == nil {
			t.Fatalf("%s %s %s: passed the schema", c.version, c.action, c.raw)
		}
		ok, enum, other := ocpp.Deviations(c.version, c.action, c.response, raw, verr)
		if ok != c.ok || (ok && (enum != c.enum || other != c.other)) {
			t.Errorf("%s %s %s: Deviations = %v %v %v, want %v %v %v", c.version, c.action, c.raw, ok, enum, other, c.ok, c.enum, c.other)
		}
	}
}
