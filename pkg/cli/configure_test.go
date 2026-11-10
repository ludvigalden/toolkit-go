package cli

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/knadh/koanf/v2"

	"github.com/ludvigalden/toolkit-go/pkg/exitcode"
	"github.com/ludvigalden/toolkit-go/pkg/output"
)

func TestConfigureOutput(t *testing.T) {
	_, _, env := sandbox(t)
	env["DEMO_DIR"] = "fromenv"
	var out, errb, logs bytes.Buffer
	closes := 0
	app := &App{
		Name: "demo", Stdout: &out, Stderr: &errb,
		Defaults: map[string]any{"demo.dir": "default"},
		Getenv:   func(k string) string { return env[k] },
		ConfigureOutput: func(cfg *koanf.Koanf, g Global) (OutputSetup, error) {
			if cfg.String("demo.dir") != "fromenv" || g.AppName != "demo" {
				t.Errorf("resolved config not passed: %q %q", cfg.String("demo.dir"), g.AppName)
			}
			h := slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})
			return OutputSetup{
				Options: []output.Option{output.WithLogHandler(h)},
				Close:   func() error { closes++; return nil },
			}, nil
		},
	}
	r, code := app.Parse(&root{}, []string{"show"})
	if r == nil {
		t.Fatalf("parse: %d %s", code, errb.String())
	}
	r.Out.Info("narration")
	if code := r.Exec(); code != 0 {
		t.Fatalf("exec = %d", code)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if !strings.Contains(logs.String(), "narration") {
		t.Errorf("configured handler missed the record: %q", logs.String())
	}
	if closes != 1 {
		t.Errorf("setup Close ran %d times, want 1", closes)
	}
}

func TestConfigureOutputErrors(t *testing.T) {
	_, _, env := sandbox(t)
	var errb bytes.Buffer
	app := &App{
		Name: "demo", Stderr: &errb,
		Getenv: func(k string) string { return env[k] },
		ConfigureOutput: func(*koanf.Koanf, Global) (OutputSetup, error) {
			return OutputSetup{}, errors.New("log file locked")
		},
	}
	if r, code := app.Parse(&root{}, []string{"show"}); r != nil || code != int(exitcode.CantCreate) {
		t.Fatalf("code = %d, want CantCreate", code)
	}
	if !strings.Contains(errb.String(), "log file locked") {
		t.Errorf("error not reported: %q", errb.String())
	}

	closed := false
	app = &App{
		Name: "demo", Stderr: &errb,
		Getenv: func(k string) string { return env[k] },
		ConfigureOutput: func(*koanf.Koanf, Global) (OutputSetup, error) {
			return OutputSetup{Close: func() error { closed = true; return nil }}, nil
		},
	}
	if r, _ := app.Parse(&root{}, []string{"--stdout-format=nope", "show"}); r != nil {
		t.Fatal("bad mode parsed")
	}
	if !closed {
		t.Error("setup not closed when output creation failed")
	}

	app.ConfigureOutput = func(*koanf.Koanf, Global) (OutputSetup, error) {
		return OutputSetup{Close: func() error { return errors.New("flush failed") }}, nil
	}
	errb.Reset()
	if code := app.Main(&root{}, []string{"show"}); code != int(exitcode.Software) {
		t.Errorf("close error: code %d, want Software", code)
	}
	if !strings.Contains(errb.String(), "flush failed") {
		t.Errorf("close error not reported: %q", errb.String())
	}
}
