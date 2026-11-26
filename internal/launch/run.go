package launch

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// ErrRunning means another tab already started the same desktop entry. A
// graphical app takes over the single cage client, so a second copy would
// either fight the first one or be invisible.
var ErrRunning = errors.New("app is already running")

// runLock is the per-entry lock shared by every tab. flock is tied to the open
// file description, so the kernel releases it when the last holder exits; a
// stale PID file can never block a future launch.
type runLock struct {
	file *os.File
}

// claim takes the lock for one desktop entry. An empty id (used by tests and
// internal callers) skips the lock.
func claim(id string) (*runLock, error) {
	if id == "" {
		return nil, nil
	}
	dir := filepath.Join(specDir(), "apps")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	// A digest keeps any desktop-file name out of the path syntax and under
	// NAME_MAX. The file itself stays behind on purpose: unlinking a lock file
	// lets a third process lock a new inode while another still holds the old
	// one.
	sum := sha256.Sum256([]byte(id))
	path := filepath.Join(dir, hex.EncodeToString(sum[:])+".lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, ErrRunning
		}
		return nil, err
	}
	return &runLock{file: f}, nil
}

func (l *runLock) release() {
	if l == nil || l.file == nil {
		return
	}
	_ = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	_ = l.file.Close()
	l.file = nil
}
