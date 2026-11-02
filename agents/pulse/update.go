package main

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"git.juzo.io/juzo/hashi/internal/semver"
)

// maxBinary bounds a download: the agent is a few MB.
const maxBinary = 32 << 20

// newer reports whether Hashi's version is a later release than this agent's. Development builds never update.
func newer(want, have string) bool {
	c, ok := semver.Compare(want, have)
	return ok && c > 0
}

// signedText is what the release signature vouches for (cmd/pulsesign).
func signedText(version, arch string, sum []byte) []byte {
	return []byte("hashi-pulse " + version + " " + arch + " " + hex.EncodeToString(sum))
}

// update replaces this agent's file with release version from base (ADR 0003): the download must carry a valid
// signature for that version and this architecture, and must run and name that version, before it takes this file's
// place. Anything else leaves this file as it is.
func update(ctx context.Context, base, version, self string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	name := "hashi-pulse-linux-" + runtime.GOARCH
	sigRes, err := roundTrip(ctx, request{Method: "GET", URL: base + "/" + name + ".sig"}, nil)
	if err != nil {
		return err
	}
	if sigRes.Status != 200 {
		return fmt.Errorf("signature: HTTP %d", sigRes.Status)
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(sigRes.Body))
	if err != nil {
		return errors.New("signature: not base64")
	}
	tmp := filepath.Join(filepath.Dir(self), ".hashi-pulse.new")
	_ = os.Remove(tmp)
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //nolint:gosec // next to this agent's own file
	if err != nil {
		return err
	}
	done := false
	defer func() {
		if !done {
			_ = os.Remove(tmp)
		}
	}()
	h := sha256.New()
	err = download(ctx, base+"/"+name, io.MultiWriter(f, h))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if !ed25519.Verify(releasePublicKey(), signedText(version, runtime.GOARCH, h.Sum(nil)), sig) {
		return errors.New("the signature does not match this release")
	}
	if err := os.Chmod(tmp, 0o755); err != nil { //nolint:gosec // a program, readable like the one it replaces
		return err
	}
	out, err := exec.CommandContext(ctx, tmp, "version").Output() //nolint:gosec // the file whose signature was just checked
	if err != nil {
		return fmt.Errorf("the new file does not run here: %w", err)
	}
	if got := strings.TrimSpace(string(out)); got != version {
		return fmt.Errorf("the new file says it is %q", got)
	}
	if err := os.Rename(tmp, self); err != nil {
		return err
	}
	done = true
	return nil
}

// trustedKey is releaseKey; tests sign with their own.
var trustedKey = releaseKey

func releasePublicKey() ed25519.PublicKey {
	k, _ := base64.StdEncoding.DecodeString(trustedKey) // a constant, checked by a test
	return k
}

// download streams a file from url into w. Release files come with their length.
func download(ctx context.Context, url string, w io.Writer) error {
	conn, _, err := open(ctx, request{Method: "GET", URL: url}, nil)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	br := bufio.NewReader(conn)
	line, err := br.ReadString('\n')
	if err != nil {
		return err
	}
	f := strings.Fields(line)
	if len(f) < 2 || !strings.HasPrefix(f[0], "HTTP/1.") {
		return errors.New("not an HTTP answer")
	}
	if f[1] != "200" {
		return fmt.Errorf("download: HTTP %s", f[1])
	}
	length := int64(-1)
	for {
		l, err := br.ReadString('\n')
		if err != nil {
			return err
		}
		l = strings.TrimRight(l, "\r\n")
		if l == "" {
			break
		}
		k, v, _ := strings.Cut(l, ":")
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "content-length":
			if length, err = strconv.ParseInt(strings.TrimSpace(v), 10, 64); err != nil {
				return errors.New("bad download length")
			}
		case "transfer-encoding":
			return errors.New("the download has no length")
		}
	}
	if length < 0 || length > maxBinary {
		return fmt.Errorf("download of %d bytes refused", length)
	}
	n, err := io.CopyN(w, br, length)
	if err != nil {
		return fmt.Errorf("download cut off after %d of %d bytes: %w", n, length, err)
	}
	return nil
}
