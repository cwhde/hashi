package httpapi

import (
	"context"
	"errors"
	"net/netip"
	"strconv"
	"strings"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/ipsets"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// pageSize bounds element and prefix pages (feed and dynamic sets can be large, 05a §6).
const pageSize = 200

func ipSetError(err error) error {
	var ce *ipsets.CycleError
	switch {
	case errors.As(err, &ce):
		return badRequest(err.Error())
	case errors.Is(err, ipsets.ErrInvalid):
		return invalid(strings.TrimPrefix(err.Error(), ipsets.ErrInvalid.Error()+": "))
	case errors.Is(err, ipsets.ErrNotFound):
		return notFound("No such IP set or element.")
	case errors.Is(err, ipsets.ErrLocked), errors.Is(err, ipsets.ErrTaken), errors.Is(err, ipsets.ErrInUse):
		return conflict(err.Error())
	}
	return err
}

// ipSetBody summarizes a set with its compile result.
func ipSetBody(s ipsets.Set, res ipsets.Result) gen.IPSet {
	return gen.IPSet{Id: s.ID, Slug: s.Slug, Name: s.Name, Description: s.Description, Kind: gen.IPSetKind(s.Kind),
		Origin: gen.IPSetOrigin(s.Origin), SystemKey: s.SystemKey, Editable: s.Editable(), ElementCount: len(s.Elements),
		ResolutionStatus: gen.IPSetResolutionStatus(res.Status), Problems: problemsBody(res.Problems),
		AddressSet: res.AddressSet, V4Count: len(res.V4), V6Count: len(res.V6), CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt}
}

func problemsBody(ps []ipsets.Problem) []gen.IPSetProblem {
	out := make([]gen.IPSetProblem, 0, len(ps))
	for _, p := range ps {
		out = append(out, gen.IPSetProblem{SetId: p.SetID, ElementId: p.ElementID, Path: p.Path, Message: p.Message})
	}
	return out
}

func itemBody(it ipsets.Item) (gen.IPItem, error) {
	var out gen.IPItem
	var err error
	switch v := it.(type) {
	case ipsets.IP:
		err = out.FromIPItemIP(gen.IPItemIP{Type: gen.Ip, Address: v.Addr.String()})
	case ipsets.CIDR:
		err = out.FromIPItemCIDR(gen.IPItemCIDR{Type: gen.Cidr, Prefix: v.Prefix.String()})
	case ipsets.Host:
		err = out.FromIPItemHost(gen.IPItemHost{Type: gen.Host, HostId: v.HostID, Scope: gen.IPItemHostScope(v.Scope)})
	case ipsets.DNS:
		err = out.FromIPItemDNS(gen.IPItemDNS{Type: gen.Dns, Fqdn: v.FQDN})
	case ipsets.SetRef:
		err = out.FromIPItemSet(gen.IPItemSet{Type: gen.Set, SetId: v.SetID})
	}
	return out, err
}

// itemFromBody turns the API union into an item; field validation happens in ipsets.Normalize.
func itemFromBody(b gen.IPItem) (ipsets.Item, error) {
	v, err := b.ValueByDiscriminator()
	if err != nil {
		return nil, invalid("item.type must be ip, cidr, host, dns or set")
	}
	switch v := v.(type) {
	case gen.IPItemIP:
		it, err := ipsets.ParseIPOrCIDR(v.Address)
		if err != nil {
			return nil, err
		}
		if _, ok := it.(ipsets.CIDR); ok {
			return nil, invalid("an ip item takes one address; use a cidr item for a prefix")
		}
		return it, nil
	case gen.IPItemCIDR:
		if !strings.Contains(v.Prefix, "/") {
			return nil, invalid("a cidr item takes a prefix such as 192.0.2.0/24")
		}
		return ipsets.ParseIPOrCIDR(v.Prefix)
	case gen.IPItemHost:
		return ipsets.Host{HostID: v.HostId, Scope: string(v.Scope)}, nil
	case gen.IPItemDNS:
		return ipsets.DNS{FQDN: v.Fqdn}, nil
	case gen.IPItemSet:
		return ipsets.SetRef{SetID: v.SetId}, nil
	}
	return nil, invalid("unknown item type")
}

func elementBody(g ipsets.Graph, e ipsets.Element) (gen.IPSetElement, error) {
	it, err := itemBody(e.Item)
	if err != nil {
		return gen.IPSetElement{}, err
	}
	return gen.IPSetElement{Id: e.ID, Position: e.Position, Origin: e.Origin, Locked: e.Locked, Item: it,
		Label: ipsets.Describe(g, e.Item), Note: e.Note, ExpiresAt: e.ExpiresAt, CreatedBy: e.CreatedBy,
		CreatedAt: e.CreatedAt}, nil
}

func (s *Server) ListIPSets(ctx context.Context, req gen.ListIPSetsRequestObject) (gen.ListIPSetsResponseObject, error) {
	g, err := ipsets.LoadGraph(ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	rows, err := s.Store.R.ListIPSets(ctx)
	if err != nil {
		return nil, err
	}
	r, now := s.IPSets.Resolver(ctx), s.now()
	q := strings.ToLower(strings.TrimSpace(deref(req.Params.Q)))
	out := gen.IPSetList{Sets: []gen.IPSet{}}
	for _, row := range rows { // the query's order: system first, then by name
		set := g[row.ID]
		if req.Params.Kind != nil && set.Kind != string(*req.Params.Kind) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(set.Slug+" "+set.Name+" "+set.Description), q) {
			continue
		}
		out.Sets = append(out.Sets, ipSetBody(*set, g.Compile(set.ID, r, now)))
	}
	return gen.ListIPSets200JSONResponse(out), nil
}

func (s *Server) GetIPSet(ctx context.Context, req gen.GetIPSetRequestObject) (gen.GetIPSetResponseObject, error) {
	body, err := s.ipSetSummary(ctx, s.Store.R, req.Id)
	if err != nil {
		return nil, ipSetError(err)
	}
	return gen.GetIPSet200JSONResponse(body), nil
}

func (s *Server) ipSetSummary(ctx context.Context, q *db.Queries, id string) (gen.IPSet, error) {
	g, err := ipsets.LoadGraph(ctx, q)
	if err != nil {
		return gen.IPSet{}, err
	}
	set, ok := g[id]
	if !ok {
		return gen.IPSet{}, ipsets.ErrNotFound
	}
	return ipSetBody(*set, g.Compile(id, s.IPSets.Resolver(ctx), s.now())), nil
}

// changeIPSet runs one change with its audit entry in a transaction, then schedules the recompile that tells the
// set's consumers (05a §4.2).
func (s *Server) changeIPSet(ctx context.Context, action string, setID func() string, f func(q *db.Queries) (map[string]any, error)) error {
	ri := info(ctx)
	err := s.Store.Tx(ctx, func(q *db.Queries) error {
		detail, err := f(q)
		if err != nil {
			return err
		}
		return s.Audit.Record(ctx, q, audit.Event{Actor: s.actor(ri), Action: action, IP: ri.ipString(),
			Target: "ip_set:" + setID(), Outcome: audit.Success, Detail: detail})
	})
	if err != nil {
		return ipSetError(err)
	}
	s.IPSets.Changed(ctx)
	return nil
}

func (s *Server) CreateIPSet(ctx context.Context, req gen.CreateIPSetRequestObject) (gen.CreateIPSetResponseObject, error) {
	var set ipsets.Set
	err := s.changeIPSet(ctx, "ip_set.created", func() string { return set.ID }, func(q *db.Queries) (map[string]any, error) {
		var err error
		set, err = ipsets.CreateSet(ctx, q, setInput(*req.Body), s.now())
		return map[string]any{"slug": set.Slug, "name": set.Name}, err
	})
	if err != nil {
		return nil, err
	}
	return gen.CreateIPSet200JSONResponse(ipSetBody(set, ipsets.Result{Status: ipsets.StatusEmpty})), nil
}

func (s *Server) UpdateIPSet(ctx context.Context, req gen.UpdateIPSetRequestObject) (gen.UpdateIPSetResponseObject, error) {
	err := s.changeIPSet(ctx, "ip_set.updated", func() string { return req.Id }, func(q *db.Queries) (map[string]any, error) {
		set, err := ipsets.UpdateSet(ctx, q, req.Id, setInput(*req.Body), s.now())
		return map[string]any{"slug": set.Slug, "name": set.Name}, err
	})
	if err != nil {
		return nil, err
	}
	body, err := s.ipSetSummary(ctx, s.Store.R, req.Id)
	if err != nil {
		return nil, ipSetError(err)
	}
	return gen.UpdateIPSet200JSONResponse(body), nil
}

func (s *Server) DeleteIPSet(ctx context.Context, req gen.DeleteIPSetRequestObject) (gen.DeleteIPSetResponseObject, error) {
	err := s.changeIPSet(ctx, "ip_set.deleted", func() string { return req.Id }, func(q *db.Queries) (map[string]any, error) {
		set, err := ipsets.DeleteSet(ctx, q, req.Id, s.IPSets.Sources())
		return map[string]any{"slug": set.Slug, "name": set.Name}, err
	})
	if err != nil {
		return nil, err
	}
	return gen.DeleteIPSet204Response{}, nil
}

func setInput(b gen.IPSetInput) ipsets.SetInput {
	return ipsets.SetInput{Slug: b.Slug, Name: b.Name, Description: deref(b.Description)}
}

func (s *Server) ListIPSetElements(ctx context.Context, req gen.ListIPSetElementsRequestObject) (gen.ListIPSetElementsResponseObject, error) {
	g, err := ipsets.LoadGraph(ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	if _, ok := g[req.Id]; !ok {
		return nil, ipSetError(ipsets.ErrNotFound)
	}
	all := g.Search(req.Id, deref(req.Params.Q), s.IPSets.Resolver(ctx), s.now())
	from, to, next, err := page(len(all), req.Params.Cursor)
	if err != nil {
		return nil, err
	}
	out := gen.IPSetElementPage{Elements: make([]gen.IPSetElement, 0, to-from), Total: len(all), NextCursor: next}
	for _, e := range all[from:to] {
		b, err := elementBody(g, e)
		if err != nil {
			return nil, err
		}
		out.Elements = append(out.Elements, b)
	}
	return gen.ListIPSetElements200JSONResponse(out), nil
}

func (s *Server) AddIPSetElement(ctx context.Context, req gen.AddIPSetElementRequestObject) (gen.AddIPSetElementResponseObject, error) {
	in, err := elementInput(*req.Body)
	if err != nil {
		return nil, ipSetError(err)
	}
	var e ipsets.Element
	err = s.changeIPSet(ctx, "ip_set.element.added", func() string { return req.Id }, func(q *db.Queries) (map[string]any, error) {
		var err error
		e, err = s.IPSets.Editor().AddElement(ctx, q, req.Id, in, s.actor(info(ctx)), s.now())
		return map[string]any{"element_id": e.ID, "type": in.Item.Type()}, err
	})
	if err != nil {
		return nil, err
	}
	b, err := s.elementResponse(ctx, e)
	return gen.AddIPSetElement200JSONResponse(b), err
}

func (s *Server) UpdateIPSetElement(ctx context.Context, req gen.UpdateIPSetElementRequestObject) (gen.UpdateIPSetElementResponseObject, error) {
	in, err := elementInput(*req.Body)
	if err != nil {
		return nil, ipSetError(err)
	}
	var e ipsets.Element
	err = s.changeIPSet(ctx, "ip_set.element.updated", func() string { return req.Id }, func(q *db.Queries) (map[string]any, error) {
		var err error
		e, err = s.IPSets.Editor().UpdateElement(ctx, q, req.Id, req.ElementId, in, s.now())
		return map[string]any{"element_id": req.ElementId, "type": in.Item.Type()}, err
	})
	if err != nil {
		return nil, err
	}
	b, err := s.elementResponse(ctx, e)
	return gen.UpdateIPSetElement200JSONResponse(b), err
}

func (s *Server) DeleteIPSetElement(ctx context.Context, req gen.DeleteIPSetElementRequestObject) (gen.DeleteIPSetElementResponseObject, error) {
	err := s.changeIPSet(ctx, "ip_set.element.removed", func() string { return req.Id }, func(q *db.Queries) (map[string]any, error) {
		e, err := s.IPSets.Editor().DeleteElement(ctx, q, req.Id, req.ElementId, s.now())
		if err != nil {
			return nil, err
		}
		return map[string]any{"element_id": e.ID, "type": e.Item.Type()}, nil
	})
	if err != nil {
		return nil, err
	}
	return gen.DeleteIPSetElement204Response{}, nil
}

func (s *Server) elementResponse(ctx context.Context, e ipsets.Element) (gen.IPSetElement, error) {
	g, err := ipsets.LoadGraph(ctx, s.Store.R)
	if err != nil {
		return gen.IPSetElement{}, err
	}
	return elementBody(g, e)
}

func elementInput(b gen.IPSetElementInput) (ipsets.ElementInput, error) {
	it, err := itemFromBody(b.Item)
	if err != nil {
		return ipsets.ElementInput{}, err
	}
	in := ipsets.ElementInput{Item: it, Note: deref(b.Note), ExpiresAt: b.ExpiresAt}
	if in.ExpiresAt != nil {
		t := in.ExpiresAt.UTC()
		in.ExpiresAt = &t
	}
	return in, nil
}

func (s *Server) GetIPSetResolved(ctx context.Context, req gen.GetIPSetResolvedRequestObject) (gen.GetIPSetResolvedResponseObject, error) {
	res, err := s.IPSets.Compile(ctx, s.Store.R, req.Id)
	if err != nil {
		return nil, ipSetError(err)
	}
	var prefixes []netip.Prefix
	switch {
	case req.Params.Family == nil:
		prefixes = append(append(prefixes, res.V4...), res.V6...)
	case *req.Params.Family == gen.N4:
		prefixes = res.V4
	default:
		prefixes = res.V6
	}
	from, to, next, err := page(len(prefixes), req.Params.Cursor)
	if err != nil {
		return nil, err
	}
	out := gen.IPSetResolved{Status: gen.IPSetResolvedStatus(res.Status), Problems: problemsBody(res.Problems),
		AddressSet: res.AddressSet, V4Count: len(res.V4), V6Count: len(res.V6), Prefixes: make([]string, 0, to-from),
		Addresses: []string{}, NextCursor: next}
	for _, p := range prefixes[from:to] {
		out.Prefixes = append(out.Prefixes, p.String())
	}
	if res.AddressSet {
		for _, a := range res.Addresses {
			out.Addresses = append(out.Addresses, a.String())
		}
	}
	return gen.GetIPSetResolved200JSONResponse(out), nil
}

func (s *Server) GetIPSetConsumers(ctx context.Context, req gen.GetIPSetConsumersRequestObject) (gen.GetIPSetConsumersResponseObject, error) {
	if _, err := s.Store.R.GetIPSet(ctx, req.Id); err != nil {
		return nil, ipSetError(ipsets.ErrNotFound)
	}
	cons, err := ipsets.Consumers(ctx, s.Store.R, req.Id, s.IPSets.Sources())
	if err != nil {
		return nil, err
	}
	out := gen.IPSetConsumerList{Consumers: make([]gen.IPSetConsumer, 0, len(cons))}
	for _, c := range cons {
		out.Consumers = append(out.Consumers, gen.IPSetConsumer{Type: c.Type, Id: c.ID, Name: c.Name,
			Position: gen.IPSetConsumerPosition(c.Position)})
	}
	return gen.GetIPSetConsumers200JSONResponse(out), nil
}

func (s *Server) LookupAddress(ctx context.Context, req gen.LookupAddressRequestObject) (gen.LookupAddressResponseObject, error) {
	a, err := ipsets.ParseAddress(req.Address)
	if err != nil {
		return nil, ipSetError(err)
	}
	g, err := ipsets.LoadGraph(ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	r, now := s.IPSets.Resolver(ctx), s.now()
	out := gen.AddressLookup{Address: a.String(), Sets: []gen.AddressSetMatch{}}
	for _, id := range g.Containing(a, r, now) {
		m := gen.AddressSetMatch{Set: ipSetBody(*g[id], g.Compile(id, r, now)), Matches: []gen.AddressMatch{}}
		for _, x := range g.Explain(id, a, r, now) {
			m.Matches = append(m.Matches, gen.AddressMatch{Path: x.Path, SetId: x.SetID, ElementId: x.ElementID,
				Label: ipsets.Describe(g, x.Item)})
		}
		out.Sets = append(out.Sets, m)
	}
	return gen.LookupAddress200JSONResponse(out), nil
}

// page turns an offset cursor into slice bounds.
func page(total int, cursor *string) (from, to int, next string, err error) {
	if c := deref(cursor); c != "" {
		if from, err = strconv.Atoi(c); err != nil || from < 0 || from > total {
			return 0, 0, "", badRequest("cursor is not valid")
		}
	}
	to = min(from+pageSize, total)
	if to < total {
		next = strconv.Itoa(to)
	}
	return from, to, next, nil
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}
