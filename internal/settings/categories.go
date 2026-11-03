package settings

import (
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"git.juzo.io/juzo/hashi/internal/clientip"
)

// General: base settings from setup step 2 (02-setup-auth.md §1.2) plus trusted proxies (01-platform.md §8).
type General struct {
	RootDomain          string   `json:"root_domain"`
	AdminDomain         string   `json:"admin_domain"`
	InternalURL         string   `json:"internal_url"`
	SyncIntervalSeconds int      `json:"sync_interval_seconds"`
	TrustedProxies      []string `json:"trusted_proxies"`
}

// Security: admin session limits and bootstrap network restriction.
type Security struct {
	AdminIdleTimeoutMinutes int      `json:"admin_idle_timeout_minutes"`
	BootstrapAllowedCIDRs   []string `json:"bootstrap_allowed_cidrs"`
}

// Appearance: theme and public page toggles chosen at setup step 2.
type Appearance struct {
	Theme                  string `json:"theme"`
	PublicDashboardEnabled bool   `json:"public_dashboard_enabled"`
	PublicStatusEnabled    bool   `json:"public_status_enabled"`
}

// AgentDNS: stable internal names for Pulse agents (03 §9). DNS only, never reverse proxy.
type AgentDNS struct {
	Enabled bool   `json:"enabled"`
	Domain  string `json:"domain"`
	IPMode  string `json:"ip_mode"`
	// RemoveStaleAfterHours removes a stale agent's rewrite after this long; 0 keeps it (03 §9).
	RemoveStaleAfterHours int `json:"remove_stale_after_hours"`
}

// Pulse: what agents are told and when they count as stale (09 §2, 08 §2).
type Pulse struct {
	HeartbeatIntervalSeconds int `json:"heartbeat_interval_seconds"`
	StaleFactor              int `json:"stale_factor"`
	// AutoUpdate lets agents installed on a machine update themselves to this Hashi's version (ADR 0003).
	AutoUpdate bool `json:"auto_update"`
}

// EdgeSSO: edge session limits (06 §7).
type EdgeSSO struct {
	MaxSessionHours  int `json:"max_session_hours"`
	IdleTimeoutHours int `json:"idle_timeout_hours"`
	// RememberDevice keeps the session across browser restarts.
	RememberDevice bool `json:"remember_device"`
}

// GeoIP: where the databases come from (06 §12). MaxMind with an account (licence key in the vault), or a download
// address with {edition} in it, such as a mirror of the same files. Empty means no downloads.
type GeoIP struct {
	AccountID string `json:"account_id"`
	URL       string `json:"url"`
}

