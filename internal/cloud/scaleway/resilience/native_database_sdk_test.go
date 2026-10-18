package resilience

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAwaitScalewayRDBCallReturnsWhenContextTimesOut(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := awaitScalewayRDBCall(ctx, func() (string, error) {
		time.Sleep(time.Second)
		return "late", nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("awaitScalewayRDBCall() error = %v, want context deadline", err)
	}
	if time.Since(started) > 200*time.Millisecond {
		t.Fatalf("awaitScalewayRDBCall() returned after %s, want the context deadline", time.Since(started))
	}
}
