// Command hashi runs the Hashi control plane.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"git.juzo.io/juzo/hashi/internal/app"
	"git.juzo.io/juzo/hashi/internal/logring"
	"git.juzo.io/juzo/hashi/internal/vault"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version":
			fmt.Println(app.Version, app.Commit)
			return
		case "healthcheck":
			// For container HEALTHCHECK: the runtime image has no shell or curl.
			os.Exit(healthcheck())
		case "keygen":
			k, err := vault.Keygen()
			if err != nil {
				fmt.Fprintln(os.Stderr, "keygen:", err)
				os.Exit(1)
			}
			fmt.Println(k)
			return
		case "bootstrap", "vault", "admin":
			os.Exit(console(os.Args[1:]))
		}
	}
	cfg, err := app.LoadConfig(os.Args[1:], nil)
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "hashi:", err)
		os.Exit(2)
	}
	// The live log (Activity) sees exactly what stdout sees: lines after redaction.
	cfg.Logs = logring.New(2000)
	log := app.NewLogger(io.MultiWriter(os.Stdout, cfg.Logs), cfg.LogLevel)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := app.Run(ctx, cfg, log, nil); err != nil {
		log.Error("exiting", "err", err)
		os.Exit(1)
	}
}

// console runs `hashi bootstrap reset` and `hashi vault recover` with the server's environment config.
func console(args []string) int {
	cfg, err := app.LoadConfig(nil, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "hashi:", err)
		return 2
	}
	ctx := context.Background()
	switch strings.Join(args, " ") {
	case "bootstrap reset":
		err = app.BootstrapReset(ctx, cfg, os.Stdout)
	case "admin recover":
		err = app.AdminRecover(ctx, cfg, os.Stdout)
	case "vault recover":
		err = app.VaultRecover(ctx, cfg, os.Stdin, os.Stdout)
	default:
		fmt.Fprintln(os.Stderr, "usage: hashi bootstrap reset | hashi admin recover | hashi vault recover")
		return 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "hashi:", err)
		return 1
	}
	return 0
}

// healthcheck asks this container's own admin listener (loopback, port from the same config) for /readyz.
func healthcheck() int {
	cfg, err := app.LoadConfig(nil, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	_, port, err := net.SplitHostPort(cfg.AdminAddr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	url := "http://" + net.JoinHostPort("127.0.0.1", port) + "/readyz"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil) //nolint:gosec // see above
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	res, err := http.DefaultClient.Do(req) //nolint:gosec // loopback URL built from our own listener config
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck: status", res.StatusCode)
		return 1
	}
	return 0
}
