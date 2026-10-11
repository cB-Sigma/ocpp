// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package interop

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	ocpp16 "github.com/lorenzodonini/ocpp-go/ocpp1.6"
	"github.com/lorenzodonini/ocpp-go/ocpp1.6/core"
	types16 "github.com/lorenzodonini/ocpp-go/ocpp1.6/types"
	ocpp2 "github.com/lorenzodonini/ocpp-go/ocpp2.0.1"
	"github.com/lorenzodonini/ocpp-go/ocpp2.0.1/availability"
	"github.com/lorenzodonini/ocpp-go/ocpp2.0.1/provisioning"
	"github.com/lorenzodonini/ocpp-go/ocpp2.0.1/remotecontrol"
	"github.com/lorenzodonini/ocpp-go/ocpp2.0.1/transactions"
	types2 "github.com/lorenzodonini/ocpp-go/ocpp2.0.1/types"

	"github.com/cB-Sigma/ocpp"
)

var errUnused = errors.New("not used by the test")

// cp16 answers the central system's 1.6 core CALLs and reports them.
type cp16 struct{ got chan string }

func (h *cp16) OnChangeAvailability(*core.ChangeAvailabilityRequest) (*core.ChangeAvailabilityConfirmation, error) {
	return nil, errUnused
}
func (h *cp16) OnChangeConfiguration(*core.ChangeConfigurationRequest) (*core.ChangeConfigurationConfirmation, error) {
	return core.NewChangeConfigurationConfirmation(core.ConfigurationStatusAccepted), nil
}
func (h *cp16) OnClearCache(*core.ClearCacheRequest) (*core.ClearCacheConfirmation, error) {
	return core.NewClearCacheConfirmation(core.ClearCacheStatusAccepted), nil
}
func (h *cp16) OnDataTransfer(*core.DataTransferRequest) (*core.DataTransferConfirmation, error) {
	return nil, errUnused
}
func (h *cp16) OnGetConfiguration(r *core.GetConfigurationRequest) (*core.GetConfigurationConfirmation, error) {
	h.got <- fmt.Sprintf("GetConfiguration %v", r.Key)
	v := "300"
	res := core.NewGetConfigurationConfirmation([]core.ConfigurationKey{{Key: "HeartbeatInterval", Value: &v}})
	res.UnknownKey = []string{"NoSuchKey"}
	return res, nil
}
func (h *cp16) OnRemoteStartTransaction(r *core.RemoteStartTransactionRequest) (*core.RemoteStartTransactionConfirmation, error) {
	h.got <- "RemoteStartTransaction " + r.IdTag
	return core.NewRemoteStartTransactionConfirmation(types16.RemoteStartStopStatusAccepted), nil
}
func (h *cp16) OnRemoteStopTransaction(r *core.RemoteStopTransactionRequest) (*core.RemoteStopTransactionConfirmation, error) {
	h.got <- fmt.Sprintf("RemoteStopTransaction %d", r.TransactionId)
	return core.NewRemoteStopTransactionConfirmation(types16.RemoteStartStopStatusAccepted), nil
}
func (h *cp16) OnReset(r *core.ResetRequest) (*core.ResetConfirmation, error) {
	h.got <- "Reset " + string(r.Type)
	return core.NewResetConfirmation(core.ResetStatusAccepted), nil
}
func (h *cp16) OnUnlockConnector(*core.UnlockConnectorRequest) (*core.UnlockConnectorConfirmation, error) {
	return nil, errUnused
}

