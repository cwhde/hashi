package vault

import "git.juzo.io/juzo/hashi/internal/store/db"

func asRecord(p db.InsertSecretParams) db.SecretRecord {
	return db.SecretRecord{
		ID: p.ID, Class: p.Class, Purpose: p.Purpose, KeyVersion: p.KeyVersion, Nonce: p.Nonce,
		Ciphertext: p.Ciphertext, DekNonce: p.DekNonce, WrappedDek: p.WrappedDek, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

func recoveryRecord(p db.UpsertRecoveryKeyParams) db.RecoveryKey {
	return db.RecoveryKey{ID: 1, KeyVersion: p.KeyVersion, Verifier: p.Verifier, Nonce: p.Nonce, WrappedMaster: p.WrappedMaster, CreatedAt: p.CreatedAt}
}
