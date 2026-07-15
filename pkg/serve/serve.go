package serve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// DefaultShutdownGrace is the drain time given to in-flight requests
// during shutdown when Config.ShutdownGrace is zero.
const DefaultShutdownGrace = 30 * time.Second

// Config carries the parameters for [Run].
type Config struct {
	// Handler serves every non-health endpoint. Required.
	Handler http.Handler
	// Addr is the listen address, such as ":8080". Required.
	Addr string
	// Readiness gates /readyz: nil means always ready; otherwise ok
	// yields 200 with detail as the JSON body and !ok yields 503.
	Readiness func(context.Context) (detail any, ok bool)
	// ShutdownGrace bounds the drain; zero uses DefaultShutdownGrace.
	ShutdownGrace time.Duration
	// ExecWatchInterval polls the executable's mtime; zero uses
	// DefaultExecWatchInterval, negative disables watching.
	ExecWatchInterval time.Duration
}

// Run serves cfg.Handler under /healthz and /readyz until ctx is
// canceled, serving fails, or a watched executable is replaced, then
// drains in-flight requests within the grace period. A clean shutdown
// returns nil; an executable watch that cannot start is an error.
func Run(ctx context.Context, cfg Config) error {
	if cfg.Handler == nil {
		return errors.New("serve: Config.Handler is required")
	}
	if cfg.Addr == "" {
		return errors.New("serve: Config.Addr is required")
	}
	grace := cfg.ShutdownGrace
	if grace <= 0 {
		grace = DefaultShutdownGrace
	}
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           wrap(cfg.Handler, cfg.Readiness),
		ReadHeaderTimeout: 30 * time.Second,
	}
	serveErr := make(chan error, 1)
	go func() {
		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			err = fmt.Errorf("serve: %w", err)
		} else {
			err = nil // a closed server is a clean stop, not a failure
		}
		serveErr <- err
	}()

	var execChanged <-chan struct{}
	if cfg.ExecWatchInterval >= 0 {
		ch, err := WatchExec(ctx, cfg.ExecWatchInterval)
		if err != nil {
			// The listener is already up; close it so no goroutine
			// outlives the failed Run.
			_ = srv.Close()
			return err
		}
		execChanged = ch
	}
	select {
	case <-ctx.Done():
		return shutdown(srv, serveErr, grace)
	case err := <-serveErr:
		return err
	case <-execChanged:
		return shutdown(srv, serveErr, grace)
	}
}

// shutdown drains srv with a fresh context, so the canceled parent does
// not cut the grace period short.
func shutdown(srv *http.Server, serveErr <-chan error, grace time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), grace)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		_ = srv.Close() // best-effort force close after a timed-out drain
		return fmt.Errorf("serve: shutdown: %w", err)
	}
	return <-serveErr
}

// wrap serves /healthz and /readyz directly and delegates the rest.
func wrap(user http.Handler, readiness func(context.Context) (any, bool)) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, healthBody{Status: "ok"})
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if readiness == nil {
			writeJSON(w, http.StatusOK, healthBody{Status: "ok"})
			return
		}
		detail, ok := readiness(r.Context())
		status := http.StatusServiceUnavailable
		if ok {
			status = http.StatusOK
		}
		if detail != nil {
			writeJSON(w, status, detail)
		} else if ok {
			writeJSON(w, status, healthBody{Status: "ok"})
		} else {
			writeJSON(w, status, healthBody{Status: "unavailable"})
		}
	})
	mux.Handle("/", user)
	return mux
}

// healthBody is frozen: external monitors may only rely on "status".
type healthBody struct {
	Status string `json:"status"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
