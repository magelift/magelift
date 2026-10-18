package bootstrap

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

const syntheticPassphraseCanary = "synthetic-canary-passphrase-bytes"

func testSpec() Spec {
	return Spec{
		Project:     "shop",
		Environment: "preview",
		GCPProject:  "digital-lab-341608",
		Region:      "europe-west1",
	}
}

func testPreparationPlan(t *testing.T) PreparationPlan {
	plan, err := BuildPreparationPlan(testSpec())
	if err != nil {
		t.Fatalf("BuildPreparationPlan() error = %v", err)
	}
	return plan
}

func TestGCSObjectListQueryUsesVersions(t *testing.T) {
	t.Parallel()
	if q := gcsObjectListQuery(".pulumi/"); q == nil || !q.Versions {
		t.Fatalf("query = %#v", q)
	}
}

func TestHasPriorCheckpointDataIncludesArchivedGenerations(t *testing.T) {
	t.Parallel()
	lister := &stubGCSBucketLister{
		keys: map[string][]string{
			".pulumi/": {".pulumi/stacks/shop/preview/.checkpoint.json"},
		},
	}
	reader := &gcsCheckpointReader{lister: lister}
	prior, err := reader.HasPriorCheckpointData(context.Background(), "bucket", "shop", "preview")
	if err != nil {
		t.Fatal(err)
	}
	if !prior {
		t.Fatal("expected archived checkpoint object name to count as prior state")
	}
}

