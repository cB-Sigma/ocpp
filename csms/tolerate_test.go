// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package csms_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/cB-Sigma/ocpp"
	"github.com/cB-Sigma/ocpp/chargepoint"
	"github.com/cB-Sigma/ocpp/csms"
)

// OnInvalid lets an application take a deviation (here a vendor value
// outside an enumeration, which ocpp.Deviations calls tolerable) and
// refuse the rest; the answer echoing the vendor value is sent as is.
func TestOnInvalidTolerates(t *testing.T) {
	s := csms.New(csms.Config{Timeout: 5 * time.Second,
		OnInvalid: func(st *csms.Station, action string, p json.RawMessage, verr *ocpp.RPCError) error {
			if ok, _, _ := ocpp.Deviations(st.Version(), action, false, p, verr); ok {
				return nil
			}
			return verr
		}})
	got := make(chan string, 2)
	s.Handle("StatusNotification", func(_ context.Context, _ *csms.Station, p json.RawMessage) (any, error) {
		var r struct {
			ConnectorStatus string `json:"connectorStatus"`
		}
		_ = json.Unmarshal(p, &r)
		got <- r.ConnectorStatus
		return map[string]any{}, nil
	})
	c := chargepoint.New(chargepoint.Config{URL: serve(t, s), ID: "TOL", Versions: []string{ocpp.V201}, SkipValidation: true, Timeout: 5 * time.Second})
	if err := c.Connect(t.Context()); err != nil {
		t.Fatal(err)
	}
	st := map[string]any{"timestamp": time.Now().UTC().Format(time.RFC3339), "connectorStatus": "VendorBusy", "evseId": 1, "connectorId": 1}
	if _, err := c.Call(t.Context(), "StatusNotification", st); err != nil {
		t.Fatalf("a tolerated deviation was refused: %v", err)
	}
	if g := <-got; g != "VendorBusy" {
		t.Errorf("handler saw %q", g)
	}
	st["connectorId"] = "one" // a type error is never tolerable
	var ce *ocpp.CallError
	if _, err := c.Call(t.Context(), "StatusNotification", st); !errors.As(err, &ce) || ce.Code != "TypeConstraintViolation" {
		t.Errorf("a type error: %v", err)
	}
}
