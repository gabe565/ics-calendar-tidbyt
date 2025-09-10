package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"gabe565.com/ics-calendar-tidbyt/internal/config"
	"gabe565.com/utils/bytefmt"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/sync/errgroup"
)

func ListenAndServe(ctx context.Context, conf *config.Config) error {
	r := chi.NewRouter()
	r.Use(middleware.Heartbeat("/ping"))
	if conf.RealIPHeader {
		r.Use(middleware.RealIP)
	}
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.GetHead)

	r.Post("/", ICS())

	server := &http.Server{
		Addr:           conf.ListenAddress,
		Handler:        r,
		ReadTimeout:    5 * time.Second,
		MaxHeaderBytes: 100 * bytefmt.KiB,
	}

	group, ctx := errgroup.WithContext(ctx)

	group.Go(func() error {
		slog.Info("Listening for http connections", "address", conf.ListenAddress)
		return server.ListenAndServe()
	})

	group.Go(func() error {
		<-ctx.Done()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		slog.Info("Gracefully shutting down server")
		return server.Shutdown(ctx)
	})

	err := group.Wait()
	if errors.Is(err, context.Canceled) || errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	return err
}
