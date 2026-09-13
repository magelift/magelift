package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/dumpimport"
)

func TestSchemaEpochTransportConfigured(t *testing.T) {
	t.Parallel()
	if schemaEpochTransportConfigured(dumpimport.Options{}) {
		t.Fatal("empty options opened the default localhost mysql path")
	}
	if !schemaEpochTransportConfigured(dumpimport.Options{Runner: dumpimport.RunnerKube, Namespace: "magento", Pod: "deploy/web"}) {
		t.Fatal("kube transport was rejected")
	}
	if schemaEpochTransportConfigured(dumpimport.Options{Runner: dumpimport.RunnerKube, Namespace: "magento"}) {
		t.Fatal("kube without pod or selector was accepted")
	}
	if !schemaEpochTransportConfigured(dumpimport.Options{Host: "10.0.0.8"}) {
		t.Fatal("explicit host was rejected")
	}
}

func TestObserveLiveSchemaEpochSkipsUnconfiguredTransport(t *testing.T) {
	o := testOptions(nil, &fakeTerminal{interactive: false})
	o.configPath = "magelift.yaml"
	_, err := o.observeLiveSchemaEpoch(context.Background())
	if err == nil || !strings.Contains(err.Error(), "transport is not configured") {
		t.Fatalf("error = %v", err)
	}
}
