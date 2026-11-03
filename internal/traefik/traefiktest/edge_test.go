package traefiktest_test

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/certs"
	"git.juzo.io/juzo/hashi/internal/certs/certstest"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
	"git.juzo.io/juzo/hashi/internal/traefik"
	"git.juzo.io/juzo/hashi/internal/traefik/traefiktest"
	"git.juzo.io/juzo/hashi/internal/vault"
)

type memSolver map[string][]string

func (m memSolver) Present(_ context.Context, fqdn, v string) error {
	m[fqdn] = append(m[fqdn], v)
	return nil
}
func (m memSolver) CleanUp(_ context.Context, fqdn, _ string) error { delete(m, fqdn); return nil }

// The dev fakes behave like the real pieces the managers expect: the edge validates, installs, verifies and serves
// the certificate the local CA issued.
func TestDevEdgeAndCAThroughTheManagers(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	v, _ := vault.New(bytes.Repeat([]byte{7}, vault.KeySize), 1)
	if err := settings.Put(ctx, st.Q, settings.General{RootDomain: "example.com", AdminDomain: "hashi.example.com",
		InternalURL: "http://192.0.2.5:3340", SyncIntervalSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	probe, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "contracts", "os", "os_debian13.txt"))
	if err != nil {
		t.Fatal(err)
	}
	edge, err := traefiktest.NewEdge(string(probe))
	if err != nil {
		t.Fatal(err)
	}
	a := audit.New([]byte("k"))
	tm := traefik.NewManager(st, v, a, nil, nil)
	tm.UseFakes(edge.Connect, edge.Binary, edge.Scan, edge.DialTLS)
	eng := hsync.New(st, a, tm.Subsystems()...)

	hk, _ := tm.ScanHostKey(ctx, "192.0.2.10", 22)
	row, err := tm.Save(ctx, "admin", "", "", traefik.ConnectionInput{Name: "edge", Host: "192.0.2.10", Port: 22, User: "root",
		Auth: traefik.AuthPassword, Credential: secrets.New("pw"), HostKey: hk.Key})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := tm.Validate(ctx, row.ID); err != nil {
		t.Fatal(err)
	}
	if err := tm.ConfirmOwnership(ctx, "admin", "", row.ID); err != nil {
		t.Fatal(err)
	}

	// A certificate from the local CA, stored the way the certificates manager stores it.
	ca, _ := certstest.NewCA()
	solver := memSolver{}
	iss, err := ca.Clients(nil, certs.Account{}).Issue(ctx, solver, []string{"example.com", "*.example.com"}, certs.IssueOptions{})
	if err != nil || len(solver) != 0 {
		t.Fatalf("issue: %v, values left %v", err, solver)
	}
	if err := storeIssued(ctx, st, v, iss); err != nil {
		t.Fatal(err)
	}

	p, err := eng.Propose(ctx, traefik.Subsystem, row.ID, hsync.TriggerManual, "admin")
	if err != nil {
		t.Fatal(err)
	}
	_ = eng.Approve(ctx, p.ID, "admin", "")
	done, err := eng.Apply(ctx, p.ID, "admin", "")
	if err != nil || done.Status != hsync.StatusApplied {
		t.Fatalf("apply: %v %+v", err, done)
	}
	dists, _ := st.R.ListCertificateDistributions(ctx)
	if len(dists) != 1 || dists[0].State != "verified" || dists[0].Fingerprint != iss.Fingerprint {
		t.Fatalf("distribution: %+v", dists)
	}
	if again, err := eng.Propose(ctx, traefik.Subsystem, row.ID, hsync.TriggerReconcile, "system"); err != nil || again != nil {
		t.Errorf("second plan: %+v %v", again, err)
	}
}

func storeIssued(ctx context.Context, st *store.Store, v *vault.Vault, iss certs.Issued) error {
	rec, err := v.Seal(vault.Service, certs.KeyPurpose, iss.KeyPEM)
	if err != nil {
		return err
	}
	if err := st.Q.InsertSecret(ctx, rec); err != nil {
		return err
	}
	if err := st.Q.InsertCertificate(ctx, db.InsertCertificateParams{ID: "c1", Name: "example.com", NamesJson: `["example.com","*.example.com"]`}); err != nil {
		return err
	}
	return st.Q.SetCertificateIssued(ctx, db.SetCertificateIssuedParams{NamesJson: `["example.com","*.example.com"]`,
		KeySecretID: sql.NullString{String: rec.ID, Valid: true}, CertPem: string(iss.CertPEM), Fingerprint: iss.Fingerprint,
		LastRenewalResult: "issued", ID: "c1"})
}
