package cryptobox

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

var (
	// ErrNoPassphrase means neither the environment nor a prompt yielded
	// a non-empty passphrase.
	ErrNoPassphrase = errors.New("cryptobox: no passphrase available")
	// ErrPassphraseMismatch means the confirmation entry differed.
	ErrPassphraseMismatch = errors.New("cryptobox: passphrases do not match")
)

// Prompter asks the user for one secret line, showing label first.
type Prompter interface {
	Prompt(label string) ([]byte, error)
}

// PromptFunc adapts a function to Prompter.
type PromptFunc func(label string) ([]byte, error)

// Prompt calls f.
func (f PromptFunc) Prompt(label string) ([]byte, error) { return f(label) }

// PassphraseSource resolves a passphrase from EnvVar via Getenv, then from
// Prompter. A nil Getenv or empty EnvVar skips the environment; a nil
// Prompter makes the prompt step report ErrNoPassphrase.
type PassphraseSource struct {
	EnvVar   string
	Getenv   func(string) string
	Prompter Prompter
}

// Resolve returns the passphrase for opening a stream.
func (s PassphraseSource) Resolve() ([]byte, error) {
	if v, ok := s.fromEnv(); ok {
		return v, nil
	}
	return s.prompt("Passphrase: ")
}

// ResolveNew returns a passphrase for sealing. A prompted one must be
// entered twice to guard against typos; one from the environment is not.
func (s PassphraseSource) ResolveNew() ([]byte, error) {
	if v, ok := s.fromEnv(); ok {
		return v, nil
	}
	first, err := s.prompt("Passphrase: ")
	if err != nil {
		return nil, err
	}
	second, err := s.prompt("Confirm passphrase: ")
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(first, second) {
		return nil, ErrPassphraseMismatch
	}
	return first, nil
}

func (s PassphraseSource) fromEnv() ([]byte, bool) {
	if s.EnvVar == "" || s.Getenv == nil {
		return nil, false
	}
	v := s.Getenv(s.EnvVar)
	return []byte(v), v != ""
}

func (s PassphraseSource) prompt(label string) ([]byte, error) {
	if s.Prompter == nil {
		return nil, ErrNoPassphrase
	}
	p, err := s.Prompter.Prompt(label)
	if err != nil {
		return nil, err
	}
	if len(p) == 0 {
		return nil, ErrNoPassphrase
	}
	return p, nil
}

// LinePrompter writes labels to an output and reads one line per prompt,
// without echo when the input is a terminal.
type LinePrompter struct {
	out  io.Writer
	open func() (io.Reader, func(), error)
	buf  *bufio.Reader
}

// NewLinePrompter reads from in and writes labels to out. A nil in uses
// the controlling terminal (/dev/tty, else a terminal stdin) per prompt,
// so piped stdin stays free for data; a nil out uses os.Stderr.
func NewLinePrompter(in io.Reader, out io.Writer) *LinePrompter {
	if out == nil {
		out = os.Stderr
	}
	p := &LinePrompter{out: out}
	if in == nil {
		p.open = openTTY
	} else {
		p.open = func() (io.Reader, func(), error) { return in, func() {}, nil }
	}
	return p
}

// Prompt implements Prompter.
func (p *LinePrompter) Prompt(label string) ([]byte, error) {
	in, closeIn, err := p.open()
	if err != nil {
		return nil, err
	}
	defer closeIn()

	if _, err := fmt.Fprint(p.out, label); err != nil {
		return nil, fmt.Errorf("write prompt: %w", err)
	}
	if f, ok := in.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		pass, err := term.ReadPassword(int(f.Fd()))
		_, _ = fmt.Fprintln(p.out) // the echo-less read leaves the cursor on the label line
		if err != nil {
			return nil, fmt.Errorf("read passphrase: %w", err)
		}
		return pass, nil
	}
	return p.readLine(in)
}

// readLine keeps one buffered reader across prompts so a confirmation line
// already buffered by the first read is not lost.
func (p *LinePrompter) readLine(in io.Reader) ([]byte, error) {
	if p.buf == nil {
		p.buf = bufio.NewReader(in)
	}
	line, err := p.buf.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("read passphrase: %w", err)
	}
	// Strip one line terminator (a "\n", or "\r\n" from Windows tools)
	// so a passphrase genuinely ending in CR or LF survives the read
	// instead of being trimmed away with every trailing terminator.
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r")
	return []byte(line), nil
}

func openTTY() (io.Reader, func(), error) {
	if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
		return tty, func() { _ = tty.Close() }, nil
	}
	if term.IsTerminal(int(os.Stdin.Fd())) {
		return os.Stdin, func() {}, nil
	}
	return nil, nil, ErrNoPassphrase
}
