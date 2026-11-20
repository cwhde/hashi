package httpapi

import (
	"context"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
)

// The AdGuard rewrites live on the AdGuard instances, not in Hashi's database, and one slow instance must not slow
// every search down (seen on prod 2026-11-20: one instance took 15 s to list its rewrites, so each search waited out
// its 2 s limit). Search reads them from a copy that a background refresh keeps; it never waits for an instance.

const (
	// rewritesFresh is how old the copy may get before a search starts a refresh.
	rewritesFresh = 2 * time.Minute
	// rewritesWait bounds one refresh of one instance; it runs in the background, so it can be generous.
	rewritesWait = 30 * time.Second
)

// rewriteCache holds each AdGuard instance's rewrites as search hits. An instance that fails to answer keeps what it
// had; an instance that is gone loses its entry.
type rewriteCache struct {
	mu     sync.Mutex
	byConn map[string][]searchHit
	at     time.Time
	busy   bool
}

// rewriteSource reads the instances: which exist, and one instance's rewrites.
type rewriteSource struct {
	conns func(context.Context) ([]string, error)
	read  func(context.Context, string) ([]searchHit, error)
}

// hits returns the copy as it is now and starts a background refresh when it is older than rewritesFresh. done, when
// not nil, is called once that refresh has finished (tests wait on it).
func (c *rewriteCache) hits(ctx context.Context, now time.Time, src rewriteSource, done func()) []searchHit {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.busy && now.Sub(c.at) >= rewritesFresh {
		c.busy = true
		go c.refresh(context.WithoutCancel(ctx), src, done)
	}
	var out []searchHit
	for _, h := range c.byConn {
		out = append(out, h...)
	}
	slices.SortFunc(out, func(a, b searchHit) int { return strings.Compare(a.title+a.detail, b.title+b.detail) })
	return out
}

func (c *rewriteCache) refresh(ctx context.Context, src rewriteSource, done func()) {
	defer func() {
		if done != nil {
			done()
		}
	}()
	ids, err := src.conns(ctx)
	if err != nil {
		c.mu.Lock()
		c.busy = false
		c.mu.Unlock()
		return
	}
	read := map[string][]searchHit{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Go(func() {
			cctx, cancel := context.WithTimeout(ctx, rewritesWait)
			defer cancel()
			if h, err := src.read(cctx, id); err == nil {
				mu.Lock()
				read[id] = h
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	c.mu.Lock()
	defer c.mu.Unlock()
	next := map[string][]searchHit{}
	for _, id := range ids {
		if h, ok := read[id]; ok {
			next[id] = h
		} else if old, ok := c.byConn[id]; ok {
			next[id] = old
		}
	}
	c.byConn, c.at, c.busy = next, time.Now(), false
}

func (s *Server) searchRewrites(ctx context.Context, q searchQuery) (*gen.SearchGroup, error) {
	if s.AdGuard == nil {
		return nil, nil
	}
	names := map[string]string{}
	src := rewriteSource{
		conns: func(ctx context.Context) ([]string, error) {
			rows, err := s.Store.R.ListAdGuardConnections(ctx)
			if err != nil {
				return nil, err
			}
			ids := make([]string, 0, len(rows))
			for _, r := range rows {
				names[r.ID] = r.Name
				ids = append(ids, r.ID)
			}
			return ids, nil
		},
		read: func(ctx context.Context, id string) ([]searchHit, error) {
			v, err := s.AdGuard.Rewrites(ctx, id)
			if err != nil {
				return nil, err
			}
			hits := make([]searchHit, 0, len(v.Rewrites))
			for _, r := range v.Rewrites {
				hits = append(hits, searchHit{title: r.Domain, detail: r.Answer + " · " + names[id],
					href: "/dns?tab=adguard&instance=" + url.QueryEscape(id) + "&find=" + url.QueryEscape(r.Domain)})
			}
			return hits, nil
		},
	}
	return q.group(gen.SearchGroupKindRewrite, "AdGuard rewrites", s.rewrites.hits(ctx, s.now(), src, nil), false), nil
}
