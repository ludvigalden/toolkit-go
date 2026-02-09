// Package output renders a command's results and diagnostics. Results
// are the data payload on stdout in the selected Mode; diagnostics go
// to stderr, gated by Verbosity. Extra modes plug in as Renderers.
package output
