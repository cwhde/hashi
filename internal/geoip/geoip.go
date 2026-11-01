// Package geoip reads the GeoLite2 databases and keeps them current (06 §12): lookups for rules at L7 and prefix lists for
// the firewall (05a §4.3). A new file is checked before it replaces the old one; failure keeps what is there.
package geoip

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oschwald/maxminddb-golang/v2"

	"git.juzo.io/juzo/hashi/internal/ipsets"
)

// Editions Hashi downloads, and the job's interval.
const (
	Country     = "GeoLite2-Country"
	ASN         = "GeoLite2-ASN"
	JobKind     = "geoip.update"
	JobInterval = 24 * time.Hour
	// UpdateEvery is the "weekly" of 06 §10; StaleAfter the warning of 06 §12.
	UpdateEvery = 7 * 24 * time.Hour
	StaleAfter  = 14 * 24 * time.Hour

	maxDownload  = 256 << 20
	maxmindURL   = "https://download.maxmind.com/geoip/databases/{edition}/download?suffix=tar.gz"
	maxmindSumOf = "https://download.maxmind.com/geoip/databases/{edition}/download?suffix=tar.gz.sha256"
)

// Source says where the files come from. With an account, MaxMind; otherwise URL with {edition} in it.
type Source struct {
	AccountID  string
	LicenseKey string
	URL        string
}

// Configured reports whether downloads can run.
func (s Source) Configured() bool { return s.URL != "" || (s.AccountID != "" && s.LicenseKey != "") }

// Status describes one database.
type Status struct {
	Edition string
	Build   time.Time
	Fetched time.Time
}

// Service owns the open readers.
type Service struct {
	dir    string
	client *http.Client
	now    func() time.Time

	mu      sync.RWMutex
	readers map[string]*maxminddb.Reader
	status  map[string]Status
	cache   map[ipsets.Geo][]netip.Prefix
	gen     int
}

// New opens what is already on disk under dir.
func New(dir string, client *http.Client, now func() time.Time) *Service {
	s := &Service{dir: dir, client: client, now: now, readers: map[string]*maxminddb.Reader{}, status: map[string]Status{}, cache: map[ipsets.Geo][]netip.Prefix{}}
	for _, ed := range []string{Country, ASN} {
		_ = s.open(ed) // no file yet: lookups answer "unavailable"
	}
	return s
}

func (s *Service) path(edition string) string { return filepath.Join(s.dir, edition+".mmdb") }

func (s *Service) open(edition string) error {
	p := s.path(edition)
	r, err := maxminddb.Open(p)
	if err != nil {
		return err
	}
	st := Status{Edition: edition, Build: time.Unix(int64(r.Metadata.BuildEpoch), 0).UTC()} //nolint:gosec // a timestamp in the file
	if fi, err := os.Stat(p); err == nil {
		st.Fetched = fi.ModTime().UTC()
	}
	s.mu.Lock()
	old := s.readers[edition]
	s.readers[edition], s.status[edition] = r, st
	s.cache = map[ipsets.Geo][]netip.Prefix{}
	s.gen++
	s.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	return nil
}

// Statuses lists the databases on hand.
func (s *Service) Statuses() []Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Status, 0, len(s.status))
	for _, ed := range []string{Country, ASN} {
		if st, ok := s.status[ed]; ok {
			out = append(out, st)
		}
	}
	return out
}

// Available reports whether the database a predicate needs is open. Regions need a City database, which Hashi does not
// download yet.
func (s *Service) Available(g ipsets.Geo) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.readers[editionOf(g)] != nil
}

func editionOf(g ipsets.Geo) string {
	if g.Kind == ipsets.GeoASN {
		return ASN
	}
	if g.Kind == ipsets.GeoRegion {
		return "GeoLite2-City"
	}
	return Country
}

