package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/knadh/koanf/v2"

	"github.com/ludvigalden/toolkit-go/pkg/errkind"
	"github.com/ludvigalden/toolkit-go/pkg/exitcode"
	"github.com/ludvigalden/toolkit-go/pkg/output"
)

type root struct {
	GlobalFlags
	Show showCmd `cmd:"" help:"Show the resolved value."`
	Fail failCmd `cmd:"" help:"Fail with a classified error."`
}

type showCmd struct {
	Upper bool `help:"Upper-case the value."`
}

func (c *showCmd) Run(out *output.Output, k *koanf.Koanf, g *Global) error {
	v := k.String("demo.dir")
	if c.Upper || k.Bool("show.upper") {
		v = strings.ToUpper(v)
	}
	out.Result(map[string]any{"dir": v, "session": g.SessionID})
	return nil
}

type failCmd struct{}

func (failCmd) Run() error { return errkind.New(errkind.NotFound, "nothing here") }

// sandbox makes a home with a project below it and chdirs into the
// project; the returned env serves App.Getenv.
func sandbox(t *testing.T) (home, project string, env map[string]string) {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	project = filepath.Join(home, "src", "proj")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(project)
	return home, project, map[string]string{"HOME": home}
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func run(t *testing.T, env map[string]string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	app := &App{
		Name: "demo", Stdout: &out, Stderr: &errb,
		Defaults: map[string]any{"demo.dir": "default"},
		Getenv:   func(k string) string { return env[k] },
	}
	code = app.Main(&root{}, args)
	return code, out.String(), errb.String()
}

func dir(t *testing.T, stdout string) string {
	t.Helper()
	var doc struct {
		Value []struct{ Dir string } `json:"value"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil || len(doc.Value) != 1 {
		t.Fatalf("stdout %q: %v", stdout, err)
	}
	return doc.Value[0].Dir
}

func TestCascadePrecedence(t *testing.T) {
	home, project, env := sandbox(t)
	show := []string{"--stdout-format=json", "show"}

	_, out, _ := run(t, env, show...)
	if got := dir(t, out); got != "default" {
		t.Errorf("defaults: %q", got)
	}
	write(t, filepath.Join(home, ".config", "demo", "config.yaml"), "demo:\n  dir: user\n")
	if _, out, _ = run(t, env, show...); dir(t, out) != "user" {
		t.Errorf("user config: %q", dir(t, out))
	}
	write(t, filepath.Join(home, "demo.yaml"), "demo:\n  dir: home\n")
	if _, out, _ = run(t, env, show...); dir(t, out) != "home" {
		t.Errorf("home walk-up file: %q", dir(t, out))
	}
	write(t, filepath.Join(project, ".demo.yaml"), "demo:\n  dir: project\n")
	if _, out, _ = run(t, env, show...); dir(t, out) != "project" {
		t.Errorf("project file beats home file: %q", dir(t, out))
	}
	write(t, filepath.Join(home, ".config", "demo", ".env"), "DEMO_DIR=dotenv\n")
	if _, out, _ = run(t, env, show...); dir(t, out) != "dotenv" {
		t.Errorf("user .env: %q", dir(t, out))
	}
	env["DEMO_DIR"] = "env"
	if _, out, _ = run(t, env, show...); dir(t, out) != "env" {
		t.Errorf("process env beats dotenv: %q", dir(t, out))
	}
	extra := filepath.Join(project, "extra.yaml")
	write(t, extra, "demo:\n  dir: flagfile\n")
	if _, out, _ = run(t, env, "--stdout-format=json", "--config", extra, "show"); dir(t, out) != "env" {
		t.Errorf("env beats --config: %q", dir(t, out))
	}
	delete(env, "DEMO_DIR")
	if _, out, _ = run(t, env, "--stdout-format=json", "--config", extra, "show"); dir(t, out) != "dotenv" {
		t.Errorf("dotenv still beats --config: %q", dir(t, out))
	}
}

func TestOutsideHomeIgnoresParents(t *testing.T) {
	_, project, env := sandbox(t)
	write(t, filepath.Join(filepath.Dir(project), "demo.yaml"), "demo:\n  dir: parent\n")
	env["HOME"] = "/nonexistent-home"
	if _, out, _ := run(t, env, "--stdout-format=json", "show"); dir(t, out) != "default" {
		t.Errorf("outside $HOME walked up: %q", dir(t, out))
	}
}

func TestEnvFileAndGlobalsFromEnv(t *testing.T) {
	_, project, env := sandbox(t)
	ef := filepath.Join(project, "x.env")
	write(t, ef, "DEMO_DIR=fromfile\nDEMO_STDOUT_FORMAT=json\nDEMO_SESSION=abc\n")
	code, out, _ := run(t, env, "--env-file", ef, "show")
	if code != 0 || dir(t, out) != "fromfile" || !strings.Contains(out, `"session":"abc"`) {
		t.Errorf("code=%d out=%q", code, out)
	}
}

func TestCommandFlagKeyAndExplicitOnly(t *testing.T) {
	_, project, env := sandbox(t)
	write(t, filepath.Join(project, "demo.yaml"), "show:\n  upper: true\n")
	if _, out, _ := run(t, env, "--stdout-format=json", "show"); dir(t, out) != "DEFAULT" {
		t.Errorf("config show.upper ignored: %q", out)
	}
	env["DEMO_STDOUT_FORMAT"] = "json"
	if _, out, _ := run(t, env, "show"); !strings.HasPrefix(out, "{") {
		t.Errorf("unset flag default overrode env: %q", out)
	}
}

func TestExitCodes(t *testing.T) {
	_, _, env := sandbox(t)
	cases := []struct {
		args []string
		want exitcode.Code
	}{
		{[]string{"show"}, exitcode.OK},
		{[]string{"--help"}, exitcode.OK},
		{nil, exitcode.Usage},
		{[]string{"nope"}, exitcode.Usage},
		{[]string{"--stdout-verbosity=loud", "show"}, exitcode.Usage},
		{[]string{"fail"}, exitcode.NoInput},
	}
	for _, c := range cases {
		code, out, errOut := run(t, env, c.args...)
		if exitcode.Code(code) != c.want {
			t.Errorf("%v: code %d, want %d (stderr %q)", c.args, code, c.want, errOut)
		}
		if c.want != exitcode.OK && out != "" {
			t.Errorf("%v: failure wrote stdout %q", c.args, out)
		}
	}
	if code, _, errOut := run(t, env, "--stdout-verbosity=silent", "nope"); code != int(exitcode.Usage) || errOut != "" {
		t.Errorf("silent parse error: code %d stderr %q", code, errOut)
	}
	if code, _, errOut := run(t, env, "--stdout-verbosity=quiet", "nope"); code != int(exitcode.Usage) || !strings.Contains(errOut, "Usage: demo") {
		t.Errorf("quiet parse error hid usage: code %d stderr %q", code, errOut)
	}
	if code, _, errOut := run(t, env, "--stdout-format=quiet", "nope"); code != int(exitcode.Usage) || errOut == "" {
		t.Errorf("quiet format hid usage: code %d stderr %q", code, errOut)
	}
	env["DEMO_STDOUT_VERBOSITY"] = "0"
	if code, _, errOut := run(t, env, "nope"); code != int(exitcode.Usage) || errOut != "" {
		t.Errorf("silent from env: code %d stderr %q", code, errOut)
	}
	delete(env, "DEMO_STDOUT_VERBOSITY")
	if _, _, errOut := run(t, env, "--help"); !strings.Contains(errOut, "Usage: demo") {
		t.Errorf("help not on stderr: %q", errOut)
	}
}

func TestConfigErrorsExitConfig(t *testing.T) {
	_, project, env := sandbox(t)
	write(t, filepath.Join(project, "demo.yaml"), "demo: [unclosed\n")
	if code, _, _ := run(t, env, "show"); code != int(exitcode.Config) {
		t.Errorf("code = %d", code)
	}
	delete(env, "HOME")
	if code, _, _ := run(t, env, "show"); code != int(exitcode.Config) {
		t.Errorf("no HOME: code = %d", code)
	}
}

func TestDebugLog(t *testing.T) {
	_, project, env := sandbox(t)
	logPath := filepath.Join(project, "debug.ndjson")
	env["DEMO_DEBUG_LOG"] = logPath
	var out, errb bytes.Buffer
	app := &App{Name: "demo", Stdout: &out, Stderr: &errb, Getenv: func(k string) string { return env[k] }}
	r, code := app.Parse(&root{}, []string{"show"})
	if r == nil {
		t.Fatalf("parse: %d %s", code, errb.String())
	}
	r.Logger.Debug("deep detail", "n", 1)
	r.Out.Info("narration")
	if code := r.Exec(); code != 0 {
		t.Fatalf("exec = %d", code)
	}
	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte("deep detail")) || !bytes.Contains(b, []byte("narration")) || errb.Len() != 0 {
		t.Errorf("log=%s stderr=%q", b, errb.String())
	}
}

type markRenderer struct{ w io.Writer }

func (m markRenderer) Log(output.Record)            {}
func (m markRenderer) Event(string, map[string]any) {}
func (m markRenderer) Result(any)                   { _, _ = io.WriteString(m.w, "custom\n") }
func (m markRenderer) Close(output.Outcome) error   { return nil }

func TestRegisteredModeIsSelectable(t *testing.T) {
	_, _, env := sandbox(t)
	var out, errb bytes.Buffer
	app := &App{
		Name: "demo", Stdout: &out, Stderr: &errb,
		Defaults: map[string]any{"demo.dir": "default"},
		Getenv:   func(k string) string { return env[k] },
		OutputOptions: []output.Option{output.WithRenderer("custom",
			func(stdout, _ io.Writer, _ output.Args) (output.Renderer, error) { return markRenderer{stdout}, nil })},
	}
	if code := app.Main(&root{}, []string{"--stdout-format=custom", "show"}); code != 0 || out.String() != "custom\n" {
		t.Fatalf("registered mode: code %d stdout %q stderr %q", code, out.String(), errb.String())
	}
	if code, out, _ := run(t, env, "--stdout-format=nope", "show"); code == 0 || out != "" {
		t.Fatalf("unregistered mode: code %d stdout %q", code, out)
	}
}

func TestStdoutFormatExitCodes(t *testing.T) {
	_, project, env := sandbox(t)
	if code, _, errOut := run(t, env, "--stdout-format=nope", "show"); code != int(exitcode.Usage) {
		t.Fatalf("bad command-line format: code %d stderr %q", code, errOut)
	}
	write(t, filepath.Join(project, "demo.yaml"), "stdout_format: nope\n")
	if code, _, _ := run(t, env, "show"); code != int(exitcode.CantCreate) {
		t.Fatalf("bad format from config: code %d", code)
	}
}

type brokenClose struct{}

func (brokenClose) Log(output.Record)            {}
func (brokenClose) Event(string, map[string]any) {}
func (brokenClose) Result(any)                   {}
func (brokenClose) Close(output.Outcome) error   { return errors.New("sink exploded") }

func TestCloseFailureReportsAndExitsSoftware(t *testing.T) {
	_, _, env := sandbox(t)
	var out, errb bytes.Buffer
	app := &App{
		Name: "demo", Stdout: &out, Stderr: &errb,
		Getenv: func(k string) string { return env[k] },
		OutputOptions: []output.Option{output.WithRenderer("boom",
			func(stdout, _ io.Writer, _ output.Args) (output.Renderer, error) { return brokenClose{}, nil })},
	}
	if code := app.Main(&root{}, []string{"--stdout-format=boom", "show"}); code != int(exitcode.Software) {
		t.Fatalf("code = %d, want %d (stderr %q)", code, int(exitcode.Software), errb.String())
	}
	if !strings.Contains(errb.String(), "sink exploded") {
		t.Errorf("close error not reported on stderr: %q", errb.String())
	}
}

func TestStdoutFormatFallback(t *testing.T) {
	_, _, env := sandbox(t)
	env["STDOUT_FORMAT"] = "json"
	if _, out, _ := run(t, env, "show"); !strings.HasPrefix(out, `{"`) {
		t.Errorf("STDOUT_FORMAT fallback ignored: %q", out)
	}
	env["DEMO_STDOUT_FORMAT"] = "text"
	if _, out, _ := run(t, env, "show"); strings.HasPrefix(out, `{"`) {
		t.Errorf("prefixed variable lost to the fallback: %q", out)
	}
	delete(env, "DEMO_STDOUT_FORMAT")
	var out bytes.Buffer
	app := &App{
		Name: "demo", Stdout: &out, Stderr: io.Discard,
		Getenv:       func(k string) string { return env[k] },
		EnvFallbacks: map[string][]string{"stdout_format": nil, "session": {"SHARED_SESSION"}},
	}
	env["SHARED_SESSION"] = "s1"
	if code := app.Main(&root{}, []string{"show"}); code != 0 || strings.HasPrefix(out.String(), `{"`) ||
		!strings.Contains(out.String(), "s1") {
		t.Errorf("tool fallbacks: code %d stdout %q", code, out.String())
	}
}

type clashRoot struct {
	GlobalFlags
	Deploy deployCmd `cmd:"" help:"A command with its own --profile and --session."`
}

type deployCmd struct {
	Profile string `help:"Deployment profile."`
	Session string `help:"Remote session."`
}

func (c *deployCmd) Run(out *output.Output, k *koanf.Koanf, g *Global) error {
	out.Result(map[string]any{
		"profile": c.Profile, "session": c.Session,
		"key": k.String("deploy.profile"), "global": g.SessionID, "global_profile": g.Profile,
	})
	return nil
}

func TestRootFlagsLeaveCommonNamesFree(t *testing.T) {
	home, _, env := sandbox(t)
	write(t, filepath.Join(home, ".config", "demo", "profiles", "ci.yaml"), "x: 1\n")
	var out, errb bytes.Buffer
	app := &App{Name: "demo", Stdout: &out, Stderr: &errb, Getenv: func(k string) string { return env[k] }}
	code := app.Main(&clashRoot{}, []string{
		"--stdout-format=json", "--session-id=s9", "--config-profile=ci",
		"deploy", "--profile=prod", "--session=remote",
	})
	if code != 0 {
		t.Fatalf("code %d stderr %q", code, errb.String())
	}
	for _, want := range []string{
		`"profile":"prod"`, `"session":"remote"`, `"key":"prod"`,
		`"global":"s9"`, `"global_profile":"ci"`,
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("stdout %q lacks %s", out.String(), want)
		}
	}
}

type tempRoot struct {
	GlobalFlags
	Touch touchCmd `cmd:"" help:"Write a file under Dirs.Temp."`
}

type touchCmd struct{}

func (touchCmd) Run(g *Global) error {
	if err := os.MkdirAll(g.Dirs.Temp, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(g.Dirs.Temp, "f"), nil, 0o600)
}

func TestTempDir(t *testing.T) {
	_, project, env := sandbox(t)
	env["TMPDIR"] = filepath.Join(project, "tmp")
	app := &App{Name: "demo", Stdout: io.Discard, Stderr: io.Discard, Getenv: func(k string) string { return env[k] }}
	r, code := app.Parse(&tempRoot{}, []string{"--session-id=s1", "touch"})
	if r == nil {
		t.Fatalf("parse: %d", code)
	}
	want := filepath.Join(project, "tmp", "demo", "s1")
	if r.Global.Dirs.Temp != want {
		t.Fatalf("Temp = %q, want %q", r.Global.Dirs.Temp, want)
	}
	if _, err := os.Stat(want); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Temp created eagerly: %v", err)
	}
	if code := r.Exec(); code != 0 {
		t.Fatalf("exec = %d", code)
	}
	if _, err := os.Stat(want); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Temp survived Close: %v", err)
	}
	for _, bad := range []string{"..", "a/b"} {
		if r, code := app.Parse(&tempRoot{}, []string{"--session-id=" + bad, "touch"}); r != nil || code != int(exitcode.Usage) {
			t.Errorf("session %q: code %d, want Usage", bad, code)
		}
	}
	delete(env, "TMPDIR")
	if r, _ := app.Parse(&tempRoot{}, []string{"touch"}); r == nil || filepath.Dir(filepath.Dir(r.Global.Dirs.Temp)) != filepath.Clean(os.TempDir()) {
		t.Errorf("no TMPDIR: Temp not under os.TempDir()")
	} else {
		_ = r.Close()
	}
}

type nestedRoot struct {
	GlobalFlags
	Remote struct {
		Add showCmd `cmd:"" help:"Add a remote."`
	} `cmd:"" help:"Manage remotes."`
	Show showCmd `cmd:"" help:"Show."`
}

func TestCommand(t *testing.T) {
	_, _, env := sandbox(t)
	app := &App{Name: "demo", Stdout: io.Discard, Stderr: io.Discard, Getenv: func(k string) string { return env[k] }}
	for args, want := range map[string]string{"remote add": "remote add", "show --upper": "show"} {
		r, code := app.Parse(&nestedRoot{}, strings.Fields(args))
		if r == nil {
			t.Fatalf("%q: parse %d", args, code)
		}
		if got := strings.Join(r.Command(), " "); got != want {
			t.Errorf("%q: Command() = %q, want %q", args, got, want)
		}
		_ = r.Close()
	}
}
