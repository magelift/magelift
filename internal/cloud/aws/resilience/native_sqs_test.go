package resilience

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"testing"

	aws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"

	cloudresilience "github.com/magelift/magelift/internal/cloud/resilience"
	sdk "github.com/magelift/magelift/sdk/v1"
)

func TestNativeAPISQSExportRestoreIntegrityAndCleanup(t *testing.T) {
	ctx := context.Background()
	sqsAPI := newFakeSQS()
	sourceURL := "https://sqs.test/123/source"
	sqsAPI.addQueue(fakeSQSQueue{
		name: "source", url: sourceURL,
		attributes: fakeSQSAttributes(false, "alias/source", 1, 0, 0),
		tags:       map[string]string{ownershipTagKey: "magelift/test/aws-sqs", classTagKey: "queue", sqsFixtureTagKey: "fixture/sqs"},
		messages:   []sqstypes.Message{{Body: aws.String("known-message"), MessageAttributes: map[string]sqstypes.MessageAttributeValue{"fixture": {DataType: aws.String("String"), StringValue: aws.String("fixture/sqs")}}}},
	})
	verifier := &fakeSQSVerifier{}
	native, err := NewNativeAPIWithSQS(newFakeRDS(), newFakeS3(), newFakeSecrets(), sqsAPI, NativeAPIConfig{
		ArchiveBucket:          "archive",
		RetentionDays:          14,
		QueueMaxMessages:       10,
		QueueVisibilityTimeout: 30,
		Verifier:               verifier,
	})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceBackup, DataClasses: []string{"queue"}, Destination: sdk.RecoverySameRegionIsolated,
		FixtureID: "fixture/sqs", OwnershipMarker: "magelift/test/aws-sqs", IdempotencyKey: "sqs/backup/1", ApprovalReference: "quiescence-approved",
		ResourceReferences: map[string]string{"queue": sqsReference(sourceURL)},
	}
	backup, err := client.Start(ctx, request)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if backup.Status != sdk.ResilienceOperationSucceeded || len(backup.Evidence) != 1 || backup.Evidence[0].BackupID == "" {
		t.Fatalf("backup observation = %#v", backup)
	}
	exportURL := backup.Evidence[0].BackupID
	if len(sqsAPI.queueByReference(exportURL).messages) != 1 || sqsAPI.queueByReference(exportURL).tags[sqsStatusTagKey] != sqsStatusComplete {
		t.Fatalf("export queue = %#v", sqsAPI.queueByReference(exportURL))
	}
	if len(sqsAPI.queueByReference(sqsReference(sourceURL)).messages) != 1 {
		t.Fatal("source message was deleted during export")
	}
	sendsAfterFirstBackup := sqsAPI.sendCount
	if _, err := client.Start(ctx, request); err != nil {
		t.Fatalf("idempotent backup: %v", err)
	}
	if sqsAPI.sendCount != sendsAfterFirstBackup {
		t.Fatalf("idempotent backup sent messages again: first=%d second=%d", sendsAfterFirstBackup, sqsAPI.sendCount)
	}

	restoreRequest := request
	restoreRequest.Action = sdk.ResilienceRestore
	restoreRequest.IdempotencyKey = "sqs/restore/1"
	restoreRequest.ApprovalReference = ""
	restoreRequest.BackupReferences = map[string]string{"queue": exportURL}
	restore, err := client.Start(ctx, restoreRequest)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if restore.Status != sdk.ResilienceOperationSucceeded || restore.Evidence[0].RestoreID == "" || !restore.Evidence[0].CountsVerified || !restore.Evidence[0].ApplicationReadsVerified {
		t.Fatalf("restore observation = %#v", restore)
	}
	sendsAfterRestore := sqsAPI.sendCount
	if _, err := client.Start(ctx, restoreRequest); err != nil {
		t.Fatalf("idempotent restore: %v", err)
	}
	if sqsAPI.sendCount != sendsAfterRestore {
		t.Fatalf("idempotent restore replayed messages: first=%d second=%d", sendsAfterRestore, sqsAPI.sendCount)
	}

	integrityRequest := restoreRequest
	integrityRequest.Action = sdk.ResilienceIntegrityCheck
	integrityRequest.IdempotencyKey = "sqs/integrity/1"
	integrity, err := client.Start(ctx, integrityRequest)
	if err != nil {
		t.Fatalf("integrity: %v", err)
	}
	if integrity.Status != sdk.ResilienceOperationSucceeded || !integrity.Evidence[0].CountsVerified || !integrity.Evidence[0].PermissionsVerified {
		t.Fatalf("integrity observation = %#v", integrity)
	}

	resources, err := client.Inventory(ctx, request.OwnershipMarker)
	if err != nil || len(resources) != 2 {
		t.Fatalf("inventory before cleanup = %#v, err=%v", resources, err)
	}
	cleanupRequest := request
	cleanupRequest.Action = sdk.ResilienceCleanup
	cleanupRequest.IdempotencyKey = "sqs/cleanup/1"
	cleanupRequest.ApprovalReference = ""
	cleanup, err := client.Start(ctx, cleanupRequest)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if cleanup.Status != sdk.ResilienceOperationSucceeded || len(cleanup.ResourceRefs) != 2 {
		t.Fatalf("cleanup observation = %#v", cleanup)
	}
	resources, err = client.Inventory(ctx, request.OwnershipMarker)
	if err != nil || len(resources) != 0 {
		t.Fatalf("inventory after cleanup = %#v, err=%v", resources, err)
	}
	if len(sqsAPI.queueByReference(sqsReference(sourceURL)).messages) != 1 {
		t.Fatal("cleanup deleted the source queue message")
	}
	if len(verifier.resources) != 3 {
		t.Fatalf("verifier calls = %v", verifier.resources)
	}
}

