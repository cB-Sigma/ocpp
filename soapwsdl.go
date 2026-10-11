// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package ocpp

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// OCPP-S 1.6 message types as the two embedded OCA WSDLs declare them.
// The WSDL files are read when OCPP-S is first used (never changed, never
// re-typed): each complexType's xs:sequence gives the order elements are
// written in, and the sequences with the simple types (enumerations,
// maxLength, fractionDigits on string, int, decimal, dateTime, boolean and
// anyURI) let a strict receiver check a message as a schema-validating
// SOAP stack would. The WSDLs use only that part of XML Schema; anything
// else in them is an error when they are read, so a different file cannot
// be half understood.

const xsdNS = "http://www.w3.org/2001/XMLSchema"

// wsdlElem is one element of a sequence.
type wsdlElem struct {
	name     string
	typ      string // local name of a complex or simple type, or "xs:" + a built-in
	min, max int    // max -1: unbounded
}

// wsdlSimple is a simple type: a restriction of a built-in.
type wsdlSimple struct {
	base      string // built-in local name
	enum      []string
	maxLength int // -1: none
	fraction  int // fractionDigits, -1: none
}

// wsdlTypes is the schema of one WSDL (one service, one namespace).
type wsdlTypes struct {
	ns      string
	top     map[string]string // global element -> complex type
	complex map[string][]wsdlElem
	simple  map[string]*wsdlSimple
}

var (
	wsdlOnce sync.Once
	wsdlByNS map[string]*wsdlTypes
	wsdlErr  error
)

// wsdlFiles are the OCA WSDLs of OCPP-S 1.6 in oca/1.6/wsdl.
var wsdlFiles = []string{"OCPP_CentralSystemService_1.6.wsdl", "OCPP_ChargePointService_1.6.wsdl"}

// soapTypes is the schema of the service with namespace ns (csNS, cpNS).
func soapTypes(ns string) (*wsdlTypes, error) {
	wsdlOnce.Do(func() {
		wsdlByNS = map[string]*wsdlTypes{}
		for _, f := range wsdlFiles {
			raw, err := fs.ReadFile(ocaFS, path.Join("oca", "1.6", "wsdl", f))
			if err != nil {
				wsdlErr = err
				return
			}
			w, err := parseWSDL(raw)
			if err != nil {
				wsdlErr = fmt.Errorf("%s: %w", f, err)
				return
			}
			wsdlByNS[w.ns] = w
		}
	})
	if wsdlErr != nil {
		return nil, wsdlErr
	}
	w := wsdlByNS[ns]
	if w == nil {
		return nil, fmt.Errorf("no OCPP-S 1.6 WSDL has namespace %s", ns)
	}
	return w, nil
}

