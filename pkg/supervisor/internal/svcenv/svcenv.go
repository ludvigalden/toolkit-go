// Package svcenv builds the environment frozen into unit files and plists.
package svcenv

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ludvigalden/toolkit-go/pkg/xdg"
)

// Capture returns HOME, the XDG base directories and a canonical PATH.
// Service managers do not inherit the caller's environment, so every key
// is emitted even when unset here; PATH never includes the ambient value.
func Capture(getenv func(string) string, exePath, systemPATH string) (map[string]string, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	home := getenv("HOME")
	if !filepath.IsAbs(home) {
		return nil, errors.New("supervisor: capture environment: $HOME is unset or relative")
	}
	dirs, err := xdg.Resolve(getenv)
	if err != nil {
		return nil, fmt.Errorf("supervisor: capture environment: %w", err)
	}
	env := map[string]string{
		"HOME":            filepath.Clean(home),
		"XDG_CACHE_HOME":  dirs.CacheHome,
		"XDG_CONFIG_HOME": dirs.ConfigHome,
		"XDG_DATA_HOME":   dirs.DataHome,
		"XDG_STATE_HOME":  dirs.StateHome,
		"PATH":            PATH(exePath, systemPATH),
	}
	for k, v := range env {
		if strings.ContainsAny(v, "\n\r") {
			return nil, fmt.Errorf("supervisor: capture environment: %s contains a newline", k)
		}
	}
	return env, nil
}

// PATH is the executable's directory followed by systemPATH.
func PATH(exePath, systemPATH string) string {
	if exePath == "" {
		return systemPATH
	}
	return filepath.Dir(exePath) + ":" + systemPATH
}
