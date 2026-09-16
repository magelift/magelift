package resilience

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	aws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"

	provider "github.com/magelift/magelift/internal/provider"
	"github.com/magelift/magelift/sdk"
)

const (
	sqsExportQueuePrefix  = "magelift-export-"
	sqsRestoreQueuePrefix = "magelift-restore-"
	sqsFixtureTagKey      = "magelift.io/fixture"
	sqsStatusTagKey       = "magelift.io/status"
	sqsSourceDigestTagKey = "magelift.io/source-digest"
	sqsBackupDigestTagKey = "magelift.io/backup-digest"
	sqsStatusCreating     = "creating"
	sqsStatusComplete     = "complete"
	sqsMaximumRetention   = 14
	sqsBatchSize          = 10
)

type observedSQSQueue struct {
	URL        string
	Attributes map[string]string
	Tags       map[string]string
	Available  int64
}

func (api *NativeAPI) startQueue(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	if api == nil || api.sqs == nil {
		return provider.NativeOperationObservation{}, capabilityError(state, "AWS SQS recovery translator is not configured")
	}
	if err := api.validateSQSProfile(state); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if state.Action == sdk.ResilienceCleanup {
		return api.cleanupSQS(ctx, state, operationID)
	}
	switch state.Action {
	case sdk.ResilienceBackup:
		if strings.TrimSpace(state.ApprovalReference) == "" {
			return provider.NativeOperationObservation{}, capabilityError(state, "AWS SQS export requires an operator-approved quiescence reference because SQS has no snapshot or writer-fencing API")
		}
		sourceURL, err := parseSQSReference(state.Resource)
		if err != nil {
			return provider.NativeOperationObservation{}, err
		}
		source, err := api.inspectSQSQueue(ctx, sourceURL, state, true)
		if err != nil {
			return provider.NativeOperationObservation{}, fmt.Errorf("inspect AWS SQS source queue: %w", err)
		}
		return api.backupSQS(ctx, state, operationID, source)
	case sdk.ResilienceRestore:
		return api.restoreSQS(ctx, state, operationID)
	case sdk.ResilienceIntegrityCheck:
		return api.integritySQS(ctx, state, operationID)
	default:
		return provider.NativeOperationObservation{}, capabilityError(state, "AWS SQS recovery does not implement this action")
	}
}

func (api *NativeAPI) pollQueue(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	return api.startQueue(ctx, state, operationID)
}

func (api *NativeAPI) validateSQSProfile(state operationState) error {
	if api.retentionDays() <= 0 || api.retentionDays() > sqsMaximumRetention {
		return capabilityError(state, "AWS SQS message retention is bounded to the documented fourteen-day maximum")
	}
	if api.config.QueueMaxMessages <= 0 {
		return errors.New("AWS SQS recovery message limit must be positive")
	}
	if api.config.QueueVisibilityTimeout <= 0 || api.config.QueueVisibilityTimeout > 43200 {
		return errors.New("AWS SQS recovery visibility timeout must be between one second and twelve hours")
	}
	return nil
}

func (api *NativeAPI) backupSQS(ctx context.Context, state operationState, operationID string, source observedSQSQueue) (provider.NativeOperationObservation, error) {
	queueName := sqsRecoveryQueueName(sqsExportQueuePrefix, state)
	queue, created, err := api.ensureSQSRecoveryQueue(ctx, queueName, sqsExportQueueAttributes(api), sqsExportTags(state), state)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("prepare AWS SQS export queue: %w", err)
	}
	if queue.Tags[sqsSourceDigestTagKey] != shortDigest(sqsReference(source.URL)) {
		return provider.NativeOperationObservation{}, errors.New("AWS SQS export queue source identity does not match the requested source")
	}
	if !created {
		if queue.Tags[sqsStatusTagKey] != sqsStatusComplete {
			return provider.NativeOperationObservation{}, errors.New("AWS SQS export queue exists without a complete marker; refusing to replay a partial export")
		}
		return api.sqsObservation(state, operationID, queue, sqsProofEvidence(state, queue, "reused the completed ownership-scoped SQS export queue"))
	}
	if source.Available > int64(api.config.QueueMaxMessages) {
		return provider.NativeOperationObservation{}, fmt.Errorf("AWS SQS source queue has approximately %d available messages, above the configured recovery limit of %d", source.Available, api.config.QueueMaxMessages)
	}
	if _, err := api.copySQSMessages(ctx, source.URL, queue.URL, source.Available); err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("export AWS SQS messages: %w", err)
	}
	if err := api.tagSQSQueue(ctx, queue.URL, map[string]string{sqsStatusTagKey: sqsStatusComplete}); err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("seal AWS SQS export queue: %w", err)
	}
	queue, err = api.inspectSQSQueue(ctx, queue.URL, state, false)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("verify AWS SQS export queue: %w", err)
	}
	return api.sqsObservation(state, operationID, queue, sqsProofEvidence(state, queue, "created and sealed a bounded SQS export after operator-approved quiescence; the source queue was never deleted"))
}

