package feeds

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"regexp"
	"strconv"
	"time"

	"github.com/oklog/ulid/v2"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/ipsets"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// Entry is a recommended feed (06 §8): all disabled until the admin adds one.
type Entry struct {
	Key         string  `json:"key"`
	Name        string  `json:"name"`
	URL         string  `json:"url"`
	Format      string  `json:"format"`
	Options     Options `json:"options"`
	Description string  `json:"description"`
	Risk        string  `json:"risk"` // false-positive risk: low, medium, high
	Licence     string  `json:"licence"`
}

// Recommended are the feeds Hashi offers, with the formats captured in testdata/contracts/feeds/.
var Recommended = []Entry{
	{Key: "feodo", Name: "Feodo Tracker botnet C2", URL: "https://feodotracker.abuse.ch/downloads/ipblocklist_recommended.txt",
		Format: FormatLines, Description: "Command-and-control servers of active banking-trojan botnets (abuse.ch).",
		Risk: "low", Licence: "abuse.ch terms: free for commercial and non-commercial use; you are responsible for its effects."},
	{Key: "spamhaus-drop", Name: "Spamhaus DROP", URL: "https://www.spamhaus.org/drop/drop.txt", Format: FormatLines,
		Description: "Address ranges hijacked or leased by professional spam and cyber-crime operations.",
		Risk:        "low", Licence: "Spamhaus DROP terms: free use; you are responsible for its effects."},
	{Key: "spamhaus-dropv6", Name: "Spamhaus DROPv6", URL: "https://www.spamhaus.org/drop/dropv6.txt", Format: FormatLines,
		Description: "The IPv6 counterpart of Spamhaus DROP.", Risk: "low",
		Licence: "Spamhaus DROP terms: free use; you are responsible for its effects."},
	{Key: "dshield", Name: "DShield recommended block list", URL: "https://feeds.dshield.org/block.txt", Format: FormatCSV,
		Options:     Options{Separator: "tab", Column: 1, PrefixColumn: 3},
		Description: "The twenty /24 networks that attacked the most in the last three days (SANS ISC).",
		Risk:        "medium", Licence: "SANS ISC: free use with attribution; you are responsible for its effects."},
	{Key: "firehol-level1", Name: "FireHOL Level 1", URL: "https://iplists.firehol.org/files/firehol_level1.netset",
		Format:      FormatLines,
		Description: "A combination of several lists for a basic block. It lists private ranges too; Hashi never takes those.",
		Risk:        "medium", Licence: "Combines lists under their own terms; you are responsible for its effects."},
}

// Service owns feed sets.
type Service struct {
	st    *store.Store
	al    *audit.Log
	sets  *ipsets.Service
	fetch *Fetcher
	log   *slog.Logger
	now   func() time.Time
}

// NewService returns the feed service.
func NewService(st *store.Store, al *audit.Log, sets *ipsets.Service, f *Fetcher, log *slog.Logger, now func() time.Time) *Service {
	return &Service{st: st, al: al, sets: sets, fetch: f, log: log, now: now}
}

// Errors the API maps.
var (
	ErrNotFound = errors.New("feed not found")
	ErrInvalid  = errors.New("invalid feed")
)

// Settings are a feed's source.
type Settings struct {
	URL       string
	Format    string
	Options   Options
	AllowHTTP bool
}

func (f *Fetcher) check(s Settings) error {
	if err := Validate(s.Format, s.Options); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if err := f.CheckURL(s.URL, s.AllowHTTP); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	return nil
}

// Preview is what a download would give, before anything is stored (06 §8 "preview before adding").
type Preview struct {
	Count    int      `json:"count"`
	Sample   []string `json:"sample"`
	Rejected int      `json:"rejected"`
	Rejects  []Reject `json:"rejects"`
	Ignored  int      `json:"ignored"`
	Hash     string   `json:"hash"`
}

func (s *Service) download(ctx context.Context, set Settings, etag, lastModified string) (Download, Parsed, error) {
	d, err := s.fetch.Get(ctx, set.URL, set.AllowHTTP, etag, lastModified)
	if err != nil || d.NotModified {
		return d, Parsed{}, err
	}
	p, err := Parse(set.Format, set.Options, d.Body)
	if err != nil {
		return d, Parsed{}, err
	}
	return d, Filter(p), nil
}

