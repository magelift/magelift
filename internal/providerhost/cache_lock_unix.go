//go:build unix

package providerhost

import (
	"context"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

func withCacheInstallLock(ctx context.Context, lockPath string) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open provider cache lock: %w", err)
	}
	fd := int(file.Fd())
	for {
		err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			break
		}
		if err != unix.EWOULDBLOCK && err != unix.EAGAIN {
			file.Close()
			return nil, fmt.Errorf("lock provider cache: %w", err)
		}
		if err := sleepContext(ctx, 25*time.Millisecond); err != nil {
			file.Close()
			return nil, err
		}
	}
	return func() {
		unix.Flock(fd, unix.LOCK_UN)
		file.Close()
	}, nil
}
