// Package wire encodes the cryptobox stream format: headers, chunk
// framing and the sentinel errors both report.
package wire

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	KeySize    = 32
	NonceSize  = 12
	SaltSize   = 16
	PrefixSize = NonceSize - 4
	MagicSize  = 6
	Version    = 1

	// MaxChunk bounds plaintext chunks so a forged length cannot force a
	// huge allocation on open.
	MaxChunk = 64 << 20

	// Argon2id cost ceilings: the header is unauthenticated, so a
	// crafted file must not demand unbounded memory or computation
	// before authentication; ReadPass rejects beyond these instead of
	// clamping.
	MaxArgonMemory  uint32 = 1 << 20 // KiB; 1 GiB
	MaxArgonTime    uint32 = 16
	MaxArgonThreads uint8  = 64

	kdfArgon2id    = 1
	passHeaderSize = MagicSize + 1 + 1 + 4 + 4 + 1 + SaltSize + PrefixSize
	rawHeaderSize  = MagicSize + 1 + PrefixSize
)

// The magic tags are fixed bytes of the stream format; changing them
// would make every existing stream unreadable.
const (
	passMagic = "CBXPAS"
	rawMagic  = "CBXRAW"
)

var (
	ErrAuth               = errors.New("cryptobox: authentication failed (wrong passphrase or corrupt data)")
	ErrBadFormat          = errors.New("cryptobox: bad stream format")
	ErrUnsupportedVersion = errors.New("cryptobox: unsupported stream version")
	ErrTruncated          = errors.New("cryptobox: stream truncated")
)

// Argon holds argon2id parameters; Memory is in KiB.
type Argon struct {
	Time    uint32
	Memory  uint32
	Threads uint8
}

// PassHeader is the header of a passphrase stream.
type PassHeader struct {
	Argon  Argon
	Salt   [SaltSize]byte
	Prefix [PrefixSize]byte
}

// IsPass reports whether b starts with the passphrase-stream magic.
func IsPass(b []byte) bool { return len(b) >= MagicSize && string(b[:MagicSize]) == passMagic }

// IsRaw reports whether b starts with the raw-key-stream magic.
func IsRaw(b []byte) bool { return len(b) >= MagicSize && string(b[:MagicSize]) == rawMagic }

// WritePass writes h.
func WritePass(w io.Writer, h PassHeader) error {
	b := make([]byte, 0, passHeaderSize)
	b = append(b, passMagic...)
	b = append(b, Version, kdfArgon2id)
	b = binary.BigEndian.AppendUint32(b, h.Argon.Time)
	b = binary.BigEndian.AppendUint32(b, h.Argon.Memory)
	b = append(b, h.Argon.Threads)
	b = append(b, h.Salt[:]...)
	b = append(b, h.Prefix[:]...)
	return writeHeader(w, b)
}

// ReadPass reads and validates a passphrase-stream header.
func ReadPass(r io.Reader) (PassHeader, error) {
	var h PassHeader
	raw, err := readHeader(r, passHeaderSize, passMagic)
	if err != nil {
		return h, err
	}
	if kdf := raw[0]; kdf != kdfArgon2id {
		return h, fmt.Errorf("%w: unknown kdf %d", ErrBadFormat, kdf)
	}
	h.Argon = Argon{
		Time:    binary.BigEndian.Uint32(raw[1:5]),
		Memory:  binary.BigEndian.Uint32(raw[5:9]),
		Threads: raw[9],
	}
	if h.Argon.Time == 0 || h.Argon.Memory == 0 || h.Argon.Threads == 0 {
		return h, fmt.Errorf("%w: invalid argon params", ErrBadFormat)
	}
	switch {
	case h.Argon.Memory > MaxArgonMemory:
		return h, fmt.Errorf("%w: argon memory %d KiB exceeds the %d KiB limit", ErrBadFormat, h.Argon.Memory, MaxArgonMemory)
	case h.Argon.Time > MaxArgonTime:
		return h, fmt.Errorf("%w: argon time %d exceeds the limit %d", ErrBadFormat, h.Argon.Time, MaxArgonTime)
	case h.Argon.Threads > MaxArgonThreads:
		return h, fmt.Errorf("%w: argon threads %d exceeds the limit %d", ErrBadFormat, h.Argon.Threads, MaxArgonThreads)
	}
	copy(h.Salt[:], raw[10:10+SaltSize])
	copy(h.Prefix[:], raw[10+SaltSize:])
	return h, nil
}

// WriteRaw writes a raw-key-stream header.
func WriteRaw(w io.Writer, prefix [PrefixSize]byte) error {
	b := make([]byte, 0, rawHeaderSize)
	b = append(b, rawMagic...)
	b = append(b, Version)
	b = append(b, prefix[:]...)
	return writeHeader(w, b)
}

// ReadRaw reads a raw-key-stream header and returns its nonce prefix.
func ReadRaw(r io.Reader) ([PrefixSize]byte, error) {
	var prefix [PrefixSize]byte
	raw, err := readHeader(r, rawHeaderSize, rawMagic)
	if err != nil {
		return prefix, err
	}
	copy(prefix[:], raw)
	return prefix, nil
}

func writeHeader(w io.Writer, b []byte) error {
	if _, err := w.Write(b); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	return nil
}

// readHeader reads size bytes, checks magic and version, and returns the
// bytes after the version.
func readHeader(r io.Reader, size int, magic string) ([]byte, error) {
	raw := make([]byte, size)
	if _, err := io.ReadFull(r, raw); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, fmt.Errorf("%w: short header", ErrBadFormat)
		}
		return nil, fmt.Errorf("read header: %w", err)
	}
	if string(raw[:MagicSize]) != magic {
		return nil, fmt.Errorf("%w: bad magic", ErrBadFormat)
	}
	if v := raw[MagicSize]; v != Version {
		return nil, fmt.Errorf("%w: version %d", ErrUnsupportedVersion, v)
	}
	return raw[MagicSize+1:], nil
}
