package schema

import (
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	t.Parallel()
	if problems := Validate(nil, "gke-autopilot"); len(problems) == 0 {
		t.Error("nil target passed validation")
	}
	cases := []struct {
		name    string
		mutate  func(*GCPTarget)
		runtime string
		want    string
	}{
		{"missing project", func(g *GCPTarget) { g.Project = "" }, "gke-autopilot", "target.gcp.project is required"},
		{"bad runtime", func(*GCPTarget) {}, "gke-nano", "target.runtime must be gke-autopilot or gke-standard"},
		{"bad availability", func(g *GCPTarget) { g.CloudSQLAvailability = "GLOBAL" }, "gke-autopilot", "cloudSqlAvailability must be ZONAL or REGIONAL"},
		{"bad search mode", func(g *GCPTarget) { g.OpenSearchMode = "solr" }, "gke-autopilot", "openSearchMode must be opensearch or disabled"},
		{"bad queue mode", func(g *GCPTarget) { g.QueueMode = "nats" }, "gke-autopilot", "queueMode must be database or rabbitmq"},
	}
	for _, tc := range cases {
		target := &GCPTarget{Project: "example-invalid-preview"}
		tc.mutate(target)
		problems := Validate(target, tc.runtime)
		found := false
		for _, problem := range problems {
			if strings.Contains(problem, tc.want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s: problems = %v, want %q", tc.name, problems, tc.want)
		}
	}
}

func TestValidateAcceptsMinimalTarget(t *testing.T) {
	t.Parallel()
	if problems := Validate(&GCPTarget{Project: "example-invalid-preview"}, "gke-autopilot"); len(problems) != 0 {
		t.Fatalf("minimal target problems = %v", problems)
	}
}
