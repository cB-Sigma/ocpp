// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package interop

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	slcsms "github.com/seanlee0923/ocpp/csms"
	slprotocol "github.com/seanlee0923/ocpp/protocol"
	slstation "github.com/seanlee0923/ocpp/station"
	slv21 "github.com/seanlee0923/ocpp/v21"

	"github.com/cB-Sigma/ocpp"
	"github.com/cB-Sigma/ocpp/chargepoint"
)

// seanlee0923/ocpp's 2.1 station (typed, generated from the 2.1 schemas)
// against a central system on this module's csms package: boot and
// heartbeat from them, a Reset from us.
func TestSeanleeStation21(t *testing.T) {
	s := newCentral()
	ts := httptest.NewServer(s)
	defer ts.Close()

	resets := make(chan string, 1)
	st, err := slstation.New(slstation.Config{URL: "ws" + strings.TrimPrefix(ts.URL, "http") + "/ocpp", Identity: "SL21", Version: slprotocol.OCPP21, CallTimeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if err := slstation.Handle(st, func(_ context.Context, r slv21.ResetRequest) (slv21.ResetConfirmation, error) {
		resets <- string(r.Type)
		return slv21.ResetConfirmation{Status: slv21.ResetConfirmationResetStatusEnumAccepted}, nil
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go func() { _ = st.Run(ctx) }()
	select {
	case <-s.conned:
	case <-time.After(10 * time.Second):
		t.Fatal("the station did not connect")
	}
	boot, err := slstation.Call[slv21.BootNotificationRequest, slv21.BootNotificationConfirmation](ctx, st, slv21.BootNotificationRequest{
		Reason: slv21.BootNotificationRequestBootReasonEnumPowerUp, ChargingStation: slv21.BootNotificationRequestChargingStation{Model: "M", VendorName: "V"}})
	if err != nil || boot.Status != "Accepted" || boot.Interval != 300 {
		t.Fatalf("boot: %+v %v", boot, err)
	}
	if _, err := slstation.Call[slv21.HeartbeatRequest, slv21.HeartbeatConfirmation](ctx, st, slv21.HeartbeatRequest{}); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	for _, want := range []string{"BootNotification", "Heartbeat"} {
		if r := s.next(t); r.action != want || r.err != nil {
			t.Errorf("central system got %s (%v), want %s accepted", r.action, r.err, want)
		}
	}
	var res struct {
		Status string `json:"status"`
	}
	if err := s.srv.Station("SL21").CallInto(ctx, "Reset", map[string]any{"type": "OnIdle"}, &res); err != nil || res.Status != "Accepted" {
		t.Fatalf("reset: %+v %v", res, err)
	}
	expectGot(t, resets, "OnIdle")
}

// This module's chargepoint (2.1) against seanlee0923/ocpp's CSMS: boot,
// a SEND (NotifyPeriodicEventStream, unconfirmed) that its typed SEND
// handler decodes, and a Reset from them.
func TestChargepointAgainstSeanleeCSMS21(t *testing.T) {
	router := slcsms.NewRouter()
	got := make(chan string, 4)
	if err := slcsms.Handle(router, func(_ context.Context, _ *slcsms.Session, r slv21.BootNotificationRequest) (slv21.BootNotificationConfirmation, error) {
		got <- "BootNotification " + r.ChargingStation.Model
		return slv21.BootNotificationConfirmation{CurrentTime: time.Now().UTC().Format(time.RFC3339), Interval: 300, Status: "Accepted"}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := slcsms.HandleSend(router, func(_ context.Context, _ *slcsms.Session, r slv21.NotifyPeriodicEventStreamRequest) error {
		got <- fmt.Sprintf("NotifyPeriodicEventStream %d %d", r.ID, len(r.Data))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	connected := make(chan *slcsms.Session, 1)
	srv, err := slcsms.New(slcsms.Config{Router: router, Versions: []slprotocol.Version{slprotocol.OCPP21}, OnConnect: func(s *slcsms.Session) { connected <- s }})
	if err != nil {
		t.Fatal(err)
	}
	// Its identity is the whole request path, so it is mounted under /ocpp.
	ts := httptest.NewServer(http.StripPrefix("/ocpp", srv))
	defer ts.Close()

	resets := make(chan string, 1)
	c := chargepoint.New(chargepoint.Config{URL: "ws" + strings.TrimPrefix(ts.URL, "http") + "/ocpp", ID: "OURS21", Timeout: 10 * time.Second})
	c.Handle("Reset", resetHandler(resets))
	if err := c.Connect(t.Context()); err != nil {
		t.Fatal(err)
	}
	if c.Version() != ocpp.V21 {
		t.Fatalf("version %q", c.Version())
	}
	var session *slcsms.Session
	select {
	case session = <-connected:
	case <-time.After(10 * time.Second):
		t.Fatal("no session")
	}
	if session.Identity() != "OURS21" {
		t.Errorf("identity %q", session.Identity())
	}
	boot := map[string]any{"reason": "PowerUp", "chargingStation": map[string]any{"model": "Lib", "vendorName": "Sigma"}}
	var b struct {
		Status string `json:"status"`
	}
	if err := c.CallInto(t.Context(), "BootNotification", boot, &b); err != nil || b.Status != "Accepted" {
		t.Fatalf("boot: %+v %v", b, err)
	}
	expectGot(t, got, "BootNotification Lib")
	stream := map[string]any{"id": 4, "pending": 0, "basetime": time.Now().UTC().Format(time.RFC3339),
		"data": []any{map[string]any{"t": 0, "v": "230.1"}, map[string]any{"t": 1.5, "v": "229.8"}}}
	if err := c.Send(t.Context(), "NotifyPeriodicEventStream", stream); err != nil {
		t.Fatal(err)
	}
	expectGot(t, got, "NotifyPeriodicEventStream 4 2")

	r, err := slcsms.Call[slv21.ResetRequest, slv21.ResetConfirmation](t.Context(), session, slv21.ResetRequest{Type: "Immediate"})
	if err != nil || r.Status != "Accepted" {
		t.Fatalf("reset from seanlee: %+v %v", r, err)
	}
	expectGot(t, resets, "Reset Immediate")

}
