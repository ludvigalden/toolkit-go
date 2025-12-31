// Package errkind classifies errors into a small, stable vocabulary of
// kinds. A kind tells callers whether retrying may help and which
// sysexits code a CLI should exit with, without string matching.
package errkind

import "github.com/ludvigalden/toolkit-go/pkg/exitcode"

// Kind is one error classification. Kinds compare by value, and a Kind
// is itself an error so errors.Is(err, errkind.NotFound) works. The
// type is closed: use a built-in kind, not a Kind value of your own.
type Kind struct {
	name        string        // kebab-case identifier, e.g. "not-found"
	retryable   bool          // a caller may retry the failed call
	exit        exitcode.Code // process exit code for a CLI
	description string        // one-line user-facing summary
}

// Error returns the kind's name, so a Kind can serve as an errors.Is
// target.
func (k Kind) Error() string { return k.name }

// String returns the kind's name.
func (k Kind) String() string { return k.name }

// Name returns the kind's kebab-case identifier.
func (k Kind) Name() string { return k.name }

// Retryable reports whether a caller may retry the failed call.
func (k Kind) Retryable() bool { return k.retryable }

// Exit returns the process exit code a CLI should use for the kind.
func (k Kind) Exit() exitcode.Code { return k.exit }

// Description returns the kind's one-line user-facing summary.
func (k Kind) Description() string { return k.description }

// The built-in kinds are the canonical values of the closed Kind type;
// their fields are unexported, so no importer can corrupt an errors.Is
// match. Declare nothing: use these.
var (
	InvalidInput       = Kind{"invalid-input", false, exitcode.Usage, "the caller supplied an input the tool rejects"}
	InvalidConfig      = Kind{"invalid-config", false, exitcode.Config, "a configuration value is missing or invalid"}
	NotFound           = Kind{"not-found", false, exitcode.NoInput, "a requested thing does not exist"}
	AlreadyExists      = Kind{"already-exists", false, exitcode.CantCreate, "a thing to create already exists"}
	Conflict           = Kind{"conflict", false, exitcode.Failure, "the operation conflicts with current state"}
	FailedPrecondition = Kind{"failed-precondition", false, exitcode.Failure, "a precondition of the operation does not hold"}
	PermissionDenied   = Kind{"permission-denied", false, exitcode.NoPerm, "the caller lacks permission for the operation"}
	Unauthenticated    = Kind{"unauthenticated", false, exitcode.NoPerm, "the caller is not authenticated"}
	ResourceExhausted  = Kind{"resource-exhausted", true, exitcode.TempFail, "a resource is exhausted; retrying after backoff may succeed"}
	Unavailable        = Kind{"unavailable", true, exitcode.Unavailable, "a dependency could not be started or reached"}
	Timeout            = Kind{"timeout", false, exitcode.TempFail, "the operation exceeded its deadline"}
	Cancelled          = Kind{"cancelled", false, exitcode.Interrupted, "the operation was cancelled before completing"}
	Unimplemented      = Kind{"unimplemented", false, exitcode.Software, "the operation is not implemented"}
	Internal           = Kind{"internal", false, exitcode.Software, "an internal error occurred"}
)

// Builtins returns the built-in kinds in declaration order. The slice
// is fresh on every call, so callers may modify it.
func Builtins() []Kind {
	return []Kind{
		InvalidInput, InvalidConfig, NotFound, AlreadyExists, Conflict,
		FailedPrecondition, PermissionDenied, Unauthenticated,
		ResourceExhausted, Unavailable, Timeout, Cancelled,
		Unimplemented, Internal,
	}
}

// Lookup returns the built-in kind with the given name.
func Lookup(name string) (Kind, bool) {
	for _, k := range Builtins() {
		if k.name == name {
			return k, true
		}
	}
	return Kind{}, false
}
