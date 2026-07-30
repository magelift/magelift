package eksops

import (
	"context"
	"errors"
	"testing"

	"github.com/acourtiol/magelift/internal/platform"
)

func TestAcquireLockDelegatesToState(t *testing.T) {
	_, err := Ops{}.AcquireLock(context.Background(), nil)
	if err == nil {
		t.Fatal("AcquireLock must not silently succeed — must call State.Lock")
	}
	if errors.Is(err, platform.ErrNotSupported) {
		t.Fatal("AcquireLock must not return ErrNotSupported once State is wired")
	}
}
