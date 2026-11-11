package settings

import (
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// consumerTests names, for every setting, the test that proves it changes behaviour (09 §2: "every setting has a
// runtime consumer and a test proving it changes behaviour"). Go tests are "path::TestName"; settings whose consumer is
// the admin UI point at the e2e spec and a line the step contains ("path::text"). A new setting fails
// TestEverySettingHasAConsumerTest until it is listed here with its test.
var consumerTests = map[string]string{
	"general.root_domain":           "internal/app/setup_https_test.go::TestSetupStep8HTTPSProof",
	"general.admin_domain":          "internal/httpapi/settings_system_test.go::TestAdminDomainChangeIsAllOrNothing",
	"general.internal_url":          "internal/app/setup_https_test.go::TestSetupStep8HTTPSProof",
	"general.time_display":          "web/e2e/setup-and-login.spec.ts::Times in UTC: Activity shows them with the zone",
	"general.sync_interval_seconds": "internal/app/settings_consumers_test.go::TestSyncIntervalSetting",
	"general.trusted_proxies":       "internal/app/auth_integration_test.go::TestSessionBoundToClientIPThroughTrustedProxy",

	"security.admin_idle_timeout_minutes": "internal/auth/session_test.go::TestIdleNeverExceedsConfiguredValueBetweenPersistedTouches",
	"security.bootstrap_allowed_cidrs":    "internal/app/auth_integration_test.go::TestBootstrapNetworksSetting",

	"appearance.theme":                    "internal/notify/notify_test.go::TestMailCustomTheme",
	"appearance.custom_theme":             "web/e2e/setup-and-login.spec.ts::A custom theme: unreadable text cannot be saved",
	"appearance.reduce_transparency":      "web/e2e/setup-and-login.spec.ts::Reduce transparency is applied at once and saved",
	"appearance.public_dashboard_enabled": "internal/app/public_test.go::TestPublicPageSettings",
	"appearance.public_status_enabled":    "internal/app/public_test.go::TestPublicPageSettings",

	"monitoring.interval_seconds": "internal/monitor/service_test.go::TestMonitoringSettingsApply",
	"monitoring.timeout_seconds":  "internal/monitor/service_test.go::TestMonitoringSettingsApply",
	"monitoring.code_min":         "internal/monitor/service_test.go::TestMonitoringSettingsApply",
	"monitoring.code_max":         "internal/monitor/service_test.go::TestMonitoringSettingsApply",
	"monitoring.degraded_ms":      "internal/monitor/service_test.go::TestMonitoringSettingsApply",
	"monitoring.retention_days":   "internal/monitor/service_test.go::TestMonitoringSettingsApply",

	"dashboard.default_sort":         "web/e2e/setup-and-login.spec.ts::the public page starts sorted by name",
	"dashboard.new_services_visible": "web/e2e/setup-and-login.spec.ts::new services start hidden from it",

	"firewall_defaults.netbird_mode": "web/e2e/setup-and-login.spec.ts::name: 'Peer only'",
	"firewall_defaults.mss_clamp":    "internal/firewall/render_test.go::TestRenderMSSClampSetting",

	"internal_agent_dns.enabled":                  "internal/pulse/pulse_test.go::TestAgentDNS",
	"internal_agent_dns.domain":                   "internal/pulse/pulse_test.go::TestAgentDNS",
	"internal_agent_dns.ip_mode":                  "internal/pulse/pulse_test.go::TestAgentNames",
	"internal_agent_dns.remove_stale_after_hours": "internal/pulse/pulse_test.go::TestAgentDNS",

	"pulse.heartbeat_interval_seconds": "internal/pulse/pulse_test.go::TestPulseSettingsAreUsed",
	"pulse.stale_factor":               "internal/pulse/pulse_test.go::TestPulseSettingsAreUsed",
	"pulse.auto_update":                "internal/pulse/pulse_test.go::TestReplyNamesUpdate",

	"edge_sso.max_session_hours":  "internal/edge/sso_test.go::TestSSOSessionSettings",
	"edge_sso.idle_timeout_hours": "internal/edge/sso_test.go::TestSSOSessionSettings",
	"edge_sso.remember_device":    "internal/httpapi/edge_cookie_test.go::TestEdgeSessionCookieRememberDevice",

	"geoip.account_id": "internal/geoip/source_test.go::TestGeoIPAccountSetting",
	"geoip.url":        "internal/geoip/source_test.go::TestGeoIPAccountSetting",

	"captcha.enabled":          "internal/httpapi/challenge_test.go::TestChallengeFlow",
	"captcha.challenge_domain": "internal/httpapi/challenge_test.go::TestChallengeFlow",
	"captcha.cap_address":      "internal/httpapi/captcha_handlers_test.go::TestCreateCaptchaKey",
	"captcha.cap_port":         "internal/httpapi/captcha_handlers_test.go::TestCreateCaptchaKey",
	"captcha.verify_url":       "internal/httpapi/challenge_test.go::TestChallengeFlow",
	"captcha.site_key":         "internal/httpapi/challenge_test.go::TestChallengeFlow",
	"captcha.timeout_seconds":  "internal/httpapi/challenge_test.go::TestChallengeFlow",
	"captcha.admin_service_id": "internal/httpapi/challenge_test.go::TestChallengeFlow",

	"engine.waf_default_mode":         "internal/httpapi/edge_cookie_test.go::TestWAFDefaultModeSetting",
	"engine.waf_violations":           "internal/security/settings_consumers_test.go::TestSettingsWAFViolationsAndWindow",
	"engine.waf_window_seconds":       "internal/security/settings_consumers_test.go::TestSettingsWAFViolationsAndWindow",
	"engine.rate_requests":            "internal/security/stagea_test.go::TestRateTriggerChallengeAndEscalation",
	"engine.rate_window_seconds":      "internal/security/stagea_test.go::TestRateTriggerChallengeAndEscalation",
	"engine.challenge_expiry_seconds": "internal/security/settings_consumers_test.go::TestSettingsChallengeExpiry",
	"engine.challenge_max_failed":     "internal/security/stagea_test.go::TestRateTriggerChallengeAndEscalation",
	"engine.captcha_clearance_hours":  "internal/httpapi/challenge_test.go::TestChallengeFlow",
	"engine.meters_enabled":           "internal/firewall/render_test.go::TestRenderMeterSettings",
	"engine.tcp_new_per_minute":       "internal/firewall/render_test.go::TestRenderMeterSettings",
	"engine.tcp_burst":                "internal/firewall/render_test.go::TestRenderMeterSettings",
	"engine.tcp_max_concurrent":       "internal/firewall/render_test.go::TestRenderMeterSettings",
	"engine.udp_new_per_minute":       "internal/firewall/render_test.go::TestRenderMeterSettings",
	"engine.udp_burst":                "internal/firewall/render_test.go::TestRenderMeterSettings",
	"engine.udp_per_second":           "internal/firewall/render_test.go::TestRenderMeterSettings",
	"engine.udp_pps_burst":            "internal/firewall/render_test.go::TestRenderMeterSettings",
	"engine.staging_timeout_seconds":  "internal/firewall/render_test.go::TestRenderMeterSettings",
	"engine.poll_interval_seconds":    "internal/firewall/render_test.go::TestHostLinkPollSetting",
	"engine.ban_function":             "internal/security/settings_consumers_test.go::TestSettingsBanCurve",
	"engine.ban_base_seconds":         "internal/security/settings_consumers_test.go::TestSettingsBanCurve",
	"engine.ban_k":                    "internal/security/settings_consumers_test.go::TestSettingsBanCurve",
	"engine.ban_max_seconds":          "internal/security/settings_consumers_test.go::TestSettingsBanCurve",
	"engine.ban_permanent_after":      "internal/security/stagea_test.go::TestPermanentBanNeverDecays",
	"engine.counter_decay_seconds":    "internal/security/counters_test.go::TestEditDuringARunningBanDecaysAfterIt",
	"engine.feed_refresh_seconds":     "internal/app/settings_consumers_test.go::TestFeedTimesSetting",
	"engine.feed_stale_seconds":       "internal/app/settings_consumers_test.go::TestFeedTimesSetting",

	"traefik_middlewares.every_service": "internal/traefik/traffic_test.go::TestEveryServiceSettingChangesRender",
}

// allCategories is every settings category; Load notifies exactly these.
var allCategories = []Category{General{}, Security{}, Appearance{}, Monitoring{}, Dashboard{}, FirewallDefaults{}, AgentDNS{},
	Pulse{}, EdgeSSO{}, GeoIP{}, Captcha{}, Engine{}, TraefikMiddlewares{}}

func TestEverySettingHasAConsumerTest(t *testing.T) {
	want := map[string]bool{}
	for _, c := range allCategories {
		rt := reflect.TypeOf(c)
		for i := range rt.NumField() {
			name, _, _ := strings.Cut(rt.Field(i).Tag.Get("json"), ",")
			key := c.Key() + "." + name
			want[key] = true
			ref, ok := consumerTests[key]
			if !ok {
				t.Errorf("%s has no consumer test: write one and list it in consumerTests", key)
				continue
			}
			file, what, _ := strings.Cut(ref, "::")
			b, err := os.ReadFile("../../" + file)
			if err != nil {
				t.Errorf("%s: %v", key, err)
				continue
			}
			src := string(b)
			if strings.HasSuffix(file, "_test.go") {
				if !regexp.MustCompile(`(?m)^func ` + regexp.QuoteMeta(what) + `\(t \*testing\.T\)`).MatchString(src) {
					t.Errorf("%s: %s has no %s", key, file, what)
				}
				if !strings.Contains(src, rt.Field(i).Name) && !strings.Contains(src, name) {
					t.Errorf("%s: %s never mentions %s", key, file, rt.Field(i).Name)
				}
			} else if !strings.Contains(src, what) {
				t.Errorf("%s: %s has no step %q", key, file, what)
			}
		}
	}
	for k := range consumerTests {
		if !want[k] {
			t.Errorf("consumerTests lists %s, which is no setting", k)
		}
	}
}
