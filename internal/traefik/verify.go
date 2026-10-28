package traefik

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// rawData is the part of GET /api/rawdata Verify reads (testdata/contracts/traefik/api_rawdata.json): top-level maps
// keyed "<name>@<provider>" with a status and, when broken, an error list.
type rawData struct {
	Routers     map[string]rawObject `json:"routers"`
	Middlewares map[string]rawObject `json:"middlewares"`
	Services    map[string]rawObject `json:"services"`
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

// Verify reads the local Traefik API through the SSH connection (04 §6 step 4): every expected router, middleware and
// service exists and none is disabled or reports an error. Problems are listed, not just counted.
func Verify(ctx context.Context, h Host, apiPort int, files []File) error {
	want, err := Expected(files)
	if err != nil {
		return err
	}
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
		m := map[string]map[string]rawObject{"router": rd.Routers, "middleware": rd.Middlewares, "service": rd.Services}[kind]
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
