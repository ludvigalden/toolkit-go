// Package cryptobox encrypts byte streams with AES-256-GCM in fixed-size
// chunks whose AAD binds the index and a final marker, so reordered or
// truncated streams fail to open. Keys are raw or argon2id-derived.
package cryptobox

import (
	"crypto/rand"
	"fmt"
	"io"

	"github.com/ludvigalden/toolkit-go/pkg/cryptobox/internal/wire"
	"golang.org/x/crypto/argon2"
)

const (
	// KeySize is the AES-256 key length in bytes.
	KeySize = wire.KeySize
	// SaltSize is the argon2id salt length in bytes.
	SaltSize = wire.SaltSize
	// MagicSize is how many leading bytes the Is*Stream checks inspect.
	MagicSize = wire.MagicSize
	// DefaultChunkSize is the plaintext chunk size (1 MiB).
	DefaultChunkSize = 1 << 20

	minChunkSize = 16
	maxChunkSize = wire.MaxChunk
)

// Default argon2id cost; memory is in KiB (64 MiB).
const (
	DefaultArgonTime    uint32 = 2
	DefaultArgonMemory  uint32 = 64 * 1024
	DefaultArgonThreads uint8  = 4
)

// Sentinel errors, matched with errors.Is.
var (
	// ErrWrongPassphrase means a chunk failed authentication: wrong
	// passphrase or key, or tampered data; GCM cannot tell them apart.
	ErrWrongPassphrase    = wire.ErrAuth
	ErrBadFormat          = wire.ErrBadFormat
	ErrUnsupportedVersion = wire.ErrUnsupportedVersion
	ErrTruncated          = wire.ErrTruncated
)

type options struct {
	chunkSize int
	argon     wire.Argon
}

// Option customizes sealing and key derivation.
type Option func(*options)

// WithChunkSize sets the plaintext chunk size, clamped to [16 B, 64 MiB].
func WithChunkSize(n int) Option {
	return func(o *options) { o.chunkSize = min(max(n, minChunkSize), maxChunkSize) }
}

// WithArgon2Params overrides the argon2id cost; memory is in KiB. Opening
// reads the cost from the stream header, so it needs no option.
func WithArgon2Params(time, memory uint32, threads uint8) Option {
	return func(o *options) { o.argon = wire.Argon{Time: time, Memory: memory, Threads: threads} }
}

func newOptions(opts []Option) options {
	o := options{
		chunkSize: DefaultChunkSize,
		argon:     wire.Argon{Time: DefaultArgonTime, Memory: DefaultArgonMemory, Threads: DefaultArgonThreads},
	}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

func deriveKey(passphrase, salt []byte, p wire.Argon) []byte {
	return argon2.IDKey(passphrase, salt, p.Time, p.Memory, p.Threads, KeySize)
}

// DeriveKEK derives a key-encryption key from passphrase and salt with
// argon2id at the default cost unless WithArgon2Params overrides it.
// Callers persist the salt and any non-default cost beside the wrapped key.
func DeriveKEK(passphrase, salt []byte, opts ...Option) [KeySize]byte {
	return [KeySize]byte(deriveKey(passphrase, salt, newOptions(opts).argon))
}

// EncryptStream seals r into w under a key derived from passphrase with a
// fresh random salt; the header records the salt and argon2id cost.
func EncryptStream(w io.Writer, r io.Reader, passphrase []byte, opts ...Option) error {
	o := newOptions(opts)
	h := wire.PassHeader{Argon: o.argon}
	if _, err := io.ReadFull(rand.Reader, h.Salt[:]); err != nil {
		return fmt.Errorf("generate salt: %w", err)
	}
	if _, err := io.ReadFull(rand.Reader, h.Prefix[:]); err != nil {
		return fmt.Errorf("generate nonce prefix: %w", err)
	}
	aead, err := wire.NewAEAD(deriveKey(passphrase, h.Salt[:], h.Argon))
	if err != nil {
		return err
	}
	if err := wire.WritePass(w, h); err != nil {
		return err
	}
	return wire.Seal(w, r, aead, h.Prefix, o.chunkSize)
}

// DecryptStream opens a passphrase stream from r into w. Plaintext of
// chunks that authenticated may reach w before a later failure is found.
func DecryptStream(w io.Writer, r io.Reader, passphrase []byte) error {
	h, err := wire.ReadPass(r)
	if err != nil {
		return err
	}
	aead, err := wire.NewAEAD(deriveKey(passphrase, h.Salt[:], h.Argon))
	if err != nil {
		return err
	}
	return wire.Open(w, r, aead, h.Prefix)
}

// SealWithKey seals r into w under key with no key derivation. A fresh
// random nonce prefix per call keeps repeated use of one key nonce-safe.
func SealWithKey(w io.Writer, r io.Reader, key [KeySize]byte, opts ...Option) error {
	o := newOptions(opts)
	aead, err := wire.NewAEAD(key[:])
	if err != nil {
		return err
	}
	var prefix [wire.PrefixSize]byte
	if _, err := io.ReadFull(rand.Reader, prefix[:]); err != nil {
		return fmt.Errorf("generate nonce prefix: %w", err)
	}
	if err := wire.WriteRaw(w, prefix); err != nil {
		return err
	}
	return wire.Seal(w, r, aead, prefix, o.chunkSize)
}

// OpenWithKey opens a raw-key stream from r into w; a wrong key reports
// ErrWrongPassphrase.
func OpenWithKey(w io.Writer, r io.Reader, key [KeySize]byte) error {
	prefix, err := wire.ReadRaw(r)
	if err != nil {
		return err
	}
	aead, err := wire.NewAEAD(key[:])
	if err != nil {
		return err
	}
	return wire.Open(w, r, aead, prefix)
}

// IsCryptoboxStream reports whether b starts like a passphrase stream.
func IsCryptoboxStream(b []byte) bool { return wire.IsPass(b) }

// IsRawKeyStream reports whether b starts like a raw-key stream.
func IsRawKeyStream(b []byte) bool { return wire.IsRaw(b) }
