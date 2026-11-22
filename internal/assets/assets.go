// Package assets keeps the public pages' icon, logos and background (07 §9): files copied once from the owner's asset
// source or uploaded, stored under <data>/assets and served only by Hashi.
package assets

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Slot names one replaceable public page asset.
type Slot string

const (
	Icon       Slot = "icon"
	LogoDark   Slot = "logo_dark"
	LogoLight  Slot = "logo_light"
	Background Slot = "background"
)

// Slots lists every slot in display order.
var Slots = []Slot{Icon, LogoDark, LogoLight, Background}

// Valid reports whether s is a known slot.
func (s Slot) Valid() bool { return slices.Contains(Slots, s) }

// Limit is the largest file a slot takes (07 §9): a background is a photo, the rest are small.
func (s Slot) Limit() int64 {
	if s == Background {
		return 5 << 20
	}
	return 1 << 20
}

// ErrInvalid marks input the owner can fix (wrong type, too large, bad path); the message says what.
var ErrInvalid = errors.New("invalid asset")

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// Types are the image types a slot takes, by extension.
var Types = map[string]string{".png": "image/png", ".webp": "image/webp", ".svg": "image/svg+xml", ".ico": "image/x-icon"}

// Sniff returns the content type of data when it is one of Types, judged by its bytes, not by a name or header.
func Sniff(data []byte) (string, error) {
	switch ct := http.DetectContentType(data); ct {
	case "image/png", "image/webp", "image/x-icon", "image/vnd.microsoft.icon":
		if ct == "image/vnd.microsoft.icon" {
			ct = "image/x-icon"
		}
		return ct, nil
	}
	if isSVG(data) {
		return "image/svg+xml", nil
	}
	return "", invalid("only PNG, WebP, SVG or ICO images")
}

var svgStart = regexp.MustCompile(`(?is)^\s*(<\?xml[^>]*>\s*)?(<!--.*?-->\s*)*(<!doctype svg[^>]*>\s*)?<svg[\s>]`)

func isSVG(data []byte) bool {
	return svgStart.Match(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")))
}

// Store holds the slot files in one directory. A slot's file is written whole to a temporary name and renamed, so a
// reader never sees half a file.
type Store struct{ dir string }

// NewStore uses dir, creating it when needed.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("asset folder: %w", err)
	}
	return &Store{dir: dir}, nil
}

// Put checks data against the slot's limit and types and stores it. It returns the content type and an ETag.
func (s *Store) Put(slot Slot, data []byte) (contentType, etag string, err error) {
	if !slot.Valid() {
		return "", "", invalid("unknown slot %q", slot)
	}
	if int64(len(data)) > slot.Limit() {
		return "", "", invalid("larger than %d MB", slot.Limit()>>20)
	}
	ct, err := Sniff(data)
	if err != nil {
		return "", "", err
	}
	// Uploads and files copied from the source both land here, so both are stored at the size pages show.
	data, ct = fit(slot, data, ct)
	tmp, err := os.CreateTemp(s.dir, "."+string(slot)+"-*")
	if err != nil {
		return "", "", fmt.Errorf("store %s: %w", slot, err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return "", "", fmt.Errorf("store %s: %w", slot, err)
	}
	if err := tmp.Close(); err != nil {
		return "", "", fmt.Errorf("store %s: %w", slot, err)
	}
	if err := os.Rename(tmp.Name(), s.path(slot)); err != nil {
		return "", "", fmt.Errorf("store %s: %w", slot, err)
	}
	sum := sha256.Sum256(data)
	return ct, hex.EncodeToString(sum[:8]), nil
}

// Remove empties a slot. An empty slot stays empty.
func (s *Store) Remove(slot Slot) error {
	if err := os.Remove(s.path(slot)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", slot, err)
	}
	return nil
}

// Open returns the slot's file, or an error wrapping os.ErrNotExist when the slot is empty.
func (s *Store) Open(slot Slot) (*os.File, error) {
	if !slot.Valid() {
		return nil, os.ErrNotExist
	}
	return os.Open(s.path(slot))
}

func (s *Store) path(slot Slot) string { return filepath.Join(s.dir, string(slot)) }

// Source reads the owner's asset source: a plain web server with folder listings (captured: Apache autoindex).
type Source struct {
	Client *http.Client
}

// Listing is one folder of the source: its subfolders and the image files Hashi can use.
type Listing struct {
	Dirs  []string
	Files []string
}

// CleanDir turns a folder the owner picked into a safe relative path ending in "/", or "" for the root.
func CleanDir(dir string) (string, error) {
	if strings.Contains(dir, "\\") || strings.Contains(dir, "..") {
		return "", invalid("folder %q", dir)
	}
	d := strings.Trim(path.Clean("/"+dir), "/")
	if d == "" {
		return "", nil
	}
	return d + "/", nil
}

// CleanFile turns a file path the owner picked into a safe relative path with an image extension.
func CleanFile(p string) (string, error) {
	if strings.Contains(p, "\\") || strings.Contains(p, "..") || strings.HasSuffix(p, "/") {
		return "", invalid("file %q", p)
	}
	f := strings.TrimPrefix(path.Clean("/"+p), "/")
	if _, ok := Types[strings.ToLower(path.Ext(f))]; !ok || f == "" {
		return "", invalid("file %q is not a PNG, WebP, SVG or ICO image", p)
	}
	return f, nil
}

func join(base, rel string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", invalid("the asset source must be an http or https address")
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + "/" + rel
	u.RawQuery, u.Fragment = "", ""
	return u.String(), nil
}

const fetchTimeout = 20 * time.Second

func (s Source) get(ctx context.Context, u string, limit int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "hashi/assets")
	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("asset source: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, invalid("the asset source answered %d for %s", resp.StatusCode, req.URL.Path)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("asset source: %w", err)
	}
	if int64(len(b)) > limit {
		return nil, invalid("larger than %d MB", limit>>20)
	}
	return b, nil
}

