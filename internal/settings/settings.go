// Package settings is the typed settings framework (09-scripts-settings.md §2). Each category is a Go struct stored
// as one JSON row in app_settings, validated on read and on write. Consumers subscribe to changes.
package settings

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// Category is one settings group. Defaults are applied before stored JSON is decoded over them.
type Category interface {
	Key() string
	Validate() error
}

// ErrInvalid wraps validation failures so the API can answer 422 with the message.
var ErrInvalid = errors.New("invalid settings")

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// Service loads, caches and saves categories.
type Service struct {
	st   *store.Store
	mu   sync.RWMutex
	subs []func(Category)
}

// New returns a settings service on st.
func New(st *store.Store) *Service { return &Service{st: st} }

// Subscribe registers fn to run after every successful save (and once per category at Load).
func (s *Service) Subscribe(fn func(Category)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subs = append(s.subs, fn)
}

// Get returns the stored category over its defaults. A stored value that no longer validates is
// an error: the UI must never save values it failed to load.
func Get[T Category](ctx context.Context, q *db.Queries) (T, error) {
	var v T
	v = defaults(v).(T)
	row, err := q.GetSetting(ctx, v.Key())
	if errors.Is(err, sql.ErrNoRows) {
		return v, nil
	}
	if err != nil {
		return v, fmt.Errorf("read %s settings: %w", v.Key(), err)
	}
	if err := json.Unmarshal([]byte(row.ValueJson), &v); err != nil {
		return v, fmt.Errorf("decode %s settings: %w", v.Key(), err)
	}
	if err := v.Validate(); err != nil {
		return v, fmt.Errorf("stored %s settings: %w", v.Key(), err)
	}
	return v, nil
}

// Load reads every category once and notifies subscribers, so consumers start with the stored values.
func (s *Service) Load(ctx context.Context) error {
	for _, c := range []Category{General{}, Security{}, Appearance{}} {
		v, err := s.getAny(ctx, c)
		if err != nil {
			return err
		}
		s.notify(v)
	}
	return nil
}

func (s *Service) getAny(ctx context.Context, c Category) (Category, error) {
	switch c.(type) {
	case General:
		return Get[General](ctx, s.st.Q)
	case Security:
		return Get[Security](ctx, s.st.Q)
	case Appearance:
		return Get[Appearance](ctx, s.st.Q)
	}
	return nil, fmt.Errorf("unknown settings category %T", c)
}

// Put validates v and stores it with q (inside the caller's transaction, next to its audit event).
// Call Notify after the transaction commits.
func Put(ctx context.Context, q *db.Queries, v Category) error {
	if err := v.Validate(); err != nil {
		return err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return q.UpsertSetting(ctx, db.UpsertSettingParams{Key: v.Key(), ValueJson: string(b), UpdatedAt: time.Now().UnixMilli()})
}

// Notify tells subscribers that v was saved.
func (s *Service) Notify(v Category) { s.notify(v) }

func (s *Service) notify(v Category) {
	s.mu.RLock()
	subs := append([]func(Category){}, s.subs...)
	s.mu.RUnlock()
	for _, fn := range subs {
		fn(v)
	}
}
