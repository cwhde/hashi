package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSign(t *testing.T) {
	dir := t.TempDir()
	for _, a := range []string{"amd64", "arm64"} {
		if err := os.WriteFile(filepath.Join(dir, "hashi-pulse-linux-"+a), []byte("binary "+a), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	pub, priv, _ := ed25519.GenerateKey(nil)
	t.Setenv("PULSE_SIGNING_KEY", base64.StdEncoding.EncodeToString(priv.Seed()))
	if err := run([]string{"sign", dir, "v1.2.3"}); err != nil {
		t.Fatal(err)
	}
	for _, a := range []string{"amd64", "arm64"} {
		raw, _ := os.ReadFile(filepath.Join(dir, "hashi-pulse-linux-"+a+".sig"))
		sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
		sum := sha256.Sum256([]byte("binary " + a))
		if err != nil || !ed25519.Verify(pub, Message("v1.2.3", a, sum[:]), sig) {
			t.Fatalf("%s: signature does not verify", a)
		}
	}
	t.Setenv("PULSE_SIGNING_KEY", "")
	if err := run([]string{"sign", dir, "v1"}); err == nil {
		t.Fatal("signing without a key must fail")
	}
}
