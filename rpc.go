package ocpp

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// OCPP-J message type numbers (OCPP-J 1.6 clause 4.1.3; OCPP 2.0.1 part 4
// clause 4.1.3; OCPP 2.1 part 4 clause 4.1.3 adds CALLRESULTERROR and SEND).
const (
	MsgCall            = 2
	MsgCallResult      = 3
	MsgCallError       = 4
	MsgCallResultError = 5 // 2.1: error answer to a CALLRESULT
	MsgSend            = 6 // 2.1: unconfirmed message, never answered
)

const (
	msgCall            = MsgCall
	msgCallResult      = MsgCallResult
	msgCallError       = MsgCallError
	msgCallResultError = MsgCallResultError
	msgSend            = MsgSend
)

// Subprotocol names negotiated in Sec-WebSocket-Protocol.
const (
	V16  = "ocpp1.6"
	V201 = "ocpp2.0.1"
	V21  = "ocpp2.1"
)

// Is2 reports whether version is OCPP 2.0.1 or 2.1 (the 2.x message
// family and error code names).
func Is2(version string) bool { return version == V201 || version == V21 }

func is2(version string) bool { return Is2(version) }

// ErrKind is a version-neutral RPC error; Code gives the version's name.
type ErrKind int

const (
	KindFormat ErrKind = iota
	KindProperty
	KindOccurrence
	KindType
	KindNotImplemented
	KindNotSupported
	KindInternal
	KindProtocol
	KindSecurity
	KindGeneric
	KindRPCFramework
	KindMessageType
)

// Code is the CALLERROR code of k in version.
func (k ErrKind) Code(version string) string { return k.code(version) }

func (k ErrKind) code(version string) string {
	switch k {
	case KindFormat:
		if version == V16 {
			return "FormationViolation"
		}
		return "FormatViolation"
	case KindProperty:
		return "PropertyConstraintViolation"
	case KindOccurrence:
		if version == V16 {
			return "OccurenceConstraintViolation" // the spelling OCPP-J 1.6 uses
		}
		return "OccurrenceConstraintViolation"
	case KindType:
		return "TypeConstraintViolation"
	case KindNotImplemented:
		return "NotImplemented"
	case KindNotSupported:
		return "NotSupported"
	case KindProtocol:
		return "ProtocolError"
	case KindSecurity:
		return "SecurityError"
	case KindRPCFramework:
		if version == V16 {
			return "FormationViolation"
		}
		return "RpcFrameworkError"
	case KindMessageType:
		if version == V16 {
			return "ProtocolError"
		}
		return "MessageTypeNotSupported"
	case KindGeneric:
		if version == V16 {
			return "GenericError"
		}
		return "GenericError"
	default:
		return "InternalError"
	}
}

// RPCError is a problem to answer with a CALLERROR.
type RPCError struct {
	Kind ErrKind
	Desc string
	// Enum marks a value outside an enumeration (our Go types' check).
	Enum bool
}

func (e *RPCError) Error() string { return e.Desc }

// RPCErrorf is an RPCError of kind k with a formatted description.
func RPCErrorf(k ErrKind, f string, a ...any) *RPCError {
	return &RPCError{Kind: k, Desc: fmt.Sprintf(f, a...)}
}

// CallError is a CALLERROR received from the peer.
type CallError struct {
	Code        string
	Description string
	Details     any
}

func (e *CallError) Error() string {
	if e.Description == "" {
		return "the peer answered " + e.Code
	}
	return "the peer answered " + e.Code + ": " + e.Description
}

// ErrTimeout is returned by Call when no answer arrived in time.
var ErrTimeout = errors.New("no answer from the peer in time")

// ErrClosed is returned by Call on a closed connection.
var ErrClosed = errors.New("the connection is closed")

// Handler answers a CALL: a payload to send in a CALLRESULT, or an error
// (an *RPCError chooses the CALLERROR code; any other error is an
// InternalError).
type Handler func(ctx context.Context, action string, payload json.RawMessage) (any, error)

// frame is one decoded OCPP-J message.
type frame struct {
	typ     int
	id      string
	action  string
	payload json.RawMessage
	code    string
	desc    string
	details json.RawMessage
}