// Preview downloads and parses without storing.
func (s *Service) Preview(ctx context.Context, set Settings) (Preview, error) {
	if err := s.fetch.check(set); err != nil {
		return Preview{}, err
	}
	d, p, err := s.download(ctx, set, "", "")
	if err != nil {
		return Preview{}, err
	}
	out := Preview{Count: len(p.Prefixes), Rejected: p.Rejected, Rejects: p.Rejects, Ignored: p.Ignored, Hash: d.Hash,
		Sample: []string{}}
	for _, x := range p.Prefixes[:min(20, len(p.Prefixes))] {
		out.Sample = append(out.Sample, x.String())
	}
	if out.Rejects == nil {
		out.Rejects = []Reject{}
	}
	return out, nil
}

var slugRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// Create adds a feed set (kind feed, empty until its first refresh). catalogKey names a recommended feed, if any.
func (s *Service) Create(ctx context.Context, name, slug, description, catalogKey string, set Settings, actor, ip string) (string, error) {
	if err := s.fetch.check(set); err != nil {
		return "", err
	}
	if name == "" || !slugRe.MatchString(slug) {
		return "", fmt.Errorf("%w: a feed needs a name and a slug of lowercase letters, digits and dashes", ErrInvalid)
	}
	opts, err := json.Marshal(set.Options)
	if err != nil {
		return "", err
	}
	id, ms := ulid.Make().String(), s.now().UnixMilli()
	err = s.st.Tx(ctx, func(q *db.Queries) error {
		if err := ipsets.SlugFree(ctx, q, slug); errors.Is(err, ipsets.ErrTaken) {
			return fmt.Errorf("%w: another IP set already uses the slug %s", ErrInvalid, slug)
		} else if err != nil {
			return err
		}
		if err := q.InsertIPSet(ctx, db.InsertIPSetParams{ID: id, Slug: slug, Name: name, Description: description,
			Kind: ipsets.KindFeed, Origin: "user", CreatedAt: ms, UpdatedAt: ms}); err != nil {
			return err
		}
		if err := q.InsertFeed(ctx, db.InsertFeedParams{IpSetID: id, Url: set.URL, Format: set.Format, OptionsJson: string(opts),
			AllowHttp: b2i(set.AllowHTTP), CatalogKey: catalogKey, CreatedAt: ms, UpdatedAt: ms}); err != nil {
			return err
		}
		return s.al.Record(ctx, q, audit.Event{Actor: actor, Action: "feed.added", IP: ip, Target: "ip_set:" + id,
			Outcome: audit.Success, Detail: map[string]any{"name": name, "url": set.URL, "format": set.Format}})
	})
	return id, err
}

func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// Run is one refresh's record.
type Run struct {
	Outcome                                     string
	Error                                       string
	Parsed, Added, Removed, Unchanged, Rejected int
	Hash                                        string
}

// Outcomes of a refresh.
const (
	OutcomeUpdated   = "updated"
	OutcomeUnchanged = "unchanged"
	OutcomeFailed    = "failed"
	OutcomeStale     = "stale"
)

// SettingsOf reads a stored feed's source.
func SettingsOf(f db.IpSetFeed) (Settings, error) {
	var o Options
	if err := json.Unmarshal([]byte(f.OptionsJson), &o); err != nil {
		return Settings{}, err
	}
	return Settings{URL: f.Url, Format: f.Format, Options: o, AllowHTTP: f.AllowHttp == 1}, nil
}

