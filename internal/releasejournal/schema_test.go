package releasejournal

import (
	"errors"
	"strings"
	"testing"
)

func TestRefuseIncompatibleRollback(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		live, target int
		want         error
		wantText     []string
	}{
		{name: "compatible", live: 2, target: 2},
		{name: "digest-newer-than-live", live: 2, target: 3},
		{
			name:     "older-digest",
			live:     2,
			target:   1,
			want:     ErrSchemaMismatch,
			wantText: []string{"schema epoch 2", "digest schema epoch 1", "restore the previous database", "forward-fix"},
		},
		{
			name:     "live-missing",
			live:     0,
			target:   1,
			want:     ErrSchemaUnavailable,
			wantText: []string{"not recorded", "restore the previous database", "forward-fix"},
		},
		{
			name:     "target-missing",
			live:     1,
			target:   0,
			want:     ErrSchemaUnavailable,
			wantText: []string{"not recorded", "restore the previous database"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := RefuseIncompatibleRollback(test.live, test.target)
			if test.want == nil {
				if err != nil {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if !errors.Is(err, test.want) {
				t.Fatalf("err = %v, want %v", err, test.want)
			}
			got := err.Error()
			for _, fragment := range test.wantText {
				if !strings.Contains(got, fragment) {
					t.Fatalf("error %q missing %q", got, fragment)
				}
			}
		})
	}
}

func TestSignatureMetadataForDigest(t *testing.T) {
	t.Parallel()
	entries := []Entry{
		{DigestReference: "a", SignatureIdentity: "old@example.invalid", SignatureIssuer: "https://old.example.invalid"},
		{DigestReference: "b"},
		{DigestReference: "a", SignatureIdentity: "new@example.invalid", SignatureIssuer: "https://new.example.invalid"},
		{DigestReference: "a"},
	}
	identity, issuer := SignatureMetadataForDigest(entries, "a")
	if identity != "new@example.invalid" || issuer != "https://new.example.invalid" {
		t.Fatalf("got %q %q", identity, issuer)
	}
	identity, issuer = SignatureMetadataForDigest(entries, "b")
	if identity != "" || issuer != "" {
		t.Fatalf("unsigned digest leaked %q %q", identity, issuer)
	}
}

func TestSchemaEpochForDigest(t *testing.T) {
	t.Parallel()
	entries := []Entry{
		{DigestReference: "a", SchemaEpoch: 1},
		{DigestReference: "b"},
		{DigestReference: "a", SchemaEpoch: 3},
		{DigestReference: "a"},
	}
	if got := SchemaEpochForDigest(entries, "a"); got != 3 {
		t.Fatalf("got %d", got)
	}
	if got := SchemaEpochForDigest(entries, "b"); got != 0 {
		t.Fatalf("unsigned digest leaked epoch %d", got)
	}
}
