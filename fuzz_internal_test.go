package ocpp

import (
	"encoding/json"
	"testing"
)

// Covers: SIG-IPE-261
func FuzzFrameAndPayload(f *testing.F) {
	f.Add([]byte(`[2,"1","BootNotification",{"chargePointVendor":"v","chargePointModel":"m"}]`))
	f.Add([]byte(`[2,"2","TransactionEvent",{"eventType":"Started","timestamp":"2026-09-24T10:00:00Z","triggerReason":"Authorized","seqNo":0,"transactionInfo":{"transactionId":"t"}}]`))
	f.Add([]byte(`[4,"3","GenericError","",{}]`))
	f.Fuzz(func(t *testing.T, b []byte) {
		for _, v := range []string{V16, V201, V21} {
			_, _, _ = parseFrame(b, v) // never panics
		}
		if fr, _, err := parseFrame(b, V21); err == nil && (fr.typ == msgCall || fr.typ == msgSend) {
			_, _ = validate21(fr.action, false, fr.payload)
		}
		fr, _, err := parseFrame(b, V201)
		if err != nil || fr.typ != msgCall {
			return
		}
		for _, v := range []string{V16, V201} {
			defs := v16In
			if v == V201 {
				defs = v201In
			}
			if md, ok := defs[fr.action]; ok {
				if res, verr := validate(md.Req, fr.payload, v); verr == nil {
					if _, err := json.Marshal(res); err != nil {
						t.Fatalf("a validated payload does not encode: %v", err)
					}
				}
			}
		}
	})
}
