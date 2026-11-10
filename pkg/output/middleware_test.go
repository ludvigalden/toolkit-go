package output

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// tagged prefixes every result with its tag, proving it wrapped the
// renderer of the selected mode.
type tagged struct {
	Renderer
	tag string
}

func (t tagged) Result(data any) { t.Renderer.Result(t.tag + ":" + data.(string)) }

func tagWith(tag string, seen *[]string) Middleware {
	return func(next Factory) Factory {
		return func(stdout, stderr io.Writer, args Args) (Renderer, error) {
			*seen = append(*seen, tag)
			r, err := next(stdout, stderr, args)
			if err != nil {
				return nil, err
			}
			return tagged{r, tag}, nil
		}
	}
}

func TestMiddlewareWrapsEveryMode(t *testing.T) {
	custom := WithRenderer("custom", TextRenderer)
	for _, m := range []Mode{ModeText, ModeJSON, ModeNDJSON, ModeYamlish, ModeQuiet, "custom"} {
		var seen []string
		var stdout bytes.Buffer
		o, err := New(&stdout, io.Discard, WithMode(m), custom, WithMiddleware(tagWith("t", &seen)))
		if err != nil {
			t.Fatal(m, err)
		}
		o.Result("x")
		_ = o.Close()
		if len(seen) != 1 {
			t.Errorf("%s: middleware built %d times", m, len(seen))
		}
		if m != ModeQuiet && !strings.Contains(stdout.String(), "t:x") {
			t.Errorf("%s: stdout = %q", m, stdout.String())
		}
	}
}

func TestMiddlewareFirstRegisteredIsOutermost(t *testing.T) {
	var seen []string
	var stdout bytes.Buffer
	o, err := New(&stdout, io.Discard, WithMiddleware(tagWith("a", &seen)), WithMiddleware(tagWith("b", &seen)))
	if err != nil {
		t.Fatal(err)
	}
	o.Result("x")
	if got := strings.Join(seen, ","); got != "a,b" {
		t.Errorf("build order = %s", got)
	}
	if got := stdout.String(); got != "b:a:x\n" {
		t.Errorf("stdout = %q", got)
	}
}

func TestMiddlewareFactoryErrorPropagates(t *testing.T) {
	boom := func(Factory) Factory {
		return func(io.Writer, io.Writer, Args) (Renderer, error) { return nil, errors.New("boom") }
	}
	if _, err := New(io.Discard, io.Discard, WithMiddleware(boom)); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err = %v", err)
	}
}