func (api *NativeAPI) restoreSQS(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	if state.Destination != sdk.RecoverySameRegionIsolated {
		return provider.NativeOperationObservation{}, capabilityError(state, "AWS SQS replay only supports a new isolated queue in the same region; in-place and alternate-region recovery require a fencing and replication adapter")
	}
	backupURL, err := parseSQSReference(state.Backup)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("parse AWS SQS backup reference: %w", err)
	}
	backup, err := api.inspectSQSQueue(ctx, backupURL, state, false)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect AWS SQS export queue: %w", err)
	}
	if backup.Tags[sqsStatusTagKey] != sqsStatusComplete {
		return provider.NativeOperationObservation{}, errors.New("AWS SQS restore requires a sealed export queue")
	}

	queueName := sqsRecoveryQueueName(sqsRestoreQueuePrefix, state)
	queue, created, err := api.ensureSQSRecoveryQueue(ctx, queueName, sqsRestoreQueueAttributes(api), sqsRestoreTags(state, backupURL), state)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("prepare AWS SQS isolated restore queue: %w", err)
	}
	if queue.Tags[sqsBackupDigestTagKey] != shortDigest(backupURL) {
		return provider.NativeOperationObservation{}, errors.New("AWS SQS restore queue backup identity does not match the requested export")
	}
	if created {
		if _, err := api.copySQSMessages(ctx, backupURL, queue.URL, backup.Available); err != nil {
			return provider.NativeOperationObservation{}, fmt.Errorf("replay AWS SQS messages into isolated queue: %w", err)
		}
		if err := api.tagSQSQueue(ctx, queue.URL, map[string]string{sqsStatusTagKey: sqsStatusComplete}); err != nil {
			return provider.NativeOperationObservation{}, fmt.Errorf("seal AWS SQS isolated restore queue: %w", err)
		}
		queue, err = api.inspectSQSQueue(ctx, queue.URL, state, false)
		if err != nil {
			return provider.NativeOperationObservation{}, fmt.Errorf("verify AWS SQS isolated restore queue: %w", err)
		}
	} else if queue.Tags[sqsStatusTagKey] != sqsStatusComplete {
		return provider.NativeOperationObservation{}, errors.New("AWS SQS isolated restore queue exists without a complete marker; refusing to replay duplicate messages")
	}
	verification, err := api.verifySQSFixture(ctx, state, queue.URL)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	evidence := sqsProofEvidence(state, backup, "replayed the sealed SQS export into an ownership-scoped same-region isolated queue")
	evidence.RestoreID = queue.URL
	evidence.CountsVerified = verification.CountsVerified
	evidence.ApplicationReadsVerified = verification.ApplicationReadsVerified
	evidence.PermissionsVerified = verification.PermissionsVerified
	evidence.SecretReferencesVerified = verification.SecretReferencesVerified
	evidence.ServiceHealthVerified = verification.ServiceHealthVerified
	return api.sqsObservation(state, operationID, queue, evidence)
}

func (api *NativeAPI) integritySQS(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	backupURL, err := parseSQSReference(state.Backup)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("parse AWS SQS integrity backup reference: %w", err)
	}
	backup, err := api.inspectSQSQueue(ctx, backupURL, state, false)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect AWS SQS integrity export: %w", err)
	}
	if backup.Tags[sqsStatusTagKey] != sqsStatusComplete {
		return provider.NativeOperationObservation{}, errors.New("AWS SQS integrity requires a sealed export queue")
	}
	verification, err := api.verifySQSFixture(ctx, state, backup.URL)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	evidence := sqsProofEvidence(state, backup, "verified the sealed SQS export with an application-owned known-message probe")
	evidence.RestoreID = backup.URL
	evidence.CountsVerified = verification.CountsVerified
	evidence.ApplicationReadsVerified = verification.ApplicationReadsVerified
	evidence.PermissionsVerified = verification.PermissionsVerified
	evidence.SecretReferencesVerified = verification.SecretReferencesVerified
	evidence.ServiceHealthVerified = verification.ServiceHealthVerified
	return api.sqsObservation(state, operationID, backup, evidence)
}

