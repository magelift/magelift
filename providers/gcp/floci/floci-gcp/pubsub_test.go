//go:build floci_gcp

package flocigcp_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	pubsub "cloud.google.com/go/pubsub/v2"
	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
)

func TestTopicPublishAgainstFlociGCP(t *testing.T) {
	host := requireFlociGCP(t)
	t.Setenv("PUBSUB_EMULATOR_HOST", host)
	t.Setenv("GOOGLE_CLOUD_PROJECT", "floci-local")

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	const project = "floci-local"
	client, err := pubsub.NewClient(ctx, project, flociGCPClientOptions(host)...)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	topicName := fmt.Sprintf("projects/%s/topics/magelift-floci-gcp-topic", project)
	topic, err := client.TopicAdminClient.CreateTopic(ctx, &pubsubpb.Topic{Name: topicName})
	if err != nil {
		t.Fatalf("create topic: %v", err)
	}
	defer func() {
		_ = client.TopicAdminClient.DeleteTopic(context.Background(), &pubsubpb.DeleteTopicRequest{Topic: topic.Name})
	}()

	publisher := client.Publisher(topic.Name)
	defer publisher.Stop()
	id, err := publisher.Publish(ctx, &pubsub.Message{Data: []byte("ping")}).Get(ctx)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if id == "" {
		t.Fatal("publish returned an empty message id")
	}
}
