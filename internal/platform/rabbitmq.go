package platform

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// RabbitMQTopologyClass is the Magento broker cold-boundary class.
const (
	RabbitMQTopologyNone       = ""
	RabbitMQTopologySingleNode = "single-node"
	RabbitMQTopologyQuorum     = "quorum"
)

// RabbitMQTopologyClassFor returns single-node for one replica and quorum for
// multi-node HA brokers. Zero replicas means the environment is not on RabbitMQ.
func RabbitMQTopologyClassFor(replicas int) string {
	switch {
	case replicas <= 0:
		return RabbitMQTopologyNone
	case replicas == 1:
		return RabbitMQTopologySingleNode
	default:
		return RabbitMQTopologyQuorum
	}
}

// RefuseInPlaceRabbitMQTopologyChange fails closed before mutate when YAML
// would convert a live single-node Magento broker into a quorum/HA broker or
// the reverse. The operator path is a new environment, restore, or rebuild.
func RefuseInPlaceRabbitMQTopologyChange(previousReplicas, nextReplicas int) error {
	previous := RabbitMQTopologyClassFor(previousReplicas)
	next := RabbitMQTopologyClassFor(nextReplicas)
	if previous == RabbitMQTopologyNone || next == RabbitMQTopologyNone || previous == next {
		return nil
	}
	return fmt.Errorf("MageLift: refusing in-place RabbitMQ change from %s (%d node(s)) to %s (%d node(s)); create a new environment, restore, or rebuild instead of converting a live broker", previous, previousReplicas, next, nextReplicas)
}

// LiveQueueReplicaBinder is implemented by planned stacks that can refuse an
// in-place Magento RabbitMQ 1↔quorum change once live replica count is known.
type LiveQueueReplicaBinder interface {
	WithLiveQueueReplicas(replicas int) (PlannedStack, error)
}

// QueueReplicasFromOutputs reads the optional live broker replica export.
func QueueReplicasFromOutputs(outputs map[string]any) (int, bool) {
	if len(outputs) == 0 {
		return 0, false
	}
	value, ok := outputs[OutputQueueReplicas]
	if !ok || value == nil {
		return 0, false
	}
	switch typed := value.(type) {
	case int:
		return typed, true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		n, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		return int(n), true
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

// BindLiveQueueReplicas copies the live replica export onto a planned stack so
// Validate can refuse an in-place 1↔quorum Magento broker change. Missing
// outputs (first create) leave the plan unchanged.
func BindLiveQueueReplicas(planned PlannedStack, outputs map[string]any) (PlannedStack, error) {
	if planned == nil {
		return nil, fmt.Errorf("planned stack is required")
	}
	binder, ok := planned.(LiveQueueReplicaBinder)
	if !ok {
		return planned, nil
	}
	replicas, found := QueueReplicasFromOutputs(outputs)
	if !found {
		return planned, nil
	}
	return binder.WithLiveQueueReplicas(replicas)
}