func (api *NativeAPI) verifySQSFixture(ctx context.Context, state operationState, resource string) (RecoveryVerification, error) {
	if api.config.Verifier == nil {
		return RecoveryVerification{}, errors.New("AWS SQS recovery requires an application recovery verifier")
	}
	verification, err := api.config.Verifier.Verify(ctx, RecoveryVerificationRequest{DataClass: state.DataClass, Resource: resource, FixtureID: state.FixtureID, OwnershipMarker: state.OwnershipMarker})
	if err != nil {
		return RecoveryVerification{}, fmt.Errorf("verify AWS SQS application fixture: %w", err)
	}
	if !verification.CountsVerified || !verification.ApplicationReadsVerified || !verification.PermissionsVerified || !verification.ServiceHealthVerified {
		return RecoveryVerification{}, errors.New("AWS SQS application verifier did not prove message counts, reads, permissions, and service health")
	}
	return verification, nil
}

func (api *NativeAPI) cleanupSQS(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	resources, err := api.listOwnedSQSQueues(ctx, state.OwnershipMarker)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	refs := make([]string, 0, len(resources))
	for _, resource := range resources {
		if err := api.deleteOwnedSQSQueue(ctx, resource.Identity, state.OwnershipMarker); err != nil {
			return provider.NativeOperationObservation{}, err
		}
		refs = append(refs, resource.Identity)
	}
	sort.Strings(refs)
	return observationWithOperation(state, operationID, refs, []string{"aws.sqs.ownership-cleanup"}, nil, string(sdk.ResilienceOperationSucceeded)), nil
}

func (api *NativeAPI) ensureSQSRecoveryQueue(ctx context.Context, name string, attributes, tags map[string]string, state operationState) (observedSQSQueue, bool, error) {
	output, err := api.sqs.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{QueueName: aws.String(name)})
	if err == nil {
		if output == nil || strings.TrimSpace(aws.ToString(output.QueueUrl)) == "" {
			return observedSQSQueue{}, false, errors.New("AWS SQS GetQueueUrl returned no queue identity")
		}
		queue, inspectErr := api.inspectSQSQueue(ctx, aws.ToString(output.QueueUrl), state, false)
		if inspectErr != nil {
			return observedSQSQueue{}, false, inspectErr
		}
		if err := verifySQSRecoveryQueueConfiguration(queue, attributes, state); err != nil {
			return observedSQSQueue{}, false, err
		}
		return queue, false, nil
	}
	if !isSQSNotFound(err) {
		return observedSQSQueue{}, false, fmt.Errorf("get AWS SQS recovery queue URL: %w", err)
	}
	created, err := api.sqs.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: aws.String(name), Attributes: attributes, Tags: tags})
	if err != nil {
		if isSQSAlreadyExists(err) {
			return api.ensureSQSRecoveryQueue(ctx, name, attributes, tags, state)
		}
		return observedSQSQueue{}, false, err
	}
	if created == nil || strings.TrimSpace(aws.ToString(created.QueueUrl)) == "" {
		return observedSQSQueue{}, false, errors.New("AWS SQS CreateQueue returned no queue identity")
	}
	queue, err := api.inspectSQSQueue(ctx, aws.ToString(created.QueueUrl), state, false)
	if err != nil {
		_, _ = api.sqs.DeleteQueue(ctx, &sqs.DeleteQueueInput{QueueUrl: created.QueueUrl})
		return observedSQSQueue{}, false, err
	}
	if err := verifySQSRecoveryQueueConfiguration(queue, attributes, state); err != nil {
		_, _ = api.sqs.DeleteQueue(ctx, &sqs.DeleteQueueInput{QueueUrl: created.QueueUrl})
		return observedSQSQueue{}, false, err
	}
	return queue, true, nil
}

