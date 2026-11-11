package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// ErrInUse means another Hashi process (the server, or a console command that needs it stopped) holds the data folder.
var ErrInUse = errors.New("another Hashi process is using this data folder; stop the server first")

// Lock takes an exclusive lock on <dir>/hashi.lock for as long as the process needs the database to itself: the
// server while it runs, `hashi vault rotate` while it re-seals. The kernel drops it when the process exits.
func Lock(dir string) (unlock func(), err error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "hashi.lock"), os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // configured data dir
	if err != nil {
		return nil, fmt.Errorf("lock data folder: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil { //nolint:gosec // fd fits in int
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrInUse
		}
		return nil, fmt.Errorf("lock data folder: %w", err)
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil //nolint:gosec // see above
}