func expectGot(t *testing.T, ch chan string, want string) {
	t.Helper()
	select {
	case g := <-ch:
		if g != want {
			t.Errorf("the charge point got %q, want %q", g, want)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("the charge point got nothing, want %q", want)
	}
}

// ocpp-go's 1.6 charge point against a central system on this module.
func TestOCPPGoChargePoint16(t *testing.T) {
	s := newCentral()
	port := serve(t, s)
	h := &cp16{got: make(chan string, 8)}
	cp := ocpp16.NewChargePoint("GO16", nil, nil)
	cp.SetCoreHandler(h)
	if err := cp.Start(fmt.Sprintf("ws://127.0.0.1:%d/ocpp", port)); err != nil {
		t.Fatal(err)
	}
	defer cp.Stop()
	<-s.conned

	if r, err := cp.BootNotification("GoModel", "GoVendor"); err != nil || r.Status != core.RegistrationStatusAccepted || r.Interval != 300 {
		t.Fatalf("boot: %+v %v", r, err)
	}
	if _, err := cp.Heartbeat(); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if _, err := cp.StatusNotification(1, core.NoError, core.ChargePointStatusAvailable); err != nil {
		t.Fatalf("status: %v", err)
	}
	if r, err := cp.Authorize("TAG-1"); err != nil || r.IdTagInfo.Status != types16.AuthorizationStatusAccepted {
		t.Fatalf("authorize: %+v %v", r, err)
	}
	st, err := cp.StartTransaction(1, "TAG-1", 1000, types16.NewDateTime(time.Now()))
	if err != nil || st.TransactionId != 101 {
		t.Fatalf("start: %+v %v", st, err)
	}
	mv := []types16.MeterValue{{Timestamp: types16.NewDateTime(time.Now()), SampledValue: []types16.SampledValue{
		{Value: "1500", Measurand: types16.MeasurandEnergyActiveImportRegister, Unit: types16.UnitOfMeasureWh}}}}
	if _, err := cp.MeterValues(1, mv, func(r *core.MeterValuesRequest) { r.TransactionId = &st.TransactionId }); err != nil {
		t.Fatalf("meter values: %v", err)
	}
	if _, err := cp.StopTransaction(2000, types16.NewDateTime(time.Now()), st.TransactionId, func(r *core.StopTransactionRequest) { r.Reason = core.ReasonLocal }); err != nil {
		t.Fatalf("stop: %v", err)
	}
	for _, want := range []string{"BootNotification", "Heartbeat", "StatusNotification", "Authorize", "StartTransaction", "MeterValues", "StopTransaction"} {
		if r := s.next(t); r.action != want || r.err != nil {
			t.Errorf("central system got %s (%v), want %s accepted", r.action, r.err, want)
		}
	}

	c := s.conn("GO16")
	if m := call(t, c, "RemoteStartTransaction", map[string]any{"connectorId": 1, "idTag": "TAG-2"}); m["status"] != "Accepted" {
		t.Errorf("RemoteStartTransaction: %v", m)
	}
	expectGot(t, h.got, "RemoteStartTransaction TAG-2")
	if m := call(t, c, "RemoteStopTransaction", map[string]any{"transactionId": 101}); m["status"] != "Accepted" {
		t.Errorf("RemoteStopTransaction: %v", m)
	}
	expectGot(t, h.got, "RemoteStopTransaction 101")
	if m := call(t, c, "Reset", map[string]any{"type": "Soft"}); m["status"] != "Accepted" {
		t.Errorf("Reset: %v", m)
	}
	expectGot(t, h.got, "Reset Soft")
	m := call(t, c, "GetConfiguration", map[string]any{"key": []string{"HeartbeatInterval", "NoSuchKey"}})
	expectGot(t, h.got, "GetConfiguration [HeartbeatInterval NoSuchKey]")
	if keys, _ := m["configurationKey"].([]any); len(keys) != 1 || keys[0].(map[string]any)["value"] != "300" {
		t.Errorf("GetConfiguration: %v", m)
	}

	// A Reset outside the enumeration: ocpp-go refuses it with a CALLERROR.
	_, err = c.Call(t.Context(), "Reset", map[string]any{"type": "Sometimes"})
	var ce *ocpp.CallError
	if !errors.As(err, &ce) {
		t.Fatalf("a Reset outside the enumeration: %v, want a CALLERROR", err)
	}
	t.Logf("ocpp-go answered the invalid Reset with %s (%s)", ce.Code, ce.Description)
	if m := call(t, c, "ClearCache", map[string]any{}); m["status"] != "Accepted" {
		t.Errorf("ClearCache after the CALLERROR: %v", m)
	}
}

// cs201 answers the CSMS's 2.0.1 provisioning and remote control CALLs.
type cs201 struct{ got chan string }

func (h *cs201) OnGetBaseReport(*provisioning.GetBaseReportRequest) (*provisioning.GetBaseReportResponse, error) {
	return nil, errUnused
}
func (h *cs201) OnGetReport(*provisioning.GetReportRequest) (*provisioning.GetReportResponse, error) {
	return nil, errUnused
}
func (h *cs201) OnGetVariables(r *provisioning.GetVariablesRequest) (*provisioning.GetVariablesResponse, error) {
	h.got <- "GetVariables " + r.GetVariableData[0].Variable.Name
	return provisioning.NewGetVariablesResponse([]provisioning.GetVariableResult{{AttributeStatus: provisioning.GetVariableStatusAccepted,
		AttributeValue: "120", Component: r.GetVariableData[0].Component, Variable: r.GetVariableData[0].Variable}}), nil
}
func (h *cs201) OnReset(r *provisioning.ResetRequest) (*provisioning.ResetResponse, error) {
	h.got <- "Reset " + string(r.Type)
	return provisioning.NewResetResponse(provisioning.ResetStatusAccepted), nil
}
func (h *cs201) OnSetNetworkProfile(*provisioning.SetNetworkProfileRequest) (*provisioning.SetNetworkProfileResponse, error) {
	return nil, errUnused
}
func (h *cs201) OnSetVariables(r *provisioning.SetVariablesRequest) (*provisioning.SetVariablesResponse, error) {
	h.got <- "SetVariables " + r.SetVariableData[0].AttributeValue
	return provisioning.NewSetVariablesResponse([]provisioning.SetVariableResult{{AttributeStatus: provisioning.SetVariableStatusAccepted,
		Component: r.SetVariableData[0].Component, Variable: r.SetVariableData[0].Variable}}), nil
}
func (h *cs201) OnRequestStartTransaction(r *remotecontrol.RequestStartTransactionRequest) (*remotecontrol.RequestStartTransactionResponse, error) {
	h.got <- fmt.Sprintf("RequestStartTransaction %s %d", r.IDToken.IdToken, r.RemoteStartID)
	return remotecontrol.NewRequestStartTransactionResponse(remotecontrol.RequestStartStopStatusAccepted), nil
}
func (h *cs201) OnRequestStopTransaction(r *remotecontrol.RequestStopTransactionRequest) (*remotecontrol.RequestStopTransactionResponse, error) {
	h.got <- "RequestStopTransaction " + r.TransactionID
	return remotecontrol.NewRequestStopTransactionResponse(remotecontrol.RequestStartStopStatusAccepted), nil
}
func (h *cs201) OnTriggerMessage(*remotecontrol.TriggerMessageRequest) (*remotecontrol.TriggerMessageResponse, error) {
	return nil, errUnused
}
func (h *cs201) OnUnlockConnector(*remotecontrol.UnlockConnectorRequest) (*remotecontrol.UnlockConnectorResponse, error) {
	return nil, errUnused
}

// ocpp-go's 2.0.1 charging station against a central system on this
// module (ocpp-go has no 2.1).
func TestOCPPGoChargingStation201(t *testing.T) {
	s := newCentral()
	port := serve(t, s)
	h := &cs201{got: make(chan string, 8)}
	cs := ocpp2.NewChargingStation("GO201", nil, nil)
	cs.SetProvisioningHandler(h)
	cs.SetRemoteControlHandler(h)
	if err := cs.Start(fmt.Sprintf("ws://127.0.0.1:%d/ocpp", port)); err != nil {
		t.Fatal(err)
	}
	defer cs.Stop()
	<-s.conned

	if r, err := cs.BootNotification(provisioning.BootReasonPowerUp, "GoModel", "GoVendor"); err != nil || r.Status != provisioning.RegistrationStatusAccepted {
		t.Fatalf("boot: %+v %v", r, err)
	}
	if _, err := cs.Heartbeat(); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if _, err := cs.StatusNotification(types2.NewDateTime(time.Now()), availability.ConnectorStatusAvailable, 1, 1); err != nil {
		t.Fatalf("status: %v", err)
	}
	if r, err := cs.Authorize("TAG-1", types2.IdTokenTypeISO14443); err != nil || r.IdTokenInfo.Status != types2.AuthorizationStatusAccepted {
		t.Fatalf("authorize: %+v %v", r, err)
	}
	tok := &types2.IdToken{IdToken: "TAG-1", Type: types2.IdTokenTypeISO14443}
	r, err := cs.TransactionEvent(transactions.TransactionEventStarted, types2.NewDateTime(time.Now()), transactions.TriggerReasonAuthorized, 0,
		transactions.Transaction{TransactionID: "GO-TX-1"}, func(r *transactions.TransactionEventRequest) {
			r.IDToken = tok
			r.Evse = &types2.EVSE{ID: 1}
		})
	if err != nil || r.IDTokenInfo == nil || r.IDTokenInfo.Status != types2.AuthorizationStatusAccepted {
		t.Fatalf("started: %+v %v", r, err)
	}
	mv := []types2.MeterValue{{Timestamp: *types2.NewDateTime(time.Now()), SampledValue: []types2.SampledValue{
		{Value: 1500, Measurand: types2.MeasurandEnergyActiveImportRegister}}}}
	if _, err := cs.TransactionEvent(transactions.TransactionEventUpdated, types2.NewDateTime(time.Now()), transactions.TriggerReasonMeterValuePeriodic, 1,
		transactions.Transaction{TransactionID: "GO-TX-1", ChargingState: transactions.ChargingStateCharging},
		func(r *transactions.TransactionEventRequest) { r.MeterValue = mv }); err != nil {
		t.Fatalf("updated: %v", err)
	}
	if _, err := cs.TransactionEvent(transactions.TransactionEventEnded, types2.NewDateTime(time.Now()), transactions.TriggerReasonStopAuthorized, 2,
		transactions.Transaction{TransactionID: "GO-TX-1", StoppedReason: transactions.ReasonLocal}); err != nil {
		t.Fatalf("ended: %v", err)
	}
	for _, want := range []string{"BootNotification", "Heartbeat", "StatusNotification", "Authorize", "TransactionEvent", "TransactionEvent", "TransactionEvent"} {
		if r := s.next(t); r.action != want || r.err != nil {
			t.Errorf("central system got %s (%v), want %s accepted", r.action, r.err, want)
		}
	}

	c := s.conn("GO201")
	if m := call(t, c, "RequestStartTransaction", map[string]any{"evseId": 1, "remoteStartId": 7,
		"idToken": map[string]any{"idToken": "TAG-2", "type": "ISO14443"}}); m["status"] != "Accepted" {
		t.Errorf("RequestStartTransaction: %v", m)
	}
	expectGot(t, h.got, "RequestStartTransaction TAG-2 7")
	if m := call(t, c, "RequestStopTransaction", map[string]any{"transactionId": "GO-TX-2"}); m["status"] != "Accepted" {
		t.Errorf("RequestStopTransaction: %v", m)
	}
	expectGot(t, h.got, "RequestStopTransaction GO-TX-2")
	// ocpp-go answers Reset with "statusInfo": null, outside the schema
	// (statusInfo is an optional object); Deviations reads it as absent.
	raw, err := c.Call(t.Context(), "Reset", map[string]any{"type": "OnIdle"})
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	expectGot(t, h.got, "Reset OnIdle")
	_, verr := ocpp.ValidateJSON(ocpp.V201, "Reset", true, raw)
	if verr == nil {
		t.Logf("ocpp-go's Reset answer passed the schema: %s", raw)
	} else if ok, _, _ := ocpp.Deviations(ocpp.V201, "Reset", true, raw, verr); !ok || !strings.Contains(string(raw), `"statusInfo":null`) {
		t.Errorf("Reset answer %s: %v (tolerable: %v)", raw, verr, ok)
	}
	cv := map[string]any{"component": map[string]any{"name": "OCPPCommCtrlr"}, "variable": map[string]any{"name": "HeartbeatInterval"}}
	call(t, c, "SetVariables", map[string]any{"setVariableData": []any{map[string]any{"component": cv["component"], "variable": cv["variable"], "attributeValue": "120"}}})
	expectGot(t, h.got, "SetVariables 120")
	m := call(t, c, "GetVariables", map[string]any{"getVariableData": []any{cv}})
	expectGot(t, h.got, "GetVariables HeartbeatInterval")
	if res, _ := m["getVariableResult"].([]any); len(res) != 1 || res[0].(map[string]any)["attributeValue"] != "120" {
		t.Errorf("GetVariables: %v", m)
	}

	_, err = c.Call(t.Context(), "Reset", map[string]any{"type": "Sometimes"})
	var ce *ocpp.CallError
	if !errors.As(err, &ce) {
		t.Fatalf("a Reset outside the enumeration: %v, want a CALLERROR", err)
	}
	t.Logf("ocpp-go answered the invalid Reset with %s (%s)", ce.Code, ce.Description)
}
