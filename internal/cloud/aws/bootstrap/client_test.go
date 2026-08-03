package bootstrap

import (
	"context"
	"errors"
	"testing"
)

func TestVerifyAccountPreservesCancellationBeforeAWS(t *testing.T) {
	cause := errors.New("bootstrap canceled")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(cause)
	if err := VerifyAccount(ctx, "eu-west-3", "123456789012"); !errors.Is(err, cause) {
		t.Fatalf("error = %v", err)
	}
}
