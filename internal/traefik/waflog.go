package traefik

import (
	"bytes"
	"context"
	"encoding/json"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// WAFBlock is one request the WAF refused, from Traefik's general log (testdata/contracts/coraza/traefik_log_matches.jsonl).
// Only "Access denied" lines are blocking decisions; the "Warning" lines before them are single rules' contributions
// to the anomaly score, also for harmless requests (06 §6.2).
type WAFBlock struct {
	At       time.Time
	Client   netip.Addr
	Router   string // e.g. "svc-files@file"
	Rule     int    // the deciding rule, 949110 for the anomaly score
	URI      string
	UniqueID string
}

type generalLine struct {
	Level          string    `json:"level"`
	RouterName     string    `json:"routerName"`
	MiddlewareName string    `json:"middlewareName"`
	Time           time.Time `json:"time"`
	Message        string    `json:"message"`
}

var corazaField = regexp.MustCompile(`\[(client|id|uri|unique_id) "([^"]*)"\]`)

// ParseWAFBlocks returns the blocking decisions of Hashi's WAF middlewares in b, one per request.
func ParseWAFBlocks(b []byte) []WAFBlock {
	var out []WAFBlock
	seen := map[string]bool{}
	for l := range bytes.SplitSeq(b, []byte{'\n'}) {
		if !bytes.Contains(l, []byte("Coraza: Access denied")) {
			continue
		}
		var g generalLine
		if json.Unmarshal(l, &g) != nil || !strings.HasPrefix(g.MiddlewareName, WAFMiddlewarePrefix) {
			continue
		}
		w := WAFBlock{At: g.Time, Router: g.RouterName}
		for _, m := range corazaField.FindAllStringSubmatch(g.Message, -1) {
			switch m[1] {
			case "client":
				if w.Client.IsValid() {
					continue
				}
				if a, err := netip.ParseAddr(m[2]); err == nil {
					w.Client = a.Unmap()
				}
			case "id":
				if w.Rule == 0 {
					w.Rule, _ = strconv.Atoi(m[2])
				}
			case "uri":
				if w.URI == "" {
					w.URI = m[2]
				}
			case "unique_id":
				w.UniqueID = m[2]
			}
		}
		if !w.Client.IsValid() || (w.UniqueID != "" && seen[w.UniqueID]) {
			continue
		}
		seen[w.UniqueID] = true
		out = append(out, w)
	}
	return out
}

// deliverWAF hands the general log's WAF blocks to the sink; it returns the lag of the newest one.
func (t *Tailer) deliverWAF(ctx context.Context, id string, b []byte) time.Duration {
	blocks := ParseWAFBlocks(b)
	if len(blocks) == 0 {
		return 0
	}
	if t.waf != nil {
		t.waf(ctx, id, blocks)
	}
	return max(0, t.m.now().Sub(blocks[len(blocks)-1].At))
}

// RouterService names the service slug a rendered router belongs to ("svc-<slug>", "-r-<route>", "-w-<path>"); the
// longest matching slug wins, since slugs may contain "-r-" themselves.
func RouterService(router string, slugs []string) (string, bool) {
	router = strings.TrimSuffix(router, "@file")
	best := ""
	for _, s := range slugs {
		base := "svc-" + s
		if (router == base || strings.HasPrefix(router, base+"-r-") || strings.HasPrefix(router, base+"-w-")) && len(s) > len(best) {
			best = s
		}
	}
	return best, best != ""
}
