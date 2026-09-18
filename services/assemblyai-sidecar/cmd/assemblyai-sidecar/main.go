package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"askolo/assemblyai-sidecar/internal/server"
)

const (
	defaultPort       = 8090
	shutdownTimeout   = 10 * time.Second
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 15 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 60 * time.Second
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	port := envInt("PORT", defaultPort)
	handler := server.NewHandler(logger)
	httpServer := &http.Server{
		Addr:              ":" + strconv.Itoa(port),
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	serverErrors := make(chan error, 1)
	go func() {
		slog.Info("assemblyai sidecar listening", "addr", httpServer.Addr)
		serverErrors <- httpServer.ListenAndServe()
	}()

	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			slog.Error("assemblyai sidecar stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	case signal := <-shutdownSignal:
		slog.Info("assemblyai sidecar shutting down", "signal", signal.String())
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := httpServer.Shutdown(ctx); err != nil {
			slog.Error("assemblyai sidecar shutdown failed", "error", err)
			os.Exit(1)
		}
	}
}

func envInt(name string, fallback int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > 65535 {
		slog.Warn("invalid port configuration; using default", "name", name, "value", raw, "default", fallback)
		return fallback
	}
	return value
}
