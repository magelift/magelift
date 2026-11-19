package plugin

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/smithy-go"
	"github.com/magelift/magelift/internal/s3state"
	"github.com/magelift/magelift/sdk"
)

func TestOperationErrorClassifiesProviderFailures(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want sdk.OperationErrorCode
	}{
		{s3state.ErrLocked, sdk.ErrCodeConflict},
		{s3state.ErrLockReplaced, sdk.ErrCodeConflict},
		{context.DeadlineExceeded, sdk.ErrCodeTimeout},
		{&smithy.GenericAPIError{Code: "ExpiredToken"}, sdk.ErrCodeCredential},
		{&smithy.GenericAPIError{Code: "PreconditionFailed"}, sdk.ErrCodeConflict},
		{integrity("changed request"), sdk.ErrCodeIntegrity},
		{errors.New("upstream request failed"), sdk.ErrCodeUpstream},
	} {
		if result := operationError(tc.err); result.Code != tc.want {
			t.Fatalf("%v: got %s, want %s", tc.err, result.Code, tc.want)
		}
	}
}
