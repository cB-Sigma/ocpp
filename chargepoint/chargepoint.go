// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

// Package chargepoint is the charging station side of OCPP-J as a library
// role: it dials a central system as <url>/<identity> with the versions it
// offers, authenticates (security profiles 1 to 3), routes the central
// system's CALLs to handlers by action, checks every request and answer
// against the OCA schemas and, run with Run, reconnects with the
// randomised, doubling back-off of the 2.x OCPPCommCtrlr variables.
//
// It holds no charger behaviour (connectors, transactions, meter values);
// handlers and the application's own calls supply it. ocpp-sim's
// simulated charge point is built on Dial.
package chargepoint

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/cB-Sigma/ocpp"
)

// HandlerFunc answers a CALL of the central system. Return an
// *ocpp.RPCError to choose the CALLERROR, any other error for an
// InternalError.
type HandlerFunc func(ctx context.Context, c *Client, payload json.RawMessage) (any, error)

// Backoff is the reconnection back-off, read the way the OCPP 2.x
// OCPPCommCtrlr variables RetryBackOffWaitMinimum, RetryBackOffRandomRange
// and RetryBackOffRepeatTimes are: the first wait is Minimum plus a random
// part of RandomRange, and each later wait doubles the previous one, for
// at most RepeatTimes doublings; then it stays there.
type Backoff struct {
	Minimum     time.Duration // default 1 s
	RandomRange time.Duration // default 1 s
	RepeatTimes int           // default 5
}

// Wait is the back-off before reconnection attempt n (0 for the first).
func (b Backoff) Wait(n int) time.Duration {
	if b.Minimum <= 0 {
		b.Minimum = time.Second
	}
	if b.RandomRange < 0 {
		b.RandomRange = 0
	}
	if b.RepeatTimes <= 0 {
		b.RepeatTimes = 5
	}
	d := b.Minimum
	if b.RandomRange > 0 {
		d += time.Duration(rand.Int64N(int64(b.RandomRange) + 1))
	}
	for i := 0; i < n && i < b.RepeatTimes; i++ {
		d *= 2
	}
	return d
}

// Config describes a charging station's connection.
type Config struct {
	// URL is the central system's endpoint without the identity, e.g.
	// wss://csms.example/ocpp; the identity is appended as a path segment.
	URL string
	// ID is the charging station identity (1 to 48 characters).
	ID string
	// Versions offered, in order of preference (default 2.1, 2.0.1, 1.6).
	Versions []string
	// Password for HTTP basic authentication (security profiles 1 and 2).
	Password string
	// TLS for wss (profiles 2 and 3; a client certificate for 3).
	TLS *tls.Config
	// Header holds further upgrade request headers.
	Header http.Header
	// Timeout of our CALLs and of each handler (default 30 s).
	Timeout time.Duration
	// Ping is the WebSocket ping interval (0: none).
	Ping time.Duration
	// SkipValidation turns off the schema checks.
	SkipValidation bool
	// OnInvalid, when set, decides about a central system request that
	// fails the schema check: nil hands it to the handler anyway, an error
	// (normally verr) refuses it. Unset, such requests are refused.
	OnInvalid func(c *Client, action string, payload json.RawMessage, verr *ocpp.RPCError) error
	// Backoff between reconnection attempts in Run.
	Backoff Backoff
	// OnConnect runs, on its own goroutine, after every connection (send
	// BootNotification there); OnDisconnect after it ends.
	OnConnect    func(ctx context.Context, c *Client)
	OnDisconnect func(c *Client, err error)
	// OnResultError is told about a 2.1 CALLRESULTERROR the central system
	// sent about one of our answers.
	OnResultError func(id, code, desc string)
}

// Dialled is an open WebSocket to a central system and the version it
// chose.
type Dialled struct {
	WS      *websocket.Conn
	Version string
}

