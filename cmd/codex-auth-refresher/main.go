package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"codex-auth-refresher/internal/config"
	"codex-auth-refresher/internal/httpapi"
	"codex-auth-refresher/internal/metrics"
	"codex-auth-refresher/internal/oauth"
	"codex-auth-refresher/internal/refresher"
	"codex-auth-refresher/internal/scheduler"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Parse(os.Args[1:], os.Environ())
	if err != nil {
		return err
	}

	logger := newLogger(cfg.LogFormat)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	httpClient, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}

	metricsRegistry := metrics.New()
	oauthClient := oauth.NewClient(cfg.TokenEndpoint, httpClient)
	refreshService := refresher.NewService(oauthClient, cfg.RefreshBefore, cfg.RefreshMaxAge, cfg.ClientID)
	manager := scheduler.NewManager(cfg.AuthDir, cfg.ScanInterval, cfg.MaxParallel, refreshService, metricsRegistry, logger)

	handler := httpapi.NewHandler(manager, metricsRegistry, httpapi.Options{
		StatusEnabled: cfg.StatusEnable,
		WebEnabled:    cfg.WebEnable,
		RefreshBefore: cfg.RefreshBefore.String(),
		RefreshMaxAge: formatRefreshMaxAge(cfg.RefreshMaxAge),
		ScanInterval:  cfg.ScanInterval.String(),
		MaxParallel:   cfg.MaxParallel,
	})
	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 2)
	go func() {
		logger.Info("starting refresh manager", "auth_dir", cfg.AuthDir, "refresh_before", cfg.RefreshBefore.String(), "refresh_max_age", formatRefreshMaxAge(cfg.RefreshMaxAge), "scan_interval", cfg.ScanInterval.String(), "web_enable", cfg.WebEnable)
		errCh <- manager.Run(ctx)
	}()
	go func() {
		logger.Info("starting http server", "listen_addr", cfg.ListenAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil {
			stop()
			return err
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return err
	}
	return nil
}

func newLogger(format string) *slog.Logger {
	var handler slog.Handler
	switch format {
	case "text":
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	default:
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	}
	return slog.New(handler)
}

func newHTTPClient(cfg config.Config) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	if cfg.CAFile != "" {
		pemBytes, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read CA file: %w", err)
		}
		pool, err := x509.SystemCertPool()
		if err != nil {
			pool = x509.NewCertPool()
		}
		if ok := pool.AppendCertsFromPEM(pemBytes); !ok {
			return nil, fmt.Errorf("append CA file: no certificates found")
		}
		transport.TLSClientConfig.RootCAs = pool
	}
	return &http.Client{Timeout: cfg.HTTPTimeout, Transport: roundTripperWithUA{base: transport}}, nil
}

type roundTripperWithUA struct {
	base http.RoundTripper
}

func (r roundTripperWithUA) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "codex-auth-refresher/1.0")
	}
	resp, err := r.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	resp.Body = struct {
		io.Reader
		io.Closer
	}{Reader: resp.Body, Closer: resp.Body}
	return resp, nil
}

func formatRefreshMaxAge(value time.Duration) string {
	if value <= 0 {
		return "disabled"
	}
	return value.String()
}
