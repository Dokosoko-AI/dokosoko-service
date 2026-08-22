package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dokosoko/examples/go-backend-integration/internal/backend"
	postgresstore "github.com/dokosoko/examples/go-backend-integration/internal/postgres"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	startupContext, cancelStartup := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelStartup()
	store, err := postgresstore.Open(startupContext, os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.Migrate(startupContext); err != nil {
		return err
	}
	handler, err := backend.NewHandler(store, os.Getenv("DOKOSOKO_BACKEND_BEARER_TOKEN"), logger)
	if err != nil {
		return err
	}
	address := os.Getenv("LISTEN_ADDR")
	if address == "" {
		address = ":8081"
	}
	server := &http.Server{
		Addr:              address,
		Handler:           handler.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}

	shutdownContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serverError := make(chan error, 1)
	go func() {
		logger.Info("backend integration listening", "address", address)
		serverError <- server.ListenAndServe()
	}()

	select {
	case err := <-serverError:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-shutdownContext.Done():
		logger.Info("shutting down backend integration")
		context, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(context)
	}
}
