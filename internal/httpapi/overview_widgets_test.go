package httpapi

import (
	"context"
	"testing"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
)

// Bug 31: the overview's widget order is stored with Appearance; a save that leaves the list out (the theme cards,
// Public pages) keeps it, and an unknown widget is refused.
func TestOverviewWidgetsSaveAndSurvive(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	s := &Server{Store: st, Audit: audit.New([]byte("k")), Settings: settings.New(st)}
	order := []gen.OverviewWidget{{Id: gen.OverviewWidgetIdHealth, Visible: true}, {Id: gen.OverviewWidgetIdChanges, Visible: false}}
	put := func(b gen.AppearanceSettings) error {
		_, err := s.PutAppearanceSettings(ctx, gen.PutAppearanceSettingsRequestObject{Body: &b})
		return err
	}
	if err := put(gen.AppearanceSettings{Theme: "graphite", OverviewWidgets: &order}); err != nil {
		t.Fatal(err)
	}
	if err := put(gen.AppearanceSettings{Theme: "verdant"}); err != nil {
		t.Fatal(err)
	}
	res, err := s.GetAppearanceSettings(ctx, gen.GetAppearanceSettingsRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	got := gen.AppearanceSettings(res.(gen.GetAppearanceSettings200JSONResponse))
	if got.Theme != "verdant" || got.OverviewWidgets == nil || len(*got.OverviewWidgets) != 2 ||
		(*got.OverviewWidgets)[0].Id != gen.OverviewWidgetIdHealth || (*got.OverviewWidgets)[1].Visible {
		t.Fatalf("widgets lost or changed: %+v", got.OverviewWidgets)
	}
	bad := []gen.OverviewWidget{{Id: "weather", Visible: true}}
	if err := put(gen.AppearanceSettings{Theme: "graphite", OverviewWidgets: &bad}); err == nil {
		t.Fatal("an unknown widget must be refused")
	}
}
