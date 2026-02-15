package cli

import (
	"log/slog"

	"github.com/ludvigalden/toolkit-go/pkg/output"
)

// GlobalFlags are the flags every tool shares; embed it in the grammar
// root. Each maps to config key stdout_format and env var
// <PREFIX>_STDOUT_FORMAT (style); <PREFIX>_SESSION and
// <PREFIX>_DEBUG_LOG have no flag. Resolved values live in Global.
type GlobalFlags struct {
	StdoutFormat    string   `name:"stdout-format" help:"Format of the data payload on stdout (text, json, ndjson, yamlish, quiet, or a mode the tool registers); diagnostics always go to stderr." default:"text"`
	StdoutVerbosity string   `name:"stdout-verbosity" help:"Which diagnostics reach stderr: silent, quiet, error, warn, info, verbose, debug, trace (or 0-10). Default: warn."`
	Color           string   `help:"Color on stderr: auto, always, never." default:"auto" enum:"auto,always,never" group:"Advanced"`
	Config          []string `name:"config" help:"Extra config files, loaded in order above the discovered ones." type:"path" sep:"," group:"Advanced"`
	EnvFile         []string `name:"env-file" help:"Dotenv files whose <PREFIX>_* entries act as environment variables." type:"path" sep:"," group:"Advanced"`
	Profile         string   `help:"Config profile to layer from the user config directory." group:"Advanced"`
	Session         string   `help:"Session ID shared by related invocations." group:"Advanced"`
}

func (g *GlobalFlags) cliGlobals() *GlobalFlags { return g }

// Global holds the resolved values every tool shares. Tool-specific
// values stay in Run.Config under "<name>.".
type Global struct {
	AppName    string
	Profile    string
	Cwd        string // physical working directory
	ExeDir     string
	SessionID  string
	OutputMode output.Mode
	Verbosity  output.Verbosity
	LogLevel   slog.Level
	Debug      bool
	Dirs       Paths
}
