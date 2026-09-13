package localdev

import (
	"fmt"
	"strings"

	"github.com/magelift/magelift/internal/config"
)

// CloudHints describes the selected cloud environment so local Compose can
// follow it with named substitutes instead of a second catalog.
type CloudHints struct {
	Environment string
	Preset      string
	Provider    string

	DatabaseEngine  string
	DatabaseVersion string
	SearchMode      string
	QueueMode       string
	CacheProduct    string

	IsolateCacheAndSession bool
}

// Substitute records a managed cloud product that has no container twin.
type Substitute struct {
	Cloud string `json:"cloud" yaml:"cloud"`
	Local string `json:"local" yaml:"local"`
}

// CloudOnlyNote is the doctor/local warning that edge products stay in cloud.
const CloudOnlyNote = "CloudFront, Cloud Armor, and Fastly are cloud-only; magelift local does not emulate them"

// HintsFromConfig extracts iso-prod inputs from a resolved environment.
func HintsFromConfig(cfg config.Config) CloudHints {
	preset := strings.TrimSpace(cfg.Preset)
	if preset == "" {
		preset = strings.TrimSpace(cfg.Defaults.Preset)
	}
	hints := CloudHints{
		Preset:                 preset,
		Provider:               strings.TrimSpace(cfg.Target.Provider),
		IsolateCacheAndSession: preset == "standard" || preset == "high-availability",
	}
	switch hints.Provider {
	case "aws":
		hints.CacheProduct = "elasticache-valkey"
		if cfg.Target.AWS != nil {
			catalog := cfg.Target.AWS.Catalog
			hints.DatabaseEngine = strings.TrimSpace(catalog.DatabaseEngine)
			switch hints.DatabaseEngine {
			case "aurora-mysql":
				hints.DatabaseVersion = strings.TrimSpace(catalog.Versions.AuroraMySQL)
			case "rds-mysql":
				hints.DatabaseVersion = strings.TrimSpace(catalog.Versions.MySQL)
			case "rds-mariadb":
				hints.DatabaseVersion = strings.TrimSpace(catalog.Versions.MariaDB)
			}
			hints.SearchMode = strings.TrimSpace(catalog.SearchMode)
			hints.QueueMode = defaultAWSQueueHint(hints.Preset, catalog.QueueMode)
		} else if hints.Preset == "preview" {
			hints.DatabaseEngine = "rds-mysql"
			hints.SearchMode = "serverless"
			hints.QueueMode = "db"
		} else {
			hints.DatabaseEngine = "aurora-mysql"
			hints.SearchMode = "provisioned"
			hints.QueueMode = "ecs-rabbitmq"
		}
	case "gcp":
		hints.DatabaseEngine = "cloudsql-mysql"
		hints.CacheProduct = "memorystore"
		if cfg.Target.GCP != nil {
			hints.QueueMode = strings.TrimSpace(cfg.Target.GCP.QueueMode)
			hints.SearchMode = strings.TrimSpace(cfg.Target.GCP.OpenSearchMode)
		}
	case "scaleway":
		hints.DatabaseEngine = "scaleway-mysql"
		if cfg.Target.Scaleway != nil && strings.TrimSpace(cfg.Target.Scaleway.CacheMode) == "redis" {
			hints.CacheProduct = "scaleway-redis"
		}
	case "ovh":
		hints.DatabaseEngine = "ovh-mysql"
	}
	return hints
}

func applyCloudHints(local *config.LocalRuntime, hints CloudHints, row localReleaseRow) error {
	if family, _, ok := databaseSubstitute(hints.DatabaseEngine); ok && strings.TrimSpace(local.Database.Family) == "" {
		if err := preferFamily(&local.Database, family, mysqlMajor(hints.DatabaseVersion), row.Database); err != nil {
			return err
		}
	}
	if hints.SearchMode == "serverless" && strings.TrimSpace(local.Search.Family) == "" {
		if err := preferFamily(&local.Search, "opensearch", "", row.Search); err != nil {
			return err
		}
	}
	if queueUsesRabbitFamily(hints.QueueMode) && strings.TrimSpace(local.Queue.Family) == "" {
		if err := preferFamily(&local.Queue, "rabbitmq", "", row.Queue); err != nil {
			return err
		}
	}
	if _, ok := cacheTwinProduct(hints.CacheProduct); ok && strings.TrimSpace(local.Cache.Family) == "" {
		if err := preferFamily(&local.Cache, "valkey", "", row.Cache); err != nil {
			return err
		}
	}
	return nil
}