func verifySQSRecoveryQueueConfiguration(queue observedSQSQueue, expected map[string]string, state operationState) error {
	for _, name := range []sqstypes.QueueAttributeName{sqstypes.QueueAttributeNameMessageRetentionPeriod, sqstypes.QueueAttributeNameVisibilityTimeout} {
		if queue.Attributes[string(name)] != expected[string(name)] {
			return fmt.Errorf("AWS SQS recovery queue attribute %q drifted from the requested value", name)
		}
	}
	if expected[string(sqstypes.QueueAttributeNameKmsMasterKeyId)] != "" {
		if queue.Attributes[string(sqstypes.QueueAttributeNameKmsMasterKeyId)] != expected[string(sqstypes.QueueAttributeNameKmsMasterKeyId)] {
			return errors.New("AWS SQS recovery queue KMS key drifted from the requested key")
		}
	} else if !strings.EqualFold(queue.Attributes[string(sqstypes.QueueAttributeNameSqsManagedSseEnabled)], "true") {
		return errors.New("AWS SQS recovery queue is not using SQS-managed encryption")
	}
	if strings.HasSuffix(sqsQueueNameFromURL(queue.URL), ".fifo") {
		return capabilityError(state, "AWS SQS FIFO recovery is not included in the standard-queue export adapter")
	}
	return nil
}

func (api *NativeAPI) inspectSQSQueue(ctx context.Context, queueURL string, state operationState, requireQuiescence bool) (observedSQSQueue, error) {
	queueURL, err := parseSQSReference(sqsReference(queueURL))
	if err != nil {
		return observedSQSQueue{}, err
	}
	attributesOutput, err := api.sqs.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{QueueUrl: aws.String(queueURL), AttributeNames: sqsRecoveryAttributeNames()})
	if err != nil {
		return observedSQSQueue{}, err
	}
	if attributesOutput == nil {
		return observedSQSQueue{}, errors.New("AWS SQS GetQueueAttributes returned no response")
	}
	tagsOutput, err := api.sqs.ListQueueTags(ctx, &sqs.ListQueueTagsInput{QueueUrl: aws.String(queueURL)})
	if err != nil {
		return observedSQSQueue{}, err
	}
	if tagsOutput == nil {
		return observedSQSQueue{}, errors.New("AWS SQS ListQueueTags returned no response")
	}
	queue := observedSQSQueue{URL: queueURL, Attributes: cloneSQSStringMap(attributesOutput.Attributes), Tags: cloneSQSStringMap(tagsOutput.Tags)}
	if queue.Tags[ownershipTagKey] != state.OwnershipMarker || queue.Tags[classTagKey] != state.DataClass || queue.Tags[sqsFixtureTagKey] != state.FixtureID {
		return observedSQSQueue{}, errors.New("AWS SQS queue is not owned by this operation")
	}
	if strings.HasSuffix(sqsQueueNameFromURL(queue.URL), ".fifo") {
		return observedSQSQueue{}, capabilityError(state, "AWS SQS FIFO recovery is not included in the standard-queue export adapter because ordering and deduplication semantics require a dedicated translator")
	}
	encrypted := strings.TrimSpace(queue.Attributes[string(sqstypes.QueueAttributeNameKmsMasterKeyId)]) != "" || strings.EqualFold(queue.Attributes[string(sqstypes.QueueAttributeNameSqsManagedSseEnabled)], "true")
	if !encrypted {
		return observedSQSQueue{}, errors.New("refusing to recover an unencrypted AWS SQS queue")
	}
	if api.config.RequireKMS && strings.TrimSpace(queue.Attributes[string(sqstypes.QueueAttributeNameKmsMasterKeyId)]) == "" {
		return observedSQSQueue{}, errors.New("AWS SQS queue does not use the required customer-managed KMS encryption boundary")
	}
	if _, err := sqsQueueAttributeInt(queue.Attributes, sqstypes.QueueAttributeNameMessageRetentionPeriod); err != nil {
		return observedSQSQueue{}, fmt.Errorf("inspect AWS SQS message retention: %w", err)
	}
	available, err := sqsQueueAttributeInt(queue.Attributes, sqstypes.QueueAttributeNameApproximateNumberOfMessages)
	if err != nil {
		return observedSQSQueue{}, fmt.Errorf("inspect AWS SQS available message count: %w", err)
	}
	queue.Available = available
	if requireQuiescence {
		notVisible, err := sqsQueueAttributeInt(queue.Attributes, sqstypes.QueueAttributeNameApproximateNumberOfMessagesNotVisible)
		if err != nil {
			return observedSQSQueue{}, fmt.Errorf("inspect AWS SQS in-flight message count: %w", err)
		}
		if notVisible != 0 {
			return observedSQSQueue{}, fmt.Errorf("AWS SQS source queue has approximately %d in-flight messages; refusing an unquiesced export", notVisible)
		}
		delayed, err := sqsQueueAttributeInt(queue.Attributes, sqstypes.QueueAttributeNameApproximateNumberOfMessagesDelayed)
		if err != nil {
			return observedSQSQueue{}, fmt.Errorf("inspect AWS SQS delayed message count: %w", err)
		}
		if delayed != 0 {
			return observedSQSQueue{}, fmt.Errorf("AWS SQS source queue has approximately %d delayed messages; refusing a partial export", delayed)
		}
	}
	return queue, nil
}

