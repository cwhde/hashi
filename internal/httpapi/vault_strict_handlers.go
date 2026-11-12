package httpapi

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"strings"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/auth"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// Strict vault mode (02 §5.4): unlock with the recovery key or a PRF passkey, add and remove passkey unlockers, switch
// modes. Unlocking needs a signed-in admin with secrets.manage; switching modes also a fresh passkey.

func (s *Server) vaultState(ctx context.Context) (gen.VaultState, error) {
	metas, err := s.Store.R.ListSecretMeta(ctx)
	if err != nil {
		return gen.VaultState{}, err
	}
	n := 0
	for _, m := range metas {
		if !strings.HasPrefix(m.Purpose, internalSecretPrefix) {
			n++
		}
	}
	rk, err := s.Store.R.GetRecoveryKey(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return gen.VaultState{}, err
	}
	confirmed := err == nil && rk.ConfirmedAt.Valid
	mode, err := vault.ReadMode(ctx, s.Store.R)
	if err != nil {
		return gen.VaultState{}, err
	}
	rows, err := s.Store.R.ListVaultUnlockers(ctx)
	if err != nil {
		return gen.VaultState{}, err
	}
	us := []gen.VaultUnlocker{}
	for _, u := range rows {
		us = append(us, gen.VaultUnlocker{Id: u.ID, PasskeyId: u.PasskeyID, PasskeyName: u.PasskeyName,
			CreatedAt: time.UnixMilli(u.CreatedAt).UTC()})
	}
	return gen.VaultState{
		Mode: gen.VaultStateMode(mode), Locked: s.Vault.Locked(), KeyVersion: int(s.Vault.KeyVersion()), Secrets: n,
		RecoveryKeyConfirmed: confirmed, Unlockers: us, MasterKeyFile: s.fileKey() != nil,
	}, nil
}

// fileKey returns HASHI_MASTER_KEY_FILE's key when it is this vault's master key, else nil.
func (s *Server) fileKey() []byte {
	if s.MasterKeyFile == "" || s.Vault.Locked() {
		return nil
	}
	k, err := vault.LoadMasterKey(s.MasterKeyFile)
	if err != nil || !s.Vault.MatchesMaster(k) {
		return nil
	}
	return k
}

// unlock installs master and records who unlocked how; a failure is audited too.
func (s *Server) unlock(ctx context.Context, ri *request, method string, master []byte) error {
	err := s.Store.Tx(ctx, func(q *db.Queries) error {
		if err := s.Vault.Unlock(ctx, q, master); err != nil {
			return err
		}
		return s.Audit.Record(ctx, q, audit.Event{Actor: s.actor(ri), Action: "vault.unlocked", IP: ri.ipString(),
			Detail: map[string]any{"method": method}})
	})
	if err != nil {
		s.recordSessionFailure(ctx, ri, "vault.unlocked")
	}
	return err
}

func (s *Server) UnlockVaultWithRecoveryKey(ctx context.Context, req gen.UnlockVaultWithRecoveryKeyRequestObject) (gen.UnlockVaultWithRecoveryKeyResponseObject, error) {
	ri := info(ctx)
	if s.Vault.Locked() {
		key, err := vault.ParseRecoveryKey(req.Body.RecoveryKey)
		if err != nil {
			return nil, invalid(err.Error())
		}
		rec, err := s.Store.R.GetRecoveryKey(ctx)
		if err != nil {
			return nil, conflict("This installation has no recovery key.")
		}
		master, err := vault.RecoverMaster(rec, key)
		if err != nil {
			s.recordSessionFailure(ctx, ri, "vault.unlocked")
			return nil, invalid(err.Error())
		}
		if err := s.unlock(ctx, ri, "recovery_key", master); err != nil {
			return nil, err
		}
	}
	st, err := s.vaultState(ctx)
	if err != nil {
		return nil, err
	}
	return gen.UnlockVaultWithRecoveryKey200JSONResponse(st), nil
}

func (s *Server) VaultUnlockOptions(ctx context.Context, _ gen.VaultUnlockOptionsRequestObject) (gen.VaultUnlockOptionsResponseObject, error) {
	if !s.Vault.Locked() {
		return nil, conflict("The vault is already unlocked.")
	}
	rows, err := s.Store.R.ListVaultUnlockers(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, conflict("No passkey can unlock the vault; use the recovery key.")
	}
	salts := map[string][]byte{}
	for _, u := range rows {
		salts[string(u.CredentialID)] = u.Salt
	}
	id, opts, err := s.Passkeys.BeginPRF(ctx, info(ctx).session.IDHash, auth.PRFUnlock, nil, salts)
	if err != nil {
		return nil, ceremonyError(err)
	}
	m, err := optionsMap(opts)
	if err != nil {
		return nil, err
	}
	return gen.VaultUnlockOptions200JSONResponse{CeremonyId: id, Options: m}, nil
}

