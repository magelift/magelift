package main

import (
	"strings"
	"testing"
)

func TestAcceptancePlanUsesProviderNeutralSignalsAndLowRetention(t *testing.T) {
	plan := acceptancePlan("magelift/scaleway/observability/test")
	if plan.TargetProvider != "scaleway" || plan.TargetRuntime != "scaleway-cockpit-live-cell" {
		t.Fatalf("plan target = %#v", plan)
	}
	if len(plan.Bindings) != 2 || len(plan.Alerts) != 0 || len(plan.Dashboards) != 0 {
		t.Fatalf("plan shape = %#v", plan)
	}
	for _, binding := range plan.Bindings {
		if binding.Destination != "scaleway-cockpit" || binding.RetentionDays != 1 || binding.OwnershipMarker != plan.OwnershipMarker {
			t.Fatalf("binding = %#v", binding)
		}
	}
}

func TestRequiredPartRejectsMultilineValues(t *testing.T) {
	if err := requiredPart("ok\nnot-ok", "marker"); err == nil || !strings.Contains(err.Error(), "single-line") {
		t.Fatalf("requiredPart error = %v", err)
	}
}
