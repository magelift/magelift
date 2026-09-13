package platform

import (
	"strings"
	"testing"
)

func TestRefuseInPlaceRabbitMQTopologyChange(t *testing.T) {
	if err := RefuseInPlaceRabbitMQTopologyChange(1, 3); err == nil || !strings.Contains(err.Error(), "refusing in-place RabbitMQ") || !strings.Contains(err.Error(), "new environment") {
		t.Fatalf("1→3 = %v", err)
	}
	if err := RefuseInPlaceRabbitMQTopologyChange(3, 1); err == nil || !strings.Contains(err.Error(), "quorum") {
		t.Fatalf("3→1 = %v", err)
	}
	if err := RefuseInPlaceRabbitMQTopologyChange(1, 1); err != nil {
		t.Fatalf("same class = %v", err)
	}
	if err := RefuseInPlaceRabbitMQTopologyChange(0, 3); err != nil {
		t.Fatalf("create = %v", err)
	}
}

func TestQueueReplicasFromOutputs(t *testing.T) {
	n, ok := QueueReplicasFromOutputs(map[string]any{OutputQueueReplicas: 1})
	if !ok || n != 1 {
		t.Fatalf("got %d %v", n, ok)
	}
	if _, found := QueueReplicasFromOutputs(map[string]any{}); found {
		t.Fatal("empty outputs should not look live")
	}
}
