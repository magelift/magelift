package bootstrap

import (
	"context"
	"testing"

	"google.golang.org/api/googleapi"
)

func TestRetryWIFCallRetriesTransientBackendError(t *testing.T) {
	attempts := 0
	value, err := retryWIFCall(context.Background(), 0, func() (string, error) {
		attempts++
		if attempts < 3 {
			return "", &googleapi.Error{Code: 503, Message: "Policy checks are unavailable."}
		}
		return "ready", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if value != "ready" || attempts != 3 {
		t.Fatalf("value=%q attempts=%d", value, attempts)
	}
}

func TestRetryWIFCallDoesNotRetryPermanentError(t *testing.T) {
	attempts := 0
	_, err := retryWIFCall(context.Background(), 0, func() (string, error) {
		attempts++
		return "", &googleapi.Error{Code: 403, Message: "permission denied"}
	})
	if err == nil || attempts != 1 {
		t.Fatalf("err=%v attempts=%d", err, attempts)
	}
}
