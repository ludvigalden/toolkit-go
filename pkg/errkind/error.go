package errkind

import (
	"context"
	"errors"
	"fmt"

	"github.com/ludvigalden/toolkit-go/pkg/exitcode"
)

// Error is a classified error. Code is an optional stable,
// machine-readable identifier finer than the kind; Details carries
// structured context for JSON output.
type Error struct {
	Kind    Kind
	Code    string
	Message string
	Cause   error
	Details map[string]any
}

// New returns an Error of kind with message msg.
func New(kind Kind, msg string) *Error {
	return &Error{Kind: kind, Message: msg}
}

// Newf is New with a fmt-formatted message. Use Wrap, not %w, to keep a
// cause.
func Newf(kind Kind, format string, args ...any) *Error {
	return &Error{Kind: kind, Message: fmt.Sprintf(format, args...)}
}

// Wrap classifies err as kind, prefixing msg when it is non-empty.
func Wrap(kind Kind, err error, msg string) *Error {
	return &Error{Kind: kind, Message: msg, Cause: err}
}

// Error renders "message: cause", dropping whichever part is empty and
// falling back to the kind's description.
func (e *Error) Error() string {
	switch {
	case e.Message != "" && e.Cause != nil:
		return e.Message + ": " + e.Cause.Error()
	case e.Message != "":
		return e.Message
	case e.Cause != nil:
		return e.Cause.Error()
	case e.Kind.Description() != "":
		return e.Kind.Description()
	}
	return e.Kind.Name()
}

// Unwrap returns the cause.
func (e *Error) Unwrap() error { return e.Cause }

// Is reports whether target is this error's Kind, which makes
// errors.Is(err, errkind.NotFound) match.
func (e *Error) Is(target error) bool {
	k, ok := target.(Kind)
	return ok && k == e.Kind
}

// KindOf returns the kind of the outermost *Error in err's chain.
// Unclassified context.Canceled and context.DeadlineExceeded map to
// Cancelled and Timeout so cancellation needs no wrapping.
func KindOf(err error) (Kind, bool) {
	var e *Error
	switch {
	case err == nil:
		return Kind{}, false
	case errors.As(err, &e):
		return e.Kind, true
	case errors.Is(err, context.Canceled):
		return Cancelled, true
	case errors.Is(err, context.DeadlineExceeded):
		return Timeout, true
	}
	return Kind{}, false
}

// ExitCode returns the exit code for err: OK for nil, the kind's code
// when classified, Failure otherwise.
func ExitCode(err error) exitcode.Code {
	if err == nil {
		return exitcode.OK
	}
	if k, ok := KindOf(err); ok {
		return k.Exit()
	}
	return exitcode.Failure
}

// Retryable reports whether err's kind permits retrying.
func Retryable(err error) bool {
	k, ok := KindOf(err)
	return ok && k.Retryable()
}
