// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

// Package interop runs charge points written by others against a central
// system built from this module alone (ocpp.Conn, DecodeIn, ValidateJSON,
// CheckAnswer): mobilityhouse/ocpp's (Python, peer/cp.py, in a container;
// skipped unless SIGMA_OCPP_INTEROP=1 and Docker answers) and
// lorenzodonini/ocpp-go's (in process).
package interop

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/cB-Sigma/ocpp"
)

// received is one CALL the central system got, with the CALLERROR it
// answered (nil when it accepted the CALL).
type received struct {
	id, action string
	payload    json.RawMessage
	err        *ocpp.RPCError
}

// central is an OCPP-J central system for 1.6, 2.0.1 and 2.1 on this
// module's RPC layer: it checks every request against the OCA schemas
// (1.6 and 2.0.1 also through the Go types) and its own answers too.
type central struct {
	mu     sync.Mutex
	conns  map[string]*ocpp.Conn
	nextTx int
	got    chan received
	conned chan string
}

func newCentral() *central {
	return &central{conns: map[string]*ocpp.Conn{}, nextTx: 100, got: make(chan received, 64), conned: make(chan string, 4)}
}

func (s *central) conn(id string) *ocpp.Conn {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conns[id]
}

func (s *central) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: ocpp.Subprotocols([]string{ocpp.V21, ocpp.V201, ocpp.V16})})
	if err != nil {
		return
	}
	version := ocpp.VersionOf(ws.Subprotocol())
	if version == "" {
		_ = ws.Close(websocket.StatusPolicyViolation, "no OCPP subprotocol")
		return
	}
	c := ocpp.NewConn(ws, version, 10*time.Second, nil)
	c.SetHandler(func(ctx context.Context, action string, payload json.RawMessage) (any, error) {
		return s.handle(id, version, action, payload)
	})
	s.mu.Lock()
	s.conns[id] = c
	s.mu.Unlock()
	s.conned <- id
	_ = c.Serve(r.Context(), 0)
}

func (s *central) handle(id, version, action string, payload json.RawMessage) (any, error) {
	var verr *ocpp.RPCError
	switch version {
	case ocpp.V16:
		if md, ok := ocpp.V16In[action]; ok {
			_, verr = ocpp.DecodeIn(version, action, md, payload)
		} else {
			verr = ocpp.RPCErrorf(ocpp.KindNotImplemented, "%s is not handled", action)
		}
	case ocpp.V201:
		if md, ok := ocpp.V201In[action]; ok {
			_, verr = ocpp.DecodeIn(version, action, md, payload)
		} else {
			verr = ocpp.RPCErrorf(ocpp.KindNotImplemented, "%s is not handled", action)
		}
	default:
		_, verr = ocpp.ValidateJSON(version, action, false, payload)
	}
	s.got <- received{id: id, action: action, payload: payload, err: verr}
	if verr != nil {
		return nil, verr
	}
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
	return ocpp.CheckAnswer(version, action, res, payload)
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
