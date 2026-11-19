package registry

import (
	"slices"
	"testing"

	"github.com/magelift/magelift/internal/providerhost"
	"github.com/magelift/magelift/sdk"
)

func TestStandaloneDescriptorsDriveDefaultAndOptInRegistration(t *testing.T) {
	defaultModules, err := NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(defaultModules.Close)
	experimentalModules, err := NewDefaultWithExperimental()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(experimentalModules.Close)

	providers := make([]string, 0)
	for _, runtime := range providerhost.StandaloneRuntimes() {
		if !slices.Contains(providers, string(runtime.Provider)) {
			providers = append(providers, string(runtime.Provider))
		}
		defaultModule, found := defaultModules.Module(runtime.Provider, runtime.Runtime)
		if runtime.OptIn {
			if found {
				t.Errorf("opt-in runtime %s/%s registered by default", runtime.Provider, runtime.Runtime)
			}
		} else {
			if !found {
				t.Errorf("default runtime %s/%s missing", runtime.Provider, runtime.Runtime)
			} else if defaultModule.CertificationTier() != runtime.Tier || !slices.Equal(defaultModule.OutputKeys(), runtime.OutputKeys) {
				t.Errorf("default runtime %s/%s does not match descriptor", runtime.Provider, runtime.Runtime)
			}
		}
		experimentalModule, found := experimentalModules.Module(runtime.Provider, runtime.Runtime)
		if !found {
			t.Errorf("runtime %s/%s missing from opt-in set", runtime.Provider, runtime.Runtime)
		} else if experimentalModule.CertificationTier() != runtime.Tier || !slices.Equal(experimentalModule.OutputKeys(), runtime.OutputKeys) {
			t.Errorf("experimental runtime %s/%s does not match descriptor", runtime.Provider, runtime.Runtime)
		}
	}
	if got := providerhost.StandaloneProviderIDs(); !slices.Equal(got, providers) {
		t.Fatalf("standalone providers = %v, runtime descriptor providers = %v", got, providers)
	}
	for _, provider := range providers {
		if !providerhost.IsStandaloneProvider(provider) {
			t.Errorf("registered provider %q is not eligible for standalone acquisition", provider)
		}
	}
}

func TestAWSStandaloneOutputKeysCoverBothRuntimeContracts(t *testing.T) {
	for _, descriptor := range providerhost.StandaloneRuntimes() {
		if descriptor.Provider != "aws" {
			continue
		}
		for _, key := range []string{
			"edgeDistributionId", "mediaBucket", "mediaDistributionId", "clusterArn",
			"deployTaskDefinitionArn", "queueHost", "databaseSecretName", "encryptionKeySecretName",
		} {
			found := false
			for _, output := range descriptor.OutputKeys {
				if output == key {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("AWS %s descriptor omits output %q", descriptor.Runtime, key)
			}
		}
	}
}

func TestNewDefaultRegistersAlphaSet(t *testing.T) {
	modules, err := NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(modules.Close)
	for _, want := range [][2]string{
		{"aws", "ecs-fargate"},
		{"gcp", "gke-autopilot"},
		{"gcp", "gke-standard"},
		{"azure", "aks"},
	} {
		if _, found := modules.Module(sdk.ProviderID(want[0]), sdk.RuntimeID(want[1])); !found {
			t.Errorf("alpha set lacks %s/%s", want[0], want[1])
		}
	}
	for _, absent := range [][2]string{
		{"aws", "eks"},
		{"ovh", "mks"},
		{"scaleway", "kapsule"},
	} {
		if _, found := modules.Module(sdk.ProviderID(absent[0]), sdk.RuntimeID(absent[1])); found {
			t.Errorf("alpha set registers deferred %s/%s", absent[0], absent[1])
		}
	}
}

func TestNewDefaultWithExperimentalAddsDeferred(t *testing.T) {
	modules, err := NewDefaultWithExperimental()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(modules.Close)
	for _, want := range [][2]string{
		{"aws", "ecs-fargate"},
		{"aws", "eks"},
		{"ovh", "mks"},
		{"scaleway", "kapsule"},
		{"gcp", "gke-autopilot"},
		{"gcp", "gke-standard"},
		{"azure", "aks"},
	} {
		if _, found := modules.Module(sdk.ProviderID(want[0]), sdk.RuntimeID(want[1])); !found {
			t.Errorf("experimental set lacks %s/%s", want[0], want[1])
		}
	}
}

func TestDefaultSetSharesHookAndModuleLifecycle(t *testing.T) {
	modules, hooks, err := NewDefaultSet()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(modules.Close)
	if hooks.Close != nil {
		t.Fatal("default hooks must share module-owned provider sessions")
	}
	for _, want := range [][2]string{
		{"aws", "ecs-fargate"},
		{"gcp", "gke-autopilot"},
		{"gcp", "gke-standard"},
		{"azure", "aks"},
	} {
		if _, found := modules.Module(sdk.ProviderID(want[0]), sdk.RuntimeID(want[1])); !found {
			t.Errorf("default set lacks %s/%s", want[0], want[1])
		}
	}
}
