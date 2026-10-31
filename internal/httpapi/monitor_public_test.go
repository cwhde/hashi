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
		{gen.PublicStatus{}, []string{"endpoints", "theme", "title", "updated_at"}},
		{gen.PublicEndpoint{}, []string{"group", "name", "pending_dns", "state", "strip", "uptime_24h"}},
		{gen.MonitorStripSample{}, []string{"at", "verdict"}},
		{gen.PublicApps{}, []string{"apps", "firewall_online", "firewall_total", "hosts_online", "hosts_total", "theme", "title", "updated_at"}},
		{gen.PublicApp{}, []string{"group", "name", "state", "url"}},
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
