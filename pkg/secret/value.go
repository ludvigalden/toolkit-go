// Package secret resolves named secrets from pluggable sources (env,
// a command, a vault) with a TTL cache, and wraps values so that
// printing, logging or JSON-encoding them shows "[redacted]".
package secret

import "log/slog"

const redacted = "[redacted]"

// Value holds a secret behind redacting String, GoString, MarshalJSON,
// MarshalText and LogValue methods; only Reveal returns it. A nil
// *Value is the empty secret.
type Value struct {
	b []byte
}

// NewValue wraps s.
func NewValue(s string) *Value { return &Value{b: []byte(s)} }

// Reveal returns the secret.
func (v *Value) Reveal() string {
	if v == nil {
		return ""
	}
	return string(v.b)
}

// Len returns the secret's length in bytes without revealing it.
func (v *Value) Len() int {
	if v == nil {
		return 0
	}
	return len(v.b)
}

// Zero overwrites the held bytes. Strings already returned by Reveal
// are unaffected; Zero limits how long this copy lives in memory.
func (v *Value) Zero() {
	if v == nil {
		return
	}
	clear(v.b)
}

// String returns "[redacted]".
func (v *Value) String() string { return redacted }

// GoString returns "[redacted]" for %#v.
func (v *Value) GoString() string { return redacted }

// MarshalJSON encodes "[redacted]".
func (v *Value) MarshalJSON() ([]byte, error) { return []byte(`"` + redacted + `"`), nil }

// MarshalText encodes "[redacted]".
func (v *Value) MarshalText() ([]byte, error) { return []byte(redacted), nil }

// LogValue renders "[redacted]" in slog output.
func (v *Value) LogValue() slog.Value { return slog.StringValue(redacted) }
