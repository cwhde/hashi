package httpapi

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/ipsets"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// The top bar search (10b §2, bug 4). The server searches what it stores; pages, tabs and settings live in the web app,
// which adds them itself. A query matches when every word of it appears in a result's text, case ignored; results
// whose title starts with the query come first. Each group lists at most searchMax results and counts the rest.

const searchMax = 8

// searchRemoteWait bounds the one search that asks another system: the AdGuard instances' rewrites.
const searchRemoteWait = 2 * time.Second

type searchHit struct {
	title, detail, href string
	// text is everything the query may match beyond title and detail (values, addresses, notes).
	text string
}

type searchQuery struct {
	raw   string
	words []string
}

func newSearchQuery(q string) searchQuery {
	q = strings.TrimSpace(q)
	return searchQuery{raw: strings.ToLower(q), words: strings.Fields(strings.ToLower(q))}
}

func (q searchQuery) matches(h searchHit) bool {
	hay := strings.ToLower(h.title + " " + h.detail + " " + h.text)
	for _, w := range q.words {
		if !strings.Contains(hay, w) {
			return false
		}
	}
	return len(q.words) > 0
}

// group keeps the hits that match, best first: title equal to the query, then title starting with it, then the rest in
// the order given.
func (q searchQuery) group(kind gen.SearchGroupKind, label string, hits []searchHit, prematched bool) *gen.SearchGroup {
	var keep []searchHit
	for _, h := range hits {
		if prematched || q.matches(h) {
			keep = append(keep, h)
		}
	}
	if len(keep) == 0 {
		return nil
	}
	rank := func(h searchHit) int {
		t := strings.ToLower(h.title)
		switch {
		case t == q.raw:
			return 0
		case strings.HasPrefix(t, q.raw):
			return 1
		}
		return 2
	}
	slices.SortStableFunc(keep, func(a, b searchHit) int { return rank(a) - rank(b) })
	g := &gen.SearchGroup{Kind: kind, Label: label, Results: []gen.SearchResult{}}
	for i, h := range keep {
		if i == searchMax {
			g.More = len(keep) - searchMax
			break
		}
		g.Results = append(g.Results, gen.SearchResult{Title: h.title, Detail: h.detail, Href: h.href})
	}
	return g
}

func (s *Server) Search(ctx context.Context, req gen.SearchRequestObject) (gen.SearchResponseObject, error) {
	q := newSearchQuery(req.Params.Q)
	out := gen.SearchResults{Groups: []gen.SearchGroup{}}
	if len(q.words) == 0 {
		return gen.Search200JSONResponse(out), nil
	}
	// An address or CIDR opens the address page; the IP sets that hold it come with the IP set items below.
	switch it, _ := ipsets.ParseIPOrCIDR(req.Params.Q); v := it.(type) {
	case ipsets.IP:
		out.Address = ptr(v.Addr.String())
	case ipsets.CIDR:
		out.Address = ptr(v.Prefix.String())
	}
	add := func(g *gen.SearchGroup) {
		if g != nil {
			out.Groups = append(out.Groups, *g)
		}
	}
	// The rewrites are read from the AdGuard instances while the rest is searched here.
	rewrites := make(chan []searchHit, 1)
	go func() { rewrites <- s.searchRewrites(ctx) }()

	steps := []func(context.Context, searchQuery) (*gen.SearchGroup, error){
		s.searchServices, s.searchDNS,
	}
	for _, step := range steps {
		g, err := step(ctx, q)
		if err != nil {
			return nil, err
		}
		add(g)
	}
	add(q.group(gen.SearchGroupKindRewrite, "AdGuard rewrites", <-rewrites, false))
	for _, step := range []func(context.Context, searchQuery) (*gen.SearchGroup, error){
		s.searchIPSets, s.searchIPSetItems, s.searchRuleSets, s.searchFirewallHosts, s.searchTraefikHosts,
		s.searchAgents, s.searchConnections, s.searchScripts, s.searchChecks, s.searchPlans, s.searchActivity,
	} {
		g, err := step(ctx, q)
		if err != nil {
			return nil, err
		}
		add(g)
	}
	return gen.Search200JSONResponse(out), nil
}

