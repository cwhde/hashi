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
	"git.juzo.io/juzo/hashi/internal/settings"
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

// MaxGroupWait caps how long grouping holds the first alert back, however many follow (07 §6).
const MaxGroupWait = 2 * time.Minute

// Errors the API maps.
var (
	ErrNotFound = errors.New("notification provider not found")
	ErrInvalid  = errors.New("invalid")
)

// Message is what a provider sends. Subject and Body are always set for a real delivery; the rest shapes its look.
// Nothing here may name an internal address: it leaves through someone else's mail or chat service.
type Message struct {
	Subject, Body string
	State         string                // up, degraded, down or test: picks the colour
	Theme         string                // the admin's theme, for mail
	Custom        *settings.CustomTheme // its colours when Theme is custom
	Link          string                // this alert's page in the admin UI
	At            time.Time
	Stats         []Stat
}

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

// Update changes a provider's name and settings. Its type is fixed. A zero secret keeps the stored one.
func (s *Service) Update(ctx context.Context, id, name string, config func(typ string) (any, error), secret secrets.Value) (db.NotificationProvider, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return db.NotificationProvider{}, fmt.Errorf("%w: a name is required", ErrInvalid)
	}
	var row db.NotificationProvider
	err := s.st.Tx(ctx, func(q *db.Queries) error {
		var err error
		row, err = q.GetNotificationProvider(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		c, err := config(row.Type)
		if err != nil {
			return err
		}
		b, err := json.Marshal(c)
		if err != nil {
			return err
		}
		old := row.SecretID
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
		row.Name, row.ConfigJson, row.UpdatedAt = name, string(b), s.now().UnixMilli()
		if err := q.UpdateNotificationProvider(ctx, db.UpdateNotificationProviderParams{Name: row.Name, ConfigJson: row.ConfigJson, SecretID: row.SecretID, UpdatedAt: row.UpdatedAt, ID: row.ID}); err != nil {
			return err
		}
		if old.Valid && old != row.SecretID {
			return q.DeleteSecret(ctx, old.String)
		}
		return nil
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
	insert := func(kind, subject string, at int64, body string) error {
		return q.InsertNotificationDelivery(ctx, db.InsertNotificationDeliveryParams{ID: ulid.Make().String(), ProviderID: provider.String,
			EndpointID: id, Kind: kind, Subject: subject, Body: body, NextAttemptAt: at, CreatedAt: now})
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
		if r.GroupSeconds > 0 { // wait a moment for others; each new alert restarts the wait, up to MaxGroupWait
			until := now + r.GroupSeconds*1000
			oldest, err := q.OldestGroupedAlert(ctx, provider.String)
			if err != nil {
				return err
			}
			if oldest > 0 {
				until = min(until, max(oldest+MaxGroupWait.Milliseconds(), now))
			}
			if err := q.DelayGroupedAlerts(ctx, db.DelayGroupedAlertsParams{Until: until, ProviderID: provider.String}); err != nil {
				return err
			}
			at = max(at, until)
		}
		return insert("alert", e.Name+" is "+to, at, plainReason(message))
	case alarm(from):
		cancelled, err := q.DeletePendingAlerts(ctx, id)
		if err != nil {
			return err
		}
		if cancelled > 0 || r.Recovery != 1 {
			return nil
		}
		return insert("recovery", e.Name+" is "+to+" again", now, "It was "+from+" for "+plainDuration(s.now().Sub(time.UnixMilli(e.StateSince)))+".")
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
	// Alerts due for the same provider leave as one message, so a host going down with twenty services is one alert.
	var batches [][]db.NotificationDelivery
	alerts := map[string]int{} // provider → its batch
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
		if d.Kind != "alert" {
			batches = append(batches, []db.NotificationDelivery{d})
			continue
		}
		if i, ok := alerts[d.ProviderID]; ok {
			batches[i] = append(batches[i], d)
			continue
		}
		alerts[d.ProviderID] = len(batches)
		batches = append(batches, []db.NotificationDelivery{d})
	}
	sent, failed := 0, 0
	for _, b := range batches {
		sendErr := s.send(ctx, b)
		for _, d := range b {
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
	}
	return fmt.Sprintf("%d sent, %d failed", sent, failed), nil
}

// send delivers one delivery, or several alerts for the same provider as one message.
func (s *Service) send(ctx context.Context, b []db.NotificationDelivery) error {
	p, err := s.st.R.GetNotificationProvider(ctx, b[0].ProviderID)
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
	m := s.message(ctx, b[0])
	if len(b) > 1 {
		m = s.combined(ctx, b)
	}
	return sender.Send(ctx, m)
}

// combined is one message for several alerts: a subject that counts them and one line per check. A host's own check
// (Traefik host, firewall host, Pulse agent) goes first and is named in the subject: it is the likely cause (bug 12).
func (s *Service) combined(ctx context.Context, b []db.NotificationDelivery) Message {
	m := s.message(ctx, b[0])
	m.Stats, m.State = nil, "degraded"
	var hosts, rest []db.NotificationDelivery
	var names []string
	for _, d := range b {
		e, err := s.st.R.GetMonitorEndpoint(ctx, d.EndpointID.String)
		if err == nil && (e.SourceType == "traefik" || e.SourceType == "firewall_host" || e.SourceType == "pulse") {
			hosts = append(hosts, d)
			names = append(names, e.Name)
		} else {
			rest = append(rest, d)
		}
	}
	down := 0
	lines := make([]string, 0, len(b))
	for _, d := range append(hosts, rest...) {
		if !strings.HasSuffix(d.Subject, " is degraded") {
			down++
			m.State = "down"
		}
		line := d.Subject
		if d.Body != "" {
			line += ": " + d.Body
		}
		lines = append(lines, line)
		if at := time.UnixMilli(d.CreatedAt); at.Before(m.At) {
			m.At = at
		}
	}
	verb := "are down or degraded"
	switch down {
	case len(b):
		verb = "are down"
	case 0:
		verb = "are degraded"
	}
	switch {
	case len(hosts) == 0:
		m.Subject = fmt.Sprintf("%d checks %s", len(b), verb)
	case len(rest) == 0:
		m.Subject = strings.Join(names, ", ") + " " + verb
	case len(rest) == 1:
		m.Subject = strings.Join(names, ", ") + " and 1 other check " + verb
	default:
		m.Subject = fmt.Sprintf("%s and %d other checks %s", strings.Join(names, ", "), len(rest), verb)
	}
	m.Body = strings.Join(lines, "\n")
	if g, err := settings.Get[settings.General](ctx, s.st.R); err == nil && g.AdminDomain != "" {
		m.Link = "https://" + g.AdminDomain + "/status"
	}
	return m
}

// message dresses a delivery. Everything added is optional: a failed lookup leaves it out and never blocks an alert.
func (s *Service) message(ctx context.Context, d db.NotificationDelivery) Message {
	m := Message{Subject: d.Subject, Body: d.Body, At: time.UnixMilli(d.CreatedAt)}
	if a, err := settings.Get[settings.Appearance](ctx, s.st.R); err == nil {
		m.Theme, m.Custom = a.Theme, a.CustomTheme
	}
	path := "/status"
	switch {
	case d.Kind == "test":
		m.State = "test"
	case d.Kind == "recovery":
		m.State = "up"
	case strings.HasSuffix(d.Subject, " is degraded"):
		m.State = "degraded"
	default:
		m.State = "down"
	}
	if d.EndpointID.Valid {
		path += "/" + d.EndpointID.String
		for _, w := range []struct {
			label string
			since time.Duration
		}{{"24 h", 24 * time.Hour}, {"7 d", 7 * 24 * time.Hour}, {"30 d", 30 * 24 * time.Hour}} {
			r, err := s.st.R.UptimeSince(ctx, db.UptimeSinceParams{EndpointID: d.EndpointID.String, At: s.now().Add(-w.since).UnixMilli()})
			if err == nil && r.Total > 0 {
				m.Stats = append(m.Stats, Stat{Label: w.label, Percent: float64(r.OkCount) / float64(r.Total) * 100})
			}
		}
	}
	if g, err := settings.Get[settings.General](ctx, s.st.R); err == nil && g.AdminDomain != "" {
		m.Link = "https://" + g.AdminDomain + path
	}
	return m
}

func nullStr(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }
