package ocpp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// pair connects two Conns over a real WebSocket.
func pair(t *testing.T, version string, timeout time.Duration, server, client Handler) (*Conn, *Conn, *websocket.Conn) {
	t.Helper()
	got := make(chan *Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{version}})
		if err != nil {
			return
		}
		c := NewConn(ws, version, timeout, server)
		got <- c
		_ = c.Serve(context.Background(), 0)
	}))
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), &websocket.DialOptions{Subprotocols: []string{version}})
	if err != nil {
		t.Fatal(err)
	}
	var cc *Conn
	if client != nil {
		cc = NewConn(ws, version, timeout, client)
		go func() { _ = cc.Serve(context.Background(), 0) }()
		t.Cleanup(func() { cc.Close("done") })
	}
	sc := <-got
	return sc, cc, ws
}

// Covers: SIG-IPE-260
func TestRPCFraming(t *testing.T) {
	var calls atomic.Int32
	echo := func(_ context.Context, action string, p json.RawMessage) (any, error) {
		calls.Add(1)
		switch action {
		case "Echo":
			return p, nil
		case "Slow":
			time.Sleep(300 * time.Millisecond)
			return map[string]any{}, nil
		case "Fail":
			return nil, RPCErrorf(KindProperty, "bad value")
		}
		return nil, errors.New("boom")
	}
	s, c, _ := pair(t, V16, 200*time.Millisecond, echo, echo)
	ctx := context.Background()

	// CALL → CALLRESULT with the payload.
	res, err := c.Call(ctx, "Echo", map[string]any{"a": 1})
	if err != nil || string(res) != `{"a":1}` {
		t.Fatalf("echo: %s %v", res, err)
	}
	// Both directions work on one connection.
	if res, err = s.Call(ctx, "Echo", map[string]any{"b": true}); err != nil || string(res) != `{"b":true}` {
		t.Fatalf("server call: %s %v", res, err)
	}
	// CALLERROR carries the code and description.
	_, err = c.Call(ctx, "Fail", nil)
	var ce *CallError
	if !errors.As(err, &ce) || ce.Code != "PropertyConstraintViolation" || ce.Description != "bad value" {
		t.Fatalf("fail: %v", err)
	}
	// A plain handler error is an InternalError.
	if _, err = c.Call(ctx, "Other", nil); !errors.As(err, &ce) || ce.Code != "InternalError" {
		t.Fatalf("other: %v", err)
	}
	// No answer in time: ErrTimeout, and a late answer is dropped.
	if _, err = c.Call(ctx, "Slow", nil); !errors.Is(err, ErrTimeout) {
		t.Fatalf("slow: %v", err)
	}
	time.Sleep(400 * time.Millisecond)
	if res, err = c.Call(ctx, "Echo", map[string]any{"c": 2}); err != nil || string(res) != `{"c":2}` {
		t.Fatalf("after timeout: %s %v", res, err)
	}
}

