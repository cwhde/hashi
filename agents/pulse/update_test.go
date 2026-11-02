package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReleaseKey(t *testing.T) {
	if k, err := base64.StdEncoding.DecodeString(releaseKey); err != nil || len(k) != ed25519.PublicKeySize {
		t.Fatal("releaseKey is not an Ed25519 public key")
	}
}

func TestNewer(t *testing.T) {
	for want, ok := range map[[2]string]bool{{"v0.9.3", "v0.9.2"}: true, {"v0.9.2", "v0.9.2"}: false, {"v0.9.1", "v0.9.2"}: false, {"v0.9.3", "dev"}: false, {"v0.9.3", "v0.9.2-3-gabc1234"}: false} {
		if newer(want[0], want[1]) != ok {
			t.Errorf("%v", want)
		}
	}
}

// An update installs only a signed file for this version and architecture that runs and names that version.
func TestUpdate(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	old := trustedKey
	trustedKey = base64.StdEncoding.EncodeToString(pub)
	t.Cleanup(func() { trustedKey = old })
	name := "hashi-pulse-linux-" + runtime.GOARCH
	serve := func(t *testing.T, body string, sigText []byte, chunked bool) string {
		t.Helper()
		sig := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, sigText))
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/rel/" + name + ".sig":
				_, _ = w.Write([]byte(sig + "\n"))
			case "/rel/" + name:
				if chunked {
					w.(http.Flusher).Flush() // no length: Go then answers in chunks
				}
				_, _ = w.Write([]byte(body))
			default:
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(srv.Close)
		return srv.URL + "/rel"
	}
	script := func(v string) string { return "#!/bin/sh\necho " + v + "\n" }
	sum := func(b string) []byte { s := sha256.Sum256([]byte(b)); return s[:] }
	self := func(t *testing.T) string {
		t.Helper()
		p := filepath.Join(t.TempDir(), "hashi-pulse")
		if err := os.WriteFile(p, []byte("old"), 0o755); err != nil { //nolint:gosec // a test file
			t.Fatal(err)
		}
		return p
	}
	t.Run("good", func(t *testing.T) {
		p, body := self(t), script("v9.9.9")
		if err := update(t.Context(), serve(t, body, signedText("v9.9.9", runtime.GOARCH, sum(body)), false), "v9.9.9", p); err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(p); string(got) != body {
			t.Fatal("the file was not replaced")
		}
		if st, _ := os.Stat(p); st.Mode().Perm() != 0o755 {
			t.Fatalf("mode %v", st.Mode())
		}
		if _, err := os.Stat(filepath.Join(filepath.Dir(p), ".hashi-pulse.new")); err == nil {
			t.Fatal("the temporary file was left behind")
		}
	})
	for _, c := range []struct {
		name, body string
		signed     []byte
		chunked    bool
	}{
		{"bad signature", script("v9.9.9"), []byte("something else"), false},
		{"signed for another version", script("v9.9.9"), signedText("v9.9.8", runtime.GOARCH, sum(script("v9.9.9"))), false},
		{"signed for another architecture", script("v9.9.9"), signedText("v9.9.9", "riscv64", sum(script("v9.9.9"))), false},
		{"names another version", script("v1.0.0"), signedText("v9.9.9", runtime.GOARCH, sum(script("v1.0.0"))), false},
		{"does not run", "not a program", signedText("v9.9.9", runtime.GOARCH, sum("not a program")), false},
		{"no length", script("v9.9.9"), signedText("v9.9.9", runtime.GOARCH, sum(script("v9.9.9"))), true},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := self(t)
			if err := update(t.Context(), serve(t, c.body, c.signed, c.chunked), "v9.9.9", p); err == nil {
				t.Fatal("must be refused")
			}
			if got, _ := os.ReadFile(p); string(got) != "old" {
				t.Fatal("the agent's file must stay")
			}
			if entries, _ := os.ReadDir(filepath.Dir(p)); len(entries) != 1 {
				t.Fatal("the temporary file was left behind")
			}
		})
	}
}

func TestParseReplyUpdate(t *testing.T) {
	r, err := parseReply(`{"check_public_ip":false,"interval_seconds":60,"version":"v0.9.3","update_from":"https://git.example.com/r/download/v0.9.3"}`)
	if err != nil || r.Version != "v0.9.3" || r.UpdateFrom != "https://git.example.com/r/download/v0.9.3" {
		t.Fatalf("%+v %v", r, err)
	}
	if !strings.Contains(state{Tried: "v0.9.3"}.encode(), "v0.9.3") || decodeState(state{Tried: "v0.9.3"}.encode()).Tried != "v0.9.3" {
		t.Fatal("the tried version must survive the sleeper")
	}
}
