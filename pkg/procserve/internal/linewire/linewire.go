// Package linewire reads bounded control lines and hands a connection
// over with any bytes buffered past the handshake still readable.
package linewire

import (
	"bufio"
	"fmt"
	"net"
)

// ReadLine returns one line without its terminator, failing once it
// exceeds max bytes. Read errors (io.EOF included) pass through as is.
func ReadLine(r *bufio.Reader, max int) ([]byte, error) {
	var line []byte
	for {
		chunk, isPrefix, err := r.ReadLine()
		if err != nil {
			return nil, err //nolint:wrapcheck // io.EOF must stay recognizable
		}
		line = append(line, chunk...)
		if len(line) > max {
			return nil, fmt.Errorf("procserve: control line exceeds %d bytes", max)
		}
		if !isPrefix {
			return line, nil
		}
	}
}

// bufConn is a net.Conn whose reads first drain bytes a bufio.Reader
// buffered past the control handshake.
type bufConn struct {
	net.Conn
	r *bufio.Reader
}

// Read implements io.Reader.
func (c *bufConn) Read(p []byte) (int, error) {
	return c.r.Read(p) //nolint:wrapcheck // net.Conn errors pass through
}

// Wrap returns conn itself when nothing was buffered.
func Wrap(conn net.Conn, r *bufio.Reader) net.Conn {
	if r.Buffered() == 0 {
		return conn
	}
	return &bufConn{Conn: conn, r: r}
}
