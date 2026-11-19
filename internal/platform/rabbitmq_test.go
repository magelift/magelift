package platform

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type contextReplicaBinder struct {
	fakePlannedStack
	context  context.Context
	replicas int
}

func (*contextReplicaBinder) WithLiveQueueReplicas(int) (PlannedStack, error) {
	return nil, errors.New("context-aware binder expected")
}

func (p *contextReplicaBinder) WithLiveQueueReplicasContext(ctx context.Context, replicas int) (PlannedStack, error) {
	p.context, p.replicas = ctx, replicas
	return p, nil
}

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

func TestBindLiveQueueReplicasPreservesCallerContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), struct{}{}, "caller"))
	defer cancel()
	planned := &contextReplicaBinder{fakePlannedStack: fakePlannedStack{provider: "aws", runtime: "eks"}}
	bound, err := BindLiveQueueReplicasContext(ctx, planned, map[string]any{OutputQueueReplicas: 3})
	if err != nil {
		t.Fatal(err)
	}
	if bound != planned || planned.context != ctx || planned.replicas != 3 {
		t.Fatalf("bound=%T context=%v replicas=%d", bound, planned.context, planned.replicas)
	}
}
