package sshx_test

import (
	"encoding/pem"
	"os"
	"syscall"
	"testing"

	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/sshx"
)

// testPassword is the in-process test server's login, not a real credential.
const testPassword = "test-only-pw" // hashi:allow-secret-pattern

// pwAuth logs in as root with a test password.
func pwAuth(pw string) sshx.Auth {
	a := sshx.Auth{User: "root"}
	a.Password = secrets.New(pw) // hashi:allow-secret-pattern
	return a
}

func pemEncode(b *pem.Block) []byte { return pem.EncodeToMemory(b) }

func inode(t *testing.T, p string) uint64 {
	t.Helper()
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return st.Sys().(*syscall.Stat_t).Ino
}
