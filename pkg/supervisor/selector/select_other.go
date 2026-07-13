//go:build !linux && !darwin

package selector

import (
	"context"

	"github.com/ludvigalden/toolkit-go/pkg/supervisor"
)

// New returns supervisor.ErrUnsupportedPlatform: only Linux and macOS are
// supported.
func New(_ context.Context, exec supervisor.CmdExecutor, o Options) (*Selected, error) {
	if err := checkOptions(exec, o); err != nil {
		return nil, err
	}
	return nil, supervisor.ErrUnsupportedPlatform
}
