package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/auth"
	"git.juzo.io/juzo/hashi/internal/feeds"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/ipsets"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
)

func TestFeedsAPI(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	al := audit.New([]byte("k"))
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	feedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "; Spamhaus-like\n45.0.0.0/24 ; SBL1\n10.0.0.0/8 ; private\n")
	}))
	defer feedSrv.Close()
	sets := ipsets.NewService(st, al, log, nil)
	f := &feeds.Fetcher{UserAgent: "t", MaxBytes: 1 << 20, Timeout: 5 * time.Second, AllowInternal: true}
	s := &Server{Store: st, Audit: al, Sessions: auth.NewManager(st, al, []byte("csrf")), IPSets: sets,
		Feeds: feeds.NewService(st, al, sets, f, log, time.Now), Log: slog.Default()}
	src := gen.FeedSource{Url: feedSrv.URL + "/drop.txt", Format: gen.Lines, AllowHttp: ptr(true)}

	pv, err := s.PreviewFeed(ctx, gen.PreviewFeedRequestObject{Body: &src})
	if err != nil {
		t.Fatal(err)
	}
	if p := pv.(gen.PreviewFeed200JSONResponse); p.Count != 1 || p.Rejected != 1 || p.Rejects[0].Reason != "private range" {
		t.Fatalf("preview %+v", p)
	}
	// (Internal addresses are refused by the production fetcher: internal/feeds/fetch_test.go. This one allows loopback.)
	if _, err := s.PreviewFeed(ctx, gen.PreviewFeedRequestObject{Body: &gen.FeedSource{Url: "ftp://example.com/x", Format: gen.Lines}}); status(err) != 422 {
		t.Errorf("non-HTTPS URL: %v", err)
	}

	res, err := s.CreateFeed(ctx, gen.CreateFeedRequestObject{Body: &gen.FeedInput{Url: src.Url, Format: src.Format, AllowHttp: src.AllowHttp,
		Name: "Spamhaus DROP", Slug: "feed-spamhaus-drop", CatalogKey: ptr("spamhaus-drop")}})
	if err != nil {
		t.Fatal(err)
	}
	created := res.(gen.CreateFeed200JSONResponse)
	if created.Run.Outcome != gen.FeedRunOutcomeUpdated || created.Run.Added != 1 {
		t.Fatalf("create %+v", created)
	}
	rec, _ := s.ListRecommendedFeeds(ctx, gen.ListRecommendedFeedsRequestObject{})
	found := false
	for _, r := range rec.(gen.ListRecommendedFeeds200JSONResponse) {
		if r.Key == "spamhaus-drop" {
			found = r.SetId != nil && *r.SetId == created.SetId
		}
	}
	if !found {
		t.Error("the recommended entry does not point at its set")
	}
	if r, _ := s.RefreshFeed(ctx, gen.RefreshFeedRequestObject{Id: created.SetId}); r.(gen.RefreshFeed200JSONResponse).Outcome != gen.FeedRunOutcomeUnchanged {
		t.Errorf("second refresh %+v", r)
	}
	g, err := s.GetFeed(ctx, gen.GetFeedRequestObject{Id: created.SetId})
	if err != nil {
		t.Fatal(err)
	}
	if fd := g.(gen.GetFeed200JSONResponse); fd.EntryCount != 1 || fd.RejectedCount != 1 || len(fd.Runs) != 2 || fd.LastSuccessAt == nil {
		t.Errorf("feed %+v", fd)
	}
	// A feed set is owned by its fetcher: its elements cannot be edited by hand (05a §3).
	if _, err := s.GetFeed(ctx, gen.GetFeedRequestObject{Id: "nope"}); status(err) != 404 {
		t.Errorf("not a feed: %v", err)
	}
}