type countryRecord struct {
	Country struct {
		ISOCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
}

type asnRecord struct {
	Number uint `maxminddb:"autonomous_system_number"`
}

// Lookup answers for one address; ok is false with no database at all.
func (s *Service) Lookup(a netip.Addr) (country, region string, asn uint, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if r := s.readers[Country]; r != nil {
		var rec countryRecord
		if r.Lookup(a).Decode(&rec) == nil {
			country = rec.Country.ISOCode
		}
		ok = true
	}
	if r := s.readers[ASN]; r != nil {
		var rec asnRecord
		if r.Lookup(a).Decode(&rec) == nil {
			asn = rec.Number
		}
		ok = true
	}
	return country, region, asn, ok
}

// Prefixes lists every network of a predicate, cached until a database changes (05a §4.3).
func (s *Service) Prefixes(g ipsets.Geo) ([]netip.Prefix, error) {
	s.mu.RLock()
	if c, ok := s.cache[g]; ok {
		s.mu.RUnlock()
		return c, nil
	}
	r, gen := s.readers[editionOf(g)], s.gen
	s.mu.RUnlock()
	if r == nil {
		return nil, errors.New("GeoIP database is not available")
	}
	var out []netip.Prefix
	s.mu.RLock()
	for res := range r.Networks() {
		if err := res.Err(); err != nil {
			s.mu.RUnlock()
			return nil, fmt.Errorf("read GeoIP database: %w", err)
		}
		var hit bool
		if g.Kind == ipsets.GeoASN {
			var rec asnRecord
			hit = res.Decode(&rec) == nil && g.Matches("", "", rec.Number)
		} else {
			var rec countryRecord
			hit = res.Decode(&rec) == nil && g.Matches(rec.Country.ISOCode, "", 0)
		}
		if hit {
			p := res.Prefix()
			if p.Addr().Is4In6() {
				p = netip.PrefixFrom(p.Addr().Unmap(), p.Bits()-96)
			}
			out = append(out, p.Masked())
		}
	}
	s.mu.RUnlock()
	s.mu.Lock()
	if s.gen == gen {
		s.cache[g] = out
	}
	s.mu.Unlock()
	return out, nil
}

// Update downloads any edition that is missing or older than UpdateEvery (force: all). It reports what changed.
func (s *Service) Update(ctx context.Context, src Source, force bool) (changed bool, err error) {
	if !src.Configured() {
		return false, nil
	}
	var errs []error
	for _, ed := range []string{Country, ASN} {
		s.mu.RLock()
		st, have := s.status[ed]
		s.mu.RUnlock()
		if have && !force && s.now().Sub(st.Fetched) < UpdateEvery {
			continue
		}
		if err := s.fetch(ctx, src, ed); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", ed, err))
			continue
		}
		changed = true
	}
	return changed, errors.Join(errs...)
}

func (s *Service) request(ctx context.Context, src Source, tmpl, edition string) (*http.Response, error) {
	u := strings.ReplaceAll(tmpl, "{edition}", edition)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if src.URL == "" {
		req.SetBasicAuth(src.AccountID, src.LicenseKey)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		// http.Client errors quote the URL; MaxMind's request carries its login separately, but keep logs to the cause.
		var ue interface{ Unwrap() error }
		if errors.As(err, &ue) {
			return nil, fmt.Errorf("download failed: %w", ue.Unwrap())
		}
		return nil, errors.New("download failed")
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("the download answered %d", resp.StatusCode)
	}
	return resp, nil
}

func (s *Service) fetch(ctx context.Context, src Source, edition string) error {
	tmpl := src.URL
	if tmpl == "" {
		tmpl = maxmindURL
	}
	resp, err := s.request(ctx, src, tmpl, edition)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	archive, err := os.CreateTemp(s.dir, edition+"-*.tar.gz")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(archive.Name()) }()
	defer func() { _ = archive.Close() }()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(archive, h), io.LimitReader(resp.Body, maxDownload+1))
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	if n > maxDownload {
		return errors.New("the download is too large")
	}
	if src.URL == "" { // MaxMind publishes a checksum (06 §12)
		sum, err := s.request(ctx, src, maxmindSumOf, edition)
		if err != nil {
			return fmt.Errorf("checksum: %w", err)
		}
		defer func() { _ = sum.Body.Close() }()
		b, err := io.ReadAll(io.LimitReader(sum.Body, 4096))
		if err != nil {
			return fmt.Errorf("checksum: %w", err)
		}
		fields := strings.Fields(string(b))
		if len(fields) == 0 || !strings.EqualFold(fields[0], hex.EncodeToString(h.Sum(nil))) {
			return errors.New("the download does not match its checksum")
		}
	}
	if _, err := archive.Seek(0, io.SeekStart); err != nil {
		return err
	}
	return s.install(archive, edition)
}

// install takes the .mmdb out of the archive, proves it opens as the right edition, then swaps it in atomically.
func (s *Service) install(r io.Reader, edition string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("the download is not a gzip archive: %w", err)
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		hd, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return errors.New("the archive has no database")
		}
		if err != nil {
			return fmt.Errorf("read the archive: %w", err)
		}
		if hd.Typeflag != tar.TypeReg || filepath.Base(hd.Name) != edition+".mmdb" {
			continue
		}
		tmp, err := os.CreateTemp(s.dir, edition+"-*.mmdb")
		if err != nil {
			return err
		}
		defer func() { _ = os.Remove(tmp.Name()) }()
		if _, err := io.Copy(tmp, io.LimitReader(tr, maxDownload)); err != nil {
			_ = tmp.Close()
			return fmt.Errorf("unpack: %w", err)
		}
		if err := tmp.Close(); err != nil {
			return err
		}
		check, err := maxminddb.Open(tmp.Name())
		if err != nil {
			return fmt.Errorf("the database does not open: %w", err)
		}
		got := check.Metadata.DatabaseType
		_ = check.Close()
		if got != edition {
			return fmt.Errorf("the file is %q, expected %q", got, edition)
		}
		if err := os.Rename(tmp.Name(), s.path(edition)); err != nil {
			return err
		}
		return s.open(edition)
	}
}