func TestNativeAPISQSRefusesUnsafeBoundariesBeforeMutation(t *testing.T) {
	ctx := context.Background()
	for name, mutate := range map[string]func(*fakeSQSQueue){
		"missing quiescence approval": func(_ *fakeSQSQueue) {},
		"in-flight messages": func(queue *fakeSQSQueue) {
			queue.attributes[string(sqstypes.QueueAttributeNameApproximateNumberOfMessagesNotVisible)] = "1"
		},
		"delayed messages": func(queue *fakeSQSQueue) {
			queue.attributes[string(sqstypes.QueueAttributeNameApproximateNumberOfMessagesDelayed)] = "1"
		},
		"FIFO": func(queue *fakeSQSQueue) { queue.attributes[string(sqstypes.QueueAttributeNameFifoQueue)] = "true" },
	} {
		t.Run(name, func(t *testing.T) {
			sqsAPI := newFakeSQS()
			sourceURL := "https://sqs.test/123/source"
			sourceName := "source"
			if name == "FIFO" {
				sourceURL += ".fifo"
				sourceName += ".fifo"
			}
			source := &fakeSQSQueue{
				name: sourceName, url: sourceURL,
				attributes: fakeSQSAttributes(false, "alias/source", 1, 0, 0),
				tags:       map[string]string{ownershipTagKey: "marker", classTagKey: "queue", sqsFixtureTagKey: "fixture"},
				messages:   []sqstypes.Message{{Body: aws.String("message")}},
			}
			mutate(source)
			sqsAPI.addQueue(*source)
			native, err := NewNativeAPIWithSQS(newFakeRDS(), newFakeS3(), newFakeSecrets(), sqsAPI, NativeAPIConfig{ArchiveBucket: "archive", RetentionDays: 14, QueueMaxMessages: 10})
			if err != nil {
				t.Fatal(err)
			}
			approval := "approved"
			if name == "missing quiescence approval" {
				approval = ""
			}
			_, err = native.Start(ctx, cloudNativeRequest(sdk.ResilienceBackup, "https://sqs.test/123/source", "marker", "fixture", "backup", approval))
			if err == nil {
				t.Fatal("backup unexpectedly succeeded")
			}
			if sqsAPI.createCount != 0 || sqsAPI.receiveCount != 0 || sqsAPI.sendCount != 0 {
				t.Fatalf("unsafe request mutated SQS: creates=%d receives=%d sends=%d", sqsAPI.createCount, sqsAPI.receiveCount, sqsAPI.sendCount)
			}
		})
	}
}