// parseWSDL reads the xs:schema in a WSDL's types.
func parseWSDL(raw []byte) (*wsdlTypes, error) {
	w := &wsdlTypes{top: map[string]string{}, complex: map[string][]wsdlElem{}, simple: map[string]*wsdlSimple{}}
	d := xml.NewDecoder(bytes.NewReader(raw))
	// Prefixes are resolved through the declarations in scope, so a type
	// reference is either the target namespace or XML Schema.
	type frame struct {
		name   string
		prefix map[string]string
	}
	stack := []frame{{prefix: map[string]string{}}}
	resolve := func(q string) (string, error) {
		pfx, loc, ok := strings.Cut(q, ":")
		if !ok {
			pfx, loc = "", q
		}
		ns := ""
		for i := len(stack) - 1; i >= 0 && ns == ""; i-- {
			ns = stack[i].prefix[pfx]
		}
		switch ns {
		case xsdNS:
			return "xs:" + loc, nil
		case w.ns:
			return loc, nil
		}
		return "", fmt.Errorf("type %s is not in XML Schema or %s", q, w.ns)
	}
	attr := func(se xml.StartElement, name string) string {
		for _, a := range se.Attr {
			if a.Name.Space == "" && a.Name.Local == name {
				return a.Value
			}
		}
		return ""
	}
	occurs := func(v string, def int) (int, error) {
		if v == "" {
			return def, nil
		}
		if v == "unbounded" {
			return -1, nil
		}
		return strconv.Atoi(v)
	}
	inSchema := false
	ct, st := "", "" // complexType, simpleType being read
	for {
		tok, err := d.RawToken()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		switch x := tok.(type) {
		case xml.StartElement:
			f := frame{name: x.Name.Local, prefix: map[string]string{}}
			for _, a := range x.Attr {
				switch {
				case a.Name.Space == "xmlns":
					f.prefix[a.Name.Local] = a.Value
				case a.Name.Space == "" && a.Name.Local == "xmlns":
					f.prefix[""] = a.Value
				}
			}
			stack = append(stack, f)
			ns := ""
			for i := len(stack) - 1; i >= 0 && ns == ""; i-- {
				ns = stack[i].prefix[x.Name.Space]
			}
			if ns != xsdNS {
				if inSchema {
					return nil, fmt.Errorf("unexpected element %s in the schema", x.Name.Local)
				}
				continue
			}
			parent := stack[len(stack)-2].name
			switch x.Name.Local {
			case "schema":
				inSchema = true
				w.ns = attr(x, "targetNamespace")
				if attr(x, "elementFormDefault") != "qualified" {
					return nil, errors.New("elements are not qualified")
				}
			case "annotation":
				if err := skipRaw(d); err != nil {
					return nil, err
				}
				stack = stack[:len(stack)-1]
			case "complexType":
				ct = attr(x, "name")
				w.complex[ct] = []wsdlElem{}
			case "sequence":
				if parent != "complexType" {
					return nil, errors.New("a sequence outside a complexType")
				}
			case "element":
				typ, err := resolve(attr(x, "type"))
				if err != nil {
					return nil, err
				}
				switch {
				case parent == "schema":
					w.top[attr(x, "name")] = typ
				case parent == "sequence" && ct != "":
					e := wsdlElem{name: attr(x, "name"), typ: typ}
					if e.min, err = occurs(attr(x, "minOccurs"), 1); err != nil {
						return nil, err
					}
					if e.max, err = occurs(attr(x, "maxOccurs"), 1); err != nil {
						return nil, err
					}
					w.complex[ct] = append(w.complex[ct], e)
				default:
					return nil, fmt.Errorf("element %s in %s", attr(x, "name"), parent)
				}
			case "simpleType":
				st = attr(x, "name")
				w.simple[st] = &wsdlSimple{maxLength: -1, fraction: -1}
			case "restriction":
				base, err := resolve(attr(x, "base"))
				if err != nil || st == "" || !strings.HasPrefix(base, "xs:") {
					return nil, fmt.Errorf("restriction of %q in %q", attr(x, "base"), st)
				}
				w.simple[st].base = strings.TrimPrefix(base, "xs:")
			case "enumeration":
				w.simple[st].enum = append(w.simple[st].enum, attr(x, "value"))
			case "maxLength":
				if w.simple[st].maxLength, err = strconv.Atoi(attr(x, "value")); err != nil {
					return nil, err
				}
			case "fractionDigits":
				if w.simple[st].fraction, err = strconv.Atoi(attr(x, "value")); err != nil {
					return nil, err
				}
			default:
				return nil, fmt.Errorf("XML Schema %s is not read", x.Name.Local)
			}
		case xml.EndElement:
			switch stack[len(stack)-1].name {
			case "schema":
				inSchema = false
			case "complexType":
				ct = ""
			case "simpleType":
				st = ""
			}
			stack = stack[:len(stack)-1]
		}
	}
	if w.ns == "" || len(w.top) == 0 {
		return nil, errors.New("no schema")
	}
	// Every type referred to is declared (the global elements include
	// the chargeBoxIdentity header, a simple type).
	known := func(typ string) bool {
		_, c := w.complex[typ]
		_, s := w.simple[typ]
		return c || s || strings.HasPrefix(typ, "xs:")
	}
	for el, typ := range w.top {
		if !known(typ) {
			return nil, fmt.Errorf("element %s has an undeclared type %s", el, typ)
		}
	}
	for ct, seq := range w.complex {
		for _, e := range seq {
			if !known(e.typ) {
				return nil, fmt.Errorf("%s/%s has an undeclared type %s", ct, e.name, e.typ)
			}
		}
	}
	return w, nil
}

// skipRaw reads past the end of the element just started.
func skipRaw(d *xml.Decoder) error {
	for depth := 1; depth > 0; {
		tok, err := d.RawToken()
		if err != nil {
			return err
		}
		switch tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		}
	}
	return nil
}

