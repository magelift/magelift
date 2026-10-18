package config

import "testing"

func TestComposerEnvironmentReferenceIsBuildOnly(t *testing.T) {
	for _, provider := range []string{"gcp", "aws"} {
		if got := validateComposerCredentialsReference("env://COMPOSER_AUTH", provider); len(got) != 0 {
			t.Fatal(got)
		}
		for _, ref := range []string{"env://", "env://COMPOSER_AUTH?field=x", "env://COMPOSER_AUTH/extra", "{\"token\":\"literal\"}"} {
			if got := validateComposerCredentialsReference(ref, provider); len(got) == 0 {
				t.Fatalf("accepted %s", ref)
			}
		}
		if got := validateProviderSecretReference("target.secret", "env://COMPOSER_AUTH", provider); len(got) == 0 {
			t.Fatal("environment scheme escaped into runtime secret contract")
		}
	}
}
