package pulse

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultDir holds the agent binaries in the Hashi image, so an install needs only Hashi itself.
const DefaultDir = "/usr/local/share/hashi/pulse"

// ServeDownload serves the agent binary for an architecture ("amd64", "arm64") and its checksum ("amd64.sha256").
func ServeDownload(dir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		file := r.PathValue("file")
		arch, sum := strings.CutSuffix(file, ".sha256")
		if arch != "amd64" && arch != "arm64" {
			http.NotFound(w, r)
			return
		}
		path := filepath.Join(dir, "hashi-pulse-linux-"+arch)
		f, err := os.Open(path) //nolint:gosec // arch is one of two fixed values
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer func() { _ = f.Close() }()
		if sum {
			h := sha256.New()
			if _, err := io.Copy(h, f); err != nil {
				http.Error(w, "read failed", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(hex.EncodeToString(h.Sum(nil)) + "\n"))
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeContent(w, r, "", fileTime(f), f)
	}
}

func fileTime(f *os.File) (t time.Time) {
	if st, err := f.Stat(); err == nil {
		t = st.ModTime()
	}
	return t
}