func sqsRecoveryAttributeNames() []sqstypes.QueueAttributeName {
	return []sqstypes.QueueAttributeName{
		sqstypes.QueueAttributeNameMessageRetentionPeriod,
		sqstypes.QueueAttributeNameApproximateNumberOfMessages,
		sqstypes.QueueAttributeNameApproximateNumberOfMessagesNotVisible,
		sqstypes.QueueAttributeNameApproximateNumberOfMessagesDelayed,
		sqstypes.QueueAttributeNameKmsMasterKeyId,
		sqstypes.QueueAttributeNameSqsManagedSseEnabled,
		sqstypes.QueueAttributeNameVisibilityTimeout,
		sqstypes.QueueAttributeNameReceiveMessageWaitTimeSeconds,
	}
}

func (api *NativeAPI) tagSQSQueue(ctx context.Context, queueURL string, tags map[string]string) error {
	if len(tags) == 0 {
		return errors.New("AWS SQS queue tag set is required")
	}
	output, err := api.sqs.TagQueue(ctx, &sqs.TagQueueInput{QueueUrl: aws.String(queueURL), Tags: tags})
	if err != nil {
		return err
	}
	if output == nil {
		return errors.New("AWS SQS TagQueue returned no response")
	}
	return nil
}

func (api *NativeAPI) copySQSMessages(ctx context.Context, sourceURL, destinationURL string, expectedCount int64) (int, error) {
	copied := 0
	seen := make(map[string]struct{})
	for {
		output, err := api.sqs.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:              aws.String(sourceURL),
			MaxNumberOfMessages:   sqsBatchSize,
			MessageAttributeNames: []string{"All"},
			VisibilityTimeout:     api.config.QueueVisibilityTimeout,
			WaitTimeSeconds:       0,
		})
		if err != nil {
			return copied, err
		}
		if output == nil {
			return copied, errors.New("AWS SQS ReceiveMessage returned no response")
		}
		if len(output.Messages) == 0 {
			if expectedCount == 0 && copied > 0 {
				return copied, errors.New("AWS SQS recovery could not establish a stable source message-count boundary")
			}
			return copied, nil
		}
		if expectedCount == 0 {
			if err := api.resetSQSVisibility(ctx, sourceURL, output.Messages); err != nil {
				return copied, fmt.Errorf("AWS SQS source count was zero but visibility reset failed: %w", err)
			}
			return copied, errors.New("AWS SQS source message count was zero while ReceiveMessage returned messages; wait for owning-service metrics to settle and retry")
		}
		newMessages := make([]sqstypes.Message, 0, len(output.Messages))
		for _, message := range output.Messages {
			messageID := strings.TrimSpace(aws.ToString(message.MessageId))
			if messageID == "" {
				_ = api.resetSQSVisibility(ctx, sourceURL, output.Messages)
				return copied, errors.New("AWS SQS recovery requires a stable MessageId for every received message")
			}
			if _, exists := seen[messageID]; exists {
				continue
			}
			seen[messageID] = struct{}{}
			newMessages = append(newMessages, message)
		}
		if len(newMessages) == 0 {
			if err := api.resetSQSVisibility(ctx, sourceURL, output.Messages); err != nil {
				return copied, fmt.Errorf("reset AWS SQS source visibility after reaching the export boundary: %w", err)
			}
			return copied, nil
		}
		if copied+len(newMessages) > api.config.QueueMaxMessages {
			visibilityErr := api.resetSQSVisibility(ctx, sourceURL, output.Messages)
			if visibilityErr != nil {
				return copied, fmt.Errorf("AWS SQS export limit exceeded and source visibility reset failed: %w", visibilityErr)
			}
			return copied, fmt.Errorf("AWS SQS message count exceeded the configured recovery limit of %d", api.config.QueueMaxMessages)
		}
		entries, err := sqsSendEntries(newMessages, copied)
		if err != nil {
			_ = api.resetSQSVisibility(ctx, sourceURL, output.Messages)
			return copied, err
		}
		sendOutput, err := api.sqs.SendMessageBatch(ctx, &sqs.SendMessageBatchInput{QueueUrl: aws.String(destinationURL), Entries: entries})
		if err != nil {
			_ = api.resetSQSVisibility(ctx, sourceURL, output.Messages)
			return copied, err
		}
		if sendOutput == nil {
			_ = api.resetSQSVisibility(ctx, sourceURL, output.Messages)
			return copied, errors.New("AWS SQS SendMessageBatch returned no response")
		}
		if len(sendOutput.Failed) > 0 {
			_ = api.resetSQSVisibility(ctx, sourceURL, output.Messages)
			return copied, fmt.Errorf("AWS SQS SendMessageBatch reported %d failed entries", len(sendOutput.Failed))
		}
		if err := api.resetSQSVisibility(ctx, sourceURL, output.Messages); err != nil {
			return copied, fmt.Errorf("reset AWS SQS source visibility after export: %w", err)
		}
		copied += len(newMessages)
	}
}

