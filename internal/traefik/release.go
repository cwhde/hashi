package traefik

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
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
