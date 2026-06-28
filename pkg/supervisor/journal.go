package supervisor

import (
	"context"
	"strconv"
)

// JournalTail returns the last lines of the systemd user journal for unit.
// It lives here, not in the linux-only backend, so callers holding only a
// Supervisor stay portable; any failure yields "".
func JournalTail(ctx context.Context, exec CmdExecutor, unit string, lines int) string {
	if exec == nil || unit == "" || lines <= 0 {
		return ""
	}
	out, _, err := exec.Run(ctx, "journalctl", "--user", "-u", unit, "--no-pager", "-n", strconv.Itoa(lines))
	if err != nil {
		return ""
	}
	return string(out)
}
