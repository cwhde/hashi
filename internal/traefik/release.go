package traefik

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Version is the Traefik release Hashi installs (04 §2: official release binary, pinned, checksum-verified).
const Version = "v3.7.16"

// MaxBinary bounds the extracted binary; it only stops a runaway archive.
const MaxBinary = 512 << 20

//go:embed release/*.sha256
var checksumFS embed.FS

// AssetName is the release archive for arch, in the checksum file's format `traefik_<tag>_<os>_<arch>.tar.gz`.
func AssetName(arch string) string { return fmt.Sprintf("traefik_%s_linux_%s.tar.gz", Version, arch) }

// ReleaseURL is where Hashi downloads the archive from (GitHub release assets, testdata/contracts/traefik/README.md).
func ReleaseURL(arch string) string {
	return "https://github.com/traefik/traefik/releases/download/" + Version + "/" + AssetName(arch)
}

// Checksum is the embedded SHA-256 (hex) of the pinned release archive for arch.
func Checksum(arch string) (string, error) {
	f, err := checksumFS.Open("release/traefik_" + Version + ".sha256")
	if err != nil {
		return "", fmt.Errorf("embedded checksums for %s: %w", Version, err)
	}
	defer func() { _ = f.Close() }()
	want := AssetName(arch)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		sum, name, ok := strings.Cut(sc.Text(), "  ")
		if ok && name == want {
			return sum, nil
		}
	}
	return "", fmt.Errorf("%w: no %s release for linux/%s", ErrUnsupported, Version, arch)
}

// ErrChecksum means the downloaded archive is not the pinned release.
var ErrChecksum = errors.New("release archive checksum mismatch")

// Binary checks archive against the embedded checksum and returns the traefik binary inside it.
func Binary(arch string, archive []byte) ([]byte, error) {
	want, err := Checksum(arch)
	if err != nil {
		return nil, err
	}
	got := sha256.Sum256(archive)
	if hex.EncodeToString(got[:]) != want {
		return nil, fmt.Errorf("%w: %s", ErrChecksum, AssetName(arch))
	}
	return extract(arch, archive)
}

// extract returns the regular file "traefik" from a release archive.
func extract(arch string, archive []byte) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", AssetName(arch), err)
	}
	tr := tar.NewReader(zr)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("%s has no traefik binary", AssetName(arch))
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", AssetName(arch), err)
		}
		if h.Typeflag != tar.TypeReg || h.Name != "traefik" {
			continue
		}
		b, err := io.ReadAll(io.LimitReader(tr, MaxBinary+1))
		if err != nil {
			return nil, fmt.Errorf("extract traefik: %w", err)
		}
		if len(b) > MaxBinary {
			return nil, errors.New("traefik binary exceeds the size limit")
		}
		return b, nil
	}
}

// MaxArchive bounds the download.
const MaxArchive = 256 << 20

// downloadAttempts is how often a temporary failure (network, 5xx, 429, a broken body) is tried before the plan
// fails (bug 9). A missing file or a wrong checksum fails at once: trying again can't fix them.
const downloadAttempts = 3

// Download fetches the pinned release archive for arch from baseURL ("" = ReleaseURL) and returns the verified binary.
func Download(ctx context.Context, client *http.Client, baseURL, arch string) ([]byte, error) {
	return download(ctx, client, baseURL, arch, func(attempt int) time.Duration { return time.Duration(attempt) * 5 * time.Second })
}

func download(ctx context.Context, client *http.Client, baseURL, arch string, wait func(attempt int) time.Duration) ([]byte, error) {
	u := ReleaseURL(arch)
	if baseURL != "" {
		u = strings.TrimSuffix(baseURL, "/") + "/" + AssetName(arch)
	}
	var last error
	for attempt := 1; attempt <= downloadAttempts; attempt++ {
		b, retry, err := fetchArchive(ctx, client, u, arch)
		if err == nil {
			return Binary(arch, b)
		}
		if !retry {
			return nil, err
		}
		last = err
		if attempt < downloadAttempts {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("%w (stopped: %w)", last, ctx.Err())
			case <-time.After(wait(attempt)):
			}
		}
	}
	return nil, fmt.Errorf("%w (after %d attempts)", last, downloadAttempts)
}

// fetchArchive makes one attempt. retry says whether the failure may pass on its own.
func fetchArchive(ctx context.Context, client *http.Client, u, arch string) (b []byte, retry bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, false, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, ctx.Err() == nil, fmt.Errorf("download %s: %w", AssetName(arch), err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		temporary := resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests
		return nil, temporary, fmt.Errorf("download %s: HTTP %d", AssetName(arch), resp.StatusCode)
	}
	b, err = io.ReadAll(io.LimitReader(resp.Body, MaxArchive+1))
	if err != nil {
		return nil, ctx.Err() == nil, fmt.Errorf("download %s: %w", AssetName(arch), err)
	}
	if len(b) > MaxArchive {
		return nil, false, fmt.Errorf("download %s: larger than %d bytes", AssetName(arch), MaxArchive)
	}
	return b, false, nil
}
