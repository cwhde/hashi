// Command pulsesign signs Pulse release binaries (ADR 0003, 08 §6).
//
//	pulsesign keygen          prints a new private key (base64) to stdout and its public key to stderr
//	pulsesign sign DIR VERSION signs DIR/hashi-pulse-linux-<arch> with $PULSE_SIGNING_KEY into <file>.sig
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Message is what a signature vouches for; agents/pulse checks the same text.
func Message(version, arch string, sum []byte) []byte {
	return []byte("hashi-pulse " + version + " " + arch + " " + hex.EncodeToString(sum))
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "pulsesign:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	switch {
	case len(args) == 1 && args[0] == "keygen":
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return err
		}
		fmt.Println(base64.StdEncoding.EncodeToString(priv.Seed()))
		fmt.Fprintln(os.Stderr, "public key:", base64.StdEncoding.EncodeToString(pub))
		return nil
	case len(args) == 3 && args[0] == "sign":
		seed, err := base64.StdEncoding.DecodeString(os.Getenv("PULSE_SIGNING_KEY"))
		if err != nil || len(seed) != ed25519.SeedSize {
			return errors.New("PULSE_SIGNING_KEY must hold the base64 private key")
		}
		key := ed25519.NewKeyFromSeed(seed)
		for _, arch := range []string{"amd64", "arm64"} {
			file := filepath.Join(args[1], "hashi-pulse-linux-"+arch)
			b, err := os.ReadFile(file) //nolint:gosec // the release's own build output
			if err != nil {
				return err
			}
			sum := sha256.Sum256(b)
			sig := ed25519.Sign(key, Message(args[2], arch, sum[:]))
			if err := os.WriteFile(file+".sig", []byte(base64.StdEncoding.EncodeToString(sig)+"\n"), 0o644); err != nil { //nolint:gosec // a public signature
				return err
			}
		}
		return nil
	}
	return errors.New("usage: pulsesign keygen | pulsesign sign DIR VERSION")
}
