// Package gh runs the gh CLI non-interactively and decodes its release
// JSON. A missing binary is ErrNotFound so callers can map it to their
// own unavailability error.
package gh

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ErrNotFound means gh is not on PATH.
var ErrNotFound = errors.New("gh: not found on PATH")

// Exec runs bin (or "gh" from PATH when empty) with prompts and color
// disabled and returns stdout. Failures carry gh's stderr.
func Exec(ctx context.Context, bin string, args ...string) ([]byte, error) {
	if bin == "" {
		p, err := exec.LookPath("gh")
		if err != nil {
			return nil, ErrNotFound
		}
		bin = p
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(cmd.Environ(), "GH_PROMPT_DISABLED=1", "NO_COLOR=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("gh %s: %s: %w", strings.Join(args[:min(2, len(args))], " "), msg, err)
	}
	return stdout.Bytes(), nil
}

// ViewFields are the --json fields of `gh release view`.
const ViewFields = "tagName,name,body,publishedAt,url,isPrerelease,isDraft"

// ListFields are the --json fields of `gh release list`, which has no
// body.
const ListFields = "tagName,name,publishedAt,isPrerelease,isDraft,isLatest"

// Release is gh's release JSON.
type Release struct {
	TagName      string    `json:"tagName"`
	Name         string    `json:"name"`
	Body         string    `json:"body"`
	PublishedAt  time.Time `json:"publishedAt"`
	URL          string    `json:"url"`
	IsPrerelease bool      `json:"isPrerelease"`
	IsDraft      bool      `json:"isDraft"`
	IsLatest     bool      `json:"isLatest"`
}

// DecodeRelease decodes `gh release view --json` output.
func DecodeRelease(b []byte) (Release, error) {
	var r Release
	if err := json.Unmarshal(b, &r); err != nil {
		return Release{}, fmt.Errorf("ghcli: decode release: %w", err)
	}
	return r, nil
}

// DecodeList decodes `gh release list --json` output.
func DecodeList(b []byte) ([]Release, error) {
	var rs []Release
	if err := json.Unmarshal(b, &rs); err != nil {
		return nil, fmt.Errorf("ghcli: decode release list: %w", err)
	}
	return rs, nil
}