func TestNativeAPISQSForeignRecoveryQueueCollisionRefusesMutation(t *testing.T) {
	sqsAPI := newFakeSQS()
	state := operationState{Action: sdk.ResilienceBackup, DataClass: "queue", FixtureID: "fixture", OwnershipMarker: "marker", IdempotencyKey: "backup"}
	name := sqsRecoveryQueueName(sqsExportQueuePrefix, state)
	sqsAPI.addQueue(fakeSQSQueue{
		name: name, url: "https://sqs.test/123/" + name,
		attributes: fakeSQSAttributes(false, "alias/foreign", 1, 0, 0),
		tags:       map[string]string{ownershipTagKey: "someone-else", classTagKey: "queue", sqsFixtureTagKey: "fixture"},
	})
	sqsAPI.addQueue(fakeSQSQueue{
		name: "source", url: "https://sqs.test/123/source",
		attributes: fakeSQSAttributes(false, "alias/source", 1, 0, 0),
		tags:       map[string]string{ownershipTagKey: "marker", classTagKey: "queue", sqsFixtureTagKey: "fixture"},
		messages:   []sqstypes.Message{{Body: aws.String("message")}},
	})
	native, err := NewNativeAPIWithSQS(newFakeRDS(), newFakeS3(), newFakeSecrets(), sqsAPI, NativeAPIConfig{ArchiveBucket: "archive", RetentionDays: 14, QueueMaxMessages: 10})
	if err != nil {
		t.Fatal(err)
	}
	_, err = native.Start(context.Background(), cloudNativeRequest(sdk.ResilienceBackup, "https://sqs.test/123/source", "marker", "fixture", "backup", "approved"))
	if err == nil || !strings.Contains(err.Error(), "owned by this operation") {
		t.Fatalf("collision error = %v", err)
	}
	if sqsAPI.receiveCount != 0 || sqsAPI.sendCount != 0 {
		t.Fatalf("collision mutated messages: receives=%d sends=%d", sqsAPI.receiveCount, sqsAPI.sendCount)
	}
}

func cloudNativeRequest(action sdk.ResilienceAction, sourceURL, marker, fixture, idempotency, approval string) cloudresilience.NativeOperationRequest {
	return cloudresilience.NativeOperationRequest{
		Provider: "aws", Operation: "aws.queue." + string(action) + ".queue",
		Request: sdk.ResilienceOperationRequest{
			Action: action, DataClasses: []string{"queue"}, Destination: sdk.RecoverySameRegionIsolated, FixtureID: fixture, OwnershipMarker: marker, IdempotencyKey: idempotency, ApprovalReference: approval,
			ResourceReferences: map[string]string{"queue": sqsReference(sourceURL)},
		},
	}
}

type fakeSQSVerifier struct {
	resources []string
}

func (f *fakeSQSVerifier) Verify(_ context.Context, request RecoveryVerificationRequest) (RecoveryVerification, error) {
	f.resources = append(f.resources, request.Resource)
	return RecoveryVerification{CountsVerified: true, ApplicationReadsVerified: true, PermissionsVerified: true, ServiceHealthVerified: true}, nil
}

type fakeSQSQueue struct {
	name       string
	url        string
	attributes map[string]string
	tags       map[string]string
	messages   []sqstypes.Message
	inflight   map[string]int
	nextID     int
}

type fakeSQS struct {
	queues       map[string]*fakeSQSQueue
	byName       map[string]string
	createCount  int
	receiveCount int
	sendCount    int
	deleteCount  int
}

func newFakeSQS() *fakeSQS {
	return &fakeSQS{queues: make(map[string]*fakeSQSQueue), byName: make(map[string]string)}
}

func (f *fakeSQS) addQueue(queue fakeSQSQueue) {
	if queue.attributes == nil {
		queue.attributes = fakeSQSAttributes(false, "alias/test", len(queue.messages), 0, 0)
	}
	if queue.tags == nil {
		queue.tags = make(map[string]string)
	}
	queue.messages = append([]sqstypes.Message(nil), queue.messages...)
	queue.inflight = make(map[string]int)
	for index := range queue.messages {
		if strings.TrimSpace(aws.ToString(queue.messages[index].MessageId)) == "" {
			queue.messages[index].MessageId = aws.String("message-" + strconv.Itoa(index))
		}
	}
	copyQueue := queue
	f.queues[queue.url] = &copyQueue
	f.byName[queue.name] = queue.url
}

func (f *fakeSQS) queueByReference(reference string) *fakeSQSQueue {
	queueURL, err := parseSQSReference(reference)
	if err != nil {
		panic(err)
	}
	return f.queues[queueURL]
}

