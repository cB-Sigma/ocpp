package ocpp

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"

	jose "github.com/go-jose/go-jose/v4"
)

// Signed messages (OCPP-J 2.x part 4 clause 7): a message's payload is
// wrapped in a JWS in the flattened JSON serialization and its action gets
// the suffix "-Signed". The protected header carries OCPPAction (the
// action) and OCPPMessageTypedId (the message type number) and, as
// recommended, x5t#S256 (the SHA-256 of the signing certificate's DER).
// Algorithms: ES256, RS256, RS384 (clause 7.3).
//
// A signed message is verified when its x5t#S256 names a trusted
// certificate (one of the trusted ones, or our own signing certificate),
// then processed as the plain message (clause 7.2); a signature that does
// not verify is a SecurityError, and so is an unknown key when the signer
// requires known keys. A CALLRESULT with a JWS payload (a station
// answering a signed request) is verified and unwrapped the same way.

// MsgSigner signs and verifies signed messages. A nil *MsgSigner verifies
// nothing (Open passes payloads through unverified) and cannot sign.
type MsgSigner struct {
	signer  jose.Signer
	alg     jose.SignatureAlgorithm
	key     crypto.Signer
	thumb   string // base64url SHA-256 of our certificate
	trusted map[string]crypto.PublicKey
	require bool

	// Leaf is our signing certificate, when one is configured.
	Leaf *x509.Certificate
	// Trusted are the certificates whose signatures are verified, with
	// the index of the PEM text they came from.
	Trusted []TrustedCert
}

// TrustedCert is a certificate whose signatures a MsgSigner verifies.
type TrustedCert struct {
	Index int // of the PEM text in NewMsgSigner's trusted
	Cert  *x509.Certificate
}

var signedAlgs = []jose.SignatureAlgorithm{jose.ES256, jose.RS256, jose.RS384}

func thumbprint(c *x509.Certificate) string {
	s := sha256.Sum256(c.Raw)
	return base64.RawURLEncoding.EncodeToString(s[:])
}

// NewMsgSigner makes a signer from our signing certificate and key (PEM,
// both empty for a verify-only signer) and PEM texts of certificates whose
// signatures are verified; require refuses signed messages from unknown
// keys. An EC signing key must be P-256 (ES256); an RSA key signs RS256.
func NewMsgSigner(certPEM, keyPEM []byte, trusted [][]byte, require bool) (*MsgSigner, error) {
	m := &MsgSigner{trusted: map[string]crypto.PublicKey{}, require: require}
	for i, b := range trusted {
		for {
			var blk *pem.Block
			blk, b = pem.Decode(b)
			if blk == nil {
				break
			}
			c, err := x509.ParseCertificate(blk.Bytes)
			if err != nil {
				return nil, fmt.Errorf("trusted[%d]: %w", i, err)
			}
			m.trusted[thumbprint(c)] = c.PublicKey
			m.Trusted = append(m.Trusted, TrustedCert{Index: i, Cert: c})
		}
	}
	if len(certPEM) > 0 || len(keyPEM) > 0 {
		pair, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			return nil, err
		}
		leaf, err := x509.ParseCertificate(pair.Certificate[0])
		if err != nil {
			return nil, err
		}
		key, ok := pair.PrivateKey.(crypto.Signer)
		if !ok {
			return nil, errors.New("the signing key cannot sign")
		}
		switch k := key.(type) {
		case *ecdsa.PrivateKey:
			if k.Curve.Params().Name != "P-256" {
				return nil, errors.New("an EC signing key is P-256 (ES256)")
			}
			m.alg = jose.ES256
		case *rsa.PrivateKey:
			m.alg = jose.RS256
		default:
			return nil, errors.New("the signing key is not ECDSA P-256 or RSA")
		}
		m.key, m.thumb = key, thumbprint(leaf)
		m.trusted[m.thumb] = leaf.PublicKey
		m.Leaf = leaf
	}
	return m, nil
}

// Sign wraps a payload for action (message type typ).
func (m *MsgSigner) Sign(action string, typ int, payload []byte) (json.RawMessage, error) {
	if m == nil || m.key == nil {
		return nil, errors.New("no signing key is configured")
	}
	opts := (&jose.SignerOptions{}).WithHeader("OCPPAction", action).WithHeader("OCPPMessageTypedId", typ).WithHeader("x5t#S256", m.thumb)
	s, err := jose.NewSigner(jose.SigningKey{Algorithm: m.alg, Key: m.key}, opts)
	if err != nil {
		return nil, err
	}
	jws, err := s.Sign(payload)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(jws.FullSerialize()), nil
}

// IsJWS reports whether a payload is a flattened JWS.
func IsJWS(p json.RawMessage) bool {
	var m map[string]json.RawMessage
	if json.Unmarshal(p, &m) != nil {
		return false
	}
	_, a := m["signature"]
	_, b := m["protected"]
	_, c := m["payload"]
	return a && b && c && len(m) <= 4
}

// Open verifies a signed payload and returns the plain one; action is the
// expected OCPPAction ("" to skip the check).
func (m *MsgSigner) Open(action string, p json.RawMessage) (json.RawMessage, *RPCError) {
	jws, err := jose.ParseSigned(string(p), signedAlgs)
	if err != nil || len(jws.Signatures) != 1 {
		return nil, RPCErrorf(KindFormat, "the signed payload is not a flattened JWS with an allowed algorithm")
	}
	h := jws.Signatures[0].Protected
	if action != "" {
		if a, _ := h.ExtraHeaders["OCPPAction"].(string); a != action {
			return nil, RPCErrorf(KindSecurity, "the signed payload names action %q", a)
		}
	}
	thumb, _ := h.ExtraHeaders["x5t#S256"].(string)
	var key crypto.PublicKey
	if m != nil {
		key = m.trusted[thumb]
	}
	if key == nil {
		if m != nil && m.require {
			return nil, RPCErrorf(KindSecurity, "the signing key is not trusted")
		}
		// Not verifiable: processed as the plain message (clause 7.2
		// makes verification optional).
		return json.RawMessage(jws.UnsafePayloadWithoutVerification()), nil
	}
	out, err := jws.Verify(key)
	if err != nil {
		return nil, RPCErrorf(KindSecurity, "the signature does not verify")
	}
	return json.RawMessage(out), nil
}

// SignedSuffix is appended to the action of a signed message.
const SignedSuffix = "-Signed"

// BaseAction is the action of a possibly signed message without the
// suffix, and whether it had it.
func BaseAction(a string) (string, bool) { return strings.CutSuffix(a, SignedSuffix) }
