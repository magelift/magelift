package infra

type LifecycleOperation string

const (
	OperationDurableInfrastructure LifecycleOperation = "durable-infrastructure"
	OperationCandidateResource     LifecycleOperation = "candidate-resource"
	OperationDeploymentLock        LifecycleOperation = "deployment-lock"
	OperationReleaseOrchestration  LifecycleOperation = "release-orchestration"
)

type LifecycleOwner string

const (
	OwnerTarget       LifecycleOwner = "target"
	OwnerOrchestrator LifecycleOwner = "orchestrator"
)

func OwnerOf(operation LifecycleOperation) (LifecycleOwner, bool) {
	switch operation {
	case OperationDurableInfrastructure:
		return OwnerTarget, true
	case OperationCandidateResource, OperationDeploymentLock, OperationReleaseOrchestration:
		return OwnerOrchestrator, true
	default:
		return "", false
	}
}
