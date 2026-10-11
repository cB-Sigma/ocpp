// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

// Package csms is the central system side of OCPP-J as a library role: an
// http.Handler that accepts charging stations of 1.6, 2.0.1 and 2.1 at
// <path>/<identity>, authenticates them (security profiles 1 to 3 through
// a hook), routes their CALLs and 2.1 SENDs to handlers by action, checks
// every request and every answer against the OCA schemas, keeps a
// registry of connected stations and calls them.
//
// It holds no charging business logic (authorization lists, tariffs,
// smart charging); handlers supply it.
package csms

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/cB-Sigma/ocpp"
)

// HandlerFunc answers a CALL (or takes a 2.1 SEND, whose answer is
// dropped; ocpp.IsSend(ctx) tells them apart) from station st. Return
// an *ocpp.RPCError to choose the CALLERROR, any other error for an
// InternalError.
type HandlerFunc func(ctx context.Context, st *Station, payload json.RawMessage) (any, error)

// Config sets up a Server. The zero value accepts 2.1, 2.0.1 and 1.6
// from anyone, which suits tests only: set Authenticate in production.
type Config struct {
	// Versions offered, in order of preference (default 2.1, 2.0.1, 1.6).
	// Variant spellings of each (ocpp.Subprotocols) are accepted too.
	Versions []string
	// NoSubprotocol is the version assumed for a station that offers no
	// subprotocol; empty refuses such a station.
	NoSubprotocol string
	// Authenticate admits a station before the WebSocket upgrade: check the
	// basic authentication password (profiles 1 and 2; ocpp.BasicAuthPassword,
	// ocpp.PasswordOK) or r.TLS (profile 3; ocpp.ClientCertNames). A
	// non-nil error refuses the station with HTTP 401. Nil admits everyone.
	Authenticate func(r *http.Request, identity string) error
	// Timeout of our CALLs and of each handler (default 30 s).
	Timeout time.Duration
	// Ping is the WebSocket ping interval (0: none).
	Ping time.Duration
	// SkipValidation turns off the schema checks of requests and answers
	// (for a station whose deviations the application handles itself).
	SkipValidation bool
	// OnInvalid, when set, decides about a station's request that fails
	// the schema check: return nil to hand it to the handler anyway (a
	// deviation the application tolerates; see ocpp.Deviations) or an
	// error to refuse it, normally verr. Unset, such requests are refused.
	OnInvalid func(st *Station, action string, payload json.RawMessage, verr *ocpp.RPCError) error
	// CheckSerial closes the connection of a 2.x station using a TLS client
	// certificate whose BootNotification serial number differs from the
	// certificate's common name (security profile 3).
	CheckSerial bool
	// OnConnect and OnDisconnect follow the registry.
	OnConnect    func(st *Station)
	OnDisconnect func(st *Station, err error)
	// AcceptOptions are passed to websocket.Accept (Subprotocols is set by
	// the Server); for example compression or origin checks.
	AcceptOptions websocket.AcceptOptions
}

// Server is a central system. Register handlers before serving.
type Server struct {
	cfg Config

	hmu      sync.RWMutex
	handlers map[string]HandlerFunc // "<version> <action>" or " <action>"

	mu       sync.Mutex
	stations map[string]*Station
}

// New makes a Server.
func New(cfg Config) *Server {
	if len(cfg.Versions) == 0 {
		cfg.Versions = []string{ocpp.V21, ocpp.V201, ocpp.V16}
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	return &Server{cfg: cfg, handlers: map[string]HandlerFunc{}, stations: map[string]*Station{}}
}

// Handle routes action, in every version, to h.
func (s *Server) Handle(action string, h HandlerFunc) { s.HandleVersion("", action, h) }

// HandleVersion routes action in version to h, ahead of a Handle for
// every version.
func (s *Server) HandleVersion(version, action string, h HandlerFunc) {
	s.hmu.Lock()
	defer s.hmu.Unlock()
	s.handlers[version+" "+action] = h
}

func (s *Server) handler(version, action string) HandlerFunc {
	s.hmu.RLock()
	defer s.hmu.RUnlock()
	if h := s.handlers[version+" "+action]; h != nil {
		return h
	}
	return s.handlers[" "+action]
}

// Station is a connected charging station, or nil.
func (s *Server) Station(identity string) *Station {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stations[identity]
}

// Stations lists the connected charging stations.
func (s *Server) Stations() []*Station {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Station, 0, len(s.stations))
	for _, st := range s.stations {
		out = append(out, st)
	}
	return out
}

