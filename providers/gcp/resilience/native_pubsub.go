package resilience

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	provider "github.com/magelift/magelift/internal/provider"
	cloudrecovery "github.com/magelift/magelift/internal/shared/recovery"
	"github.com/magelift/magelift/sdk"
)

// PubSubSnapshot is the provider-local view of a Pub/Sub snapshot. The
// generated client model is translated here so the shared recovery contract
// never depends on Pub/Sub protobufs.
type PubSubSnapshot struct {
	Name               string
	Topic              string
	ExpireTime         time.Time
	Labels             map[string]string
	EncryptionVerified bool
}

// PubSubSubscription is the provider-local view needed to verify that a
// snapshot belongs to the source subscription and that the source is owned by
// the current recovery operation.
type PubSubSubscription struct {
	Name   string
	Topic  string
	Labels map[string]string
}

// PubSubAPI is the narrow provider-owned port for the official Pub/Sub
// snapshot/seek APIs. It is intentionally injectable for deterministic tests
// and community-maintained GCP-compatible implementations.
type PubSubAPI interface {
	GetSubscription(context.Context, string) (PubSubSubscription, error)
	CreateSnapshot(context.Context, string, string) (PubSubSnapshot, error)
	GetSnapshot(context.Context, string) (PubSubSnapshot, error)
	SetSnapshotLabels(context.Context, string, map[string]string) (PubSubSnapshot, error)
	Seek(context.Context, string, string) error
	ListSnapshots(context.Context, string) ([]PubSubSnapshot, error)
	DeleteSnapshot(context.Context, string) error
	CreateSubscription(context.Context, string, string, map[string]string) (PubSubSubscription, error)
	ListSubscriptions(context.Context, string) ([]PubSubSubscription, error)
	DeleteSubscription(context.Context, string) error
}

type pubSubSubscriptionReference struct {
	Project      string
	Subscription string
}

func (api *NativeAPI) startQueue(ctx context.Context, state cloudrecovery.OperationState, operationID string) (provider.NativeOperationObservation, error) {
	if api == nil || api.pubsub == nil {
		return provider.NativeOperationObservation{}, capabilityError(state, "GCP Pub/Sub recovery translator is not configured")
	}
	if api.retentionDays() > 7 {
		return provider.NativeOperationObservation{}, capabilityError(state, "GCP Pub/Sub snapshots have a maximum seven-day retention boundary; configure a queue retention target of seven days or less")
	}
	if api.config.RequireCMEK {
		return provider.NativeOperationObservation{}, capabilityError(state, "GCP Pub/Sub queue recovery cannot prove a customer-managed encryption key with the configured adapter boundary")
	}
	reference, err := parsePubSubSubscriptionReference(state.Resource)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	subscription, err := api.pubsub.GetSubscription(ctx, subscriptionName(reference))
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect GCP Pub/Sub subscription: %w", err)
	}
	if err := api.verifyPubSubSubscription(subscription, state); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	switch state.Action {
	case sdk.ResilienceBackup:
		return api.backupQueue(ctx, state, operationID, reference, subscription)
	case sdk.ResilienceRestore:
		return api.restoreQueue(ctx, state, operationID, reference, subscription)
	case sdk.ResilienceIntegrityCheck:
		return api.integrityQueue(ctx, state, operationID, reference, subscription)
	default:
		return provider.NativeOperationObservation{}, capabilityError(state, "GCP Pub/Sub recovery does not implement this action")
	}
}

func (api *NativeAPI) pollQueue(ctx context.Context, state cloudrecovery.OperationState, operationID string) (provider.NativeOperationObservation, error) {
	return api.startQueue(ctx, state, operationID)
}

