package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/andrewedunn/agent-reading-relay/internal/config"
	"github.com/andrewedunn/agent-reading-relay/internal/instapaper"
	"github.com/andrewedunn/agent-reading-relay/internal/relay"
	"github.com/andrewedunn/agent-reading-relay/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("reading relay stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	listener, err := listenUnix(cfg.SocketPath)
	if err != nil {
		return err
	}
	defer func() {
		listener.Close()
		_ = os.Remove(cfg.SocketPath)
	}()

	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o750); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	articles, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer articles.Close()

	var delivery relay.Instapaper
	if cfg.Instapaper.Configured() {
		delivery = instapaper.Client{Credentials: cfg.Instapaper.Credentials}
		slog.Info("Instapaper delivery enabled")
	} else {
		slog.Info("Instapaper credentials absent; relay is in draft-only mode")
	}
	service := &relay.Service{
		Store: articles, Instapaper: delivery, PublicBaseURL: cfg.PublicBaseURL,
		OwnerEmail: cfg.OwnerEmail, AllowedAgents: cfg.AllowedAgents,
	}

	publicServer := &http.Server{
		Addr: cfg.PublicAddr, Handler: relay.PublicHandler(service),
		ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second,
	}
	apiServer := &http.Server{Handler: relay.APIHandler(service), ReadHeaderTimeout: 5 * time.Second}

	errorsCh := make(chan error, 2)
	go func() {
		slog.Info("public article server listening", "addr", cfg.PublicAddr)
		errorsCh <- normalizeServerError(publicServer.ListenAndServe())
	}()
	go func() {
		slog.Info("agent API listening", "socket", cfg.SocketPath)
		errorsCh <- normalizeServerError(apiServer.Serve(listener))
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case <-ctx.Done():
	case err := <-errorsCh:
		if err != nil {
			return err
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = apiServer.Shutdown(shutdownCtx)
	_ = publicServer.Shutdown(shutdownCtx)
	return nil
}

func listenUnix(path string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create socket directory: %w", err)
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("refusing to replace non-socket path %s", path)
		}
		connection, dialErr := net.DialTimeout("unix", path, 200*time.Millisecond)
		if dialErr == nil {
			connection.Close()
			return nil, fmt.Errorf("reading relay socket is already active at %s", path)
		}
		if !errors.Is(dialErr, syscall.ECONNREFUSED) && !errors.Is(dialErr, os.ErrNotExist) {
			return nil, fmt.Errorf("inspect existing Unix socket: %w", dialErr)
		}
		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("remove stale socket: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect socket path: %w", err)
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen on Unix socket: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		listener.Close()
		return nil, fmt.Errorf("secure Unix socket: %w", err)
	}
	return listener, nil
}

func normalizeServerError(err error) error {
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
