package secrets

import (
	"context"
	"log/slog"
)

// allowedKeys are attribute keys whose values may reach the log as-is. Every other key's value is
// replaced with "[redacted]" (01-platform.md §1: allow-listed attribute keys). Add a key here when
// you introduce it; a missing key fails safe (redacted), never open. Read-only after init.
var allowedKeys = map[string]bool{
	"err": true, "error": true, "component": true, "version": true, "commit": true,
	"addr": true, "listener": true, "method": true, "path": true, "status": true, "duration_ms": true,
	"kind": true, "run_id": true, "job": true, "next_run_at": true, "summary": true,
	"db_path": true, "migration": true, "from_version": true, "to_version": true, "backup": true,
	"key_id": true, "count": true,
}

// RedactingHandler wraps another slog.Handler and redacts every attribute whose key is not allow-listed.
// Value attributes are redacted by Value.LogValue regardless of key.
type RedactingHandler struct {
	next slog.Handler
}

// NewRedactingHandler returns h wrapped so that non-allow-listed attributes are redacted.
func NewRedactingHandler(h slog.Handler) *RedactingHandler { return &RedactingHandler{next: h} }

func (h *RedactingHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.next.Enabled(ctx, l)
}

func (h *RedactingHandler) Handle(ctx context.Context, r slog.Record) error {
	out := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		out.AddAttrs(redactAttr(a))
		return true
	})
	return h.next.Handle(ctx, out)
}

func (h *RedactingHandler) WithAttrs(as []slog.Attr) slog.Handler {
	red := make([]slog.Attr, len(as))
	for i, a := range as {
		red[i] = redactAttr(a)
	}
	return &RedactingHandler{next: h.next.WithAttrs(red)}
}

func (h *RedactingHandler) WithGroup(name string) slog.Handler {
	return &RedactingHandler{next: h.next.WithGroup(name)}
}

func redactAttr(a slog.Attr) slog.Attr {
	v := a.Value.Resolve()
	if v.Kind() == slog.KindGroup {
		g := v.Group()
		red := make([]slog.Attr, len(g))
		for i, ga := range g {
			red[i] = redactAttr(ga)
		}
		return slog.Attr{Key: a.Key, Value: slog.GroupValue(red...)}
	}
	if !allowedKeys[a.Key] {
		return slog.String(a.Key, redacted)
	}
	return slog.Attr{Key: a.Key, Value: v}
}
