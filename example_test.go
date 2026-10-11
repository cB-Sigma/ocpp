// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package ocpp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"github.com/cB-Sigma/ocpp"
)

// A payload is checked against the OCA schema of its version; a failure
// carries the CALLERROR code of that version.
func ExampleValidateMessage() {
	err := ocpp.ValidateMessage(ocpp.V21, "Heartbeat", false, json.RawMessage(`{"extra":1}`))
	fmt.Println(err.(*ocpp.SchemaError).Code)
	fmt.Println(ocpp.ValidateMessage(ocpp.V16, "BootNotification", false, json.RawMessage(`{"chargePointVendor":"v","chargePointModel":"m"}`)))
	// Output:
	// FormatViolation
	// <nil>
}

// A 1.6 request is checked against its schema and decoded into its Go type.
func ExampleDecodeIn() {
	md := ocpp.V16In["BootNotification"]
	v, verr := ocpp.DecodeIn(ocpp.V16, "BootNotification", md, json.RawMessage(`{"chargePointVendor":"Acme","chargePointModel":"AC22"}`))
	if verr != nil {
		fmt.Println(verr.Kind.Code(ocpp.V16), verr.Desc)
		return
	}
	fmt.Println(v.(*ocpp.BootNotification16).ChargePointVendor)
	_, verr = ocpp.DecodeIn(ocpp.V16, "BootNotification", md, json.RawMessage(`{"chargePointVendor":"Acme"}`))
	fmt.Println(verr.Kind.Code(ocpp.V16))
	// Output:
	// Acme
	// OccurenceConstraintViolation
}

// A central system accepts a charger's WebSocket in any version, answers
// its CALLs from a handler, and checks its own answers before they go out.
func ExampleNewConn() {
	http.HandleFunc("/ocpp/", func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: ocpp.Subprotocols([]string{ocpp.V21, ocpp.V201, ocpp.V16})})
		if err != nil {
			return
		}
		version := ocpp.VersionOf(ws.Subprotocol())
		c := ocpp.NewConn(ws, version, 30*time.Second, func(ctx context.Context, action string, payload json.RawMessage) (any, error) {
			if _, verr := ocpp.ValidateJSON(version, action, false, payload); verr != nil {
				return nil, verr // answered with the version's CALLERROR code
			}
			switch action {
			case "BootNotification":
				res := map[string]any{"status": "Accepted", "currentTime": time.Now().UTC().Format(time.RFC3339), "interval": 300}
				return ocpp.CheckAnswer(version, action, res, payload)
			}
			return nil, ocpp.RPCErrorf(ocpp.KindNotImplemented, "%s is not handled", action)
		})
		_ = c.Serve(r.Context(), time.Minute)
	})
}
