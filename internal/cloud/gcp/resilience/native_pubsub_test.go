package resilience

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	cloudrecovery "github.com/magelift/magelift/internal/cloud/recovery"
	cloudresilience "github.com/magelift/magelift/internal/cloud/resilience"
	sdk "github.com/magelift/magelift/sdk/v1"
)

type fakePubSub struct {
	subscriptions map[string]PubSubSubscription
	snapshots     map[string]PubSubSnapshot
	seeks         []string
	deletes       []string
	labelSets     int
	now           time.Time
}

func newFakePubSub() *fakePubSub {
	return &fakePubSub{
		subscriptions: map[string]PubSubSubscription{},
		snapshots:     map[string]PubSubSnapshot{},
		now:           time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC),
	}
}

func (f *fakePubSub) GetSubscription(_ context.Context, name string) (PubSubSubscription, error) {
	value, ok := f.subscriptions[name]
	if !ok {
		return PubSubSubscription{}, errors.New("subscription not found")
	}
	return value, nil
}

func (f *fakePubSub) CreateSnapshot(_ context.Context, name, subscription string) (PubSubSnapshot, error) {
	if _, exists := f.snapshots[name]; exists {
		return PubSubSnapshot{}, errors.New("already exists")
	}
	for _, value := range f.subscriptions {
		if value.Name == subscription {
			snapshot := PubSubSnapshot{Name: name, Topic: value.Topic, ExpireTime: f.now.Add(7 * 24 * time.Hour), EncryptionVerified: true}
			f.snapshots[name] = snapshot
			return snapshot, nil
		}
	}
	return PubSubSnapshot{}, errors.New("subscription not found")
}

func (f *fakePubSub) GetSnapshot(_ context.Context, name string) (PubSubSnapshot, error) {
	value, ok := f.snapshots[name]
	if !ok {
		return PubSubSnapshot{}, errors.New("snapshot not found")
	}
	value.Labels = cloneMetadata(value.Labels)
	return value, nil
}

func (f *fakePubSub) SetSnapshotLabels(_ context.Context, name string, labels map[string]string) (PubSubSnapshot, error) {
	f.labelSets++
	value, ok := f.snapshots[name]
	if !ok {
		return PubSubSnapshot{}, errors.New("snapshot not found")
	}
	value.Labels = cloneMetadata(labels)
	f.snapshots[name] = value
	return value, nil
}

func (f *fakePubSub) Seek(_ context.Context, subscription, snapshot string) error {
	if _, ok := f.snapshots[snapshot]; !ok {
		return errors.New("snapshot not found")
	}
	f.seeks = append(f.seeks, subscription+"<-"+snapshot)
	return nil
}

func (f *fakePubSub) ListSnapshots(_ context.Context, _ string) ([]PubSubSnapshot, error) {
	values := make([]PubSubSnapshot, 0, len(f.snapshots))
	for _, value := range f.snapshots {
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool { return values[i].Name < values[j].Name })
	return values, nil
}

func (f *fakePubSub) DeleteSnapshot(_ context.Context, name string) error {
	if _, ok := f.snapshots[name]; !ok {
		return errors.New("snapshot not found")
	}
	delete(f.snapshots, name)
	f.deletes = append(f.deletes, name)
	return nil
}

func (f *fakePubSub) CreateSubscription(_ context.Context, name, topic string, labels map[string]string) (PubSubSubscription, error) {
	if _, exists := f.subscriptions[name]; exists {
		return PubSubSubscription{}, errors.New("already exists")
	}
	subscription := PubSubSubscription{Name: name, Topic: topic, Labels: cloneMetadata(labels)}
	f.subscriptions[name] = subscription
	return subscription, nil
}

func (f *fakePubSub) ListSubscriptions(_ context.Context, _ string) ([]PubSubSubscription, error) {
	values := make([]PubSubSubscription, 0, len(f.subscriptions))
	for _, value := range f.subscriptions {
		copied := value
		copied.Labels = cloneMetadata(value.Labels)
		values = append(values, copied)
	}
	sort.Slice(values, func(i, j int) bool { return values[i].Name < values[j].Name })
	return values, nil
}

func (f *fakePubSub) DeleteSubscription(_ context.Context, name string) error {
	if _, ok := f.subscriptions[name]; !ok {
		return errors.New("subscription not found")
	}
	delete(f.subscriptions, name)
	f.deletes = append(f.deletes, name)
	return nil
}

