package state

import (
	"context"
	"errors"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
)

func TestReleaseReceiptRejectsNonConcreteETagsBeforeDelete(t *testing.T) {
	for _, etag := range []azcore.ETag{"", " ", "*", `"*"`, `"prefix*"`, `"a","b"`, `"has space"`, "\"control\x00\""} {
		t.Run(string(etag), func(t *testing.T) {
			manager := &Manager{}
			if err := manager.ReleaseReceipt(context.Background(), etag); !errors.Is(err, ErrLockReplaced) {
				t.Fatalf("ReleaseReceipt(%q) = %v, want ErrLockReplaced", etag, err)
			}
		})
	}
}

func TestConcreteETagAcceptsAzureEntityTag(t *testing.T) {
	if !concreteETag(azcore.ETag(`"0x8DBA123456789AB"`)) {
		t.Fatal("quoted concrete Azure ETag was rejected")
	}
}
