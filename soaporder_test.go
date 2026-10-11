// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package ocpp_test

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path"
	"sort"
	"strings"
	"testing"

	"github.com/cB-Sigma/ocpp"
)

// ---- the test's own reading of the WSDLs ------------------------------------------
//
// The order is read here independently of the library's WSDL reader, so a
// mistake in one is not repeated in the other: a token walk over the
// embedded OCA files, keeping each complexType's xs:sequence and each
// top-level element's type.

type testWSDL struct {
	ns    string
	top   map[string]string            // element -> complexType
	seq   map[string][]string          // complexType -> element names in sequence order
	child map[string]map[string]string // complexType -> element -> type (local name)
}

func readTestWSDL(t *testing.T, file string) *testWSDL {
	t.Helper()
	raw, err := fs.ReadFile(ocpp.OCAFiles(), path.Join("1.6", "wsdl", file))
	if err != nil {
		t.Fatal(err)
	}
	const xsd = "http://www.w3.org/2001/XMLSchema"
	w := &testWSDL{top: map[string]string{}, seq: map[string][]string{}, child: map[string]map[string]string{}}
	d := xml.NewDecoder(strings.NewReader(string(raw)))
	var stack []string // XSD element names open
	ct := ""           // the complexType being read
	attr := func(se xml.StartElement, name string) string {
		for _, a := range se.Attr {
			if a.Name.Local == name && a.Name.Space == "" {
				return a.Value
			}
		}
		return ""
	}
	local := func(q string) string {
		if i := strings.LastIndex(q, ":"); i >= 0 {
			return q[i+1:]
		}
		return q
	}
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		switch x := tok.(type) {
		case xml.StartElement:
			if x.Name.Space != xsd {
				stack = append(stack, "")
				continue
			}
			parent := ""
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			switch x.Name.Local {
			case "schema":
				w.ns = attr(x, "targetNamespace")
			case "complexType":
				ct = attr(x, "name")
				w.seq[ct] = nil
				w.child[ct] = map[string]string{}
			case "element":
				if parent == "schema" {
					w.top[attr(x, "name")] = local(attr(x, "type"))
				} else if parent == "sequence" && ct != "" {
					n := attr(x, "name")
					w.seq[ct] = append(w.seq[ct], n)
					w.child[ct][n] = local(attr(x, "type"))
				}
			}
			stack = append(stack, x.Name.Local)
		case xml.EndElement:
			if len(stack) > 0 {
				if stack[len(stack)-1] == "complexType" {
					ct = ""
				}
				stack = stack[:len(stack)-1]
			}
		}
	}
	if w.ns == "" || len(w.top) == 0 {
		t.Fatalf("%s: no schema read", file)
	}
	return w
}

// ---- sample payloads from the OCA 1.6 JSON schemas ---------------------------------

// sampleOf fills every property a JSON schema allows, so each optional
// element is written and its place in the sequence is checked.
func sampleOf(s map[string]any) any {
	if e, ok := s["enum"].([]any); ok && len(e) > 0 {
		return e[0]
	}
	switch s["type"] {
	case "object":
		out := map[string]any{}
		props, _ := s["properties"].(map[string]any)
		for k, v := range props {
			ps, _ := v.(map[string]any)
			out[k] = sampleOf(ps)
		}
		return out
	case "array":
		it, _ := s["items"].(map[string]any)
		return []any{sampleOf(it), sampleOf(it)}
	case "integer":
		return 1
	case "number":
		return 1.5
	case "boolean":
		return true
	}
	switch s["format"] {
	case "date-time":
		return "2026-10-06T10:00:00Z"
	case "uri":
		return "http://example.com/file.bin"
	}
	v := "Sample"
	if ml, ok := s["maxLength"].(float64); ok && int(ml) < len(v) {
		v = v[:int(ml)]
	}
	return v
}

func samplePayload(t *testing.T, schema string) json.RawMessage {
	t.Helper()
	files, err := ocpp.SchemaFiles("1.6")
	if err != nil {
		t.Fatal(err)
	}
	raw, ok := files[schema]
	if !ok {
		t.Fatalf("no 1.6 schema %s", schema)
	}
	var s map[string]any
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(sampleOf(s))
	action, resp := strings.CutSuffix(schema, "Response")
	if err := ocpp.ValidateMessage(ocpp.V16, action, resp, b); err != nil {
		t.Fatalf("sample %s does not pass its schema: %v", schema, err)
	}
	return b
}

// ---- checking an emitted envelope ---------------------------------------------------

type gnode struct {
	XMLName  xml.Name
	Children []gnode `xml:",any"`
}

