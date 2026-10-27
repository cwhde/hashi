// Package logring keeps the most recent log lines in memory for Activity → Live log. It sits behind the redacting
// handler (it is an io.Writer for the JSON handler), so it only ever holds already-redacted lines.
package logring

import (
	"bytes"
	"sync"
)

// Line is one JSON log record with its sequence number (monotonic since process start).
type Line struct {
	Seq  int64
	JSON []byte
}

// Ring holds up to size lines.
type Ring struct {
	mu    sync.Mutex
	lines []Line
	size  int
	seq   int64
}

// New returns a ring holding size lines.
func New(size int) *Ring { return &Ring{size: size, lines: make([]Line, 0, size)} }

// Write stores one record. slog's JSON handler emits exactly one record per Write.
func (r *Ring) Write(p []byte) (int, error) {
	line := bytes.TrimSpace(p)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	if len(r.lines) == r.size {
		copy(r.lines, r.lines[1:])
		r.lines = r.lines[:r.size-1]
	}
	r.lines = append(r.lines, Line{Seq: r.seq, JSON: append([]byte(nil), line...)})
	return len(p), nil
}

// After returns lines with Seq > after, oldest first, and the latest sequence number.
func (r *Ring) After(after int64) ([]Line, int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Line
	for _, l := range r.lines {
		if l.Seq > after {
			out = append(out, l)
		}
	}
	return out, r.seq
}
