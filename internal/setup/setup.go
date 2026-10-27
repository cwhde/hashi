// Package setup is the persisted, resumable setup state machine (02-setup-auth.md §1). A step is marked done only
// after its validation succeeded; setup never reports complete before a passkey and a confirmed recovery key exist
// (invariant 14).
package setup

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/auth"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// Status of one step.
type Status string

const (
	Pending    Status = "pending"
	InProgress Status = "in_progress"
	Done       Status = "done"
	Failed     Status = "failed"
)

// Step numbers (02 §1 table).
const (
	StepBootstrap = 1
	StepBase      = 2
	StepDNS       = 3
	StepCert      = 4
	StepTraefik   = 5
	StepFirewall  = 6
	StepSystemRes = 7
	StepHTTPS     = 8
	StepPasskey   = 9
	StepOptional  = 10
	StepComplete  = 11
)

// Definition describes a step; Slice says which delivery slice builds it.
type Definition struct {
	N        int
	Key      string
	Title    string
	Required bool
	Slice    string
}

// Steps lists all eleven steps in order.
var Steps = []Definition{
	{StepBootstrap, "bootstrap", "Bootstrap login", true, "S1"},
	{StepBase, "base_settings", "Base settings", true, "S1"},
	{StepDNS, "dns_provider", "DNS provider", true, "S2"},
	{StepCert, "certificates", "Certificate provider", true, "S5"},
	{StepTraefik, "traefik", "Traefik connection", true, "S4"},
	{StepFirewall, "firewall", "Linux firewall host", true, "S6"},
	{StepSystemRes, "system_resource", "Hashi system resource", true, "S5"},
	{StepHTTPS, "https", "HTTPS reachability", true, "S5"},
	{StepPasskey, "passkey", "Passkey and recovery key", true, "S1"},
	{StepOptional, "integrations", "Optional integrations", false, "S8"},
	{StepComplete, "complete", "Complete", true, "S1"},
}

// built are the steps this build implements.
var built = map[int]bool{StepBootstrap: true, StepBase: true, StepPasskey: true, StepComplete: true}

// StepState is a step's definition plus its persisted state.
type StepState struct {
	Definition
	Built     bool
	Status    Status
	LastError string
	UpdatedAt time.Time
}

// State is the whole wizard.
type State struct {
	Steps     []StepState
	Completed bool
}

// Errors returned to the API.
var (
	ErrCompleted        = errors.New("setup is already complete")
	ErrNotReady         = errors.New("earlier required steps are not done")
	ErrNoPasskey        = errors.New("register a passkey first")
	ErrNoPendingKey     = errors.New("no recovery key is waiting for confirmation; generate a new one")
	ErrChecksumMismatch = errors.New("that is not the last group of the recovery key shown")
)

// Service runs the wizard.
type Service struct {
	st    *store.Store
	vault *vault.Vault
	audit *audit.Log
	now   func() time.Time
	// SkipUnbuilt treats steps from later slices as done. Only `-tags dev` builds set it (local dev and E2E).
	SkipUnbuilt bool

	mu      sync.Mutex
	pending map[string]pendingKey // recovery keys shown but not yet confirmed, by session id hash
}

type pendingKey struct {
	key   []byte
	check string
}

// New returns the setup service.
func New(st *store.Store, v *vault.Vault, a *audit.Log) *Service {
	return &Service{st: st, vault: v, audit: a, now: time.Now, pending: map[string]pendingKey{}}
}

// DropSession forgets a recovery key pending for a revoked session.
func (s *Service) DropSession(idHash []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pending, string(idHash))
}

// Load returns the persisted state.
func (s *Service) Load(ctx context.Context, q *db.Queries) (State, error) {
	rows, err := q.ListSetupSteps(ctx)
	if err != nil {
		return State{}, fmt.Errorf("read setup state: %w", err)
	}
	byStep := map[int]db.SetupState{}
	for _, r := range rows {
		byStep[int(r.Step)] = r
	}
	st := State{}
	for _, d := range Steps {
		ss := StepState{Definition: d, Built: built[d.N], Status: Pending}
		if r, ok := byStep[d.N]; ok {
			ss.Status, ss.LastError, ss.UpdatedAt = Status(r.Status), r.LastError, time.UnixMilli(r.UpdatedAt).UTC()
		}
		st.Steps = append(st.Steps, ss)
	}
	st.Completed = st.Steps[StepComplete-1].Status == Done
	return st, nil
}

// Completed reports whether setup has finished.
func (s *Service) Completed(ctx context.Context) (bool, error) {
	st, err := s.Load(ctx, s.st.Q)
	return st.Completed, err
}

func (s *Service) mark(ctx context.Context, q *db.Queries, n int, status Status, inputs any, lastErr string) error {
	b, err := json.Marshal(inputs)
	if err != nil {
		return err
	}
	if inputs == nil {
		b = []byte("{}")
	}
	return q.UpsertSetupStep(ctx, db.UpsertSetupStepParams{
		Step: int64(n), Status: string(status), InputsJson: string(b), LastError: lastErr, UpdatedAt: s.now().UnixMilli(),
	})
}

// MarkDone records a successfully validated step with q (the caller's transaction).
func (s *Service) MarkDone(ctx context.Context, q *db.Queries, n int, inputs any) error {
	return s.mark(ctx, q, n, Done, inputs, "")
}

// MarkFailed records a failed validation; the step does not advance.
func (s *Service) MarkFailed(ctx context.Context, q *db.Queries, n int, reason string) error {
	return s.mark(ctx, q, n, Failed, nil, reason)
}