// parseFrame decodes an OCPP-J array. On failure it returns the message ID
// when one could be read, so the error can be answered.
//
// OCPP 2.x part 4 clause 4.2.3: a message whose ID cannot be read is
// answered with the message ID "-1". OCPP 2.1 (clauses 4.1.3, 4.2.4):
// CALLRESULTERROR (5) and SEND (6) are message types. For both 2.x
// versions the 2026-06 errata (part 4, 4.1 to 4.3) make a message of an
// unknown type ignored: no ID is returned, so nothing is answered, and
// MessageTypeNotSupported is no longer sent.
func parseFrame(b []byte, version string) (frame, string, *RPCError) {
	unreadable := ""
	if is2(version) {
		unreadable = "-1"
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(b, &arr); err != nil || len(arr) < 3 {
		return frame{}, unreadable, RPCErrorf(KindRPCFramework, "the message is not an OCPP-J array")
	}
	var f frame
	if err := json.Unmarshal(arr[0], &f.typ); err != nil {
		return frame{}, unreadable, RPCErrorf(KindRPCFramework, "the message type is not a number")
	}
	if err := json.Unmarshal(arr[1], &f.id); err != nil || f.id == "" || len(f.id) > 36 {
		return frame{}, unreadable, RPCErrorf(KindRPCFramework, "the message ID is not a string of 1 to 36 characters")
	}
	v21 := version == V21
	switch {
	case f.typ == msgCall || (v21 && f.typ == msgSend):
		if len(arr) != 4 {
			if f.typ == msgSend {
				return f, "", RPCErrorf(KindRPCFramework, "a SEND has four elements")
			}
			return f, f.id, RPCErrorf(KindRPCFramework, "a CALL has four elements")
		}
		if err := json.Unmarshal(arr[2], &f.action); err != nil || f.action == "" {
			return f, f.id, RPCErrorf(KindRPCFramework, "the action is not a string")
		}
		f.payload = arr[3]
		if string(f.payload) == "null" {
			f.payload = json.RawMessage("{}") // OCPP-J 1.6 4.1.2 and 2.x 4.1.5: null is an empty payload
		}
	case f.typ == msgCallResult:
		if len(arr) != 3 {
			return f, "", RPCErrorf(KindRPCFramework, "a CALLRESULT has three elements")
		}
		f.payload = arr[2]
	case f.typ == msgCallError || (v21 && f.typ == msgCallResultError):
		if len(arr) != 5 {
			return f, "", RPCErrorf(KindRPCFramework, "a CALLERROR has five elements")
		}
		_ = json.Unmarshal(arr[2], &f.code)
		_ = json.Unmarshal(arr[3], &f.desc)
		f.details = arr[4]
	default:
		// ignored, not answered (OCPP-J 1.6 4.1.3; 2.x errata 2026-06)
		return f, "", RPCErrorf(KindMessageType, "message type %d is not known", f.typ)
	}
	return f, f.id, nil
}

// Conn is one OCPP-J connection, either side. At most one CALL is
// outstanding in each direction: Call waits for the previous one to end,
// and received CALLs are handled one after the other.
type Conn struct {
	ws      *websocket.Conn
	version string
	handler Handler
	timeout time.Duration

	callMu sync.Mutex // one outstanding outgoing CALL
	mu     sync.Mutex
	pendID string
	pendCh chan frame
	closed chan struct{}
	once   sync.Once
	err    error

	seq  atomic.Uint64
	wmu  sync.Mutex
	inq  chan frame
	done chan struct{}

	// CheckResult, when set on a 2.1 connection, validates a CALLRESULT
	// to one of our CALLs; a failure is answered with a CALLRESULTERROR
	// (part 4 clause 4.2.3) and returned by Call as a *ResultError.
	CheckResult func(action string, payload json.RawMessage) error
	// OnResultError is told about a CALLRESULTERROR the peer sent about one
	// of our CALLRESULTs.
	OnResultError func(id, code, desc string)

	// sender, when set, carries CALLs instead of the WebSocket (OCPP-S).
	sender func(ctx context.Context, action string, payload json.RawMessage) (json.RawMessage, error)
}

// ResultError is a CALLRESULT that failed validation and was answered
// with a CALLRESULTERROR.
type ResultError struct {
	Code        string
	Description string
}

func (e *ResultError) Error() string {
	return "the peer's answer is not valid (" + e.Code + "): " + e.Description
}

type sendKey struct{}

type relayIDKey struct{}

type msgIDKey struct{}

// MessageID is the ID of the CALL a handler runs for.
func MessageID(ctx context.Context) string { v, _ := ctx.Value(msgIDKey{}).(string); return v }

// WithMessageID makes Call use id (a Local Controller relaying a message).
func WithMessageID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, relayIDKey{}, id)
}

// IsSend reports whether a handler runs for a SEND message, whose result
// is never sent.
func IsSend(ctx context.Context) bool { v, _ := ctx.Value(sendKey{}).(bool); return v }

