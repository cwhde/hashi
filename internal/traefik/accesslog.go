package traefik

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"git.juzo.io/juzo/hashi/internal/sshx"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// Access-log tail (06 §6.2): Hashi follows every installed Traefik's JSON access log over one SSH connection per host.
// The cursor is the file's inode and the byte offset of the next unread line; it is stored, so a restart or a lost
// connection resumes where it stopped. After a rotation (logrotate renames the file, 04 §3) the rest of the old inode is
// read first. Commands were tried on BusyBox 1.37 (Alpine) and GNU coreutils.

const (
	// TailRead bounds one read (06 §6.2 allows 8 MB; one SSH command keeps at most sshx.MaxOutput).
	TailRead = sshx.MaxOutput
	// TailEvery is the poll interval (target lag: a few seconds).
	TailEvery = 2 * time.Second
)

// AccessLine is one parsed access-log line, the fields Hashi uses (testdata/contracts/traefik/access_log_tail.jsonl).
type AccessLine struct {
	Client     netip.Addr
	Host       string
	Method     string
	Path       string
	Status     int
	Router     string
	EntryPoint string
	Start      time.Time
	// Raw keeps the decoded line for consumers that read more fields (the WAF plugin's).
	Raw map[string]json.RawMessage
}

type rawLine struct {
	ClientHost       string    `json:"ClientHost"`
	RequestHost      string    `json:"RequestHost"`
	RequestMethod    string    `json:"RequestMethod"`
	RequestPath      string    `json:"RequestPath"`
	DownstreamStatus int       `json:"DownstreamStatus"`
	RouterName       string    `json:"RouterName"`
	EntryPointName   string    `json:"entryPointName"`
	StartUTC         time.Time `json:"StartUTC"`
}

// FAEntryPoint is the loopback entry point of the forward-auth hop (06 §2.1). Every protected request is logged twice,
// once there with Traefik itself as the client (integration test); the tail drops those lines.
const FAEntryPoint = "hashi-fa"

// ParseAccessLines decodes complete lines of client requests; a line that is not JSON is skipped and counted, a
// forward-auth hop is skipped.
func ParseAccessLines(b []byte) (out []AccessLine, bad int) {
	for l := range bytes.SplitSeq(b, []byte{'\n'}) {
		if len(bytes.TrimSpace(l)) == 0 {
			continue
		}
		var r rawLine
		var all map[string]json.RawMessage
		if json.Unmarshal(l, &r) != nil || json.Unmarshal(l, &all) != nil {
			bad++
			continue
		}
		if r.EntryPointName == FAEntryPoint {
			continue
		}
		a, _ := netip.ParseAddr(r.ClientHost)
		out = append(out, AccessLine{Client: a.Unmap(), Host: r.RequestHost, Method: r.RequestMethod, Path: r.RequestPath,
			Status: r.DownstreamStatus, Router: r.RouterName, EntryPoint: r.EntryPointName, Start: r.StartUTC, Raw: all})
	}
	return out, bad
}

// LineSink receives the access-log lines of one Traefik host in order.
type LineSink func(ctx context.Context, connID string, lines []AccessLine)

// AccessLogFile is the file the tail follows in LogDir.
const AccessLogFile = "access.log"

// Tailer runs one tail per installed Traefik host.
type Tailer struct {
	m    *Manager
	log  *slog.Logger
	sink LineSink
	dir  string // LogDir; tests use another
	mu   sync.Mutex
	runs map[string]context.CancelFunc
}

// NewTailer returns the tails of m's installed hosts, delivering access-log lines to sink.
func NewTailer(m *Manager, log *slog.Logger, sink LineSink) *Tailer {
	return &Tailer{m: m, log: log, sink: sink, dir: LogDir, runs: map[string]context.CancelFunc{}}
}