// Identity is the charging station identity of a request to
// <path>/<identity>: the last path segment, unescaped.
func Identity(r *http.Request) (string, error) {
	p := r.URL.EscapedPath()
	seg := p[strings.LastIndex(p, "/")+1:]
	id, err := url.PathUnescape(seg)
	if err != nil {
		return "", err
	}
	return id, ocpp.CheckIdentity(id)
}

// ServeHTTP accepts one charging station and serves it until it leaves.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, err := Identity(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if s.cfg.Authenticate != nil {
		if err := s.cfg.Authenticate(r, id); err != nil {
			w.Header().Set("WWW-Authenticate", `Basic realm="OCPP"`)
			http.Error(w, "not authorized", http.StatusUnauthorized)
			return
		}
	}
	offered := ocpp.OfferedSubprotocols(r)
	if len(offered) == 0 && s.cfg.NoSubprotocol == "" {
		http.Error(w, "no OCPP subprotocol offered", http.StatusBadRequest)
		return
	}
	opts := s.cfg.AcceptOptions
	opts.Subprotocols = ocpp.Subprotocols(s.cfg.Versions)
	ws, err := websocket.Accept(w, r, &opts)
	if err != nil {
		return
	}
	version := ocpp.VersionOf(ws.Subprotocol())
	if ws.Subprotocol() == "" && len(offered) == 0 {
		version = s.cfg.NoSubprotocol
	}
	if version == "" {
		_ = ws.Close(websocket.StatusPolicyViolation, "no OCPP version in common")
		return
	}
	st := &Station{srv: s, id: id, version: version, tls: r.TLS, header: r.Header.Clone()}
	st.conn = ocpp.NewConn(ws, version, s.cfg.Timeout, st.handle)
	if version == ocpp.V21 && !s.cfg.SkipValidation {
		st.conn.CheckResult = func(action string, p json.RawMessage) error {
			if verr := ocpp.ValidateResponse(ocpp.V21, action, p); verr != nil {
				return &ocpp.SchemaError{Code: verr.Kind.Code(ocpp.V21), Description: verr.Desc, Schema: ocpp.V21 + " " + action + "Response"}
			}
			return nil
		}
	}
	s.mu.Lock()
	old := s.stations[id]
	s.stations[id] = st
	s.mu.Unlock()
	if old != nil {
		// A station that reconnects replaces its older connection.
		old.conn.Close("replaced by a newer connection")
	}
	if s.cfg.OnConnect != nil {
		s.cfg.OnConnect(st)
	}
	err = st.conn.Serve(r.Context(), s.cfg.Ping)
	s.mu.Lock()
	if s.stations[id] == st {
		delete(s.stations, id)
	}
	s.mu.Unlock()
	if s.cfg.OnDisconnect != nil {
		s.cfg.OnDisconnect(st, err)
	}
}

// Station is one connected charging station.
type Station struct {
	srv     *Server
	id      string
	version string
	tls     *tls.ConnectionState
	header  http.Header
	conn    *ocpp.Conn

	mu   sync.Mutex
	vals map[any]any
}

// ID is the station's identity.
func (st *Station) ID() string { return st.id }

// Version is the negotiated OCPP version.
func (st *Station) Version() string { return st.version }

// TLS is the TLS state of the connection (nil over plain WebSocket).
func (st *Station) TLS() *tls.ConnectionState { return st.tls }

// Header is the upgrade request's header.
func (st *Station) Header() http.Header { return st.header }

// Conn is the RPC connection.
func (st *Station) Conn() *ocpp.Conn { return st.conn }

// Done is closed when the station disconnects.
func (st *Station) Done() <-chan struct{} { return st.conn.Done() }

// Close disconnects the station.
func (st *Station) Close(reason string) { st.conn.Close(reason) }

// Set keeps an application value with the station.
func (st *Station) Set(key, value any) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.vals == nil {
		st.vals = map[any]any{}
	}
	st.vals[key] = value
}

// Get returns a value kept with Set.
func (st *Station) Get(key any) any {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.vals[key]
}

