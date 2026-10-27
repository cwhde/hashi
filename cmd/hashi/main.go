// Command hashi runs the Hashi control plane.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"git.juzo.io/juzo/hashi/internal/app"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(app.Version, app.Commit)
		return
	}
	cfg, err := app.LoadConfig(os.Args[1:], nil)
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "hashi:", err)
		os.Exit(2)
	}
	log := app.NewLogger(os.Stdout, cfg.LogLevel)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := app.Run(ctx, cfg, log, nil); err != nil {
		log.Error("exiting", "err", err)
		os.Exit(1)
	}
}
