package cryptobox

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// fakePrompter answers prompts from a queue and records the labels.
type fakePrompter struct {
	answers []string
	labels  []string
	err     error
}

func (f *fakePrompter) Prompt(label string) ([]byte, error) {
	f.labels = append(f.labels, label)
	if f.err != nil {
		return nil, f.err
	}
	if len(f.answers) == 0 {
		return nil, nil
	}
	a := f.answers[0]
	f.answers = f.answers[1:]
	return []byte(a), nil
}

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestPassphraseSource(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name    string
		src     PassphraseSource
		answers []string
		perr    error
		newPass bool
		want    string
		wantErr error
		prompts int
	}{
		{"env wins", PassphraseSource{EnvVar: "PW", Getenv: env(map[string]string{"PW": "fromenv"})}, nil, nil, false, "fromenv", nil, 0},
		{"env wins on new", PassphraseSource{EnvVar: "PW", Getenv: env(map[string]string{"PW": "fromenv"})}, nil, nil, true, "fromenv", nil, 0},
		{"empty env prompts", PassphraseSource{EnvVar: "PW", Getenv: env(nil)}, []string{"typed"}, nil, false, "typed", nil, 1},
		{"no getenv prompts", PassphraseSource{EnvVar: "PW"}, []string{"typed"}, nil, false, "typed", nil, 1},
		{"confirmed", PassphraseSource{}, []string{"a", "a"}, nil, true, "a", nil, 2},
		{"mismatch", PassphraseSource{}, []string{"a", "b"}, nil, true, "", ErrPassphraseMismatch, 2},
		{"empty entry", PassphraseSource{}, []string{""}, nil, false, "", ErrNoPassphrase, 1},
		{"prompt error", PassphraseSource{}, nil, boom, false, "", boom, 1},
	}
	for _, tt := range tests {
		fp := &fakePrompter{answers: tt.answers, err: tt.perr}
		tt.src.Prompter = fp
		resolve := tt.src.Resolve
		if tt.newPass {
			resolve = tt.src.ResolveNew
		}
		got, err := resolve()
		if !errors.Is(err, tt.wantErr) || string(got) != tt.want {
			t.Errorf("%s: got %q, %v; want %q, %v", tt.name, got, err, tt.want, tt.wantErr)
		}
		if len(fp.labels) != tt.prompts {
			t.Errorf("%s: %d prompts, want %d", tt.name, len(fp.labels), tt.prompts)
		}
	}
}

func TestNoPrompter(t *testing.T) {
	if _, err := (PassphraseSource{}).Resolve(); !errors.Is(err, ErrNoPassphrase) {
		t.Errorf("err = %v", err)
	}
}

func TestPromptFunc(t *testing.T) {
	src := PassphraseSource{Prompter: PromptFunc(func(string) ([]byte, error) { return []byte("f"), nil })}
	if got, err := src.Resolve(); err != nil || string(got) != "f" {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestLinePrompterNonTerminal(t *testing.T) {
	var out bytes.Buffer
	src := PassphraseSource{Prompter: NewLinePrompter(strings.NewReader("s3cret\r\ns3cret\n"), &out)}
	got, err := src.ResolveNew()
	if err != nil || string(got) != "s3cret" {
		t.Fatalf("got %q, %v", got, err)
	}
	if out.String() != "Passphrase: Confirm passphrase: " {
		t.Errorf("prompt output = %q", out.String())
	}
}

func TestLinePrompterEOF(t *testing.T) {
	src := PassphraseSource{Prompter: NewLinePrompter(strings.NewReader(""), &bytes.Buffer{})}
	if _, err := src.Resolve(); !errors.Is(err, ErrNoPassphrase) {
		t.Errorf("err = %v", err)
	}
}

// A passphrase whose true final characters are CR keeps one after the
// line terminator is stripped: exactly one "\n" and one "\r" are
// removed, where trimming every trailing terminator would erase both.
func TestLinePrompterKeepsTrailingNewlines(t *testing.T) {
	var out bytes.Buffer
	p := NewLinePrompter(strings.NewReader("pw\r\r\n"), &out)
	got, err := p.Prompt("Passphrase: ")
	if err != nil || string(got) != "pw\r" {
		t.Fatalf("prompt = %q, %v; want %q", got, err, "pw\r")
	}
}

func TestPromptedPassphraseOpensStream(t *testing.T) {
	src := PassphraseSource{Prompter: &fakePrompter{answers: []string{"pw", "pw", "pw"}}}
	sealPW, err := src.ResolveNew()
	if err != nil {
		t.Fatal(err)
	}
	var sealed, out bytes.Buffer
	if err := EncryptStream(&sealed, strings.NewReader("data"), sealPW, cheap); err != nil {
		t.Fatal(err)
	}
	openPW, err := src.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if err := DecryptStream(&out, &sealed, openPW); err != nil || out.String() != "data" {
		t.Fatalf("got %q, %v", out.String(), err)
	}
}
