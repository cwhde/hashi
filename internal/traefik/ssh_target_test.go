package traefik

import (
	"context"
	"errors"
	"testing"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/ipsets"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
)

// Bug 198: Hashi's SSH connection to a Traefik host follows a Pulse agent directly, through the agent's hidden set;
// going back to the literal address lets the set go.
func TestSSHTargetFollowsAgent(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	if err := st.Q.InsertTraefikConnection(ctx, db.InsertTraefikConnectionParams{ID: "c1", Name: "edge", SshHost: "192.0.2.10",
		SshPort: 22, SshUser: "root", Auth: "hashi_key", HostKeyType: "ssh-ed25519", HostKey: []byte{1}, HostKeyFingerprint: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Q.InsertPulseAgent(ctx, db.InsertPulseAgentParams{ID: "A1", Name: "edge", TokenHash: []byte("x"), InstallType: "linux", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	m := NewManager(st, nil, audit.New([]byte("k")), nil, nil)
	if err := m.SetSSHTarget(ctx, "c1", "", "nope", true); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown agent: %v", err)
	}
	if err := m.SetSSHTarget(ctx, "c1", "", "A1", true); err != nil {
		t.Fatal(err)
	}
	row, err := st.R.GetTraefikConnection(ctx, "c1")
	if err != nil || row.SshTargetSetID.String != ipsets.AgentSetID("A1") {
		t.Fatalf("ssh target %q %v", row.SshTargetSetID.String, err)
	}
	if err := m.SetSSHTarget(ctx, "c1", "", "", true); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Q.GetIPSet(ctx, ipsets.AgentSetID("A1")); err == nil {
		t.Fatal("the unused hidden set stayed")
	}
}