func (api *NativeAPI) backupQueue(ctx context.Context, state cloudrecovery.OperationState, operationID string, reference pubSubSubscriptionReference, subscription PubSubSubscription) (provider.NativeOperationObservation, error) {
	snapshotName := pubSubSnapshotName(reference.Project, state)
	minimumExpiry := api.now().Add(time.Duration(api.retentionDays()) * 24 * time.Hour).Add(-time.Minute)
	snapshot, err := api.pubsub.GetSnapshot(ctx, snapshotName)
	created := false
	if err != nil {
		if !isPubSubNotFound(err) {
			return provider.NativeOperationObservation{}, fmt.Errorf("inspect GCP Pub/Sub snapshot: %w", err)
		}
		if readiness, ok := api.config.Verifier.(RecoveryFixtureReadiness); ok {
			request := RecoveryVerificationRequest{
				DataClass: state.DataClass, Resource: subscription.Name,
				FixtureID: state.FixtureID, OwnershipMarker: state.OwnershipMarker,
			}
			if err := readiness.WaitForFixture(ctx, request); err != nil {
				return provider.NativeOperationObservation{}, fmt.Errorf("wait for GCP Pub/Sub fixture before snapshot: %w", err)
			}
		}
		snapshot, err = api.pubsub.CreateSnapshot(ctx, snapshotName, subscription.Name)
		created = err == nil
		if err != nil {
			if isPubSubAlreadyExists(err) {
				snapshot, err = api.pubsub.GetSnapshot(ctx, snapshotName)
			} else {
				return provider.NativeOperationObservation{}, fmt.Errorf("create GCP Pub/Sub snapshot: %w", err)
			}
		}
		if err != nil {
			return provider.NativeOperationObservation{}, fmt.Errorf("inspect created GCP Pub/Sub snapshot: %w", err)
		}
	}
	if !created {
		if err := api.verifyPubSubSnapshot(snapshot, state, subscription.Topic, minimumExpiry); err != nil {
			return provider.NativeOperationObservation{}, err
		}
	}
	labels := secretLabelsFor(state)
	if created {
		snapshot, err = api.pubsub.SetSnapshotLabels(ctx, snapshot.Name, labels)
		if err != nil {
			return provider.NativeOperationObservation{}, fmt.Errorf("label GCP Pub/Sub snapshot: %w", err)
		}
	}
	if err := api.verifyPubSubSnapshot(snapshot, state, subscription.Topic, minimumExpiry); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	return api.queueObservation(state, operationID, snapshot, sdk.ResilienceProofEvidence{
		DataClass: state.DataClass, Destination: string(state.Destination), BackupID: snapshot.Name,
		FixtureID: state.FixtureID, RetentionDays: api.retentionDays(), EncryptionVerified: snapshot.EncryptionVerified,
		ProtectionVerified: true, ServiceHealthVerified: true,
		Reason: "created or reused an ownership-labeled Pub/Sub snapshot with the documented at-least-once seek boundary",
	})
}

func (api *NativeAPI) restoreQueue(ctx context.Context, state cloudrecovery.OperationState, operationID string, reference pubSubSubscriptionReference, subscription PubSubSubscription) (provider.NativeOperationObservation, error) {
	if state.Destination == sdk.RecoveryAlternateRegion {
		return provider.NativeOperationObservation{}, capabilityError(state, "GCP Pub/Sub snapshot seek restores same-region subscriptions only; alternate-region queue destinations require a separate subscription topology")
	}
	snapshotName, err := parsePubSubSnapshotReference(state.Backup)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	snapshot, err := api.pubsub.GetSnapshot(ctx, snapshotName)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect GCP Pub/Sub restore snapshot: %w", err)
	}
	if err := api.verifyPubSubSnapshot(snapshot, state, subscription.Topic, api.now()); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	target := subscription
	reason := "sought the owned Pub/Sub subscription to an ownership-labeled snapshot and verified queue health"
	if state.Destination == sdk.RecoverySameRegionIsolated {
		isolated, err := api.ensureIsolatedQueueSubscription(ctx, state, reference, subscription)
		if err != nil {
			return provider.NativeOperationObservation{}, err
		}
		target = isolated
		reason = "created an owned isolated Pub/Sub subscription, sought it to an ownership-labeled snapshot, and verified queue health without seeking the source subscription"
	}
	if err := api.pubsub.Seek(ctx, target.Name, snapshot.Name); err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("seek GCP Pub/Sub subscription to snapshot: %w", err)
	}
	verification, err := api.verifyQueueFixture(ctx, state, target.Name)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	evidence := queueEvidence(state, snapshot, verification, api.retentionDays())
	evidence.RestoreID = target.Name
	evidence.ServiceHealthVerified = verification.ServiceHealthVerified
	evidence.Reason = reason
	return api.queueObservation(state, operationID, snapshot, evidence)
}

