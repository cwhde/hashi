package appicons

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"git.juzo.io/juzo/hashi/internal/assets"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// ErrInvalid marks input the owner can fix; ErrNoApp an app that isn't on the list.
var (
	ErrInvalid = errors.New("invalid icon")
	ErrNoApp   = errors.New("no such app")
)

// Modes of an app's icon.
const (
	ModeAuto    = "auto"
	ModeUpload  = "upload"
	ModeLibrary = "library"
)

const (
	maxIcon = 1 << 20
	// recheck is how often an automatic icon is looked for again (07 §9.1).
	recheck = 24 * time.Hour
)

func found(b []byte, from string) (Found, error) {
	if len(b) > maxIcon {
		return Found{}, fmt.Errorf("%w: larger than 1 MB", ErrInvalid)
	}
	ct, err := assets.Sniff(b)
	if err != nil {
		return Found{}, fmt.Errorf("%w: only PNG, WebP, SVG or ICO images", ErrInvalid)
	}
	return Found{Data: b, ContentType: ct, From: from}, nil
}

// App is one app that can be on the dashboard: a web service with a public name, or a DNS entry shown there.
type App struct {
	Key       string // service-<id> or dns-<id>; names its files and its icon address
	Name      string
	Public    string
	Upstream  string
	ServiceID string
	DNSID     string
}

// Key names an app's icon.
func Key(source, id string) string {
	if source == "dns_record" {
		return "dns-" + id
	}
	return "service-" + id
}

var keyForm = regexp.MustCompile(`^(service|dns)-[A-Za-z0-9_-]{1,64}$`)

// Service keeps the icons.
type Service struct {
	st   *store.Store
	dir  string
	Find Finder
	Lib  *Library
	Log  *slog.Logger
	now  func() time.Time
	mu   sync.Mutex // one change per app at a time is enough: writes are rare
}

// New stores icon files in dir.
func New(st *store.Store, dir string, f Finder, lib *Library, log *slog.Logger) (*Service, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("app icon folder: %w", err)
	}
	return &Service{st: st, dir: dir, Find: f, Lib: lib, Log: log, now: time.Now}, nil
}

// Apps lists every app that can carry an icon, in no particular order.
func (s *Service) Apps(ctx context.Context) ([]App, error) {
	svcs, err := services.List(ctx, s.st.R)
	if err != nil {
		return nil, err
	}
	var out []App
	for _, v := range svcs {
		if v.System() || v.HostName == "" || v.Stream() {
			continue
		}
		a := App{Key: Key("service", v.ID), Name: v.Name, Public: "https://" + strings.TrimSuffix(v.HostName, ".") + "/", ServiceID: v.ID}
		if v.Target.Address != "" && v.Target.Port > 0 {
			scheme := "http"
			if v.Kind == services.KindHTTPS {
				scheme = "https"
			}
			a.Upstream = scheme + "://" + net.JoinHostPort(v.Target.Address, strconv.Itoa(v.Target.Port)) + "/"
		}
		out = append(out, a)
	}
	recs, err := s.st.R.ListDashboardDNSRecords(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range recs {
		host := r.ZoneName
		if r.Name != "@" {
			host = r.Name + "." + r.ZoneName
		}
		name := r.DisplayName
		if name == "" {
			name = host
		}
		out = append(out, App{Key: Key("dns_record", r.ID), Name: name, Public: "https://" + host + "/", DNSID: r.ID})
	}
	return out, nil
}

func (s *Service) app(ctx context.Context, key string) (App, error) {
	apps, err := s.Apps(ctx)
	if err != nil {
		return App{}, err
	}
	for _, a := range apps {
		if a.Key == key {
			return a, nil
		}
	}
	return App{}, ErrNoApp
}

// Icons returns every stored icon row by app key.
func (s *Service) Icons(ctx context.Context) (map[string]db.AppIcon, error) {
	rows, err := s.st.R.ListAppIcons(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]db.AppIcon, len(rows))
	for _, r := range rows {
		out[r.App] = r
	}
	return out, nil
}

func (s *Service) path(key string, light bool) string {
	if light {
		key += "-light"
	}
	return filepath.Join(s.dir, key)
}