// Captcha connects Hashi to Cap Standalone (06 §11). The secret key is in the vault, not here.
type Captcha struct {
	Enabled bool `json:"enabled"`
	// ChallengeDomain is the challenge service's name (e.g. cap.hashi.example.com); the browser reaches Cap's public
	// paths and Hashi's challenge page there.
	ChallengeDomain string `json:"challenge_domain"`
	// CapAddress and CapPort are where edges reach Cap's public paths (the compose host port).
	CapAddress string `json:"cap_address"`
	CapPort    int    `json:"cap_port"`
	// VerifyURL is where Hashi checks tokens (over the compose network: http://cap:3000).
	VerifyURL      string `json:"verify_url"`
	SiteKey        string `json:"site_key"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	// AdminServiceID is the Cap dashboard's service, if any: pass_captcha never applies to it (06 §11).
	AdminServiceID string `json:"admin_service_id,omitempty"`
}

// Engine: the abuse engine's numbers and timers (06 §10). Rules decide who; these decide how much and how long.
type Engine struct {
	WAFDefaultMode      string `json:"waf_default_mode"`
	WAFViolations       int    `json:"waf_violations"`
	WAFWindowSeconds    int    `json:"waf_window_seconds"`
	RateRequests        int    `json:"rate_requests"`
	RateWindowSeconds   int    `json:"rate_window_seconds"`
	ChallengeExpirySecs int    `json:"challenge_expiry_seconds"` // 0: a challenge never expires
	ChallengeMaxFailed  int    `json:"challenge_max_failed"`
	ClearanceHours      int    `json:"captcha_clearance_hours"`

	MetersEnabled    bool `json:"meters_enabled"`
	TCPNewPerMinute  int  `json:"tcp_new_per_minute"`
	TCPBurst         int  `json:"tcp_burst"`
	TCPMaxConcurrent int  `json:"tcp_max_concurrent"`
	UDPNewPerMinute  int  `json:"udp_new_per_minute"`
	UDPBurst         int  `json:"udp_burst"`
	UDPPerSecond     int  `json:"udp_per_second"`
	UDPPPSBurst      int  `json:"udp_pps_burst"`
	StagingSeconds   int  `json:"staging_timeout_seconds"`
	PollIntervalSecs int  `json:"poll_interval_seconds"`

	BanFunction       string  `json:"ban_function"`
	BanBaseSeconds    int     `json:"ban_base_seconds"`
	BanK              float64 `json:"ban_k"`
	BanMaxSeconds     int     `json:"ban_max_seconds"`
	BanPermanentAfter int     `json:"ban_permanent_after"` // 0: never permanent
	DecaySeconds      int     `json:"counter_decay_seconds"`

	FeedRefreshSeconds int `json:"feed_refresh_seconds"`
	FeedStaleSeconds   int `json:"feed_stale_seconds"`
}

// WAF modes and ban functions (06 §9, §5.2).
const (
	WAFEnabled     = "enabled"
	WAFHeadersOnly = "headers_only"
	WAFDisabled    = "disabled"

	BanConstant    = "constant"
	BanLinear      = "linear"
	BanExponential = "exponential"
)

// Agent DNS address modes.
const (
	AgentIPSelected = "selected"
	AgentIPPrivate  = "private"
	AgentIPPublic   = "public"
)

// Limits from the spec.
const (
	MaxAdminIdle    = 4 * time.Hour // 02 §3.2
	minSyncInterval = 60
	maxSyncInterval = 24 * 3600
)

// Themes are the built-in themes (10-ui.md §2).
var Themes = []string{"graphite", "linen", "verdant", "violet"}

func defaults(c Category) Category {
	switch c.(type) {
	case General:
		return General{SyncIntervalSeconds: 3600, TrustedProxies: []string{}}
	case Security:
		return Security{
			AdminIdleTimeoutMinutes: int(MaxAdminIdle / time.Minute),
			BootstrapAllowedCIDRs: []string{
				"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "127.0.0.0/8", "::1/128", "fc00::/7",
			},
		}
	case Pulse:
		return Pulse{HeartbeatIntervalSeconds: 60, StaleFactor: 3, AutoUpdate: true}
	case EdgeSSO:
		return EdgeSSO{MaxSessionHours: 30 * 24, IdleTimeoutHours: 7 * 24, RememberDevice: true}
	case GeoIP:
		return GeoIP{}
	case Captcha:
		return Captcha{VerifyURL: "http://cap:3000", CapPort: 3343, TimeoutSeconds: 5}
	case Engine:
		return Engine{WAFDefaultMode: WAFHeadersOnly, WAFViolations: 5, WAFWindowSeconds: 60, RateRequests: 300,
			RateWindowSeconds: 10, ChallengeExpirySecs: 30 * 60, ChallengeMaxFailed: 15, ClearanceHours: 24,
			MetersEnabled: true, TCPNewPerMinute: 600, TCPBurst: 120, TCPMaxConcurrent: 1000, UDPNewPerMinute: 200,
			UDPBurst: 50, UDPPerSecond: 5000, UDPPPSBurst: 1000, StagingSeconds: 5 * 60, PollIntervalSecs: 3,
			BanFunction: BanExponential, BanBaseSeconds: 15 * 60, BanK: 2, BanMaxSeconds: 30 * 24 * 3600,
			DecaySeconds: 7 * 24 * 3600, FeedRefreshSeconds: 6 * 3600, FeedStaleSeconds: 72 * 3600}
	case AgentDNS:
		return AgentDNS{Domain: "hashi.home.arpa", IPMode: AgentIPSelected}
	case Appearance:
		return Appearance{Theme: "graphite", PublicDashboardEnabled: true, PublicStatusEnabled: true}
	}
	return c
}

func (General) Key() string    { return "general" }
func (Security) Key() string   { return "security" }
func (Appearance) Key() string { return "appearance" }
func (AgentDNS) Key() string   { return "internal_agent_dns" }

func (Pulse) Key() string { return "pulse" }

func (EdgeSSO) Key() string { return "edge_sso" }
func (GeoIP) Key() string   { return "geoip" }
func (Engine) Key() string  { return "engine" }
func (Captcha) Key() string { return "captcha" }

var siteKeyRe = regexp.MustCompile(`^[0-9a-f]{10}$`)

// Validate checks Captcha. Off, only the shape of what is filled in is checked.
func (c Captcha) Validate() error {
	if c.TimeoutSeconds < 1 || c.TimeoutSeconds > 30 {
		return invalid("the verification timeout is between 1 and 30 seconds")
	}
	if c.SiteKey != "" && !siteKeyRe.MatchString(c.SiteKey) {
		return invalid("a Cap site key is 10 hexadecimal characters")
	}
	if c.VerifyURL != "" {
		if u, err := url.Parse(c.VerifyURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
			(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.User != nil {
			return invalid("the verify address is http(s)://host[:port]")
		}
	}
	if c.CapPort < 1 || c.CapPort > 65535 {
		return invalid("the Cap port is between 1 and 65535")
	}
	if !c.Enabled {
		return nil
	}
	switch {
	case c.ChallengeDomain == "" || strings.ContainsAny(c.ChallengeDomain, "/: "):
		return invalid("CAPTCHA needs a challenge domain")
	case c.CapAddress == "":
		return invalid("CAPTCHA needs the address edges reach Cap on")
	case c.SiteKey == "" || c.VerifyURL == "":
		return invalid("CAPTCHA needs a site key and the verify address")
	}
	return nil
}

// Validate checks EdgeSSO.
func (e EdgeSSO) Validate() error {
	if e.MaxSessionHours < 1 || e.MaxSessionHours > 365*24 {
		return invalid("the longest session must be between an hour and a year")
	}
	if e.IdleTimeoutHours < 1 || e.IdleTimeoutHours > e.MaxSessionHours {
		return invalid("the idle timeout must be at least an hour and no longer than the longest session")
	}
	return nil
}

// Validate checks GeoIP.
func (g GeoIP) Validate() error {
	if g.URL == "" {
		return nil
	}
	u, err := url.Parse(g.URL)
	if err != nil || u.Scheme != "https" || u.Host == "" || !strings.Contains(g.URL, "{edition}") {
		return invalid("the download address must be https and contain {edition}")
	}
	// It is shown to anyone who can read settings, so it may not hold a password or token.
	if u.User != nil || u.RawQuery != "" {
		return invalid("the download address may not contain a login or a query")
	}
	return nil
}

// Validate checks Engine. Bounds keep a typo from switching protection off or banning for decades.
func (e Engine) Validate() error {
	day := 24 * 3600
	switch {
	case e.WAFDefaultMode == WAFEnabled:
		// 06 §9: body inspection breaks request bodies above about 160 KB with the pinned plugin.
		return invalid("checking request bodies is off until the WAF plugin's upload bug is fixed; use headers_only")
	case e.WAFDefaultMode != WAFHeadersOnly && e.WAFDefaultMode != WAFDisabled:
		return invalid("the WAF mode must be headers_only or disabled")
	case e.WAFViolations < 1 || e.WAFWindowSeconds < 1 || e.WAFWindowSeconds > 3600:
		return invalid("the WAF trigger needs at least one match within a window of up to an hour")
	case e.RateRequests < 10 || e.RateWindowSeconds < 1 || e.RateWindowSeconds > 3600:
		return invalid("the rate trigger needs at least 10 requests within a window of up to an hour")
	case e.ChallengeExpirySecs < 0 || e.ChallengeExpirySecs > 30*day:
		return invalid("a challenge lasts at most 30 days (0 means until solved)")
	case e.ChallengeMaxFailed < 1 || e.ChallengeMaxFailed > 10000:
		return invalid("unsolved requests before a ban must be between 1 and 10,000")
	case e.ClearanceHours < 1 || e.ClearanceHours > 30*24:
		return invalid("a CAPTCHA clearance lasts between an hour and 30 days")
	case e.TCPNewPerMinute < 1 || e.TCPBurst < 1 || e.TCPMaxConcurrent < 1 || e.UDPNewPerMinute < 1 || e.UDPBurst < 1 ||
		e.UDPPerSecond < 1 || e.UDPPPSBurst < 1:
		return invalid("meter limits must be positive")
	case e.StagingSeconds < 10 || e.StagingSeconds > day:
		return invalid("the staging timeout must be between 10 seconds and a day")
	case e.PollIntervalSecs < 1 || e.PollIntervalSecs > 60:
		return invalid("the host link polls every 1 to 60 seconds")
	case e.BanFunction != BanConstant && e.BanFunction != BanLinear && e.BanFunction != BanExponential:
		return invalid("the ban function must be constant, linear or exponential")
	case e.BanBaseSeconds < 60 || e.BanMaxSeconds < e.BanBaseSeconds || e.BanMaxSeconds > 3650*day:
		return invalid("a ban lasts at least a minute, and the longest ban at least as long as the first")
	case e.BanK < 1 || e.BanK > 100:
		return invalid("the ban factor must be between 1 and 100")
	case e.BanPermanentAfter < 0:
		return invalid("permanent after cannot be negative")
	case e.DecaySeconds < 3600 || e.DecaySeconds > 365*day:
		return invalid("counters drop by one per quiet period of an hour to a year")
	case e.FeedRefreshSeconds < 15*60 || e.FeedStaleSeconds < e.FeedRefreshSeconds:
		return invalid("feeds refresh at most every 15 minutes, and stay valid at least one refresh")
	}
	return nil
}

// Validate checks Pulse.
func (p Pulse) Validate() error {
	if p.HeartbeatIntervalSeconds < 10 || p.HeartbeatIntervalSeconds > 86400 {
		return invalid("heartbeat interval must be between 10 seconds and a day")
	}
	if p.StaleFactor < 2 || p.StaleFactor > 100 {
		return invalid("stale threshold must be between 2 and 100 missed heartbeats")
	}
	return nil
}

// Validate checks AgentDNS.
func (a AgentDNS) Validate() error {
	d, ok := normalizeFQDN(a.Domain)
	if !ok || d != a.Domain || !strings.Contains(d, ".") {
		return invalid("internal domain %q must be a lower-case domain name", a.Domain)
	}
	if a.IPMode != AgentIPSelected && a.IPMode != AgentIPPrivate && a.IPMode != AgentIPPublic {
		return invalid("address mode must be selected, private or public")
	}
	if a.RemoveStaleAfterHours < 0 {
		return invalid("the stale removal time cannot be negative")
	}
	return nil
}

// Configured reports whether setup step 2 has saved the base settings.
func (g General) Configured() bool { return g.RootDomain != "" }

// Validate checks General. Empty domains are allowed only before setup step 2 (all three empty together).
func (g General) Validate() error {
	if g.RootDomain == "" && g.AdminDomain == "" && g.InternalURL == "" {
		return g.validateCommon()
	}
	root, ok := normalizeFQDN(g.RootDomain)
	if !ok || !strings.Contains(root, ".") {
		return invalid("root domain %q is not a domain name", g.RootDomain)
	}
	admin, ok := normalizeFQDN(g.AdminDomain)
	if !ok || (admin != root && !strings.HasSuffix(admin, "."+root)) {
		return invalid("admin domain %q must be a domain name under %s", g.AdminDomain, root)
	}
	if root != g.RootDomain || admin != g.AdminDomain {
		return invalid("domains must be lower case without a trailing dot")
	}
	u, err := url.Parse(g.InternalURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return invalid("internal URL %q must be an absolute http(s) URL", g.InternalURL)
	}
	return g.validateCommon()
}

func (g General) validateCommon() error {
	if g.SyncIntervalSeconds < minSyncInterval || g.SyncIntervalSeconds > maxSyncInterval {
		return invalid("sync interval must be between 1 minute and 24 hours")
	}
	if _, err := clientip.ParsePrefixes(g.TrustedProxies); err != nil {
		return invalid("trusted proxies: %v", err)
	}
	return nil
}

// Validate checks Security.
func (s Security) Validate() error {
	if s.AdminIdleTimeoutMinutes < 5 || time.Duration(s.AdminIdleTimeoutMinutes)*time.Minute > MaxAdminIdle {
		return invalid("admin idle timeout must be between 5 minutes and 4 hours")
	}
	if len(s.BootstrapAllowedCIDRs) == 0 {
		return invalid("bootstrap needs at least one allowed network")
	}
	if _, err := clientip.ParsePrefixes(s.BootstrapAllowedCIDRs); err != nil {
		return invalid("bootstrap networks: %v", err)
	}
	return nil
}

// AdminIdle is the configured idle lifetime.
func (s Security) AdminIdle() time.Duration {
	return time.Duration(s.AdminIdleTimeoutMinutes) * time.Minute
}

// Validate checks Appearance.
func (a Appearance) Validate() error {
	if !slices.Contains(Themes, a.Theme) {
		return invalid("unknown theme %q", a.Theme)
	}
	return nil
}

// normalizeFQDN lower-cases and checks label syntax (RFC 1123 host names).
func normalizeFQDN(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(s), "."))
	if s == "" || len(s) > 253 {
		return "", false
	}
	for l := range strings.SplitSeq(s, ".") {
		if l == "" || len(l) > 63 || l[0] == '-' || l[len(l)-1] == '-' {
			return "", false
		}
		for _, r := range l {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				return "", false
			}
		}
	}
	return s, true
}