// typeOf is the complex type of global element el, nil when there is
// none.
func (w *wsdlTypes) typeOf(el string) []wsdlElem {
	if w == nil {
		return nil
	}
	return w.complex[w.top[el]]
}

// ---- writing in sequence order --------------------------------------------------------

// toXMLSeq writes a JSON object's properties as elements: those the WSDL
// sequence seq declares first, in its order, then the others the JSON
// schema s knows in name order (deterministic; a strict peer may refuse
// them, but the WSDL and the 1.6 JSON schemas declare the same names, so
// this only happens to a property the WSDL lacks). Properties neither
// knows are not written. Nested objects follow their own WSDL types.
func toXMLSeq(b *bytes.Buffer, v any, w *wsdlTypes, seq []wsdlElem, s *oschema) {
	m, ok := v.(map[string]any)
	if !ok {
		return
	}
	done := map[string]bool{}
	for _, e := range seq {
		x, ok := m[e.name]
		if !ok {
			continue
		}
		done[e.name] = true
		var ps *oschema
		if s != nil {
			ps = s.props[e.name]
		}
		writeElems(b, e.name, x, w, w.complex[e.typ], ps)
	}
	if s == nil {
		return
	}
	var rest []string
	for k := range m {
		if !done[k] && s.props[k] != nil {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	for _, k := range rest {
		writeElems(b, k, m[k], w, nil, s.props[k])
	}
}

// writeElems writes x as element name (once per item for an array).
func writeElems(b *bytes.Buffer, name string, x any, w *wsdlTypes, seq []wsdlElem, ps *oschema) {
	vals := []any{x}
	if l, ok := x.([]any); ok {
		vals = l
		if ps != nil && ps.items != nil {
			ps = ps.items
		}
	}
	for _, e := range vals {
		b.WriteString("<" + name + ">")
		switch t := e.(type) {
		case map[string]any:
			toXMLSeq(b, t, w, seq, ps)
		default:
			_ = xml.EscapeText(b, []byte(scalar(t)))
		}
		b.WriteString("</" + name + ">")
	}
}

// ---- checking a received message ---------------------------------------------------

// CheckSOAPEnvelope checks the one element in a SOAP envelope's Body
// against the embedded OCPP-S 1.6 WSDL of its namespace, as a
// schema-validating SOAP stack would: the element is declared, its
// children are in the WSDL's sequence order and namespace, required
// elements are there and none repeats more than allowed, and values have
// their simple type (enumeration, length, int, decimal digits, dateTime,
// boolean, URI). A SOAP Fault passes. SigmaM2M's CSMS does not call it
// (it reads any order); the simulator does unless it is lenient.
func CheckSOAPEnvelope(raw []byte) error {
	var env soapEnvelope
	if err := xml.Unmarshal(raw, &env); err != nil || env.XMLName.Space != soapEnvNS {
		return errors.New("not a SOAP 1.2 envelope")
	}
	if len(env.Body.Children) != 1 {
		return errors.New("the body does not hold one element")
	}
	return checkSOAPElement(&env.Body.Children[0])
}

// CheckWSDL checks the request's body element as CheckSOAPEnvelope does.
func (q *SOAPRequest) CheckWSDL() error {
	if len(q.env.Body.Children) != 1 {
		return errors.New("the body does not hold one element")
	}
	return checkSOAPElement(&q.env.Body.Children[0])
}

func checkSOAPElement(el *xnode) error {
	if el.XMLName.Space == soapEnvNS && el.XMLName.Local == "Fault" {
		return nil
	}
	w, err := soapTypes(el.XMLName.Space)
	if err != nil {
		return err
	}
	typ, ok := w.top[el.XMLName.Local]
	if !ok {
		return fmt.Errorf("the WSDL of %s declares no element %s", w.ns, el.XMLName.Local)
	}
	var errs []string
	w.check(el, el.XMLName.Local, typ, &errs)
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func (w *wsdlTypes) check(n *xnode, where, typ string, errs *[]string) {
	if seq, ok := w.complex[typ]; ok {
		if strings.TrimSpace(n.Text) != "" {
			*errs = append(*errs, where+" holds text")
		}
		i, count := 0, 0
		for c := range n.Children {
			ch := &n.Children[c]
			name := ch.XMLName.Local
			if ch.XMLName.Space != w.ns {
				*errs = append(*errs, fmt.Sprintf("%s/%s is in namespace %q, not %s", where, name, ch.XMLName.Space, w.ns))
			}
			// Move along the sequence to the child's element.
			j := i
			for j < len(seq) && seq[j].name != name {
				j++
			}
			if j == len(seq) {
				if declared(seq, name) {
					*errs = append(*errs, fmt.Sprintf("%s: %s is out of order (the WSDL sequence is %s)", where, name, seqNames(seq)))
				} else {
					*errs = append(*errs, fmt.Sprintf("%s: %s is not declared", where, name))
				}
				return
			}
			for ; i < j; i, count = i+1, 0 {
				if count < seq[i].min {
					*errs = append(*errs, fmt.Sprintf("%s: %s is missing before %s", where, seq[i].name, name))
				}
			}
			count++
			if seq[i].max >= 0 && count > seq[i].max {
				*errs = append(*errs, fmt.Sprintf("%s: %s occurs more than %d times", where, name, seq[i].max))
			}
			w.check(ch, where+"/"+name, seq[i].typ, errs)
		}
		for ; i < len(seq); i, count = i+1, 0 {
			if count < seq[i].min {
				*errs = append(*errs, fmt.Sprintf("%s: %s is missing", where, seq[i].name))
			}
		}
		return
	}
	if len(n.Children) > 0 {
		*errs = append(*errs, where+" has child elements")
		return
	}
	if err := w.checkValue(n.Text, typ); err != nil {
		*errs = append(*errs, fmt.Sprintf("%s: %v", where, err))
	}
}

func declared(seq []wsdlElem, name string) bool {
	for _, e := range seq {
		if e.name == name {
			return true
		}
	}
	return false
}

func seqNames(seq []wsdlElem) string {
	n := make([]string, len(seq))
	for i, e := range seq {
		n[i] = e.name
	}
	return strings.Join(n, ", ")
}

var (
	xsDecimal  = regexp.MustCompile(`^[+-]?(\d+(\.\d*)?|\.\d+)$`)
	xsDateTime = regexp.MustCompile(`^-?\d{4,}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})?$`)
)

func (w *wsdlTypes) checkValue(text, typ string) error {
	base := strings.TrimPrefix(typ, "xs:")
	var st *wsdlSimple
	if !strings.HasPrefix(typ, "xs:") {
		st = w.simple[typ]
		if st == nil {
			return fmt.Errorf("type %s is not declared", typ)
		}
		base = st.base
	}
	// string keeps white space; the other built-ins collapse it.
	v := text
	if base != "string" {
		v = strings.TrimSpace(text)
	}
	switch base {
	case "string":
	case "int":
		if _, err := strconv.ParseInt(v, 10, 32); err != nil {
			return fmt.Errorf("%q is not an xs:int", v)
		}
	case "decimal":
		if !xsDecimal.MatchString(v) {
			return fmt.Errorf("%q is not an xs:decimal", v)
		}
		if st != nil && st.fraction >= 0 {
			if _, frac, ok := strings.Cut(v, "."); ok && len(strings.TrimRight(frac, "0")) > st.fraction {
				return fmt.Errorf("%q has more than %d fraction digits", v, st.fraction)
			}
		}
	case "dateTime":
		if !xsDateTime.MatchString(v) {
			return fmt.Errorf("%q is not an xs:dateTime", v)
		}
		probe := v
		if !strings.HasSuffix(probe, "Z") && !strings.ContainsAny(probe[len(probe)-6:], "+-") {
			probe += "Z"
		}
		if _, err := time.Parse(time.RFC3339Nano, strings.TrimPrefix(probe, "-")); err != nil {
			return fmt.Errorf("%q is not an xs:dateTime", v)
		}
	case "boolean":
		switch v {
		case "true", "false", "1", "0":
		default:
			return fmt.Errorf("%q is not an xs:boolean", v)
		}
	case "anyURI":
		if _, err := url.Parse(v); err != nil {
			return fmt.Errorf("%q is not an xs:anyURI", v)
		}
	default:
		return fmt.Errorf("built-in type %s is not checked", base)
	}
	if st == nil {
		return nil
	}
	if len(st.enum) > 0 {
		ok := false
		for _, e := range st.enum {
			if v == e {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("%q is not one of %s", v, strings.Join(st.enum, ", "))
		}
	}
	if st.maxLength >= 0 && utf8.RuneCountInString(v) > st.maxLength {
		return fmt.Errorf("%q is longer than %d", v, st.maxLength)
	}
	return nil
}
