// Package secrets holds the types that keep secret material out of logs and API responses.
package secrets

import (
	"encoding/json"
	"fmt"
	"log/slog"
)

const redacted = "[redacted]"

// Value wraps secret material. Every rendering path (fmt verbs, slog, JSON, text) prints
// "[redacted]"; only Reveal returns the real bytes.
type Value struct {
	b []byte
}

// New copies s into a Value.
func New(s string) Value { return Value{b: []byte(s)} }

// Reveal returns the secret. Call it only at the point of use (an HTTP header, a cipher).
func (v Value) Reveal() string { return string(v.b) }

// IsZero reports whether no secret is set.
func (v Value) IsZero() bool { return len(v.b) == 0 }

func (Value) String() string               { return redacted }
func (Value) GoString() string             { return redacted }
func (Value) LogValue() slog.Value         { return slog.StringValue(redacted) }
func (Value) MarshalJSON() ([]byte, error) { return json.Marshal(redacted) }
func (Value) MarshalText() ([]byte, error) { return []byte(redacted), nil }

// Format covers %v, %+v, %#v, %s, %q, %x so no verb can print the secret.
func (Value) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte(redacted)) }
