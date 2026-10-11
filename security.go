// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package ocpp

import (
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
)

// Security profiles (OCPP 1.6 security whitepaper; OCPP 2.0.1 and 2.1
// part 2 section A). Profiles 1 and 2 authenticate a charging station by
// HTTP basic authentication with its identity as the user name (profile 2
// over TLS); profile 3 by its TLS client certificate, whose common name is
// the identity, and for 2.x the serial number in BootNotification must be
// that common name too. These are the checks; the listener, the password
// store and certificate revocation belong to the application.

// CheckIdentity accepts a charging station identity: 1 to 48 characters,
// none of them '/', ':' (a basic authentication user name cannot hold it)
// or a control or space character.
func CheckIdentity(id string) error {
	if id == "" || len(id) > 48 {
		return fmt.Errorf("charger id %q must have 1 to 48 characters", id)
	}
	for _, r := range id {
		if r == '/' || r == ':' || r < 0x21 || r == 0x7f {
			return fmt.Errorf("charger id %q holds a character that cannot be used", id)
		}
	}
	return nil
}

// PasswordOK compares a basic authentication password with the stored
// one in constant time: want is the password itself or "sha256:" and the
// hex SHA-256 of it.
func PasswordOK(want, got string) bool {
	if h, ok := strings.CutPrefix(want, "sha256:"); ok {
		sum := sha256.Sum256([]byte(got))
		exp, _ := hex.DecodeString(h)
		return subtle.ConstantTimeCompare(sum[:], exp) == 1
	}
	return subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}

// BasicAuthPassword is the password of a profile 1 or 2 request from
// charging station id: ok is false when the request has no basic
// authentication or its user name is not id.
func BasicAuthPassword(r *http.Request, id string) (pass string, ok bool) {
	user, pass, ok := r.BasicAuth()
	if !ok || user != id {
		return "", false
	}
	return pass, true
}

// ClientCertNames reports whether the TLS client certificate of a
// profile 3 connection names charging station id as its common name.
func ClientCertNames(st *tls.ConnectionState, id string) bool {
	return st != nil && len(st.PeerCertificates) > 0 && st.PeerCertificates[0].Subject.CommonName == id
}

// SerialMismatch reports whether a 2.x BootNotification's serial number
// differs from the common name of the profile 3 client certificate (both
// known), in which case the connection is to be closed.
func SerialMismatch(serial, cn string) bool {
	return serial != "" && cn != "" && serial != cn
}
