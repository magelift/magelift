//go:build windows

package providerhost

import (
	"context"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

func withCacheInstallLock(ctx context.Context, lockPath string) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open provider cache lock: %w", err)
	}
	handle := windows.Handle(file.Fd())
	overlapped := new(windows.Overlapped)
	for {
		err := windows.LockFileEx(
			handle,
			windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
			0, 1, 0,
			overlapped,
		)
		if err == nil {
			break
		}
		if err != windows.ERROR_LOCK_VIOLATION {
			file.Close()
			return nil, fmt.Errorf("lock provider cache: %w", err)
		}
		if err := sleepContext(ctx, 25*time.Millisecond); err != nil {
			file.Close()
			return nil, err
		}
	}
	return func() {
		_ = windows.UnlockFileEx(handle, 0, 1, 0, overlapped)
		file.Close()
	}, nil
}
