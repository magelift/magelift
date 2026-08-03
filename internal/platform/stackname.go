package platform

import (
	"fmt"
	"strings"

	sdk "github.com/acourtiol/magelift/sdk/v1"
)

// FormatStackName builds a DIY-backend-safe stack identity that includes
// provider and runtime so targets sharing project+env do not collide
// (for example aws/ecs-fargate vs gcp/gke-autopilot, or a future aws/eks).
func FormatStackName(project, environment string, provider sdk.ProviderID, runtime sdk.RuntimeID) string {
	project = strings.TrimSpace(project)
	environment = strings.TrimSpace(environment)
	p := strings.TrimSpace(string(provider))
	r := strings.ReplaceAll(strings.TrimSpace(string(runtime)), ".", "-")
	switch {
	case p == "" && r == "":
		return project + "-" + environment
	case r == "":
		return project + "-" + environment + "-" + p
	case p == "":
		return project + "-" + environment + "-" + r
	default:
		return project + "-" + environment + "-" + p + "-" + r
	}
}

// RequireOutputs fails when required keys are missing from Automation API outputs.
func RequireOutputs(outputs map[string]any, keys []string) error {
	var missing []string
	for _, key := range keys {
		if _, ok := outputs[key]; !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("Pulumi stack outputs missing required keys: %s", strings.Join(missing, ", "))
}
