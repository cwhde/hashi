package firewall

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/security"
	"git.juzo.io/juzo/hashi/internal/sshx"
)

// The listings captured on a real nft 1.1.3 (testdata/contracts/nft/dynamic_sets.txt) parse as the host link needs.
func TestParseSetElemsFromContract(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/contracts/nft/dynamic_sets.txt")
	if err != nil {
		t.Fatal(err)
	}
	var docs []string
	for l := range strings.SplitSeq(string(raw), "\n") {
		if strings.HasPrefix(l, `{"nftables"`) {
			docs = append(docs, l)
		}
	}
	// staging4 (2 entries, expires only), staging6 (1), staging6 empty, dyn4 (timeouts), dyn6 (/64 prefixes),
	// dyn4 with a permanent entry …
	got := func(i int) []listedElem {
		t.Helper()
		e, err := parseSetElems([]byte(docs[i]))
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	if s := got(0); len(s) != 2 || s[0].prefix.String() != "198.51.100.7/32" || s[0].expires != 299 {
		t.Fatalf("staging4: %+v", s)
	}
	if s := got(2); len(s) != 0 {
		t.Fatalf("empty set: %+v", s)
	}
	if s := got(4); len(s) != 2 || s[1].prefix.String() != "2001:db8:5::/64" || s[1].expires != 899 {
		t.Fatalf("dyn6: %+v", s)
	}
	if s := got(5); len(s) != 3 || s[2].prefix.String() != "198.51.100.20/32" || s[2].expires != -1 {
		t.Fatalf("permanent: %+v", s)
	}
	// Two documents in one output, as one command lists both families.
	if s, err := parseSetElems([]byte(docs[0] + "\n" + docs[1])); err != nil || len(s) != 3 {
		t.Fatalf("two documents: %+v %v", s, err)
	}
}

// scripted answers the host link's commands like nft would, and records each pushed batch.
type scripted struct {
	dyn, staging string
	pushes       []string
}

func (s *scripted) Run(_ context.Context, cmd string, stdin io.Reader) (sshx.Result, error) {
	switch {
	case strings.Contains(cmd, "list set inet hashi dyn4"):
		return sshx.Result{Stdout: []byte(s.dyn)}, nil
	case strings.Contains(cmd, "list set inet hashi staging4"):
		return sshx.Result{Stdout: []byte(s.staging)}, nil
	case strings.HasSuffix(cmd, "-j -f -"):
		b, _ := io.ReadAll(stdin)
		s.pushes = append(s.pushes, string(b))
		return sshx.Result{}, nil
	}
	return sshx.Result{ExitCode: 1, Stderr: []byte("unexpected " + cmd)}, nil
}
func (s *scripted) ReadFile(string) ([]byte, error)                           { return nil, os.ErrNotExist }
func (s *scripted) WriteFileAtomic(string, []byte, os.FileMode) (bool, error) { return false, nil }
func (s *scripted) MkdirAll(string, os.FileMode) error                        { return nil }

func listing(name string, elems ...string) string {
	return `{"nftables":[{"metainfo":{}},{"set":{"family":"inet","name":"` + name + `","table":"hashi","elem":[` + strings.Join(elems, ",") + `]}}]}`
}

// 05 §6 steps 1–5 on one tick: a staged address becomes a ban with T_ban and is pushed with its timeout; staging is
// drained; an entry Hashi does not know is removed; an allowed address is drained but never banned; a quiet tick pushes
// nothing.
func TestHostLinkTick(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	h, err := e.m.Save(ctx, "admin", "", "", managedInput(t))
	if err != nil {
		t.Fatal(err)
	}
	row, _ := e.st.R.GetFirewallHost(ctx, h.ID)
	clock := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	e.m.now = func() time.Time { return clock }
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	eng := security.NewEngine(e.st, log, e.m.now)
	allowed := netip.MustParseAddr("198.51.100.50")
	eng.UseAllowed(func(a netip.Addr) bool { return a == allowed })

	sh := &scripted{
		dyn: listing("dyn4", `{"elem":{"val":"203.0.113.9","timeout":600,"expires":599}}`) + "\n" + listing("dyn6"),
		staging: listing("staging4", `{"elem":{"val":"198.51.100.7","expires":299}}`, `{"elem":{"val":"198.51.100.50","expires":299}}`) +
			"\n" + listing("staging6"),
	}
	s := &linkSession{l: NewLink(e.m, eng, log), row: row, h: sh, nft: "nft"}
	if err := s.tick(ctx); err != nil {
		t.Fatal(err)
	}
	if !eng.Banned(netip.MustParseAddr("198.51.100.7")) || eng.Banned(allowed) {
		t.Fatalf("bans %+v", eng.Bans())
	}
	if len(sh.pushes) != 1 {
		t.Fatalf("pushes %v", sh.pushes)
	}
	var batch struct {
		Nftables []map[string]struct {
			Element struct {
				Name string            `json:"name"`
				Elem []json.RawMessage `json:"elem"`
			} `json:"element"`
		} `json:"nftables"`
	}
	if err := json.Unmarshal([]byte(sh.pushes[0]), &batch); err != nil {
		t.Fatal(err)
	}
	var ops []string
	for _, o := range batch.Nftables {
		for verb, x := range o {
			ops = append(ops, verb+" "+x.Element.Name+" "+string(x.Element.Elem[0]))
		}
	}
	want := []string{
		`add dyn4 {"elem":{"timeout":900,"val":"198.51.100.7"}}`,
		`destroy dyn4 "203.0.113.9"`,
		`destroy staging4 "198.51.100.7"`,
		`destroy staging4 "198.51.100.50"`,
	}
	if strings.Join(ops, "\n") != strings.Join(want, "\n") {
		t.Fatalf("batch:\n%s", strings.Join(ops, "\n"))
	}
	st, err := e.st.R.ListDynamicEntryHosts(ctx, row.ID)
	if err != nil || len(st) != 1 || st[0].State != "applied" {
		t.Fatalf("entry hosts %+v %v", st, err)
	}
	// Invariant 7: every change the link makes to dyn outside a plan is a security event, per host.
	hostEvents := func() map[string]string {
		t.Helper()
		eng.DrainEvents(ctx)
		out := map[string]string{}
		rows, err := e.st.R.ListRecentSecurityEvents(ctx, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rows {
			if r.Host == row.Name {
				out[r.Address+" "+r.EventType] = r.Host
			}
		}
		return out
	}
	ev := hostEvents()
	if _, ok := ev["198.51.100.7 ban_applied"]; !ok {
		t.Errorf("no ban_applied event for the pushed ban: %v", ev)
	}
	if _, ok := ev["203.0.113.9 ban_removed_host"]; !ok {
		t.Errorf("no ban_removed_host event for the entry taken off the host: %v", ev)
	}

	// Next tick: the host now holds the ban (as Hashi believes); staging is empty; nothing to push.
	sh.staging = listing("staging4") + "\n" + listing("staging6")
	clock = clock.Add(3 * time.Second)
	if err := s.tick(ctx); err != nil {
		t.Fatal(err)
	}
	if len(sh.pushes) != 1 {
		t.Fatalf("quiet tick pushed: %v", sh.pushes[1:])
	}
	// The ban is lifted in Hashi: the next tick removes it from the host.
	if _, err := eng.Remove(ctx, netip.MustParsePrefix("198.51.100.7/32"), "admin", "test"); err != nil {
		t.Fatal(err)
	}
	if err := s.tick(ctx); err != nil {
		t.Fatal(err)
	}
	if len(sh.pushes) != 2 || !strings.Contains(sh.pushes[1], `"destroy"`) || !strings.Contains(sh.pushes[1], "198.51.100.7") {
		t.Fatalf("unban push: %v", sh.pushes)
	}
	if _, ok := hostEvents()["198.51.100.7 ban_removed_host"]; !ok {
		t.Error("no ban_removed_host event after the unban reached the host")
	}
}
