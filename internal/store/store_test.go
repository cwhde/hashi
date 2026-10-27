package store

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "hashi.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestMigrateFreshAndReady(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	if err := s.Ready(ctx); err == nil {
		t.Fatal("fresh database must not be ready before migrating")
	}
	if err := s.Migrate(ctx, quiet()); err != nil {
		t.Fatal(err)
	}
	if err := s.Ready(ctx); err != nil {
		t.Fatalf("not ready after migrate: %v", err)
	}
	// Second run is a no-op and writes no backup.
	if err := s.Migrate(ctx, quiet()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(s.path), "backups")); !os.IsNotExist(err) {
		t.Fatalf("no backup expected when nothing is pending, stat err=%v", err)
	}
}

func TestPragmas(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	var mode string
	var fk int
	if err := s.Writer.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if err := s.Reader.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" || fk != 1 {
		t.Fatalf("journal_mode=%s foreign_keys=%d", mode, fk)
	}
	if _, err := s.Reader.ExecContext(ctx, "CREATE TABLE x (a INTEGER)"); err == nil {
		t.Fatal("reader pool must be read-only")
	}
}

func TestBackupTo(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	if err := s.Migrate(ctx, quiet()); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "b", "copy.db")
	if err := s.BackupTo(ctx, dst); err != nil {
		t.Fatal(err)
	}
	c, err := Open(ctx, dst)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if err := c.Ready(ctx); err != nil {
		t.Fatalf("backup is not a migrated database: %v", err)
	}
}

func TestFailedMigrationRestoresBackup(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	s.migrations = fstest.MapFS{
		"00001_a.sql": {Data: []byte("-- +goose Up\nCREATE TABLE a (v INTEGER);\n")},
	}
	if err := s.Migrate(ctx, quiet()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Writer.ExecContext(ctx, "INSERT INTO a VALUES (42)"); err != nil {
		t.Fatal(err)
	}
	s.migrations = fstest.MapFS{
		"00001_a.sql": {Data: []byte("-- +goose Up\nCREATE TABLE a (v INTEGER);\n")},
		"00002_b.sql": {Data: []byte("-- +goose Up\nDELETE FROM a;\nCREATE TABLE broken (;\n")},
	}
	if err := s.Migrate(ctx, quiet()); err == nil {
		t.Fatal("broken migration must fail")
	}
	var v int
	if err := s.Reader.QueryRowContext(ctx, "SELECT v FROM a").Scan(&v); err != nil || v != 42 {
		t.Fatalf("data not restored: v=%d err=%v", v, err)
	}
	backups, _ := filepath.Glob(filepath.Join(filepath.Dir(s.path), "backups", "pre-migrate-v1-*.db"))
	if len(backups) != 1 {
		t.Fatalf("want one pre-migration backup, got %v", backups)
	}
}