func write(p string, b []byte) error {
	tmp := p + ".new"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func etag(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}

// save stores the files and the row. dark nil means no icon (the letter shows).
func (s *Service) save(ctx context.Context, a App, mode, ref string, dark, light *Found) (db.AppIcon, error) {
	row := db.AppIcon{App: a.Key, Mode: mode, LibraryRef: ref, CheckedAt: s.now().UnixMilli()}
	if a.ServiceID != "" {
		row.ServiceID = sql.NullString{String: a.ServiceID, Valid: true}
	} else {
		row.DnsRecordID = sql.NullString{String: a.DNSID, Valid: true}
	}
	for _, f := range []struct {
		in       *Found
		light    bool
		ct, etag *string
	}{{dark, false, &row.ContentType, &row.Etag}, {light, true, &row.LightType, &row.LightEtag}} {
		if f.in == nil {
			if err := os.Remove(s.path(a.Key, f.light)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return db.AppIcon{}, err
			}
			continue
		}
		if err := write(s.path(a.Key, f.light), f.in.Data); err != nil {
			return db.AppIcon{}, fmt.Errorf("store icon: %w", err)
		}
		*f.ct, *f.etag = f.in.ContentType, etag(f.in.Data)
	}
	if dark != nil {
		row.Found = dark.From
	}
	err := s.st.Q.UpsertAppIcon(ctx, db.UpsertAppIconParams(row))
	return row, err
}

// Upload sets an app's icon to an uploaded image.
func (s *Service) Upload(ctx context.Context, key string, data []byte) (db.AppIcon, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, err := s.app(ctx, key)
	if err != nil {
		return db.AppIcon{}, err
	}
	f, err := found(data, "upload")
	if err != nil {
		return db.AppIcon{}, err
	}
	return s.save(ctx, a, ModeUpload, "", &f, nil)
}

// UseLibrary sets an app's icon to a selfh.st icon, with its white variant for dark themes when there is one.
func (s *Service) UseLibrary(ctx context.Context, key, ref string) (db.AppIcon, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, err := s.app(ctx, key)
	if err != nil {
		return db.AppIcon{}, err
	}
	h, err := s.Lib.Lookup(ctx, ref)
	if err != nil {
		return db.AppIcon{}, err
	}
	dark, light, err := s.Lib.Fetch(ctx, h)
	if err != nil {
		return db.AppIcon{}, err
	}
	return s.save(ctx, a, ModeLibrary, ref, &dark, light)
}

// Auto looks for the app's own icon now; with none found, the app shows its letter until the next look.
func (s *Service) Auto(ctx context.Context, key string) (db.AppIcon, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, err := s.app(ctx, key)
	if err != nil {
		return db.AppIcon{}, err
	}
	return s.auto(ctx, a)
}

func (s *Service) auto(ctx context.Context, a App) (db.AppIcon, error) {
	f, err := s.Find.Find(ctx, Target{Public: a.Public, Upstream: a.Upstream})
	if err != nil {
		if !errors.Is(err, ErrNone) {
			s.Log.Debug("app icon", "app", a.Key, "err", err)
		}
		return s.save(ctx, a, ModeAuto, "", nil, nil)
	}
	return s.save(ctx, a, ModeAuto, "", &f, nil)
}

// Refresh looks again for every automatic icon not checked within a day (and for apps never looked at), then removes
// files whose app is gone.
func (s *Service) Refresh(ctx context.Context) error {
	apps, err := s.Apps(ctx)
	if err != nil {
		return err
	}
	icons, err := s.Icons(ctx)
	if err != nil {
		return err
	}
	for _, a := range apps {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		r, ok := icons[a.Key]
		if ok && (r.Mode != ModeAuto || s.now().Sub(time.UnixMilli(r.CheckedAt)) < recheck) {
			continue
		}
		s.mu.Lock()
		_, err := s.auto(ctx, a)
		s.mu.Unlock()
		if err != nil {
			return err
		}
	}
	icons, err = s.Icons(ctx)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if _, ok := icons[strings.TrimSuffix(e.Name(), "-light")]; !ok {
			_ = os.Remove(filepath.Join(s.dir, e.Name()))
		}
	}
	return nil
}

// Run refreshes once shortly after start and then every hour; each app is looked at no more than once a day.
func (s *Service) Run(ctx context.Context) error {
	t := time.NewTimer(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
		if err := s.Refresh(ctx); err != nil && ctx.Err() == nil {
			s.Log.Warn("app icons", "err", err)
		}
		t.Reset(time.Hour)
	}
}

// Open returns an app icon's file and its stored row; name is the key, with "-light" for the white variant.
func (s *Service) Open(ctx context.Context, name string) (*os.File, string, string, error) {
	key, light := strings.CutSuffix(name, "-light")
	if !keyForm.MatchString(key) {
		return nil, "", "", os.ErrNotExist
	}
	r, err := s.st.R.GetAppIcon(ctx, key)
	if err != nil {
		return nil, "", "", os.ErrNotExist
	}
	ct, tag := r.ContentType, r.Etag
	if light {
		ct, tag = r.LightType, r.LightEtag
	}
	if tag == "" {
		return nil, "", "", os.ErrNotExist
	}
	f, err := os.Open(s.path(key, light))
	return f, ct, tag, err
}

// URLs are where Hashi serves an app's icon and its white variant; empty when there is none.
func URLs(r db.AppIcon) (icon, light string) {
	if r.Etag != "" {
		icon = "/assets/apps/" + r.App + "?v=" + r.Etag
	}
	if r.LightEtag != "" {
		light = "/assets/apps/" + r.App + "-light?v=" + r.LightEtag
	}
	return icon, light
}
