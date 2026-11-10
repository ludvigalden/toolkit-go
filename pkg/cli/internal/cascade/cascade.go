// Package cascade loads a tool's layered configuration into a koanf
// tree: defaults, user config, profile, walk-up project files, extra
// files, environment (dotenv below the process), then explicit flags.
package cascade

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/knadh/koanf/parsers/json"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"

	"github.com/ludvigalden/toolkit-go/pkg/dotenv"
)

// Layers says where configuration comes from; Load reads them lowest
// priority first.
type Layers struct {
	Name, Prefix string
	Defaults     map[string]any
	UserDir      string // per-tool XDG config directory
	Profile      string
	Walk         []string // walk-up directories, home side first
	LocalNames   []string
	Files        []string // --config
	EnvFiles     []string // --env-file
	// Getenv reads the process environment LookupEnv-style: a variable
	// set to the empty string is present and wins over dotenv and
	// config layers. Nil means os.LookupEnv.
	Getenv func(string) (string, bool)
	// EnvFallbacks lists extra variables per config key, read in order
	// when the prefixed variable is absent.
	EnvFallbacks map[string][]string
	Flags        map[string]any
}

// Load builds the cascade: defaults, user config.yaml, profile, the
// walk-up files, --config files, the process environment (a variable
// set, even to the empty string, wins and clears lower layers), dotenv
// entries below it, then explicitly given flags.
func (l Layers) Load() (*koanf.Koanf, error) {
	k := koanf.New(".")
	if err := k.Load(confmap.Provider(l.Defaults, "."), nil); err != nil {
		return nil, fmt.Errorf("defaults: %w", err)
	}
	if err := loadOptional(k, filepath.Join(l.UserDir, "config.yaml")); err != nil {
		return nil, err
	}
	if l.Profile != "" {
		p := filepath.Join(l.UserDir, "profiles", l.Profile+".yaml")
		if _, err := os.Stat(p); err != nil {
			return nil, fmt.Errorf("profile %q: %w", l.Profile, err)
		}
		if err := loadFile(k, p); err != nil {
			return nil, err
		}
	}
	for _, dir := range l.Walk {
		for _, name := range l.LocalNames {
			if err := loadOptional(k, filepath.Join(dir, name)); err != nil {
				return nil, err
			}
		}
	}
	for _, p := range l.Files {
		if err := loadFile(k, p); err != nil {
			return nil, err
		}
	}
	lookup, err := l.envLookup()
	if err != nil {
		return nil, err
	}
	if err := k.Load(confmap.Provider(l.envValues(k.Keys(), lookup), "."), nil); err != nil {
		return nil, fmt.Errorf("environment: %w", err)
	}
	if err := k.Load(confmap.Provider(l.Flags, "."), nil); err != nil {
		return nil, fmt.Errorf("flags: %w", err)
	}
	return k, nil
}

// envLookup reads the process environment first — presence counts, so
// a set-but-empty variable wins — then the dotenv files (user .env,
// then --env-file in order, later files winning).
func (l Layers) envLookup() (func(string) (string, bool), error) {
	vars := map[string]string{}
	paths := append([]string{filepath.Join(l.UserDir, ".env")}, l.EnvFiles...)
	for i, p := range paths {
		entries, err := dotenv.ParseFile(p)
		if i == 0 && errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("env file: %w", err)
		}
		for _, e := range entries {
			vars[e.Key] = e.Value
		}
	}
	proc := l.Getenv
	if proc == nil {
		proc = os.LookupEnv
	}
	return func(key string) (string, bool) {
		if v, ok := proc(key); ok {
			return v, true
		}
		v, ok := vars[key]
		return v, ok
	}, nil
}

// envValues binds each known key to its variable: "<name>.dir" and
// "dir" both read <PREFIX>_DIR, so tool keys need no doubled prefix. A
// present variable always binds, so a set-but-empty one clears the
// config-layer value.
func (l Layers) envValues(keys []string, lookup func(string) (string, bool)) map[string]any {
	out := map[string]any{}
	ns := l.Name + "."
	for _, key := range keys {
		envKey := strings.TrimPrefix(key, ns)
		envKey = l.Prefix + strings.ToUpper(strings.NewReplacer(".", "_", "-", "_").Replace(envKey))
		for _, name := range append([]string{envKey}, l.EnvFallbacks[key]...) {
			if v, ok := lookup(name); ok {
				out[key] = v
				break
			}
		}
	}
	return out
}

func loadOptional(k *koanf.Koanf, path string) error {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil //nolint:nilerr // an absent optional file is not an error
		}
		return fmt.Errorf("config %s: %w", path, err)
	}
	return loadFile(k, path)
}

func loadFile(k *koanf.Koanf, path string) error {
	var parser koanf.Parser = yaml.Parser()
	if filepath.Ext(path) == ".json" {
		parser = json.Parser()
	}
	if err := k.Load(file.Provider(path), parser); err != nil {
		return fmt.Errorf("config %s: %w", path, err)
	}
	return nil
}

// PhysicalCwd is the working directory with symlinks resolved; a
// deleted working directory is an error.
func PhysicalCwd() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("working directory: %w", err)
	}
	phys, err := filepath.EvalSymlinks(wd)
	if err != nil {
		return "", fmt.Errorf("working directory: %w", err)
	}
	return phys, nil
}

// WalkUp lists dir and its ancestors up to home, home side first, so
// files nearer the working directory win. A directory outside home
// yields itself only: the walk never climbs toward /.
func WalkUp(dir, home string) []string {
	if home != "/" && dir != home && !strings.HasPrefix(dir, home+"/") {
		return []string{dir}
	}
	var out []string
	for d := dir; ; d = filepath.Dir(d) {
		out = append([]string{d}, out...)
		if d == home || d == "/" {
			return out
		}
	}
}

// HomeDir returns $HOME, which must be set and absolute for the walk.
func HomeDir(getenv func(string) string) (string, error) {
	h := getenv("HOME")
	if h == "" || !filepath.IsAbs(h) {
		return "", fmt.Errorf("$HOME must be an absolute path, got %q", h)
	}
	return filepath.Clean(h), nil
}

// WithBuiltins returns the shared global keys at their defaults,
// overlaid with the tool's own defaults.
func WithBuiltins(tool map[string]any) map[string]any {
	d := map[string]any{
		"stdout_format": "text", "stdout_verbosity": "warn", "color": "auto",
		"debug": false, "session": "", "debug_log": "",
	}
	for k, v := range tool {
		d[k] = v
	}
	return d
}

// FallbacksWithBuiltins returns the shared fallbacks (plain
// STDOUT_FORMAT for stdout_format) overlaid with the tool's: a tool
// entry replaces the built-in one for its key, and a nil entry drops it.
func FallbacksWithBuiltins(tool map[string][]string) map[string][]string {
	f := map[string][]string{"stdout_format": {"STDOUT_FORMAT"}}
	for k, v := range tool {
		f[k] = v
	}
	return f
}

// LocalNamesFor returns override, or "<name>.yaml" and ".<name>.yaml"
// when override is nil.
func LocalNamesFor(name string, override []string) []string {
	if override != nil {
		return override
	}
	return []string{name + ".yaml", "." + name + ".yaml"}
}

// Profile returns flag, or the <PREFIX>PROFILE variable when the flag
// is empty.
func Profile(flag, prefix string, getenv func(string) string) string {
	if flag != "" {
		return flag
	}
	return getenv(prefix + "PROFILE")
}