func TestNativeAPIPubSubSnapshotRecoveryIsIdempotentAndOwned(t *testing.T) {
	pubsub := newFakePubSub()
	pubsub.subscriptions["projects/demo/subscriptions/orders"] = PubSubSubscription{
		Name: "projects/demo/subscriptions/orders", Topic: "projects/demo/topics/orders",
		Labels: secretLabelsFor(cloudrecovery.OperationState{FixtureID: "fixture/queue", OwnershipMarker: "owner", DataClass: "queue"}),
	}
	native, err := NewNativeAPIWithSQLAndProjectionAndPubSub(newFakeGCS(), newFakeSecretAPI(), nil, pubsub, NativeAPIConfig{
		ArchiveBucket: "archive", Project: "demo", RetentionDays: 7, Now: func() time.Time { return pubsub.now }, Verifier: fakeRecoveryVerifier{},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceBackup, DataClasses: []string{"queue"}, Destination: sdk.RecoverySameRegion,
		FixtureID: "fixture/queue", OwnershipMarker: "owner", IdempotencyKey: "queue/backup/1",
		ResourceReferences: map[string]string{"queue": "gcp-pubsub://projects/demo/subscriptions/orders"},
	}
	backup, err := client.Start(context.Background(), request)
	if err != nil || backup.Status != sdk.ResilienceOperationSucceeded {
		t.Fatalf("queue backup = %#v err=%v", backup, err)
	}
	if len(backup.Evidence) != 1 || backup.Evidence[0].RetentionDays != 7 || !backup.Evidence[0].EncryptionVerified {
		t.Fatalf("queue backup evidence = %#v", backup.Evidence)
	}
	seeksBefore := len(pubsub.seeks)
	reused, err := client.Start(context.Background(), request)
	if err != nil || reused.Status != sdk.ResilienceOperationSucceeded || len(pubsub.seeks) != seeksBefore {
		t.Fatalf("idempotent queue backup = %#v seeks=%d err=%v", reused, len(pubsub.seeks), err)
	}

	restore := request
	restore.Action = sdk.ResilienceRestore
	restore.IdempotencyKey = "queue/restore/1"
	restore.BackupReferences = map[string]string{"queue": backup.Evidence[0].BackupID}
	restored, err := client.Start(context.Background(), restore)
	if err != nil || restored.Status != sdk.ResilienceOperationSucceeded || len(pubsub.seeks) != seeksBefore+1 {
		t.Fatalf("queue restore = %#v seeks=%d err=%v", restored, len(pubsub.seeks), err)
	}

	integrity := restore
	integrity.Action = sdk.ResilienceIntegrityCheck
	integrity.IdempotencyKey = "queue/integrity/1"
	checked, err := client.Start(context.Background(), integrity)
	if err != nil || checked.Status != sdk.ResilienceOperationSucceeded || !checked.Evidence[0].CountsVerified || !checked.Evidence[0].ServiceHealthVerified {
		t.Fatalf("queue integrity = %#v err=%v", checked, err)
	}

	inventory, err := native.Inventory(context.Background(), "owner")
	if err != nil || len(inventory) != 1 || !strings.Contains(inventory[0].Identity, "gcp-pubsub-snapshot://projects/demo/snapshots/") {
		t.Fatalf("queue inventory = %#v err=%v", inventory, err)
	}
	if err := native.DeleteOwnedPubSubSnapshots(context.Background(), "owner"); err != nil {
		t.Fatal(err)
	}
	if len(pubsub.snapshots) != 0 || len(pubsub.deletes) != 1 {
		t.Fatalf("queue cleanup snapshots=%v deletes=%v", pubsub.snapshots, pubsub.deletes)
	}
}

func TestNativeAPIPubSubIsolatedRestoreSeeksOwnedSubscriptionWithoutSeekingSource(t *testing.T) {
	pubsub := newFakePubSub()
	source := "projects/demo/subscriptions/orders"
	pubsub.subscriptions[source] = PubSubSubscription{
		Name: source, Topic: "projects/demo/topics/orders",
		Labels: secretLabelsFor(cloudrecovery.OperationState{FixtureID: "fixture/queue", OwnershipMarker: "owner", DataClass: "queue"}),
	}
	native, err := NewNativeAPIWithSQLAndProjectionAndPubSub(newFakeGCS(), newFakeSecretAPI(), nil, pubsub, NativeAPIConfig{
		ArchiveBucket: "archive", Project: "demo", RetentionDays: 7, Now: func() time.Time { return pubsub.now }, Verifier: fakeRecoveryVerifier{},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceBackup, DataClasses: []string{"queue"}, Destination: sdk.RecoverySameRegionIsolated,
		FixtureID: "fixture/queue", OwnershipMarker: "owner", IdempotencyKey: "queue/backup/isolated",
		ResourceReferences: map[string]string{"queue": "gcp-pubsub://projects/demo/subscriptions/orders"},
	}
	backup, err := client.Start(context.Background(), request)
	if err != nil || backup.Status != sdk.ResilienceOperationSucceeded {
		t.Fatalf("queue backup = %#v err=%v", backup, err)
	}
	restore := request
	restore.Action = sdk.ResilienceRestore
	restore.IdempotencyKey = "queue/restore/isolated"
	restore.BackupReferences = map[string]string{"queue": backup.Evidence[0].BackupID}
	restored, err := client.Start(context.Background(), restore)
	if err != nil || restored.Status != sdk.ResilienceOperationSucceeded {
		t.Fatalf("isolated queue restore = %#v err=%v", restored, err)
	}
	isolatedName := pubSubIsolatedSubscriptionName("demo", cloudrecovery.OperationState{
		FixtureID: "fixture/queue", OwnershipMarker: "owner", DataClass: "queue", IdempotencyKey: "queue/restore/isolated",
	})
	if restored.Evidence[0].RestoreID != isolatedName {
		t.Fatalf("isolated restore id = %q, want %q", restored.Evidence[0].RestoreID, isolatedName)
	}
	if _, ok := pubsub.subscriptions[source]; !ok {
		t.Fatal("isolated restore deleted the source subscription")
	}
	isolated, ok := pubsub.subscriptions[isolatedName]
	if !ok || isolated.Topic != "projects/demo/topics/orders" || isolated.Labels[recoveryRoleLabelKey] != isolatedRestoreRole {
		t.Fatalf("isolated restore subscription = %#v", isolated)
	}
	if len(pubsub.seeks) != 1 || pubsub.seeks[0] != isolatedName+"<-"+backup.Evidence[0].BackupID {
		t.Fatalf("isolated restore seeks = %v", pubsub.seeks)
	}

	inventory, err := native.Inventory(context.Background(), "owner")
	if err != nil || len(inventory) != 2 {
		t.Fatalf("isolated inventory = %#v err=%v", inventory, err)
	}
	if err := native.DeleteOwnedPubSubSnapshots(context.Background(), "owner"); err != nil {
		t.Fatal(err)
	}
	if _, ok := pubsub.subscriptions[source]; !ok {
		t.Fatal("cleanup deleted the source subscription")
	}
	if _, ok := pubsub.subscriptions[isolatedName]; ok || len(pubsub.snapshots) != 0 {
		t.Fatalf("cleanup left restore resources snapshots=%v subscriptions=%v", pubsub.snapshots, pubsub.subscriptions)
	}
}

func TestNativeAPIPubSubStillRefusesAlternateRegionRestoreBeforeMutation(t *testing.T) {
	pubsub := newFakePubSub()
	pubsub.subscriptions["projects/demo/subscriptions/orders"] = PubSubSubscription{
		Name: "projects/demo/subscriptions/orders", Topic: "projects/demo/topics/orders",
		Labels: secretLabelsFor(cloudrecovery.OperationState{FixtureID: "fixture/queue", OwnershipMarker: "owner", DataClass: "queue"}),
	}
	native, err := NewNativeAPIWithSQLAndProjectionAndPubSub(newFakeGCS(), newFakeSecretAPI(), nil, pubsub, NativeAPIConfig{
		ArchiveBucket: "archive", Project: "demo", RetentionDays: 7, Now: func() time.Time { return pubsub.now }, Verifier: fakeRecoveryVerifier{},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceBackup, DataClasses: []string{"queue"}, Destination: sdk.RecoverySameRegion,
		FixtureID: "fixture/queue", OwnershipMarker: "owner", IdempotencyKey: "queue/backup/alt",
		ResourceReferences: map[string]string{"queue": "gcp-pubsub://projects/demo/subscriptions/orders"},
	}
	backup, err := client.Start(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	restore := request
	restore.Action = sdk.ResilienceRestore
	restore.Destination = sdk.RecoveryAlternateRegion
	restore.IdempotencyKey = "queue/restore/alt"
	restore.BackupReferences = map[string]string{"queue": backup.Evidence[0].BackupID}
	_, err = client.Start(context.Background(), restore)
	var capability sdk.ResilienceCapabilityError
	if !errors.As(err, &capability) || capability.Status != sdk.ResilienceCapabilityUnsupported {
		t.Fatalf("error = %v, want unsupported capability", err)
	}
	if len(pubsub.seeks) != 0 {
		t.Fatalf("alternate-region restore mutated Pub/Sub seeks=%v", pubsub.seeks)
	}
	if len(pubsub.subscriptions) != 1 {
		t.Fatalf("alternate-region restore created subscriptions: %#v", pubsub.subscriptions)
	}
}

func TestNativeAPIPubSubWaitsForFixtureBeforeCreatingSnapshot(t *testing.T) {
	pubsub := newFakePubSub()
	state := cloudrecovery.OperationState{FixtureID: "fixture/queue", OwnershipMarker: "owner", DataClass: "queue", IdempotencyKey: "queue/backup/1"}
	pubsub.subscriptions["projects/demo/subscriptions/orders"] = PubSubSubscription{
		Name: "projects/demo/subscriptions/orders", Topic: "projects/demo/topics/orders", Labels: secretLabelsFor(state),
	}
	verifier := &pubSubReadinessVerifier{pubsub: pubsub}
	native, err := NewNativeAPIWithSQLAndProjectionAndPubSub(newFakeGCS(), newFakeSecretAPI(), nil, pubsub, NativeAPIConfig{
		ArchiveBucket: "archive", Project: "demo", RetentionDays: 7, Now: func() time.Time { return pubsub.now }, Verifier: verifier,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceBackup, DataClasses: []string{"queue"}, Destination: sdk.RecoverySameRegion,
		FixtureID: state.FixtureID, OwnershipMarker: state.OwnershipMarker, IdempotencyKey: state.IdempotencyKey,
		ResourceReferences: map[string]string{"queue": "gcp-pubsub://projects/demo/subscriptions/orders"},
	}
	if _, err := client.Start(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if verifier.waits != 1 {
		t.Fatalf("fixture readiness calls = %d, want 1", verifier.waits)
	}
	if verifier.sawSnapshotDuringWait {
		t.Fatal("fixture readiness ran after snapshot creation")
	}

	verifier.err = errors.New("fixture was not ready")
	request.IdempotencyKey = "queue/backup/2"
	if _, err := client.Start(context.Background(), request); err == nil || !strings.Contains(err.Error(), "before snapshot") {
		t.Fatalf("fixture readiness failure = %v", err)
	}
	if len(pubsub.snapshots) != 1 {
		t.Fatalf("fixture readiness failure created a snapshot: %#v", pubsub.snapshots)
	}
}

func TestNativeAPIPubSubRejectsTopicReferenceAndLongRetentionBeforeMutation(t *testing.T) {
	pubsub := newFakePubSub()
	native, err := NewNativeAPIWithSQLAndProjectionAndPubSub(newFakeGCS(), newFakeSecretAPI(), nil, pubsub, NativeAPIConfig{ArchiveBucket: "archive", Project: "demo", RetentionDays: 30}, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := cloudresilience.NativeOperationRequest{Provider: "gcp", Operation: "gcp.pubsub.backup.queue", Request: sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceBackup, DataClasses: []string{"queue"}, FixtureID: "fixture", OwnershipMarker: "owner", IdempotencyKey: "queue/1",
		ResourceReferences: map[string]string{"queue": "gcp-pubsub://projects/demo/topics/orders"},
	}}
	_, err = native.Start(context.Background(), request)
	if !strings.Contains(err.Error(), "maximum seven-day retention") {
		t.Fatalf("long retention error = %v", err)
	}
	if len(pubsub.snapshots) != 0 {
		t.Fatalf("long retention mutated Pub/Sub: %#v", pubsub.snapshots)
	}

	native.config.RetentionDays = 7
	request.Request.ResourceReferences["queue"] = "gcp-pubsub://projects/demo/subscriptions/orders"
	request.Request.IdempotencyKey = "queue/2"
	_, err = native.Start(context.Background(), request)
	if !strings.Contains(err.Error(), "subscription not found") {
		t.Fatalf("missing subscription error = %v", err)
	}
}

func TestNativeAPIPubSubRefusesSnapshotLabelCollisionWithoutMutation(t *testing.T) {
	pubsub := newFakePubSub()
	state := cloudrecovery.OperationState{FixtureID: "fixture/queue", OwnershipMarker: "owner", DataClass: "queue", IdempotencyKey: "queue/backup/1"}
	pubsub.subscriptions["projects/demo/subscriptions/orders"] = PubSubSubscription{
		Name: "projects/demo/subscriptions/orders", Topic: "projects/demo/topics/orders", Labels: secretLabelsFor(state),
	}
	snapshotName := pubSubSnapshotName("demo", state)
	pubsub.snapshots[snapshotName] = PubSubSnapshot{
		Name: snapshotName, Topic: "projects/demo/topics/orders", ExpireTime: pubsub.now.Add(7 * 24 * time.Hour),
		Labels: map[string]string{ownershipLabelKey: "owner", classLabelKey: "queue", fixtureLabelKey: "another-fixture"}, EncryptionVerified: true,
	}
	native, err := NewNativeAPIWithSQLAndProjectionAndPubSub(newFakeGCS(), newFakeSecretAPI(), nil, pubsub, NativeAPIConfig{
		ArchiveBucket: "archive", Project: "demo", RetentionDays: 7, Now: func() time.Time { return pubsub.now }, Verifier: fakeRecoveryVerifier{},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceBackup, DataClasses: []string{"queue"}, Destination: sdk.RecoverySameRegion,
		FixtureID: state.FixtureID, OwnershipMarker: state.OwnershipMarker, IdempotencyKey: state.IdempotencyKey,
		ResourceReferences: map[string]string{"queue": "gcp-pubsub://projects/demo/subscriptions/orders"},
	}
	_, err = client.Start(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "not owned") {
		t.Fatalf("label collision error = %v", err)
	}
	if pubsub.labelSets != 0 {
		t.Fatalf("label collision mutated snapshot labels %d times", pubsub.labelSets)
	}
	if pubsub.snapshots[snapshotName].Labels[fixtureLabelKey] != "another-fixture" {
		t.Fatalf("label collision changed existing labels: %#v", pubsub.snapshots[snapshotName].Labels)
	}
}

func TestNativeAPIPubSubRequiresExplicitCMEKCapability(t *testing.T) {
	pubsub := newFakePubSub()
	native, err := NewNativeAPIWithSQLAndProjectionAndPubSub(newFakeGCS(), newFakeSecretAPI(), nil, pubsub, NativeAPIConfig{
		ArchiveBucket: "archive", Project: "demo", RetentionDays: 7, RequireCMEK: true, KMSKeyName: "projects/demo/locations/global/keyRings/test/cryptoKeys/test",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := cloudresilience.NativeOperationRequest{Provider: "gcp", Operation: "gcp.pubsub.backup.queue", Request: sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceBackup, DataClasses: []string{"queue"}, FixtureID: "fixture", OwnershipMarker: "owner", IdempotencyKey: "queue/1",
		ResourceReferences: map[string]string{"queue": "gcp-pubsub://projects/demo/subscriptions/orders"},
	}}
	_, err = native.Start(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "customer-managed encryption key") {
		t.Fatalf("CMEK error = %v", err)
	}
	if len(pubsub.snapshots) != 0 {
		t.Fatalf("CMEK capability failure mutated Pub/Sub: %#v", pubsub.snapshots)
	}
}

type pubSubReadinessVerifier struct {
	pubsub                *fakePubSub
	waits                 int
	sawSnapshotDuringWait bool
	err                   error
}

func (v *pubSubReadinessVerifier) WaitForFixture(context.Context, RecoveryVerificationRequest) error {
	v.waits++
	if len(v.pubsub.snapshots) != 0 {
		v.sawSnapshotDuringWait = true
	}
	return v.err
}

func (v *pubSubReadinessVerifier) Verify(context.Context, RecoveryVerificationRequest) (RecoveryVerification, error) {
	return RecoveryVerification{CountsVerified: true, ApplicationReadsVerified: true, PermissionsVerified: true, ServiceHealthVerified: true}, nil
}