func namedSubstitutes(hints CloudHints, plan RuntimePlan) []Substitute {
	var substitutes []Substitute
	if family, cloudName, ok := databaseSubstitute(hints.DatabaseEngine); ok && strings.EqualFold(plan.Database.Family, family) {
		substitutes = append(substitutes, Substitute{Cloud: cloudName, Local: plan.Database.Family + " " + plan.Database.Version})
	}
	if hints.SearchMode == "serverless" {
		substitutes = append(substitutes, Substitute{Cloud: "OpenSearch Serverless", Local: plan.Search.Family + " " + plan.Search.Version})
	}
	if hints.SearchMode == "provisioned" {
		substitutes = append(substitutes, Substitute{Cloud: "OpenSearch provisioned domain", Local: plan.Search.Family + " " + plan.Search.Version})
	}
	switch hints.QueueMode {
	case "amazon-mq":
		substitutes = append(substitutes, Substitute{Cloud: "Amazon MQ", Local: plan.Queue.Family + " " + plan.Queue.Version})
	case "ecs-rabbitmq":
		substitutes = append(substitutes, Substitute{Cloud: "ECS RabbitMQ", Local: plan.Queue.Family + " " + plan.Queue.Version})
	case "ecs-artemis":
		substitutes = append(substitutes, Substitute{Cloud: "ECS Artemis", Local: plan.Queue.Family + " " + plan.Queue.Version})
	case "rabbitmq":
		substitutes = append(substitutes, Substitute{Cloud: rabbitWorkloadName(hints.Provider), Local: plan.Queue.Family + " " + plan.Queue.Version})
	}
	if cloudName, ok := cacheTwinProduct(hints.CacheProduct); ok {
		substitutes = append(substitutes, Substitute{Cloud: cloudName, Local: plan.Cache.Family + " " + plan.Cache.Version})
	}
	return substitutes
}

// cacheTwinProduct names the managed Valkey twin for a cloud cache
// product. ElastiCache and Memorystore both pair with a local Valkey
// container; anything else has no container twin rule.
func cacheTwinProduct(product string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(product)) {
	case "elasticache-valkey":
		return "ElastiCache Valkey", true
	case "memorystore":
		return "Memorystore", true
	default:
		return "", false
	}
}

// rabbitWorkloadName qualifies a bare rabbitmq queue mode by provider.
func rabbitWorkloadName(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "gcp":
		return "GKE RabbitMQ"
	case "aws":
		return "EKS RabbitMQ"
	default:
		return "managed RabbitMQ"
	}
}

func databaseSubstitute(engine string) (family, cloudName string, ok bool) {
	switch strings.ToLower(strings.TrimSpace(engine)) {
	case "aurora-mysql":
		return "mysql", "Aurora MySQL", true
	case "rds-mysql":
		return "mysql", "RDS MySQL", true
	case "cloudsql-mysql":
		return "mysql", "Cloud SQL MySQL", true
	case "scaleway-mysql":
		return "mysql", "Scaleway MySQL", true
	case "ovh-mysql":
		return "mysql", "OVH MySQL", true
	case "rds-mariadb":
		return "mariadb", "RDS MariaDB", true
	default:
		return "", "", false
	}
}

func preferFamily(requested *config.LocalService, family, major string, rows []localServiceRow) error {
	match, ok := firstFamily(rows, family, major)
	if !ok {
		choices := make([]string, 0, len(rows))
		for _, row := range rows {
			choices = append(choices, row.Family+" "+row.Version)
		}
		return fmt.Errorf("no verified local %s container twin; nearest supported choices: %s", family, strings.Join(choices, ", "))
	}
	requested.Family = match.Family
	requested.Version = match.Version
	return nil
}

func firstFamily(rows []localServiceRow, family, major string) (localServiceRow, bool) {
	if major != "" {
		for _, row := range rows {
			if row.Family == family && serviceVersionMatches(major, row.Version) {
				return row, true
			}
		}
	}
	for _, row := range rows {
		if row.Family == family {
			return row, true
		}
	}
	return localServiceRow{}, false
}

func mysqlMajor(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return ""
	}
	if strings.HasPrefix(version, "8.0.mysql_aurora") || strings.Contains(version, ".mysql_aurora.3.") {
		return "8.0"
	}
	parts := strings.Split(version, ".")
	if len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
		return parts[0] + "." + parts[1]
	}
	return version
}

func defaultAWSQueueHint(preset, queueMode string) string {
	if mode := strings.TrimSpace(queueMode); mode != "" {
		return mode
	}
	if strings.TrimSpace(preset) == "preview" {
		return "db"
	}
	return "ecs-rabbitmq"
}

func queueUsesRabbitFamily(mode string) bool {
	switch strings.TrimSpace(mode) {
	case "amazon-mq", "ecs-rabbitmq", "ecs-artemis", "rabbitmq":
		return true
	default:
		return false
	}
}