// NewConn wraps an accepted or dialled WebSocket. Serve must run for calls
// to be answered.
func NewConn(ws *websocket.Conn, version string, timeout time.Duration, h Handler) *Conn {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ws.SetReadLimit(1 << 20)
	return &Conn{
		ws: ws, version: version, handler: h, timeout: timeout,
		closed: make(chan struct{}), inq: make(chan frame, 16), done: make(chan struct{}),
	}
}

// Version is the negotiated OCPP version.
func (c *Conn) Version() string { return c.version }

// Done is closed when the connection ended.
func (c *Conn) Done() <-chan struct{} { return c.closed }

// Err is why the connection ended.
func (c *Conn) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

func (c *Conn) shut(err error) {
	c.once.Do(func() {
		c.mu.Lock()
		c.err = err
		c.mu.Unlock()
		close(c.closed)
	})
}

// Close ends the connection with a normal closure.
func (c *Conn) Close(reason string) {
	if c.ws == nil {
		c.shut(errors.New("closed: " + reason))
		return
	}
	_ = c.ws.Close(websocket.StatusNormalClosure, reason)
	c.shut(errors.New("closed: " + reason))
}

// Serve reads messages until the connection ends; received CALLs are
// handled in order on a second goroutine.
func (c *Conn) Serve(ctx context.Context, ping time.Duration) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go c.handleLoop(ctx)
	if ping > 0 {
		go c.pingLoop(ctx, ping)
	}
	for {
		_, b, err := c.ws.Read(ctx)
		if err != nil {
			c.shut(err)
			_ = c.ws.CloseNow()
			return err
		}
		f, id, perr := parseFrame(b, c.version)
		if perr != nil {
			if id != "" {
				_ = c.writeError(ctx, id, perr)
			}
			continue
		}
		switch f.typ {
		case msgSend:
			select {
			case c.inq <- f:
			default: // never answered: dropped when we are flooded
			}
		case msgCallResultError:
			if c.OnResultError != nil {
				c.OnResultError(f.id, f.code, f.desc)
			}
		case msgCall:
			select {
			case c.inq <- f:
			default:
				// A peer that ignores "one outstanding call" and floods us.
				_ = c.writeError(ctx, f.id, RPCErrorf(KindGeneric, "too many calls outstanding"))
			}
		case msgCallResult, msgCallError:
			c.mu.Lock()
			if c.pendID == f.id && c.pendCh != nil {
				c.pendCh <- f
				c.pendCh, c.pendID = nil, ""
			}
			c.mu.Unlock()
			// An answer to no outstanding call is dropped.
		}
	}
}

func (c *Conn) pingLoop(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pctx, cancel := context.WithTimeout(ctx, every)
			err := c.ws.Ping(pctx)
			cancel()
			if err != nil && ctx.Err() == nil {
				_ = c.ws.Close(websocket.StatusGoingAway, "no pong")
				return
			}
		}
	}
}

func (c *Conn) handleLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case f := <-c.inq:
			hctx, cancel := context.WithTimeout(ctx, c.timeout)
			if f.typ == msgSend {
				_, _ = c.safeHandle(context.WithValue(hctx, sendKey{}, true), f)
				cancel()
				continue
			}
			res, err := c.safeHandle(context.WithValue(hctx, msgIDKey{}, f.id), f)
			cancel()
			if err != nil {
				var re *RPCError
				var ce *CallError
				if errors.As(err, &ce) {
					// relayed: the peer's own error code
					_ = c.write(ctx, []any{msgCallError, f.id, ce.Code, limitDesc(ce.Description), struct{}{}})
					continue
				}
				if !errors.As(err, &re) {
					re = RPCErrorf(KindInternal, "%v", err)
				}
				_ = c.writeError(ctx, f.id, re)
				continue
			}
			if res == nil {
				res = struct{}{}
			}
			pl, merr := json.Marshal(res)
			if merr != nil {
				_ = c.writeError(ctx, f.id, RPCErrorf(KindInternal, "the answer cannot be encoded"))
				continue
			}
			_ = c.write(ctx, []any{msgCallResult, f.id, json.RawMessage(pl)})
		}
	}
}

func (c *Conn) safeHandle(ctx context.Context, f frame) (res any, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("handler panicked: %v", p)
		}
	}()
	return c.handler(ctx, f.action, f.payload)
}

func (c *Conn) write(ctx context.Context, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	wctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	return c.ws.Write(wctx, websocket.MessageText, b)
}

