package ocpp

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

// OCPP-S 1.6 (SOAP 1.2 with WS-Addressing, OCPP-S 1.6 and its errata,
// the two OCA WSDLs in oca/1.6/wsdl). The binding only translates: a
// request's body element (e.g. bootNotificationRequest) becomes the
// OCPP-J 1.6 payload, typed by the OCA 1.6 JSON schema of the action (the
// WSDL's element names are the JSON property names), so the application
// handles it as it handles OCPP-J; the answer is written back as
// <action>Response in the Central System namespace, with Action
// /<Action>Response and RelatesTo the request's MessageID. A problem
// becomes a SOAP Fault with the OCPP sub-code (SecurityError,
// IdentityMismatch, ProtocolError, InternalError, NotSupported).
//
// The charger's identity is the chargeBoxIdentity header; the address in
// the WS-Addressing From header is where the Central System calls the
// charger: SOAPCall POSTs a Charge Point service request there
// (chargeBoxIdentity, Action /<Action>, MessageID, From our own URL,
// ReplyTo anonymous, To the charger's address) and reads its answer back
// through the same schema. gzip and deflate request bodies are taken
// (OCPP-S 6.1). Elements are written in the xs:sequence order of the
// WSDLs (soapwsdl.go) and read in any order.

// WSAAnonymous is the WS-Addressing anonymous address.
const WSAAnonymous = "http://www.w3.org/2005/08/addressing/anonymous"

const (
	soapEnvNS = "http://www.w3.org/2003/05/soap-envelope"
	wsaNS     = "http://www.w3.org/2005/08/addressing"
	wsaAnon   = WSAAnonymous
	csNS      = "urn://Ocpp/Cs/2015/10/"
	cpNS      = "urn://Ocpp/Cp/2015/10/"
)

// xnode is a generic XML element.
type xnode struct {
	XMLName  xml.Name
	Attrs    []xml.Attr `xml:",any,attr"`
	Children []xnode    `xml:",any"`
	Text     string     `xml:",chardata"`
}

func (n *xnode) child(local string) *xnode {
	for i := range n.Children {
		if strings.EqualFold(n.Children[i].XMLName.Local, local) {
			return &n.Children[i]
		}
	}
	return nil
}

func (n *xnode) text(local string) string {
	if c := n.child(local); c != nil {
		if a := c.child("Address"); a != nil {
			return strings.TrimSpace(a.Text)
		}
		return strings.TrimSpace(c.Text)
	}
	return ""
}

// ---- schema-driven XML ↔ JSON ------------------------------------------------------

// oschema is the part of a 1.6 JSON schema that types XML text: each
// property's JSON type, and an array's items. The order elements are
// written in comes from the WSDLs (soapwsdl.go), not from here.
type oschema struct {
	typ   string
	props map[string]*oschema
	items *oschema
}

var (
	oschemaMu    sync.Mutex
	oschemaCache = map[string]*oschema{}
)

