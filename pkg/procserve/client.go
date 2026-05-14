package procserve

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ludvigalden/toolkit-go/pkg/procserve/internal/conns"
	"github.com/ludvigalden/toolkit-go/pkg/procserve/internal/inherit"
	"github.com/ludvigalden/toolkit-go/pkg/procserve/internal/linewire"
	"github.com/ludvigalden/toolkit-go/pkg/procserve/internal/lockfile"
	"github.com/ludvigalden/toolkit-go/pkg/procserve/internal/sockpath"
)

// Client connects to warm servers, spawning them on demand.
type Client struct {
	// Dir holds sockets and lock files, typically a per-user runtime
	// directory. Required.
	Dir string
	// FallbackDir replaces Dir when socket paths under Dir would exceed
	// the unix socket path limit (default /tmp/procserve-<uid>).
	FallbackDir string
	// Command builds the server process for an identity. procserve sets
	// ExtraFiles, Setsid, and an env marker; the caller sets the rest.
	Command func(identity string) *exec.Cmd
	// MaxIdle caps idle servers: before spawning, the least recently
	// idle server is asked to shut down once MaxIdle are idle (0: off).
	MaxIdle int
	// Protocol frames control messages (default JSONLines); it must
	// match the servers'.
	Protocol Protocol
	// SpawnTimeout bounds one Connect when ctx has no earlier deadline
	// (default 15s).
	SpawnTimeout time.Duration
}

// Status describes one live server.
type Status struct {
	Socket    string
	Hash      string
	PID       int
	InFlight  int
	IdleSince time.Time
	Draining  bool
}

// Timings of the connect-or-spawn protocol.
const (
	defaultSpawnTimeout = 15 * time.Second
	replyWait           = 5 * time.Second
	spawnPasses         = 3
	backoffCap          = 16 * time.Millisecond
	stopOvershoot       = 2 * time.Second
)

// errDraining marks a server that answered but is shutting down.
var errDraining = errors.New("procserve: server is draining")

