package eksops

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestAcquireLockWarnsNoDIYLockTaken(t *testing.T) {
	var buf bytes.Buffer
	prev := diyLockWarnOut
	diyLockWarnOut = &buf
	t.Cleanup(func() { diyLockWarnOut = prev })

	release, err := Ops{}.AcquireLock(context.Background(), Planned{Spec: validSpec()})
	if err != nil {
		t.Fatalf("AcquireLock error: %v", err)
	}
	if release == nil {
		t.Fatal("AcquireLock must return a noop release")
	}
	if err := release(context.Background()); err != nil {
		t.Fatalf("noop release: %v", err)
	}
	msg := buf.String()
	lower := strings.ToLower(msg)
	if !strings.Contains(lower, "lock") || !strings.Contains(msg, "DIY") || !strings.Contains(lower, "not taken") {
		t.Fatalf("expected warning that no DIY lock was taken, got %q", msg)
	}
	if !strings.Contains(msg, "aws") && !strings.Contains(msg, "eks") {
		t.Fatalf("expected provider/runtime context in warning, got %q", msg)
	}
}