func (api *NativeAPI) integrityQueue(ctx context.Context, state cloudrecovery.OperationState, operationID string, reference pubSubSubscriptionReference, subscription PubSubSubscription) (provider.NativeOperationObservation, error) {
	if state.Backup == "" {
		return provider.NativeOperationObservation{}, errors.New("GCP Pub/Sub queue integrity requires a snapshot reference")
	}
	snapshotName, err := parsePubSubSnapshotReference(state.Backup)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	snapshot, err := api.pubsub.GetSnapshot(ctx, snapshotName)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect GCP Pub/Sub integrity snapshot: %w", err)
	}
	if err := api.verifyPubSubSnapshot(snapshot, state, subscription.Topic, api.now()); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	verification, err := api.verifyQueueFixture(ctx, state, subscription.Name)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	evidence := queueEvidence(state, snapshot, verification, api.retentionDays())
	evidence.RestoreID = subscription.Name
	evidence.Reason = "verified the ownership-labeled Pub/Sub snapshot, subscription permissions, queue fixture, and service health"
	return api.queueObservation(state, operationID, snapshot, evidence)
}

func (api *NativeAPI) verifyQueueFixture(ctx context.Context, state cloudrecovery.OperationState, subscription string) (RecoveryVerification, error) {
	if api.config.Verifier == nil {
		return RecoveryVerification{}, errors.New("GCP Pub/Sub queue verification requires an application recovery verifier")
	}
	verification, err := api.config.Verifier.Verify(ctx, RecoveryVerificationRequest{DataClass: state.DataClass, Resource: subscription, FixtureID: state.FixtureID, OwnershipMarker: state.OwnershipMarker})
	if err != nil {
		return RecoveryVerification{}, fmt.Errorf("verify GCP Pub/Sub application fixture: %w", err)
	}
	if !verification.CountsVerified || !verification.ApplicationReadsVerified || !verification.PermissionsVerified || !verification.ServiceHealthVerified {
		return RecoveryVerification{}, errors.New("GCP Pub/Sub application verifier did not prove message counts, reads, permissions, and service health")
	}
	return verification, nil
}

func (api *NativeAPI) queueObservation(state cloudrecovery.OperationState, operationID string, snapshot PubSubSnapshot, evidence sdk.ResilienceProofEvidence) (provider.NativeOperationObservation, error) {
	if strings.TrimSpace(snapshot.Name) == "" {
		return provider.NativeOperationObservation{}, errors.New("GCP Pub/Sub snapshot returned no resource identity")
	}
	return observation(state, operationID, string(sdk.ResilienceOperationSucceeded), []string{snapshot.Name}, []string{"gcp.pubsub.snapshot", "gcp.pubsub.retention", "gcp.pubsub.ownership"}, []sdk.ResilienceProofEvidence{evidence}), nil
}

func queueEvidence(state cloudrecovery.OperationState, snapshot PubSubSnapshot, verification RecoveryVerification, retentionDays int) sdk.ResilienceProofEvidence {
	return sdk.ResilienceProofEvidence{
		DataClass: state.DataClass, Destination: string(state.Destination), BackupID: snapshot.Name,
		FixtureID: state.FixtureID, RetentionDays: retentionDays, EncryptionVerified: snapshot.EncryptionVerified,
		ProtectionVerified: true, CountsVerified: verification.CountsVerified,
		ApplicationReadsVerified: verification.ApplicationReadsVerified, PermissionsVerified: verification.PermissionsVerified,
		SecretReferencesVerified: verification.SecretReferencesVerified, ServiceHealthVerified: verification.ServiceHealthVerified,
	}
}

func (api *NativeAPI) verifyPubSubSubscription(subscription PubSubSubscription, state cloudrecovery.OperationState) error {
	if strings.TrimSpace(subscription.Name) == "" || strings.TrimSpace(subscription.Topic) == "" {
		return errors.New("GCP Pub/Sub subscription response is incomplete")
	}
	if !recoveryLabelsMatch(subscription.Labels, state) {
		return errors.New("GCP Pub/Sub subscription is not owned by this operation")
	}
	return nil
}

func (api *NativeAPI) verifyPubSubSnapshot(snapshot PubSubSnapshot, state cloudrecovery.OperationState, topic string, minimumExpiry time.Time) error {
	if strings.TrimSpace(snapshot.Name) == "" || strings.TrimSpace(snapshot.Topic) == "" {
		return errors.New("GCP Pub/Sub snapshot response is incomplete")
	}
	if snapshot.Topic != topic {
		return errors.New("GCP Pub/Sub snapshot topic does not match the source subscription")
	}
	if !recoveryLabelsMatch(snapshot.Labels, state) {
		return errors.New("GCP Pub/Sub snapshot is not owned by this operation")
	}
	if !snapshot.EncryptionVerified {
		return errors.New("GCP Pub/Sub snapshot encryption was not verified")
	}
	if snapshot.ExpireTime.Before(minimumExpiry) {
		if minimumExpiry.After(api.now()) {
			return fmt.Errorf("GCP Pub/Sub snapshot expires before the requested %d-day retention target", api.retentionDays())
		}
		return errors.New("GCP Pub/Sub snapshot has expired")
	}
	return nil
}

