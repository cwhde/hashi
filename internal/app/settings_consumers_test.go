package app

import (
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/settings"
)

// The General settings' sync interval sets how often the DNS zones are reconciled (01 §5.4).
func TestSyncIntervalSetting(t *testing.T) {
	if j := dnsRefreshJob(settings.General{SyncIntervalSeconds: 900}, nil); j.Interval != 15*time.Minute || j.Kind != "dns.refresh" {
		t.Fatalf("%+v", j)
	}
}

// The feed settings set how often feeds refresh and when an unreachable one counts as stale (06 §8).
func TestFeedTimesSetting(t *testing.T) {
	e := settings.DefaultOf[settings.Engine]()
	e.FeedRefreshSeconds, e.FeedStaleSeconds = 1800, 86400
	if r, s := feedTimes(e); r != 30*time.Minute || s != 24*time.Hour {
		t.Fatalf("refresh %v stale %v", r, s)
	}
}