// Connect returns a connection to the identity's server, spawning one if
// none answers. The handshake is complete: the next bytes belong to the
// server's Handler.
func (c *Client) Connect(ctx context.Context, identity string) (net.Conn, error) {
	if c.Command == nil {
		return nil, errors.New("procserve: Client.Command is required")
	}
	p, err := c.resolvePaths(identity)
	if err != nil {
		return nil, err
	}
	budget := c.SpawnTimeout
	if budget <= 0 {
		budget = defaultSpawnTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	hash := sockpath.Hash(identity)
	var last error
	for range spawnPasses {
		conn, retry, err := c.connectOnce(ctx, identity, hash, p)
		if err == nil {
			return conn, nil
		}
		last = err
		if !retry || ctx.Err() != nil {
			break
		}
	}
	return nil, fmt.Errorf("procserve: connect: %w", last)
}

// connectOnce dials, or takes the lock and publishes a new server. retry
// reports a failure another pass may overcome.
func (c *Client) connectOnce(ctx context.Context, identity, hash string, p sockpath.Paths) (net.Conn, bool, error) {
	lock, conn, err := c.dialOrLock(ctx, hash, p)
	if err != nil || conn != nil {
		return conn, err != nil && noServer(err), err
	}
	// Under the lock: a racer may have published since our last dial.
	if conn, _, err := c.exchange(ctx, p.Socket, Message{Op: OpConnect, Identity: hash}); err == nil {
		_ = lock.Close() //nolint:errcheck // releases the flock
		return conn, false, nil
	} else if !noServer(err) {
		_ = lock.Close() //nolint:errcheck // releases the flock
		return nil, false, err
	}
	if err := c.publish(ctx, identity, hash, p, lock); err != nil {
		return nil, true, err
	}
	conn, _, err = c.exchange(ctx, p.Socket, Message{Op: OpConnect, Identity: hash})
	return conn, err != nil && noServer(err), err
}

// dialOrLock dials until a server answers or the identity's lock is ours,
// backing off 1ms doubling to 16ms while another process holds it.
func (c *Client) dialOrLock(ctx context.Context, hash string, p sockpath.Paths) (*os.File, net.Conn, error) {
	delay := time.Millisecond
	for {
		conn, _, err := c.exchange(ctx, p.Socket, Message{Op: OpConnect, Identity: hash})
		if err == nil {
			return nil, conn, nil
		}
		if !noServer(err) {
			return nil, nil, err
		}
		lock, lerr := lockfile.TryLock(p.Lock)
		if lerr == nil {
			return lock, nil, nil
		}
		if !errors.Is(lerr, lockfile.ErrLocked) {
			return nil, nil, lerr
		}
		if err := lockfile.Sleep(ctx, delay); err != nil {
			return nil, nil, err //nolint:wrapcheck // context errors pass through
		}
		delay = min(2*delay, backoffCap)
	}
}

// publish binds a fresh listener at the temp name, renames it over the
// socket path, records the inode in the lock, and spawns the server with
// listener and lock inherited. Every parent copy is closed on return.
func (c *Client) publish(ctx context.Context, identity, hash string, p sockpath.Paths, lock *os.File) error {
	defer lock.Close() //nolint:errcheck // the child holds its own copy
	c.evictIdle(ctx)
	if err := os.Remove(p.Temp); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("procserve: stale temp socket: %w", err)
	}
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: p.Temp, Net: "unix"})
	if err != nil {
		return fmt.Errorf("procserve: bind: %w", err)
	}
	ln.SetUnlinkOnClose(false)
	defer ln.Close() //nolint:errcheck // the child holds its own copy
	// Rename publishes atomically and replaces a dead server's socket.
	if err := os.Rename(p.Temp, p.Socket); err != nil {
		return fmt.Errorf("procserve: publish socket: %w", err)
	}
	ino, err := sockpath.Inode(p.Socket)
	if err != nil {
		return fmt.Errorf("procserve: published socket: %w", err)
	}
	if err := lockfile.Write(lock, lockfile.Record{Hash: hash, Socket: p.Socket, Inode: ino}); err != nil {
		return err
	}
	return c.spawn(identity, ln, lock)
}

// spawn starts the detached server with the listener as fd 3 and the lock
// as fd 4, and reaps it in the background.
func (c *Client) spawn(identity string, ln *net.UnixListener, lock *os.File) error {
	cmd := c.Command(identity)
	if cmd == nil {
		return errors.New("procserve: Command returned nil")
	}
	return inherit.Spawn(cmd, ln, lock) //nolint:wrapcheck // errors carry the procserve prefix
}

// exchange dials socket and runs one request/reply. For OpConnect the
// returned conn stays open for the Handler; otherwise it is closed.
func (c *Client) exchange(ctx context.Context, socket string, req Message) (net.Conn, Message, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", socket)
	if err != nil {
		return nil, Message{}, err //nolint:wrapcheck // classified by noServer
	}
	deadline := time.Now().Add(replyWait)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	_ = conn.SetDeadline(deadline) //nolint:errcheck // a failed deadline surfaces as an I/O error
	proto := protocolOr(c.Protocol)
	br := bufio.NewReader(conn)
	reply, err := func() (Message, error) {
		if err := proto.Write(conn, req); err != nil {
			return Message{}, err
		}
		return proto.Read(br)
	}()
	switch {
	case err != nil:
	case reply.Error != "":
		err = fmt.Errorf("procserve: server: %s", reply.Error)
	case reply.Draining && req.Op == OpConnect:
		err = errDraining
	}
	if err != nil || req.Op != OpConnect {
		_ = conn.Close() //nolint:errcheck // one-shot exchange
		return nil, reply, err
	}
	_ = conn.SetDeadline(time.Time{}) //nolint:errcheck // handler owns deadlines now
	return linewire.Wrap(conn, br), reply, nil
}

// noServer classifies a failure as "no live server answers": absent or
// stale socket, a server dying before its first reply, or draining.
func noServer(err error) bool {
	return errors.Is(err, syscall.ENOENT) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, errDraining)
}

