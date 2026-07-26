package serve

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func testHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("app"))
	})
}

func TestWrapEndpoints(t *testing.T) {
	srv := httptest.NewServer(wrap(testHandler(), func(context.Context) (any, bool) {
		return map[string]string{"dep": "up"}, true
	}))
	defer srv.Close()
	for _, tc := range []struct{ path, status, body string }{
		{"/healthz", "200 OK", "{\"status\":\"ok\"}\n"},
		{"/readyz", "200 OK", "{\"dep\":\"up\"}\n"},
		{"/anything", "200 OK", "app"},
	} {
		resp, err := http.Get(srv.URL + tc.path)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.Status != tc.status {
			t.Errorf("GET %s = %s, want %s", tc.path, resp.Status, tc.status)
		}
	}
}

func TestReadinessNotReady(t *testing.T) {
	h := wrap(http.NewServeMux(), func(context.Context) (any, bool) {
		return map[string]string{"reason": "db down"}, false
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("/readyz code = %d, want 503", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["reason"] != "db down" {
		t.Fatalf("/readyz body = %q (%v)", rec.Body.String(), err)
	}
}

func TestReadinessNilDetail(t *testing.T) {
	rec := httptest.NewRecorder()
	wrap(http.NewServeMux(), func(context.Context) (any, bool) { return nil, false }).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable || rec.Body.String() != "{\"status\":\"unavailable\"}\n" {
		t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestRunValidatesConfig(t *testing.T) {
	if err := Run(context.Background(), Config{}); err == nil {
		t.Fatal("Run accepted an empty config")
	}
	if err := Run(context.Background(), Config{Handler: http.NewServeMux()}); err == nil {
		t.Fatal("Run accepted an empty Addr")
	}
}

func TestRunGracefulStopOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{
			Handler:           testHandler(),
			Addr:              listenAddr(t),
			ExecWatchInterval: -1,
			ShutdownGrace:     time.Second,
		})
	}()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run after cancel = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestRunFailsOnTakenPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	err = Run(context.Background(), Config{
		Handler:           http.NewServeMux(),
		Addr:              ln.Addr().String(),
		ExecWatchInterval: -1,
		ShutdownGrace:     time.Second,
	})
	if err == nil {
		t.Fatal("Run on a taken port returned nil")
	}
}

func listenAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}