func orderedSchema(name string) (*oschema, error) {
	oschemaMu.Lock()
	defer oschemaMu.Unlock()
	if s, ok := oschemaCache[name]; ok {
		return s, nil
	}
	raw, ok := schemaSets[V16].raw()[name]
	if !ok {
		return nil, fmt.Errorf("no OCPP 1.6 schema %s", name)
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	v, err := readOrdered(d)
	if err != nil {
		return nil, err
	}
	s := toOSchema(v)
	oschemaCache[name] = s
	return s, nil
}

type omap struct {
	keys []string
	vals map[string]any
}

func readOrdered(d *json.Decoder) (any, error) {
	t, err := d.Token()
	if err != nil {
		return nil, err
	}
	switch x := t.(type) {
	case json.Delim:
		switch x {
		case '{':
			m := &omap{vals: map[string]any{}}
			for d.More() {
				kt, err := d.Token()
				if err != nil {
					return nil, err
				}
				k, _ := kt.(string)
				v, err := readOrdered(d)
				if err != nil {
					return nil, err
				}
				m.keys = append(m.keys, k)
				m.vals[k] = v
			}
			_, err := d.Token()
			return m, err
		case '[':
			var l []any
			for d.More() {
				v, err := readOrdered(d)
				if err != nil {
					return nil, err
				}
				l = append(l, v)
			}
			_, err := d.Token()
			return l, err
		}
	}
	return t, nil
}

func toOSchema(v any) *oschema {
	m, ok := v.(*omap)
	if !ok {
		return &oschema{}
	}
	s := &oschema{props: map[string]*oschema{}}
	s.typ, _ = m.vals["type"].(string)
	if p, ok := m.vals["properties"].(*omap); ok {
		for _, k := range p.keys {
			s.props[k] = toOSchema(p.vals[k])
		}
	}
	if it, ok := m.vals["items"]; ok {
		s.items = toOSchema(it)
	}
	return s
}

// fromXML turns an element into the JSON value its schema describes.
// Children are taken in any order (a peer that does not follow the WSDL
// sequence is still understood); CheckSOAPEnvelope is the strict check.
func fromXML(n *xnode, s *oschema) (any, error) {
	switch s.typ {
	case "object", "":
		out := map[string]any{}
		for i := range n.Children {
			c := &n.Children[i]
			ps := s.props[c.XMLName.Local]
			if ps == nil {
				return nil, fmt.Errorf("%s has no element %s", n.XMLName.Local, c.XMLName.Local)
			}
			if ps.typ == "array" {
				v, err := fromXML(c, ps.items)
				if err != nil {
					return nil, err
				}
				l, _ := out[c.XMLName.Local].([]any)
				out[c.XMLName.Local] = append(l, v)
				continue
			}
			v, err := fromXML(c, ps)
			if err != nil {
				return nil, err
			}
			out[c.XMLName.Local] = v
		}
		return out, nil
	case "integer":
		i, err := strconv.ParseInt(strings.TrimSpace(n.Text), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s is not an integer", n.XMLName.Local)
		}
		return i, nil
	case "number":
		f, err := strconv.ParseFloat(strings.TrimSpace(n.Text), 64)
		if err != nil {
			return nil, fmt.Errorf("%s is not a number", n.XMLName.Local)
		}
		return json.Number(strconv.FormatFloat(f, 'f', -1, 64)), nil
	case "boolean":
		b, err := strconv.ParseBool(strings.TrimSpace(n.Text))
		if err != nil {
			return nil, fmt.Errorf("%s is not a boolean", n.XMLName.Local)
		}
		return b, nil
	}
	return n.Text, nil
}

// writeBody writes the children of body element el of the service with
// namespace ns in the order of its WSDL sequence.
func writeBody(b *bytes.Buffer, ns, el string, v any, s *oschema) {
	w, _ := soapTypes(ns) // the embedded WSDLs read (TestOCPPSElementOrderFollowsWSDL)
	var seq []wsdlElem
	if w != nil {
		seq = w.typeOf(el)
	}
	toXMLSeq(b, v, w, seq, s)
}

func scalar(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		return t.String()
	case bool:
		return strconv.FormatBool(t)
	case nil:
		return ""
	}
	return fmt.Sprint(v)
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToLower(r[0])
	return string(r)
}

// ---- inbound -------------------------------------------------------------------------

type soapEnvelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Header  xnode    `xml:"Header"`
	Body    xnode    `xml:"Body"`
}

func soapBody(r *http.Request) ([]byte, error) {
	var rd io.Reader = http.MaxBytesReader(nil, r.Body, 1<<20)
	switch strings.ToLower(r.Header.Get("Content-Encoding")) {
	case "gzip":
		z, err := gzip.NewReader(rd)
		if err != nil {
			return nil, err
		}
		rd = z
	case "deflate":
		rd = flate.NewReader(rd)
	}
	return io.ReadAll(io.LimitReader(rd, 1<<20))
}

func writeSOAP(w http.ResponseWriter, status int, header, body string) {
	w.Header().Set("Content-Type", "application/soap+xml; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><s:Envelope xmlns:s="%s" xmlns:a="%s"><s:Header>%s</s:Header><s:Body>%s</s:Body></s:Envelope>`,
		soapEnvNS, wsaNS, header, body)
}

