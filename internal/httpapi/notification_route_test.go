package httpapi

import (
	"context"
	"net/http"
	"testing"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
)

// The alert route keeps its grouping when a save leaves it out, and refuses a wait outside 0–120 seconds (bug 144).
func TestNotificationRouteGrouping(t *testing.T) {
	ctx := context.Background()
	s := &Server{Store: storetest.New(t), Audit: audit.New([]byte("k"))}
	group := func() int64 {
		t.Helper()
		r, err := s.GetNotificationRoute(ctx, gen.GetNotificationRouteRequestObject{})
		if err != nil {
			t.Fatal(err)
		}
		return *r.(gen.GetNotificationRoute200JSONResponse).GroupSeconds
	}
	if g := group(); g != 15 {
		t.Fatalf("default %d, want 15", g)
	}
	put := func(g *int64) error {
		_, err := s.PutNotificationRoute(ctx, gen.PutNotificationRouteRequestObject{Body: &gen.NotificationRoute{
			MinSeverity: gen.NotificationRouteMinSeverityDown, CooldownSeconds: 600, GroupSeconds: g}})
		return err
	}
	if err := put(ptr(int64(30))); err != nil {
		t.Fatal(err)
	}
	if err := put(nil); err != nil || group() != 30 {
		t.Fatalf("a save without grouping must keep it: %v, %d", err, group())
	}
	if err := put(ptr(int64(121))); status(err) != http.StatusBadRequest && status(err) != http.StatusUnprocessableEntity {
		t.Fatalf("121 s: %v", err)
	}
}