// find is the parameter a page reads to open or show one row (web/src/lib/find.ts).
func find(path, key string) string { return path + "?find=" + url.QueryEscape(key) }

func (s *Server) searchServices(ctx context.Context, q searchQuery) (*gen.SearchGroup, error) {
	all, err := services.List(ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	hits := make([]searchHit, 0, len(all))
	for _, sv := range all {
		up := sv.Target.Address
		if sv.Target.Port != 0 && up != "" {
			up = fmt.Sprintf("%s:%d", up, sv.Target.Port)
		}
		detail := sv.HostName
		if detail == "" {
			detail = sv.Kind
		}
		var more []string
		for _, r := range sv.Routes {
			for _, t := range r.Targets {
				more = append(more, t.Address)
			}
		}
		hits = append(hits, searchHit{title: sv.Name, detail: detail, href: "/services/" + sv.ID,
			text: sv.Slug + " " + up + " " + strings.Join(more, " ")})
	}
	return q.group(gen.SearchGroupKindService, "Services", hits, false), nil
}

func (s *Server) searchDNS(ctx context.Context, q searchQuery) (*gen.SearchGroup, error) {
	if s.DNS == nil {
		return nil, nil
	}
	zones, err := s.DNS.Zones(ctx)
	if err != nil {
		return nil, err
	}
	var hits []searchHit
	for _, z := range zones {
		rows, err := s.DNS.RRSets(ctx, z.ID, "", "")
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			hits = append(hits, searchHit{title: r.Name, detail: r.Type + " " + strings.Join(r.Values, ", "),
				href: find("/dns", r.Name), text: z.Name})
		}
	}
	return q.group(gen.SearchGroupKindDnsRecord, "DNS records", hits, false), nil
}

