// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package interop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/cB-Sigma/ocpp"
)

// serve starts the central system on a free loopback port and returns the
// port.
func serve(t *testing.T, s *central) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: s}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return ln.Addr().(*net.TCPAddr).Port
}

// call sends a CALL from the central system and checks the answer against
// the OCA schema of its response.
func call(t *testing.T, c *ocpp.Conn, action string, payload any) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	raw, err := c.Call(ctx, action, payload)
	if err != nil {
		t.Fatalf("%s: %v", action, err)
	}
	if verr := ocpp.ValidateMessage(c.Version(), action, true, raw); verr != nil {
		t.Errorf("%s answer outside the OCA schema: %v", action, verr)
	}
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	return m
}

// mobilityhouse/ocpp's charge point of each version against a central
// system on this module: boot, heartbeat, status, authorization, a
// transaction with meter values, a request with a type error that this
// module answers with TypeConstraintViolation, then remote start and
// stop, reset and configuration or variables from the central system, and
// a request the charge point's schema check refuses.
func TestMobilityhouseChargePoint(t *testing.T) {
	needDocker(t)
	for _, v := range []string{ocpp.V16, ocpp.V201, ocpp.V21} {
		t.Run(v, func(t *testing.T) {
			s := newCentral()
			port := serve(t, s)
			id := "PEER" + strconv.Itoa(port)
			p := startPeer(t, "--add-host=host.docker.internal:host-gateway", image,
				"python", "-u", "cp.py", fmt.Sprintf("ws://host.docker.internal:%d/ocpp/%s", port, id), v)
			p.expect(60*time.Second, "connected", v)

			want := []string{"BootNotification", "Heartbeat", "StatusNotification", "Authorize"}
			if v == ocpp.V16 {
				want = append(want, "StartTransaction", "MeterValues", "StopTransaction")
			} else {
				want = append(want, "TransactionEvent", "TransactionEvent", "TransactionEvent")
			}
			for _, action := range want {
				r := s.next(t)
				if r.action != action || r.err != nil {
					t.Fatalf("got %s (%v), want %s accepted", r.action, r.err, action)
				}
				p.expect(10*time.Second, "result", `"`+action+`"`)
			}
			r := s.next(t)
			if r.action != "StatusNotification" || r.err == nil || r.err.Kind.Code(v) != "TypeConstraintViolation" {
				t.Errorf("the request with a string where an integer belongs: %s %v", r.action, r.err)
			}
			ev := event(t, p.expect(10*time.Second, "error", `"StatusNotification"`))
			if ev["code"] != "TypeConstraintViolation" {
				t.Errorf("mobilityhouse read our CALLERROR as %v", ev)
			}
			if r := s.next(t); r.action != "Heartbeat" || r.err != nil {
				t.Errorf("after the CALLERROR: %s %v", r.action, r.err)
			}
			p.expect(10*time.Second, "waiting")

			c := s.conn(id)
			if v == ocpp.V16 {
				if m := call(t, c, "RemoteStartTransaction", map[string]any{"connectorId": 1, "idTag": "TAG-2"}); m["status"] != "Accepted" {
					t.Errorf("RemoteStartTransaction: %v", m)
				}
				p.expect(10*time.Second, "recv", `"RemoteStartTransaction"`, `"idTag": "TAG-2"`)
				if m := call(t, c, "RemoteStopTransaction", map[string]any{"transactionId": 101}); m["status"] != "Accepted" {
					t.Errorf("RemoteStopTransaction: %v", m)
				}
				p.expect(10*time.Second, "recv", `"RemoteStopTransaction"`, `"transactionId": 101`)
				if m := call(t, c, "Reset", map[string]any{"type": "Hard"}); m["status"] != "Accepted" {
					t.Errorf("Reset: %v", m)
				}
				m := call(t, c, "GetConfiguration", map[string]any{"key": []string{"HeartbeatInterval", "NoSuchKey"}})
				keys, _ := m["configurationKey"].([]any)
				unknown, _ := m["unknownKey"].([]any)
				if len(keys) != 1 || keys[0].(map[string]any)["value"] != "300" || len(unknown) != 1 || unknown[0] != "NoSuchKey" {
					t.Errorf("GetConfiguration: %v", m)
				}
			} else {
				if m := call(t, c, "RequestStartTransaction", map[string]any{"evseId": 1, "remoteStartId": 7,
					"idToken": map[string]any{"idToken": "TAG-2", "type": "ISO14443"}}); m["status"] != "Accepted" {
					t.Errorf("RequestStartTransaction: %v", m)
				}
				p.expect(10*time.Second, "recv", `"RequestStartTransaction"`, `"remoteStartId": 7`)
				if m := call(t, c, "RequestStopTransaction", map[string]any{"transactionId": "PEER-TX-2"}); m["status"] != "Accepted" {
					t.Errorf("RequestStopTransaction: %v", m)
				}
				p.expect(10*time.Second, "recv", `"RequestStopTransaction"`, `"PEER-TX-2"`)
				if m := call(t, c, "Reset", map[string]any{"type": "OnIdle"}); m["status"] != "Accepted" {
					t.Errorf("Reset: %v", m)
				}
				cv := map[string]any{"component": map[string]any{"name": "OCPPCommCtrlr"}, "variable": map[string]any{"name": "HeartbeatInterval"}}
				set := map[string]any{"component": cv["component"], "variable": cv["variable"], "attributeValue": "120"}
				call(t, c, "SetVariables", map[string]any{"setVariableData": []any{set}})
				m := call(t, c, "GetVariables", map[string]any{"getVariableData": []any{cv}})
				res, _ := m["getVariableResult"].([]any)
				if len(res) != 1 || res[0].(map[string]any)["attributeValue"] != "120" {
					t.Errorf("GetVariables: %v", m)
				}
			}

			// A Reset outside the enumeration: mobilityhouse's schema check
			// answers with a CALLERROR that this module returns as a
			// *CallError, and the connection carries on.
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			_, err := c.Call(ctx, "Reset", map[string]any{"type": "Sometimes"})
			var ce *ocpp.CallError
			if !errors.As(err, &ce) || ce.Code == "" {
				t.Fatalf("a Reset outside the enumeration: %v, want a CALLERROR", err)
			}
			t.Logf("mobilityhouse answered the invalid Reset with %s", ce.Code)
			if v != ocpp.V16 && ce.Code != "FormatViolation" {
				t.Errorf("CALLERROR code %s, want FormatViolation", ce.Code)
			}
			if m := call(t, c, "ClearCache", map[string]any{}); m["status"] != "Accepted" {
				t.Errorf("ClearCache after the CALLERROR: %v", m)
			}
		})
	}
}
