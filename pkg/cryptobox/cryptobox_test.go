package cryptobox

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"
)

// cheap keeps argon2id cost trivial; the default cost is for attackers.
var cheap = WithArgon2Params(1, 8, 1)

// Streams that pin the wire format: header bytes, chunk framing, and
// the argon2id derivation and key-wrap values the format depends on.
const (
	compatPlain = "compat-vector-compat-vector-compat-vector-"
	compatPass  = "4342585041530101000000010000000801ea9aa94401c35c094ce0aa8c156ac9e853e4072d5b49f0ab00000020b72c4f4e1978bcc63f2beb6e209a675198e87b9fcf756a395beeb975c13b5eb600000020775bf6d90507fe8d1f30d50310de6b5441bb05dfaa531cb838891ec56d7739bb0000001a6e9bda7b1ab2c966f9cd8fa84da97428f5e1eb6f1d9ff041f984"
	compatRaw   = "43425852415701102713a3277a04930000002007dd7147cb7301b512a9bdf3ae01e88822047895f1dc4f2e74ece3788ad8698f000000207abbb734ebfcf943d6adf4dc8adb36f88a529a0435fbc0dae554eb12dfcce7730000001ae7399554ab611417119f0a903a5944938b8112202c0be410aba6"
	compatKEK   = "a0ffce6c0a466c429bbd988fc6b7f53b477e4e479f385cd9057da03d27898c8f"
)

func testKey() [KeySize]byte {
	var k [KeySize]byte
	for i := range k {
		k[i] = byte(i)
	}
	return k
}

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestCompatVectors(t *testing.T) {
	var out bytes.Buffer
	if err := DecryptStream(&out, bytes.NewReader(unhex(t, compatPass)), []byte("correct horse")); err != nil {
		t.Fatalf("passphrase vector: %v", err)
	}
	if out.String() != compatPlain {
		t.Errorf("passphrase vector = %q", out.String())
	}
	out.Reset()
	if err := OpenWithKey(&out, bytes.NewReader(unhex(t, compatRaw)), testKey()); err != nil {
		t.Fatalf("raw vector: %v", err)
	}
	if out.String() != compatPlain {
		t.Errorf("raw vector = %q", out.String())
	}
	kek := DeriveKEK([]byte("pw"), bytes.Repeat([]byte{7}, SaltSize), cheap)
	if hex.EncodeToString(kek[:]) != compatKEK {
		t.Errorf("DeriveKEK = %x", kek)
	}
}

type (
	sealFn func(w io.Writer, r io.Reader, chunk int) error
	openFn func(w io.Writer, r io.Reader) error
)

func modes() map[string]struct {
	seal sealFn
	open openFn
	bad  openFn
} {
	key, other := testKey(), testKey()
	other[0] ^= 1
	return map[string]struct {
		seal sealFn
		open openFn
		bad  openFn
	}{
		"passphrase": {
			seal: func(w io.Writer, r io.Reader, c int) error {
				return EncryptStream(w, r, []byte("pw"), WithChunkSize(c), cheap)
			},
			open: func(w io.Writer, r io.Reader) error { return DecryptStream(w, r, []byte("pw")) },
			bad:  func(w io.Writer, r io.Reader) error { return DecryptStream(w, r, []byte("wrong")) },
		},
		"rawkey": {
			seal: func(w io.Writer, r io.Reader, c int) error { return SealWithKey(w, r, key, WithChunkSize(c)) },
			open: func(w io.Writer, r io.Reader) error { return OpenWithKey(w, r, key) },
			bad:  func(w io.Writer, r io.Reader) error { return OpenWithKey(w, r, other) },
		},
	}
}

func TestRoundTrip(t *testing.T) {
	sizes := []int{0, 1, 15, 16, 17, 64, 100}
	for name, m := range modes() {
		for _, n := range sizes {
			plain := bytes.Repeat([]byte{'a'}, n)
			var sealed, out bytes.Buffer
			if err := m.seal(&sealed, bytes.NewReader(plain), 16); err != nil {
				t.Fatalf("%s/%d seal: %v", name, n, err)
			}
			if err := m.open(&out, &sealed); err != nil {
				t.Fatalf("%s/%d open: %v", name, n, err)
			}
			if !bytes.Equal(out.Bytes(), plain) {
				t.Errorf("%s/%d: got %d bytes", name, n, out.Len())
			}
		}
	}
}

func TestFreshNonces(t *testing.T) {
	for name, m := range modes() {
		var a, b bytes.Buffer
		_ = m.seal(&a, strings.NewReader("same"), 16)
		_ = m.seal(&b, strings.NewReader("same"), 16)
		if bytes.Equal(a.Bytes(), b.Bytes()) {
			t.Errorf("%s: identical ciphertext for repeated seal", name)
		}
	}
}

