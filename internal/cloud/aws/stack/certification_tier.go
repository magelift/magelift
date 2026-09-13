package stack

import (
	"strings"

	sdk "github.com/magelift/magelift/sdk/v1"
)

// awsPreviewFargateMagentoCertified is the evidenced Magento cell in
// docs/evidence/README.md: ECS Fargate Magento 2.4.9 preview. Denser SKUs stay experimental.
func awsPreviewFargateMagentoCertified(spec Spec) bool {
	if spec.Identity.Preset != sdk.PresetPreview {
		return false
	}
	if spec.Application.Version != "2.4.9" {
		return false
	}
	runtime := strings.TrimSpace(spec.Application.WebRuntime)
	if runtime != "" && runtime != "nginx-fpm" {
		return false
	}
	compute := strings.TrimSpace(spec.Catalog.Fargate.ComputeMode)
	switch compute {
	case "", "fargate":
	default:
		return false
	}
	switch spec.Catalog.DatabaseEngine {
	case "", DatabaseEngineRDSMySQL:
	default:
		return false
	}
	switch spec.Catalog.QueueMode {
	case "", QueueModeDB, QueueModeECSRabbitMQ:
	default:
		return false
	}
	switch spec.Catalog.SearchMode {
	case "", SearchModeDisabled:
	default:
		return false
	}
	return true
}