// Covers: SIG-IPE-260
func TestRPCOneOutstandingCall(t *testing.T) {
	var inFlight, maxInFlight atomic.Int32
	h := func(_ context.Context, _ string, _ json.RawMessage) (any, error) {
		n := inFlight.Add(1)
		for {
			m := maxInFlight.Load()
			if n <= m || maxInFlight.CompareAndSwap(m, n) {
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
		inFlight.Add(-1)
		return map[string]any{}, nil
	}
	_, c, _ := pair(t, V201, 2*time.Second, h, h)
	done := make(chan error, 5)
	for i := 0; i < 5; i++ {
		go func() { _, err := c.Call(context.Background(), "X", nil); done <- err }()
	}
	for i := 0; i < 5; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if maxInFlight.Load() != 1 {
		t.Fatalf("%d calls were outstanding at once", maxInFlight.Load())
	}
}

// Covers: SIG-IPE-260
func TestRPCMalformedFrames(t *testing.T) {
	h := func(_ context.Context, _ string, _ json.RawMessage) (any, error) { return map[string]any{}, nil }
	for _, tc := range []struct {
		version, in, code string
	}{
		{V16, `[2,"m1","Heartbeat"]`, "FormationViolation"},
		{V201, `[2,"m1","Heartbeat"]`, "RpcFrameworkError"},
		{V201, `[2,{"x":1},"Heartbeat",{}]`, "RpcFrameworkError"}, // answered with "-1"
		{V16, `[2,"m2","Heartbeat",{},5]`, "FormationViolation"},
	} {
		_, _, ws := pair(t, tc.version, time.Second, h, nil)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		if err := ws.Write(ctx, websocket.MessageText, []byte(tc.in)); err != nil {
			t.Fatal(err)
		}
		_, b, err := ws.Read(ctx)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		var arr []any
		_ = json.Unmarshal(b, &arr)
		if len(arr) != 5 || arr[0] != float64(4) || arr[2] != tc.code {
			t.Fatalf("%s %s: got %s", tc.version, tc.in, b)
		}
	}
}

// Covers: SIG-IPE-2108
// OCPP 2.x part 4 errata 2026-06 (4.1 to 4.3): a message of an unknown
// type is ignored, not answered with MessageTypeNotSupported; the next
// CALL is answered normally.
func TestRPCUnknownMessageTypeIgnored(t *testing.T) {
	h := func(_ context.Context, _ string, _ json.RawMessage) (any, error) { return map[string]any{}, nil }
	for _, v := range []string{V16, V201, V21} {
		_, _, ws := pair(t, v, time.Second, h, nil)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = ws.Write(ctx, websocket.MessageText, []byte(`[7,"m2",{}]`))
		_ = ws.Write(ctx, websocket.MessageText, []byte(`[2,"m3","Heartbeat",null]`))
		_, b, err := ws.Read(ctx)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(string(b), `[3,"m3"`) {
			t.Fatalf("%s: the first answer is %s, want the Heartbeat's CALLRESULT", v, b)
		}
	}
}

// Covers: SIG-IPE-261
func TestValidation(t *testing.T) {
	for _, tc := range []struct {
		version, action, payload, code string
	}{
		{V16, "BootNotification", `{"chargePointVendor":"v","chargePointModel":"m"}`, ""},
		{V16, "BootNotification", `{"chargePointVendor":"v"}`, "OccurenceConstraintViolation"},
		{V201, "BootNotification", `{"reason":"PowerUp","chargingStation":{"vendorName":"v"}}`, "OccurrenceConstraintViolation"},
		{V16, "BootNotification", `{"chargePointVendor":"v","chargePointModel":"m","colour":"red"}`, "FormationViolation"},
		{V201, "Heartbeat", `{"extra":1}`, "FormatViolation"},
		{V201, "Heartbeat", `{"customData":{"vendorId":"x"}}`, ""},
		{V16, "BootNotification", `{"chargePointVendor":"123456789012345678901","chargePointModel":"m"}`, "PropertyConstraintViolation"},
		{V16, "StatusNotification", `{"connectorId":1,"errorCode":"NoError","status":"Sleeping"}`, "PropertyConstraintViolation"},
		{V16, "StatusNotification", `{"connectorId":"1","errorCode":"NoError","status":"Available"}`, "TypeConstraintViolation"},
		{V16, "StatusNotification", `{"connectorId":1.5,"errorCode":"NoError","status":"Available"}`, "TypeConstraintViolation"},
		{V16, "StartTransaction", `{"connectorId":1,"idTag":"A","meterStart":0,"timestamp":"yesterday"}`, "PropertyConstraintViolation"},
		{V16, "MeterValues", `{"connectorId":1,"meterValue":[]}`, "OccurenceConstraintViolation"},
		{V16, "MeterValues", `{"connectorId":1,"meterValue":[{"timestamp":"2026-09-24T10:00:00Z","sampledValue":[{"value":"1","unit":"parsecs"}]}]}`, "PropertyConstraintViolation"},
		{V201, "TransactionEvent", `{"eventType":"Started","timestamp":"2026-09-24T10:00:00Z","triggerReason":"Authorized","seqNo":0,"transactionInfo":{"transactionId":"t1"},"meterValue":[{"timestamp":"2026-09-24T10:00:00Z","sampledValue":[{"value":"12"}]}]}`, "TypeConstraintViolation"},
		{V201, "TransactionEvent", `{"eventType":"Started","timestamp":"2026-09-24T10:00:00Z","triggerReason":"Authorized","seqNo":0,"transactionInfo":{"transactionId":"t1"}}`, ""},
		{V16, "Heartbeat", `[1]`, "TypeConstraintViolation"},
		{V16, "Heartbeat", `{`, "FormationViolation"},
	} {
		defs := v16In
		if tc.version == V201 {
			defs = v201In
		}
		_, err := decodeIn(tc.version, tc.action, defs[tc.action], json.RawMessage(tc.payload))
		got := ""
		if err != nil {
			got = err.Kind.code(tc.version)
		}
		if got != tc.code {
			t.Errorf("%s %s %s: got %q (%v), want %q", tc.version, tc.action, tc.payload, got, err, tc.code)
		}
	}
}

// Covers: SIG-IPE-2109
// OCPP 2.1 part 4 clauses 4.1.3, 4.1.4, 4.2.3, 4.2.4: SEND (6) is handled
// and never answered; a CALLRESULT that fails validation is answered with
// CALLRESULTERROR (5) carrying the CALLRESULT's message ID; an unreadable
// message ID is answered with "-1"; message IDs are UUIDs of at most 36
// characters.
func TestRPC21SendAndCallResultError(t *testing.T) {
	got := make(chan bool, 1)
	h := func(ctx context.Context, action string, _ json.RawMessage) (any, error) {
		if action == "NotifyPeriodicEventStream" {
			got <- IsSend(ctx)
		}
		return map[string]any{}, nil
	}
	sc, _, ws := pair(t, V21, time.Second, h, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// SEND: handled as a SEND, nothing comes back; the CALL after it is
	// answered first.
	_ = ws.Write(ctx, websocket.MessageText, []byte(`[6,"s1","NotifyPeriodicEventStream",{"id":1,"pending":0,"basetime":"2026-09-25T00:00:00Z","data":[{"t":0,"v":"1"}]}]`))
	if !<-got {
		t.Fatal("the handler did not see a SEND")
	}
	_ = ws.Write(ctx, websocket.MessageText, []byte(`[2,"c1","Heartbeat",{}]`))
	_, b, err := ws.Read(ctx)
	if err != nil || !strings.HasPrefix(string(b), `[3,"c1"`) {
		t.Fatalf("%s %v", b, err)
	}
	// Unreadable message ID.
	_ = ws.Write(ctx, websocket.MessageText, []byte(`[2,17,"Heartbeat",{}]`))
	_, b, _ = ws.Read(ctx)
	if !strings.HasPrefix(string(b), `[4,"-1","RpcFrameworkError"`) {
		t.Fatalf("%s", b)
	}
	// CALLRESULTERROR for an invalid CALLRESULT.
	sc.CheckResult = func(action string, p json.RawMessage) error { return ValidateMessage(V21, action, true, p) }
	res := make(chan error, 1)
	go func() { _, err := sc.Call(ctx, "GetTransactionStatus", map[string]any{}); res <- err }()
	_, b, _ = ws.Read(ctx)
	var call []any
	_ = json.Unmarshal(b, &call)
	id, _ := call[1].(string)
	if len(id) != 36 || strings.Count(id, "-") != 4 {
		t.Fatalf("message ID %q is not a UUID", id)
	}
	_ = ws.Write(ctx, websocket.MessageText, []byte(`[3,"`+id+`",{"messagesInQueue":"yes"}]`))
	_, b, _ = ws.Read(ctx)
	if !strings.HasPrefix(string(b), `[5,"`+id+`","TypeConstraintViolation"`) {
		t.Fatalf("%s", b)
	}
	var re *ResultError
	if err := <-res; !errors.As(err, &re) {
		t.Fatalf("%v", err)
	}
	// Send is refused on older versions.
	if err := (&Conn{version: V201}).Send(ctx, "X", nil); err == nil {
		t.Fatal("SEND on 2.0.1")
	}
}
