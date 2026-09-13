package certification

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

const (
	defaultFileRegistryLockPoll       = 25 * time.Millisecond
	defaultFileRegistryStaleLockAfter = 5 * time.Minute
)

// withFileRegistryLock serializes one local file-registry operation. The lock
// is deliberately a directory created with Mkdir, which is an atomic
// same-filesystem claim on the supported local filesystems. Distributed
// runners must inject a conditional object-store or database registry instead
// of treating this lock as a cross-host primitive.
func withFileRegistryLock(ctx context.Context, lockPath string, poll, staleAfter time.Duration, operation func() error) (err error) {
	if ctx == nil {
		return errors.New("file registry context is required")
	}
	if lockPath == "" {
		return errors.New("file registry lock path is required")
	}
	if operation == nil {
		return errors.New("file registry operation is required")
	}
	if poll <= 0 {
		poll = defaultFileRegistryLockPoll
	}
	if staleAfter <= 0 {
		staleAfter = defaultFileRegistryStaleLockAfter
	}
	unlock, err := acquireFileRegistryLock(ctx, lockPath, poll, staleAfter)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, unlock())
	}()
	return operation()
}

func acquireFileRegistryLock(ctx context.Context, lockPath string, poll, staleAfter time.Duration) (func() error, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		err := os.Mkdir(lockPath, 0o700)
		if err == nil {
			return func() error {
				err := os.Remove(lockPath)
				if errors.Is(err, os.ErrNotExist) {
					return nil
				}
				return err
			}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("acquire file registry lock: %w", err)
		}
		if info, statErr := os.Stat(lockPath); statErr == nil && time.Since(info.ModTime()) > staleAfter {
			if removeErr := os.Remove(lockPath); removeErr == nil || errors.Is(removeErr, os.ErrNotExist) {
				continue
			}
		}
		timer := time.NewTimer(poll)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
