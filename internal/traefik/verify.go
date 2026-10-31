package traefik

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// rawData is the part of GET /api/rawdata Verify reads (testdata/contracts/traefik/api_rawdata.json): top-level maps
// keyed "<name>@<provider>" with a status and, when broken, an error list.
// Streams appear under tcpRouters, tcpServices, udpRouters and udpServices (testdata/contracts/traefik/streams).
type rawData struct {
	Routers     map[string]rawObject `json:"routers"`
	Middlewares map[string]rawObject `json:"middlewares"`
	Services    map[string]rawObject `json:"services"`
	TCPRouters  map[string]rawObject `json:"tcpRouters"`
	TCPServices map[string]rawObject `json:"tcpServices"`
	UDPRouters  map[string]rawObject `json:"udpRouters"`
	UDPServices map[string]rawObject `json:"udpServices"`
}

type rawObject struct {
	Status string   `json:"status"`
	Error  []string `json:"error"`
}

// ErrVerify means Traefik did not load what Hashi wrote.
var ErrVerify = errors.New("traefik did not load the configuration")

// Expected lists the routers, middlewares and services the rendered dynamic files define, as "<kind> <name>@file".
func Expected(files []File) ([]string, error) {
	var out []string
	for _, f := range files {
		if f.Kind != KindDynamic {
			continue
		}
		var c dynamicConfig
		if err := yaml.Unmarshal(f.Data, &c); err != nil {
			return nil, fmt.Errorf("parse %s: %w", f.Path, err)
		}
		if c.TCP != nil {
			for n := range c.TCP.Routers {
				out = append(out, "tcp-router "+n+"@file")
			}
			for n := range c.TCP.Services {
				out = append(out, "tcp-service "+n+"@file")
			}
		}
		if c.UDP != nil {
			for n := range c.UDP.Routers {
				out = append(out, "udp-router "+n+"@file")
			}
			for n := range c.UDP.Services {
				out = append(out, "udp-service "+n+"@file")
			}
		}
		if c.HTTP == nil {
			continue
		}
		for n := range c.HTTP.Routers {
			out = append(out, "router "+n+"@file")
		}
		for n := range c.HTTP.Middlewares {
			out = append(out, "middleware "+n+"@file")
		}
		for n := range c.HTTP.Services {
			out = append(out, "service "+n+"@file")
		}
	}
	slices.Sort(out)
	return out, nil
}

// VerifyWait is how long Verify keeps reading the API until Traefik shows what Hashi wrote. Dynamic files are loaded by
// Traefik's watcher a moment after the write (S5 gate: an immediate read still showed the previous configuration).
var VerifyWait = 20 * time.Second

// Verify reads the local Traefik API through the SSH connection (04 §6 step 4): every expected router, middleware and
// service exists and none is disabled or reports an error. It retries until VerifyWait has passed, then lists the
// problems of the last read, not just a count.
func Verify(ctx context.Context, h Host, apiPort int, files []File) error {
	want, err := Expected(files)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(VerifyWait)
	for {
		err = verifyOnce(ctx, h, apiPort, want)
		if err == nil || time.Now().After(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return errors.Join(err, ctx.Err())
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func verifyOnce(ctx context.Context, h Host, apiPort int, want []string) error {
	b, err := h.Fetch(ctx, apiPort, "/api/rawdata")
	if err != nil {
		return fmt.Errorf("read the traefik API: %w", err)
	}
	var rd rawData
	if err := json.Unmarshal(b, &rd); err != nil {
		return fmt.Errorf("parse /api/rawdata: %w", err)
	}
	var problems []string
	for _, w := range want {
		kind, name, _ := strings.Cut(w, " ")
		m := map[string]map[string]rawObject{"router": rd.Routers, "middleware": rd.Middlewares, "service": rd.Services,
			"tcp-router": rd.TCPRouters, "tcp-service": rd.TCPServices, "udp-router": rd.UDPRouters, "udp-service": rd.UDPServices}[kind]
		o, ok := m[name]
		switch {
		case !ok:
			problems = append(problems, w+" is missing")
		case len(o.Error) > 0:
			problems = append(problems, w+": "+strings.Join(o.Error, "; "))
		case o.Status != "enabled":
			problems = append(problems, w+" is "+o.Status)
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("%w: %s", ErrVerify, strings.Join(problems, ", "))
	}
	return nil
}
