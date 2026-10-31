package sshx

import (
	"errors"
	"testing"
)

// A reconnect cannot turn a failed validation healthy: ownership confirmation rests on that state.
func TestAfterReconnect(t *testing.T) {
	for _, c := range []struct {
		health, msg string
		dial        error
		wantH, want string
	}{
		{"failed", "nft is missing", nil, "failed", "nft is missing"},
		{"degraded", "apply failed", nil, "degraded", "apply failed"},
		{"healthy", "", nil, "healthy", ""},
		{"failed", "connect to a (10.0.0.1:22): refused", nil, "healthy", ""},
		{"healthy", "", errors.New("connect to a: refused"), "failed", "connect to a: refused"},
	} {
		if h, m := AfterReconnect(c.health, c.msg, c.dial); h != c.wantH || m != c.want {
			t.Errorf("%+v: %s %q", c, h, m)
		}
	}
}