// requireOpen fails once setup is complete: setup endpoints are single-use.
func (s *Service) requireOpen(ctx context.Context, q *db.Queries) (State, error) {
	st, err := s.Load(ctx, q)
	if err != nil {
		return st, err
	}
	if st.Completed {
		return st, ErrCompleted
	}
	return st, nil
}

// stepDone reports whether step n counts as done (unbuilt steps count only with SkipUnbuilt).
func (s *Service) stepDone(st State, n int) bool {
	ss := st.Steps[n-1]
	return ss.Status == Done || (!ss.Built && s.SkipUnbuilt)
}

// GenerateRecoveryKey creates the recovery key for step 9 (passkey must exist), stores the wrapped master key and
// returns the display form once. The key itself waits in memory for confirmation by this session.
func (s *Service) GenerateRecoveryKey(ctx context.Context, owner []byte) (secrets.Value, error) {
	var display secrets.Value
	err := s.st.Tx(ctx, func(q *db.Queries) error {
		if _, err := s.requireOpen(ctx, q); err != nil {
			return err
		}
		n, err := q.CountPasskeys(ctx)
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNoPasskey
		}
		key, d, check, err := vault.NewRecoveryKey()
		if err != nil {
			return err
		}
		p, err := s.vault.WrapMaster(key)
		if err != nil {
			return err
		}
		if err := q.UpsertRecoveryKey(ctx, p); err != nil {
			return fmt.Errorf("store recovery key: %w", err)
		}
		if err := s.mark(ctx, q, StepPasskey, InProgress, map[string]any{"recovery_key": "generated"}, ""); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, q, audit.Event{Actor: s.audit.SessionActor(owner), Action: "vault.recovery_key.generated"}); err != nil {
			return err
		}
		s.mu.Lock()
		s.pending[string(owner)] = pendingKey{key: key, check: check}
		s.mu.Unlock()
		display = d
		return nil
	})
	return display, err
}

// ConfirmRecoveryKey checks the re-entered checksum group, test-unlocks the stored copy of the master key with the
// pending key, and marks step 9 done.
func (s *Service) ConfirmRecoveryKey(ctx context.Context, owner []byte, checksum string) error {
	s.mu.Lock()
	pk, ok := s.pending[string(owner)]
	s.mu.Unlock()
	if !ok {
		return ErrNoPendingKey
	}
	entered := strings.ToUpper(strings.TrimSpace(checksum))
	if subtle.ConstantTimeCompare([]byte(entered), []byte(pk.check)) != 1 {
		return ErrChecksumMismatch
	}
	err := s.st.Tx(ctx, func(q *db.Queries) error {
		if _, err := s.requireOpen(ctx, q); err != nil {
			return err
		}
		rec, err := q.GetRecoveryKey(ctx)
		if err != nil {
			return fmt.Errorf("read recovery key: %w", err)
		}
		m, err := vault.RecoverMaster(rec, pk.key)
		if err != nil || !s.vault.MatchesMaster(m) {
			_ = s.MarkFailed(ctx, q, StepPasskey, "test unlock with the recovery key failed")
			return errors.New("test unlock with the recovery key failed; generate a new key")
		}
		if err := q.ConfirmRecoveryKey(ctx, sql.NullInt64{Int64: s.now().UnixMilli(), Valid: true}); err != nil {
			return err
		}
		if err := s.MarkDone(ctx, q, StepPasskey, map[string]any{"recovery_key": "confirmed"}); err != nil {
			return err
		}
		return s.audit.Record(ctx, q, audit.Event{Actor: s.audit.SessionActor(owner), Action: "vault.recovery_key.confirmed"})
	})
	if err != nil {
		return err
	}
	s.DropSession(owner)
	return nil
}

// Complete finishes setup (step 11): every required step done, passkey + confirmed recovery key present, then the
// bootstrap credentials are destroyed and bootstrap sessions revoked in the same transaction.
func (s *Service) Complete(ctx context.Context, actor, ip string) error {
	return s.st.Tx(ctx, func(q *db.Queries) error {
		st, err := s.requireOpen(ctx, q)
		if err != nil {
			return err
		}
		for _, d := range Steps {
			if d.Required && d.N != StepComplete && !s.stepDone(st, d.N) {
				return fmt.Errorf("%w: step %d (%s)", ErrNotReady, d.N, d.Title)
			}
		}
		// Invariant 14, checked against the data rather than the step flag.
		n, err := q.CountPasskeys(ctx)
		if err != nil {
			return err
		}
		rk, err := q.GetRecoveryKey(ctx)
		if n == 0 || err != nil || !rk.ConfirmedAt.Valid {
			return fmt.Errorf("%w: passkey and confirmed recovery key required", ErrNotReady)
		}
		if err := q.DeleteBootstrapCredentials(ctx); err != nil {
			return err
		}
		if _, err := q.RevokeSessionsByMethod(ctx, db.RevokeSessionsByMethodParams{
			RevokedAt: sql.NullInt64{Int64: s.now().UnixMilli(), Valid: true}, RevocationReason: sql.NullString{String: auth.ReasonSetupDone, Valid: true},
			AuthMethod: string(auth.MethodBootstrap),
		}); err != nil {
			return err
		}
		if err := s.MarkDone(ctx, q, StepComplete, nil); err != nil {
			return err
		}
		return s.audit.Record(ctx, q, audit.Event{Actor: actor, Action: "setup.completed", IP: ip})
	})
}
