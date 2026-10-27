// Package storetest opens a migrated throwaway database for tests.
package storetest

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"git.juzo.io/juzo/hashi/internal/store"
)

// New returns a migrated store in t.TempDir(), closed when the test ends.
func New(t testing.TB) *store.Store {
	t.Helper()
	return Open(t, filepath.Join(t.TempDir(), "hashi.db"))
}

// Open opens and migrates the database at path (reopen the same path to simulate a restart).
func Open(t testing.TB, path string) *store.Store {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(ctx, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}
	return s
}
