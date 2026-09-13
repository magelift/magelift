package certification

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	sdk "github.com/magelift/magelift/sdk/v1"
)

func testArtifactRequest(owner string) ArtifactBuildRequest {
	return ArtifactBuildRequest{CompatibilityFingerprint: strings.Repeat("a", 64), OwnershipMarker: owner}
}

func testArtifactContract(fingerprint string) sdk.ImmutableArtifactContract {
	return sdk.ImmutableArtifactContract{
		ImageDigest:         "registry.example.invalid/magelift@sha256:" + strings.Repeat("b", 64),
		ManifestDigest:      "sha256:" + strings.Repeat("c", 64),
		InputFingerprint:    fingerprint,
		ProvenanceReference: "https://example.invalid/provenance/" + fingerprint,
		SignatureReference:  "oci://example.invalid/signature/" + fingerprint,
	}
}

func TestEnsureImmutableArtifactBuildsAndThenReusesPublishedRecord(t *testing.T) {
	registry := NewMemoryImmutableArtifactRegistry()
	request := testArtifactRequest("magelift/test/artifact")
	builds := 0
	build := func(context.Context, ArtifactBuildRequest) (sdk.ImmutableArtifactContract, error) {
		builds++
		return testArtifactContract(request.CompatibilityFingerprint), nil
	}

	record, reused, err := EnsureImmutableArtifact(context.Background(), registry, request, build)
	if err != nil || reused || builds != 1 {
		t.Fatalf("first artifact = %#v, reused=%v, builds=%d, err=%v", record, reused, builds, err)
	}
	reusedRecord, reused, err := EnsureImmutableArtifact(context.Background(), registry, request, build)
	if err != nil || !reused || builds != 1 || reusedRecord != record {
		t.Fatalf("reused artifact = %#v, reused=%v, builds=%d, err=%v", reusedRecord, reused, builds, err)
	}
}

func TestMemoryImmutableArtifactRegistryClaimsOnlyOneConcurrentBuilder(t *testing.T) {
	registry := NewMemoryImmutableArtifactRegistry()
	fingerprint := strings.Repeat("a", 64)
	owners := []string{"magelift/test/artifact-a", "magelift/test/artifact-b"}
	var waitGroup sync.WaitGroup
	results := make(chan ArtifactReuseLookup, len(owners))
	errorsCh := make(chan error, len(owners))
	for _, owner := range owners {
		owner := owner
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			lookup, err := registry.Claim(context.Background(), fingerprint, owner)
			if err != nil {
				errorsCh <- err
				return
			}
			results <- lookup
		}()
	}
	waitGroup.Wait()
	close(results)
	close(errorsCh)
	for err := range errorsCh {
		t.Fatalf("concurrent artifact claim error = %v", err)
	}
	var claimed, blocked int
	for lookup := range results {
		switch lookup.Decision {
		case ArtifactReuseMiss:
			claimed++
		case ArtifactReuseBlocked:
			blocked++
		default:
			t.Fatalf("unexpected concurrent artifact decision = %#v", lookup)
		}
	}
	if claimed != 1 || blocked != 1 {
		t.Fatalf("concurrent artifact claims claimed=%d blocked=%d", claimed, blocked)
	}
}

func TestMemoryImmutableArtifactRegistryBlocksASecondClaimByTheSameOwner(t *testing.T) {
	registry := NewMemoryImmutableArtifactRegistry()
	fingerprint := strings.Repeat("a", 64)
	owner := "magelift/test/same-owner"
	first, err := registry.Claim(context.Background(), fingerprint, owner)
	if err != nil || first.Decision != ArtifactReuseMiss {
		t.Fatalf("first claim = %#v, err=%v", first, err)
	}
	second, err := registry.Claim(context.Background(), fingerprint, owner)
	if err != nil || second.Decision != ArtifactReuseBlocked || !strings.Contains(second.Reason, "already claimed") {
		t.Fatalf("second claim = %#v, err=%v", second, err)
	}
}

