package kube

import (
	"testing"

	"github.com/magelift/magelift/internal/platform"
)

func TestDatabaseCredentialsSecretNameIsStable(t *testing.T) {
	if got, want := DatabaseCredentialsSecretName("shop-preview-app"), "shop-preview-app-db-credentials"; got != want {
		t.Fatalf("secret name = %q, want %q", got, want)
	}
	if got, want := DatabaseAdminCredentialsSecretName("shop-preview-app"), "shop-preview-app-db-admin-credentials"; got != want {
		t.Fatalf("admin secret name = %q, want %q", got, want)
	}
	if got, want := EncryptionKeySecretName("shop-preview-app"), "shop-preview-app-encryption-key"; got != want {
		t.Fatalf("encryption secret name = %q, want %q", got, want)
	}
	if got, want := QueuePasswordSecretName("shop-preview-app"), "shop-preview-app-queue-password"; got != want {
		t.Fatalf("queue password secret name = %q, want %q", got, want)
	}
}

func TestDatabaseCredentialEnvVarsUseSecretKeyRefs(t *testing.T) {
	env := DatabaseCredentialEnvVars("shop-preview-app-db-credentials")
	if len(env) != 2 {
		t.Fatalf("got %d credential env vars, want 2", len(env))
	}
	for _, want := range []struct {
		name string
		key  string
	}{
		{platform.EnvMagentoDBUser, "username"},
		{platform.EnvMagentoDBPass, "password"},
	} {
		var found bool
		for _, binding := range env {
			if binding.Name != want.name {
				continue
			}
			found = true
			if binding.Value != "" {
				t.Fatalf("%s contains a literal value", binding.Name)
			}
			if binding.ValueFrom == nil || binding.ValueFrom.SecretKeyRef == nil {
				t.Fatalf("%s is missing a SecretKeyRef", binding.Name)
			}
			if binding.ValueFrom.SecretKeyRef.Name != "shop-preview-app-db-credentials" || binding.ValueFrom.SecretKeyRef.Key != want.key {
				t.Fatalf("%s SecretKeyRef = %#v", binding.Name, binding.ValueFrom.SecretKeyRef)
			}
		}
		if !found {
			t.Fatalf("missing %s", want.name)
		}
	}
}

func TestEncryptionKeyEnvVarsUseSecretKeyRef(t *testing.T) {
	env := EncryptionKeyEnvVars("shop-preview-app-encryption-key")
	if len(env) != 1 {
		t.Fatalf("got %d encryption env vars, want 1", len(env))
	}
	binding := env[0]
	if binding.Name != platform.EnvMagentoCryptKey || binding.Value != "" {
		t.Fatalf("encryption binding = %#v", binding)
	}
	if binding.ValueFrom == nil || binding.ValueFrom.SecretKeyRef == nil {
		t.Fatal("encryption binding is missing a SecretKeyRef")
	}
	ref := binding.ValueFrom.SecretKeyRef
	if ref.Name != "shop-preview-app-encryption-key" || ref.Key != "key" {
		t.Fatalf("encryption SecretKeyRef = %#v", ref)
	}
}

func TestQueuePasswordEnvVarsUseSecretKeyRef(t *testing.T) {
	env := QueuePasswordEnvVars("shop-preview-app-queue-password")
	if len(env) != 1 {
		t.Fatalf("got %d queue password env vars, want 1", len(env))
	}
	binding := env[0]
	if binding.Name != platform.EnvMagentoQueuePassword || binding.Value != "" {
		t.Fatalf("queue password binding = %#v", binding)
	}
	if binding.ValueFrom == nil || binding.ValueFrom.SecretKeyRef == nil {
		t.Fatal("queue password binding is missing a SecretKeyRef")
	}
	ref := binding.ValueFrom.SecretKeyRef
	if ref.Name != "shop-preview-app-queue-password" || ref.Key != "password" {
		t.Fatalf("queue password SecretKeyRef = %#v", ref)
	}
}