// chunks splits a sealed stream into its header and length-prefixed chunks.
func chunks(t *testing.T, b []byte, hdr int) (head []byte, cs [][]byte) {
	t.Helper()
	head, rest := b[:hdr], b[hdr:]
	for len(rest) > 0 {
		n := 4 + int(binary.BigEndian.Uint32(rest))
		cs, rest = append(cs, rest[:n]), rest[n:]
	}
	return head, cs
}

func join(head []byte, cs ...[]byte) []byte {
	return bytes.Join(append([][]byte{head}, cs...), nil)
}

func TestRejectsTampering(t *testing.T) {
	hdr := map[string]int{"passphrase": 41, "rawkey": 15}
	for name, m := range modes() {
		var sealed bytes.Buffer
		if err := m.seal(&sealed, strings.NewReader(compatPlain), 16); err != nil {
			t.Fatal(err)
		}
		b := sealed.Bytes()
		head, cs := chunks(t, b, hdr[name])
		if len(cs) != 3 {
			t.Fatalf("%s: %d chunks", name, len(cs))
		}
		flipped := bytes.Clone(b)
		flipped[len(flipped)-1] ^= 1

		cases := []struct {
			desc   string
			stream []byte
			open   openFn
			want   error
		}{
			{"wrong secret", b, m.bad, ErrWrongPassphrase},
			{"flipped byte", flipped, m.open, ErrWrongPassphrase},
			{"reordered", join(head, cs[1], cs[0], cs[2]), m.open, ErrWrongPassphrase},
			{"final dropped", join(head, cs[0], cs[1]), m.open, ErrTruncated},
			{"middle dropped", join(head, cs[0], cs[2]), m.open, ErrWrongPassphrase},
			{"cut mid-chunk", b[:len(b)-5], m.open, ErrTruncated},
			{"cut mid-length", join(head, cs[0], cs[1][:2]), m.open, ErrTruncated},
			{"header only", head, m.open, ErrTruncated},
			{"short header", head[:4], m.open, ErrBadFormat},
			{"bad version", append(append(bytes.Clone(head[:6]), 9), b[7:]...), m.open, ErrUnsupportedVersion},
		}
		for _, c := range cases {
			if err := c.open(io.Discard, bytes.NewReader(c.stream)); !errors.Is(err, c.want) {
				t.Errorf("%s/%s: err = %v, want %v", name, c.desc, err, c.want)
			}
		}
	}
}

func TestModesDoNotCrossOpen(t *testing.T) {
	var raw, pass bytes.Buffer
	_ = SealWithKey(&raw, strings.NewReader("x"), testKey())
	_ = EncryptStream(&pass, strings.NewReader("x"), []byte("pw"), cheap)
	if err := DecryptStream(io.Discard, &raw, []byte("pw")); !errors.Is(err, ErrBadFormat) {
		t.Errorf("raw via DecryptStream: %v", err)
	}
	if err := OpenWithKey(io.Discard, &pass, testKey()); !errors.Is(err, ErrBadFormat) {
		t.Errorf("passphrase via OpenWithKey: %v", err)
	}
}

func TestStreamDetection(t *testing.T) {
	var raw, pass bytes.Buffer
	_ = SealWithKey(&raw, strings.NewReader("x"), testKey())
	_ = EncryptStream(&pass, strings.NewReader("x"), []byte("pw"), cheap)
	tests := []struct {
		name      string
		b         []byte
		pass, key bool
	}{
		{"passphrase", pass.Bytes(), true, false},
		{"rawkey", raw.Bytes(), false, true},
		{"plain", []byte("hello world"), false, false},
		{"short", pass.Bytes()[:MagicSize-1], false, false},
		{"empty", nil, false, false},
	}
	for _, tt := range tests {
		if got := IsCryptoboxStream(tt.b); got != tt.pass {
			t.Errorf("%s: IsCryptoboxStream = %v", tt.name, got)
		}
		if got := IsRawKeyStream(tt.b); got != tt.key {
			t.Errorf("%s: IsRawKeyStream = %v", tt.name, got)
		}
	}
}

func TestDecryptReadsCostFromHeader(t *testing.T) {
	var sealed, out bytes.Buffer
	if err := EncryptStream(&sealed, strings.NewReader("hi"), []byte("pw"), WithArgon2Params(2, 16, 2)); err != nil {
		t.Fatal(err)
	}
	if err := DecryptStream(&out, &sealed, []byte("pw")); err != nil || out.String() != "hi" {
		t.Fatalf("got %q, %v", out.String(), err)
	}
}

func TestWithChunkSizeClamps(t *testing.T) {
	for _, tc := range []struct{ in, want int }{{1, minChunkSize}, {1 << 30, maxChunkSize}, {4096, 4096}} {
		var o options
		WithChunkSize(tc.in)(&o)
		if o.chunkSize != tc.want {
			t.Errorf("WithChunkSize(%d) = %d", tc.in, o.chunkSize)
		}
	}
}