func fakeSQSAttributes(fifo bool, kms string, available, notVisible, delayed int) map[string]string {
	attributes := map[string]string{
		string(sqstypes.QueueAttributeNameFifoQueue):                             strconvBool(fifo),
		string(sqstypes.QueueAttributeNameMessageRetentionPeriod):                "1209600",
		string(sqstypes.QueueAttributeNameApproximateNumberOfMessages):           strconv.Itoa(available),
		string(sqstypes.QueueAttributeNameApproximateNumberOfMessagesNotVisible): strconv.Itoa(notVisible),
		string(sqstypes.QueueAttributeNameApproximateNumberOfMessagesDelayed):    strconv.Itoa(delayed),
	}
	if kms != "" {
		attributes[string(sqstypes.QueueAttributeNameKmsMasterKeyId)] = kms
	} else {
		attributes[string(sqstypes.QueueAttributeNameSqsManagedSseEnabled)] = "true"
	}
	return attributes
}

func strconvBool(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func (f *fakeSQS) GetQueueUrl(_ context.Context, input *sqs.GetQueueUrlInput, _ ...func(*sqs.Options)) (*sqs.GetQueueUrlOutput, error) {
	queueURL, ok := f.byName[aws.ToString(input.QueueName)]
	if !ok {
		return nil, &sqstypes.QueueDoesNotExist{}
	}
	return &sqs.GetQueueUrlOutput{QueueUrl: aws.String(queueURL)}, nil
}

func (f *fakeSQS) GetQueueAttributes(_ context.Context, input *sqs.GetQueueAttributesInput, _ ...func(*sqs.Options)) (*sqs.GetQueueAttributesOutput, error) {
	queue, ok := f.queues[aws.ToString(input.QueueUrl)]
	if !ok {
		return nil, &sqstypes.QueueDoesNotExist{}
	}
	attributes := cloneSQSStringMap(queue.attributes)
	available := 0
	for _, message := range queue.messages {
		if message.ReceiptHandle == nil {
			available++
		}
	}
	attributes[string(sqstypes.QueueAttributeNameApproximateNumberOfMessages)] = strconv.Itoa(available)
	notVisible := len(queue.inflight)
	configuredNotVisible, _ := strconv.Atoi(queue.attributes[string(sqstypes.QueueAttributeNameApproximateNumberOfMessagesNotVisible)])
	if configuredNotVisible > notVisible {
		notVisible = configuredNotVisible
	}
	attributes[string(sqstypes.QueueAttributeNameApproximateNumberOfMessagesNotVisible)] = strconv.Itoa(notVisible)
	return &sqs.GetQueueAttributesOutput{Attributes: attributes}, nil
}

func (f *fakeSQS) ListQueueTags(_ context.Context, input *sqs.ListQueueTagsInput, _ ...func(*sqs.Options)) (*sqs.ListQueueTagsOutput, error) {
	queue, ok := f.queues[aws.ToString(input.QueueUrl)]
	if !ok {
		return nil, &sqstypes.QueueDoesNotExist{}
	}
	return &sqs.ListQueueTagsOutput{Tags: cloneSQSStringMap(queue.tags)}, nil
}

func (f *fakeSQS) CreateQueue(_ context.Context, input *sqs.CreateQueueInput, _ ...func(*sqs.Options)) (*sqs.CreateQueueOutput, error) {
	name := aws.ToString(input.QueueName)
	if _, ok := f.byName[name]; ok {
		return nil, &sqstypes.QueueNameExists{}
	}
	f.createCount++
	queueURL := "https://sqs.test/123/" + name
	attributes := cloneSQSStringMap(input.Attributes)
	attributes[string(sqstypes.QueueAttributeNameFifoQueue)] = "false"
	attributes[string(sqstypes.QueueAttributeNameApproximateNumberOfMessages)] = "0"
	attributes[string(sqstypes.QueueAttributeNameApproximateNumberOfMessagesNotVisible)] = "0"
	attributes[string(sqstypes.QueueAttributeNameApproximateNumberOfMessagesDelayed)] = "0"
	f.addQueue(fakeSQSQueue{name: name, url: queueURL, attributes: attributes, tags: input.Tags})
	return &sqs.CreateQueueOutput{QueueUrl: aws.String(queueURL)}, nil
}

func (f *fakeSQS) TagQueue(_ context.Context, input *sqs.TagQueueInput, _ ...func(*sqs.Options)) (*sqs.TagQueueOutput, error) {
	queue, ok := f.queues[aws.ToString(input.QueueUrl)]
	if !ok {
		return nil, &sqstypes.QueueDoesNotExist{}
	}
	for key, value := range input.Tags {
		queue.tags[key] = value
	}
	return &sqs.TagQueueOutput{}, nil
}

func (f *fakeSQS) ReceiveMessage(_ context.Context, input *sqs.ReceiveMessageInput, _ ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
	queue, ok := f.queues[aws.ToString(input.QueueUrl)]
	if !ok {
		return nil, &sqstypes.QueueDoesNotExist{}
	}
	f.receiveCount++
	messages := make([]sqstypes.Message, 0, input.MaxNumberOfMessages)
	for index := range queue.messages {
		if queue.messages[index].ReceiptHandle != nil || len(messages) >= int(input.MaxNumberOfMessages) {
			continue
		}
		receipt := "receipt-" + strconv.Itoa(queue.nextID)
		queue.nextID++
		queue.messages[index].ReceiptHandle = aws.String(receipt)
		queue.inflight[receipt] = index
		message := queue.messages[index]
		messages = append(messages, message)
	}
	return &sqs.ReceiveMessageOutput{Messages: messages}, nil
}

func (f *fakeSQS) SendMessageBatch(_ context.Context, input *sqs.SendMessageBatchInput, _ ...func(*sqs.Options)) (*sqs.SendMessageBatchOutput, error) {
	queue, ok := f.queues[aws.ToString(input.QueueUrl)]
	if !ok {
		return nil, &sqstypes.QueueDoesNotExist{}
	}
	f.sendCount++
	successful := make([]sqstypes.SendMessageBatchResultEntry, 0, len(input.Entries))
	for _, entry := range input.Entries {
		queue.messages = append(queue.messages, sqstypes.Message{Body: entry.MessageBody, MessageAttributes: cloneSQSMessageAttributes(entry.MessageAttributes), MessageId: aws.String(aws.ToString(entry.Id))})
		successful = append(successful, sqstypes.SendMessageBatchResultEntry{Id: entry.Id, MessageId: entry.Id})
	}
	return &sqs.SendMessageBatchOutput{Successful: successful}, nil
}

func (f *fakeSQS) ChangeMessageVisibilityBatch(_ context.Context, input *sqs.ChangeMessageVisibilityBatchInput, _ ...func(*sqs.Options)) (*sqs.ChangeMessageVisibilityBatchOutput, error) {
	queue, ok := f.queues[aws.ToString(input.QueueUrl)]
	if !ok {
		return nil, &sqstypes.QueueDoesNotExist{}
	}
	for _, entry := range input.Entries {
		index, ok := queue.inflight[aws.ToString(entry.ReceiptHandle)]
		if !ok {
			return &sqs.ChangeMessageVisibilityBatchOutput{Failed: []sqstypes.BatchResultErrorEntry{{Id: entry.Id}}}, nil
		}
		if entry.VisibilityTimeout == 0 {
			queue.messages[index].ReceiptHandle = nil
			delete(queue.inflight, aws.ToString(entry.ReceiptHandle))
		}
	}
	return &sqs.ChangeMessageVisibilityBatchOutput{}, nil
}

func (f *fakeSQS) ListQueues(_ context.Context, input *sqs.ListQueuesInput, _ ...func(*sqs.Options)) (*sqs.ListQueuesOutput, error) {
	urls := make([]string, 0)
	for name, queueURL := range f.byName {
		if strings.HasPrefix(name, aws.ToString(input.QueueNamePrefix)) {
			urls = append(urls, queueURL)
		}
	}
	sort.Strings(urls)
	return &sqs.ListQueuesOutput{QueueUrls: urls}, nil
}

func (f *fakeSQS) DeleteQueue(_ context.Context, input *sqs.DeleteQueueInput, _ ...func(*sqs.Options)) (*sqs.DeleteQueueOutput, error) {
	queueURL := aws.ToString(input.QueueUrl)
	queue, ok := f.queues[queueURL]
	if !ok {
		return nil, &sqstypes.QueueDoesNotExist{}
	}
	f.deleteCount++
	delete(f.queues, queueURL)
	delete(f.byName, queue.name)
	return &sqs.DeleteQueueOutput{}, nil
}

var _ SQSAPI = (*fakeSQS)(nil)
