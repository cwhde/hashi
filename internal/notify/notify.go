// Package notify delivers monitoring notifications (07 §6): providers, the route, cooldowns, and deliveries with retry.
package notify

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// Job kind and interval (invariant 15).
const (
	JobKind     = "notify.deliver"
	JobInterval = 15 * time.Second
)

// MaxAttempts bounds retries; backoff doubles from a minute.
const MaxAttempts = 6

// Errors the API maps.
var (
	ErrNotFound = errors.New("notification provider not found")
	ErrInvalid  = errors.New("invalid")
)

// Message is what a provider sends.
type Message struct{ Subject, Body string }

// Sender delivers to one configured provider.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

// Service keeps providers and deliveries.
type Service struct {
	st    *store.Store
	vault *vault.Vault
	now   func() time.Time
	// build makes the sender for a provider row; tests replace it.
	build func(db.NotificationProvider, secrets.Value) (Sender, error)
}

// New returns the service.
func New(st *store.Store, v *vault.Vault) *Service {
	return &Service{st: st, vault: v, now: time.Now, build: buildSender}
}

func buildSender(p db.NotificationProvider, password secrets.Value) (Sender, error) {
	if p.Type == TypeTelegram {
		var c TelegramConfig
		if err := json.Unmarshal([]byte(p.ConfigJson), &c); err != nil {
			return nil, fmt.Errorf("provider %s config: %w", p.Name, err)
		}
		return Telegram{Token: password, ChatID: c.ChatID}, nil
	}
	if p.Type != TypeSMTP {
		return nil, fmt.Errorf("provider type %s", p.Type)
	}
	var c SMTPConfig
	if err := json.Unmarshal([]byte(p.ConfigJson), &c); err != nil {
		return nil, fmt.Errorf("provider %s config: %w", p.Name, err)
	}
	return SMTP{Config: c, Password: password}, nil
}

// CreateSMTP stores an SMTP provider with its password in the vault (invariant 5).
func (s *Service) CreateSMTP(ctx context.Context, name string, c SMTPConfig, password secrets.Value) (db.NotificationProvider, error) {
	if err := c.Validate(); err != nil {
		return db.NotificationProvider{}, err
	}
	return s.create(ctx, TypeSMTP, name, c, password)
}

// CreateTelegram stores a Telegram provider with its bot token in the vault (invariant 5).
func (s *Service) CreateTelegram(ctx context.Context, name string, c TelegramConfig, token secrets.Value) (db.NotificationProvider, error) {
	if err := c.Validate(); err != nil {
		return db.NotificationProvider{}, err
	}
	if token.IsZero() {
		return db.NotificationProvider{}, fmt.Errorf("%w: a bot token is required", ErrInvalid)
	}
	return s.create(ctx, TypeTelegram, name, c, token)
}

func (s *Service) create(ctx context.Context, typ, name string, config any, secret secrets.Value) (db.NotificationProvider, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return db.NotificationProvider{}, fmt.Errorf("%w: a name is required", ErrInvalid)
	}
	b, err := json.Marshal(config)
	if err != nil {
		return db.NotificationProvider{}, err
	}
	now := s.now().UnixMilli()
	row := db.NotificationProvider{ID: ulid.Make().String(), Type: typ, Name: name, ConfigJson: string(b), CreatedAt: now, UpdatedAt: now}
	err = s.st.Tx(ctx, func(q *db.Queries) error {
		if !secret.IsZero() {
			p, err := s.vault.Seal(vault.Service, "notification:"+row.ID, secret)
			if err != nil {
				return err
			}
			if err := q.InsertSecret(ctx, p); err != nil {
				return err
			}
			row.SecretID = sql.NullString{String: p.ID, Valid: true}
		}
		return q.InsertNotificationProvider(ctx, db.InsertNotificationProviderParams{ID: row.ID, Type: row.Type, Name: row.Name, ConfigJson: row.ConfigJson,
			SecretID: row.SecretID, CreatedAt: now, UpdatedAt: now})
	})
	return row, err
}

// Delete removes a provider and its secret. Its deliveries go with it; a route naming it falls back to none.
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.st.Tx(ctx, func(q *db.Queries) error {
		p, err := q.GetNotificationProvider(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := q.DeleteNotificationProvider(ctx, id); err != nil {
			return err
		}
		if p.SecretID.Valid {
			return q.DeleteSecret(ctx, p.SecretID.String)
		}
		return nil
	})
}

