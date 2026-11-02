package traefik

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"git.juzo.io/juzo/hashi/internal/sshx"
)

// The Coraza WAF plugin (06 §9), pinned by version and by the sha256 of the catalog archive and of each file Hashi
// puts on a host. Traefik loads it as a local plugin from its working directory: experimental.plugins downloads again
// at every start and leaves every WAF router at 404 without network (testdata/contracts/coraza/plugin_load.txt).
const (
	PluginName    = "coraza"
	PluginModule  = "github.com/jcchavezs/coraza-http-wasm-traefik"
	PluginVersion = "v0.3.0"
	// PluginURL is the catalog download; the GitHub release asset has the same bytes.
	PluginURL        = "https://plugins.traefik.io/public/download/" + PluginModule + "/" + PluginVersion
	pluginArchiveSum = "b96680b46e61287faae243e360db5de34e4f315e2a8ee105fedddc176e79b1e7" // hashi:allow-secret-pattern (sha256 of a public file)
	maxPluginArchive = 32 << 20
)

// PluginDir is where the plugin's files live: <working directory>/plugins-local/src/<module>/.
const PluginDir = StateDir + "/plugins-local/src/" + PluginModule

// pluginFiles are the archive members Traefik needs, with their pinned hashes.
var pluginFiles = []struct{ name, sum string }{
	{".traefik.yml", "124872e1119a0bf0810dcab9a1c35b996330f4184eaa6b357feaee40d22c7ff7"},          // hashi:allow-secret-pattern (sha256 of a public file)
	{"coraza-http-wasm.wasm", "b59fd2266c03699da2de9d6d281ffc2a3e791eb6646069d2b9744543ccd90b1c"}, // hashi:allow-secret-pattern (sha256 of a public file)
}

// renderPluginFiles are the plugin's files for a render. Their content is fetched only when a host needs them
// (Desired.Plugin), so a plan never downloads; the pinned hash stands in for the content until then.
func renderPluginFiles() []File {
	out := make([]File, 0, len(pluginFiles))
	for _, f := range pluginFiles {
		out = append(out, File{Path: PluginDir + "/" + f.name, Kind: KindPlugin, Mode: 0o644, Sum: f.sum})
	}
	return out
}

// PluginArchive checks the catalog archive against its pinned hash and returns the plugin's files by host path.
func PluginArchive(archive []byte) (map[string][]byte, error) {
	return pluginArchive(archive, pluginArchiveSum)
}

func pluginArchive(archive []byte, want string) (map[string][]byte, error) {
	got := sha256.Sum256(archive)
	if hex.EncodeToString(got[:]) != want {
		return nil, fmt.Errorf("%w: WAF plugin %s", ErrChecksum, PluginVersion)
	}
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("open WAF plugin archive: %w", err)
	}
	out := map[string][]byte{}
	for _, f := range pluginFiles {
		var member *zip.File
		for _, z := range zr.File {
			if path.Clean(z.Name) == f.name {
				member = z
			}
		}
		if member == nil {
			return nil, fmt.Errorf("WAF plugin archive has no %s", f.name)
		}
		r, err := member.Open()
		if err != nil {
			return nil, fmt.Errorf("WAF plugin %s: %w", f.name, err)
		}
		b, err := io.ReadAll(io.LimitReader(r, maxPluginArchive+1))
		_ = r.Close()
		if err != nil {
			return nil, fmt.Errorf("WAF plugin %s: %w", f.name, err)
		}
		if s := sha256.Sum256(b); hex.EncodeToString(s[:]) != f.sum {
			return nil, fmt.Errorf("%w: WAF plugin file %s", ErrChecksum, f.name)
		}
		out[PluginDir+"/"+f.name] = b
	}
	return out, nil
}

// DownloadPlugin fetches the pinned plugin archive from url ("" = PluginURL) and returns its verified files.
func DownloadPlugin(ctx context.Context, client *http.Client, url string) (map[string][]byte, error) {
	if url == "" {
		url = PluginURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download WAF plugin: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download WAF plugin: HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxPluginArchive+1))
	if err != nil {
		return nil, fmt.Errorf("download WAF plugin: %w", err)
	}
	if len(b) > maxPluginArchive {
		return nil, fmt.Errorf("download WAF plugin: larger than %d bytes", maxPluginArchive)
	}
	return PluginArchive(b)
}

// PinnedPluginSum is the pinned hash of the plugin file at host path p.
func PinnedPluginSum(p string) (string, bool) {
	for _, f := range pluginFiles {
		if PluginDir+"/"+f.name == p {
			return f.sum, true
		}
	}
	return "", false
}

// remoteSum hashes a file on the host with sha256sum (busybox and coreutils print "<hex>  <path>"), so a plan does not
// read the 3 MB plugin over SSH. "" when the file does not exist.
func remoteSum(ctx context.Context, h Host, p string) (string, error) {
	res, err := h.Run(ctx, "sha256sum "+sshx.Quote(p)+" 2>/dev/null || true", nil)
	if err != nil {
		return "", fmt.Errorf("hash %s: %w", p, err)
	}
	sum, _, _ := strings.Cut(strings.TrimSpace(string(res.Stdout)), " ")
	if len(sum) != 64 {
		return "", nil
	}
	return sum, nil
}

// cachedPlugin downloads the plugin on first use and keeps it; a failed download is tried again next time.
func cachedPlugin() func(ctx context.Context) (map[string][]byte, error) {
	var mu sync.Mutex
	var files map[string][]byte
	client := &http.Client{Timeout: 2 * time.Minute}
	return func(ctx context.Context) (map[string][]byte, error) {
		mu.Lock()
		defer mu.Unlock()
		if files != nil {
			return files, nil
		}
		f, err := DownloadPlugin(ctx, client, "")
		if err != nil {
			return nil, err
		}
		files = f
		return files, nil
	}
}
