// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package interop

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"testing"
	"time"

	ocpp16 "github.com/lorenzodonini/ocpp-go/ocpp1.6"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/core"
	types16 "github.com/lorenzodonini/ocpp-go/ocpp1.6/types"
	ocpp2 "github.com/lorenzodonini/ocpp-go/ocpp2.0.1"
	"github.com/lorenzodonini/ocpp-go/ocpp2.0.1/availability"
	"github.com/lorenzodonini/ocpp-go/ocpp2.0.1/provisioning"
	types2 "github.com/lorenzodonini/ocpp-go/ocpp2.0.1/types"

	"github.com/cB-Sigma/ocpp"
	"github.com/cB-Sigma/ocpp/chargepoint"
)

// waitListening waits until something accepts on port.
func waitListening(t *testing.T, port int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if c, err := net.Dial("tcp", "127.0.0.1:"+strconv.Itoa(port)); err == nil {
			c.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("nothing listens on %d", port)
}

// cs16 is ocpp-go's 1.6 central system core handler.
type cs16 struct{ got chan string }

func (h *cs16) OnAuthorize(id string, r *core.AuthorizeRequest) (*core.AuthorizeConfirmation, error) {
	h.got <- "Authorize " + r.IdTag
	return core.NewAuthorizationConfirmation(types16.NewIdTagInfo(types16.AuthorizationStatusAccepted)), nil
}
func (h *cs16) OnBootNotification(id string, r *core.BootNotificationRequest) (*core.BootNotificationConfirmation, error) {
	h.got <- "BootNotification " + r.ChargePointModel
	return core.NewBootNotificationConfirmation(types16.NewDateTime(time.Now()), 300, core.RegistrationStatusAccepted), nil
}
func (h *cs16) OnDataTransfer(string, *core.DataTransferRequest) (*core.DataTransferConfirmation, error) {
	return nil, errUnused
}
func (h *cs16) OnHeartbeat(string, *core.HeartbeatRequest) (*core.HeartbeatConfirmation, error) {
	h.got <- "Heartbeat"
	return core.NewHeartbeatConfirmation(types16.NewDateTime(time.Now())), nil
}
func (h *cs16) OnMeterValues(string, *core.MeterValuesRequest) (*core.MeterValuesConfirmation, error) {
	return core.NewMeterValuesConfirmation(), nil
}
func (h *cs16) OnStatusNotification(id string, r *core.StatusNotificationRequest) (*core.StatusNotificationConfirmation, error) {
	h.got <- "StatusNotification " + string(r.Status)
	return core.NewStatusNotificationConfirmation(), nil
}
func (h *cs16) OnStartTransaction(string, *core.StartTransactionRequest) (*core.StartTransactionConfirmation, error) {
	return core.NewStartTransactionConfirmation(types16.NewIdTagInfo(types16.AuthorizationStatusAccepted), 7), nil
}
func (h *cs16) OnStopTransaction(string, *core.StopTransactionRequest) (*core.StopTransactionConfirmation, error) {
	return core.NewStopTransactionConfirmation(), nil
}

func resetHandler(got chan string) chargepoint.HandlerFunc {
	return func(_ context.Context, _ *chargepoint.Client, p json.RawMessage) (any, error) {
		var r struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(p, &r)
		got <- "Reset " + r.Type
		return map[string]any{"status": "Accepted"}, nil
	}
}

// This module's chargepoint (1.6) against ocpp-go's 1.6 central system:
// boot, heartbeat, status and authorization from us, a Reset from them.
func TestChargepointAgainstOCPPGo16(t *testing.T) {
	port := freePort(t)
	h := &cs16{got: make(chan string, 16)}
	cs := ocpp16.NewCentralSystem(nil, nil)
	cs.SetCoreHandler(h)
	connected := make(chan string, 2)
	cs.SetNewChargePointHandler(func(c ocpp16.ChargePointConnection) { connected <- c.ID() })
	go cs.Start(port, "/ocpp/{ws}")
	defer cs.Stop()
	waitListening(t, port)

	got := make(chan string, 4)
	c := chargepoint.New(chargepoint.Config{URL: fmt.Sprintf("ws://127.0.0.1:%d/ocpp", port), ID: "OURS16", Versions: []string{ocpp.V16}, Timeout: 10 * time.Second})
	c.Handle("Reset", resetHandler(got))
	if err := c.Connect(t.Context()); err != nil {
		t.Fatal(err)
	}
	if id := <-connected; id != "OURS16" {
		t.Fatalf("ocpp-go saw %q", id)
	}
	var boot struct {
		Status   string `json:"status"`
		Interval int    `json:"interval"`
	}
	if err := c.CallInto(t.Context(), "BootNotification", map[string]any{"chargePointVendor": "Sigma", "chargePointModel": "Lib"}, &boot); err != nil || boot.Status != "Accepted" || boot.Interval != 300 {
		t.Fatalf("boot: %+v %v", boot, err)
	}
	if _, err := c.Call(t.Context(), "Heartbeat", nil); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if _, err := c.Call(t.Context(), "StatusNotification", map[string]any{"connectorId": 1, "errorCode": "NoError", "status": "Available"}); err != nil {
		t.Fatalf("status: %v", err)
	}
	var auth struct {
		IdTagInfo struct {
			Status string `json:"status"`
		} `json:"idTagInfo"`
	}
	if err := c.CallInto(t.Context(), "Authorize", map[string]any{"idTag": "TAG-1"}, &auth); err != nil || auth.IdTagInfo.Status != "Accepted" {
		t.Fatalf("authorize: %+v %v", auth, err)
	}
	for _, want := range []string{"BootNotification Lib", "Heartbeat", "StatusNotification Available", "Authorize TAG-1"} {
		expectGot(t, h.got, want)
	}

	status := make(chan string, 1)
	if err := cs.Reset("OURS16", func(r *core.ResetConfirmation, err error) {
		if err != nil {
			status <- err.Error()
			return
		}
		status <- string(r.Status)
	}, core.ResetTypeSoft); err != nil {
		t.Fatal(err)
	}
	expectGot(t, got, "Reset Soft")
	expectGot(t, status, "Accepted")
}

// csms201 is ocpp-go's 2.0.1 CSMS handler.
type csms201 struct{ got chan string }

func (h *csms201) OnBootNotification(id string, r *provisioning.BootNotificationRequest) (*provisioning.BootNotificationResponse, error) {
	h.got <- "BootNotification " + r.ChargingStation.Model
	return provisioning.NewBootNotificationResponse(types2.NewDateTime(time.Now()), 300, provisioning.RegistrationStatusAccepted), nil
}
func (h *csms201) OnNotifyReport(string, *provisioning.NotifyReportRequest) (*provisioning.NotifyReportResponse, error) {
	return provisioning.NewNotifyReportResponse(), nil
}
func (h *csms201) OnHeartbeat(string, *availability.HeartbeatRequest) (*availability.HeartbeatResponse, error) {
	h.got <- "Heartbeat"
	return availability.NewHeartbeatResponse(*types2.NewDateTime(time.Now())), nil
}
func (h *csms201) OnStatusNotification(id string, r *availability.StatusNotificationRequest) (*availability.StatusNotificationResponse, error) {
	h.got <- "StatusNotification " + string(r.ConnectorStatus)
	return availability.NewStatusNotificationResponse(), nil
}

// This module's chargepoint (2.0.1) against ocpp-go's 2.0.1 CSMS.
func TestChargepointAgainstOCPPGo201(t *testing.T) {
	port := freePort(t)
	h := &csms201{got: make(chan string, 16)}
	cs := ocpp2.NewCSMS(nil, nil)
	cs.SetProvisioningHandler(h)
	cs.SetAvailabilityHandler(h)
	connected := make(chan string, 2)
	cs.SetNewChargingStationHandler(func(c ocpp2.ChargingStationConnection) { connected <- c.ID() })
	go cs.Start(port, "/ocpp/{ws}")
	defer cs.Stop()
	waitListening(t, port)

	got := make(chan string, 4)
	c := chargepoint.New(chargepoint.Config{URL: fmt.Sprintf("ws://127.0.0.1:%d/ocpp", port), ID: "OURS201", Versions: []string{ocpp.V21, ocpp.V201}, Timeout: 10 * time.Second})
	c.Handle("Reset", resetHandler(got))
	if err := c.Connect(t.Context()); err != nil {
		t.Fatal(err)
	}
	if c.Version() != ocpp.V201 {
		t.Fatalf("ocpp-go chose %q", c.Version())
	}
	if id := <-connected; id != "OURS201" {
		t.Fatalf("ocpp-go saw %q", id)
	}
	boot := map[string]any{"reason": "PowerUp", "chargingStation": map[string]any{"model": "Lib", "vendorName": "Sigma"}}
	if _, err := c.Call(t.Context(), "BootNotification", boot); err != nil {
		t.Fatalf("boot: %v", err)
	}
	if _, err := c.Call(t.Context(), "Heartbeat", nil); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	st := map[string]any{"timestamp": time.Now().UTC().Format(time.RFC3339), "connectorStatus": "Available", "evseId": 1, "connectorId": 1}
	if _, err := c.Call(t.Context(), "StatusNotification", st); err != nil {
		t.Fatalf("status: %v", err)
	}
	for _, want := range []string{"BootNotification Lib", "Heartbeat", "StatusNotification Available"} {
		expectGot(t, h.got, want)
	}
	status := make(chan string, 1)
	if err := cs.Reset("OURS201", func(r *provisioning.ResetResponse, err error) {
		if err != nil {
			status <- err.Error()
			return
		}
		status <- string(r.Status)
	}, provisioning.ResetTypeOnIdle); err != nil {
		t.Fatal(err)
	}
	expectGot(t, got, "Reset OnIdle")
	expectGot(t, status, "Accepted")
}
