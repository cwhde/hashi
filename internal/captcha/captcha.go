// Package captcha checks Cap tokens (06 §11) and issues the clearance cookie a solved `pass_captcha` gives
// (06 §3.3, scope service). Cap's answers are in testdata/contracts/cap/siteverify.txt.
package captcha

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.juzo.io/juzo/hashi/internal/secrets"
)

// Result of one check.
type Result int

const (
	// NotSolved: Cap answered and the token is not good (wrong, used, another site key, malformed).
	NotSolved Result = iota
	Solved
	// Unavailable: Cap did not answer (connection error, timeout, unreadable answer).
	Unavailable
)

// ErrUnavailable wraps why Cap could not be asked.
var ErrUnavailable = errors.New("the Cap server is unavailable")

// Verify asks Cap whether token solves a challenge of siteKey: `POST <base>/<site key>/siteverify`. Only `200` with
// `success:true` is solved; any other answer is not (Cap uses 400, 403, 404 and 500 for failures, finding 1). A token is
// single use (finding 2).
func Verify(ctx context.Context, client *http.Client, base, siteKey string, secret secrets.Value, token string) (Result, error) {
	body, err := json.Marshal(map[string]string{"secret": secret.Reveal(), "response": token})
	if err != nil {
		return Unavailable, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(base, "/")+"/"+siteKey+"/siteverify",
		bytes.NewReader(body))
	if err != nil {
		return Unavailable, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return Unavailable, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return Unavailable, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	if json.Unmarshal(b, &out) != nil {
		return Unavailable, fmt.Errorf("%w: HTTP %d, not JSON", ErrUnavailable, resp.StatusCode)
	}
	if resp.StatusCode == http.StatusOK && out.Success {
		return Solved, nil
	}
	// Cap's "Invalid site key or secret" on a well-formed request means Hashi's settings are wrong, not the visitor.
	return NotSolved, fmt.Errorf("the Cap server refused the token: HTTP %d %s", resp.StatusCode, out.Error)
}

// Test checks the settings without a visitor (06 §11 "test verification"): a well-formed token Cap does not know is
// "Token not found" (404) when the site key and secret are right, and "Invalid site key or secret" otherwise. Cap
// checks the secret before the token (finding 2).
func Test(ctx context.Context, client *http.Client, base, siteKey string, secret secrets.Value) error {
	r, err := Verify(ctx, client, base, siteKey, secret, siteKey+":hashi-test:0")
	switch {
	case r == Unavailable:
		return err
	case r == Solved:
		return errors.New("the Cap server accepted a made-up token")
	case err != nil && strings.Contains(err.Error(), "Token not found"):
		return nil
	}
	return err
}

// CookiePrefix names a service's clearance cookie: one per service, so a solve for one never clears another.
const CookiePrefix = "hashi_cap_"

// CookieName is the clearance cookie of serviceID.
func CookieName(serviceID string) string { return CookiePrefix + strings.ToLower(serviceID) }

// Clearance signs "<service>|<expiry unix>" with key.
func Clearance(key []byte, serviceID string, until time.Time) string {
	msg := serviceID + "|" + strconv.FormatInt(until.Unix(), 10)
	m := hmac.New(sha256.New, key)
	m.Write([]byte(msg))
	return base64.RawURLEncoding.EncodeToString([]byte(msg)) + "." + base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// Cleared reports whether value is a valid, unexpired clearance of serviceID.
func Cleared(key []byte, serviceID, value string, now time.Time) bool {
	enc, sig, ok := strings.Cut(value, ".")
	if !ok {
		return false
	}
	msg, err1 := base64.RawURLEncoding.DecodeString(enc)
	got, err2 := base64.RawURLEncoding.DecodeString(sig)
	if err1 != nil || err2 != nil {
		return false
	}
	m := hmac.New(sha256.New, key)
	m.Write(msg)
	if !hmac.Equal(got, m.Sum(nil)) {
		return false
	}
	id, exp, ok := strings.Cut(string(msg), "|")
	until, err := strconv.ParseInt(exp, 10, 64)
	return ok && err == nil && id == serviceID && now.Unix() < until
}
