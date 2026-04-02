package proclock

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ludvigalden/toolkit-go/pkg/proclock/internal/liveness"
)

// schemaVersion versions the metadata; files from a newer writer are
// treated as live foreign owners and never stolen.
const schemaVersion = 1

// errFutureSchema marks metadata written by a newer proclock.
var errFutureSchema = errors.New("proclock: metadata schema is newer than supported")

// Owner is the metadata a lock holder publishes. Endpoint and Extra are
// caller-defined; the rest is filled at acquisition.
type Owner struct {
	Schema     int               `json:"schema"`
	Name       string            `json:"name"`
	Repo       string            `json:"repo,omitempty"`
	PID        int               `json:"pid"`
	Binary     string            `json:"binary,omitempty"`
	BootID     string            `json:"boot_id,omitempty"`
	StartedAt  time.Time         `json:"started_at"`
	Nonce      string            `json:"nonce"`
	Generation uint64            `json:"generation"`
	Endpoint   string            `json:"endpoint,omitempty"`
	Extra      map[string]string `json:"extra,omitempty"`
}

// Starting reports an owner that holds the lock but has not published an
// endpoint yet; contenders should wait rather than connect.
func (o Owner) Starting() bool { return o.Endpoint == "" }

// encode renders the metadata file body.
func (o Owner) encode() ([]byte, error) {
	b, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("proclock: encode metadata: %w", err)
	}
	return append(b, '\n'), nil
}

// decodeOwner parses a metadata file body; a newer schema returns the
// parsed owner with errFutureSchema.
func decodeOwner(raw []byte) (Owner, error) {
	var o Owner
	if err := json.Unmarshal(bytes.TrimSpace(raw), &o); err != nil {
		return Owner{}, fmt.Errorf("proclock: decode metadata: %w", err)
	}
	if o.Schema > schemaVersion {
		return o, errFutureSchema
	}
	return o, nil
}

// alive reports a recorded PID that is running in this boot.
func alive(o Owner) bool { return liveness.Alive(o.PID, o.BootID) }

// aliveAndOurs adds an executable check against PID reuse.
func aliveAndOurs(o Owner) bool { return liveness.AliveAndOurs(o.PID, o.BootID, o.Binary) }

// deadBody reports a lock body whose owner is gone; unparseable bodies
// count as dead, newer schemas never do.
func deadBody(raw []byte) bool {
	o, err := decodeOwner(raw)
	if errors.Is(err, errFutureSchema) {
		return false
	}
	return err != nil || !aliveAndOurs(o)
}