// searchRewrites reads every AdGuard instance's rewrites, each within searchRemoteWait; an instance that doesn't answer
// is left out, so a slow server never holds up the search.
func (s *Server) searchRewrites(ctx context.Context) []searchHit {
	if s.AdGuard == nil {
		return nil
	}
	conns, err := s.Store.R.ListAdGuardConnections(ctx)
	if err != nil {
		return nil
	}
	var (
		mu   sync.Mutex
		hits []searchHit
		wg   sync.WaitGroup
	)
	for _, c := range conns {
		wg.Go(func() {
			cctx, cancel := context.WithTimeout(ctx, searchRemoteWait)
			defer cancel()
			v, err := s.AdGuard.Rewrites(cctx, c.ID)
			if err != nil {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			for _, r := range v.Rewrites {
				hits = append(hits, searchHit{title: r.Domain, detail: r.Answer + " · " + c.Name,
					href: "/dns?tab=adguard&instance=" + url.QueryEscape(c.ID) + "&find=" + url.QueryEscape(r.Domain)})
			}
		})
	}
	wg.Wait()
	slices.SortFunc(hits, func(a, b searchHit) int { return strings.Compare(a.title+a.detail, b.title+b.detail) })
	return hits
}

func (s *Server) searchIPSets(ctx context.Context, q searchQuery) (*gen.SearchGroup, error) {
	rows, err := s.Store.R.ListIPSets(ctx)
	if err != nil {
		return nil, err
	}
	var hits []searchHit
	for _, r := range rows {
		if _, hidden := ipsets.AgentOf(r.ID); hidden {
			continue
		}
		hits = append(hits, searchHit{title: r.Name, detail: r.Kind + " set", href: "/ip-sets/" + r.ID,
			text: r.Slug + " " + r.Description})
	}
	return q.group(gen.SearchGroupKindIpSet, "IP sets", hits, false), nil
}

// searchIPSetItems finds the items that hold an address or CIDR, or whose text matches: the set's own search (05a §5).
func (s *Server) searchIPSetItems(ctx context.Context, q searchQuery) (*gen.SearchGroup, error) {
	g, err := ipsets.LoadGraph(ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	r, now := s.IPSets.Resolver(ctx), s.now()
	ids := make([]string, 0, len(g))
	for id := range g {
		if _, hidden := ipsets.AgentOf(id); !hidden {
			ids = append(ids, id)
		}
	}
	slices.SortFunc(ids, func(a, b string) int { return strings.Compare(g[a].Name, g[b].Name) })
	var hits []searchHit
	for _, id := range ids {
		for _, e := range g.Search(id, q.raw, r, now) {
			title := ipsets.Describe(g, e.Item)
			switch v := e.Item.(type) {
			case ipsets.IP:
				title = v.Addr.String()
			case ipsets.CIDR:
				if v.Prefix.Bits() == 0 {
					continue // "every address" holds everything and tells nothing
				}
				title = v.Prefix.String()
			}
			hits = append(hits, searchHit{title: title, detail: "in " + g[id].Name,
				href: "/ip-sets/" + id + "?q=" + url.QueryEscape(q.raw)})
		}
	}
	return q.group(gen.SearchGroupKindIpSetItem, "IP set items", hits, true), nil
}

func (s *Server) searchRuleSets(ctx context.Context, q searchQuery) (*gen.SearchGroup, error) {
	rows, err := s.Store.R.ListRuleSets(ctx)
	if err != nil {
		return nil, err
	}
	var hits []searchHit
	for _, r := range rows {
		hits = append(hits, searchHit{title: r.Name, detail: strings.ReplaceAll(r.Kind, "_", " ") + " rule set",
			href: "/rule-sets/" + r.ID, text: r.Slug + " " + r.Description})
	}
	return q.group(gen.SearchGroupKindRuleSet, "Rule sets", hits, false), nil
}

func (s *Server) searchFirewallHosts(ctx context.Context, q searchQuery) (*gen.SearchGroup, error) {
	rows, err := s.Store.R.ListFirewallHosts(ctx)
	if err != nil {
		return nil, err
	}
	var hits []searchHit
	for _, h := range rows {
		hits = append(hits, searchHit{title: h.Name, detail: h.PublicIp, href: "/firewall/" + h.ID, text: h.SshHost})
	}
	return q.group(gen.SearchGroupKindFirewallHost, "Firewall hosts", hits, false), nil
}

func (s *Server) searchTraefikHosts(ctx context.Context, q searchQuery) (*gen.SearchGroup, error) {
	rows, err := s.Store.R.ListTraefikConnections(ctx)
	if err != nil {
		return nil, err
	}
	var hits []searchHit
	for _, c := range rows {
		hits = append(hits, searchHit{title: c.Name, detail: c.InternalIp, href: find("/traefik", c.ID), text: c.SshHost})
	}
	return q.group(gen.SearchGroupKindTraefikHost, "Traefik hosts", hits, false), nil
}

func (s *Server) searchAgents(ctx context.Context, q searchQuery) (*gen.SearchGroup, error) {
	rows, err := s.Store.R.ListPulseAgents(ctx)
	if err != nil {
		return nil, err
	}
	var hits []searchHit
	for _, a := range rows {
		hits = append(hits, searchHit{title: a.Name, detail: a.SelectedIp, href: "/pulse/" + a.ID,
			text: a.Hostname + " " + a.PublicIp + " " + a.DnsName})
	}
	return q.group(gen.SearchGroupKindAgent, "Pulse agents", hits, false), nil
}

// searchConnections covers the outside systems that have no page of their own: DNS providers, AdGuard instances, sign-in
// providers and notification channels. Traefik and firewall hosts and agents have their own groups.
func (s *Server) searchConnections(ctx context.Context, q searchQuery) (*gen.SearchGroup, error) {
	var hits []searchHit
	if s.DNS != nil {
		zones, err := s.DNS.Zones(ctx)
		if err != nil {
			return nil, err
		}
		for _, z := range zones {
			hits = append(hits, searchHit{title: z.Name, detail: "DNS zone", href: find("/connections", "dns:"+z.ID)})
		}
	}
	ag, err := s.Store.R.ListAdGuardConnections(ctx)
	if err != nil {
		return nil, err
	}
	for _, c := range ag {
		hits = append(hits, searchHit{title: c.Name, detail: "AdGuard Home", href: find("/connections", "adguard:"+c.ID),
			text: c.ServerName})
	}
	oidc, err := s.Store.R.ListOIDCProviders(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range oidc {
		hits = append(hits, searchHit{title: p.Name, detail: "Sign-in provider", href: find("/connections", "signin:"+p.ID),
			text: p.Issuer})
	}
	np, err := s.Store.R.ListNotificationProviders(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range np {
		hits = append(hits, searchHit{title: p.Name, detail: "Notifications · " + p.Type,
			href: "/settings?category=monitoring&find=" + url.QueryEscape(p.Name)})
	}
	return q.group(gen.SearchGroupKindConnection, "Connections", hits, false), nil
}

func (s *Server) searchScripts(ctx context.Context, q searchQuery) (*gen.SearchGroup, error) {
	if s.Scripts == nil {
		return nil, nil
	}
	all, err := s.Scripts.List(ctx)
	if err != nil {
		return nil, err
	}
	var hits []searchHit
	for _, sc := range all {
		hits = append(hits, searchHit{title: sc.Name, detail: sc.Description, href: find("/scripts", sc.ID),
			text: sc.Slug + " " + strings.Join(sc.Hosts, " ")})
	}
	return q.group(gen.SearchGroupKindScript, "Scripts", hits, false), nil
}

func (s *Server) searchChecks(ctx context.Context, q searchQuery) (*gen.SearchGroup, error) {
	if s.Monitor == nil {
		return nil, nil
	}
	rows, err := s.Monitor.List(ctx)
	if err != nil {
		return nil, err
	}
	var hits []searchHit
	for _, r := range rows {
		hits = append(hits, searchHit{title: r.Endpoint.Name, detail: r.State, href: "/status/" + r.Endpoint.ID,
			text: r.Endpoint.GroupName})
	}
	return q.group(gen.SearchGroupKindCheck, "Checks", hits, false), nil
}

// searchPlans covers what the changes page shows: every open plan and the latest finished ones.
func (s *Server) searchPlans(ctx context.Context, q searchQuery) (*gen.SearchGroup, error) {
	if s.Sync == nil {
		return nil, nil
	}
	open, err := s.Sync.Open(ctx)
	if err != nil {
		return nil, err
	}
	recent, err := s.Sync.Recent(ctx, 20)
	if err != nil {
		return nil, err
	}
	var hits []searchHit
	for _, p := range append(open, recent...) {
		hits = append(hits, searchHit{title: p.Title, detail: p.Status + " · " + p.CreatedAt.UTC().Format("2006-01-02 15:04") + " UTC",
			href: find("/changes", p.ID), text: p.Target + " " + p.Subsystem})
	}
	return q.group(gen.SearchGroupKindPlan, "Plans", hits, false), nil
}

// searchActivity looks through the latest audit entries; each result opens the audit log filtered to its target.
func (s *Server) searchActivity(ctx context.Context, q searchQuery) (*gen.SearchGroup, error) {
	rows, err := s.Store.R.ListAuditEvents(ctx, db.ListAuditEventsParams{Lim: 500})
	if err != nil {
		return nil, err
	}
	var hits []searchHit
	seen := map[string]bool{}
	for _, r := range rows {
		key := r.Action + "\x00" + r.Target
		if seen[key] {
			continue
		}
		h := searchHit{title: r.Target, detail: r.Action + " · " + time.UnixMilli(r.At).UTC().Format("2006-01-02 15:04") + " UTC",
			href: "/activity?target=" + url.QueryEscape(r.Target), text: r.Actor}
		if r.Target == "" || !q.matches(h) {
			continue
		}
		seen[key] = true
		hits = append(hits, h)
	}
	return q.group(gen.SearchGroupKindActivity, "Activity", hits, true), nil
}