func TestEnsureImmutableArtifactReleasesClaimAfterBuildFailure(t *testing.T) {
	registry := NewMemoryImmutableArtifactRegistry()
	request := testArtifactRequest("magelift/test/artifact")
	_, _, err := EnsureImmutableArtifact(context.Background(), registry, request, func(context.Context, ArtifactBuildRequest) (sdk.ImmutableArtifactContract, error) {
		return sdk.ImmutableArtifactContract{}, errors.New("builder failed")
	})
	if err == nil || !strings.Contains(err.Error(), "builder failed") {
		t.Fatalf("build failure = %v", err)
	}
	if _, err := registry.Claim(context.Background(), request.CompatibilityFingerprint, request.OwnershipMarker); err != nil {
		t.Fatalf("claim after failed build = %v", err)
	}
}

func TestEnsureImmutableArtifactReleasesClaimWhenBuildCancelsContext(t *testing.T) {
	registry := NewMemoryImmutableArtifactRegistry()
	request := testArtifactRequest("magelift/test/artifact-cancelled")
	ctx, cancel := context.WithCancel(context.Background())
	_, _, err := EnsureImmutableArtifact(ctx, registry, request, func(context.Context, ArtifactBuildRequest) (sdk.ImmutableArtifactContract, error) {
		cancel()
		return sdk.ImmutableArtifactContract{}, errors.New("builder canceled")
	})
	if err == nil || !strings.Contains(err.Error(), "builder canceled") {
		t.Fatalf("canceled build error = %v", err)
	}
	lookup, err := registry.Claim(context.Background(), request.CompatibilityFingerprint, "next-run")
	if err != nil {
		t.Fatal(err)
	}
	if lookup.Decision != ArtifactReuseMiss {
		t.Fatalf("artifact claim after canceled build = %#v", lookup)
	}
}

func TestImmutableArtifactRecordRejectsBoundaryMismatch(t *testing.T) {
	record := ImmutableArtifactRecord{CompatibilityFingerprint: strings.Repeat("a", 64), Artifact: testArtifactContract(strings.Repeat("b", 64))}
	if err := record.Validate(); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("mismatched artifact record was accepted: %v", err)
	}
}

func TestFileImmutableArtifactRegistryPersistsConditionalClaimsAndReuse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "artifacts.json")
	first, err := NewFileImmutableArtifactRegistry(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewFileImmutableArtifactRegistry(path)
	if err != nil {
		t.Fatal(err)
	}
	request := testArtifactRequest("magelift/test/file-artifact")
	contract := testArtifactContract(request.CompatibilityFingerprint)

	claim, err := first.Claim(context.Background(), request.CompatibilityFingerprint, request.OwnershipMarker)
	if err != nil || claim.Decision != ArtifactReuseMiss {
		t.Fatalf("first file claim = %#v, err=%v", claim, err)
	}
	blocked, err := second.Claim(context.Background(), request.CompatibilityFingerprint, "magelift/test/other-builder")
	if err != nil || blocked.Decision != ArtifactReuseBlocked {
		t.Fatalf("conflicting file claim = %#v, err=%v", blocked, err)
	}
	if err := first.Publish(context.Background(), request.CompatibilityFingerprint, request.OwnershipMarker, ImmutableArtifactRecord{CompatibilityFingerprint: request.CompatibilityFingerprint, Artifact: contract}); err != nil {
		t.Fatal(err)
	}
	record, found, err := second.Lookup(context.Background(), request.CompatibilityFingerprint)
	if err != nil || !found || record.Artifact != contract {
		t.Fatalf("persisted file artifact = %#v, found=%v, err=%v", record, found, err)
	}
	ready, err := second.Claim(context.Background(), request.CompatibilityFingerprint, "magelift/test/next-run")
	if err != nil || ready.Decision != ArtifactReuseReady || ready.Record != record {
		t.Fatalf("reused file artifact claim = %#v, err=%v", ready, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("file artifact registry mode = %o, want 600", info.Mode().Perm())
	}
}

func TestFileImmutableArtifactRegistryHonorsContextWhileWaitingForLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "artifacts.json")
	registry, err := NewFileImmutableArtifactRegistry(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path+".lock", 0o700); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path + ".lock")
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, _, lookupErr := registry.Lookup(ctx, strings.Repeat("a", 64))
		result <- lookupErr
	}()
	time.Sleep(2 * registry.lockPoll)
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled file registry lookup = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("file registry lookup did not honor cancellation while waiting for lock")
	}
}
