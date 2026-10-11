// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package ocpp

import (
	"encoding/json"
	"reflect"
)

// ISO 15118 Plug & Charge over OCPP 1.6J, as the OCA application note
// "Using ISO 15118 Plug & Charge with OCPP 1.6" (v1.0, 2020-09-16) lays it
// out: nine messages shaped after their 2.0.1 namesakes but cut down
// (an IdToken without a type, a TriggerMessage without fields), each sent
// as a DataTransfer whose vendorId is PnC16VendorID, whose messageId is
// the message name and whose data is the message as a JSON string. The
// note shows the request only; the answer here travels the same way, as
// the data string of an Accepted DataTransfer confirmation, as the
// open implementations of the note do.
//
// The note's messages have no OCA schema; they are checked against the
// Go types below, as the 1.6 security extension messages are.

// PnC16VendorID is the DataTransfer vendorId of the application note.
const PnC16VendorID = "org.openchargealliance.iso15118pnc"

// PnC16 are the application note's messages by name: their request and
// confirmation types, and whether the charge point sends them (FromCP).
var PnC16 = map[string]PnC16Def{
	"Authorize":                  {Def[PnCAuthorize16, PnCAuthorize16Conf](), true},
	"CertificateSigned":          {Def[PnCCertificateSigned16, PnCStatusConf16](), false},
	"DeleteCertificate":          {Def[PnCDeleteCertificate16, PnCStatusConf16](), false},
	"Get15118EVCertificate":      {Def[PnCGet15118EVCertificate16, PnCGet15118EVCertificate16Conf](), true},
	"GetCertificateStatus":       {Def[PnCGetCertificateStatus16, PnCGetCertificateStatus16Conf](), true},
	"GetInstalledCertificateIds": {Def[PnCGetInstalledCertificateIDs16, PnCGetInstalledCertificateIDs16Conf](), false},
	"InstallCertificate":         {Def[PnCInstallCertificate16, PnCStatusConf16](), false},
	"SignCertificate":            {Def[PnCSignCertificate16, PnCStatusConf16](), true},
	"TriggerMessage":             {Def[Empty, PnCStatusConf16](), false},
}

// PnC16Def is one application note message.
type PnC16Def struct {
	MsgDef
	FromCP bool
}

// pncStatuses are the statuses each confirmation may carry.
var pncStatuses = map[string][]string{
	"CertificateSigned":  {"Accepted", "Rejected"},
	"DeleteCertificate":  {"Accepted", "Failed", "NotFound"},
	"InstallCertificate": {"Accepted", "Rejected", "Failed"},
	"SignCertificate":    {"Accepted", "Rejected"},
	"TriggerMessage":     {"Accepted", "Rejected", "NotImplemented"},
}

type PnCIDToken16 struct {
	IDToken string `json:"idToken" ocpp:"req,max=20"`
}

type PnCIDTokenInfo16 struct {
	Status              string `json:"status" ocpp:"req,enum=Accepted|Blocked|ConcurrentTx|Expired|Invalid"`
	CacheExpiryDateTime string `json:"cacheExpiryDateTime,omitempty" ocpp:"dt"`
}

type PnCOCSPRequestData16 struct {
	HashAlgorithm  string `json:"hashAlgorithm" ocpp:"req,enum=SHA256|SHA384|SHA512"`
	IssuerNameHash string `json:"issuerNameHash" ocpp:"req,max=128"`
	IssuerKeyHash  string `json:"issuerKeyHash" ocpp:"req,max=128"`
	SerialNumber   string `json:"serialNumber" ocpp:"req,max=40"`
	ResponderURL   string `json:"responderURL" ocpp:"req,max=512"`
}

type PnCCertificateHashData16 struct {
	HashAlgorithm  string `json:"hashAlgorithm" ocpp:"req,enum=SHA256|SHA384|SHA512"`
	IssuerNameHash string `json:"issuerNameHash" ocpp:"req,max=128"`
	IssuerKeyHash  string `json:"issuerKeyHash" ocpp:"req,max=128"`
	SerialNumber   string `json:"serialNumber" ocpp:"req,max=40"`
}

type PnCCertificateHashDataChain16 struct {
	CertificateType          string                     `json:"certificateType" ocpp:"req,enum=V2GRootCertificate|MORootCertificate|V2GCertificateChain"`
	CertificateHashData      PnCCertificateHashData16   `json:"certificateHashData" ocpp:"req"`
	ChildCertificateHashData []PnCCertificateHashData16 `json:"childCertificateHashData,omitempty" ocpp:"maxItems=4"`
}

type PnCAuthorize16 struct {
	Certificate                 string                 `json:"certificate,omitempty" ocpp:"max=5500"`
	IDToken                     PnCIDToken16           `json:"idToken" ocpp:"req"`
	ISO15118CertificateHashData []PnCOCSPRequestData16 `json:"iso15118CertificateHashData,omitempty" ocpp:"maxItems=4"`
}

type PnCAuthorize16Conf struct {
	CertificateStatus string           `json:"certificateStatus,omitempty" ocpp:"enum=Accepted|SignatureError|CertificateExpired|CertificateRevoked|NoCertificateAvailable|CertChainError|ContractCancelled"`
	IDTokenInfo       PnCIDTokenInfo16 `json:"idTokenInfo" ocpp:"req"`
}

type PnCCertificateSigned16 struct {
	CertificateChain string `json:"certificateChain" ocpp:"req,max=10000"`
}

type PnCDeleteCertificate16 struct {
	CertificateHashData PnCCertificateHashData16 `json:"certificateHashData" ocpp:"req"`
}