func TestEnsureStatePassphraseRefusesDeletedCheckpointGenerations(t *testing.T) {
	t.Parallel()
	lister := &stubGCSBucketLister{
		keys: map[string][]string{
			".pulumi/": {".pulumi/stacks/shop/preview/.checkpoint.json"},
		},
	}
	checkpoints := &gcsCheckpointReader{lister: lister}
	plan := testPreparationPlan(t)
	spec := testSpec()
	preparer, err := NewAccountPreparer(&fakeServices{}, &fakeRegistry{}, &fakeSecrets{}, checkpoints, &fakeLocks{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = preparer.EnsureStatePassphrase(context.Background(), plan, spec, "owner")
	if err == nil || !strings.Contains(err.Error(), "encrypted Pulumi state already exists") {
		t.Fatalf("expected deleted checkpoint refusal, got %v", err)
	}
}

func TestBuildPreparationPlanDeterministic(t *testing.T) {
	t.Parallel()
	spec := testSpec()
	first, err := BuildPreparationPlan(spec)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildPreparationPlan(spec)
	if err != nil {
		t.Fatal(err)
	}
	if first.RegistryID != second.RegistryID || first.StateSecretID != second.StateSecretID || first.ImageBase != second.ImageBase {
		t.Fatalf("plan drift: %#v vs %#v", first, second)
	}
	if first.RegistryLabels["magelift-purpose"] != purposeApplicationImages {
		t.Fatalf("registry purpose = %q", first.RegistryLabels["magelift-purpose"])
	}
	if first.StateSecretLabels["magelift-purpose"] != purposePulumiStatePassphrase {
		t.Fatalf("secret purpose = %q", first.StateSecretLabels["magelift-purpose"])
	}
	if !strings.HasPrefix(first.ImageBase, "europe-west1-docker.pkg.dev/digital-lab-341608/") {
		t.Fatalf("image base = %q", first.ImageBase)
	}
}

func TestEnsureRepositoryCreatesThenReuses(t *testing.T) {
	t.Parallel()
	plan := testPreparationPlan(t)
	registry := &fakeRegistry{}
	preparer, err := NewAccountPreparer(&fakeServices{}, registry, &fakeSecrets{}, &fakeCheckpoints{}, &fakeLocks{})
	if err != nil {
		t.Fatal(err)
	}
	spec := testSpec()
	first, err := preparer.EnsureRepository(context.Background(), plan, spec)
	if err != nil {
		t.Fatal(err)
	}
	second, err := preparer.EnsureRepository(context.Background(), plan, spec)
	if err != nil {
		t.Fatal(err)
	}
	if registry.creates != 1 {
		t.Fatalf("create count = %d, want 1", registry.creates)
	}
	if first.ImageBase != second.ImageBase || first.RepositoryID != plan.RegistryID {
		t.Fatalf("results differ: %#v %#v", first, second)
	}
}

func TestEnsureRepositoryRefusesForeignRepository(t *testing.T) {
	t.Parallel()
	plan := testPreparationPlan(t)
	registry := &fakeRegistry{
		existing: RepositoryMetadata{
			Format: "DOCKER", Location: plan.RegistryLocation,
			Labels: map[string]string{
				"magelift-managed-by": "terraform", "magelift-project": "shop",
				"magelift-environment": "preview", "magelift-purpose": purposeApplicationImages,
			},
		},
	}
	preparer, err := NewAccountPreparer(&fakeServices{}, registry, &fakeSecrets{}, &fakeCheckpoints{}, &fakeLocks{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := preparer.EnsureRepository(context.Background(), plan, testSpec()); err == nil || !strings.Contains(err.Error(), "magelift-managed-by") {
		t.Fatalf("expected ownership refusal, got %v", err)
	}
	if registry.creates != 0 {
		t.Fatal("foreign repository must not be mutated")
	}
}

func TestEnsureStatePassphraseCreatesAndReruns(t *testing.T) {
	t.Parallel()
	plan := testPreparationPlan(t)
	spec := testSpec()
	secrets := &fakeSecrets{}
	preparer, err := NewAccountPreparer(&fakeServices{}, &fakeRegistry{}, secrets, &fakeCheckpoints{}, &fakeLocks{})
	if err != nil {
		t.Fatal(err)
	}
	first, err := preparer.EnsureStatePassphrase(context.Background(), plan, spec, "prep-owner")
	if err != nil {
		t.Fatal(err)
	}
	second, err := preparer.EnsureStatePassphrase(context.Background(), plan, spec, "prep-owner")
	if err != nil {
		t.Fatal(err)
	}
	if first.VersionName != second.VersionName || secrets.adds != 1 {
		t.Fatalf("version ref=%q adds=%d", first.VersionName, secrets.adds)
	}
	if strings.Contains(first.VersionName, syntheticPassphraseCanary) {
		t.Fatal("ref must not echo canary")
	}
}

func TestEnsureStatePassphraseRefusesExistingStateWithoutSecret(t *testing.T) {
	t.Parallel()
	plan := testPreparationPlan(t)
	spec := testSpec()
	checkpoints := &fakeCheckpoints{prior: true}
	preparer, err := NewAccountPreparer(&fakeServices{}, &fakeRegistry{}, &fakeSecrets{}, checkpoints, &fakeLocks{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = preparer.EnsureStatePassphrase(context.Background(), plan, spec, "owner")
	if err == nil || !strings.Contains(err.Error(), "encrypted Pulumi state already exists") {
		t.Fatalf("expected refusal, got %v", err)
	}
}

func TestEnsureStatePassphraseRefusesDisabledVersion(t *testing.T) {
	t.Parallel()
	plan := testPreparationPlan(t)
	spec := testSpec()
	secrets := &fakeSecrets{
		existing: SecretMetadata{Labels: plan.StateSecretLabels},
		version:  SecretVersionMetadata{State: "DISABLED"},
	}
	preparer, err := NewAccountPreparer(&fakeServices{}, &fakeRegistry{}, secrets, &fakeCheckpoints{}, &fakeLocks{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = preparer.EnsureStatePassphrase(context.Background(), plan, spec, "owner")
	if err == nil || !strings.Contains(err.Error(), "version 1 is DISABLED") {
		t.Fatalf("expected disabled refusal, got %v", err)
	}
}

func TestEnsureStatePassphraseRecoversAfterAddVersionRace(t *testing.T) {
	t.Parallel()
	plan := testPreparationPlan(t)
	spec := testSpec()
	secrets := &fakeSecrets{addErrOnce: true, versionAfterAdd: SecretVersionMetadata{State: "ENABLED"}}
	preparer, err := NewAccountPreparer(&fakeServices{}, &fakeRegistry{}, secrets, &fakeCheckpoints{}, &fakeLocks{})
	if err != nil {
		t.Fatal(err)
	}
	ref, err := preparer.EnsureStatePassphrase(context.Background(), plan, spec, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(ref.VersionName, "/versions/1") {
		t.Fatalf("version ref = %q", ref.VersionName)
	}
}

func TestEnsureStatePassphraseSerializesConcurrentInitialization(t *testing.T) {
	t.Parallel()
	plan := testPreparationPlan(t)
	spec := testSpec()
	secrets := &fakeSecrets{}
	locks := &fakeLocks{serial: true}
	preparer, err := NewAccountPreparer(&fakeServices{}, &fakeRegistry{}, secrets, &fakeCheckpoints{}, locks)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := preparer.EnsureStatePassphrase(context.Background(), plan, spec, "owner")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	var success int
	for err := range errs {
		if err == nil {
			success++
			continue
		}
		if !strings.Contains(err.Error(), "deployment lock") && !errors.Is(err, errAddSecretVersionStage) {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if success < 1 {
		t.Fatal("expected at least one successful initialization")
	}
	if secrets.adds != 1 {
		t.Fatalf("add count = %d, want 1", secrets.adds)
	}
}

func TestEnsureStatePassphraseRefusesForeignSecret(t *testing.T) {
	t.Parallel()
	plan := testPreparationPlan(t)
	spec := testSpec()
	secrets := &fakeSecrets{
		existing: SecretMetadata{Labels: map[string]string{
			"magelift-managed-by": "terraform", "magelift-project": "shop",
			"magelift-environment": "preview", "magelift-purpose": purposePulumiStatePassphrase,
		}},
		version: SecretVersionMetadata{State: "ENABLED"},
	}
	preparer, err := NewAccountPreparer(&fakeServices{}, &fakeRegistry{}, secrets, &fakeCheckpoints{}, &fakeLocks{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = preparer.EnsureStatePassphrase(context.Background(), plan, spec, "owner")
	if err == nil || !strings.Contains(err.Error(), "magelift-managed-by") {
		t.Fatalf("expected foreign secret refusal, got %v", err)
	}
	if secrets.adds != 0 {
		t.Fatal("foreign secret must not receive a new version")
	}
}

func TestEnsureStatePassphraseResumesInterruptedEmptyInitialization(t *testing.T) {
	t.Parallel()
	plan := testPreparationPlan(t)
	spec := testSpec()
	secrets := &fakeSecrets{
		existing: SecretMetadata{Labels: plan.StateSecretLabels},
	}
	preparer, err := NewAccountPreparer(&fakeServices{}, &fakeRegistry{}, secrets, &fakeCheckpoints{}, &fakeLocks{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := preparer.EnsureStatePassphrase(context.Background(), plan, spec, "owner"); err != nil {
		t.Fatal(err)
	}
	if secrets.adds != 1 {
		t.Fatalf("adds = %d", secrets.adds)
	}
}

func TestBuildPreparationPlanCompactsLongNames(t *testing.T) {
	t.Parallel()
	spec := Spec{
		Project:     "shop",
		Environment: highAvailabilityEnvironment,
		GCPProject:  "digital-lab-341608",
		Region:      "europe-west1",
	}
	plan, err := BuildPreparationPlan(spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.RegistryID) > maxOwnedResourceNameLen {
		t.Fatalf("registry id length = %d", len(plan.RegistryID))
	}
}

func TestEnsureRepositoryRejectsPlanSpecMismatch(t *testing.T) {
	t.Parallel()
	plan := testPreparationPlan(t)
	mismatch := testSpec()
	mismatch.Environment = "staging"
	preparer, err := NewAccountPreparer(&fakeServices{}, &fakeRegistry{}, &fakeSecrets{}, &fakeCheckpoints{}, &fakeLocks{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = preparer.EnsureRepository(context.Background(), plan, mismatch)
	if err == nil || !strings.Contains(err.Error(), "preparation plan does not match spec") {
		t.Fatalf("expected scope mismatch, got %v", err)
	}
}

func TestEnsureStatePassphraseRejectsPlanSpecMismatchBeforeLock(t *testing.T) {
	t.Parallel()
	plan := testPreparationPlan(t)
	spec := testSpec()
	spec.Environment = "staging"
	locks := &fakeLocks{serial: true}
	preparer, err := NewAccountPreparer(&fakeServices{}, &fakeRegistry{}, &fakeSecrets{}, &fakeCheckpoints{}, locks)
	if err != nil {
		t.Fatal(err)
	}
	_, err = preparer.EnsureStatePassphrase(context.Background(), plan, spec, "owner")
	if err == nil || !strings.Contains(err.Error(), "preparation plan does not match spec") {
		t.Fatalf("expected scope mismatch, got %v", err)
	}
	if locks.held {
		t.Fatal("lock must not be acquired when plan and spec disagree")
	}
}

func TestEnsureAPIsEnablesOnlyMissing(t *testing.T) {
	t.Parallel()
	plan := testPreparationPlan(t)
	services := &fakeServices{enabled: map[string]bool{"storage.googleapis.com": true}}
	preparer, err := NewAccountPreparer(services, &fakeRegistry{}, &fakeSecrets{}, &fakeCheckpoints{}, &fakeLocks{})
	if err != nil {
		t.Fatal(err)
	}
	if err := preparer.EnsureAPIs(context.Background(), plan, testSpec()); err != nil {
		t.Fatal(err)
	}
	if services.enabledCalls == 0 {
		t.Fatal("expected enabled lookup")
	}
	if services.enableCalls == 0 {
		t.Fatal("expected enable calls for missing services")
	}
}

type fakeServices struct {
	enabled      map[string]bool
	enabledCalls int
	enableCalls  int
}

func (f *fakeServices) Close() error { return nil }

func (f *fakeServices) ProjectNumber(context.Context, string) (int64, error) { return 249, nil }

func (f *fakeServices) EnabledServices(ctx context.Context, _ int64, services []string) (map[string]bool, error) {
	f.enabledCalls++
	out := make(map[string]bool, len(services))
	for _, service := range services {
		if f.enabled != nil && f.enabled[service] {
			out[service] = true
		} else {
			out[service] = false
		}
	}
	return out, nil
}

func (f *fakeServices) EnableService(context.Context, int64, string) error {
	f.enableCalls++
	return nil
}

type fakeRegistry struct {
	existing RepositoryMetadata
	created  RepositoryMetadata
	creates  int
}

func (f *fakeRegistry) Close() error { return nil }

func (f *fakeRegistry) GetRepository(context.Context, string, string, string) (RepositoryMetadata, error) {
	if f.existing.Format != "" || len(f.existing.Labels) > 0 {
		return f.existing, nil
	}
	if f.creates > 0 {
		return f.created, nil
	}
	return RepositoryMetadata{}, ErrPreparationNotFound
}

func (f *fakeRegistry) CreateRepository(_ context.Context, _, location, _ string, labels map[string]string) (RepositoryMetadata, error) {
	f.creates++
	f.created = RepositoryMetadata{Format: "DOCKER", Location: location, Labels: copyStringMap(labels)}
	return f.created, nil
}

type fakeSecrets struct {
	mu              sync.Mutex
	existing        SecretMetadata
	version         SecretVersionMetadata
	versions        []SecretVersionMetadata
	adds            int
	addErrOnce      bool
	versionAfterAdd SecretVersionMetadata
	lastPayload     []byte
}

func (f *fakeSecrets) Close() error { return nil }

func (f *fakeSecrets) GetSecret(context.Context, string, string) (SecretMetadata, error) {
	if len(f.existing.Labels) > 0 || f.existing.Name != "" {
		return f.existing, nil
	}
	if f.adds > 0 && len(f.existing.Labels) == 0 {
		return SecretMetadata{Labels: map[string]string{
			"magelift-managed-by": "magelift", "magelift-project": "shop",
			"magelift-environment": "preview", "magelift-purpose": purposePulumiStatePassphrase,
		}}, nil
	}
	return SecretMetadata{}, ErrPreparationNotFound
}

func (f *fakeSecrets) CreateSecret(_ context.Context, _, _ string, labels map[string]string) error {
	f.existing = SecretMetadata{Labels: copyStringMap(labels)}
	return nil
}

func (f *fakeSecrets) GetVersion(context.Context, string, string, string) (SecretVersionMetadata, error) {
	if f.version.State != "" {
		return f.version, nil
	}
	if f.versionAfterAdd.State != "" && f.adds > 0 {
		return f.versionAfterAdd, nil
	}
	if len(f.versions) > 0 {
		return f.versions[0], nil
	}
	if f.adds > 0 {
		return SecretVersionMetadata{State: "ENABLED"}, nil
	}
	return SecretVersionMetadata{}, ErrPreparationNotFound
}

func (f *fakeSecrets) ListVersions(context.Context, string, string) ([]SecretVersionMetadata, error) {
	return append([]SecretVersionMetadata(nil), f.versions...), nil
}

func (f *fakeSecrets) AddVersion(_ context.Context, _, _ string, payload []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastPayload = append([]byte(nil), payload...)
	f.adds++
	if f.addErrOnce && f.adds == 1 {
		return errAddSecretVersionStage
	}
	return nil
}

type stubGCSBucketLister struct {
	keys map[string][]string
}

func (s *stubGCSBucketLister) Close() error { return nil }

func (s *stubGCSBucketLister) List(_ context.Context, _, prefix string) ([]string, error) {
	if s == nil || s.keys == nil {
		return nil, nil
	}
	return append([]string(nil), s.keys[prefix]...), nil
}

type fakeCheckpoints struct {
	prior bool
}

func (f *fakeCheckpoints) Close() error { return nil }

func (f *fakeCheckpoints) HasPriorCheckpointData(context.Context, string, string, string) (bool, error) {
	return f.prior, nil
}

type fakeLocks struct {
	serial   bool
	mu       sync.Mutex
	held     bool
	acquires int
}

func (f *fakeLocks) Close() error { return nil }

func (f *fakeLocks) Acquire(ctx context.Context, _, _, _, _ string) (func(context.Context) error, error) {
	f.acquires++
	if f.serial {
		f.mu.Lock()
		if f.held {
			f.mu.Unlock()
			return nil, errors.New("deployment lock is held")
		}
		f.held = true
		f.mu.Unlock()
		return func(context.Context) error {
			f.mu.Lock()
			f.held = false
			f.mu.Unlock()
			return nil
		}, nil
	}
	return func(context.Context) error { return nil }, nil
}

func TestEnsureAPIsRerunDoesNotRequestEnablePermission(t *testing.T) {
	enabled := map[string]bool{}
	for _, name := range PreparationRequiredServices() {
		enabled[name] = true
	}
	services := &fakeServices{enabled: enabled}
	preparer, err := NewAccountPreparer(services, &fakeRegistry{}, &fakeSecrets{}, &fakeCheckpoints{}, &fakeLocks{})
	if err != nil {
		t.Fatal(err)
	}
	if err := preparer.EnsureAPIs(context.Background(), testPreparationPlan(t), testSpec()); err != nil {
		t.Fatal(err)
	}
	if services.enableCalls != 0 {
		t.Fatalf("enabled %d services on an already prepared project", services.enableCalls)
	}
}
