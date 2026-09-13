package v1

import (
	"strings"
	"testing"
)

func TestValidateObservabilityIntentRequiresNewRelicCredentialReference(t *testing.T) {
	intent := ObservabilityIntent{
		ExternalProvider: "newrelic", Lifecycle: ExternalLifecycleExtension, Certification: ExternalExperimental,
		Endpoint: "https://otlp.nr-data.net", OwnershipMarker: "magelift/test/newrelic", Signals: []string{"metrics"},
	}
	if err := ValidateObservabilityIntent(intent); err == nil || !strings.Contains(err.Error(), "license-key credential") {
		t.Fatalf("New Relic credential omission error = %v", err)
	}
}
