package httpapi

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/settings"
)

// publicProbe asks one upstream which public page answers there. Each public listener serves only its own API path
// (app.go publicMux), so the answer is exact even though the container's port mapping is invisible to Hashi.
var publicProbe = func(ctx context.Context, addr, path string) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+path, nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK
}

// GetPublicPages lists the services that publish the public app dashboard and status page (bug 64): those whose
// upstream is Hashi's own address and whose port answers that page's API.
func (s *Server) GetPublicPages(ctx context.Context, _ gen.GetPublicPagesRequestObject) (gen.GetPublicPagesResponseObject, error) {
	out := gen.PublicPages{Apps: []gen.PublicPageLink{}, Status: []gen.PublicPageLink{}}
	g, err := settings.Get[settings.General](ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(g.InternalURL)
	if err != nil || u.Hostname() == "" {
		return gen.GetPublicPages200JSONResponse(out), nil
	}
	svcs, err := services.List(ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, v := range svcs {
		if v.System() || !v.Enabled || v.HostName == "" || v.Stream() || !strings.EqualFold(v.Target.Address, u.Hostname()) {
			continue
		}
		addr := net.JoinHostPort(v.Target.Address, strconv.Itoa(v.Target.Port))
		link := gen.PublicPageLink{ServiceId: v.ID, Name: v.Name, Url: fmt.Sprintf("https://%s/", v.HostName), Upstream: addr}
		wg.Add(1)
		go func() {
			defer wg.Done()
			apps, status := publicProbe(ctx, addr, "/api/public/apps"), publicProbe(ctx, addr, "/api/public/status")
			mu.Lock()
			defer mu.Unlock()
			if apps {
				out.Apps = append(out.Apps, link)
			}
			if status {
				out.Status = append(out.Status, link)
			}
		}()
	}
	wg.Wait()
	return gen.GetPublicPages200JSONResponse(out), nil
}