// Refresh downloads one feed. A failure keeps the entries and records why; a whole good download replaces them in one
// transaction (06 §8: never partially apply a failed update).
func (s *Service) Refresh(ctx context.Context, setID string) (Run, error) {
	f, err := s.st.R.GetFeed(ctx, setID)
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, ErrNotFound
	}
	if err != nil {
		return Run{}, err
	}
	set, err := SettingsOf(f)
	if err != nil {
		return Run{}, err
	}
	started := s.now()
	etag, lm := f.Etag, f.LastModified
	if f.EntryCount == 0 && !f.LastSuccessAt.Valid {
		etag, lm = "", ""
	}
	d, p, derr := s.download(ctx, set, etag, lm)
	if derr != nil {
		run := Run{Outcome: OutcomeFailed, Error: derr.Error()}
		return run, s.st.Tx(ctx, func(q *db.Queries) error {
			if err := q.SetFeedFailure(ctx, db.SetFeedFailureParams{LastAttemptAt: nullMs(started), LastError: run.Error,
				UpdatedAt: started.UnixMilli(), IpSetID: setID}); err != nil {
				return err
			}
			return s.record(ctx, q, setID, started, run)
		})
	}
	if d.NotModified {
		run := Run{Outcome: OutcomeUnchanged, Parsed: int(f.EntryCount), Unchanged: int(f.EntryCount), Hash: f.ContentHash}
		err := s.st.Tx(ctx, func(q *db.Queries) error {
			if err := q.SetFeedResult(ctx, db.SetFeedResultParams{Etag: f.Etag, LastModified: f.LastModified, ContentHash: f.ContentHash,
				EntryCount: f.EntryCount, RejectedCount: f.RejectedCount, LastAttemptAt: nullMs(started), LastSuccessAt: nullMs(started),
				UpdatedAt: started.UnixMilli(), IpSetID: setID}); err != nil {
				return err
			}
			return s.record(ctx, q, setID, started, run)
		})
		if err == nil && f.LastError != "" {
			s.sets.Changed(ctx) // the problem is gone
		}
		return run, err
	}
	run := Run{Outcome: OutcomeUpdated, Parsed: len(p.Prefixes), Rejected: p.Rejected, Hash: d.Hash}
	err = s.st.Tx(ctx, func(q *db.Queries) error {
		cur, err := q.ListIPSetElements(ctx, setID)
		if err != nil {
			return err
		}
		have := map[string]bool{}
		for _, e := range cur {
			if e.Origin == "feed" {
				have[elementKey(e)] = true
			}
		}
		for _, x := range p.Prefixes {
			if have[x.String()] {
				run.Unchanged++
				delete(have, x.String())
			} else {
				run.Added++
			}
		}
		run.Removed = len(have)
		if run.Added == 0 && run.Removed == 0 {
			run.Outcome = OutcomeUnchanged
		} else {
			if err := q.DeleteFeedElements(ctx, setID); err != nil {
				return err
			}
			ms := started.UnixMilli()
			for i, x := range p.Prefixes {
				if err := q.InsertIPSetElement(ctx, feedElement(setID, i, x, ms)); err != nil {
					return fmt.Errorf("store %s: %w", x, err)
				}
			}
		}
		if err := q.SetFeedResult(ctx, db.SetFeedResultParams{Etag: d.ETag, LastModified: d.LastModified, ContentHash: d.Hash,
			EntryCount: int64(len(p.Prefixes)), RejectedCount: int64(p.Rejected), LastAttemptAt: nullMs(started),
			LastSuccessAt: nullMs(started), UpdatedAt: started.UnixMilli(), IpSetID: setID}); err != nil {
			return err
		}
		return s.record(ctx, q, setID, started, run)
	})
	if err != nil {
		return run, err
	}
	if run.Outcome == OutcomeUpdated || f.LastError != "" {
		s.sets.Changed(ctx)
	}
	return run, nil
}

func elementKey(e db.IpSetElement) string {
	if e.Type == ipsets.TypeIP && e.Address.Valid {
		a, err := netip.ParseAddr(e.Address.String)
		if err == nil {
			return netip.PrefixFrom(a, a.BitLen()).String()
		}
	}
	return e.Prefix.String
}

func feedElement(setID string, pos int, p netip.Prefix, ms int64) db.InsertIPSetElementParams {
	fam := int64(4)
	if p.Addr().Is6() {
		fam = 6
	}
	e := db.InsertIPSetElementParams{ID: ulid.Make().String(), IpSetID: setID, Position: int64(pos), Origin: "feed", Locked: 1,
		Family: sql.NullInt64{Int64: fam, Valid: true}, CreatedBy: "feed", CreatedAt: ms, UpdatedAt: ms}
	if p.IsSingleIP() {
		e.Type, e.Address = ipsets.TypeIP, sql.NullString{String: p.Addr().String(), Valid: true}
	} else {
		e.Type, e.Prefix = ipsets.TypeCIDR, sql.NullString{String: p.String(), Valid: true}
	}
	return e
}