func (s *Server) UnlockVaultWithPasskey(ctx context.Context, req gen.UnlockVaultWithPasskeyRequestObject) (gen.UnlockVaultWithPasskeyResponseObject, error) {
	ri := info(ctx)
	cred, err := credentialBytes(req.Body.Credential)
	if err != nil {
		return nil, err
	}
	var passkey string
	var prf []byte
	if err := s.Store.Tx(ctx, func(q *db.Queries) error {
		passkey, prf, _, err = s.Passkeys.FinishPRF(ctx, q, ri.session.IDHash, auth.PRFUnlock, req.Body.CeremonyId, cred)
		return err
	}); err != nil {
		s.recordSessionFailure(ctx, ri, "vault.unlocked")
		return nil, ceremonyError(err)
	}
	rows, err := s.Store.R.ListVaultUnlockers(ctx)
	if err != nil {
		return nil, err
	}
	for _, u := range rows {
		if u.PasskeyID != passkey {
			continue
		}
		master, err := vault.OpenUnlocker(db.VaultUnlocker{ID: u.ID, PasskeyID: u.PasskeyID, Salt: u.Salt, KeyVersion: u.KeyVersion,
			Nonce: u.Nonce, WrappedMaster: u.WrappedMaster, CreatedAt: u.CreatedAt}, prf)
		if err != nil {
			s.recordSessionFailure(ctx, ri, "vault.unlocked")
			return nil, invalid(err.Error())
		}
		if err := s.unlock(ctx, ri, "passkey", master); err != nil {
			return nil, err
		}
		st, err := s.vaultState(ctx)
		if err != nil {
			return nil, err
		}
		return gen.UnlockVaultWithPasskey200JSONResponse(st), nil
	}
	return nil, invalid("This passkey can't unlock the vault.")
}

func (s *Server) VaultUnlockerOptions(ctx context.Context, _ gen.VaultUnlockerOptionsRequestObject) (gen.VaultUnlockerOptionsResponseObject, error) {
	if s.Vault.Locked() {
		return nil, vaultLocked()
	}
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	id, opts, err := s.Passkeys.BeginPRF(ctx, info(ctx).session.IDHash, auth.PRFEnroll, salt, nil)
	if err != nil {
		return nil, ceremonyError(err)
	}
	m, err := optionsMap(opts)
	if err != nil {
		return nil, err
	}
	return gen.VaultUnlockerOptions200JSONResponse{CeremonyId: id, Options: m}, nil
}

func (s *Server) AddVaultUnlocker(ctx context.Context, req gen.AddVaultUnlockerRequestObject) (gen.AddVaultUnlockerResponseObject, error) {
	ri := info(ctx)
	cred, err := credentialBytes(req.Body.Credential)
	if err != nil {
		return nil, err
	}
	err = s.Store.Tx(ctx, func(q *db.Queries) error {
		passkey, prf, salt, err := s.Passkeys.FinishPRF(ctx, q, ri.session.IDHash, auth.PRFEnroll, req.Body.CeremonyId, cred)
		if err != nil {
			return err
		}
		u, err := s.Vault.NewUnlocker(passkey, salt, prf)
		if err != nil {
			return err
		}
		if err := q.InsertVaultUnlocker(ctx, u); err != nil {
			return err
		}
		return s.Audit.Record(ctx, q, audit.Event{Actor: s.actor(ri), Action: "vault.unlocker.added", Target: "passkey:" + passkey,
			IP: ri.ipString()})
	})
	switch {
	case errors.Is(err, vault.ErrNoPRF):
		return nil, invalid(err.Error())
	case err != nil:
		return nil, ceremonyError(err)
	}
	st, err := s.vaultState(ctx)
	if err != nil {
		return nil, err
	}
	return gen.AddVaultUnlocker200JSONResponse(st), nil
}

func (s *Server) DeleteVaultUnlocker(ctx context.Context, req gen.DeleteVaultUnlockerRequestObject) (gen.DeleteVaultUnlockerResponseObject, error) {
	ri := info(ctx)
	err := s.Store.Tx(ctx, func(q *db.Queries) error {
		n, err := q.DeleteVaultUnlocker(ctx, req.Id)
		if err != nil {
			return err
		}
		if n == 0 {
			return notFound("No such unlocker.")
		}
		return s.Audit.Record(ctx, q, audit.Event{Actor: s.actor(ri), Action: "vault.unlocker.removed", Target: "unlocker:" + req.Id,
			IP: ri.ipString()})
	})
	if err != nil {
		return nil, err
	}
	st, err := s.vaultState(ctx)
	if err != nil {
		return nil, err
	}
	return gen.DeleteVaultUnlocker200JSONResponse(st), nil
}

func (s *Server) SetVaultMode(ctx context.Context, req gen.SetVaultModeRequestObject) (gen.SetVaultModeResponseObject, error) {
	ri := info(ctx)
	want := vault.Mode(req.Body.Mode)
	have, err := vault.ReadMode(ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	if want != have {
		if s.Vault.Locked() {
			return nil, vaultLocked()
		}
		if want == vault.ModeStrict {
			rk, err := s.Store.R.GetRecoveryKey(ctx)
			if err != nil || !rk.ConfirmedAt.Valid {
				return nil, conflict("Confirm the recovery key first (setup step 9): without it a lost passkey means a locked vault for good.")
			}
		}
		fileKey := s.fileKey()
		err = s.Store.Tx(ctx, func(q *db.Queries) error {
			var err error
			if want == vault.ModeStrict {
				err = s.Vault.EnableStrict(ctx, q)
			} else {
				err = s.Vault.DisableStrict(ctx, q, fileKey)
			}
			if err != nil {
				return err
			}
			return s.Audit.Record(ctx, q, audit.Event{Actor: s.actor(ri), Action: "vault.mode.changed", IP: ri.ipString(),
				Detail: map[string]any{"from": string(have), "to": string(want)}})
		})
		if errors.Is(err, vault.ErrFileKeyMismatch) {
			return nil, conflict(err.Error())
		}
		if err != nil {
			return nil, err
		}
	}
	st, err := s.vaultState(ctx)
	if err != nil {
		return nil, err
	}
	return gen.SetVaultMode200JSONResponse(st), nil
}
