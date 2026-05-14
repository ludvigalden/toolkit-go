package procserve

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ludvigalden/toolkit-go/pkg/procserve/internal/conns"
	"github.com/ludvigalden/toolkit-go/pkg/procserve/internal/inherit"
	"github.com/ludvigalden/toolkit-go/pkg/procserve/internal/linewire"
	"github.com/ludvigalden/toolkit-go/pkg/procserve/internal/lockfile"
	"github.com/ludvigalden/toolkit-go/pkg/procserve/internal/sockpath"
)

// Server lifecycle defaults.
const (
	DefaultIdleTimeout = 10 * time.Minute
	DefaultGrace       = 5 * time.Second
	defaultBacklogWait = 150 * time.Millisecond
)

// Spawned reports whether this process was started by Client.Connect and
// should call Listen and Serve instead of acting as a client.
func Spawned() bool { return inherit.Spawned(os.Getenv) }

// Handler serves one connection after the handshake and owns conn. ctx
// ends when the drain grace runs out.
type Handler interface {
	ServeConn(ctx context.Context, conn net.Conn)
}

// HandlerFunc adapts a function to Handler.
type HandlerFunc func(ctx context.Context, conn net.Conn)

// ServeConn calls f.
func (f HandlerFunc) ServeConn(ctx context.Context, conn net.Conn) { f(ctx, conn) }

// ServerOptions configures the server side.
type ServerOptions struct {
	// IdleTimeout exits the server after this long with no connection
	// (0: DefaultIdleTimeout).
	IdleTimeout time.Duration
	// Grace bounds the drain of in-flight connections (0: DefaultGrace).
	Grace time.Duration
	// BacklogWait is how long the drain keeps accepting connections the
	// kernel had already queued, answering them "draining" (0: 150ms).
	BacklogWait time.Duration
	// Protocol frames control messages; it must match the clients'.
	Protocol Protocol
	// Logger receives lifecycle logs (nil: discard).
	Logger *slog.Logger
}

// Listener is the server's inherited listener and identity lock.
type Listener struct {
	ln   *net.UnixListener
	lock *os.File
	rec  lockfile.Record
	opts ServerOptions
}

// Listen adopts the listener and lock inherited from Client.Connect and
// records this PID in the lock. It clears the spawn marker so this
// process's own children are not mistaken for servers.
func Listen(opts ServerOptions) (*Listener, error) {
	if !Spawned() {
		return nil, errors.New("procserve: not spawned by procserve (no inherited listener)")
	}
	ul, lock, err := inherit.Adopt()
	if err != nil {
		return nil, err //nolint:wrapcheck // errors carry the procserve prefix
	}
	rec, err := lockfile.Read(lock)
	if err == nil {
		rec.PID = os.Getpid()
		err = lockfile.Write(lock, rec)
	}
	if err != nil {
		_, _ = ul.Close(), lock.Close() //nolint:errcheck // adoption failed
		return nil, err                 //nolint:wrapcheck // errors carry the procserve prefix
	}
	if opts.IdleTimeout <= 0 {
		opts.IdleTimeout = DefaultIdleTimeout
	}
	if opts.Grace <= 0 {
		opts.Grace = DefaultGrace
	}
	if opts.BacklogWait <= 0 {
		opts.BacklogWait = defaultBacklogWait
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.DiscardHandler)
	}
	opts.Protocol = protocolOr(opts.Protocol)
	return &Listener{ln: ul, lock: lock, rec: rec, opts: opts}, nil
}

// Close releases the listener and lock without serving.
func (l *Listener) Close() error {
	err := l.ln.Close()
	if cerr := l.lock.Close(); err == nil {
		err = cerr
	}
	return err //nolint:wrapcheck // close errors pass through
}

// Serve accepts until idle timeout, shutdown request, SIGTERM, or ctx
// end, then drains: socket unlinked, lock released, queued connections
// answered "draining", in-flight handlers canceled after the grace.
func Serve(ctx context.Context, l *Listener, h Handler) error {
	s := &server{Listener: l, h: h, conns: conns.New(time.Now()), shutdown: make(chan time.Duration, 1)}
	var cancel context.CancelFunc
	s.hctx, cancel = context.WithCancel(context.WithoutCancel(ctx))
	defer cancel()
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM)
	defer signal.Stop(sigs)
	acceptErr := make(chan error, 1)
	go func() { acceptErr <- s.acceptLoop() }()
	grace, err := s.wait(ctx, sigs, acceptErr)
	s.drain(grace, cancel)
	return err
}

