package health

import "testing"

func TestSummarizeUsesWorstStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		checks []Check
		want   Status
	}{
		{name: "healthy", checks: []Check{{Status: StatusHealthy}}, want: StatusHealthy},
		{name: "unavailable", checks: []Check{{Status: StatusHealthy}, {Status: StatusUnavailable}}, want: StatusUnavailable},
		{name: "unhealthy-beats-unavailable", checks: []Check{{Status: StatusUnavailable}, {Status: StatusUnhealthy}}, want: StatusUnhealthy},
		{name: "degraded", checks: []Check{{Status: StatusHealthy}, {Status: StatusDegraded}}, want: StatusDegraded},
		{name: "stale", checks: []Check{{Status: StatusHealthy}, {Status: StatusStale}}, want: StatusStale},
		{name: "stale-beats-unavailable", checks: []Check{{Status: StatusStale}, {Status: StatusUnavailable}}, want: StatusStale},
		{name: "unavailable-then-stale", checks: []Check{{Status: StatusUnavailable}, {Status: StatusStale}}, want: StatusStale},
		{name: "degraded-beats-unavailable", checks: []Check{{Status: StatusUnavailable}, {Status: StatusDegraded}}, want: StatusDegraded},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := Summarize(test.checks); got != test.want {
				t.Fatalf("Summarize() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestClassifyLayer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		id   string
		want Layer
	}{
		{id: "runtime.web", want: LayerMagento},
		{id: "runtime.magento", want: LayerMagento},
		{id: "runtime.database", want: LayerDependency},
		{id: "runtime.ecs.service", want: LayerService},
		{id: "runtime.ecs.task.task-a", want: LayerService},
		{id: "runtime.ecs.rollout", want: LayerDeployment},
		{id: "runtime.kube.deployment", want: LayerDeployment},
		{id: "config.resolved", want: LayerInfrastructure},
		{id: "output.applicationURL", want: LayerInfrastructure},
		{id: "runtime.observe", want: LayerInfrastructure},
		{id: "target.supported", want: LayerInfrastructure},
		{id: "unknown.probe", want: ""},
	}
	for _, test := range tests {
		t.Run(test.id, func(t *testing.T) {
			t.Parallel()
			if got := ClassifyLayer(test.id); got != test.want {
				t.Fatalf("ClassifyLayer(%q) = %q, want %q", test.id, got, test.want)
			}
		})
	}
}

func TestSummarizeMagentoDownKeepsDependencyVisible(t *testing.T) {
	t.Parallel()
	checks := []Check{
		{ID: "runtime.database", Layer: LayerDependency, Status: StatusHealthy, Message: "database is up"},
		{ID: "runtime.web", Layer: LayerMagento, Status: StatusUnhealthy, Message: "Magento probe failed"},
	}
	if got := Summarize(checks); got != StatusUnhealthy {
		t.Fatalf("Summarize() = %q, want %q", got, StatusUnhealthy)
	}
	if checks[0].Layer != LayerDependency || checks[0].Status != StatusHealthy {
		t.Fatalf("database check was dropped or reclassified: %#v", checks[0])
	}
	if checks[1].Layer != LayerMagento {
		t.Fatalf("Magento check layer = %q", checks[1].Layer)
	}
}