func nullMs(t time.Time) sql.NullInt64 { return sql.NullInt64{Int64: t.UnixMilli(), Valid: true} }

func (s *Service) record(ctx context.Context, q *db.Queries, setID string, started time.Time, r Run) error {
	if err := q.InsertFeedRun(ctx, db.InsertFeedRunParams{ID: ulid.Make().String(), IpSetID: setID, StartedAt: started.UnixMilli(),
		FinishedAt: s.now().UnixMilli(), Outcome: r.Outcome, Error: r.Error, Parsed: int64(r.Parsed), Added: int64(r.Added),
		Removed: int64(r.Removed), Unchanged: int64(r.Unchanged), Rejected: int64(r.Rejected), ContentHash: r.Hash}); err != nil {
		return err
	}
	return q.PruneFeedRuns(ctx, setID)
}

// Expire empties a feed whose last success is older than staleAfter (06 §8: it then resolves empty, Overview shows it).
func (s *Service) expire(ctx context.Context, f db.IpSetFeed, staleAfter time.Duration) (bool, error) {
	now := s.now()
	last := f.CreatedAt
	if f.LastSuccessAt.Valid {
		last = f.LastSuccessAt.Int64
	}
	if f.EntryCount == 0 || now.Sub(time.UnixMilli(last)) < staleAfter {
		return false, nil
	}
	msg := "no successful refresh since " + time.UnixMilli(last).UTC().Format(time.RFC3339) + ": entries dropped"
	return true, s.st.Tx(ctx, func(q *db.Queries) error {
		if err := q.DeleteFeedElements(ctx, f.IpSetID); err != nil {
			return err
		}
		if err := q.SetFeedResult(ctx, db.SetFeedResultParams{Etag: "", LastModified: "", ContentHash: "", EntryCount: 0,
			RejectedCount: 0, LastAttemptAt: f.LastAttemptAt, LastSuccessAt: f.LastSuccessAt, LastError: msg,
			UpdatedAt: now.UnixMilli(), IpSetID: f.IpSetID}); err != nil {
			return err
		}
		return s.record(ctx, q, f.IpSetID, now, Run{Outcome: OutcomeStale, Error: msg})
	})
}

// JobKind refreshes the feeds that are due and expires stale ones (06 §15), every minute.
const (
	JobKind     = "feeds.refresh"
	JobInterval = time.Minute
)

// RunDue is the job: refreshEvery and staleAfter come from the engine settings.
func (s *Service) RunDue(ctx context.Context, refreshEvery, staleAfter time.Duration) (string, error) {
	feeds, err := s.st.R.ListFeeds(ctx)
	if err != nil {
		return "", err
	}
	now := s.now()
	refreshed, failed, expired := 0, 0, 0
	for _, f := range feeds {
		if !f.LastAttemptAt.Valid || now.Sub(time.UnixMilli(f.LastAttemptAt.Int64)) >= refreshEvery {
			run, err := s.Refresh(ctx, f.IpSetID)
			if err != nil {
				return "", err
			}
			refreshed++
			if run.Outcome == OutcomeFailed {
				failed++
				s.log.Warn("feed refresh failed", "set", f.IpSetID, "err", run.Error)
			}
			if f, err = s.st.R.GetFeed(ctx, f.IpSetID); err != nil {
				return "", err
			}
		}
		gone, err := s.expire(ctx, f, staleAfter)
		if err != nil {
			return "", err
		}
		if gone {
			expired++
			s.sets.Changed(ctx)
		}
	}
	if refreshed == 0 && expired == 0 {
		return "", nil
	}
	return strconv.Itoa(refreshed) + " refreshed, " + strconv.Itoa(failed) + " failed, " + strconv.Itoa(expired) + " expired", nil
}
