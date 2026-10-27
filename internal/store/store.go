// Package store owns the SQLite database: connections, migrations and generated queries.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite" // registers the "sqlite" driver

	"git.juzo.io/juzo/hashi/internal/store/db"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store holds one writer connection (SQLite allows one writer at a time) and a read pool.
type Store struct {
	path       string
	migrations fs.FS
	Writer     *sql.DB
	Reader     *sql.DB
	// Q runs generated queries on the writer. Inside a Tx use the transaction's queries instead: the writer has one
	// connection, so a Q call inside a Tx waits forever.
	Q *db.Queries
	// R runs read-only queries on the read pool; use it for reads that are not part of a write transaction.
	R *db.Queries
}

func dsn(path string, readOnly bool) string {
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "synchronous(NORMAL)")
	if readOnly {
		q.Add("_pragma", "query_only(1)")
	} else {
		q.Set("_txlock", "immediate") // take the write lock at BEGIN, so busy_timeout applies instead of SQLITE_BUSY mid-tx
	}
	return "file:" + path + "?" + q.Encode()
}

// Open opens (creating if needed) the database at path. It does not migrate.
func Open(ctx context.Context, path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	w, err := sql.Open("sqlite", dsn(path, false))
	if err != nil {
		return nil, fmt.Errorf("open writer: %w", err)
	}
	w.SetMaxOpenConns(1)
	r, err := sql.Open("sqlite", dsn(path, true))
	if err != nil {
		_ = w.Close()
		return nil, fmt.Errorf("open reader: %w", err)
	}
	r.SetMaxOpenConns(8)
	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return nil, err
	}
	s := &Store{path: path, migrations: sub, Writer: w, Reader: r, Q: db.New(w), R: db.New(r)}
	if err := w.PingContext(ctx); err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("ping %s: %w", path, err)
	}
	return s, nil
}

// Tx runs fn in one write transaction; fn's error rolls it back.
func (s *Store) Tx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.Writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	if err := fn(s.Q.WithTx(tx)); err != nil {
		return errors.Join(err, tx.Rollback())
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// Path is the database file.
func (s *Store) Path() string { return s.path }

// Close closes both pools.
func (s *Store) Close() error {
	return errors.Join(s.Reader.Close(), s.Writer.Close())
}

func (s *Store) provider() (*goose.Provider, error) {
	return goose.NewProvider(goose.DialectSQLite3, s.Writer, s.migrations, goose.WithDisableGlobalRegistry(true))
}

// Migrate brings the schema to head. When an existing database has pending migrations it is first
// copied with VACUUM INTO to <data>/backups/; if a migration fails, that copy is put back and the
// error returned so the process exits non-zero (11-delivery.md §8).
func (s *Store) Migrate(ctx context.Context, log *slog.Logger) error {
	p, err := s.provider()
	if err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	current, target, err := p.GetVersions(ctx)
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if current == target {
		return nil
	}
	var backup string
	if current > 0 {
		backup = filepath.Join(filepath.Dir(s.path), "backups",
			fmt.Sprintf("pre-migrate-v%d-%s.db", current, time.Now().UTC().Format("20060102T150405Z")))
		if err := s.BackupTo(ctx, backup); err != nil {
			return fmt.Errorf("pre-migration backup: %w", err)
		}
		log.Info("pre-migration backup written", "backup", backup, "from_version", current)
	}
	if _, err := p.Up(ctx); err != nil {
		if backup == "" {
			return fmt.Errorf("migrate fresh database: %w", err)
		}
		if rerr := s.restore(context.WithoutCancel(ctx), backup); rerr != nil {
			return fmt.Errorf("migrate: %w; restoring %s also failed: %w", err, backup, rerr)
		}
		return fmt.Errorf("migrate (database restored from %s): %w", backup, err)
	}
	log.Info("schema migrated", "from_version", current, "to_version", target)
	return nil
}

// BackupTo writes a consistent online copy of the database to dst.
func (s *Store) BackupTo(ctx context.Context, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	_, err := s.Writer.ExecContext(ctx, "VACUUM INTO ?", dst)
	return err
}

// restore replaces the live database with backup. Pools are reopened afterwards.
func (s *Store) restore(ctx context.Context, backup string) error {
	if err := s.Close(); err != nil {
		return err
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(s.path + suffix); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	data, err := os.ReadFile(backup) //nolint:gosec // backup path is built by Migrate from the configured data dir
	if err != nil {
		return err
	}
	tmp := s.path + ".restore"
	if err := os.WriteFile(tmp, data, 0o600); err != nil { //nolint:gosec // tmp is next to the configured DB path
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	fresh, err := Open(ctx, s.path)
	if err != nil {
		return err
	}
	fresh.migrations = s.migrations
	*s = *fresh
	return nil
}

// Ready reports nil when the database answers and the schema is at head.
func (s *Store) Ready(ctx context.Context) error {
	p, err := s.provider()
	if err != nil {
		return err
	}
	current, target, err := p.GetVersions(ctx)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	if current != target {
		return fmt.Errorf("schema at version %d, want %d", current, target)
	}
	return nil
}
