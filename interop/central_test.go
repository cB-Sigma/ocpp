// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

// Package interop runs charge points and central systems written by others
// against this module's roles: mobilityhouse/ocpp's charge point (Python,
// peer/cp.py, in a container; skipped unless SIGMA_OCPP_INTEROP=1 and
// Docker answers), lorenzodonini/ocpp-go's charge point and central system
// and seanlee0923/ocpp's station and CSMS (both in process), against the
// csms and chargepoint packages.
package interop

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cB-Sigma/ocpp"
	"github.com/cB-Sigma/ocpp/csms"
)

// received is one CALL the central system got, with the CALLERROR it
// answered (nil when it accepted the CALL).
type received struct {
	id, action string
	payload    json.RawMessage
	err        *ocpp.RPCError
}

// central is a central system for 1.6, 2.0.1 and 2.1 built on the csms
// package: it checks every request against the OCA schemas (1.6 also
// through the Go types) and its own answers too, and records what it got.
type central struct {
	srv    *csms.Server
	mu     sync.Mutex
	nextTx int
	got    chan received
	conned chan string
}

func newCentral() *central {
	s := &central{nextTx: 100, got: make(chan received, 64), conned: make(chan string, 4)}
	s.srv = csms.New(csms.Config{
		Timeout:   10 * time.Second,
		OnConnect: func(st *csms.Station) { s.conned <- st.ID() },
		OnInvalid: func(st *csms.Station, action string, payload json.RawMessage, verr *ocpp.RPCError) error {
			s.got <- received{id: st.ID(), action: action, payload: payload, err: verr}
			return verr
		},
	})
	actions := map[string]bool{}
	for a := range ocpp.V16In {
		actions[a] = true
	}
	for a := range ocpp.V201In {
		actions[a] = true
	}
	for a := range ocpp.In21 {
		actions[a] = true
	}
	for a := range actions {
		s.srv.Handle(a, s.handle)
	}
	return s
}

func (s *central) conn(id string) *ocpp.Conn {
	if st := s.srv.Station(id); st != nil {
		return st.Conn()
	}
	return nil
}

func (s *central) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.srv.ServeHTTP(w, r) }

func (s *central) handle(ctx context.Context, st *csms.Station, payload json.RawMessage) (any, error) {
	action, version := ocpp.ActionOf(ctx), st.Version()
	s.got <- received{id: st.ID(), action: action, payload: payload}
	now := time.Now().UTC().Format(time.RFC3339)
	var res any = map[string]any{}
	switch action {
	case "BootNotification":
		res = map[string]any{"status": "Accepted", "currentTime": now, "interval": 300}
	case "Heartbeat":
		res = map[string]any{"currentTime": now}
	case "Authorize":
		if version == ocpp.V16 {
			res = map[string]any{"idTagInfo": map[string]any{"status": "Accepted"}}
		} else {
			res = map[string]any{"idTokenInfo": map[string]any{"status": "Accepted"}}
		}
	case "StartTransaction":
		s.mu.Lock()
		s.nextTx++
		tx := s.nextTx
		s.mu.Unlock()
		res = map[string]any{"transactionId": tx, "idTagInfo": map[string]any{"status": "Accepted"}}
	case "TransactionEvent":
		if strings.Contains(string(payload), `"idToken"`) {
			res = map[string]any{"idTokenInfo": map[string]any{"status": "Accepted"}}
		}
	case "DataTransfer":
		res = map[string]any{"status": "UnknownVendorId"}
	}
	return res, nil // csms checks it against the response schema
}

// next returns the next CALL the central system got.
func (s *central) next(t *testing.T) received {
	t.Helper()
	select {
	case r := <-s.got:
		return r
	case <-time.After(15 * time.Second):
		t.Fatal("the central system received nothing")
	}
	return received{}
}

// expectCall waits for a CALL of action, skipping others.
func (s *central) expectCall(t *testing.T, action string) received {
	t.Helper()
	deadline := time.After(15 * time.Second)
	for {
		select {
		case r := <-s.got:
			if r.action == action {
				return r
			}
		case <-deadline:
			t.Fatalf("no %s within 15s", action)
			return received{}
		}
	}
}