// Run starts a tail per installed host and stops tails of hosts that are gone, until ctx ends.
func (t *Tailer) Run(ctx context.Context) {
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()
	for {
		rows, err := t.m.st.R.ListTraefikConnections(ctx)
		if err != nil && ctx.Err() == nil {
			t.log.Warn("access log: list connections", "err", err)
		}
		want := map[string]bool{}
		for _, r := range rows {
			if r.InstalledVersion != "" && r.OwnershipConfirmedAt.Valid {
				want[r.ID] = true
			}
		}
		t.mu.Lock()
		for id, stop := range t.runs {
			if !want[id] {
				stop()
				delete(t.runs, id)
			}
		}
		for id := range want {
			if _, ok := t.runs[id]; !ok {
				c, stop := context.WithCancel(ctx)
				t.runs[id] = stop
				go t.host(c, id)
			}
		}
		t.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (t *Tailer) host(ctx context.Context, id string) {
	backoff := time.Second
	for ctx.Err() == nil {
		row, err := t.m.st.R.GetTraefikConnection(ctx, id)
		if err != nil {
			return
		}
		conn, err := t.m.dial(ctx, row)
		if err == nil {
			backoff = time.Second
			acc := t.cursor(ctx, id, AccessLogFile)
			for err == nil {
				acc, err = t.Poll(ctx, conn, id, AccessLogFile, acc)
				if err == nil && !wait(ctx, TailEvery) {
					err = ctx.Err()
				}
			}
			_ = conn.Close()
		}
		if ctx.Err() != nil {
			return
		}
		t.save(ctx, id, AccessLogFile, Cursor{}, 0, false, err.Error(), true)
		t.log.Warn("access log tail down", "connection", row.Name, "err", err)
		if !wait(ctx, backoff) {
			return
		}
		backoff = min(2*backoff, time.Minute)
	}
}

func wait(ctx context.Context, d time.Duration) bool {
	tm := time.NewTimer(d)
	defer tm.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-tm.C:
		return true
	}
}

// Cursor is where the next unread line starts. Inode 0 means "not started": the tail begins at the current end.
type Cursor struct {
	Inode, Offset int64
}

func (t *Tailer) cursor(ctx context.Context, id, file string) Cursor {
	c, err := t.m.st.R.GetAccessLogCursor(ctx, db.GetAccessLogCursorParams{TraefikConnectionID: id, File: file})
	if err != nil {
		return Cursor{}
	}
	return Cursor{Inode: c.Inode, Offset: c.Offset}
}

func (t *Tailer) save(ctx context.Context, id, file string, c Cursor, lag time.Duration, read bool, msg string, errOnly bool) {
	now := t.m.now().UnixMilli()
	if errOnly {
		old, err := t.m.st.R.GetAccessLogCursor(ctx, db.GetAccessLogCursorParams{TraefikConnectionID: id, File: file})
		if errors.Is(err, sql.ErrNoRows) {
			return
		}
		c = Cursor{Inode: old.Inode, Offset: old.Offset}
	}
	p := db.PutAccessLogCursorParams{TraefikConnectionID: id, File: file, Inode: c.Inode, Offset: c.Offset, LagMs: lag.Milliseconds(),
		Error: msg, UpdatedAt: now}
	if read {
		p.LastReadAt = sql.NullInt64{Int64: now, Valid: true}
	}
	if err := t.m.st.Q.PutAccessLogCursor(ctx, p); err != nil && ctx.Err() == nil {
		t.log.Warn("log tail cursor", "connection", id, "file", file, "err", err)
	}
}

// Poll reads what was written to file since cur, delivers whole lines and stores the new cursor.
func (t *Tailer) Poll(ctx context.Context, h Host, id, file string, cur Cursor) (Cursor, error) {
	b, next, note, err := Follow(ctx, h, t.dir, file, cur)
	if err != nil {
		return cur, err
	}
	if note != "" {
		t.log.Warn(file+": "+note, "connection", id)
	}
	lag := t.deliver(ctx, id, b)
	t.save(ctx, id, file, next, lag, true, "", false)
	return next, nil
}

// Follow reads the whole lines written to dir/name since cur and returns them with the next cursor. A zero cursor
// starts at the current end. A file renamed by logrotate is finished first; one truncated in place restarts at 0. note says when lines were lost (a rotated file compressed before it was finished).
func Follow(ctx context.Context, h Host, dir, name string, cur Cursor) (lines []byte, next Cursor, note string, err error) {
	file := dir + "/" + name
	ino, size, err := statFile(ctx, h, file)
	if err != nil || ino == 0 { // ino 0: no such log yet (nothing written since the install)
		return nil, cur, "", err
	}
	if cur.Inode == 0 {
		return nil, Cursor{Inode: ino, Offset: size}, "", nil
	}
	var out []byte
	if cur.Inode != ino {
		// Rotated: finish the old file if it is still there under another name (not yet compressed).
		old, err := findInode(ctx, h, dir, name, cur.Inode)
		if err != nil {
			return nil, cur, "", err
		}
		if old == "" {
			note = "rotated and compressed before it was read to the end"
		} else {
			b, err := readFrom(ctx, h, old, cur.Offset)
			if err != nil {
				return nil, cur, "", err
			}
			done, rest := wholeLines(b)
			if len(b) == TailRead { // more of the old file than one read: continue there next time
				return done, Cursor{Inode: cur.Inode, Offset: cur.Offset + int64(len(done))}, "", nil
			}
			if len(rest) > 0 {
				note = "unfinished last line in the rotated file"
			}
			out = done
		}
		cur = Cursor{Inode: ino}
	}
	if size < cur.Offset { // truncated in place
		cur.Offset = 0
	}
	if size > cur.Offset {
		b, err := readFrom(ctx, h, file, cur.Offset)
		if err != nil {
			return nil, cur, "", err
		}
		done, _ := wholeLines(b)
		out = append(out, done...)
		cur.Offset += int64(len(done))
	}
	return out, cur, note, nil
}

// deliver parses and hands lines to the sink; it returns the lag of the newest line.
func (t *Tailer) deliver(ctx context.Context, id string, b []byte) time.Duration {
	if len(b) == 0 {
		return 0
	}
	lines, bad := ParseAccessLines(b)
	if bad > 0 {
		t.log.Warn("access log: lines that are not JSON", "connection", id, "count", bad)
	}
	if len(lines) == 0 {
		return 0
	}
	if t.sink != nil {
		t.sink(ctx, id, lines)
	}
	return max(0, t.m.now().Sub(lines[len(lines)-1].Start))
}

// wholeLines splits at the last newline: an unfinished last line is read again next time.
func wholeLines(b []byte) (done, rest []byte) {
	i := bytes.LastIndexByte(b, '\n')
	if i < 0 {
		return nil, b
	}
	return b[:i+1], b[i+1:]
}

func statFile(ctx context.Context, h Host, p string) (ino, size int64, err error) {
	r, err := h.Run(ctx, "stat -c '%i %s' "+p+" 2>/dev/null || echo 0 0", nil)
	if err != nil {
		return 0, 0, err
	}
	f := strings.Fields(string(r.Stdout))
	if len(f) != 2 {
		return 0, 0, fmt.Errorf("stat %s: %q", p, r.Stdout)
	}
	ino, err1 := strconv.ParseInt(f[0], 10, 64)
	size, err2 := strconv.ParseInt(f[1], 10, 64)
	if err1 != nil || err2 != nil {
		return 0, 0, fmt.Errorf("stat %s: %q", p, r.Stdout)
	}
	return ino, size, nil
}

// findInode names the log file (name or a rotated copy of it) that has inode ino, uncompressed; "" when there is none.
func findInode(ctx context.Context, h Host, dir, name string, ino int64) (string, error) {
	r, err := h.Run(ctx, fmt.Sprintf("find %s -maxdepth 1 -inum %d -name '%s*' ! -name '*.gz' | head -n 1", dir, ino, name), nil)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(r.Stdout)), nil
}

func readFrom(ctx context.Context, h Host, p string, off int64) ([]byte, error) {
	r, err := h.Run(ctx, fmt.Sprintf("tail -c +%d %s | head -c %d", off+1, p, TailRead), nil)
	if err != nil {
		return nil, err
	}
	if r.ExitCode != 0 {
		return nil, fmt.Errorf("read %s: %s", p, bytes.TrimSpace(r.Stderr))
	}
	return r.Stdout, nil
}
