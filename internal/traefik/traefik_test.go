package traefik

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"git.juzo.io/juzo/hashi/internal/secrets"
)

// fakeKey is not a key, only PEM-shaped bytes for the render tests.
const fakeKey = "-----BEGIN PRIVATE KEY-----\nBB==\n-----END PRIVATE KEY-----\n" // hashi:allow-secret-pattern

var update = flag.Bool("update", false, "rewrite golden files")

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	p := filepath.Join("testdata", "golden", name)
	if *update {
		if err := os.WriteFile(p, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/traefik -update)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from golden file:\n%s", name, got)
	}
}

func TestParseProbeFixtures(t *testing.T) {
	cases := []struct {
		file                 string
		family, pm, init, id string
		binary               string
	}{
		{"os_alpine.txt", FamilyAlpine, "apk", InitOpenRC, "alpine", "/usr/local/bin/traefik"},
		{"os_debian.txt", FamilyDebian, "apt-get", InitSystemd, "debian", ""},
		{"os_debian13.txt", FamilyDebian, "apt-get", InitSystemd, "debian", ""},
		{"os_fedora44.txt", FamilyFedora, "dnf", InitSystemd, "fedora", ""},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "contracts", "os", c.file))
			if err != nil {
				t.Fatal(err)
			}
			p, err := ParseProbe(string(b))
			if err != nil {
				t.Fatal(err)
			}
			if p.Family != c.family || p.PackageManager != c.pm || p.Init != c.init || p.ID != c.id ||
				p.Arch != "amd64" || !p.Root || p.ExistingBinary != c.binary {
				t.Errorf("got %+v", p)
			}
		})
	}
}

func TestParseProbeRefusesUnknownHosts(t *testing.T) {
	base := "ID=void\n--- uname\nx86_64\n--- id\n0\n--- pm\n%PM%--- init\n%INIT%--- traefik\n--- end\n"
	for name, c := range map[string][2]string{
		"no package manager": {"", "/usr/bin/systemctl\n"},
		"no init system":     {"/usr/bin/apt-get\n", ""},
		"openrc-run only":    {"/sbin/apk\n", "/sbin/openrc-run\n"},
	} {
		out := strings.NewReplacer("%PM%", c[0], "%INIT%", c[1]).Replace(base)
		if _, err := ParseProbe(out); !errors.Is(err, ErrUnsupported) {
			t.Errorf("%s: err = %v, want ErrUnsupported", name, err)
		}
	}
	if _, err := ParseProbe("ID=debian\n--- uname\nx86_64\n"); err == nil {
		t.Error("truncated output accepted")
	}
	if _, err := ParseProbe("ID=debian\n--- uname\nmips\n--- end\n"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("unknown arch: %v", err)
	}
	p, err := ParseProbe("ID=x\n--- uname\naarch64\n--- id\n1000\n--- pm\n/usr/bin/dnf\n--- init\n/usr/bin/systemctl\n" +
		"--- traefik\n/usr/bin/traefik\n/etc/traefik\n--- end\n")
	if err != nil || p.Root || p.Arch != "arm64" || p.ExistingBinary != "/usr/bin/traefik" ||
		!slices.Equal(p.ExistingConfigs, []string{"/etc/traefik"}) {
		t.Errorf("got %+v, %v", p, err)
	}
}

func TestReleaseChecksums(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64", "armv7"} {
		sum, err := Checksum(arch)
		if err != nil || len(sum) != 64 {
			t.Errorf("%s: %q %v", arch, sum, err)
		}
	}
	if _, err := Checksum("mips"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("mips: %v", err)
	}
	if got := ReleaseURL("amd64"); got != "https://github.com/traefik/traefik/releases/download/v3.7.16/traefik_v3.7.16_linux_amd64.tar.gz" {
		t.Error(got)
	}
	if _, err := Binary("amd64", []byte("not the release")); !errors.Is(err, ErrChecksum) {
		t.Errorf("tampered archive: %v", err)
	}
}

func TestExtractFindsTheBinary(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for name, body := range map[string]string{"LICENSE.md": "license", "traefik": "ELF"} {
		_ = tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg})
		_, _ = tw.Write([]byte(body))
	}
	_ = tw.Close()
	_ = zw.Close()
	b, err := extract("amd64", buf.Bytes())
	if err != nil || string(b) != "ELF" {
		t.Fatalf("%q %v", b, err)
	}
}

