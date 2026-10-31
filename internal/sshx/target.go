package sshx

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"git.juzo.io/juzo/hashi/internal/ipsets"
)

// ErrTargetDown: an SSH address set resolves to nothing or no address answers.
var ErrTargetDown = errors.New("the address set does not reach this host")

// AfterReconnect is a host's health after testing its connection (08 §4 step 5). A working login proves only that it can be
// reached: a failed validation or a failed apply stays as it was, because other checks rest on it. Only a connection
// failure can be cleared by a connection succeeding.
func AfterReconnect(health, lastError string, dial error) (string, string) {
	if dial != nil {
		return "failed", dial.Error()
	}
	if strings.HasPrefix(lastError, "connect to ") {
		return "healthy", ""
	}
	return health, lastError
}

// ProbeTarget checks that a set has an address with an open SSH port, without logging in or sending anything (08 §5).
func ProbeTarget(ctx context.Context, resolve func(ctx context.Context, consumer, setID string) (ipsets.Target, error), consumer, setID string, port int) error {
	t, err := resolve(ctx, consumer, setID)
	if err != nil {
		return err
	}
	if len(t.Addresses) == 0 {
		return fmt.Errorf("%w: %s", ErrTargetDown, t.Unresolved)
	}
	for _, a := range t.Addresses {
		dctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		c, err := (&net.Dialer{}).DialContext(dctx, "tcp", Addr(a.String(), port))
		cancel()
		if err == nil {
			_ = c.Close()
			return nil
		}
	}
	return ErrTargetDown
}
