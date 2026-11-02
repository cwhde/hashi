package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"git.juzo.io/juzo/hashi/internal/feeds"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

func feedOptions(o *gen.FeedOptions) feeds.Options {
	if o == nil {
		return feeds.Options{}
	}
	out := feeds.Options{Column: deref(o.Column), PrefixColumn: deref(o.PrefixColumn), Field: deref(o.Field)}
	if o.Separator != nil {
		out.Separator = string(*o.Separator)
	}
	return out
}

func feedOptionsBody(o feeds.Options) *gen.FeedOptions {
	out := &gen.FeedOptions{}
	if o.Column > 0 {
		out.Column = ptr(o.Column)
	}
	if o.PrefixColumn > 0 {
		out.PrefixColumn = ptr(o.PrefixColumn)
	}
	if o.Field != "" {
		out.Field = ptr(o.Field)
	}
	if o.Separator != "" {
		out.Separator = ptr(gen.FeedOptionsSeparator(o.Separator))
	}
	return out
}

func feedError(err error) error {
	switch {
	case errors.Is(err, feeds.ErrInvalid):
		return invalid(strings.TrimPrefix(err.Error(), feeds.ErrInvalid.Error()+": "))
	case errors.Is(err, feeds.ErrNotFound):
		return notFound("No such feed.")
	case errors.Is(err, feeds.ErrDenied), errors.Is(err, feeds.ErrFormat):
		return invalid(err.Error())
	}
	return err
}

func runBody(at time.Time, r feeds.Run) gen.FeedRun {
	return gen.FeedRun{StartedAt: at, Outcome: gen.FeedRunOutcome(r.Outcome), Error: r.Error, Parsed: r.Parsed, Added: r.Added,
		Removed: r.Removed, Unchanged: r.Unchanged, Rejected: r.Rejected}
}

func (s *Server) ListRecommendedFeeds(ctx context.Context, _ gen.ListRecommendedFeedsRequestObject) (gen.ListRecommendedFeedsResponseObject, error) {
	rows, err := s.Store.R.ListFeeds(ctx)
	if err != nil {
		return nil, err
	}
	added := map[string]string{}
	for _, r := range rows {
		if r.CatalogKey != "" {
			added[r.CatalogKey] = r.IpSetID
		}
	}
	out := gen.ListRecommendedFeeds200JSONResponse{}
	for _, e := range feeds.Recommended {
		out = append(out, gen.RecommendedFeed{Key: e.Key, Name: e.Name, Url: e.URL, Format: gen.FeedFormat(e.Format),
			Options: *feedOptionsBody(e.Options), Description: e.Description, Risk: gen.RecommendedFeedRisk(e.Risk),
			Licence: e.Licence, SetId: strPtr(added[e.Key])})
	}
	return out, nil
}

func (s *Server) PreviewFeed(ctx context.Context, req gen.PreviewFeedRequestObject) (gen.PreviewFeedResponseObject, error) {
	b := req.Body
	pv, err := s.Feeds.Preview(ctx, feeds.Settings{URL: b.Url, Format: string(b.Format), Options: feedOptions(b.Options),
		AllowHTTP: deref(b.AllowHttp)})
	if err != nil {
		return nil, feedError(err)
	}
	out := gen.PreviewFeed200JSONResponse{Count: pv.Count, Sample: pv.Sample, Rejected: pv.Rejected, Ignored: pv.Ignored, Hash: pv.Hash,
		Rejects: make([]gen.FeedReject, 0, len(pv.Rejects))}
	for _, r := range pv.Rejects {
		out.Rejects = append(out.Rejects, gen.FeedReject{Line: r.Line, Text: r.Text, Reason: r.Reason})
	}
	return out, nil
}

func (s *Server) CreateFeed(ctx context.Context, req gen.CreateFeedRequestObject) (gen.CreateFeedResponseObject, error) {
	ri := info(ctx)
	b := req.Body
	id, err := s.Feeds.Create(ctx, b.Name, b.Slug, deref(b.Description), deref(b.CatalogKey), feeds.Settings{URL: b.Url,
		Format: string(b.Format), Options: feedOptions(b.Options), AllowHTTP: deref(b.AllowHttp)}, s.actor(ri), ri.ipString())
	if err != nil {
		return nil, feedError(err)
	}
	started := s.now()
	run, err := s.Feeds.Refresh(ctx, id)
	if err != nil {
		return nil, feedError(err)
	}
	s.IPSets.Changed(ctx)
	return gen.CreateFeed200JSONResponse{SetId: id, Run: runBody(started, run)}, nil
}

func (s *Server) GetFeed(ctx context.Context, req gen.GetFeedRequestObject) (gen.GetFeedResponseObject, error) {
	f, err := s.Store.R.GetFeed(ctx, req.Id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, notFound("This set is not a feed.")
	}
	if err != nil {
		return nil, err
	}
	runs, err := s.Store.R.ListFeedRuns(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	src, err := feedSettings(f)
	if err != nil {
		return nil, err
	}
	out := gen.GetFeed200JSONResponse{Url: f.Url, Format: gen.FeedFormat(f.Format), Options: feedOptionsBody(src.Options),
		AllowHttp: ptr(f.AllowHttp == 1), CatalogKey: f.CatalogKey, EntryCount: int(f.EntryCount), RejectedCount: int(f.RejectedCount),
		LastAttemptAt: msTime(f.LastAttemptAt), LastSuccessAt: msTime(f.LastSuccessAt), LastError: f.LastError,
		Runs: make([]gen.FeedRun, 0, len(runs))}
	for _, r := range runs {
		out.Runs = append(out.Runs, runBody(time.UnixMilli(r.StartedAt).UTC(), feeds.Run{Outcome: r.Outcome, Error: r.Error,
			Parsed: int(r.Parsed), Added: int(r.Added), Removed: int(r.Removed), Unchanged: int(r.Unchanged), Rejected: int(r.Rejected)}))
	}
	return out, nil
}

func feedSettings(f db.IpSetFeed) (feeds.Settings, error) { return feeds.SettingsOf(f) }

func (s *Server) RefreshFeed(ctx context.Context, req gen.RefreshFeedRequestObject) (gen.RefreshFeedResponseObject, error) {
	started := s.now()
	run, err := s.Feeds.Refresh(ctx, req.Id)
	if err != nil {
		return nil, feedError(err)
	}
	return gen.RefreshFeed200JSONResponse(runBody(started, run)), nil
}
