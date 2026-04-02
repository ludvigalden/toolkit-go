package proclock

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ludvigalden/toolkit-go/pkg/proclock/internal/fsid"
	"github.com/ludvigalden/toolkit-go/pkg/proclock/internal/liveness"
	"github.com/ludvigalden/toolkit-go/pkg/proclock/internal/lockfs"
)

// staleAge is how old an unheld lock file must be before a sweep removes
// it; it comfortably exceeds the open-to-flock window of any acquirer.
const staleAge = 5 * time.Minute

// Sentinel errors; acquisition failures wrap one so callers can branch
// with errors.Is.
var (
	ErrHeld    = errors.New("proclock: lock held by a live owner")
	ErrTimeout = errors.New("proclock: timed out waiting for lock")
	ErrCorrupt = errors.New("proclock: lock metadata corrupt while contended")
	ErrNotHeld = errors.New("proclock: lock not held")
)

// HeldError reports a live foreign owner; it wraps ErrHeld.
type HeldError struct {
	Owner Owner
	Path  string
}

// Error implements error.
func (e *HeldError) Error() string {
	return fmt.Sprintf("proclock: %s held by pid %d", e.Owner.Name, e.Owner.PID)
}

// Unwrap returns ErrHeld.
func (e *HeldError) Unwrap() error { return ErrHeld }

// Options names a lock and configures acquisition. Dir and Name are
// required; lock files live at <Dir>/global/<Name>.lock, or under a
// per-repository directory when Repo is set.
type Options struct {
	// Dir is the root holding all lock files (for example a cache dir).
	Dir string
	// Name is the logical lock name.
	Name string
	// Repo scopes the lock to a repository; symlinked spellings of one
	// directory share a lock.
	Repo string
	// Wait blocks until the lock frees instead of returning *HeldError.
	Wait bool
	// Timeout bounds acquisition and WaitUnlock when ctx has no deadline
	// (0: ctx only).
	Timeout time.Duration
	// Binary is the owner identity checked against PID reuse; "" uses
	// os.Executable.
	Binary string
	// PID overrides the recorded PID (0: this process).
	PID int
	// Endpoint and Extra are published with the owner metadata.
	Endpoint string
	Extra    map[string]string
}

// Path returns the lock file path for opts without touching the disk
// beyond resolving Repo.
func (o Options) Path() (string, error) {
	if o.Dir == "" || o.Name == "" {
		return "", errors.New("proclock: Options.Dir and Options.Name are required")
	}
	if o.Repo == "" {
		return filepath.Join(o.Dir, "global", o.Name+".lock"), nil
	}
	seg, err := fsid.Segment(o.Repo)
	if err != nil {
		return "", fmt.Errorf("proclock: repo segment: %w", err)
	}
	return filepath.Join(o.Dir, "repos", filepath.Clean("/"+seg), o.Name+".lock"), nil
}

// Lock is an acquired lock. Its methods are safe for concurrent use.
type Lock struct {
	path     string
	mu       sync.Mutex
	file     *os.File
	owner    Owner
	released bool
}

// Path returns the lock file path.
func (l *Lock) Path() string { return l.path }

// Owner returns the published metadata.
func (l *Lock) Owner() Owner {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.owner
}

// Publish replaces the published endpoint and extra metadata, for example
// once a server has bound its port. Identity fields are preserved.
func (l *Lock) Publish(endpoint string, extra map[string]string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.released {
		return ErrNotHeld
	}
	next := l.owner
	next.Endpoint, next.Extra = endpoint, extra
	if err := writeOwner(l.file, next); err != nil {
		return err
	}
	l.owner = next
	return nil
}

// Release drops the lock and removes its file (best-effort). It is
// idempotent.
func (l *Lock) Release() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.released {
		return nil
	}
	l.released = true
	// Unlink while holding the flock so no contender can lock the old
	// inode between the unlock and the removal.
	_ = os.Remove(l.path) //nolint:errcheck // a surviving file self-heals
	if err := l.file.Close(); err != nil {
		return fmt.Errorf("proclock: release %s: %w", l.owner.Name, err)
	}
	return nil
}

// Acquire takes the lock. A live foreign owner yields *HeldError unless
// opts.Wait is set; a dead owner's lock is reclaimed at once. The wait
// is bounded by ctx; opts.Timeout applies only when ctx has no deadline.
func Acquire(ctx context.Context, opts Options) (*Lock, error) {
	path, err := opts.Path()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("proclock: lock dir: %w", err)
	}
	if opts.Timeout > 0 {
		if _, ok := ctx.Deadline(); !ok {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
			defer cancel()
		}
	}
	return acquire(ctx, path, opts)
}

// maxCorruptStreak bounds consecutive unparseable reads of a contended
// lock: short streaks are a writer mid-update, long ones are corruption.
const maxCorruptStreak = 40