func (api *NativeAPI) resetSQSVisibility(ctx context.Context, queueURL string, messages []sqstypes.Message) error {
	entries := make([]sqstypes.ChangeMessageVisibilityBatchRequestEntry, 0, len(messages))
	for index, message := range messages {
		if strings.TrimSpace(aws.ToString(message.ReceiptHandle)) == "" {
			return errors.New("AWS SQS received a message without a receipt handle")
		}
		entries = append(entries, sqstypes.ChangeMessageVisibilityBatchRequestEntry{Id: aws.String(fmt.Sprintf("m%d", index)), ReceiptHandle: message.ReceiptHandle, VisibilityTimeout: 0})
	}
	if len(entries) == 0 {
		return nil
	}
	output, err := api.sqs.ChangeMessageVisibilityBatch(ctx, &sqs.ChangeMessageVisibilityBatchInput{QueueUrl: aws.String(queueURL), Entries: entries})
	if err != nil {
		return err
	}
	if output == nil {
		return errors.New("AWS SQS ChangeMessageVisibilityBatch returned no response")
	}
	if len(output.Failed) > 0 {
		return fmt.Errorf("AWS SQS ChangeMessageVisibilityBatch reported %d failed entries", len(output.Failed))
	}
	return nil
}

func (api *NativeAPI) listOwnedSQSQueues(ctx context.Context, marker string) ([]provider.InventoryResource, error) {
	if strings.TrimSpace(marker) == "" {
		return nil, errors.New("AWS SQS inventory ownership marker is required")
	}
	resources := make([]provider.InventoryResource, 0)
	seen := make(map[string]struct{})
	for _, prefix := range []string{sqsExportQueuePrefix, sqsRestoreQueuePrefix} {
		var token *string
		for {
			output, err := api.sqs.ListQueues(ctx, &sqs.ListQueuesInput{QueueNamePrefix: aws.String(prefix), MaxResults: aws.Int32(1000), NextToken: token})
			if err != nil {
				return nil, err
			}
			if output == nil {
				return nil, errors.New("AWS SQS ListQueues returned no response")
			}
			for _, queueURL := range output.QueueUrls {
				if _, exists := seen[queueURL]; exists {
					continue
				}
				tags, err := api.sqs.ListQueueTags(ctx, &sqs.ListQueueTagsInput{QueueUrl: aws.String(queueURL)})
				if err != nil {
					if isSQSNotFound(err) {
						continue
					}
					return nil, err
				}
				if tags == nil || tags.Tags[ownershipTagKey] != marker || tags.Tags[classTagKey] != "queue" {
					continue
				}
				seen[queueURL] = struct{}{}
				resources = append(resources, provider.InventoryResource{Identity: sqsReference(queueURL), Owned: true, Live: true})
			}
			if strings.TrimSpace(aws.ToString(output.NextToken)) == "" {
				break
			}
			if token != nil && aws.ToString(token) == aws.ToString(output.NextToken) {
				return nil, errors.New("AWS SQS ListQueues returned a repeated continuation token")
			}
			token = output.NextToken
		}
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].Identity < resources[j].Identity })
	return resources, nil
}

