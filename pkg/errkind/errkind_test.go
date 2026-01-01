package errkind

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/ludvigalden/toolkit-go/pkg/exitcode"
)

func TestBuiltinMapping(t *testing.T) {
	tests := []struct {
		kind      Kind
		name      string
		exit      exitcode.Code
		retryable bool
	}{
		{InvalidInput, "invalid-input", exitcode.Usage, false},
		{InvalidConfig, "invalid-config", exitcode.Config, false},
		{NotFound, "not-found", exitcode.NoInput, false},
		{AlreadyExists, "already-exists", exitcode.CantCreate, false},
		{Conflict, "conflict", exitcode.Failure, false},
		{FailedPrecondition, "failed-precondition", exitcode.Failure, false},
		{PermissionDenied, "permission-denied", exitcode.NoPerm, false},
		{Unauthenticated, "unauthenticated", exitcode.NoPerm, false},
		{ResourceExhausted, "resource-exhausted", exitcode.TempFail, true},
		{Unavailable, "unavailable", exitcode.Unavailable, true},
		{Timeout, "timeout", exitcode.TempFail, false},
		{Cancelled, "cancelled", exitcode.Interrupted, false},
		{Unimplemented, "unimplemented", exitcode.Software, false},
		{Internal, "internal", exitcode.Software, false},
	}
	if got := len(Builtins()); got != len(tests) {
		t.Fatalf("len(Builtins()) = %d, want %d", got, len(tests))
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.kind.name != tt.name || tt.kind.exit != tt.exit || tt.kind.retryable != tt.retryable {
				t.Errorf("kind = %+v", tt.kind)
			}
			if tt.kind.Name() != tt.name || tt.kind.Exit() != tt.exit || tt.kind.Retryable() != tt.retryable {
				t.Errorf("accessors drifted for %s", tt.name)
			}
			if tt.kind.description == "" {
				t.Error("empty description")
			}
			if Builtins()[i] != tt.kind {
				t.Errorf("Builtins()[%d] = %v, want %v", i, Builtins()[i], tt.kind)
			}
			if k, ok := Lookup(tt.name); !ok || k != tt.kind {
				t.Errorf("Lookup(%q) = %v, %v", tt.name, k, ok)
			}
		})
	}
	if _, ok := Lookup("nope"); ok {
		t.Error("Lookup(nope) succeeded")
	}
}

func TestBuiltinsIsACopy(t *testing.T) {
	b := Builtins()
	b[0] = Kind{name: "mutated"}
	if Builtins()[0] != InvalidInput {
		t.Error("Builtins() shares its backing array")
	}
}

func TestErrorString(t *testing.T) {
	cause := errors.New("disk full")
	tests := []struct {
		name string
		err  *Error
		want string
	}{
		{"message and cause", Wrap(Internal, cause, "write cache"), "write cache: disk full"},
		{"message only", New(NotFound, "no such repo"), "no such repo"},
		{"cause only", Wrap(Internal, cause, ""), "disk full"},
		{"description fallback", &Error{Kind: Timeout}, Timeout.description},
		{"name fallback", &Error{Kind: Kind{name: "custom"}}, "custom"},
		{"newf", Newf(InvalidInput, "bad flag %q", "--x"), `bad flag "--x"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUnwrapAndIs(t *testing.T) {
	err := fmt.Errorf("outer: %w", Wrap(NotFound, io.EOF, "read"))
	if !errors.Is(err, io.EOF) {
		t.Error("cause not reachable through errors.Is")
	}
	if !errors.Is(err, NotFound) {
		t.Error("errors.Is(err, NotFound) = false")
	}
	if errors.Is(err, Internal) {
		t.Error("errors.Is(err, Internal) = true")
	}
}

func TestKindOfAndExitCode(t *testing.T) {
	custom := Kind{name: "quota", retryable: true, exit: exitcode.TempFail}
	tests := []struct {
		name      string
		err       error
		kind      Kind
		ok        bool
		exit      exitcode.Code
		retryable bool
	}{
		{"nil", nil, Kind{}, false, exitcode.OK, false},
		{"plain", errors.New("x"), Kind{}, false, exitcode.Failure, false},
		{"classified", New(InvalidConfig, "x"), InvalidConfig, true, exitcode.Config, false},
		{"wrapped", fmt.Errorf("ctx: %w", New(Unavailable, "x")), Unavailable, true, exitcode.Unavailable, true},
		{"outermost wins", Wrap(PermissionDenied, New(NotFound, "x"), "y"), PermissionDenied, true, exitcode.NoPerm, false},
		{"custom", New(custom, "x"), custom, true, exitcode.TempFail, true},
		{"canceled", fmt.Errorf("op: %w", context.Canceled), Cancelled, true, exitcode.Interrupted, false},
		{"deadline", context.DeadlineExceeded, Timeout, true, exitcode.TempFail, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k, ok := KindOf(tt.err)
			if k != tt.kind || ok != tt.ok {
				t.Errorf("KindOf = %v, %v; want %v, %v", k, ok, tt.kind, tt.ok)
			}
			if got := ExitCode(tt.err); got != tt.exit {
				t.Errorf("ExitCode = %v, want %v", got, tt.exit)
			}
			if got := Retryable(tt.err); got != tt.retryable {
				t.Errorf("Retryable = %v, want %v", got, tt.retryable)
			}
		})
	}
}

func TestDetailsAndCode(t *testing.T) {
	e := New(Conflict, "stale")
	e.Code = "lock.stale"
	e.Details = map[string]any{"pid": 42}
	var got *Error
	if !errors.As(fmt.Errorf("w: %w", e), &got) || got.Code != "lock.stale" || got.Details["pid"] != 42 {
		t.Errorf("errors.As lost fields: %+v", got)
	}
}
