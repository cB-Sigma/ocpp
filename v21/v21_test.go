// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package v21_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/cB-Sigma/ocpp"
	"github.com/cB-Sigma/ocpp/chargepoint"
	"github.com/cB-Sigma/ocpp/csms"
	"github.com/cB-Sigma/ocpp/internal/gen21"
	"github.com/cB-Sigma/ocpp/v21"
)

// messages.go is what the generator makes of the embedded schemas now.
func TestGeneratedUpToDate(t *testing.T) {
	files, err := ocpp.SchemaFiles("2.1")
	if err != nil {
		t.Fatal(err)
	}
	want, err := gen21.Generate(files, ocpp.SendActions21)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("messages.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("messages.go is stale: run go generate ./v21")
	}
}

// Every action of the 2.1 direction tables has types, and every type
// has a schema.
func TestEveryActionTyped(t *testing.T) {
	for _, set := range []map[string]bool{ocpp.In21, ocpp.Out21, ocpp.SendActions21} {
		for a := range set {
			m, ok := v21.Messages[a]
			if !ok {
				t.Errorf("%s has no types", a)
				continue
			}
			if m.Send != ocpp.SendActions21[a] || (m.Response == nil) != m.Send {
				t.Errorf("%s: %+v", a, m)
			}
		}
	}
	if len(v21.Messages) != len(ocpp.In21)+len(ocpp.Out21)+len(ocpp.SendActions21)-1 { // DataTransfer goes both ways
		t.Errorf("%d typed actions", len(v21.Messages))
	}
}

// For every 2.1 request and response, a payload with only the required
// properties and one with every property (and an unnamed one in each open
// object) decode into the generated type and encode back to the same
// JSON, which the schema still accepts.
func TestEveryMessageRoundTrips(t *testing.T) {
	files, err := ocpp.SchemaFiles("2.1")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for action, m := range v21.Messages {
		for _, side := range []struct {
			typ      reflect.Type
			response bool
			file     string
		}{{m.Request, false, action + "Request"}, {m.Response, true, action + "Response"}} {
			if side.typ == nil {
				continue
			}
			file := side.file
			if m.Send {
				file = action
			}
			for _, full := range []bool{false, true} {
				inst, err := gen21.Instance(files[file], full)
				if err != nil {
					t.Fatalf("%s: %v", file, err)
				}
				raw, _ := json.Marshal(inst)
				check := ocpp.ValidateRequest
				if side.response {
					check = ocpp.ValidateResponse
				}
				if verr := check(ocpp.V21, action, raw); verr != nil {
					t.Fatalf("%s full=%v: the test payload is not valid: %v", file, full, verr)
				}
				p := reflect.New(side.typ)
				if err := json.Unmarshal(raw, p.Interface()); err != nil {
					t.Errorf("%s full=%v: decode: %v", file, full, err)
					continue
				}
				back, err := json.Marshal(p.Interface())
				if err != nil {
					t.Errorf("%s full=%v: encode: %v", file, full, err)
					continue
				}
				var a, b any
				_ = json.Unmarshal(raw, &a)
				_ = json.Unmarshal(back, &b)
				if !reflect.DeepEqual(a, b) {
					t.Errorf("%s full=%v: round trip changed it\n in:  %s\n out: %s", file, full, raw, back)
				}
				if verr := check(ocpp.V21, action, back); verr != nil {
					t.Errorf("%s full=%v: the encoded payload is not valid: %v", file, full, verr)
				}
				n++
			}
		}
	}
	if n < 300 {
		t.Errorf("only %d payloads checked", n)
	}
}

// Typed calls both ways between the csms and chargepoint roles.
func TestTypedCall(t *testing.T) {
	s := csms.New(csms.Config{Timeout: 5 * time.Second})
	s.Handle("BootNotification", func(_ context.Context, _ *csms.Station, p json.RawMessage) (any, error) {
		req, err := v21.Decode[v21.BootNotificationRequest](p)
		if err != nil {
			return nil, err
		}
		if req.Reason != v21.BootReasonPowerUp || req.ChargingStation.Model != "M" {
			return nil, ocpp.RPCErrorf(ocpp.KindGeneric, "unexpected %+v", req)
		}
		return v21.BootNotificationResponse{CurrentTime: "2026-10-11T12:00:00Z", Interval: 60, Status: v21.RegistrationStatusAccepted}, nil
	})
	ts := httptest.NewServer(s)
	defer ts.Close()
	c := chargepoint.New(chargepoint.Config{URL: "ws" + strings.TrimPrefix(ts.URL, "http"), ID: "TYPED", Versions: []string{ocpp.V21}, Timeout: 5 * time.Second})
	c.Handle("Reset", func(_ context.Context, _ *chargepoint.Client, p json.RawMessage) (any, error) {
		if _, err := v21.Decode[v21.ResetRequest](p); err != nil {
			return nil, err
		}
		return v21.ResetResponse{Status: v21.ResetStatusAccepted}, nil
	})
	if err := c.Connect(t.Context()); err != nil {
		t.Fatal(err)
	}
	boot, err := v21.Call[v21.BootNotificationResponse](t.Context(), c, v21.BootNotificationRequest{
		Reason: v21.BootReasonPowerUp, ChargingStation: v21.ChargingStation{Model: "M", VendorName: "V"}})
	if err != nil || boot.Interval != 60 || boot.Status != v21.RegistrationStatusAccepted {
		t.Fatalf("boot: %+v %v", boot, err)
	}
	st := s.Station("TYPED")
	res, err := v21.Call[v21.ResetResponse](t.Context(), st, v21.ResetRequest{Type: v21.ResetImmediate})
	if err != nil || res.Status != v21.ResetStatusAccepted {
		t.Fatalf("reset: %+v %v", res, err)
	}
}

func TestDecodeChecksTheSchema(t *testing.T) {
	r, err := v21.Decode[v21.ResetRequest](json.RawMessage(`{"type":"OnIdle","evseId":1}`))
	if err != nil || r.Type != v21.ResetOnIdle || r.EvseID == nil || *r.EvseID != 1 {
		t.Fatalf("%+v %v", r, err)
	}
	_, err = v21.Decode[v21.ResetRequest](json.RawMessage(`{"type":"Sometimes"}`))
	var re *ocpp.RPCError
	if !errors.As(err, &re) || re.Kind.Code(ocpp.V21) != "PropertyConstraintViolation" {
		t.Fatalf("an enumeration outside the schema: %v", err)
	}
	res, err := v21.Decode[v21.ResetResponse](json.RawMessage(`{"status":"Scheduled"}`))
	if err != nil || res.Status != v21.ResetStatusScheduled {
		t.Fatalf("%+v %v", res, err)
	}
	cd, err := v21.Decode[v21.HeartbeatRequest](json.RawMessage(`{"customData":{"vendorId":"acme","firmwareBuild":"7"}}`))
	if err != nil || cd.CustomData == nil || cd.CustomData.VendorID != "acme" || string(cd.CustomData.Extra["firmwareBuild"]) != `"7"` {
		t.Fatalf("custom data: %+v %v", cd.CustomData, err)
	}
	b, _ := json.Marshal(cd)
	if string(b) != `{"customData":{"vendorId":"acme","firmwareBuild":"7"}}` {
		t.Errorf("custom data written back as %s", b)
	}
}
