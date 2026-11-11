package store

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"
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

func TestNightlyBackupKeepsFourteen(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	if err := s.Migrate(ctx, quiet()); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(filepath.Dir(s.path), "backups")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// A pre-migration copy and an unrelated file are never pruned.
	for _, n := range []string{"pre-migrate-v3-20261001T000000Z.db", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	day := time.Date(2026, 11, 1, 3, 0, 0, 0, time.UTC)
	for i := range 16 {
		if _, err := s.NightlyBackup(ctx, day.AddDate(0, 0, i), 14); err != nil {
			t.Fatal(err)
		}
	}
	// The same day again replaces that day's copy instead of failing (VACUUM INTO refuses an existing file).
	path, err := s.NightlyBackup(ctx, day.AddDate(0, 0, 15).Add(time.Hour), 14)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "hashi-2026-11-16.db" {
		t.Fatalf("path %s", path)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	want := []string{"hashi-2026-11-03.db", "hashi-2026-11-16.db", "notes.txt", "pre-migrate-v3-20261001T000000Z.db"}
	if len(names) != 16 || names[0] != want[0] || names[13] != want[1] || names[14] != want[2] || names[15] != want[3] {
		t.Fatalf("backups dir: %v", names)
	}
	c, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if err := c.Ready(ctx); err != nil {
		t.Fatalf("nightly copy is not a migrated database: %v", err)
	}
}

func TestLockRefusesSecondHolder(t *testing.T) {
	dir := t.TempDir()
	unlock, err := Lock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Lock(dir); !errors.Is(err, ErrInUse) {
		t.Fatalf("second lock: %v", err)
	}
	unlock()
	again, err := Lock(dir)
	if err != nil {
		t.Fatalf("after unlock: %v", err)
	}
	again()
}