func (api *NativeAPI) DeleteOwnedPubSubSnapshots(ctx context.Context, marker string) error {
	if api == nil || api.pubsub == nil {
		return errors.New("GCP Pub/Sub recovery translator is not configured")
	}
	if strings.TrimSpace(marker) == "" || strings.ContainsAny(marker, "\r\n\x00") {
		return errors.New("GCP Pub/Sub cleanup ownership marker is required")
	}
	project := strings.TrimSpace(api.config.Project)
	if project == "" {
		return errors.New("GCP Pub/Sub cleanup project is required")
	}
	subscriptions, err := api.pubsub.ListSubscriptions(ctx, project)
	if err != nil {
		return fmt.Errorf("list GCP Pub/Sub subscriptions: %w", err)
	}
	for _, subscription := range subscriptions {
		if !isolatedRestoreSubscriptionOwned(subscription.Labels, marker) {
			continue
		}
		if err := api.pubsub.DeleteSubscription(ctx, subscription.Name); err != nil && !isPubSubNotFound(err) {
			return fmt.Errorf("delete owned GCP Pub/Sub isolated restore subscription %q: %w", subscription.Name, err)
		}
	}
	snapshots, err := api.pubsub.ListSnapshots(ctx, project)
	if err != nil {
		return fmt.Errorf("list GCP Pub/Sub snapshots: %w", err)
	}
	for _, snapshot := range snapshots {
		if snapshot.Labels[ownershipLabelKey] != marker || snapshot.Labels[classLabelKey] != "queue" {
			continue
		}
		if err := api.pubsub.DeleteSnapshot(ctx, snapshot.Name); err != nil && !isPubSubNotFound(err) {
			return fmt.Errorf("delete owned GCP Pub/Sub snapshot %q: %w", snapshot.Name, err)
		}
	}
	return nil
}

func (api *NativeAPI) now() time.Time {
	if api != nil && api.config.Now != nil {
		return api.config.Now().UTC()
	}
	return time.Now().UTC()
}

func parsePubSubSubscriptionReference(reference string) (pubSubSubscriptionReference, error) {
	if strings.ContainsAny(reference, "\r\n\x00?#") {
		return pubSubSubscriptionReference{}, fmt.Errorf("Pub/Sub resource reference %q contains unsupported control or query characters", reference)
	}
	value := normalizePubSubReference(reference, "gcp-pubsub://")
	parts := strings.Split(strings.Trim(value, "/"), "/")
	if len(parts) != 4 || parts[0] != "projects" || parts[2] != "subscriptions" || parts[1] == "" || parts[3] == "" {
		return pubSubSubscriptionReference{}, fmt.Errorf("Pub/Sub resource reference %q must use gcp-pubsub://projects/PROJECT/subscriptions/SUBSCRIPTION", reference)
	}
	return pubSubSubscriptionReference{Project: parts[1], Subscription: parts[3]}, nil
}

func parsePubSubSnapshotReference(reference string) (string, error) {
	if strings.ContainsAny(reference, "\r\n\x00?#") {
		return "", fmt.Errorf("Pub/Sub snapshot reference %q contains unsupported control or query characters", reference)
	}
	value := normalizePubSubReference(reference, "gcp-pubsub-snapshot://")
	parts := strings.Split(strings.Trim(value, "/"), "/")
	if len(parts) != 4 || parts[0] != "projects" || parts[2] != "snapshots" || parts[1] == "" || parts[3] == "" {
		return "", fmt.Errorf("Pub/Sub snapshot reference %q must use gcp-pubsub-snapshot://projects/PROJECT/snapshots/SNAPSHOT", reference)
	}
	return strings.Trim(value, "/"), nil
}

func normalizePubSubReference(reference, scheme string) string {
	value := strings.TrimSpace(reference)
	if strings.HasPrefix(strings.ToLower(value), scheme) {
		value = value[len(scheme):]
	}
	return strings.TrimPrefix(value, "//")
}

