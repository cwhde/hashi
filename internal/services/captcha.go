package services

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"reflect"
	"strconv"
	"time"

	"github.com/oklog/ulid/v2"

	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// OwnerCaptcha owns the challenge service (06 §11).
const OwnerCaptcha = "captcha"

// ChallengePath is Hashi's part of the challenge service: the challenge page and its API. The rest of the name goes to
// Cap, and forward auth lets only the flow's paths through (ChallengeFlowPath).
const ChallengePath = "/api/edge-challenge"

// IsChallenge reports whether s is the challenge service.
func (s Service) IsChallenge() bool { return s.System() && s.OwnerFeature == OwnerCaptcha }

// EnsureChallenge creates, updates or removes the challenge service from the CAPTCHA and general settings: on the
// challenge domain, Cap's public paths go to Cap where edges reach it, ChallengePath goes to Hashi. It reports whether
// anything changed.
func EnsureChallenge(ctx context.Context, q *db.Queries, now time.Time) (Service, bool, error) {
	c, err := settings.Get[settings.Captcha](ctx, q)
	if err != nil {
		return Service{}, false, err
	}
	row, err := q.GetServiceByOwner(ctx, OwnerCaptcha)
	exists := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Service{}, false, err
	}
	if !c.Enabled {
		if !exists {
			return Service{}, false, nil
		}
		cur, err := Get(ctx, q, row.ID)
		if err != nil {
			return Service{}, false, err
		}
		return cur, true, q.DeleteService(ctx, row.ID)
	}
	g, err := settings.Get[settings.General](ctx, q)
	if err != nil {
		return Service{}, false, err
	}
	want, err := challengeService(g, c)
	if err != nil {
		return Service{}, false, err
	}
	if !exists {
		want.ID = ulid.Make().String()
		if err := insert(ctx, q, want, now); err != nil {
			return Service{}, false, err
		}
		s, err := Get(ctx, q, want.ID)
		return s, true, err
	}
	cur, err := Get(ctx, q, row.ID)
	if err != nil {
		return Service{}, false, err
	}
	want.ID = cur.ID
	if len(cur.Routes) == 1 {
		want.Routes[0].ID = cur.Routes[0].ID // keep the route's identity (04 §1.1)
	}
	if cur.HostName == want.HostName && cur.DomainMode == want.DomainMode && cur.Target == want.Target &&
		reflect.DeepEqual(cur.Routes, want.Routes) {
		return cur, false, nil
	}
	if err := update(ctx, q, want, now); err != nil {
		return Service{}, false, err
	}
	s, err := Get(ctx, q, cur.ID)
	return s, true, err
}

func challengeService(g settings.General, c settings.Captcha) (Service, error) {
	if g.InternalURL == "" || g.RootDomain == "" {
		return Service{}, errors.New("the root domain and internal Hashi URL are not set (setup step 2)")
	}
	u, err := url.Parse(g.InternalURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return Service{}, errors.New("the internal Hashi URL is not an http(s) URL")
	}
	port := 80
	if u.Scheme == "https" {
		port = 443
	}
	if p := u.Port(); p != "" {
		if port, err = strconv.Atoi(p); err != nil {
			return Service{}, errors.New("the internal Hashi URL has a bad port")
		}
	}
	mode := ModeSubdomain
	if c.ChallengeDomain == g.RootDomain {
		mode = ModeRoot
	}
	return Service{Name: "CAPTCHA challenge", Slug: "hashi-challenge", Kind: KindHTTP, DomainMode: mode,
		HostName: c.ChallengeDomain, Enabled: true, Origin: OriginSystem, OwnerFeature: OwnerCaptcha,
		Target: Target{Address: c.CapAddress, Port: c.CapPort}, AdvancedRoutes: true,
		Routes: []Route{{Enabled: true, Targets: []Target{{Address: u.Hostname(), Port: port}}, Kind: u.Scheme,
			PathMatch: MatchPrefix, PathValue: ChallengePath + "/", Priority: 10, RewriteMode: RewriteNone}}}, nil
}
