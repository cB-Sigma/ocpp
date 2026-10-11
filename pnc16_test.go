// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package ocpp

import (
	"encoding/json"
	"testing"
)

func TestPnC16WrapUnwrap(t *testing.T) {
	dt, err := PnC16Wrap("SignCertificate", map[string]any{"csr": "-----BEGIN CERTIFICATE REQUEST-----"})
	if err != nil {
		t.Fatal(err)
	}
	if dt.VendorID != "org.openchargealliance.iso15118pnc" || dt.MessageID != "SignCertificate" || dt.Data != `{"csr":"-----BEGIN CERTIFICATE REQUEST-----"}` {
		t.Fatalf("%+v", dt)
	}
	b, _ := json.Marshal(dt)
	if verr := ValidateRequest(V16, "DataTransfer", b); verr != nil {
		t.Fatalf("the DataTransfer is not valid: %v", verr)
	}
	name, p, ok, verr := PnC16Unwrap(b)
	if !ok || verr != nil || name != "SignCertificate" || string(p) != dt.Data {
		t.Fatalf("unwrap: %s %s %v %v", name, p, ok, verr)
	}
	// The note's IdToken has no type, unlike 2.0.1's.
	if _, err := PnC16Wrap("Authorize", map[string]any{"idToken": map[string]any{"idToken": "NL-ABC-C12345678-X"}}); err != nil {
		t.Errorf("Authorize with an idToken alone: %v", err)
	}
	// TriggerMessage has no fields.
	if _, err := PnC16Wrap("TriggerMessage", map[string]any{}); err != nil {
		t.Errorf("an empty TriggerMessage: %v", err)
	}
	if _, err := PnC16Wrap("SignCertificate", map[string]any{}); err == nil {
		t.Error("SignCertificate without csr was wrapped")
	}
	if _, err := PnC16Wrap("Reset", map[string]any{}); err == nil {
		t.Error("a message outside the note was wrapped")
	}
	if _, _, ok, _ := PnC16Unwrap(json.RawMessage(`{"vendorId":"com.example","messageId":"x","data":"y"}`)); ok {
		t.Error("another vendor's DataTransfer was taken as Plug & Charge")
	}
	_, _, ok, verr = PnC16Unwrap(json.RawMessage(`{"vendorId":"org.openchargealliance.iso15118pnc","messageId":"InstallCertificate","data":"{\"certificateType\":\"CSMSRootCertificate\",\"certificate\":\"x\"}"}`))
	if !ok || verr == nil || verr.Kind != KindProperty {
		t.Errorf("a certificate type outside the note: %v %v", ok, verr)
	}
}

func TestPnC16Answers(t *testing.T) {
	c, err := PnC16Answer("Authorize", map[string]any{"idTokenInfo": map[string]any{"status": "Accepted"}, "certificateStatus": "CertificateRevoked"})
	if err != nil || c.Status != "Accepted" {
		t.Fatalf("%+v %v", c, err)
	}
	b, _ := json.Marshal(c)
	if verr := ValidateResponse(V16, "DataTransfer", b); verr != nil {
		t.Fatalf("the confirmation is not valid: %v", verr)
	}
	ans, st, ok, verr := PnC16ReadAnswer("Authorize", b)
	if !ok || verr != nil || st != "Accepted" || len(ans) == 0 {
		t.Fatalf("%s %s %v %v", ans, st, ok, verr)
	}
	if _, err := PnC16Answer("DeleteCertificate", map[string]any{"status": "Rejected"}); err == nil {
		t.Error("a DeleteCertificate status outside the note was taken")
	}
	if _, err := PnC16Answer("TriggerMessage", map[string]any{"status": "NotImplemented"}); err != nil {
		t.Error(err)
	}
	_, st, ok, _ = PnC16ReadAnswer("SignCertificate", json.RawMessage(`{"status":"UnknownVendorId"}`))
	if ok || st != "UnknownVendorId" {
		t.Errorf("a central system without the note: %s %v", st, ok)
	}
	for name, d := range PnC16 {
		if d.Req == nil || d.Conf == nil {
			t.Errorf("%s has no types", name)
		}
	}
}
