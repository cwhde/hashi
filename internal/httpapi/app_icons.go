package httpapi

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"git.juzo.io/juzo/hashi/internal/appicons"
	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// Dashboard app icons (07 §9.1, bug 149): chosen on App display, found by Hashi otherwise, served only by Hashi.

func appIconBody(a appicons.App, r db.AppIcon, ok bool) gen.AppIcon {
	out := gen.AppIcon{App: a.Key, Name: a.Name, Mode: gen.AppIconMode(appicons.ModeAuto)}
	if a.ServiceID != "" {
		out.ServiceId = &a.ServiceID
	}
	if !ok {
		return out
	}
	out.Mode = gen.AppIconMode(r.Mode)
	icon, light := appicons.URLs(r)
	str := func(v string) *string {
		if v == "" {
			return nil
		}
		return &v
	}
	out.LibraryRef, out.Icon, out.IconLight, out.Found = str(r.LibraryRef), str(icon), str(light), str(r.Found)
	if r.CheckedAt > 0 {
		t := time.UnixMilli(r.CheckedAt).UTC()
		out.CheckedAt = &t
	}
	return out
}

func (s *Server) ListAppIcons(ctx context.Context, _ gen.ListAppIconsRequestObject) (gen.ListAppIconsResponseObject, error) {
	apps, err := s.AppIcons.Apps(ctx)
	if err != nil {
		return nil, err
	}
	icons, err := s.AppIcons.Icons(ctx)
	if err != nil {
		return nil, err
	}
	out := gen.AppIcons{Apps: make([]gen.AppIcon, 0, len(apps))}
	for _, a := range apps {
		r, ok := icons[a.Key]
		out.Apps = append(out.Apps, appIconBody(a, r, ok))
	}
	slices.SortFunc(out.Apps, func(a, b gen.AppIcon) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	return gen.ListAppIcons200JSONResponse(out), nil
}

func (s *Server) PutAppIcon(ctx context.Context, req gen.PutAppIconRequestObject) (gen.PutAppIconResponseObject, error) {
	var (
		r   db.AppIcon
		err error
	)
	b := req.Body
	switch b.Mode {
	case gen.AppIconChoiceModeUpload:
		if b.Data == nil {
			return nil, invalid("An upload needs the image.")
		}
		r, err = s.AppIcons.Upload(ctx, req.App, *b.Data)
	case gen.AppIconChoiceModeLibrary:
		if b.LibraryRef == nil {
			return nil, invalid("Pick an icon of the library.")
		}
		r, err = s.AppIcons.UseLibrary(ctx, req.App, *b.LibraryRef)
	case gen.AppIconChoiceModeAuto:
		r, err = s.AppIcons.Auto(ctx, req.App)
	default:
		return nil, invalid("mode is auto, upload or library")
	}
	switch {
	case errors.Is(err, appicons.ErrNoApp):
		return nil, notFound("No such app.")
	case errors.Is(err, appicons.ErrInvalid):
		return nil, invalid(strings.TrimPrefix(err.Error(), appicons.ErrInvalid.Error()+": "))
	case err != nil && b.Mode == gen.AppIconChoiceModeLibrary:
		return nil, providerFailed("The icon library did not answer: " + err.Error())
	case err != nil:
		return nil, err
	}
	ri := info(ctx)
	if err := s.Audit.Record(ctx, s.Store.Q, audit.Event{Actor: s.actor(ri), Action: "app_icon.set", Target: req.App,
		IP: ri.ipString(), Detail: map[string]any{"mode": r.Mode, "library_ref": r.LibraryRef}}); err != nil {
		return nil, err
	}
	apps, err := s.AppIcons.Apps(ctx)
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(apps, func(a appicons.App) bool { return a.Key == req.App })
	if i < 0 {
		return nil, notFound("No such app.")
	}
	return gen.PutAppIcon200JSONResponse(appIconBody(apps[i], r, true)), nil
}

func (s *Server) SearchIconLibrary(ctx context.Context, req gen.SearchIconLibraryRequestObject) (gen.SearchIconLibraryResponseObject, error) {
	q := ""
	if req.Params.Q != nil {
		q = *req.Params.Q
	}
	hits, err := s.AppIcons.Lib.Search(ctx, q)
	if err != nil {
		return nil, providerFailed("The icon library did not answer: " + err.Error())
	}
	out := gen.IconLibraryHits{Icons: make([]gen.IconLibraryHit, 0, len(hits))}
	for _, h := range hits {
		g := gen.IconLibraryHit{Name: h.Name, Reference: h.Reference, Url: s.AppIcons.Lib.URL(h, false)}
		if h.Light {
			u := s.AppIcons.Lib.URL(h, true)
			g.LightUrl = &u
		}
		out.Icons = append(out.Icons, g)
	}
	return gen.SearchIconLibrary200JSONResponse(out), nil
}

// ServeAppIcon serves an app's icon to anyone, as ServeAsset does: the public dashboard needs it without a session.
func (s *Server) ServeAppIcon(w http.ResponseWriter, r *http.Request) {
	if s.AppIcons == nil {
		http.NotFound(w, r)
		return
	}
	f, ct, tag, err := s.AppIcons.Open(r.Context(), r.PathValue("name"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = f.Close() }()
	h := w.Header()
	h.Set("Content-Type", ct)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	cacheAsset(h, r, tag)
	http.ServeContent(w, r, "", time.Time{}, f)
}
