// Package health defines provider-neutral health evidence.
package health

import "context"

type Status string

const (
	StatusHealthy     Status = "healthy"
	StatusUnhealthy   Status = "unhealthy"
	StatusUnavailable Status = "unavailable"
)

type Check struct {
	ID      string `json:"id" yaml:"id"`
	Status  Status `json:"status" yaml:"status"`
	Message string `json:"message" yaml:"message"`
}

type Report struct {
	Environment string  `json:"environment" yaml:"environment"`
	Target      string  `json:"target" yaml:"target"`
	Mode        string  `json:"mode" yaml:"mode"`
	Status      Status  `json:"status" yaml:"status"`
	Checks      []Check `json:"checks" yaml:"checks"`
}

// Checker supplies health evidence for a target without tying callers to a provider SDK.
type Checker interface {
	Check(context.Context) ([]Check, error)
}

func Summarize(checks []Check) Status {
	status := StatusHealthy
	for _, check := range checks {
		if check.Status == StatusUnhealthy {
			return StatusUnhealthy
		}
		if check.Status == StatusUnavailable {
			status = StatusUnavailable
		}
	}
	return status
}
