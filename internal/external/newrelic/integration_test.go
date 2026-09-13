package newrelic

import (
	"strings"
	"testing"
)

func TestPlanIntegrationKeepsWorkloadBoundariesAndReferencesOpaque(t *testing.T) {
	tests := []struct {
		name           string
		request        IntegrationRequest
		status         IntegrationStatus
		distribution   string
		reasonContains string
	}{
		{
			name:    "ecs native",
			request: IntegrationRequest{TargetProvider: "aws", TargetRuntime: "ecs-fargate", Workload: WorkloadECS, Mode: IntegrationNative, CredentialRef: "aws-secrets-manager://magelift/newrelic", Endpoint: "https://otlp.eu01.nr-data.net", NativeReference: "ecs-task-definition-ref", OwnershipMarker: "magelift/test/ecs"},
			status:  IntegrationConfigured, distribution: "opentelemetry-collector-contrib",
		},
		{
			name:    "kubernetes native",
			request: IntegrationRequest{TargetProvider: "gcp", TargetRuntime: "gke-autopilot", Workload: WorkloadKubernetes, Mode: IntegrationNative, CredentialRef: "gcp-secret-manager://magelift/newrelic", Endpoint: "https://otlp.eu01.nr-data.net", NativeReference: "cluster-ref", OwnershipMarker: "magelift/test/gke"},
			status:  IntegrationConfigured, distribution: NRDOTKubernetesDistribution,
		},
		{
			name:    "generic native is typed unsupported",
			request: IntegrationRequest{TargetProvider: "ovh", TargetRuntime: "mks", Workload: WorkloadGeneric, Mode: IntegrationNative, CredentialRef: "ovh-secret://magelift/newrelic", Endpoint: "https://otlp.nr-data.net", NativeReference: "ref", OwnershipMarker: "magelift/test/generic"},
			status:  IntegrationUnsupported, reasonContains: "OTLP",
		},
		{
			name:    "scaleway kubernetes native helm path",
			request: IntegrationRequest{TargetProvider: "scaleway", TargetRuntime: "kapsule", Workload: WorkloadKubernetes, Mode: IntegrationNative, CredentialRef: "scaleway-secret://magelift/newrelic", Endpoint: "https://otlp.nr-data.net", NativeReference: "cluster-ref", OwnershipMarker: "magelift/test/kapsule"},
			status:  IntegrationConfigured, distribution: NRDOTKubernetesDistribution,
		},
		{
			name:    "ovh kubernetes native helm path",
			request: IntegrationRequest{TargetProvider: "ovh", TargetRuntime: "mks", Workload: WorkloadKubernetes, Mode: IntegrationNative, CredentialRef: "ovh-secret://magelift/newrelic", Endpoint: "https://otlp.nr-data.net", NativeReference: "cluster-ref", OwnershipMarker: "magelift/test/mks"},
			status:  IntegrationConfigured, distribution: NRDOTKubernetesDistribution,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan, err := PlanIntegration(test.request)
			if err != nil {
				t.Fatal(err)
			}
			if plan.Provider != "newrelic" || plan.Status != test.status || plan.CollectorDistribution != test.distribution {
				t.Fatalf("plan = %#v", plan)
			}
			if test.reasonContains != "" && !strings.Contains(plan.Reason, test.reasonContains) {
				t.Fatalf("reason = %q", plan.Reason)
			}
			if strings.Contains(plan.Reason, "aws-secrets-manager") || strings.Contains(plan.Reason, "gcp-secret-manager") || strings.Contains(plan.Reason, "ovh-secret") {
				t.Fatalf("credential reference leaked into reason: %#v", plan)
			}
		})
	}
}

func TestPlanIntegrationRequiresRegionEndpointForConfiguredNativePath(t *testing.T) {
	plan, err := PlanIntegration(IntegrationRequest{TargetProvider: "aws", TargetRuntime: "ecs-fargate", Workload: WorkloadECS, Mode: IntegrationNative, CredentialRef: "aws-secrets-manager://magelift/newrelic", NativeReference: "ecs-task-definition-ref", OwnershipMarker: "magelift/test/region"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != IntegrationUnavailable || !strings.Contains(plan.Reason, "endpoint") {
		t.Fatalf("missing endpoint plan = %#v", plan)
	}
}
