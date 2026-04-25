package secret

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestValueRedacts(t *testing.T) {
	v := NewValue("hunter2")
	j, err := json.Marshal(struct{ P *Value }{v})
	if err != nil {
		t.Fatal(err)
	}
	var logBuf bytes.Buffer
	slog.New(slog.NewTextHandler(&logBuf, nil)).Info("x", "pw", v)
	surfaces := map[string]string{
		"%s":   v.String(),
		"%v":   fmt.Sprintf("%v", v),
		"%+v":  fmt.Sprintf("%+v", v),
		"%#v":  fmt.Sprintf("%#v", v),
		"json": string(j),
		"slog": logBuf.String(),
	}
	for name, out := range surfaces {
		if strings.Contains(out, "hunter2") || !strings.Contains(out, redacted) {
			t.Errorf("%s leaked or lacked redaction: %q", name, out)
		}
	}
	if txt, _ := v.MarshalText(); string(txt) != redacted {
		t.Errorf("MarshalText = %q", txt)
	}
	if v.Reveal() != "hunter2" || v.Len() != 7 {
		t.Errorf("Reveal/Len = %q/%d", v.Reveal(), v.Len())
	}
	v.Zero()
	if v.Reveal() != "\x00\x00\x00\x00\x00\x00\x00" {
		t.Errorf("after Zero = %q", v.Reveal())
	}
}

func TestNilValue(t *testing.T) {
	var v *Value
	v.Zero()
	if v.Reveal() != "" || v.Len() != 0 || v.String() != redacted {
		t.Error("nil Value misbehaves")
	}
}

