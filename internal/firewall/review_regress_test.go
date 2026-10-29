package firewall

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"git.juzo.io/juzo/hashi/internal/ipsets"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

func contains(ps []netip.Prefix, a netip.Addr) bool {
	for _, p := range ps {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// One allow set holding a /16 and a /32 inside it; a block set with the /24 between them.
// 05 §4.2: for .7, a = /32 (longest in S_allow), b = /24 → allowed.
// Review of PR B (reproduced): merging inside one set lost the /32, so the /24 block won.
func TestFoldUsesUnmergedLeavesPerSet(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	ed := ipsets.Editor{}
	allow, _ := ipsets.CreateSet(ctx, e.st.Q, ipsets.SetInput{Slug: "trusted", Name: "trusted"}, now)
	block, _ := ipsets.CreateSet(ctx, e.st.Q, ipsets.SetInput{Slug: "bad", Name: "bad"}, now)
	for _, x := range []struct{ set, item string }{{allow.ID, "198.51.0.0/16"}, {allow.ID, "198.51.100.7"}, {block.ID, "198.51.100.0/24"}} {
		it, err := ipsets.ParseIPOrCIDR(x.item)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ed.AddElement(ctx, e.st.Q, x.set, ipsets.ElementInput{Item: it}, "t", now); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.m.PutBindings(ctx, "admin", "", Bindings{Allow: []string{allow.ID}, Block: []string{block.ID}}); err != nil {
		t.Fatal(err)
	}
	f, _, _, err := e.m.Folded(ctx, e.st.R)
	if err != nil {
		t.Fatal(err)
	}
	x := netip.MustParseAddr("198.51.100.7")
	v, _, _ := Verdict([]netip.Prefix{netip.MustParsePrefix("198.51.0.0/16"), netip.MustParsePrefix("198.51.100.7/32")},
		[]netip.Prefix{netip.MustParsePrefix("198.51.100.0/24")}, x)
	if v != Allowed || !contains(f.Allow4, x) || contains(f.Block4, x) {
		t.Errorf("198.51.100.7 should be allowed (/32 > /24), got blocked")
	}
}

// A newly blocked /16 that contains one allowed address (e.g. a hashi-hosts public IP) is carved by folding;
// 05 §4.4 says a newly blocked range of /16 or larger is high.
// Review of PR B (reproduced): folding carved the /16 into smaller pieces and the risk rule saw none of /16.
func TestNewlyBlockedSlash16IsHighWhenCarved(t *testing.T) {
	base := Spec{WAN: "eth0", Subnets: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/24")}, TraefikIP: netip.MustParseAddr("10.0.0.5"),
		NetBird: NetBird{Mode: NetBirdDisabled}}
	old := base
	old.Folded = Fold([]netip.Prefix{netip.MustParsePrefix("203.0.113.9/32")}, nil)
	nw := base
	nw.Folded = Fold([]netip.Prefix{netip.MustParsePrefix("203.0.113.9/32")}, []netip.Prefix{netip.MustParsePrefix("203.0.0.0/16")})
	ob, err := Render(old)
	if err != nil {
		t.Fatal(err)
	}
	nb, _ := Render(nw)
	why := TableRisk(ob, nb, Guard{Blocks: []netip.Prefix{netip.MustParsePrefix("203.0.0.0/16")}})
	if len(why) == 0 {
		t.Errorf("a newly blocked /16 is low risk when one allowed address sits inside it")
	}
}

// Review of PR B (reproduced): the only firewall host could be deleted after setup.
func TestInvariant_11_LastFirewallHostStays(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	h, err := e.m.Save(ctx, "admin", "", "", managedInput(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.st.Q.UpsertSetupStep(ctx, db.UpsertSetupStepParams{Step: 11, Status: "done", InputsJson: "{}", UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := e.m.Delete(ctx, "admin", "", h.ID); !errors.Is(err, ErrInUse) {
		t.Fatalf("deleting the last host after setup: %v", err)
	}
	second := managedInput(t)
	second.Name, second.Host = "edge-c", "198.18.3.9"
	if _, err := e.m.Save(ctx, "admin", "", "", second); err != nil {
		t.Fatal(err)
	}
	if err := e.m.Delete(ctx, "admin", "", h.ID); err != nil {
		t.Fatalf("deleting one of two hosts: %v", err)
	}
}
