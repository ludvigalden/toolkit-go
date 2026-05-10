package procserve

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/ludvigalden/toolkit-go/pkg/procserve/internal/linewire"
)

// Ops of the control handshake that opens every connection.
const (
	// OpConnect asks to hand the connection to the server's Handler.
	OpConnect = "connect"
	// OpStatus asks for a status reply; the connection then closes.
	OpStatus = "status"
	// OpShutdown asks the server to drain within Message.Grace.
	OpShutdown = "shutdown"
	// OpReply answers any request.
	OpReply = "reply"
)

// Message is one control message. Requests carry Op and Identity (and
// Grace for shutdown); replies carry the server's state or an Error.
type Message struct {
	Op        string        `json:"op"`
	Identity  string        `json:"identity,omitempty"`
	Grace     time.Duration `json:"grace,omitempty"`
	PID       int           `json:"pid,omitempty"`
	InFlight  int           `json:"in_flight,omitempty"`
	IdleSince time.Time     `json:"idle_since,omitzero"`
	Draining  bool          `json:"draining,omitempty"`
	Error     string        `json:"error,omitempty"`
}

// Protocol frames control messages. Read must consume exactly one message
// so that bytes after it reach the Handler untouched.
type Protocol interface {
	Write(w io.Writer, m Message) error
	Read(r *bufio.Reader) (Message, error)
}

// JSONLines is the default Protocol: one JSON object per line.
type JSONLines struct{}

// maxLine bounds one control line.
const maxLine = 64 << 10

// Write implements Protocol.
func (JSONLines) Write(w io.Writer, m Message) error {
	b, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("procserve: encode %s: %w", m.Op, err)
	}
	if _, err := w.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("procserve: write %s: %w", m.Op, err)
	}
	return nil
}

// Read implements Protocol.
func (JSONLines) Read(r *bufio.Reader) (Message, error) {
	line, err := linewire.ReadLine(r, maxLine)
	if err != nil {
		return Message{}, err //nolint:wrapcheck // io.EOF must stay recognizable
	}
	var m Message
	if err := json.Unmarshal(line, &m); err != nil {
		return Message{}, fmt.Errorf("procserve: decode control line: %w", err)
	}
	return m, nil
}

// protocolOr returns p, or JSONLines when p is nil.
func protocolOr(p Protocol) Protocol {
	if p == nil {
		return JSONLines{}
	}
	return p
}
