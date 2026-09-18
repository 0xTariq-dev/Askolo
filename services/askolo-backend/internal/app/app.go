package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"askolo/backend/internal/config"
	"askolo/backend/internal/httpapi"
)

const (
	shutdownTimeout   = 10 * time.Second
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 15 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 60 * time.Second
)

type App struct {
	server *http.Server
	logger *slog.Logger
}

func New(cfg config.Config, logger *slog.Logger) *App {
	if logger == nil {
		logger = slog.Default()
	}

	return &App{
		logger: logger,
		server: &http.Server{
			Addr:              cfg.Host + ":" + strconv.Itoa(cfg.Port),
			Handler:           httpapi.New(cfg, logger),
			ReadHeaderTimeout: readHeaderTimeout,
			ReadTimeout:       readTimeout,
			WriteTimeout:      writeTimeout,
			IdleTimeout:       idleTimeout,
		},
	}
}

func (a *App) Run(ctx context.Context) error {
	serverErrors := make(chan error, 1)
	go func() {
		a.logger.Info("askolo backend listening", "addr", a.server.Addr)
		serverErrors <- a.server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := a.server.Shutdown(shutdownContext); err != nil {
			return err
		}
		return nil
	}
}