func testOptions() Options {
	return Options{
		Paths:   Paths{Binary: DefaultBinary, ConfigDir: DefaultConfigDir},
		APIPort: 8080, ForwardAuthPort: 8081, HashiURL: "http://192.0.2.5:3340/",
		Certs: []Cert{{Name: "example.com", CertPEM: []byte("-----BEGIN CERTIFICATE-----\nAA==\n-----END CERTIFICATE-----\n"),
			KeyPEM: secrets.New(fakeKey)}},
	}
}

func TestRenderGolden(t *testing.T) {
	files, err := Render(testOptions())
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, f := range files {
		order = append(order, f.Kind+" "+f.Path)
		switch {
		case f.Kind == KindCert:
		case strings.HasSuffix(f.Path, "hashi-certs.yaml"):
			// Not a golden file: its hash line would look like a token to the repository's secret guard.
			sum := sha256.Sum256(files[0].Data)
			want := generated + "# example.com.crt sha256 " + hex.EncodeToString(sum[:]) + "\n" +
				"tls:\n  certificates:\n    - certFile: /etc/hashi/traefik/certs/example.com.crt\n" +
				"      keyFile: /etc/hashi/traefik/certs/example.com.key\n"
			if string(f.Data) != want {
				t.Errorf("hashi-certs.yaml:\n%s", f.Data)
			}
		default:
			golden(t, filepath.Base(f.Path), f.Data)
		}
	}
	want := []string{
		"cert /etc/hashi/traefik/certs/example.com.crt", "cert /etc/hashi/traefik/certs/example.com.key",
		"dynamic /etc/hashi/traefik/dynamic/hashi-certs.yaml", "dynamic /etc/hashi/traefik/dynamic/00-hashi-core.yml",
		"dynamic /etc/hashi/traefik/dynamic/40-hashi-security.yml", "dynamic /etc/hashi/traefik/dynamic/90-hashi-health.yml",
		"static /etc/hashi/traefik/traefik.yaml",
	}
	if !slices.Equal(order, want) {
		t.Errorf("order:\n%s", strings.Join(order, "\n"))
	}
	if key := files[1]; !key.Secret || key.Mode != 0o600 {
		t.Errorf("key file: secret=%v mode=%o", key.Secret, key.Mode)
	}
}

// Identical inputs give identical bytes (invariant 8 relies on it); a new certificate changes hashi-certs.yaml, which
// is what makes Traefik reload it.
func TestInvariant_8_RenderIsDeterministic(t *testing.T) {
	a, _ := Render(testOptions())
	b, _ := Render(testOptions())
	for i := range a {
		if a[i].Hash() != b[i].Hash() {
			t.Errorf("%s rendered differently twice", a[i].Path)
		}
	}
	o := testOptions()
	o.Certs[0].CertPEM = []byte("-----BEGIN CERTIFICATE-----\nCC==\n-----END CERTIFICATE-----\n")
	c, _ := Render(o)
	for i := range a {
		changed := a[i].Hash() != c[i].Hash()
		wantChanged := strings.HasSuffix(a[i].Path, ".crt") || strings.HasSuffix(a[i].Path, "hashi-certs.yaml")
		if changed != wantChanged {
			t.Errorf("%s changed=%v, want %v", a[i].Path, changed, wantChanged)
		}
	}
}

func TestRenderRejectsBadOptions(t *testing.T) {
	for name, mut := range map[string]func(*Options){
		"relative config dir": func(o *Options) { o.Paths.ConfigDir = "etc/hashi" },
		"shell in binary":     func(o *Options) { o.Paths.Binary = "/usr/bin/traefik;reboot" },
		"same ports":          func(o *Options) { o.ForwardAuthPort = o.APIPort },
		"URL with path":       func(o *Options) { o.HashiURL = "http://192.0.2.5:3340/x" },
		"URL with user":       func(o *Options) { o.HashiURL = "http://u:p@192.0.2.5" },
		"cert name traversal": func(o *Options) { o.Certs[0].Name = "../x" },
		"cert without key":    func(o *Options) { o.Certs[0].KeyPEM = secrets.Value{} },
	} {
		o := testOptions()
		mut(&o)
		if _, err := Render(o); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestServiceDefinitions(t *testing.T) {
	paths := Paths{Binary: DefaultBinary, ConfigDir: DefaultConfigDir}
	for _, init := range []string{InitOpenRC, InitSystemd} {
		s, err := ServiceFor(init, paths)
		if err != nil {
			t.Fatal(err)
		}
		golden(t, "service."+init, s.Data)
	}
	if _, err := ServiceFor("runit", paths); !errors.Is(err, ErrUnsupported) {
		t.Errorf("runit: %v", err)
	}
}