func TestSources(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "tok")
	if err := os.WriteFile(path, []byte("  file-secret \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	getenv := func(k string) string { return map[string]string{"TOKEN": "env-secret"}[k] }
	tests := []struct {
		name    string
		src     Source
		want    string
		wantErr string
	}{
		{"env", Env(getenv, "TOKEN"), "env-secret", ""},
		{"env empty", Env(getenv, "MISSING"), "", "MISSING is empty"},
		{"literal", Literal("lit"), "lit", ""},
		{"file", File(path), "  file-secret", ""},
		{"file missing", File(filepath.Join(dir, "nope")), "", "no such file"},
		{"command", Command("printf", "cmd-secret\n\n"), "cmd-secret", ""},
		{"command no shell", Command("echo", "$HOME", "|", "x"), "$HOME | x", ""},
		{"command stderr", Command("sh", "-c", "echo oops >&2; exit 3"), "", "oops"},
		{"command silent fail", Command("false"), "", "exit status 1"},
		{"command empty", Command(), "", "no argv"},
		{"vault", FromVault(mapVault{"k": "v-secret"}, "k"), "v-secret", ""},
		{"vault miss", FromVault(mapVault{}, "k"), "", "not in vault"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.src.Fetch(ctx)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("Fetch = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestCommandEnvAndDir(t *testing.T) {
	dir := t.TempDir()
	c := &CommandSource{Argv: []string{"sh", "-c", `printf "%s:%s" "$X" "$(pwd -P)"`}, Env: []string{"X=1", "PATH=" + os.Getenv("PATH")}, Dir: dir}
	got, err := c.Fetch(context.Background())
	real, _ := filepath.EvalSymlinks(dir)
	if err != nil || got != "1:"+real {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestEnvNilGetenv(t *testing.T) {
	t.Setenv("SECRET_TEST_VAR", "x")
	if got, err := Env(nil, "SECRET_TEST_VAR").Fetch(context.Background()); err != nil || got != "x" {
		t.Errorf("got %q, %v", got, err)
	}
}

type mapVault map[string]string

func (m mapVault) Fetch(_ context.Context, key string) (string, error) {
	if v, ok := m[key]; ok {
		return v, nil
	}
	return "", errors.New("not in vault")
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func countingSource(n *atomic.Int32, err error) Source {
	return SourceFunc(func(context.Context) (string, error) {
		i := n.Add(1)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("v%d", i), nil
	})
}

func TestResolverCacheTTL(t *testing.T) {
	ctx := context.Background()
	clk := &clock{t: time.Unix(0, 0)}
	var n atomic.Int32
	r := NewResolver(Options{
		Sources: map[string]Source{"db": countingSource(&n, nil)},
		TTL:     time.Minute,
		Now:     clk.now,
	})
	steps := []struct {
		advance time.Duration
		want    string
	}{
		{0, "v1"},
		{30 * time.Second, "v1"},
		{31 * time.Second, "v2"},
		{59 * time.Second, "v2"},
	}
	for i, s := range steps {
		clk.t = clk.t.Add(s.advance)
		v, err := r.Resolve(ctx, "db")
		if err != nil || v.Reveal() != s.want {
			t.Fatalf("step %d: %v, %v; want %s", i, v.Reveal(), err, s.want)
		}
		v.Zero() // must not corrupt the cache
	}
	r.Purge()
	if v, _ := r.Resolve(ctx, "db"); v.Reveal() != "v3" {
		t.Errorf("after Purge = %q", v.Reveal())
	}
}

func TestResolverNoCacheAndDefaults(t *testing.T) {
	var n atomic.Int32
	r := NewResolver(Options{Sources: map[string]Source{"a": countingSource(&n, nil)}, TTL: -1})
	for want := 1; want <= 3; want++ {
		v, _ := r.Resolve(context.Background(), "a")
		if v.Reveal() != fmt.Sprintf("v%d", want) {
			t.Errorf("got %q", v.Reveal())
		}
	}
	var m atomic.Int32
	d := NewResolver(Options{Sources: map[string]Source{"a": countingSource(&m, nil)}})
	_, _ = d.Resolve(context.Background(), "a")
	if v, _ := d.Resolve(context.Background(), "a"); v.Reveal() != "v1" {
		t.Errorf("default TTL did not cache: %q", v.Reveal())
	}
	if _, err := NewResolver(Options{}).Resolve(context.Background(), "x"); !errors.Is(err, ErrUnknown) {
		t.Errorf("empty resolver = %v", err)
	}
}

func TestResolverErrors(t *testing.T) {
	ctx := context.Background()
	var n atomic.Int32
	cause := errors.New("backend down")
	r := NewResolver(Options{Sources: map[string]Source{"bad": countingSource(&n, cause)}})
	_, err := r.Resolve(ctx, "bad")
	var se *Error
	if !errors.As(err, &se) || se.Name != "bad" || !errors.Is(err, cause) {
		t.Errorf("err = %v", err)
	}
	if err.Error() != `secret "bad": backend down` {
		t.Errorf("Error() = %q", err.Error())
	}
	if _, err := r.Resolve(ctx, "bad"); err == nil || n.Load() != 2 {
		t.Error("failures must not be cached")
	}
	if _, err := r.Resolve(ctx, "missing"); !errors.Is(err, ErrUnknown) {
		t.Errorf("missing err = %v", err)
	}
}

func TestResolverCopiesSources(t *testing.T) {
	src := map[string]Source{"a": Literal("1")}
	r := NewResolver(Options{Sources: src})
	src["b"] = Literal("2")
	if _, err := r.Resolve(context.Background(), "b"); !errors.Is(err, ErrUnknown) {
		t.Error("resolver aliased the caller's map")
	}
}

func TestRefs(t *testing.T) {
	ctx := context.Background()
	r := NewResolver(Options{Sources: map[string]Source{"db": Literal("pw")}, Prefix: "secret://"})
	tests := []struct {
		in     string
		name   string
		isRef  bool
		reveal string
	}{
		{"secret://db", "db", true, "pw"},
		{"plain", "", false, "plain"},
		{"secret://", "", false, "secret://"},
		{"secret://a b", "", false, "secret://a b"},
		{"x secret://db", "", false, "x secret://db"},
	}
	for _, tt := range tests {
		name, ok := r.ParseRef(tt.in)
		if name != tt.name || ok != tt.isRef {
			t.Errorf("ParseRef(%q) = %q, %v", tt.in, name, ok)
		}
		v, isRef, err := r.ResolveRef(ctx, tt.in)
		if err != nil || isRef != tt.isRef || v.Reveal() != tt.reveal {
			t.Errorf("ResolveRef(%q) = %q, %v, %v", tt.in, v.Reveal(), isRef, err)
		}
	}
	none := NewResolver(Options{Sources: map[string]Source{"db": Literal("pw")}})
	if _, ok := none.ParseRef("secret://db"); ok {
		t.Error("references enabled without a prefix")
	}
	if _, isRef, err := r.ResolveRef(ctx, "secret://nope"); !isRef || !errors.Is(err, ErrUnknown) {
		t.Errorf("unknown ref: %v, %v", isRef, err)
	}
}

func TestResolverConcurrent(t *testing.T) {
	r := NewResolver(Options{Sources: map[string]Source{"a": Literal("x")}})
	done := make(chan struct{})
	for range 8 {
		go func() {
			defer func() { done <- struct{}{} }()
			for range 100 {
				if v, err := r.Resolve(context.Background(), "a"); err != nil || v.Reveal() != "x" {
					t.Error("bad concurrent resolve")
					return
				}
			}
		}()
	}
	for range 8 {
		<-done
	}
}