// Call sends a CALL and returns the station's answer. Unless validation is
// off, the request is checked before it is sent (a failure is returned as
// an *ocpp.RPCError and nothing is sent) and the answer after: an answer
// outside the schema is returned with an *ocpp.ResultError (on 2.1 the
// station is also told with a CALLRESULTERROR). A CALLERROR comes back as
// an *ocpp.CallError.
func (st *Station) Call(ctx context.Context, action string, req any) (json.RawMessage, error) {
	if req == nil {
		req = struct{}{}
	}
	validate := !st.srv.cfg.SkipValidation && ocpp.Known(st.version, action)
	if validate {
		b, err := json.Marshal(req)
		if err != nil {
			return nil, err
		}
		if verr := ocpp.ValidateRequest(st.version, action, b); verr != nil {
			return nil, verr
		}
		req = json.RawMessage(b)
	}
	raw, err := st.conn.Call(ctx, action, req)
	if err != nil {
		return raw, err
	}
	if validate && st.version != ocpp.V21 { // 2.1 is checked by the Conn
		if verr := ocpp.ValidateResponse(st.version, action, raw); verr != nil {
			return raw, &ocpp.ResultError{Code: verr.Kind.Code(st.version), Description: verr.Desc}
		}
	}
	return raw, nil
}

// CallInto is Call decoding the answer into out.
func (st *Station) CallInto(ctx context.Context, action string, req, out any) error {
	raw, err := st.Call(ctx, action, req)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// Send sends a 2.1 SEND message (never answered).
func (st *Station) Send(ctx context.Context, action string, payload any) error {
	if !st.srv.cfg.SkipValidation && ocpp.Known(st.version, action) {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if verr := ocpp.ValidateRequest(st.version, action, b); verr != nil {
			return verr
		}
		payload = json.RawMessage(b)
	}
	return st.conn.Send(ctx, action, payload)
}

// errSerial is the reason a profile 3 station is disconnected.
var errSerial = errors.New("the serial number differs from the certificate's common name")

func (st *Station) handle(ctx context.Context, action string, payload json.RawMessage) (any, error) {
	cfg := st.srv.cfg
	if !cfg.SkipValidation {
		if verr := ocpp.ValidateRequest(st.version, action, payload); verr != nil {
			if cfg.OnInvalid == nil {
				return nil, verr
			}
			if err := cfg.OnInvalid(st, action, payload, verr); err != nil {
				return nil, err
			}
		}
	}
	if cfg.CheckSerial && action == "BootNotification" && ocpp.Is2(st.version) && st.tls != nil && len(st.tls.PeerCertificates) > 0 {
		var b struct {
			ChargingStation struct {
				SerialNumber string `json:"serialNumber"`
			} `json:"chargingStation"`
		}
		_ = json.Unmarshal(payload, &b)
		if ocpp.SerialMismatch(b.ChargingStation.SerialNumber, st.tls.PeerCertificates[0].Subject.CommonName) {
			go st.conn.Close(errSerial.Error())
			return nil, ocpp.RPCErrorf(ocpp.KindSecurity, "%v", errSerial)
		}
	}
	h := st.srv.handler(st.version, action)
	if h == nil {
		if ocpp.Known(st.version, action) {
			return nil, ocpp.RPCErrorf(ocpp.KindNotSupported, "%s is not supported by this central system", action)
		}
		return nil, ocpp.RPCErrorf(ocpp.KindNotImplemented, "%s is not known", action)
	}
	res, err := h(ctx, st, payload)
	if err != nil || ocpp.IsSend(ctx) {
		return res, err
	}
	if cfg.SkipValidation || !ocpp.Known(st.version, action) {
		return res, nil
	}
	if ocpp.Is2(st.version) {
		return ocpp.CheckAnswer(st.version, action, res, payload)
	}
	if res == nil {
		res = struct{}{}
	}
	b, merr := json.Marshal(res)
	if merr != nil {
		return nil, ocpp.RPCErrorf(ocpp.KindInternal, "the answer cannot be encoded")
	}
	if verr := ocpp.ValidateResponse(st.version, action, b); verr != nil {
		return nil, ocpp.RPCErrorf(ocpp.KindInternal, "our %s answer does not match the %s schema: %s", action, st.version, verr.Desc)
	}
	return json.RawMessage(b), nil
}

// String names the station for logs.
func (st *Station) String() string { return fmt.Sprintf("%s (%s)", st.id, st.version) }
