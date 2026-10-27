package auth

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"git.juzo.io/juzo/hashi/internal/store/storetest"
)

func TestBootstrapCredentials(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	c, err := NewBootstrapCredentials(ctx, st.Q, t0)
	if err != nil {
		t.Fatal(err)
	}
	pw := c.Password.Reveal()
	if len(c.Username) != 8 || len(pw) != 24 || strings.ContainsAny(pw, "0OIl+/") {
		t.Fatalf("shape: %d %d", len(c.Username), len(pw))
	}
	row, err := st.Q.GetBootstrapCredentials(ctx)
	if err != nil || strings.Contains(row.PasswordHash, pw) || !strings.HasPrefix(row.PasswordHash, "$argon2id$v=19$") {
		t.Fatalf("stored %q %v", row.PasswordHash, err)
	}
	if err := CheckBootstrap(ctx, st.Q, c.Username, pw); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][2]string{{c.Username, pw + "x"}, {"someone", pw}, {"", ""}} {
		if err := CheckBootstrap(ctx, st.Q, bad[0], bad[1]); !errors.Is(err, ErrBadBootstrapLogin) {
			t.Fatalf("%v: %v", bad, err)
		}
	}
	// Reset replaces the old credentials.
	c2, err := NewBootstrapCredentials(ctx, st.Q, t0)
	if err != nil {
		t.Fatal(err)
	}
	if CheckBootstrap(ctx, st.Q, c.Username, pw) == nil {
		t.Fatal("old credentials still valid after reset")
	}
	var out bytes.Buffer
	PrintBootstrap(&out, c2)
	if !strings.Contains(out.String(), c2.Password.Reveal()) || !strings.Contains(out.String(), c2.Username) {
		t.Fatal("block does not show the credentials")
	}
	if err := st.Q.DeleteBootstrapCredentials(ctx); err != nil {
		t.Fatal(err)
	}
	if err := CheckBootstrap(ctx, st.Q, c2.Username, c2.Password.Reveal()); !errors.Is(err, ErrBadBootstrapLogin) {
		t.Fatalf("destroyed credentials: %v", err)
	}
}
