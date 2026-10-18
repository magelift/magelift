// Package health defines provider-neutral health evidence.
package health

import (
	"context"
	"strings"
	"time"
)

type Status string

const (
	StatusHealthy     Status = "healthy"
	StatusUnhealthy   Status = "unhealthy"
	StatusDegraded    Status = "degraded"
	StatusStale       Status = "stale"
	StatusUnavailable Status = "unavailable"
)

type Freshness string

const (
	FreshnessCurrent Freshness = "current"
	FreshnessStale   Freshness = "stale"
	FreshnessUnknown Freshness = "unknown"
)

type Layer string

const (
	LayerInfrastructure Layer = "infrastructure"
	LayerService        Layer = "service"
	LayerMagento        Layer = "magento"
	LayerDependency     Layer = "dependency"
	LayerDeployment     Layer = "deployment"
)

type Check struct {
	ID         string    `json:"id" yaml:"id"`
	Layer      Layer     `json:"layer,omitempty" yaml:"layer,omitempty"`
	Status     Status    `json:"status" yaml:"status"`
	Message    string    `json:"message" yaml:"message"`
	Source     string    `json:"source,omitempty" yaml:"source,omitempty"`
	ObservedAt time.Time `json:"observedAt,omitempty" yaml:"observedAt,omitempty"`
	Freshness  Freshness `json:"freshness,omitempty" yaml:"freshness,omitempty"`
}

type Report struct {
	Environment string    `json:"environment" yaml:"environment"`
	Target      string    `json:"target" yaml:"target"`
	Mode        string    `json:"mode" yaml:"mode"`
	Status      Status    `json:"status" yaml:"status"`
	Checks      []Check   `json:"checks" yaml:"checks"`
	ObservedAt  time.Time `json:"observedAt" yaml:"observedAt"`
	Source      string    `json:"source" yaml:"source"`
	Freshness   Freshness `json:"freshness" yaml:"freshness"`
}

// Checker supplies health evidence for a target without tying callers to a provider SDK.
type Checker interface {
	Check(context.Context) ([]Check, error)
}

func Summarize(checks []Check) Status {
	status := StatusHealthy
	for _, check := range checks {
		if statusRank(check.Status) > statusRank(status) {
			status = check.Status
		}
	}
	return status
}

// ClassifyLayer maps a check ID to infrastructure, service, magento,
// dependency, or deployment when the ID is a known probe. Unknown IDs
// return "" so adapters can set Layer explicitly when they can observe it.
func ClassifyLayer(id string) Layer {
	switch {
	case id == "runtime.web", id == "runtime.magento", strings.HasPrefix(id, "runtime.magento."):
		return LayerMagento
	case strings.Contains(id, "rollout"), strings.HasSuffix(id, ".deployment"), strings.Contains(id, ".deployment."):
		return LayerDeployment
	case strings.HasPrefix(id, "runtime.database"), strings.HasPrefix(id, "runtime.sql"), strings.HasPrefix(id, "runtime.cache"), strings.HasPrefix(id, "runtime.queue"), strings.HasPrefix(id, "runtime.search"):
		return LayerDependency
	case strings.HasPrefix(id, "runtime.ecs.service"), strings.HasPrefix(id, "runtime.ecs.task"), strings.HasPrefix(id, "runtime.kube"):
		return LayerService
	case strings.HasPrefix(id, "config."), strings.HasPrefix(id, "output."), id == "runtime.observe", id == "target.supported":
		return LayerInfrastructure
	default:
		return ""
	}
}

func AnnotateLayer(check Check) Check {
	if check.Layer == "" {
		check.Layer = ClassifyLayer(check.ID)
	}
	return check
}

func statusRank(status Status) int {
	switch status {
	case StatusUnavailable:
		return 1
	case StatusStale:
		return 2
	case StatusDegraded:
		return 3
	case StatusUnhealthy:
		return 4
	default:
		return 0
	}
}
