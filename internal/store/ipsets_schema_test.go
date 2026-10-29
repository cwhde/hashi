package store

import (
	"context"
	"testing"
)

// Migration 00007: an element row holds exactly the columns of its type, a set cannot include itself, system sets
// need a key and user sets must not have one, and the migration goes down cleanly.
func TestMigration7IPSetConstraints(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	if err := s.Migrate(ctx, quiet()); err != nil {
		t.Fatal(err)
	}
	exec := func(q string, args ...any) error {
		_, err := s.Writer.ExecContext(ctx, q, args...)
		return err
	}
	if err := exec(`INSERT INTO ip_sets (id, slug, name, kind, origin, created_at, updated_at) VALUES ('u', 'u', 'u', 'user', 'user', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	el := `INSERT INTO ip_set_elements (id, ip_set_id, position, type, origin, address, prefix, family, fqdn, child_set_id,
		created_at, updated_at) VALUES (?, 'u', ?, ?, 'user', ?, ?, ?, ?, ?, 1, 1)`
	ok := []struct {
		typ                   string
		addr, pre, fqdn, kids any
		fam                   any
	}{
		{"ip", "192.0.2.1", nil, nil, nil, 4},
		{"cidr", nil, "192.0.2.0/24", nil, nil, 4},
		{"dns", nil, nil, "a.example.com", nil, nil},
		{"set", nil, nil, nil, "sys-all", nil},
	}
	for i, c := range ok {
		if err := exec(el, "ok"+c.typ, i+1, c.typ, c.addr, c.pre, c.fam, c.fqdn, c.kids); err != nil {
			t.Errorf("%s: %v", c.typ, err)
		}
	}
	bad := []struct {
		name                  string
		typ                   string
		addr, pre, fqdn, kids any
		fam                   any
	}{
		{"ip without family", "ip", "192.0.2.1", nil, nil, nil, nil},
		{"ip with a prefix", "ip", "192.0.2.1", "192.0.2.0/24", nil, nil, 4},
		{"cidr with a name", "cidr", nil, "192.0.2.0/24", "a.example.com", nil, 4},
		{"dns with a family", "dns", nil, nil, "a.example.com", nil, 4},
		{"set referencing itself", "set", nil, nil, nil, "u", nil},
		{"set referencing nothing", "set", nil, nil, nil, "missing", nil},
		{"unknown type", "range", "192.0.2.1", nil, nil, nil, 4},
	}
	for i, c := range bad {
		if err := exec(el, "bad"+c.name, 100+i, c.typ, c.addr, c.pre, c.fam, c.fqdn, c.kids); err == nil {
			t.Errorf("%s accepted", c.name)
		}
	}
	if err := exec(`INSERT INTO ip_sets (id, slug, name, kind, origin, created_at, updated_at) VALUES ('s', 's', 's', 'system', 'system', 1, 1)`); err == nil {
		t.Error("system set without key accepted")
	}
	if err := exec(`INSERT INTO ip_sets (id, slug, name, kind, origin, system_key, created_at, updated_at) VALUES ('k', 'k', 'k', 'user', 'user', 'all', 1, 1)`); err == nil {
		t.Error("user set with a system key accepted")
	}
	p, err := s.provider()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.DownTo(ctx, 6); err != nil {
		t.Fatal(err)
	}
	if err := exec(`SELECT 1 FROM ip_sets`); err == nil {
		t.Fatal("down migration left ip_sets")
	}
}
