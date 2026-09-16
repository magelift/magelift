package resilience

import (
	"context"
	"errors"
	"fmt"
	"sort"

	pubsub "cloud.google.com/go/pubsub/v2/apiv1"
	pubsubpb "cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

type pubSubSDK struct {
	client *pubsub.SubscriptionAdminClient
}

func newPubSubSDK(ctx context.Context) (*pubSubSDK, error) {
	if ctx == nil {
		return nil, errors.New("GCP Pub/Sub context is required")
	}
	client, err := pubsub.NewSubscriptionAdminClient(ctx)
	if err != nil {
		return nil, err
	}
	return &pubSubSDK{client: client}, nil
}

func (s *pubSubSDK) Close() error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Close()
}

func (s pubSubSDK) GetSubscription(ctx context.Context, name string) (PubSubSubscription, error) {
	response, err := s.client.GetSubscription(ctx, &pubsubpb.GetSubscriptionRequest{Subscription: name})
	if err != nil {
		return PubSubSubscription{}, err
	}
	if response == nil {
		return PubSubSubscription{}, errors.New("GCP Pub/Sub subscription response is empty")
	}
	return PubSubSubscription{Name: response.Name, Topic: response.Topic, Labels: cloneMetadata(response.Labels)}, nil
}

func (s pubSubSDK) CreateSnapshot(ctx context.Context, name, subscription string) (PubSubSnapshot, error) {
	response, err := s.client.CreateSnapshot(ctx, &pubsubpb.CreateSnapshotRequest{Name: name, Subscription: subscription})
	if err != nil {
		return PubSubSnapshot{}, err
	}
	return decodePubSubSnapshot(response)
}

func (s pubSubSDK) GetSnapshot(ctx context.Context, name string) (PubSubSnapshot, error) {
	response, err := s.client.GetSnapshot(ctx, &pubsubpb.GetSnapshotRequest{Snapshot: name})
	if err != nil {
		return PubSubSnapshot{}, err
	}
	return decodePubSubSnapshot(response)
}

func (s pubSubSDK) SetSnapshotLabels(ctx context.Context, name string, labels map[string]string) (PubSubSnapshot, error) {
	response, err := s.client.UpdateSnapshot(ctx, &pubsubpb.UpdateSnapshotRequest{
		Snapshot:   &pubsubpb.Snapshot{Name: name, Labels: cloneMetadata(labels)},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"labels"}},
	})
	if err != nil {
		return PubSubSnapshot{}, err
	}
	return decodePubSubSnapshot(response)
}

func (s pubSubSDK) Seek(ctx context.Context, subscription, snapshot string) error {
	_, err := s.client.Seek(ctx, &pubsubpb.SeekRequest{
		Subscription: subscription,
		Target:       &pubsubpb.SeekRequest_Snapshot{Snapshot: snapshot},
	})
	return err
}

func (s pubSubSDK) ListSnapshots(ctx context.Context, project string) ([]PubSubSnapshot, error) {
	it := s.client.ListSnapshots(ctx, &pubsubpb.ListSnapshotsRequest{Project: "projects/" + project})
	result := make([]PubSubSnapshot, 0)
	for {
		value, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		decoded, err := decodePubSubSnapshot(value)
		if err != nil {
			return nil, err
		}
		result = append(result, decoded)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (s pubSubSDK) DeleteSnapshot(ctx context.Context, name string) error {
	return s.client.DeleteSnapshot(ctx, &pubsubpb.DeleteSnapshotRequest{Snapshot: name})
}

func (s pubSubSDK) CreateSubscription(ctx context.Context, name, topic string, labels map[string]string) (PubSubSubscription, error) {
	response, err := s.client.CreateSubscription(ctx, &pubsubpb.Subscription{
		Name: name, Topic: topic, Labels: cloneMetadata(labels),
	})
	if err != nil {
		return PubSubSubscription{}, err
	}
	if response == nil {
		return PubSubSubscription{}, errors.New("GCP Pub/Sub create subscription response is empty")
	}
	return PubSubSubscription{Name: response.Name, Topic: response.Topic, Labels: cloneMetadata(response.Labels)}, nil
}

func (s pubSubSDK) ListSubscriptions(ctx context.Context, project string) ([]PubSubSubscription, error) {
	it := s.client.ListSubscriptions(ctx, &pubsubpb.ListSubscriptionsRequest{Project: "projects/" + project})
	result := make([]PubSubSubscription, 0)
	for {
		value, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		if value == nil {
			return nil, errors.New("GCP Pub/Sub subscription list returned an empty item")
		}
		result = append(result, PubSubSubscription{Name: value.Name, Topic: value.Topic, Labels: cloneMetadata(value.Labels)})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (s pubSubSDK) DeleteSubscription(ctx context.Context, name string) error {
	return s.client.DeleteSubscription(ctx, &pubsubpb.DeleteSubscriptionRequest{Subscription: name})
}

func decodePubSubSnapshot(response *pubsubpb.Snapshot) (PubSubSnapshot, error) {
	if response == nil {
		return PubSubSnapshot{}, errors.New("GCP Pub/Sub snapshot response is empty")
	}
	if response.ExpireTime == nil {
		return PubSubSnapshot{}, fmt.Errorf("GCP Pub/Sub snapshot %q has no expiry", response.Name)
	}
	return PubSubSnapshot{
		Name: response.Name, Topic: response.Topic, ExpireTime: response.ExpireTime.AsTime(),
		Labels: cloneMetadata(response.Labels), EncryptionVerified: true,
	}, nil
}
