package localdev

import (
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/config"
)

func parityBuild() config.BuildSpec {
	return config.BuildSpec{
		Application: config.Application{Version: "2.4.9", WebRuntime: "nginx-fpm"},
		Build:       config.Build{PHP: "8.5", Composer: config.Composer{Version: "2.10"}},
	}
}

func substituteMap(t *testing.T, plan RuntimePlan) map[string]string {
	t.Helper()
	got := map[string]string{}
	for _, substitute := range plan.Substitutes {
		got[substitute.Cloud] = substitute.Local
	}
	return got
}

func TestPlanForRecordsElastiCacheSubstitute(t *testing.T) {
	plan, err := PlanFor(parityBuild(), CloudHints{Provider: "aws", CacheProduct: "elasticache-valkey"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Cache.Family != "valkey" {
		t.Fatalf("cache family = %q, want valkey", plan.Cache.Family)
	}
	got := substituteMap(t, plan)
	if !strings.HasPrefix(got["ElastiCache Valkey"], "valkey ") {
		t.Fatalf("substitutes = %#v", plan.Substitutes)
	}
}

func TestPlanForSteersArtemisAndRabbitModes(t *testing.T) {
	for _, tc := range []struct {
		name      string
		hints     CloudHints
		wantCloud string
	}{
		{"artemis", CloudHints{Provider: "aws", QueueMode: "ecs-artemis"}, "ECS Artemis"},
		{"gke rabbit", CloudHints{Provider: "gcp", QueueMode: "rabbitmq"}, "GKE RabbitMQ"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := PlanFor(parityBuild(), tc.hints)
			if err != nil {
				t.Fatal(err)
			}
			if plan.Queue.Family != "rabbitmq" {
				t.Fatalf("queue family = %q, want rabbitmq", plan.Queue.Family)
			}
			got := substituteMap(t, plan)
			if !strings.HasPrefix(got[tc.wantCloud], "rabbitmq ") {
				t.Fatalf("substitutes = %#v", plan.Substitutes)
			}
		})
	}
}

func TestPlanForRecordsProvisionedSearchSubstitute(t *testing.T) {
	plan, err := PlanFor(parityBuild(), CloudHints{Provider: "aws", SearchMode: "provisioned"})
	if err != nil {
		t.Fatal(err)
	}
	got := substituteMap(t, plan)
	if !strings.HasPrefix(got["OpenSearch provisioned domain"], "opensearch ") {
		t.Fatalf("substitutes = %#v", plan.Substitutes)
	}
}

func TestPlanForWarnsWhenCloudHasLessThanLocal(t *testing.T) {
	plan, err := PlanFor(parityBuild(), CloudHints{Provider: "aws", SearchMode: "disabled", QueueMode: "db"})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(plan.Warnings, "\n")
	for _, want := range []string{
		"cloud search is disabled for this environment; local OpenSearch does not prove cloud behavior",
		"cloud queues are database-backed for this environment; the local broker does not prove cloud behavior",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("warnings = %#v", plan.Warnings)
		}
	}
	gcp, err := PlanFor(parityBuild(), CloudHints{Provider: "gcp", QueueMode: "database"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(gcp.Warnings, "\n"), "database-backed") {
		t.Fatalf("warnings = %#v", gcp.Warnings)
	}
}

func TestPlanForRefusesRedisWithoutHatch(t *testing.T) {
	build := parityBuild()
	build.Local = config.LocalRuntime{Cache: config.LocalService{Family: "redis"}}
	_, err := PlanFor(build, CloudHints{Provider: "aws"})
	if err == nil || !strings.Contains(err.Error(), "Adobe compatibility row") {
		t.Fatalf("err = %v, want Adobe-row refusal", err)
	}
}