// bodyElement is the one element in an envelope's Body.
func bodyElement(t *testing.T, env string) gnode {
	t.Helper()
	var e struct {
		Body struct {
			Children []gnode `xml:",any"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal([]byte(env), &e); err != nil || len(e.Body.Children) != 1 {
		t.Fatalf("not one body element (%v): %s", err, env)
	}
	return e.Body.Children[0]
}

// checkOrder walks n against the WSDL type typ: each element's children
// come in the type's sequence order, every child is declared, and every
// element of the sequence the sample carries is written.
func checkOrder(t *testing.T, w *testWSDL, where string, n gnode, typ string) {
	t.Helper()
	seq, ok := w.seq[typ]
	if !ok {
		return // a simple type
	}
	pos := map[string]int{}
	for i, s := range seq {
		pos[s] = i
	}
	last := -1
	seen := map[string]bool{}
	for _, c := range n.Children {
		name := c.XMLName.Local
		if c.XMLName.Space != w.ns {
			t.Errorf("%s/%s: namespace %q, want %q", where, name, c.XMLName.Space, w.ns)
		}
		p, ok := pos[name]
		if !ok {
			t.Errorf("%s: %s is not in the WSDL type %s", where, name, typ)
			continue
		}
		if p < last {
			got := make([]string, 0, len(n.Children))
			for _, c := range n.Children {
				got = append(got, c.XMLName.Local)
			}
			t.Errorf("%s: children %v are not in the WSDL sequence %v of %s", where, got, seq, typ)
			return
		}
		last = p
		seen[name] = true
		checkOrder(t, w, where+"/"+name, c, w.child[typ][name])
	}
	for _, s := range seq {
		if !seen[s] {
			t.Errorf("%s: the sample carries every property, but %s (WSDL %s) was not written", where, s, typ)
		}
	}
}

func checkEnvelope(t *testing.T, w *testWSDL, env, element string) {
	t.Helper()
	el := bodyElement(t, env)
	if el.XMLName.Local != element || el.XMLName.Space != w.ns {
		t.Fatalf("body element {%s}%s, want {%s}%s", el.XMLName.Space, el.XMLName.Local, w.ns, element)
	}
	typ, ok := w.top[element]
	if !ok {
		t.Fatalf("the WSDL has no element %s", element)
	}
	checkOrder(t, w, element, el, typ)
	if err := ocpp.CheckSOAPEnvelope([]byte(env)); err != nil {
		t.Errorf("%s: the library's WSDL check refuses what the library wrote: %v", element, err)
	}
}

// operations lists the WSDL's actions (elements named <action>Request).
func operations(w *testWSDL) []string {
	var out []string
	for el := range w.top {
		if a, ok := strings.CutSuffix(el, "Request"); ok {
			out = append(out, strings.ToUpper(a[:1])+a[1:])
		}
	}
	sort.Strings(out)
	return out
}

// Covers: SIG-IPE-6414
// OCPP-S 1.6 messages are written with their elements in the order of
// the xs:sequence of their type in the embedded OCA WSDLs, nested types
// included, in all four directions: Central System service responses and
// Charge Point service requests (our CSMS), Central System service
// requests and Charge Point service responses (the simulator's charge
// point). The order is read from the WSDL files when the test runs.
func TestOCPPSElementOrderFollowsWSDL(t *testing.T) {
	cs := readTestWSDL(t, "OCPP_CentralSystemService_1.6.wsdl")
	cp := readTestWSDL(t, "OCPP_ChargePointService_1.6.wsdl")
	if len(operations(cs)) != 10 || len(operations(cp)) != 19 { // DataTransfer is in both
		t.Fatalf("operations: %d Central System, %d Charge Point", len(operations(cs)), len(operations(cp)))
	}

	// A requester that keeps what was POSTed and answers with a canned
	// envelope.
	capture := func(answer string) (*httptest.Server, *string) {
		var got string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			got = string(b)
			w.Header().Set("Content-Type", "application/soap+xml; charset=utf-8")
			io.WriteString(w, answer)
		}))
		return srv, &got
	}
	fault := `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><s:Fault><s:Code><s:Value>s:Receiver</s:Value></s:Code><s:Reason><s:Text>x</s:Text></s:Reason></s:Fault></s:Body></s:Envelope>`

	// Central System service responses (what our CSMS answers).
	for _, a := range operations(cs) {
		rec := httptest.NewRecorder()
		var res any
		_ = json.Unmarshal(samplePayload(t, a+"Response"), &res)
		ocpp.WriteSOAPResponse(rec, "urn:uuid:1", a, res)
		checkEnvelope(t, cs, rec.Body.String(), lowerFirst(a)+"Response")
	}
	// Charge Point service requests (what our CSMS sends a charger).
	for _, a := range operations(cp) {
		srv, got := capture(fault)
		_, _ = ocpp.SOAPCall(context.Background(), srv.Client(), "CP1", "", srv.URL, a, samplePayload(t, a))
		srv.Close()
		checkEnvelope(t, cp, *got, lowerFirst(a)+"Request")
	}
	// Central System service requests (what a SOAP charge point sends).
	for _, a := range operations(cs) {
		srv, got := capture(fault)
		_, _ = ocpp.SOAPCallCentralSystem(context.Background(), srv.Client(), "CP1", "http://cp.example/", srv.URL, a, samplePayload(t, a))
		srv.Close()
		checkEnvelope(t, cs, *got, lowerFirst(a)+"Request")
	}
	// Charge Point service responses (what a SOAP charge point answers).
	for _, a := range operations(cp) {
		rec := httptest.NewRecorder()
		var res any
		_ = json.Unmarshal(samplePayload(t, a+"Response"), &res)
		ocpp.WriteChargePointSOAPResponse(rec, "urn:uuid:1", a, res)
		checkEnvelope(t, cp, rec.Body.String(), lowerFirst(a)+"Response")
	}
}

func lowerFirst(s string) string { return strings.ToLower(s[:1]) + s[1:] }

func csEnvelope(body string) string {
	return `<?xml version="1.0" encoding="UTF-8"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:a="http://www.w3.org/2005/08/addressing"><s:Header><a:Action>/BootNotification</a:Action><a:MessageID>urn:uuid:1</a:MessageID></s:Header><s:Body>` + body + `</s:Body></s:Envelope>`
}

// Covers: SIG-IPE-6414
// The strict check refuses what a schema-validating SOAP stack refuses:
// elements out of the WSDL sequence (top level and nested), missing
// required elements, undeclared or repeated elements, a wrong namespace,
// and values outside their simple type.
func TestCheckSOAPEnvelopeStrict(t *testing.T) {
	ok := []string{
		`<bootNotificationResponse xmlns="urn://Ocpp/Cs/2015/10/"><status>Accepted</status><currentTime>2026-10-06T10:00:00Z</currentTime><interval>300</interval></bootNotificationResponse>`,
		`<authorizeResponse xmlns="urn://Ocpp/Cs/2015/10/"><idTagInfo><status>Accepted</status><expiryDate>2026-10-06T10:00:00.123+05:30</expiryDate></idTagInfo></authorizeResponse>`,
		`<heartbeatRequest xmlns="urn://Ocpp/Cs/2015/10/"/>`,
		`<meterValuesRequest xmlns="urn://Ocpp/Cs/2015/10/"><connectorId>1</connectorId><meterValue><timestamp>2026-10-06T10:00:00Z</timestamp><sampledValue><value>1</value><unit>Wh</unit></sampledValue><sampledValue><value>2</value></sampledValue></meterValue></meterValuesRequest>`,
		`<setChargingProfileRequest xmlns="urn://Ocpp/Cp/2015/10/"><connectorId>1</connectorId><csChargingProfiles><chargingProfileId>1</chargingProfileId><stackLevel>0</stackLevel><chargingProfilePurpose>TxDefaultProfile</chargingProfilePurpose><chargingProfileKind>Absolute</chargingProfileKind><chargingSchedule><chargingRateUnit>A</chargingRateUnit><chargingSchedulePeriod><startPeriod>0</startPeriod><limit>16.0</limit></chargingSchedulePeriod></chargingSchedule></csChargingProfiles></setChargingProfileRequest>`,
		`<s:Fault xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Code><s:Value>s:Receiver</s:Value></s:Code></s:Fault>`,
	}
	for _, b := range ok {
		if err := ocpp.CheckSOAPEnvelope([]byte(csEnvelope(b))); err != nil {
			t.Errorf("%s: %v", b, err)
		}
	}
	bad := []struct{ want, body string }{
		{"out of order", `<bootNotificationResponse xmlns="urn://Ocpp/Cs/2015/10/"><currentTime>2026-10-06T10:00:00Z</currentTime><interval>300</interval><status>Accepted</status></bootNotificationResponse>`},
		{"out of order", `<authorizeResponse xmlns="urn://Ocpp/Cs/2015/10/"><idTagInfo><expiryDate>2026-10-06T10:00:00Z</expiryDate><status>Accepted</status></idTagInfo></authorizeResponse>`},
		{"is missing", `<bootNotificationResponse xmlns="urn://Ocpp/Cs/2015/10/"><status>Accepted</status><interval>300</interval></bootNotificationResponse>`},
		{"not declared", `<heartbeatRequest xmlns="urn://Ocpp/Cs/2015/10/"><extra>1</extra></heartbeatRequest>`},
		{"more than", `<authorizeRequest xmlns="urn://Ocpp/Cs/2015/10/"><idTag>A</idTag><idTag>B</idTag></authorizeRequest>`},
		{"namespace", `<authorizeRequest xmlns="urn://Ocpp/Cs/2015/10/"><idTag xmlns="urn://other">A</idTag></authorizeRequest>`},
		{"not one of", `<bootNotificationResponse xmlns="urn://Ocpp/Cs/2015/10/"><status>Fine</status><currentTime>2026-10-06T10:00:00Z</currentTime><interval>300</interval></bootNotificationResponse>`},
		{"xs:int", `<bootNotificationResponse xmlns="urn://Ocpp/Cs/2015/10/"><status>Accepted</status><currentTime>2026-10-06T10:00:00Z</currentTime><interval>3.5</interval></bootNotificationResponse>`},
		{"xs:dateTime", `<heartbeatResponse xmlns="urn://Ocpp/Cs/2015/10/"><currentTime>yesterday</currentTime></heartbeatResponse>`},
		{"longer than", `<authorizeRequest xmlns="urn://Ocpp/Cs/2015/10/"><idTag>ABCDEFGHIJKLMNOPQRSTU</idTag></authorizeRequest>`},
		{"fraction", `<setChargingProfileRequest xmlns="urn://Ocpp/Cp/2015/10/"><connectorId>1</connectorId><csChargingProfiles><chargingProfileId>1</chargingProfileId><stackLevel>0</stackLevel><chargingProfilePurpose>TxDefaultProfile</chargingProfilePurpose><chargingProfileKind>Absolute</chargingProfileKind><chargingSchedule><chargingRateUnit>A</chargingRateUnit><chargingSchedulePeriod><startPeriod>0</startPeriod><limit>16.25</limit></chargingSchedulePeriod></chargingSchedule></csChargingProfiles></setChargingProfileRequest>`},
		{"declares no", `<bootNotificationRequest xmlns="urn://Ocpp/Cp/2015/10/"/>`},
	}
	for _, tc := range bad {
		err := ocpp.CheckSOAPEnvelope([]byte(csEnvelope(tc.body)))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want an error with %q", tc.body, err, tc.want)
		}
	}
}

// Covers: SIG-IPE-6415
// Reading stays liberal: a request or an answer whose elements are not in
// the WSDL sequence (nested ones too) is still understood, although the
// strict check names the problem.
func TestOCPPSReadsAnyOrder(t *testing.T) {
	body := `<startTransactionRequest xmlns="urn://Ocpp/Cs/2015/10/"><timestamp>2026-10-06T10:00:00Z</timestamp><meterStart>100</meterStart><idTag>TAG1</idTag><connectorId>1</connectorId></startTransactionRequest>`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(csEnvelope(body)))
	q, f := ocpp.ReadSOAPRequest(req)
	if f != nil {
		t.Fatal(f.Reason)
	}
	if err := q.CheckWSDL(); err == nil || !strings.Contains(err.Error(), "out of order") {
		t.Errorf("strict check: %v", err)
	}
	pl, f := q.Payload("StartTransaction")
	if f != nil {
		t.Fatal(f.Reason)
	}
	var p map[string]any
	_ = json.Unmarshal(pl, &p)
	if p["connectorId"] != float64(1) || p["idTag"] != "TAG1" || p["meterStart"] != float64(100) || p["timestamp"] != "2026-10-06T10:00:00Z" {
		t.Errorf("payload %s", pl)
	}
	// An answer from a charger with idTagInfo and the top level out of
	// order.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><getConfigurationResponse xmlns="urn://Ocpp/Cp/2015/10/"><unknownKey>X</unknownKey><configurationKey><value>300</value><readonly>false</readonly><key>HeartbeatInterval</key></configurationKey></getConfigurationResponse></s:Body></s:Envelope>`)
	}))
	defer srv.Close()
	ans, err := ocpp.SOAPCall(context.Background(), srv.Client(), "CP1", "", srv.URL, "GetConfiguration", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ans), `"key":"HeartbeatInterval"`) || !strings.Contains(string(ans), `"readonly":false`) || !strings.Contains(string(ans), `"unknownKey":["X"]`) {
		t.Errorf("answer %s", ans)
	}
}
