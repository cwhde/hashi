package app

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Config is the process configuration. Flags override HASHI_* environment variables, which override defaults.
// Everything else (connections, behaviour) lives in the database and is edited in the UI.
type Config struct {
	DataDir          string
	AdminAddr        string
	PublicAppsAddr   string // "" disables the listener
	PublicStatusAddr string // "" disables the listener
	LogLevel         slog.Level
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
	fs.StringVar(&level, "log-level", env("HASHI_LOG_LEVEL", "info"), "debug, info, warn or error")
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
	return c, nil
}

func lookup(getenv func(string) string, k string) (string, bool) {
	if getenv == nil {
		return os.LookupEnv(k)
	}
	v := getenv(k)
	return v, v != ""
}