// server is one Serve call's state.
type server struct {
	*Listener
	h        Handler
	hctx     context.Context
	conns    *conns.Tracker
	shutdown chan time.Duration
}

// wait blocks until a stop trigger and returns the drain grace.
func (s *server) wait(ctx context.Context, sigs <-chan os.Signal, acceptErr <-chan error) (time.Duration, error) {
	idle := time.NewTimer(s.opts.IdleTimeout)
	defer idle.Stop()
	for {
		select {
		case <-ctx.Done():
			return s.opts.Grace, nil
		case <-sigs:
			s.opts.Logger.Info("procserve: SIGTERM")
			return s.opts.Grace, nil
		case g := <-s.shutdown:
			s.opts.Logger.Info("procserve: shutdown requested")
			return g, nil
		case err := <-acceptErr:
			return s.opts.Grace, err
		case <-s.conns.Changed():
			if s.conns.Idle() {
				idle.Reset(s.opts.IdleTimeout)
			} else {
				idle.Stop()
			}
		case <-idle.C:
			if s.conns.Idle() {
				s.opts.Logger.Info("procserve: idle timeout")
				return s.opts.Grace, nil
			}
		}
	}
}

// acceptLoop serves the listener until it closes.
func (s *server) acceptLoop() error {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) || errors.Is(err, os.ErrDeadlineExceeded) {
				return nil
			}
			return fmt.Errorf("procserve: accept: %w", err)
		}
		go s.handle(conn)
	}
}

// handle runs the handshake on one connection and dispatches it.
func (s *server) handle(conn net.Conn) {
	_ = conn.SetDeadline(time.Now().Add(replyWait)) //nolint:errcheck // surfaces as a read error
	br := bufio.NewReader(conn)
	req, err := s.opts.Protocol.Read(br)
	if err != nil {
		_ = conn.Close() //nolint:errcheck // malformed or abandoned
		return
	}
	if req.Identity != s.rec.Hash {
		s.replyAndClose(conn, Message{Op: OpReply, Error: "identity mismatch"})
		return
	}
	if req.Op == OpConnect {
		if !s.conns.Admit() {
			s.replyAndClose(conn, s.status())
			return
		}
		defer s.finishHandler(conn)
		if s.opts.Protocol.Write(conn, s.status()) != nil {
			return
		}
		_ = conn.SetDeadline(time.Time{}) //nolint:errcheck // handler owns deadlines
		s.h.ServeConn(s.hctx, linewire.Wrap(conn, br))
		return
	}
	reply := s.status()
	if req.Op == OpShutdown {
		select {
		case s.shutdown <- req.Grace:
		default:
		}
	}
	s.replyAndClose(conn, reply)
}

// replyAndClose answers a one-shot exchange.
func (s *server) replyAndClose(conn net.Conn, m Message) {
	_ = s.opts.Protocol.Write(conn, m) //nolint:errcheck // one-shot reply
	_ = conn.Close()                   //nolint:errcheck // one-shot exchange
}

// finishHandler closes a handler connection and updates the counts.
func (s *server) finishHandler(conn net.Conn) {
	_ = conn.Close() //nolint:errcheck // the handler may have closed it
	s.conns.Done()
}

// status snapshots the server state as a reply.
func (s *server) status() Message {
	n, since, draining := s.conns.Snapshot()
	return Message{Op: OpReply, PID: os.Getpid(), InFlight: n, IdleSince: since, Draining: draining}
}

// drain runs the ordered shutdown. The socket is unlinked only while it
// is still the inode this server published, and before the lock is
// released, so no window exists where the lock is free but the old
// socket still answers.
func (s *server) drain(grace time.Duration, cancel context.CancelFunc) {
	if grace <= 0 {
		grace = s.opts.Grace
	}
	s.conns.StartDrain()
	if ino, err := sockpath.Inode(s.rec.Socket); err == nil && ino == s.rec.Inode {
		_ = os.Remove(s.rec.Socket) //nolint:errcheck // best-effort
	}
	_ = s.lock.Close()                                       //nolint:errcheck // releases the identity
	_ = s.ln.SetDeadline(time.Now().Add(s.opts.BacklogWait)) //nolint:errcheck // ends the accept loop
	time.Sleep(s.opts.BacklogWait)
	_ = s.ln.Close() //nolint:errcheck // drain done accepting
	s.conns.Wait(grace, cancel)
}