func (api *NativeAPI) deleteOwnedSQSQueue(ctx context.Context, queueURL, marker string) error {
	queueURL, err := parseSQSReference(queueURL)
	if err != nil {
		return err
	}
	tags, err := api.sqs.ListQueueTags(ctx, &sqs.ListQueueTagsInput{QueueUrl: aws.String(queueURL)})
	if err != nil {
		if isSQSNotFound(err) {
			return nil
		}
		return err
	}
	if tags == nil || tags.Tags[ownershipTagKey] != marker || tags.Tags[classTagKey] != "queue" {
		return errors.New("refusing to delete an AWS SQS queue outside the ownership scope")
	}
	if _, err := api.sqs.DeleteQueue(ctx, &sqs.DeleteQueueInput{QueueUrl: aws.String(queueURL)}); err != nil && !isSQSNotFound(err) {
		return err
	}
	return nil
}

func (api *NativeAPI) sqsObservation(state operationState, operationID string, queue observedSQSQueue, evidence sdk.ResilienceProofEvidence) (provider.NativeOperationObservation, error) {
	if strings.TrimSpace(queue.URL) == "" {
		return provider.NativeOperationObservation{}, errors.New("AWS SQS recovery queue returned no identity")
	}
	return observationWithOperation(state, operationID, []string{queue.URL}, []string{"aws.sqs.export-replay", "aws.sqs.encryption", "aws.sqs.ownership"}, []sdk.ResilienceProofEvidence{evidence}, string(sdk.ResilienceOperationSucceeded)), nil
}

func sqsProofEvidence(state operationState, queue observedSQSQueue, reason string) sdk.ResilienceProofEvidence {
	encrypted := strings.TrimSpace(queue.Attributes[string(sqstypes.QueueAttributeNameKmsMasterKeyId)]) != "" || strings.EqualFold(queue.Attributes[string(sqstypes.QueueAttributeNameSqsManagedSseEnabled)], "true")
	return sdk.ResilienceProofEvidence{
		DataClass: state.DataClass, Destination: string(state.Destination), BackupID: sqsReference(queue.URL), FixtureID: state.FixtureID,
		RetentionDays: defaultSQSRetentionDays(queue), EncryptionVerified: encrypted, ProtectionVerified: true, ServiceHealthVerified: true, Reason: reason,
	}
}

func defaultSQSRetentionDays(queue observedSQSQueue) int {
	seconds, err := sqsQueueAttributeInt(queue.Attributes, sqstypes.QueueAttributeNameMessageRetentionPeriod)
	if err != nil || seconds <= 0 {
		return 0
	}
	return int(seconds / (24 * 60 * 60))
}

func sqsExportQueueAttributes(api *NativeAPI) map[string]string {
	return sqsRecoveryQueueAttributes(api)
}

func sqsRestoreQueueAttributes(api *NativeAPI) map[string]string {
	return sqsRecoveryQueueAttributes(api)
}

func sqsRecoveryQueueAttributes(api *NativeAPI) map[string]string {
	attributes := map[string]string{
		string(sqstypes.QueueAttributeNameMessageRetentionPeriod):        strconv.Itoa(api.retentionDays() * 24 * 60 * 60),
		string(sqstypes.QueueAttributeNameVisibilityTimeout):             strconv.FormatInt(int64(api.config.QueueVisibilityTimeout), 10),
		string(sqstypes.QueueAttributeNameReceiveMessageWaitTimeSeconds): "0",
	}
	if strings.TrimSpace(api.config.KMSKeyID) != "" {
		attributes[string(sqstypes.QueueAttributeNameKmsMasterKeyId)] = api.config.KMSKeyID
	} else {
		attributes[string(sqstypes.QueueAttributeNameSqsManagedSseEnabled)] = "true"
	}
	return attributes
}

func sqsExportTags(state operationState) map[string]string {
	return map[string]string{
		ownershipTagKey:       state.OwnershipMarker,
		classTagKey:           state.DataClass,
		sqsFixtureTagKey:      state.FixtureID,
		sqsStatusTagKey:       sqsStatusCreating,
		operationTagKey:       shortDigest(state.IdempotencyKey),
		sqsSourceDigestTagKey: shortDigest(state.Resource),
	}
}