// EnqueueTest queues a test message through the normal delivery path and runs it now.
func (s *Service) EnqueueTest(ctx context.Context, id string) error {
	if _, err := s.st.R.GetNotificationProvider(ctx, id); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	now := s.now().UnixMilli()
	if err := s.st.Q.InsertNotificationDelivery(ctx, db.InsertNotificationDeliveryParams{ID: ulid.Make().String(), ProviderID: id, Kind: "test",
		Subject: "Hashi test message", Body: "This is a test message from Hashi. Notifications to this provider work.", NextAttemptAt: now, CreatedAt: now}); err != nil {
		return err
	}
	_, err := s.Deliver(ctx)
	return err
}

// StateChanged applies the route (07 §6) inside the transaction that records the state change: an endpoint's override
// provider or the default one, and the severity threshold. An alert inside the cooldown is not dropped but waits for its
// end, and goes only if the endpoint is still in alarm then: an outage is never left at "up again". A recovery cancels a
// waiting alert and then says nothing, because no alarm was sent.
func (s *Service) StateChanged(ctx context.Context, q *db.Queries, e db.MonitorEndpoint, from, to, message string) error {
	r, err := q.GetNotificationRoute(ctx)
	if err != nil {
		return err
	}
	provider := r.ProviderID
	if e.RouteProviderID.Valid {
		provider = e.RouteProviderID
	}
	if !provider.Valid {
		return nil
	}
	now := s.now().UnixMilli()
	id := sql.NullString{String: e.ID, Valid: true}
	alarm := func(st string) bool { return st == "down" || (st == "degraded" && r.MinSeverity == "degraded") }
	insert := func(kind, subject string, at int64) error {
		return q.InsertNotificationDelivery(ctx, db.InsertNotificationDeliveryParams{ID: ulid.Make().String(), ProviderID: provider.String,
			EndpointID: id, Kind: kind, Subject: subject, Body: message, NextAttemptAt: at, CreatedAt: now})
	}
	switch {
	case alarm(to):
		waiting, err := q.PendingAlerts(ctx, id)
		if err != nil {
			return err
		}
		if waiting > 0 {
			return nil
		}
		last, err := q.LastAlertAt(ctx, id)
		if err != nil {
			return err
		}
		at := now
		if last > 0 && now-last < r.CooldownSeconds*1000 {
			at = last + r.CooldownSeconds*1000
		}
		return insert("alert", e.Name+" is "+to, at)
	case alarm(from):
		cancelled, err := q.DeletePendingAlerts(ctx, id)
		if err != nil {
			return err
		}
		if cancelled > 0 || r.Recovery != 1 {
			return nil
		}
		return insert("recovery", e.Name+" is "+to+" again", now)
	}
	return nil
}

// Deliver is the job: it sends what is due, retrying with backoff until MaxAttempts. Nothing is held in a transaction
// while talking to a provider. A failure stores only the provider's error text.
func (s *Service) Deliver(ctx context.Context) (string, error) {
	now := s.now()
	due, err := s.st.R.DueNotificationDeliveries(ctx, now.UnixMilli())
	if err != nil {
		return "", err
	}
	sent, failed := 0, 0
	for _, d := range due {
		if d.Kind == "alert" && d.EndpointID.Valid && d.Attempts == 0 {
			e, err := s.st.R.GetMonitorEndpoint(ctx, d.EndpointID.String)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return "", err
			}
			if err != nil || (e.State != "down" && e.State != "degraded") { // it recovered while the alert waited
				if err := s.st.Q.DeleteNotificationDelivery(ctx, d.ID); err != nil {
					return "", err
				}
				continue
			}
		}
		sendErr := s.sendOne(ctx, d)
		attempts := d.Attempts + 1
		p := db.FinishNotificationDeliveryParams{Status: "delivered", Attempts: attempts, NextAttemptAt: d.NextAttemptAt, ID: d.ID}
		if sendErr != nil {
			failed++
			p.LastError = sendErr.Error()
			p.Status = "pending"
			p.NextAttemptAt = now.Add(time.Minute << min(d.Attempts, 10)).UnixMilli()
			if attempts >= MaxAttempts {
				p.Status = "failed"
			}
		} else {
			sent++
			p.DeliveredAt = sql.NullInt64{Int64: s.now().UnixMilli(), Valid: true}
		}
		if err := s.st.Q.FinishNotificationDelivery(ctx, p); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("%d sent, %d failed", sent, failed), nil
}

func (s *Service) sendOne(ctx context.Context, d db.NotificationDelivery) error {
	p, err := s.st.R.GetNotificationProvider(ctx, d.ProviderID)
	if err != nil {
		return err
	}
	var password secrets.Value
	if p.SecretID.Valid {
		rec, err := s.st.R.GetSecret(ctx, p.SecretID.String)
		if err != nil {
			return err
		}
		if password, err = s.vault.Open(rec); err != nil {
			return err
		}
	}
	sender, err := s.build(p, password)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return sender.Send(ctx, Message{Subject: d.Subject, Body: d.Body})
}

func nullStr(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }
