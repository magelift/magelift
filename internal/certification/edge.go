package certification

import (
	"errors"
	"fmt"
	"strings"

	sdk "github.com/magelift/magelift/sdk/v1"
)

// EdgeObservation is the provider-neutral result of the origin and edge
// safety probes that must precede a route mutation. Provider adapters own the
// health-check API and return this small observation to the core gate.
type EdgeObservation struct {
	OriginHealthy        bool `json:"originHealthy" yaml:"originHealthy"`
	RouteHealthy         bool `json:"routeHealthy" yaml:"routeHealthy"`
	DNSOwnershipVerified bool `json:"dnsOwnershipVerified" yaml:"dnsOwnershipVerified"`
	TLSVerified          bool `json:"tlsVerified" yaml:"tlsVerified"`
	PurgeVerified        bool `json:"purgeVerified" yaml:"purgeVerified"`
	WAFVerified          bool `json:"wafVerified" yaml:"wafVerified"`
	FailoverVerified     bool `json:"failoverVerified" yaml:"failoverVerified"`
	RollbackVerified     bool `json:"rollbackVerified" yaml:"rollbackVerified"`
}

// ValidateEdgeObservation is the apply-time safety gate shared by native and
// external edge adapters. It is deliberately stricter than plan validation:
// a syntactically valid edge intent is not evidence that traffic is safe to
// move.
func ValidateEdgeObservation(intent sdk.EdgeIntent, observation EdgeObservation) error {
	if err := sdk.ValidateEdgeIntent(intent); err != nil {
		return fmt.Errorf("validate edge intent: %w", err)
	}
	mode := strings.TrimSpace(intent.Mode)
	if mode == "" {
		switch {
		case strings.TrimSpace(intent.NativeProvider) != "" && strings.TrimSpace(intent.ExternalProvider) != "":
			mode = "both"
		case strings.TrimSpace(intent.NativeProvider) != "":
			mode = "native"
		case strings.TrimSpace(intent.ExternalProvider) != "":
			mode = "external"
		default:
			mode = "none"
		}
	}
	if mode == "none" {
		return nil
	}
	var problems []error
	if !observation.OriginHealthy {
		problems = append(problems, errors.New("edge origin health gate has not passed"))
	}
	if !observation.RouteHealthy {
		problems = append(problems, errors.New("edge route health gate has not passed"))
	}
	if len(intent.Domains) > 0 && !observation.DNSOwnershipVerified {
		problems = append(problems, errors.New("edge DNS ownership has not been verified"))
	}
	if intent.TLS && !observation.TLSVerified {
		problems = append(problems, errors.New("edge TLS verification has not passed"))
	}
	if intent.PurgeOnDeploy && !observation.PurgeVerified {
		problems = append(problems, errors.New("edge purge verification has not passed"))
	}
	if strings.TrimSpace(intent.WAFPolicyRef) != "" && !observation.WAFVerified {
		problems = append(problems, errors.New("edge WAF or security-policy verification has not passed"))
	}
	if strings.TrimSpace(intent.FailoverPolicyRef) != "" && (!observation.FailoverVerified || !observation.RollbackVerified) {
		problems = append(problems, errors.New("edge failover and rollback verification has not passed"))
	}
	return errors.Join(problems...)
}
