// Package gsux wires go-selfupdate for selfupdate: it builds the
// updater with checksum validation by default, detects the release
// for this platform, and resolves the real file to replace.
package gsux

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	gsu "github.com/creativeprojects/go-selfupdate"
)

// ErrNoAsset means no release carries an asset for this OS and
// architecture.
var ErrNoAsset = errors.New("no release has an asset for this platform")

// NewUpdater builds the go-selfupdate updater. A nil validator means
// checksumsFile validation unless skip is set.
func NewUpdater(src gsu.Source, v gsu.Validator, skip, pre bool, checksumsFile string) (*gsu.Updater, error) {
	if v == nil && !skip {
		v = &gsu.ChecksumValidator{UniqueFilename: checksumsFile}
	}
	up, err := gsu.NewUpdater(gsu.Config{Source: src, Validator: v, Prerelease: pre})
	if err != nil {
		return nil, fmt.Errorf("selfupdate: %w", err)
	}
	return up, nil
}

// Detect returns the newest release of repo with an asset for this
// platform, or an error wrapping ErrNoAsset.
func Detect(ctx context.Context, up *gsu.Updater, repo string) (*gsu.Release, error) {
	rel, found, err := up.DetectLatest(ctx, gsu.ParseSlug(repo))
	if err != nil {
		return nil, fmt.Errorf("selfupdate: detect latest release of %s: %w", repo, err)
	}
	if !found {
		return nil, fmt.Errorf("selfupdate: %s: %w", repo, ErrNoAsset)
	}
	return rel, nil
}

// Executable returns exe, or the running binary when empty, with
// symlinks resolved so the real file is replaced.
func Executable(exe string) (string, error) {
	if exe == "" {
		var err error
		if exe, err = os.Executable(); err != nil {
			return "", fmt.Errorf("selfupdate: locate executable: %w", err)
		}
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe, nil
}
