package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

// softAuthenticator is a minimal WebAuthn authenticator for tests: one ES256 key, "none" attestation,
// user presence + verification always set. go-webauthn verifies everything it produces.
type softAuthenticator struct {
	t      *testing.T
	key    *ecdsa.PrivateKey
	credID []byte
	handle []byte
	count  uint32
}

func newSoftAuthenticator(t *testing.T) *softAuthenticator {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id := make([]byte, 16)
	_, _ = rand.Read(id)
	return &softAuthenticator{t: t, key: k, credID: id}
}

var b64 = base64.RawURLEncoding

const (
	flagUP = 0x01
	flagUV = 0x04
	flagAT = 0x40
)

func (a *softAuthenticator) clientData(typ string, challenge []byte, origin string) []byte {
	b, _ := json.Marshal(map[string]any{"type": typ, "challenge": b64.EncodeToString(challenge), "origin": origin, "crossOrigin": false})
	return b
}

func (a *softAuthenticator) authData(rpID string, flags byte, attested []byte) []byte {
	h := sha256.Sum256([]byte(rpID))
	a.count++
	out := append(h[:], flags)
	out = binary.BigEndian.AppendUint32(out, a.count)
	return append(out, attested...)
}

func (a *softAuthenticator) coseKey() []byte {
	enc, _ := cbor.CoreDetEncOptions().EncMode()
	pub := a.key.PublicKey
	x, y := make([]byte, 32), make([]byte, 32)
	pub.X.FillBytes(x) //nolint:staticcheck // test helper; ecdh form not needed
	pub.Y.FillBytes(y) //nolint:staticcheck // see above
	b, err := enc.Marshal(map[int]any{1: 2, 3: -7, -1: 1, -2: x, -3: y})
	if err != nil {
		a.t.Fatal(err)
	}
	return b
}

// register answers navigator.credentials.create for userHandle at rpID from origin.
func (a *softAuthenticator) register(rpID, origin string, challenge, userHandle []byte) []byte {
	a.handle = userHandle
	attested := make([]byte, 16) // zero AAGUID
	attested = binary.BigEndian.AppendUint16(attested, uint16(len(a.credID)))
	attested = append(attested, a.credID...)
	attested = append(attested, a.coseKey()...)
	enc, _ := cbor.CoreDetEncOptions().EncMode()
	att, err := enc.Marshal(map[string]any{"fmt": "none", "attStmt": map[string]any{}, "authData": a.authData(rpID, flagUP|flagUV|flagAT, attested)})
	if err != nil {
		a.t.Fatal(err)
	}
	b, _ := json.Marshal(map[string]any{
		"id": b64.EncodeToString(a.credID), "rawId": b64.EncodeToString(a.credID), "type": "public-key",
		"response": map[string]any{
			"clientDataJSON":    b64.EncodeToString(a.clientData("webauthn.create", challenge, origin)),
			"attestationObject": b64.EncodeToString(att),
			"transports":        []string{"internal"},
		},
	})
	return b
}

// assert answers navigator.credentials.get.
func (a *softAuthenticator) assert(rpID, origin string, challenge []byte) []byte {
	ad := a.authData(rpID, flagUP|flagUV, nil)
	cd := a.clientData("webauthn.get", challenge, origin)
	cdh := sha256.Sum256(cd)
	digest := sha256.Sum256(append(append([]byte{}, ad...), cdh[:]...))
	sig, err := ecdsa.SignASN1(rand.Reader, a.key, digest[:])
	if err != nil {
		a.t.Fatal(err)
	}
	b, _ := json.Marshal(map[string]any{
		"id": b64.EncodeToString(a.credID), "rawId": b64.EncodeToString(a.credID), "type": "public-key",
		"response": map[string]any{
			"clientDataJSON": b64.EncodeToString(cd), "authenticatorData": b64.EncodeToString(ad),
			"signature": b64.EncodeToString(sig), "userHandle": b64.EncodeToString(a.handle),
		},
	})
	return b
}
