// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package ocpp

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"net/http/httptest"
	"testing"
)

func TestSecurityProfileChecks(t *testing.T) {
	for id, ok := range map[string]bool{"CP-1": true, "": false, "a/b": false, "a:b": false, "a b": false, "123456789012345678901234567890123456789012345678": true, "1234567890123456789012345678901234567890123456789": false} {
		if (CheckIdentity(id) == nil) != ok {
			t.Errorf("CheckIdentity(%q)", id)
		}
	}
	sum := sha256.Sum256([]byte("secret-password-16"))
	if !PasswordOK("secret-password-16", "secret-password-16") || !PasswordOK("sha256:"+hex.EncodeToString(sum[:]), "secret-password-16") || PasswordOK("sha256:00", "secret-password-16") || PasswordOK("a", "b") {
		t.Error("PasswordOK")
	}
	r := httptest.NewRequest("GET", "/ocpp/CP1", nil)
	r.SetBasicAuth("CP1", "pw")
	if p, ok := BasicAuthPassword(r, "CP1"); !ok || p != "pw" {
		t.Error("BasicAuthPassword")
	}
	if _, ok := BasicAuthPassword(r, "CP2"); ok {
		t.Error("BasicAuthPassword took another identity")
	}
	st := &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{Subject: pkix.Name{CommonName: "CP1"}}}}
	if !ClientCertNames(st, "CP1") || ClientCertNames(st, "CP2") || ClientCertNames(nil, "CP1") || ClientCertNames(&tls.ConnectionState{}, "CP1") {
		t.Error("ClientCertNames")
	}
	if !SerialMismatch("SN1", "CP1") || SerialMismatch("", "CP1") || SerialMismatch("CP1", "CP1") || SerialMismatch("SN1", "") {
		t.Error("SerialMismatch")
	}
}
