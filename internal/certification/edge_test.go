package certification

import (
	"strings"
	"testing"

	"github.com/magelift/magelift/sdk"
)

func TestValidateEdgeObservationRequiresOriginBeforeRouteMutation(t *testing.T) {
	intent := sdk.EdgeIntent{
		Mode: "external", ExternalProvider: "fastly", Lifecycle: sdk.ExternalLifecycleManaged, Certification: sdk.ExternalCertified,
		OriginHealthRef: "health/magento", OwnershipMarker: "magelift/edge/run-1",
	}
	if err := ValidateEdgeObservation(intent, EdgeObservation{RouteHealthy: true}); err == nil || !strings.Contains(err.Error(), "origin health") {
		t.Fatalf("unhealthy origin was accepted: %v", err)
	}
}

func TestValidateEdgeObservationRequiresDeclaredSecurityAndRecoveryChecks(t *testing.T) {
	intent := sdk.EdgeIntent{
		Mode: "both", NativeProvider: "cloudfront-waf", ExternalProvider: "fastly", Lifecycle: sdk.ExternalLifecycleManaged, Certification: sdk.ExternalCertified,
		OriginHealthRef: "health/magento", OwnershipMarker: "magelift/edge/run-1",
		Domains: []string{"shop.example.com"}, TLS: true, TLSMode: "managed", DNSMode: "customer-managed", PurgeOnDeploy: true,
		WAFPolicyRef: "policy/waf", FailoverPolicyRef: "policy/failover",
	}
	err := ValidateEdgeObservation(intent, EdgeObservation{OriginHealthy: true, RouteHealthy: true, DNSOwnershipVerified: true, TLSVerified: true, PurgeVerified: true, WAFVerified: true})
	if err == nil || !strings.Contains(err.Error(), "failover and rollback") {
		t.Fatalf("incomplete failover observation was accepted: %v", err)
	}
	if err := ValidateEdgeObservation(intent, EdgeObservation{OriginHealthy: true, RouteHealthy: true, DNSOwnershipVerified: true, TLSVerified: true, PurgeVerified: true, WAFVerified: true, FailoverVerified: true, RollbackVerified: true}); err != nil {
		t.Fatalf("complete edge observation was rejected: %v", err)
	}
}

func TestValidateEdgeObservationDoesNotRequireChecksForNoEdge(t *testing.T) {
	if err := ValidateEdgeObservation(sdk.EdgeIntent{Mode: "none"}, EdgeObservation{}); err != nil {
		t.Fatalf("no-edge intent was rejected: %v", err)
	}
}