// Status asks the identity's server for its state; it fails when no
// server answers.
func (c *Client) Status(ctx context.Context, identity string) (Status, error) {
	p, err := c.resolvePaths(identity)
	if err != nil {
		return Status{}, err
	}
	_, m, err := c.exchange(ctx, p.Socket, Message{Op: OpStatus, Identity: sockpath.Hash(identity)})
	if err != nil {
		return Status{}, fmt.Errorf("procserve: status: %w", err)
	}
	return statusOf(p.Socket, sockpath.Hash(identity), m), nil
}

// Shutdown asks the identity's server to drain within grace, then waits
// for it to release its identity, escalating to SIGTERM and SIGKILL. It
// returns nil when no server is running.
func (c *Client) Shutdown(ctx context.Context, identity string, grace time.Duration) error {
	p, err := c.resolvePaths(identity)
	if err != nil {
		return err
	}
	return c.stop(ctx, p.Lock, p.Socket, sockpath.Hash(identity), grace, true)
}

// stop runs the shutdown sequence against one lock/socket pair.
func (c *Client) stop(ctx context.Context, lockPath, socket, hash string, grace time.Duration, escalate bool) error {
	if !lockfile.Held(lockPath) {
		return nil
	}
	rec, _ := lockfile.ReadFile(lockPath) //nolint:errcheck // a missing PID only disables escalation
	_, _, _ = c.exchange(ctx, socket, Message{Op: OpShutdown, Identity: hash, Grace: grace})
	if lockfile.WaitFree(ctx, lockPath, grace+stopOvershoot) {
		return nil
	}
	if !escalate || rec.PID <= 0 {
		return fmt.Errorf("procserve: server %s did not stop within %s", socket, grace+stopOvershoot)
	}
	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGKILL} {
		_ = syscall.Kill(rec.PID, sig) //nolint:errcheck // the process may already be gone
		if lockfile.WaitFree(ctx, lockPath, stopOvershoot) {
			return nil
		}
	}
	return fmt.Errorf("procserve: server pid %d survived SIGKILL", rec.PID)
}

// List reports every live server under the client's directories.
func (c *Client) List(ctx context.Context) ([]Status, error) {
	var out []Status
	for _, dir := range c.dirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), sockpath.LockSuffix) {
				continue
			}
			lockPath := filepath.Join(dir, e.Name())
			if !lockfile.Held(lockPath) {
				continue
			}
			rec, err := lockfile.ReadFile(lockPath)
			if err != nil {
				continue
			}
			_, m, err := c.exchange(ctx, rec.Socket, Message{Op: OpStatus, Identity: rec.Hash})
			if err != nil {
				continue
			}
			out = append(out, statusOf(rec.Socket, rec.Hash, m))
		}
	}
	return out, nil
}

// evictIdle enforces MaxIdle before a spawn. It is a best-effort guard:
// a victim that does not stop in time leaves the spawn proceeding.
func (c *Client) evictIdle(ctx context.Context) {
	if c.MaxIdle <= 0 {
		return
	}
	all, _ := c.List(ctx) //nolint:errcheck // List never fails
	var idle []Status
	for _, s := range all {
		if s.InFlight == 0 && !s.Draining && s.PID != os.Getpid() {
			idle = append(idle, s)
		}
	}
	cands := make([]conns.Idler, len(idle))
	for i, st := range idle {
		cands[i] = conns.Idler{Key: st.Socket, IdleSince: st.IdleSince}
	}
	if i := conns.LeastRecent(cands, c.MaxIdle); i >= 0 {
		v := idle[i]
		_ = c.stop(ctx, v.Socket+sockpath.LockSuffix, v.Socket, v.Hash, DefaultGrace, false) //nolint:errcheck // best-effort
	}
}

// statusOf converts a status reply.
func statusOf(socket, hash string, m Message) Status {
	return Status{Socket: socket, Hash: hash, PID: m.PID, InFlight: m.InFlight, IdleSince: m.IdleSince, Draining: m.Draining}
}