func sqsRestoreTags(state operationState, backupURL string) map[string]string {
	return map[string]string{
		ownershipTagKey:       state.OwnershipMarker,
		classTagKey:           state.DataClass,
		sqsFixtureTagKey:      state.FixtureID,
		sqsStatusTagKey:       sqsStatusCreating,
		operationTagKey:       shortDigest(state.IdempotencyKey),
		sqsBackupDigestTagKey: shortDigest(backupURL),
	}
}

func sqsRecoveryQueueName(prefix string, state operationState) string {
	return prefix + shortDigest(state.OwnershipMarker+"\x00"+state.IdempotencyKey)
}

func sqsSendEntries(messages []sqstypes.Message, offset int) ([]sqstypes.SendMessageBatchRequestEntry, error) {
	entries := make([]sqstypes.SendMessageBatchRequestEntry, 0, len(messages))
	for index, message := range messages {
		if message.Body == nil {
			return nil, fmt.Errorf("AWS SQS message %d has no body", offset+index)
		}
		entries = append(entries, sqstypes.SendMessageBatchRequestEntry{Id: aws.String(fmt.Sprintf("m%d", offset+index)), MessageBody: message.Body, MessageAttributes: cloneSQSMessageAttributes(message.MessageAttributes)})
	}
	return entries, nil
}

func cloneSQSMessageAttributes(attributes map[string]sqstypes.MessageAttributeValue) map[string]sqstypes.MessageAttributeValue {
	if len(attributes) == 0 {
		return nil
	}
	clone := make(map[string]sqstypes.MessageAttributeValue, len(attributes))
	for name, value := range attributes {
		copyValue := value
		copyValue.BinaryValue = append([]byte(nil), value.BinaryValue...)
		copyValue.BinaryListValues = append([][]byte(nil), value.BinaryListValues...)
		copyValue.StringListValues = append([]string(nil), value.StringListValues...)
		clone[name] = copyValue
	}
	return clone
}

func cloneSQSStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func sqsQueueAttributeInt(attributes map[string]string, name sqstypes.QueueAttributeName) (int64, error) {
	value := strings.TrimSpace(attributes[string(name)])
	if value == "" {
		return 0, fmt.Errorf("AWS SQS queue attribute %q is missing", name)
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("AWS SQS queue attribute %q is invalid", name)
	}
	return parsed, nil
}

func parseSQSReference(reference string) (string, error) {
	reference = strings.TrimSpace(reference)
	for _, prefix := range []string{"aws-sqs://", "sqs://"} {
		if !strings.HasPrefix(strings.ToLower(reference), prefix) {
			continue
		}
		value := strings.TrimSpace(reference[len(prefix):])
		parsed, err := url.Parse(value)
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || strings.TrimSpace(parsed.Host) == "" || strings.TrimSpace(parsed.Path) == "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
			return "", fmt.Errorf("SQS resource reference %q must use aws-sqs://https://queue-url without query or fragment data", reference)
		}
		return value, nil
	}
	return "", fmt.Errorf("SQS resource reference %q must use aws-sqs://https://queue-url", reference)
}

func sqsReference(queueURL string) string {
	return "aws-sqs://" + strings.TrimSpace(queueURL)
}

func sqsQueueNameFromURL(queueURL string) string {
	parsed, err := url.Parse(queueURL)
	if err != nil {
		return ""
	}
	path := strings.Trim(parsed.Path, "/")
	if path == "" {
		return ""
	}
	name, err := url.PathUnescape(path[strings.LastIndex(path, "/")+1:])
	if err != nil {
		return ""
	}
	return name
}

func isSQSNotFound(err error) bool {
	if err == nil {
		return false
	}
	var queueMissing *sqstypes.QueueDoesNotExist
	var resourceMissing *sqstypes.ResourceNotFoundException
	if errors.As(err, &queueMissing) || errors.As(err, &resourceMissing) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "queuedoesnotexist") || strings.Contains(message, "queue does not exist") || strings.Contains(message, "nonexistentqueue")
}

func isSQSAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	var queueExists *sqstypes.QueueNameExists
	if errors.As(err, &queueExists) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "queuenameexists") || strings.Contains(message, "queue name already exists")
}
