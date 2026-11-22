package httpapi

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
)

// The public APIs expose exactly these fields (07 §7, §8, §10): a new field must be added here on purpose.
func TestPublicSchemasExposeOnlySelectedFields(t *testing.T) {
	for _, c := range []struct {
		v    any
		want []string
	}{
		{gen.PublicStatus{}, []string{"assets", "custom_theme", "groups", "theme", "title", "updated_at"}},
		{gen.PublicStatusGroup{}, []string{"entries", "name"}},
		{gen.PublicStatusEntry{}, []string{"days", "icon", "icon_light", "minutes", "name", "pending_dns", "since", "slug", "state", "uptime_24h", "uptime_90d"}},
		{gen.PublicMinute{}, []string{"at", "latency_ms", "state"}},
		{gen.PublicDay{}, []string{"date", "spells", "state", "uptime"}},
		{gen.PublicSpell{}, []string{"end", "start", "state"}},
		{gen.PublicService{}, []string{"assets", "avg_ms_24h", "badges", "custom_theme", "downtimes", "entry", "group", "series", "theme", "title", "uptime_30d", "uptime_7d"}},
		{gen.PublicPoint{}, []string{"at", "availability", "avg_ms", "p95_ms"}},
		{gen.PublicApps{}, []string{"assets", "custom_theme", "default_sort", "groups", "theme", "title", "updated_at"}},
		{gen.PublicAppGroup{}, []string{"apps", "name"}},
		{gen.PublicApp{}, []string{"has_check", "icon", "icon_light", "name", "state", "url"}},
		// Hashi's own addresses of the public assets (07 §9), never the source they were copied from.
		{gen.PublicPageAssets{}, []string{"background", "icon", "logo_dark", "logo_light"}},
		{gen.CustomTheme{}, []string{"accent", "border", "canvas", "charts", "muted", "roundness", "surface", "text"}},
	} {
		rt := reflect.TypeOf(c.v)
		var got []string
		for i := range rt.NumField() {
			name, _, _ := strings.Cut(rt.Field(i).Tag.Get("json"), ",")
			got = append(got, name)
		}
		slices.Sort(got)
		if !slices.Equal(got, c.want) {
			t.Errorf("%s fields %v, want %v", rt.Name(), got, c.want)
		}
	}
}