// Dial opens the WebSocket of cfg: the identity appended to the URL, the
// versions offered, basic authentication, TLS. The version is the one
// the central system chose; a choice outside the offer is an error.
func Dial(ctx context.Context, cfg Config) (Dialled, error) {
	if cfg.ID == "" {
		return Dialled{}, errors.New("no charging station identity")
	}
	versions := cfg.Versions
	if len(versions) == 0 {
		versions = []string{ocpp.V21, ocpp.V201, ocpp.V16}
	}
	hdr := cfg.Header.Clone()
	if hdr == nil {
		hdr = http.Header{}
	}
	if cfg.Password != "" {
		hdr.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(cfg.ID+":"+cfg.Password)))
	}
	opts := &websocket.DialOptions{Subprotocols: versions, HTTPHeader: hdr}
	if cfg.TLS != nil {
		opts.HTTPClient = &http.Client{Transport: &http.Transport{TLSClientConfig: cfg.TLS}}
	}
	ws, resp, err := websocket.Dial(ctx, EndpointURL(cfg.URL, cfg.ID), opts)
	if err != nil {
		if resp != nil {
			return Dialled{}, fmt.Errorf("dial: HTTP %d: %w", resp.StatusCode, err)
		}
		return Dialled{}, err
	}
	if !slices.Contains(versions, ws.Subprotocol()) {
		_ = ws.CloseNow()
		return Dialled{}, fmt.Errorf("the central system chose subprotocol %q, not one offered", ws.Subprotocol())
	}
	return Dialled{WS: ws, Version: ws.Subprotocol()}, nil
}

// EndpointURL is base with the identity appended as an escaped path
// segment.
func EndpointURL(base, id string) string {
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	return base + url.PathEscape(id)
}

// Client is a charging station's connection to its central system.
type Client struct {
	cfg Config

	hmu      sync.RWMutex
	handlers map[string]HandlerFunc

	mu      sync.Mutex
	conn    *ocpp.Conn
	version string
	ready   chan struct{} // closed while connected
}

// New makes a Client; Connect or Run opens it.
func New(cfg Config) *Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	return &Client{cfg: cfg, handlers: map[string]HandlerFunc{}, ready: make(chan struct{})}
}

// ID is the charging station identity.
func (c *Client) ID() string { return c.cfg.ID }

// Handle routes the central system's action, in every version, to h.
func (c *Client) Handle(action string, h HandlerFunc) { c.HandleVersion("", action, h) }

// HandleVersion routes action in version to h, ahead of Handle.
func (c *Client) HandleVersion(version, action string, h HandlerFunc) {
	c.hmu.Lock()
	defer c.hmu.Unlock()
	c.handlers[version+" "+action] = h
}

func (c *Client) handler(version, action string) HandlerFunc {
	c.hmu.RLock()
	defer c.hmu.RUnlock()
	if h := c.handlers[version+" "+action]; h != nil {
		return h
	}
	return c.handlers[" "+action]
}

// ErrNotConnected is returned by Call while there is no connection.
var ErrNotConnected = errors.New("not connected to the central system")

// Version is the version of the current connection ("" when there is
// none).
func (c *Client) Version() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.version
}

// Conn is the current RPC connection, or nil.
func (c *Client) Conn() *ocpp.Conn {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn
}

// Connect dials once and serves the connection in the background until it
// ends or ctx is done.
func (c *Client) Connect(ctx context.Context) error {
	conn, err := c.open(ctx)
	if err != nil {
		return err
	}
	go c.serve(ctx, conn)
	return nil
}

