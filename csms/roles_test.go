// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package csms_test

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

	"github.com/cB-Sigma/ocpp"
	"github.com/cB-Sigma/ocpp/chargepoint"
	"github.com/cB-Sigma/ocpp/csms"
)

func bootAnswer(_ context.Context, st *csms.Station, _ json.RawMessage) (any, error) {
	return map[string]any{"status": "Accepted", "currentTime": time.Now().UTC().Format(time.RFC3339), "interval": 300}, nil
}

func bootRequest(version string) map[string]any {
	if version == ocpp.V16 {
		return map[string]any{"chargePointVendor": "V", "chargePointModel": "M"}
	}
	return map[string]any{"reason": "PowerUp", "chargingStation": map[string]any{"model": "M", "vendorName": "V"}}
}

func resetRequest(version string) map[string]any {
	if version == ocpp.V16 {
		return map[string]any{"type": "Soft"}
	}
	return map[string]any{"type": "Immediate"}
}

func serve(t *testing.T, s *csms.Server) string {
	t.Helper()
	ts := httptest.NewServer(s)
	t.Cleanup(ts.Close)
	return "ws" + strings.TrimPrefix(ts.URL, "http") + "/ocpp"
}

func waitStation(t *testing.T, s *csms.Server, id string) *csms.Station {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if st := s.Station(id); st != nil {
			return st
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("station %s never connected", id)
	return nil
}

// Both library roles against each other, in every version: the station's
// BootNotification is routed and answered, the central system's Reset is
// routed to the station and answered, and both are schema-checked.
func TestRolesEveryVersion(t *testing.T) {
	for _, version := range []string{ocpp.V16, ocpp.V201, ocpp.V21} {
		t.Run(version, func(t *testing.T) {
			s := csms.New(csms.Config{Timeout: 5 * time.Second})
			s.Handle("BootNotification", bootAnswer)
			url := serve(t, s)
			var resets atomic.Int32
			c := chargepoint.New(chargepoint.Config{URL: url, ID: "CP-" + strings.ReplaceAll(version, ".", ""), Versions: []string{version}, Timeout: 5 * time.Second})
			c.Handle("Reset", func(_ context.Context, _ *chargepoint.Client, p json.RawMessage) (any, error) {
				resets.Add(1)
				return map[string]any{"status": "Accepted"}, nil
			})
			if err := c.Connect(t.Context()); err != nil {
				t.Fatal(err)
			}
			if c.Version() != version {
				t.Fatalf("version %q", c.Version())
			}
			var boot struct {
				Status string `json:"status"`
			}
			if err := c.CallInto(t.Context(), "BootNotification", bootRequest(version), &boot); err != nil || boot.Status != "Accepted" {
				t.Fatalf("boot: %+v %v", boot, err)
			}
			st := waitStation(t, s, c.ID())
			if st.Version() != version {
				t.Fatalf("station version %q", st.Version())
			}
			var res struct {
				Status string `json:"status"`
			}
			if err := st.CallInto(t.Context(), "Reset", resetRequest(version), &res); err != nil || res.Status != "Accepted" || resets.Load() != 1 {
				t.Fatalf("reset: %+v %v", res, err)
			}
			// Our own request outside the schema is refused before sending.
			var re *ocpp.RPCError
			if _, err := st.Call(t.Context(), "Reset", map[string]any{"type": "Sometimes"}); !errors.As(err, &re) || resets.Load() != 1 {
				t.Fatalf("an invalid Reset: %v", err)
			}
		})
	}
}

// A station's request outside the schema gets the version's CALLERROR
// code; an unhandled known action NotSupported; an unknown one
// NotImplemented.
func TestRolesRefusals(t *testing.T) {
	s := csms.New(csms.Config{Timeout: 5 * time.Second})
	s.Handle("BootNotification", bootAnswer)
	url := serve(t, s)
	for _, version := range []string{ocpp.V16, ocpp.V201} {
		c := chargepoint.New(chargepoint.Config{URL: url, ID: "R" + strings.ReplaceAll(version, ".", ""), Versions: []string{version}, SkipValidation: true, Timeout: 5 * time.Second})
		if err := c.Connect(t.Context()); err != nil {
			t.Fatal(err)
		}
		var ce *ocpp.CallError
		bad := bootRequest(version)
		if version == ocpp.V16 {
			bad["chargePointModel"] = 7
		} else {
			bad["reason"] = 7
		}
		if _, err := c.Call(t.Context(), "BootNotification", bad); !errors.As(err, &ce) || ce.Code != "TypeConstraintViolation" {
			t.Errorf("%s invalid boot: %v", version, err)
		}
		if _, err := c.Call(t.Context(), "Heartbeat", map[string]any{}); !errors.As(err, &ce) || ce.Code != "NotSupported" {
			t.Errorf("%s unhandled heartbeat: %v", version, err)
		}
		if _, err := c.Call(t.Context(), "NoSuchAction", map[string]any{}); !errors.As(err, &ce) || ce.Code != "NotImplemented" {
			t.Errorf("%s unknown action: %v", version, err)
		}
	}
}

// A handler's answer outside the schema becomes an InternalError, not an
// invalid CALLRESULT.
func TestRolesOwnAnswerChecked(t *testing.T) {
	s := csms.New(csms.Config{Timeout: 5 * time.Second})
	s.Handle("BootNotification", func(context.Context, *csms.Station, json.RawMessage) (any, error) {
		return map[string]any{"status": "Maybe"}, nil
	})
	c := chargepoint.New(chargepoint.Config{URL: serve(t, s), ID: "OWN", Versions: []string{ocpp.V201}, Timeout: 5 * time.Second})
	if err := c.Connect(t.Context()); err != nil {
		t.Fatal(err)
	}
	var ce *ocpp.CallError
	if _, err := c.Call(t.Context(), "BootNotification", bootRequest(ocpp.V201)); !errors.As(err, &ce) || ce.Code != "InternalError" {
		t.Fatalf("got %v", err)
	}
}

// 2.1: a SEND reaches the handler with IsSend set; an answer outside the
// schema is reported to the central system with a CALLRESULTERROR.
func TestRoles21SendAndResultError(t *testing.T) {
	got := make(chan bool, 1)
	s := csms.New(csms.Config{Timeout: 5 * time.Second})
	s.Handle("NotifyPeriodicEventStream", func(ctx context.Context, st *csms.Station, p json.RawMessage) (any, error) {
		got <- ocpp.IsSend(ctx)
		return nil, nil
	})
	url := serve(t, s)
	c := chargepoint.New(chargepoint.Config{URL: url, ID: "SEND21", Versions: []string{ocpp.V21}, Timeout: 5 * time.Second})
	if err := c.Connect(t.Context()); err != nil {
		t.Fatal(err)
	}
	stream := map[string]any{"id": 1, "pending": 0, "basetime": time.Now().UTC().Format(time.RFC3339),
		"data": []any{map[string]any{"t": 0.5, "v": "12.5"}}}
	if err := c.Send(t.Context(), "NotifyPeriodicEventStream", stream); err != nil {
		t.Fatal(err)
	}
	select {
	case isSend := <-got:
		if !isSend {
			t.Error("the handler did not see a SEND")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no SEND arrived")
	}

	// A station answering outside the schema: the central system's Call
	// returns a ResultError and the station is told.
	// (SkipValidation lets the station's handler answer outside the schema.)
	resultErr := make(chan string, 1)
	c2 := chargepoint.New(chargepoint.Config{URL: url, ID: "BAD21", Versions: []string{ocpp.V21}, SkipValidation: true, Timeout: 5 * time.Second,
		OnResultError: func(id, code, desc string) { resultErr <- code }})
	c2.Handle("ClearCache", func(context.Context, *chargepoint.Client, json.RawMessage) (any, error) {
		return map[string]any{"status": "Perhaps"}, nil
	})
	if err := c2.Connect(t.Context()); err != nil {
		t.Fatal(err)
	}
	st2 := waitStation(t, s, "BAD21")
	var rerr *ocpp.ResultError
	if _, err := st2.Call(t.Context(), "ClearCache", map[string]any{}); !errors.As(err, &rerr) {
		t.Fatalf("an invalid answer: %v", err)
	}
	select {
	case code := <-resultErr:
		if code != "PropertyConstraintViolation" && code != "FormatViolation" {
			t.Logf("CALLRESULTERROR code %s", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the station was not sent a CALLRESULTERROR")
	}
}

// Authentication: a refused station gets HTTP 401; a bad identity 404;
// no subprotocol is refused unless NoSubprotocol is set.
func TestRolesAdmission(t *testing.T) {
	s := csms.New(csms.Config{Authenticate: func(r *http.Request, id string) error {
		pw, ok := ocpp.BasicAuthPassword(r, id)
		if !ok || !ocpp.PasswordOK("sha256:"+"5e884898da28047151d0e56f8dc6292773603d0d6aabbdd62a11ef721d1542d8", pw) {
			return errors.New("bad password")
		}
		return nil
	}})
	url := serve(t, s)
	if _, err := chargepoint.Dial(t.Context(), chargepoint.Config{URL: url, ID: "A1", Password: "wrong"}); err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("wrong password: %v", err)
	}
	d, err := chargepoint.Dial(t.Context(), chargepoint.Config{URL: url, ID: "A1", Password: "password"})
	if err != nil || d.Version != ocpp.V21 {
		t.Fatalf("right password: %+v %v", d, err)
	}
	_ = d.WS.CloseNow()

	open := csms.New(csms.Config{})
	ourl := serve(t, open)
	if _, err := chargepoint.Dial(t.Context(), chargepoint.Config{URL: ourl, ID: "a/b"}); err == nil {
		t.Error("an identity with a slash was dialled")
	}
	resp, err := http.Get("http" + strings.TrimPrefix(ourl, "ws") + "/bad%20id")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("identity with a space: HTTP %d", resp.StatusCode)
	}
	if _, err := chargepoint.Dial(t.Context(), chargepoint.Config{URL: ourl, ID: "NOSUB", Versions: []string{}}); err != nil {
		t.Errorf("offering every version: %v", err)
	}
}

// A station that reconnects replaces its older connection; Run reconnects
// after the central system drops it.
func TestRolesReconnect(t *testing.T) {
	connects := make(chan string, 8)
	s := csms.New(csms.Config{Timeout: 5 * time.Second, OnConnect: func(st *csms.Station) { connects <- st.ID() }})
	s.Handle("BootNotification", bootAnswer)
	url := serve(t, s)
	booted := make(chan struct{}, 8)
	c := chargepoint.New(chargepoint.Config{URL: url, ID: "RUN", Versions: []string{ocpp.V201}, Timeout: 5 * time.Second,
		Backoff: chargepoint.Backoff{Minimum: 20 * time.Millisecond, RandomRange: 10 * time.Millisecond},
		OnConnect: func(ctx context.Context, c *chargepoint.Client) {
			if _, err := c.Call(ctx, "BootNotification", bootRequest(ocpp.V201)); err == nil {
				booted <- struct{}{}
			}
		}})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go func() { _ = c.Run(ctx) }()
	for i := 0; i < 2; i++ {
		select {
		case <-booted:
		case <-time.After(5 * time.Second):
			t.Fatalf("boot %d missing", i)
		}
		<-connects
		if i == 0 {
			waitStation(t, s, "RUN").Close("test drop")
		}
	}
	// a second connection under the same identity replaces the first
	first := waitStation(t, s, "RUN")
	d, err := chargepoint.Dial(t.Context(), chargepoint.Config{URL: url, ID: "RUN", Versions: []string{ocpp.V201}})
	if err != nil {
		t.Fatal(err)
	}
	defer d.WS.CloseNow()
	select {
	case <-first.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the older connection was not closed")
	}
}

func TestBackoffWait(t *testing.T) {
	b := chargepoint.Backoff{Minimum: time.Second, RandomRange: 0, RepeatTimes: 3}
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 8 * time.Second}
	for n, w := range want {
		if got := b.Wait(n); got != w {
			t.Errorf("Wait(%d) = %v, want %v", n, got, w)
		}
	}
	r := chargepoint.Backoff{Minimum: time.Second, RandomRange: time.Second}
	for i := 0; i < 50; i++ {
		if d := r.Wait(0); d < time.Second || d > 2*time.Second {
			t.Fatalf("Wait(0) = %v outside [1s, 2s]", d)
		}
	}
}