type PnCGet15118EVCertificate16 struct {
	ISO15118SchemaVersion string `json:"iso15118SchemaVersion" ocpp:"req,max=50"`
	Action                string `json:"action" ocpp:"req,enum=Install|Update"`
	EXIRequest            string `json:"exiRequest" ocpp:"req,max=5600"`
}

type PnCGet15118EVCertificate16Conf struct {
	Status      string `json:"status" ocpp:"req,enum=Accepted|Failed"`
	EXIResponse string `json:"exiResponse" ocpp:"req,max=5600"`
}

type PnCGetCertificateStatus16 struct {
	OCSPRequestData PnCOCSPRequestData16 `json:"ocspRequestData" ocpp:"req"`
}

type PnCGetCertificateStatus16Conf struct {
	Status     string `json:"status" ocpp:"req,enum=Accepted|Failed"`
	OCSPResult string `json:"ocspResult,omitempty" ocpp:"max=5500"`
}

type PnCGetInstalledCertificateIDs16 struct {
	CertificateType []string `json:"certificateType,omitempty"`
}

type PnCGetInstalledCertificateIDs16Conf struct {
	Status                   string                          `json:"status" ocpp:"req,enum=Accepted|NotFound"`
	CertificateHashDataChain []PnCCertificateHashDataChain16 `json:"certificateHashDataChain,omitempty"`
}

type PnCInstallCertificate16 struct {
	CertificateType string `json:"certificateType" ocpp:"req,enum=V2GRootCertificate|MORootCertificate"`
	Certificate     string `json:"certificate" ocpp:"req,max=5500"`
}

type PnCSignCertificate16 struct {
	CSR string `json:"csr" ocpp:"req,max=5500"`
}

// PnCStatusConf16 is a confirmation with a status only; PnC16Check
// checks the status against the message's values.
type PnCStatusConf16 struct {
	Status string `json:"status" ocpp:"req"`
}

// PnC16Check validates the payload of application note message name (a
// confirmation when response is set) and decodes it into its Go type.
func PnC16Check(name string, response bool, payload json.RawMessage) (any, *RPCError) {
	d, ok := PnC16[name]
	if !ok {
		return nil, RPCErrorf(KindNotImplemented, "%s is not a message of the Plug & Charge application note", name)
	}
	t := d.Req
	if response {
		t = d.Conf
	}
	v, verr := validate(t, payload, V16)
	if verr != nil {
		return nil, verr
	}
	if name == "GetInstalledCertificateIds" && !response {
		for _, ct := range v.(*PnCGetInstalledCertificateIDs16).CertificateType {
			if !contains([]string{"V2GRootCertificate", "MORootCertificate", "V2GCertificateChain"}, ct) {
				return nil, &RPCError{Kind: KindProperty, Desc: "certificateType: " + ct + " is not a value of the enumeration", Enum: true}
			}
		}
	}
	if st, ok := pncStatuses[name]; ok && response {
		s := reflect.ValueOf(v).Elem().FieldByName("Status").String()
		if !contains(st, s) {
			return nil, &RPCError{Kind: KindProperty, Desc: "status: " + s + " is not a value of the enumeration", Enum: true}
		}
	}
	return v, nil
}

// PnC16Wrap makes the DataTransfer request carrying application note
// message name with payload, checked first.
func PnC16Wrap(name string, payload any) (DataTransfer16, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return DataTransfer16{}, err
	}
	if _, verr := PnC16Check(name, false, b); verr != nil {
		return DataTransfer16{}, verr
	}
	return DataTransfer16{VendorID: PnC16VendorID, MessageID: name, Data: string(b)}, nil
}

// PnC16Unwrap reads a DataTransfer request: ok is false when it is not
// the application note's (another vendorId); otherwise the message name
// and its checked payload, or the error to answer with.
func PnC16Unwrap(dt json.RawMessage) (name string, payload json.RawMessage, ok bool, verr *RPCError) {
	var d DataTransfer16
	if err := json.Unmarshal(dt, &d); err != nil || d.VendorID != PnC16VendorID {
		return "", nil, false, nil
	}
	if d.Data == "" {
		d.Data = "{}"
	}
	if _, verr := PnC16Check(d.MessageID, false, json.RawMessage(d.Data)); verr != nil {
		return d.MessageID, nil, true, verr
	}
	return d.MessageID, json.RawMessage(d.Data), true, nil
}

// PnC16Answer makes the DataTransfer confirmation carrying the answer to
// message name, checked first.
func PnC16Answer(name string, answer any) (DataTransfer16Conf, error) {
	b, err := json.Marshal(answer)
	if err != nil {
		return DataTransfer16Conf{}, err
	}
	if _, verr := PnC16Check(name, true, b); verr != nil {
		return DataTransfer16Conf{}, verr
	}
	return DataTransfer16Conf{Status: "Accepted", Data: string(b)}, nil
}

// PnC16ReadAnswer reads the DataTransfer confirmation to message name: the
// checked answer, or ok false when the peer did not take the message
// (status other than Accepted: UnknownVendorId and the like, which the
// note reads as no Plug & Charge support).
func PnC16ReadAnswer(name string, conf json.RawMessage) (answer json.RawMessage, status string, ok bool, verr *RPCError) {
	var c DataTransfer16Conf
	if err := json.Unmarshal(conf, &c); err != nil {
		return nil, "", false, RPCErrorf(KindFormat, "the confirmation is not a DataTransfer confirmation")
	}
	if c.Status != "Accepted" {
		return nil, c.Status, false, nil
	}
	if _, verr := PnC16Check(name, true, json.RawMessage(c.Data)); verr != nil {
		return nil, c.Status, true, verr
	}
	return json.RawMessage(c.Data), c.Status, true, nil
}
