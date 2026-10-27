package app

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/auth"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/setup"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// Console commands run inside the container (`docker exec hashi-v3 hashi …`) against the same database. They never
// migrate: the server owns the schema.

func openForConsole(ctx context.Context, cfg Config) (*store.Store, error) {
	st, err := store.Open(ctx, cfg.DBPath())
	if err != nil {
		return nil, err
	}
	if err := st.Ready(ctx); err != nil {
		_ = st.Close()
		return nil, fmt.Errorf("database not ready (start the server once first): %w", err)
	}
	return st, nil
}

// BootstrapReset regenerates the bootstrap credentials and prints them (02 §1.1 point 6).
func BootstrapReset(ctx context.Context, cfg Config, out io.Writer) error {
	st, err := openForConsole(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	master, err := vault.LoadMasterKey(cfg.MasterKeyFile)
	if err != nil {
		return err
	}
	v, err := vault.New(master, 1)
	if err != nil {
		return err
	}
	if err := v.CheckKey(ctx, st.Q); err != nil {
		return err
	}
	al := audit.New(v.Operational("audit-correlation"))
	if done, err := setup.New(st, v, al).Completed(ctx); err != nil {
		return err
	} else if done {
		return errors.New("setup is complete; bootstrap login no longer exists (use `hashi admin recover`)")
	}
	var c auth.BootstrapCredentials
	err = st.Tx(ctx, func(q *db.Queries) error {
		if c, err = auth.NewBootstrapCredentials(ctx, q, time.Now()); err != nil {
			return err
		}
		if _, err := q.RevokeSessionsByMethod(ctx, db.RevokeSessionsByMethodParams{
			RevokedAt:        sql.NullInt64{Int64: time.Now().UnixMilli(), Valid: true},
			RevocationReason: sql.NullString{String: "bootstrap_reset", Valid: true}, AuthMethod: string(auth.MethodBootstrap),
		}); err != nil {
			return err
		}
		return al.Record(ctx, q, audit.Event{Actor: audit.Console, Action: "setup.bootstrap_credentials.reset"})
	})
	if err != nil {
		return err
	}
	auth.PrintBootstrap(out, c)
	return nil
}

// AdminRecover prints a one-time, 15-minute login link that lets the admin register a new passkey (02 §2).
func AdminRecover(ctx context.Context, cfg Config, out io.Writer) error {
	st, err := openForConsole(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	g, err := settings.Get[settings.General](ctx, st.R)
	if err != nil {
		return err
	}
	if !g.Configured() {
		return errors.New("base settings are missing; finish setup with the bootstrap login instead")
	}
	var token string
	err = st.Tx(ctx, func(q *db.Queries) error {
		if token, err = auth.NewRecoveryLink(ctx, q, time.Now()); err != nil {
			return err
		}
		return audit.New(nil).Record(ctx, q, audit.Event{Actor: audit.Console, Action: "auth.recover.link_created"})
	})
	if err != nil {
		return err
	}
	// The token sits in the URL fragment, so it never reaches server or proxy logs.
	_, _ = fmt.Fprintf(out, "One-time recovery link (valid %s, from a private network only):\n  https://%s/login/recover#%s\n",
		auth.RecoveryLinkTTL, g.AdminDomain, token)
	return nil
}

// VaultRecover reads a recovery key from in and prints the master key it unwraps (02 §5.3). Use it after restoring a
// backup on a host without the original master key; install the printed key as the hashi_master_key secret.
func VaultRecover(ctx context.Context, cfg Config, in io.Reader, out io.Writer) error {
	st, err := openForConsole(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	rec, err := st.Q.GetRecoveryKey(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("this database has no recovery key (setup step 9 was never finished)")
	}
	if err != nil {
		return err
	}
	_, _ = fmt.Fprint(out, "Recovery key: ")
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	key, err := vault.ParseRecoveryKey(line)
	if err != nil {
		return err
	}
	master, err := vault.RecoverMaster(rec, key)
	if err != nil {
		return err
	}
	v, err := vault.New(master, rec.KeyVersion)
	if err != nil {
		return err
	}
	if err := v.CheckKey(ctx, st.Q); err != nil {
		return fmt.Errorf("recovered key does not open this database: %w", err)
	}
	_, _ = fmt.Fprintf(out, "\nMaster key (install as HASHI_MASTER_KEY_FILE content):\n%s\n", vault.EncodeMasterKey(master))
	return nil
}
