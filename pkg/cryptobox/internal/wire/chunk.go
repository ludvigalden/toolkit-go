package wire

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// NewAEAD returns AES-256-GCM for key.
func NewAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}
	return gcm, nil
}

// Seal writes r as length-prefixed chunks of at most chunkSize plaintext
// bytes. Each chunk's AAD binds its index and whether it is the last one.
func Seal(w io.Writer, r io.Reader, aead cipher.AEAD, prefix [PrefixSize]byte, chunkSize int) error {
	cur := make([]byte, chunkSize)
	next := make([]byte, chunkSize)
	var lenBuf [4]byte

	// Reading one chunk ahead lets the final marker land on the true last
	// chunk even when the input is an exact multiple of chunkSize.
	pending, err := readChunk(r, cur)
	if err != nil {
		return err
	}
	for index := uint32(0); ; {
		got, err := readChunk(r, next)
		if err != nil {
			return err
		}
		final := len(got) == 0

		sealed := aead.Seal(nil, nonce(prefix, index), pending, aad(index, final))
		binary.BigEndian.PutUint32(lenBuf[:], uint32(len(sealed)))
		if _, err := w.Write(lenBuf[:]); err != nil {
			return fmt.Errorf("write chunk length: %w", err)
		}
		if _, err := w.Write(sealed); err != nil {
			return fmt.Errorf("write chunk: %w", err)
		}
		if final {
			return nil
		}

		pending = got
		cur, next = next, cur
		index++
		if index == 0 {
			return errors.New("cryptobox: chunk counter overflow")
		}
	}
}

// Open authenticates and decrypts the chunk stream from r into w. A stream
// that ends before a final-marked chunk is ErrTruncated.
func Open(w io.Writer, r io.Reader, aead cipher.AEAD, prefix [PrefixSize]byte) error {
	overhead := uint32(aead.Overhead())
	var lenBuf [4]byte
	for index := uint32(0); ; {
		if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
			switch {
			case errors.Is(err, io.EOF):
				return ErrTruncated
			case errors.Is(err, io.ErrUnexpectedEOF):
				return fmt.Errorf("%w: short chunk length", ErrTruncated)
			}
			return fmt.Errorf("read chunk length: %w", err)
		}
		clen := binary.BigEndian.Uint32(lenBuf[:])
		if clen < overhead {
			return fmt.Errorf("%w: chunk shorter than auth tag", ErrBadFormat)
		}
		if clen > MaxChunk+overhead {
			return fmt.Errorf("%w: chunk length %d too large", ErrBadFormat, clen)
		}

		sealed := make([]byte, clen)
		if _, err := io.ReadFull(r, sealed); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return fmt.Errorf("%w: short chunk body", ErrTruncated)
			}
			return fmt.Errorf("read chunk body: %w", err)
		}

		plain, final, err := openChunk(aead, prefix, index, sealed)
		if err != nil {
			return err
		}
		if _, err := w.Write(plain); err != nil {
			return fmt.Errorf("write plaintext: %w", err)
		}
		if final {
			return nil
		}
		index++
		if index == 0 {
			return fmt.Errorf("%w: chunk counter overflow", ErrBadFormat)
		}
	}
}

// openChunk learns finality by trying both AAD markers; only the one used
// at seal time authenticates, so the marker itself stays authenticated.
func openChunk(aead cipher.AEAD, prefix [PrefixSize]byte, index uint32, sealed []byte) ([]byte, bool, error) {
	n := nonce(prefix, index)
	if p, err := aead.Open(nil, n, sealed, aad(index, false)); err == nil {
		return p, false, nil
	}
	if p, err := aead.Open(nil, n, sealed, aad(index, true)); err == nil {
		return p, true, nil
	}
	return nil, false, ErrAuth
}

// readChunk fills buf as far as r allows; an empty result means EOF.
func readChunk(r io.Reader, buf []byte) ([]byte, error) {
	n, err := io.ReadFull(r, buf)
	if err == nil || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return buf[:n], nil
	}
	return nil, fmt.Errorf("read plaintext: %w", err)
}

func nonce(prefix [PrefixSize]byte, index uint32) []byte {
	n := make([]byte, NonceSize)
	copy(n, prefix[:])
	binary.BigEndian.PutUint32(n[PrefixSize:], index)
	return n
}

func aad(index uint32, final bool) []byte {
	a := make([]byte, 5)
	binary.BigEndian.PutUint32(a, index)
	if final {
		a[4] = 1
	}
	return a
}
