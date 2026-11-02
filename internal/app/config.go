package app

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"git.juzo.io/juzo/hashi/internal/dns"
	"git.juzo.io/juzo/hashi/internal/logring"
)

// Config is the process configuration. Flags override HASHI_* environment variables, which override defaults.
// Everything else (connections, behaviour) lives in the database and is edited in the UI.
type Config struct {
	DataDir          string
	AdminAddr        string
	PublicAppsAddr   string // "" disables the listener
	PublicStatusAddr string // "" disables the listener
	LogLevel         slog.Level
	MasterKeyFile    string // HASHI_MASTER_KEY_FILE: base64 master key from `hashi keygen` (02-setup-auth.md §5.2)
	// CapEnvFile is Cap's cap.env from the same compose project (HASHI_CAP_ENV_FILE): with it Hashi logs in to Cap's
	// dashboard API and creates its own site key (06 §11). CapPort is Cap's host port, where edges reach it.
	CapEnvFile        string
	CapPort           int
	DevWebAuthnOrigin string // dev builds only, see devbuild_dev.go
	DevDNSFixtures    string // dev builds only: directory of recorded Hetzner responses to serve instead of the API

	// Set by main, not by flags: where the one-time bootstrap block goes, and the live-log ring.
	Stdout io.Writer
	Logs   *logring.Ring

	// dnsProviders replaces the real provider adapters (tests point them at the recorded fixtures).
	dnsProviders dns.Factory
}

// DBPath is the SQLite file inside DataDir.
func (c Config) DBPath() string { return filepath.Join(c.DataDir, "hashi.db") }

// LoadConfig parses args (without the program name) on top of env.
func LoadConfig(args []string, getenv func(string) string) (Config, error) {
	env := func(k, def string) string {
		if v, ok := lookup(getenv, k); ok {
			return v
		}
		return def
	}
	fs := flag.NewFlagSet("hashi", flag.ContinueOnError)
	var c Config
	var level string
	fs.StringVar(&c.DataDir, "data-dir", env("HASHI_DATA_DIR", "/data"), "directory for the database, backups and assets")
	fs.StringVar(&c.AdminAddr, "admin-addr", env("HASHI_ADMIN_ADDR", ":8080"), "admin UI + API listener")
	fs.StringVar(&c.PublicAppsAddr, "public-apps-addr", env("HASHI_PUBLIC_APPS_ADDR", ":8081"), `public app dashboard listener ("off" disables)`)
	fs.StringVar(&c.PublicStatusAddr, "public-status-addr", env("HASHI_PUBLIC_STATUS_ADDR", ":8082"), `public status page listener ("off" disables)`)
	fs.StringVar(&c.MasterKeyFile, "master-key-file", env("HASHI_MASTER_KEY_FILE", ""), "file with the base64 master key (hashi keygen)")
	fs.StringVar(&level, "log-level", env("HASHI_LOG_LEVEL", "info"), "debug, info, warn or error")
	fs.StringVar(&c.CapEnvFile, "cap-env-file", env("HASHI_CAP_ENV_FILE", ""), "Cap's cap.env (its dashboard password), when Cap runs next to Hashi")
	capPort, err := strconv.Atoi(env("HASHI_CAP_PORT", "3343"))
	if err != nil || capPort < 1 || capPort > 65535 {
		return Config{}, fmt.Errorf("HASHI_CAP_PORT must be a port number")
	}
	fs.IntVar(&c.CapPort, "cap-port", capPort, "Cap's host port, where edges reach it")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if err := c.LogLevel.UnmarshalText([]byte(level)); err != nil {
		return Config{}, fmt.Errorf("log level %q: %w", level, err)
	}
	for _, p := range []*string{&c.PublicAppsAddr, &c.PublicStatusAddr} {
		if strings.EqualFold(*p, "off") {
			*p = ""
		}
	}
	if c.AdminAddr == "" || strings.EqualFold(c.AdminAddr, "off") {
		return Config{}, fmt.Errorf("the admin listener cannot be disabled")
	}
	if c.DataDir == "" {
		return Config{}, fmt.Errorf("data dir must be set")
	}
	if devBuild {
		c.DevWebAuthnOrigin = env("HASHI_DEV_WEBAUTHN_ORIGIN", "")
		if u, err := url.Parse(c.DevWebAuthnOrigin); c.DevWebAuthnOrigin != "" && (err != nil || u.Hostname() != "localhost") {
			return Config{}, fmt.Errorf("HASHI_DEV_WEBAUTHN_ORIGIN must be a http(s)://localhost origin")
		}
		c.DevDNSFixtures = env("HASHI_DEV_DNS_FIXTURES", "")
	}
	return c, nil
}

func lookup(getenv func(string) string, k string) (string, bool) {
	if getenv == nil {
		return os.LookupEnv(k)
	}
	v := getenv(k)
	return v, v != ""
}