func (c *Conn) writeError(ctx context.Context, id string, e *RPCError) error {
	return c.write(ctx, []any{msgCallError, id, e.Kind.code(c.version), limitDesc(e.Desc), struct{}{}})
}

// limitDesc keeps an error description within the 255 characters of OCPP
// 2.x part 4 (Table 7).
func limitDesc(s string) string {
	if r := []rune(s); len(r) > 255 {
		return string(r[:252]) + "..."
	}
	return s
}

// newMessageID is a random UUID (version 4): unique over every connection
// of the same identity, as 2.1 part 4 clause 4.1.4 asks, and within the 36
// characters of a message ID.
func newMessageID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// NewSenderConn is a connection whose CALLs are carried by send instead of
// a WebSocket (OCPP-S: each CALL is an HTTP request to the charger).
func NewSenderConn(version string, timeout time.Duration, send func(ctx context.Context, action string, payload json.RawMessage) (json.RawMessage, error)) *Conn {
	return &Conn{version: version, timeout: timeout, closed: make(chan struct{}), inq: make(chan frame, 1), done: make(chan struct{}), sender: send}
}

// SetHandler sets the handler of received CALLs; call it before Serve.
func (c *Conn) SetHandler(h Handler) { c.handler = h }

// IsSender reports whether c was made by NewSenderConn.
func (c *Conn) IsSender() bool { return c.sender != nil }

// NewMessageID is a random UUID (version 4) message ID.
func NewMessageID() string { return newMessageID() }

// WriteRaw writes a frame as is (tests of the RPC framework).
func (c *Conn) WriteRaw(ctx context.Context, b []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return c.ws.Write(ctx, websocket.MessageText, b)
}

// Send sends a SEND message (OCPP 2.1 part 4 clause 4.2.4): not answered,
// and not bound to the one-outstanding-CALL rule.
func (c *Conn) Send(ctx context.Context, action string, payload any) error {
	if c.version != V21 {
		return fmt.Errorf("SEND needs OCPP 2.1, not %s", c.version)
	}
	select {
	case <-c.closed:
		return ErrClosed
	default:
	}
	pl, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return c.write(ctx, []any{msgSend, newMessageID(), action, json.RawMessage(pl)})
}

// Call sends a CALL and waits for its CALLRESULT (returned) or CALLERROR
// (a *CallError). Only one CALL is outstanding at a time; a second caller
// waits its turn within ctx.
func (c *Conn) Call(ctx context.Context, action string, req any) (json.RawMessage, error) {
	if err := lockCtx(ctx, &c.callMu); err != nil {
		return nil, err
	}
	defer c.callMu.Unlock()
	select {
	case <-c.closed:
		return nil, ErrClosed
	default:
	}
	c.seq.Add(1)
	id := newMessageID()
	if rid, ok := ctx.Value(relayIDKey{}).(string); ok && rid != "" {
		id = rid // a relayed message keeps its message ID
	}
	ch := make(chan frame, 1)
	c.mu.Lock()
	c.pendID, c.pendCh = id, ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		if c.pendID == id {
			c.pendID, c.pendCh = "", nil
		}
		c.mu.Unlock()
	}()
	if req == nil {
		req = struct{}{}
	}
	pl, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if c.sender != nil {
		return c.sender(ctx, action, pl)
	}
	if err := c.write(ctx, []any{msgCall, id, action, json.RawMessage(pl)}); err != nil {
		return nil, fmt.Errorf("sending %s: %w", action, err)
	}
	t := time.NewTimer(c.timeout)
	defer t.Stop()
	select {
	case f := <-ch:
		if f.typ == msgCallError {
			var det any
			_ = json.Unmarshal(f.details, &det)
			return nil, &CallError{Code: f.code, Description: f.desc, Details: det}
		}
		if c.version == V21 && c.CheckResult != nil {
			if re := c.CheckResult(action, f.payload); re != nil {
				code, desc := KindFormat.code(c.version), re.Error()
				var se *SchemaError
				if errors.As(re, &se) {
					code, desc = se.Code, se.Description
				}
				_ = c.write(ctx, []any{msgCallResultError, id, code, limitDesc(desc), struct{}{}})
				return f.payload, &ResultError{Code: code, Description: desc}
			}
		}
		return f.payload, nil
	case <-t.C:
		return nil, ErrTimeout
	case <-c.closed:
		return nil, ErrClosed
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// lockCtx takes mu unless ctx ends first.
func lockCtx(ctx context.Context, mu *sync.Mutex) error {
	for {
		if mu.TryLock() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
}