// Run connects and reconnects with the back-off until ctx is done.
func (c *Client) Run(ctx context.Context) error {
	attempt := 0
	for {
		conn, err := c.open(ctx)
		if err == nil {
			attempt = 0
			c.serve(ctx, conn)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		t := time.NewTimer(c.cfg.Backoff.Wait(attempt))
		attempt++
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
}

// Close drops the current connection; Run then reconnects.
func (c *Client) Close(reason string) {
	if conn := c.Conn(); conn != nil {
		conn.Close(reason)
	}
}

// WaitConnected waits until a connection is open.
func (c *Client) WaitConnected(ctx context.Context) error {
	c.mu.Lock()
	ready := c.ready
	c.mu.Unlock()
	select {
	case <-ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) open(ctx context.Context) (*ocpp.Conn, error) {
	d, err := Dial(ctx, c.cfg)
	if err != nil {
		return nil, err
	}
	conn := ocpp.NewConn(d.WS, d.Version, c.cfg.Timeout, nil)
	version := d.Version
	conn.SetHandler(func(hctx context.Context, action string, p json.RawMessage) (any, error) {
		return c.handle(hctx, version, action, p)
	})
	conn.OnResultError = c.cfg.OnResultError
	if version == ocpp.V21 && !c.cfg.SkipValidation {
		conn.CheckResult = func(action string, p json.RawMessage) error {
			if verr := ocpp.ValidateResponse(ocpp.V21, action, p); verr != nil {
				return &ocpp.SchemaError{Code: verr.Kind.Code(ocpp.V21), Description: verr.Desc, Schema: ocpp.V21 + " " + action + "Response"}
			}
			return nil
		}
	}
	c.mu.Lock()
	c.conn, c.version = conn, version
	select {
	case <-c.ready:
	default:
		close(c.ready)
	}
	c.mu.Unlock()
	return conn, nil
}

func (c *Client) serve(ctx context.Context, conn *ocpp.Conn) {
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if c.cfg.OnConnect != nil {
		go c.cfg.OnConnect(sctx, c)
	}
	err := conn.Serve(sctx, c.cfg.Ping)
	c.mu.Lock()
	if c.conn == conn {
		c.conn, c.version = nil, ""
		c.ready = make(chan struct{})
	}
	c.mu.Unlock()
	if c.cfg.OnDisconnect != nil {
		c.cfg.OnDisconnect(c, err)
	}
}

func (c *Client) handle(ctx context.Context, version, action string, payload json.RawMessage) (any, error) {
	if !c.cfg.SkipValidation {
		if verr := ocpp.ValidateRequest(version, action, payload); verr != nil {
			if c.cfg.OnInvalid == nil {
				return nil, verr
			}
			if err := c.cfg.OnInvalid(c, action, payload, verr); err != nil {
				return nil, err
			}
		}
	}
	h := c.handler(version, action)
	if h == nil {
		if ocpp.Known(version, action) {
			return nil, ocpp.RPCErrorf(ocpp.KindNotSupported, "%s is not supported by this charging station", action)
		}
		return nil, ocpp.RPCErrorf(ocpp.KindNotImplemented, "%s is not known", action)
	}
	res, err := h(ctx, c, payload)
	if err != nil || c.cfg.SkipValidation || !ocpp.Known(version, action) {
		return res, err
	}
	if res == nil {
		res = struct{}{}
	}
	b, merr := json.Marshal(res)
	if merr != nil {
		return nil, ocpp.RPCErrorf(ocpp.KindInternal, "the answer cannot be encoded")
	}
	if verr := ocpp.ValidateResponse(version, action, b); verr != nil && !ocpp.EchoesOnly(version, action, b, payload) {
		return nil, ocpp.RPCErrorf(ocpp.KindInternal, "our %s answer does not match the %s schema: %s", action, version, verr.Desc)
	}
	return json.RawMessage(b), nil
}

// Call sends a CALL and returns the central system's answer, checked as
// csms.Station.Call checks it: a request outside the schema is returned
// as an *ocpp.RPCError unsent, an answer outside it with an
// *ocpp.ResultError, a CALLERROR as an *ocpp.CallError.
func (c *Client) Call(ctx context.Context, action string, req any) (json.RawMessage, error) {
	c.mu.Lock()
	conn, version := c.conn, c.version
	c.mu.Unlock()
	if conn == nil {
		return nil, ErrNotConnected
	}
	if req == nil {
		req = struct{}{}
	}
	validate := !c.cfg.SkipValidation && ocpp.Known(version, action)
	if validate {
		b, err := json.Marshal(req)
		if err != nil {
			return nil, err
		}
		if verr := ocpp.ValidateRequest(version, action, b); verr != nil {
			return nil, verr
		}
		req = json.RawMessage(b)
	}
	raw, err := conn.Call(ctx, action, req)
	if err != nil {
		return raw, err
	}
	if validate && version != ocpp.V21 {
		if verr := ocpp.ValidateResponse(version, action, raw); verr != nil {
			return raw, &ocpp.ResultError{Code: verr.Kind.Code(version), Description: verr.Desc}
		}
	}
	return raw, nil
}

// CallInto is Call decoding the answer into out.
func (c *Client) CallInto(ctx context.Context, action string, req, out any) error {
	raw, err := c.Call(ctx, action, req)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// Send sends a 2.1 SEND message (never answered), checked against its
// schema first.
func (c *Client) Send(ctx context.Context, action string, payload any) error {
	c.mu.Lock()
	conn, version := c.conn, c.version
	c.mu.Unlock()
	if conn == nil {
		return ErrNotConnected
	}
	if !c.cfg.SkipValidation && ocpp.Known(version, action) {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if verr := ocpp.ValidateRequest(version, action, b); verr != nil {
			return verr
		}
		payload = json.RawMessage(b)
	}
	return conn.Send(ctx, action, payload)
}
