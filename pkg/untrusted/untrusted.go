// Package untrusted fences untrusted text inside prompts with delimiter
// lines that carry a random nonce, so the content cannot forge its own
// closing delimiter.
package untrusted

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const (
	delimiterPrefix = "---UNTRUSTED-CONTENT-"
	delimiterSuffix = "---"
)

// Validation failures returned by Validate.
var (
	ErrNoDelimiters     = errors.New("untrusted: no delimiters found")
	ErrSingleDelimiter  = errors.New("untrusted: only one delimiter found")
	ErrMismatchedNonces = errors.New("untrusted: delimiter nonces do not match")
	ErrExtraDelimiters  = errors.New("untrusted: more than two delimiters found")
)

// Fence wraps content between two delimiter lines sharing a 128-bit
// random nonce.
func Fence(content string) (string, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("untrusted: generate nonce: %w", err)
	}
	delimiter := delimiterPrefix + hex.EncodeToString(nonce) + delimiterSuffix
	return delimiter + "\n" + content + "\n" + delimiter, nil
}

// Validate reports whether fenced holds exactly two delimiter lines with
// matching nonces.
func Validate(fenced string) error {
	var nonces []string
	for line := range strings.SplitSeq(fenced, "\n") {
		if strings.HasPrefix(line, delimiterPrefix) && strings.HasSuffix(line, delimiterSuffix) {
			nonces = append(nonces, strings.TrimSuffix(strings.TrimPrefix(line, delimiterPrefix), delimiterSuffix))
		}
	}
	switch {
	case len(nonces) == 0:
		return ErrNoDelimiters
	case len(nonces) == 1:
		return ErrSingleDelimiter
	case len(nonces) > 2:
		return ErrExtraDelimiters
	case nonces[0] != nonces[1]:
		return ErrMismatchedNonces
	}
	return nil
}