// acquire is the try-lock / inspect-owner loop.
func acquire(ctx context.Context, path string, opts Options) (*Lock, error) {
	corrupt := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, timeout(opts.Name, err)
		}
		l, err := tryAcquire(path, opts)
		if l != nil || err != nil {
			return l, err
		}
		// Contended: inspect the owner.
		raw, rerr := os.ReadFile(path)
		if errors.Is(rerr, os.ErrNotExist) {
			continue // released between our attempt and the read
		}
		owner, derr := decodeOwner(raw)
		foreign := errors.Is(derr, errFutureSchema)
		if (rerr != nil || derr != nil) && !foreign {
			if corrupt++; corrupt >= maxCorruptStreak {
				return nil, fmt.Errorf("proclock: acquire %s: %w", opts.Name, ErrCorrupt)
			}
			if err := lockfs.Sleep(ctx, 50*time.Millisecond); err != nil {
				return nil, timeout(opts.Name, err)
			}
			continue
		}
		corrupt = 0
		if !foreign && !alive(owner) {
			// The holder is exiting; the kernel frees the flock shortly.
			if err := lockfs.Sleep(ctx, 10*time.Millisecond); err != nil {
				return nil, timeout(opts.Name, err)
			}
			continue
		}
		if !opts.Wait {
			return nil, &HeldError{Owner: owner, Path: path}
		}
		if err := lockfs.WaitFree(ctx, path); err != nil {
			return nil, timeout(opts.Name, err)
		}
	}
}

// tryAcquire makes one non-blocking attempt; (nil, nil) means contended.
func tryAcquire(path string, opts Options) (*Lock, error) {
	f, err := lockfs.TryOpen(path)
	if f == nil || err != nil {
		return nil, err //nolint:wrapcheck // errors carry the proclock prefix
	}
	owner, err := newOwner(opts, previousGeneration(path))
	if err == nil {
		err = writeOwner(f, owner)
	}
	if err != nil {
		_ = f.Close() //nolint:errcheck // acquisition failed
		return nil, err
	}
	lockfs.Sweep(filepath.Dir(path), path, staleAge, deadBody)
	return &Lock{path: path, file: f, owner: owner}, nil
}

// previousGeneration carries the generation forward from a dead owner.
func previousGeneration(path string) uint64 {
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) == 0 {
		return 0
	}
	prev, err := decodeOwner(raw)
	if err != nil || aliveAndOurs(prev) {
		return 0
	}
	return prev.Generation
}

// newOwner builds the metadata published on acquisition.
func newOwner(opts Options, prevGen uint64) (Owner, error) {
	nonce, err := liveness.Nonce()
	if err != nil {
		return Owner{}, err //nolint:wrapcheck // carries the proclock prefix
	}
	pid := opts.PID
	if pid == 0 {
		pid = os.Getpid()
	}
	bin := opts.Binary
	if bin == "" {
		bin, _ = os.Executable() //nolint:errcheck // identity is best-effort
	}
	return Owner{
		Schema: schemaVersion, Name: opts.Name, Repo: opts.Repo, PID: pid, Binary: bin,
		BootID: liveness.BootID(), StartedAt: time.Now(), Nonce: nonce, Generation: prevGen + 1,
		Endpoint: opts.Endpoint, Extra: opts.Extra,
	}, nil
}

// writeOwner replaces the file's content under the held flock.
func writeOwner(f *os.File, o Owner) error {
	b, err := o.encode()
	if err != nil {
		return err
	}
	return lockfs.Rewrite(f, b) //nolint:wrapcheck // carries the proclock prefix
}

// Read returns the current owner's metadata without acquiring. found is
// false when no lock file exists.
func Read(opts Options) (owner Owner, found bool, err error) {
	path, err := opts.Path()
	if err != nil {
		return Owner{}, false, err
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Owner{}, false, nil
	}
	if err != nil {
		return Owner{}, false, fmt.Errorf("proclock: read %s: %w", path, err)
	}
	owner, err = decodeOwner(raw)
	if err != nil && !errors.Is(err, errFutureSchema) {
		return Owner{}, true, err
	}
	return owner, true, nil
}

// WaitUnlock blocks until the lock is free (or absent) without acquiring
// it. The wait is bounded by ctx; opts.Timeout applies only when ctx has
// no deadline, matching Acquire.
func WaitUnlock(ctx context.Context, opts Options) error {
	path, err := opts.Path()
	if err != nil {
		return err
	}
	if opts.Timeout > 0 {
		if _, ok := ctx.Deadline(); !ok {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
			defer cancel()
		}
	}
	if err := lockfs.WaitFree(ctx, path); err != nil {
		return fmt.Errorf("proclock: wait %s: %w: %w", opts.Name, ErrTimeout, err)
	}
	return nil
}

// timeout wraps a context error as ErrTimeout.
func timeout(name string, cause error) error {
	return fmt.Errorf("proclock: acquire %s: %w: %w", name, ErrTimeout, cause)
}
