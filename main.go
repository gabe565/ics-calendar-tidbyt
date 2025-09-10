package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"gabe565.com/ics-calendar-tidbyt/internal/config"
	"gabe565.com/ics-calendar-tidbyt/internal/server"
	"gabe565.com/ics-calendar-tidbyt/internal/util"
	"github.com/caarlos0/env/v11"
)

var version = "beta"

func main() {
	slog.Info("ICS Calendar Tidbyt", "version", version, "commit", util.GetCommit())
	if err := run(); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func run() error {
	conf, err := env.ParseAs[config.Config]()
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	defer cancel()

	return server.ListenAndServe(ctx, &conf)
}