func esc(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// soapFault answers a SOAP Fault with an OCPP sub-code (OCPP-S Table 1).
func soapFault(w http.ResponseWriter, relatesTo, sender, sub, reason string) {
	code := "s:Receiver"
	if sender == "Sender" {
		code = "s:Sender"
	}
	hdr := `<a:Action>http://www.w3.org/2005/08/addressing/soap/fault</a:Action>`
	if relatesTo != "" {
		hdr += `<a:RelatesTo>` + esc(relatesTo) + `</a:RelatesTo>`
	}
	writeSOAP(w, http.StatusInternalServerError, hdr, fmt.Sprintf(`<s:Fault><s:Code><s:Value>%s</s:Value><s:Subcode><s:Value xmlns:o="%s">o:%s</s:Value></s:Subcode></s:Code><s:Reason><s:Text xml:lang="en">%s</s:Text></s:Reason></s:Fault>`,
		code, strings.TrimSuffix(csNS, "/"), sub, esc(reason)))
}

func faultOf(e *RPCError) (string, string) {
	switch e.Kind {
	case KindSecurity:
		return "Sender", "SecurityError"
	case KindNotImplemented, KindNotSupported:
		return "Receiver", "NotSupported"
	case KindInternal:
		return "Receiver", "InternalError"
	}
	return "Sender", "ProtocolError"
}

// SOAPFault is a SOAP Fault to answer with an OCPP sub-code (OCPP-S
// Table 1): Sender is "Sender" or "Receiver".
type SOAPFault struct {
	RelatesTo, Sender, Sub, Reason string
}

// Write answers the fault.
func (f *SOAPFault) Write(w http.ResponseWriter) {
	soapFault(w, f.RelatesTo, f.Sender, f.Sub, f.Reason)
}

// WriteSOAPFault answers a SOAP Fault with an OCPP sub-code.
func WriteSOAPFault(w http.ResponseWriter, relatesTo, sender, sub, reason string) {
	soapFault(w, relatesTo, sender, sub, reason)
}

// SOAPFaultOf is the fault code ("Sender" or "Receiver") and OCPP
// sub-code of an RPC error.
func SOAPFaultOf(e *RPCError) (string, string) { return faultOf(e) }

// SOAPRequest is an OCPP-S request to the Central System service.
type SOAPRequest struct {
	MessageID string // WS-Addressing MessageID
	From      string // WS-Addressing From: where the charger takes calls
	Identity  string // chargeBoxIdentity
	env       soapEnvelope
}

// ReadSOAPRequest reads a SOAP 1.2 envelope from a POSTed request
// (gzip and deflate bodies too).
func ReadSOAPRequest(r *http.Request) (*SOAPRequest, *SOAPFault) {
	raw, err := soapBody(r)
	if err != nil {
		return nil, &SOAPFault{"", "Sender", "ProtocolError", "the request body cannot be read"}
	}
	q := &SOAPRequest{}
	if err := xml.Unmarshal(raw, &q.env); err != nil || q.env.XMLName.Space != soapEnvNS {
		return nil, &SOAPFault{"", "Sender", "ProtocolError", "not a SOAP 1.2 envelope"}
	}
	h := &q.env.Header
	q.MessageID, q.From = h.text("MessageID"), h.text("From")
	q.Identity = h.text("chargeBoxIdentity")
	return q, nil
}

// Action is the request's action: the Action header without its slash,
// else the body element's name; the body must hold one element that
// matches it.
func (q *SOAPRequest) Action() (string, *SOAPFault) {
	if len(q.env.Body.Children) != 1 {
		return "", &SOAPFault{q.MessageID, "Sender", "ProtocolError", "the body holds one request"}
	}
	el := &q.env.Body.Children[0]
	action := strings.TrimPrefix(q.env.Header.text("Action"), "/")
	if action == "" {
		action = strings.TrimSuffix(el.XMLName.Local, "Request")
		if action != "" {
			action = strings.ToUpper(action[:1]) + action[1:]
		}
	}
	if !strings.EqualFold(el.XMLName.Local, lowerFirst(action)+"Request") {
		return "", &SOAPFault{q.MessageID, "Sender", "ProtocolError", "the body element does not match the Action"}
	}
	return action, nil
}

// Payload is the request's body element as the OCPP-J 1.6 payload of
// action, typed by the action's 1.6 JSON schema.
func (q *SOAPRequest) Payload(action string) (json.RawMessage, *SOAPFault) {
	sch, err := orderedSchema(action)
	if err != nil {
		return nil, &SOAPFault{q.MessageID, "Receiver", "NotSupported", "action " + action + " is not supported"}
	}
	payload, err := fromXML(&q.env.Body.Children[0], sch)
	if err != nil {
		return nil, &SOAPFault{q.MessageID, "Sender", "ProtocolError", err.Error()}
	}
	pb, _ := json.Marshal(payload)
	return pb, nil
}

// WriteSOAPResponse answers a Central System service request (MessageID
// msgID) to action with res, the OCPP-J 1.6 answer, as <action>Response.
func WriteSOAPResponse(w http.ResponseWriter, msgID, action string, res any) {
	writeSOAPResponse(w, csNS, msgID, action, res)
}

// WriteChargePointSOAPResponse answers a Charge Point service request
// (MessageID msgID) to action with res, as a charge point does (the
// simulator).
func WriteChargePointSOAPResponse(w http.ResponseWriter, msgID, action string, res any) {
	writeSOAPResponse(w, cpNS, msgID, action, res)
}

func writeSOAPResponse(w http.ResponseWriter, ns, msgID, action string, res any) {
	rb, _ := json.Marshal(res)
	var ans any
	dec := json.NewDecoder(bytes.NewReader(rb))
	dec.UseNumber()
	_ = dec.Decode(&ans)
	rs, err := orderedSchema(action + "Response")
	if err != nil {
		soapFault(w, msgID, "Receiver", "InternalError", err.Error())
		return
	}
	var body bytes.Buffer
	body.WriteString("<" + lowerFirst(action) + `Response xmlns="` + ns + `">`)
	writeBody(&body, ns, lowerFirst(action)+"Response", ans, rs)
	body.WriteString("</" + lowerFirst(action) + "Response>")
	hdr := `<a:Action s:mustUnderstand="true">/` + action + `Response</a:Action>`
	if msgID != "" {
		hdr += `<a:RelatesTo>` + esc(msgID) + `</a:RelatesTo>`
	}
	writeSOAP(w, http.StatusOK, hdr, body.String())
}

// UnreachableError is a SOAP call that did not reach the charger.
type UnreachableError struct{ Err error }

func (e *UnreachableError) Error() string { return e.Err.Error() }
func (e *UnreachableError) Unwrap() error { return e.Err }

// SOAPCall POSTs one Charge Point service request for charger identity
// to the address to, with own (our Central System service URL, "" for
// anonymous) in From, and returns the answer as the OCPP-J 1.6 payload.
// A SOAP Fault is returned as a *CallError, an HTTP failure as an
// *UnreachableError.
func SOAPCall(ctx context.Context, client *http.Client, identity, own, to, action string, payload json.RawMessage) (json.RawMessage, error) {
	return soapCall(ctx, client, cpNS, identity, own, to, action, payload)
}

// SOAPCallCentralSystem POSTs one Central System service request, as
// charge point identity, to the Central System at to, with own (where the
// charge point takes calls, "" for anonymous) in From, as a charge point
// does (the simulator). Answers and errors are as for SOAPCall.
func SOAPCallCentralSystem(ctx context.Context, client *http.Client, identity, own, to, action string, payload json.RawMessage) (json.RawMessage, error) {
	return soapCall(ctx, client, csNS, identity, own, to, action, payload)
}

func soapCall(ctx context.Context, client *http.Client, ns, identity, own, to, action string, payload json.RawMessage) (json.RawMessage, error) {
	var v any
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.UseNumber()
	_ = dec.Decode(&v)
	rs, err := orderedSchema(action)
	if err != nil {
		return nil, err
	}
	var body bytes.Buffer
	body.WriteString("<" + lowerFirst(action) + `Request xmlns="` + ns + `">`)
	writeBody(&body, ns, lowerFirst(action)+"Request", v, rs)
	body.WriteString("</" + lowerFirst(action) + "Request>")
	msgID := "urn:uuid:" + newMessageID()
	if own == "" {
		own = wsaAnon
	}
	hdr := fmt.Sprintf(`<chargeBoxIdentity xmlns="%s" s:mustUnderstand="true">%s</chargeBoxIdentity><a:Action s:mustUnderstand="true">/%s</a:Action><a:MessageID>%s</a:MessageID><a:From><a:Address>%s</a:Address></a:From><a:ReplyTo><a:Address>%s</a:Address></a:ReplyTo><a:To s:mustUnderstand="true">%s</a:To>`,
		ns, esc(identity), action, msgID, esc(own), wsaAnon, esc(to))
	env := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><s:Envelope xmlns:s="%s" xmlns:a="%s"><s:Header>%s</s:Header><s:Body>%s</s:Body></s:Envelope>`, soapEnvNS, wsaNS, hdr, body.String())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, to, strings.NewReader(env))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", `application/soap+xml; charset=utf-8; action="/`+action+`"`)
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, &UnreachableError{err}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var out soapEnvelope
	if err := xml.Unmarshal(raw, &out); err != nil || len(out.Body.Children) != 1 {
		return nil, fmt.Errorf("the charger's answer is not a SOAP envelope (HTTP %d)", resp.StatusCode)
	}
	el := &out.Body.Children[0]
	if el.XMLName.Local == "Fault" {
		sub := ""
		if code := el.child("Code"); code != nil {
			if sc := code.child("Subcode"); sc != nil {
				sub = strings.TrimSpace(sc.text("Value"))
				if i := strings.LastIndex(sub, ":"); i >= 0 {
					sub = sub[i+1:]
				}
			}
		}
		reason := ""
		if rs := el.child("Reason"); rs != nil {
			reason = rs.text("Text")
		}
		return nil, &CallError{Code: sub, Description: reason}
	}
	if !strings.EqualFold(el.XMLName.Local, lowerFirst(action)+"Response") {
		return nil, fmt.Errorf("the charger answered %s to %s", el.XMLName.Local, action)
	}
	ans, err := orderedSchema(action + "Response")
	if err != nil {
		return nil, err
	}
	v2, err := fromXML(el, ans)
	if err != nil {
		return nil, err
	}
	return json.Marshal(v2)
}
