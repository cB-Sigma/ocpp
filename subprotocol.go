package ocpp

import (
	"net/http"
	"slices"
	"strings"
)

// Subprotocol leniency. OCPP-J names the version in the WebSocket
// subprotocol (ocpp1.6, ocpp2.0.1, ocpp2.1). Chargers in the field also
// send spellings such as "ocpp1.6j" or "OCPP1.6", and some send no
// subprotocol at all. Subprotocols lists the spellings to accept and
// VersionOf says which version an accepted one stands for, so a listener
// can take a variant spelling of an enabled version as that version
// (echoed back as the charger spelled it) and decide what to do with a
// charger that offers none.

// subprotocolVariants are the spellings seen in the field, by version.
var subprotocolVariants = map[string][]string{
	V16:  {"ocpp1.6j", "ocpp1.6J", "OCPP1.6", "OCPP1.6J", "OCPP1.6j", "Ocpp1.6", "ocpp16", "ocpp1.6-json"},
	V201: {"OCPP2.0.1", "ocpp2.0.1j", "ocpp2.0.1J", "OCPP2.0.1J", "ocpp201"},
	V21:  {"OCPP2.1", "ocpp2.1j", "ocpp2.1J", "OCPP2.1J", "ocpp21"},
}

// Subprotocols are the names a listener for versions accepts: the
// versions first, then their variant spellings.
func Subprotocols(versions []string) []string {
	out := append([]string(nil), versions...)
	for _, v := range versions {
		out = append(out, subprotocolVariants[v]...)
	}
	return out
}

// VersionOf is the version a subprotocol name stands for, or "".
func VersionOf(name string) string {
	for v, names := range subprotocolVariants {
		if name == v || slices.Contains(names, name) {
			return v
		}
	}
	return ""
}

// OfferedSubprotocols are the subprotocols a WebSocket upgrade request offers.
func OfferedSubprotocols(r *http.Request) []string {
	var out []string
	for _, h := range r.Header.Values("Sec-WebSocket-Protocol") {
		for _, p := range strings.Split(h, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}
