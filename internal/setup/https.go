package setup

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"time"
)

// HTTPSNonceTTL is how long step 8's link stays valid (02 §1 step 8).
const HTTPSNonceTTL = 5 * time.Minute

// ErrNonce means the step 8 link is unknown, used or expired.
var ErrNonce = errors.New("this HTTPS link is unknown, used or expired; start step 8 again")

type httpsNonce struct {
	value   string
	expires time.Time
}

// NewHTTPSNonce issues the one-time nonce for step 8's link. Only the latest one is valid.
func (s *Service) NewHTTPSNonce() (string, time.Time, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", time.Time{}, err
	}
	n := httpsNonce{value: base64.RawURLEncoding.EncodeToString(b), expires: s.now().Add(HTTPSNonceTTL)}
	s.mu.Lock()
	s.https = n
	s.mu.Unlock()
	return n.value, n.expires, nil
}

// ConsumeHTTPSNonce accepts the latest nonce once, before it expires.
func (s *Service) ConsumeHTTPSNonce(value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := s.https
	if n.value == "" || value == "" || s.now().After(n.expires) ||
		subtle.ConstantTimeCompare([]byte(n.value), []byte(value)) != 1 {
		return ErrNonce
	}
	s.https = httpsNonce{}
	return nil
}
