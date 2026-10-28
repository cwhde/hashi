package certs

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"time"
)

// ErrNotPropagated means a challenge value did not show up on every server in time.
var ErrNotPropagated = errors.New("the challenge value is not visible yet")

// LookupTXT asks one DNS server (host or host:port, port 53 by default) for the TXT values at fqdn. It always uses TCP:
// answers are never truncated, and it works through port forwards that carry only TCP.
func LookupTXT(ctx context.Context, server, fqdn string) ([]string, error) {
	if _, _, err := net.SplitHostPort(server); err != nil {
		server = net.JoinHostPort(server, "53")
	}
	r := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "tcp", server)
	}}
	return r.LookupTXT(ctx, strings.TrimSuffix(fqdn, ".")+".")
}

// WaitTXT polls every server until each one answers value at fqdn, or ctx ends. A server that answers NXDOMAIN or
// without the value is asked again after every.
func WaitTXT(ctx context.Context, servers []string, fqdn, value string, every time.Duration) error {
	if len(servers) == 0 {
		return errors.New("no DNS servers to check the challenge value on")
	}
	pending := slices.Clone(servers)
	for {
		var last error
		next := pending[:0]
		for _, s := range pending {
			vals, err := LookupTXT(ctx, s, fqdn)
			if err == nil && slices.Contains(vals, value) {
				continue
			}
			last = err
			next = append(next, s)
		}
		pending = next
		if len(pending) == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			err := fmt.Errorf("%w: %s on %s", ErrNotPropagated, fqdn, strings.Join(pending, ", "))
			if last != nil {
				err = fmt.Errorf("%w (last answer: %w)", err, last)
			}
			return err
		case <-time.After(every):
		}
	}
}