func subscriptionName(reference pubSubSubscriptionReference) string {
	return "projects/" + reference.Project + "/subscriptions/" + reference.Subscription
}

func pubSubSnapshotName(project string, state cloudrecovery.OperationState) string {
	digest := shortDigest(state.OwnershipMarker + "\x00" + state.FixtureID + "\x00" + state.IdempotencyKey)
	return "projects/" + project + "/snapshots/magelift-" + digest
}

func pubSubIsolatedSubscriptionName(project string, state cloudrecovery.OperationState) string {
	digest := shortDigest(state.OwnershipMarker + "\x00" + state.FixtureID + "\x00" + state.IdempotencyKey)
	return "projects/" + project + "/subscriptions/magelift-restore-" + digest
}

func isolatedRestoreLabelsFor(state cloudrecovery.OperationState) map[string]string {
	labels := secretLabelsFor(state)
	labels[recoveryRoleLabelKey] = isolatedRestoreRole
	return labels
}

func isolatedRestoreSubscriptionOwned(labels map[string]string, marker string) bool {
	return labels[ownershipLabelKey] == marker && labels[classLabelKey] == "queue" && labels[recoveryRoleLabelKey] == isolatedRestoreRole
}

func (api *NativeAPI) ensureIsolatedQueueSubscription(ctx context.Context, state cloudrecovery.OperationState, reference pubSubSubscriptionReference, source PubSubSubscription) (PubSubSubscription, error) {
	name := pubSubIsolatedSubscriptionName(reference.Project, state)
	existing, err := api.pubsub.GetSubscription(ctx, name)
	if err == nil {
		if existing.Topic != source.Topic {
			return PubSubSubscription{}, errors.New("GCP Pub/Sub isolated restore subscription topic does not match the source subscription")
		}
		if !isolatedRestoreSubscriptionOwned(existing.Labels, state.OwnershipMarker) || existing.Labels[fixtureLabelKey] != state.FixtureID {
			return PubSubSubscription{}, errors.New("GCP Pub/Sub isolated restore subscription is not owned by this operation")
		}
		return existing, nil
	}
	if !isPubSubNotFound(err) {
		return PubSubSubscription{}, fmt.Errorf("inspect GCP Pub/Sub isolated restore subscription: %w", err)
	}
	created, err := api.pubsub.CreateSubscription(ctx, name, source.Topic, isolatedRestoreLabelsFor(state))
	if err != nil {
		if isPubSubAlreadyExists(err) {
			existing, getErr := api.pubsub.GetSubscription(ctx, name)
			if getErr != nil {
				return PubSubSubscription{}, fmt.Errorf("inspect created GCP Pub/Sub isolated restore subscription: %w", getErr)
			}
			if existing.Topic != source.Topic || !isolatedRestoreSubscriptionOwned(existing.Labels, state.OwnershipMarker) || existing.Labels[fixtureLabelKey] != state.FixtureID {
				return PubSubSubscription{}, errors.New("GCP Pub/Sub isolated restore subscription is not owned by this operation")
			}
			return existing, nil
		}
		return PubSubSubscription{}, fmt.Errorf("create GCP Pub/Sub isolated restore subscription: %w", err)
	}
	if created.Topic != source.Topic || !isolatedRestoreSubscriptionOwned(created.Labels, state.OwnershipMarker) {
		return PubSubSubscription{}, errors.New("GCP Pub/Sub isolated restore subscription response is incomplete")
	}
	return created, nil
}

func recoveryLabelsMatch(labels map[string]string, state cloudrecovery.OperationState) bool {
	return secretLabelsMatch(labels, state.OwnershipMarker, state.DataClass) && labels[fixtureLabelKey] == state.FixtureID
}

func isPubSubNotFound(err error) bool {
	if err == nil {
		return false
	}
	value := strings.ToLower(err.Error())
	return strings.Contains(value, "not found") || strings.Contains(value, "not_found") || strings.Contains(value, "404")
}

func isPubSubAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	value := strings.ToLower(err.Error())
	return strings.Contains(value, "already exists") || strings.Contains(value, "already_exists") || strings.Contains(value, "409")
}

func sortedSnapshots(snapshots []PubSubSnapshot) []PubSubSnapshot {
	result := append([]PubSubSnapshot(nil), snapshots...)
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}
