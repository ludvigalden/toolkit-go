// Package exitcode names the process exit codes every tool shares. The
// values follow BSD sysexits(3), so scripts and supervisors can tell a
// usage error from a missing input or a transient failure.
package exitcode

import "strconv"

// Code is one process exit status.
type Code int

// The shared exit codes; 64-78 are the sysexits(3) values.
const (
	// OK is success.
	OK Code = 0
	// Failure is an unclassified failure.
	Failure Code = 1
	// Usage is a bad flag or argument.
	Usage Code = 2
	// DataErr is malformed input data.
	DataErr Code = 65
	// NoInput is a missing input file or resource.
	NoInput Code = 66
	// Unavailable is a required service or tool being unavailable.
	Unavailable Code = 69
	// Software is an internal software error.
	Software Code = 70
	// CantCreate is an output that could not be created.
	CantCreate Code = 73
	// TempFail is a transient failure; retrying may succeed.
	TempFail Code = 75
	// NoPerm is insufficient permission.
	NoPerm Code = 77
	// Config is a configuration error.
	Config Code = 78
	// Interrupted is termination by SIGINT (128 + 2).
	Interrupted Code = 130
)

// String names the code ("usage", "tempfail"); unknown codes render as
// "exit <n>".
func (c Code) String() string {
	switch c {
	case OK:
		return "ok"
	case Failure:
		return "failure"
	case Usage:
		return "usage"
	case DataErr:
		return "dataerr"
	case NoInput:
		return "noinput"
	case Unavailable:
		return "unavailable"
	case Software:
		return "software"
	case CantCreate:
		return "cantcreate"
	case TempFail:
		return "tempfail"
	case NoPerm:
		return "noperm"
	case Config:
		return "config"
	case Interrupted:
		return "interrupted"
	}
	return "exit " + strconv.Itoa(int(c))
}
