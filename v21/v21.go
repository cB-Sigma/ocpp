// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

// Package v21 holds Go types for every OCPP 2.1 message, generated from
// the OCA's 2.1 JSON schemas that the ocpp module embeds: one struct per
// request and response (<Action>Request, <Action>Response; the SEND
// message NotifyPeriodicEventStream as NotifyPeriodicEventStreamRequest),
// a named string type with constants per enumeration, and one struct per
// shared object. Optional properties are pointers or omitted slices;
// objects the schemas leave open (CustomData) keep unknown properties in
// Extra.
//
// The schemas stay the arbiter of validity: Decode and Call check
// payloads against them, so the types never have to.
package v21

//go:generate go run ../internal/cmd/gen21 messages.go

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"sort"

	"github.com/cB-Sigma/ocpp"
)

// Message is the Go types of one 2.1 action. A SEND action has no
// response.
type Message struct {
	Request, Response reflect.Type
	Send              bool
}

// Payload is a generated request or response.
type Payload interface {
	ActionName() string
	IsResponse() bool
}

// Caller sends a CALL and returns the answer: *csms.Station and
// *chargepoint.Client are Callers.
type Caller interface {
	Call(ctx context.Context, action string, req any) (json.RawMessage, error)
}

// Call sends req through c and decodes the answer into a Res, checked
// against the schema of the action's response.
func Call[Res Payload](ctx context.Context, c Caller, req Payload) (Res, error) {
	var res Res
	raw, err := c.Call(ctx, req.ActionName(), req)
	if err != nil {
		return res, err
	}
	return Decode[Res](raw)
}

// Decode checks raw against the schema of T's action (request or
// response) and decodes it; a failure is an *ocpp.RPCError carrying the
// CALLERROR kind.
func Decode[T Payload](raw json.RawMessage) (T, error) {
	var v T
	var verr *ocpp.RPCError
	if v.IsResponse() {
		verr = ocpp.ValidateResponse(ocpp.V21, v.ActionName(), raw)
	} else {
		verr = ocpp.ValidateRequest(ocpp.V21, v.ActionName(), raw)
	}
	if verr != nil {
		return v, verr
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, ocpp.RPCErrorf(ocpp.KindType, "%v", err)
	}
	return v, nil
}

// marshalOpen writes v's properties and then extra's, in name order.
func marshalOpen(v any, extra map[string]json.RawMessage) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil || len(extra) == 0 {
		return b, err
	}
	keys := make([]string, 0, len(extra))
	for k := range extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buf bytes.Buffer
	buf.Write(b[:len(b)-1])
	first := len(b) == 2
	for _, k := range keys {
		if !first {
			buf.WriteByte(',')
		}
		first = false
		kb, _ := json.Marshal(k)
		buf.Write(kb)
		buf.WriteByte(':')
		buf.Write(extra[k])
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// unmarshalOpen returns the properties of b not named in known.
func unmarshalOpen(b []byte, known ...string) (map[string]json.RawMessage, error) {
	var all map[string]json.RawMessage
	if err := json.Unmarshal(b, &all); err != nil {
		return nil, err
	}
	for _, k := range known {
		delete(all, k)
	}
	if len(all) == 0 {
		return nil, nil
	}
	return all, nil
}