// List reads one folder of the source.
func (s Source) List(ctx context.Context, base, dir string) (Listing, error) {
	d, err := CleanDir(dir)
	if err != nil {
		return Listing{}, err
	}
	u, err := join(base, d)
	if err != nil {
		return Listing{}, err
	}
	page, err := s.get(ctx, u, 2<<20)
	if err != nil {
		return Listing{}, err
	}
	return ParseListing(page), nil
}

// Fetch downloads one file of the source for slot.
func (s Source) Fetch(ctx context.Context, base, file string, slot Slot) ([]byte, error) {
	f, err := CleanFile(file)
	if err != nil {
		return nil, err
	}
	u, err := join(base, f)
	if err != nil {
		return nil, err
	}
	return s.get(ctx, u, slot.Limit())
}

var (
	href = regexp.MustCompile(`(?i)<a\s[^>]*href\s*=\s*"([^"]+)"`)
	// A listing may carry a header with scripts that build links of their own (captured); only markup counts.
	scripts = regexp.MustCompile(`(?is)<(script|style)\b.*?</(script|style)\s*>`)
)

// ParseListing takes a folder listing page (Apache or nginx autoindex) and keeps the entries inside this folder:
// subfolders, and files with an image extension. Sort links (`?C=N;O=D`), parent and absolute links are left out.
func ParseListing(page []byte) Listing {
	var l Listing
	seen := map[string]bool{}
	for _, m := range href.FindAllSubmatch(scripts.ReplaceAll(page, nil), -1) {
		h, err := url.PathUnescape(string(m[1]))
		if err != nil || seen[h] || h == "" || strings.ContainsAny(h, "?#:") || strings.HasPrefix(h, "/") ||
			strings.HasPrefix(h, ".") {
			continue
		}
		seen[h] = true
		name := strings.TrimSuffix(h, "/")
		if strings.Contains(name, "/") {
			continue
		}
		if strings.HasSuffix(h, "/") {
			l.Dirs = append(l.Dirs, name)
		} else if _, ok := Types[strings.ToLower(path.Ext(name))]; ok {
			l.Files = append(l.Files, name)
		}
	}
	return l
}

// Refit stores a slot's file again at its shown size (logos stored before Hashi fitted them, 0.13.14). changed is
// false when the file was fitted already, or the slot is empty.
func (s *Store) Refit(slot Slot) (contentType, etag string, changed bool, err error) {
	data, err := os.ReadFile(s.path(slot))
	if errors.Is(err, os.ErrNotExist) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("refit %s: %w", slot, err)
	}
	ct, err := Sniff(data)
	if err != nil {
		return "", "", false, nil //nolint:nilerr // a file Hashi can't read is served as it is
	}
	if out, _ := fit(slot, data, ct); len(out) == len(data) {
		return "", "", false, nil
	}
	ct, etag, err = s.Put(slot, data)
	return ct, etag, err == nil, err
}
