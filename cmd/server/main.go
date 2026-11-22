// Command server wires the store, services and handlers together and serves HTTP.
package main

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jul1eNotFound/webdev-273222011/internal/auth"
	"github.com/Jul1eNotFound/webdev-273222011/internal/config"
	"github.com/Jul1eNotFound/webdev-273222011/internal/handler"
	"github.com/Jul1eNotFound/webdev-273222011/internal/model"
	"github.com/Jul1eNotFound/webdev-273222011/internal/service"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(log); err != nil {
		log.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	csrf, err := auth.NewCSRF(cfg.SessionSecret)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return err
	}

	st := store.New(pool)
	// A nil interface, not a nil *auth.Google, when Google is not configured: the button stays hidden.
	var google handler.GoogleProvider
	if cfg.GoogleEnabled() {
		google = auth.NewGoogle(cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.GoogleRedirectURL())
		log.Info("sign in with google is on", "redirect", cfg.GoogleRedirectURL())
	}
	labs := service.NewLabs(st, time.Now, time.Local)
	if cfg.AnthropicAPIKey != "" {
		m := model.NewAnthropic(cfg.AnthropicAPIKey, cfg.ModelName)
		labs.WithSearchModel(m)
		log.Info("smart search is on", "model", cmp.Or(cfg.ModelName, model.DefaultModel))
	}
	h, err := handler.New(handler.Deps{
		Auth:          service.NewAuth(st, auth.Bcrypt{}, time.Now),
		Labs:          labs,
		SecureCookies: cfg.SecureCookies(),
		CSRF:          csrf,
		Google:        google,
		Log:           log,
	})
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	log.Info("listening", "port", cfg.Port)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
