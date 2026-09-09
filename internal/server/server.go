package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gabe565.com/ics-calendar-tidbyt/internal/config"
	"gabe565.com/utils/bytefmt"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
)

func ListenAndServe(ctx context.Context, conf *config.Config) error {
	r := chi.NewRouter()
	r.Use(middleware.Heartbeat("/ping"))

	if conf.TrustedProxies != nil {
		r.Use(middleware.ClientIPFromXFF(conf.TrustedProxies...))
	} else {
		if realIPHeader, err := config.RealIPHeader(); err != nil {
			return err
		} else if realIPHeader {
			slog.Warn(
				"REAL_IP_HEADER is deprecated and will be removed in a future release, use TRUSTED_PROXIES instead",
			)
			r.Use(middleware.ClientIPFromHeader("X-Real-IP"))
		}
	}

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.GetHead)
	r.Use(httprate.LimitBy(10, time.Minute, func(r *http.Request) (string, error) {
		return middleware.GetClientIP(r.Context()), nil
	}))
	r.Use(middleware.Timeout(60 * time.Second))

	r.Post("/", ICS())

	server := &http.Server{
		Addr:           conf.ListenAddress,
		Handler:        r,
		ReadTimeout:    5 * time.Second,
		MaxHeaderBytes: 100 * bytefmt.KiB,
	}

	errCh := make(chan error, 1)

	go func() {
		slog.Info("Listening for http connections", "address", conf.ListenAddress)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	defer cancel()

	select {
	case <-ctx.Done():
		ctx, cancelTimeout := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelTimeout()

		ctx, cancelSignal := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
		defer cancelSignal()

		slog.Info("Gracefully stopping server")
		if err := server.Shutdown(ctx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return nil
	case err := <-errCh:
		return err
	}
}
