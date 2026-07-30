package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	buildkit "github.com/acourtiol/magelift/internal/build/kit"
	"github.com/acourtiol/magelift/internal/secretref"
	"github.com/acourtiol/magelift/internal/source"
)

type fakeComposerSecretProvider struct {
	secretValue    []byte
	parameterValue []byte
	err            error
	secretID       string
	parameterID    string
}

func TestBuildPipelineRequestCreatesReleaseRequest(t *testing.T) {
	revision := strings.Repeat("a", 40)
	request, err := buildPipelineRequest(buildRequestOptions{
		Push:           true,
		ImageReference: "ghcr.io/acourtiol/shop:revision",
		BuilderImage:   "ghcr.io/acourtiol/magelift-builder@sha256:" + strings.Repeat("b", 64),
		RuntimeImage:   "ghcr.io/acourtiol/magelift-runtime@sha256:" + strings.Repeat("c", 64),
		Repository: source.Repository{
			Revision:  revision,
			OriginURL: "https://github.com/acourtiol/shop.git",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if request.Output != buildkit.OutputPush || len(request.Platforms) != 2 {
		t.Fatalf("unexpected release request: %#v", request)
	}
	if request.ProvenanceSource != "https://github.com/acourtiol/shop.git?checksum="+revision {
		t.Fatalf("unexpected provenance source: %q", request.ProvenanceSource)
	}
}

func TestBuildPipelineRequestRejectsIncompleteOrUnsafeReleaseOptions(t *testing.T) {
	revision := strings.Repeat("a", 40)
	base := buildRequestOptions{
		Push:           true,
		ImageReference: "ghcr.io/acourtiol/shop:revision",
		BuilderImage:   "ghcr.io/acourtiol/builder@sha256:" + strings.Repeat("b", 64),
		RuntimeImage:   "ghcr.io/acourtiol/runtime@sha256:" + strings.Repeat("c", 64),
		Repository:     source.Repository{Revision: revision, OriginURL: "git@github.com:acourtiol/shop.git"},
	}
	if _, err := buildPipelineRequest(base); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("unexpected source error: %v", err)
	}
	base.SourceURL = "https://github.com/acourtiol/shop.git?checksum=" + strings.Repeat("d", 40)
	if _, err := buildPipelineRequest(base); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("unexpected checksum error: %v", err)
	}
	base.SourceURL = "https://github.com/acourtiol/shop.git?access_token=secret"
	if _, err := buildPipelineRequest(base); err == nil || !strings.Contains(err.Error(), "only contain") {
		t.Fatalf("unexpected query credential error: %v", err)
	}
	base.SourceURL = "https://github.com/acourtiol/shop.git"
	base.RuntimeImage = ""
	if _, err := buildPipelineRequest(base); err == nil || !strings.Contains(err.Error(), "--runtime-image") {
		t.Fatalf("unexpected required flag error: %v", err)
	}
}

func TestBuildPipelineRequestRejectsReleaseFlagsWithoutPush(t *testing.T) {
	_, err := buildPipelineRequest(buildRequestOptions{
		ImageReference: "ghcr.io/acourtiol/shop:revision",
		Repository:     source.Repository{Revision: strings.Repeat("a", 40)},
	})
	if err == nil || !strings.Contains(err.Error(), "require --push") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDigestReferenceReplacesMutableTag(t *testing.T) {
	digest := "sha256:" + strings.Repeat("d", 64)
	reference, err := digestReference("registry.example.invalid:5000/team/shop:candidate", digest)
	if err != nil {
		t.Fatal(err)
	}
	want := "registry.example.invalid:5000/team/shop@" + digest
	if reference != want {
		t.Fatalf("reference=%q want=%q", reference, want)
	}
}

func TestDigestReferenceRejectsInvalidBuildMetadata(t *testing.T) {
	if _, err := digestReference("shop:candidate", "sha256:"+strings.Repeat("d", 64)); err == nil {
		t.Fatal("unqualified reference was accepted")
	}
	if _, err := digestReference("registry.example.invalid/team/shop:candidate", "sha256:short"); err == nil {
		t.Fatal("invalid digest was accepted")
	}
}

func (provider *fakeComposerSecretProvider) GetSecretValue(_ context.Context, id string) ([]byte, error) {
	provider.secretID = id
	return provider.secretValue, provider.err
}

func (provider *fakeComposerSecretProvider) GetParameter(_ context.Context, id string) ([]byte, error) {
	provider.parameterID = id
	return provider.parameterValue, provider.err
}

func TestResolveComposerCredentialsFromSupportedProviders(t *testing.T) {
	for _, test := range []struct {
		name      string
		reference secretref.Reference
		provider  *fakeComposerSecretProvider
		wantID    string
	}{
		{
			name:      "Secrets Manager",
			reference: secretref.Reference{Kind: secretref.SecretsManager, ID: "shop/composer"},
			provider:  &fakeComposerSecretProvider{secretValue: []byte(`{"http-basic":{"repo.magento.com":{"username":"public","password":"private"}}}`)},
			wantID:    "shop/composer",
		},
		{
			name:      "SSM Parameter Store",
			reference: secretref.Reference{Kind: secretref.ParameterStore, ID: "/shop/composer"},
			provider:  &fakeComposerSecretProvider{parameterValue: []byte(`{"bearer":{"example.invalid":"token"}}`)},
			wantID:    "/shop/composer",
		},
		{
			name:      "GCP Secret Manager",
			reference: secretref.Reference{Kind: secretref.GCPSecretManager, ID: "projects/p/secrets/composer/versions/latest"},
			provider:  &fakeComposerSecretProvider{secretValue: []byte(`{"http-basic":{"repo.magento.com":{"username":"public","password":"private"}}}`)},
			wantID:    "projects/p/secrets/composer/versions/latest",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			value, err := resolveComposerCredentials(context.Background(), test.reference, test.provider)
			if err != nil {
				t.Fatal(err)
			}
			if len(value) == 0 || test.provider.secretID != test.wantID && test.provider.parameterID != test.wantID {
				t.Fatalf("value length=%d secret=%q parameter=%q", len(value), test.provider.secretID, test.provider.parameterID)
			}
		})
	}
}

func TestLoadComposerCredentialsGCPUsesProvider(t *testing.T) {
	provider := &fakeComposerSecretProvider{secretValue: []byte(`{"http-basic":{"repo.magento.com":{"username":"public","password":"private"}}}`)}
	o := &options{
		newComposerGCPSecrets: func(context.Context) (composerSecretProvider, error) {
			return provider, nil
		},
	}
	value, err := o.loadComposerCredentials(context.Background(), "gcp-secret-manager://projects/p/secrets/composer/versions/latest", "europe-west1")
	if err != nil {
		t.Fatal(err)
	}
	if len(value) == 0 || provider.secretID != "projects/p/secrets/composer/versions/latest" {
		t.Fatalf("value length=%d secret=%q", len(value), provider.secretID)
	}
	if strings.Contains(strings.ToLower(errString(err)), "not implemented") {
		t.Fatal("GCP path still reports not implemented")
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestLoadComposerCredentialsGCPPropagatesProviderErrors(t *testing.T) {
	o := &options{
		newComposerGCPSecrets: func(context.Context) (composerSecretProvider, error) {
			return &fakeComposerSecretProvider{err: errors.New("missing secret")}, nil
		},
	}
	_, err := o.loadComposerCredentials(context.Background(), "gcp-secret-manager://projects/p/secrets/missing/versions/latest", "europe-west1")
	if err == nil || !strings.Contains(err.Error(), "secret provider failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(err.Error(), "not implemented") || strings.Contains(err.Error(), "missing secret") {
		t.Fatalf("error must stay loud without leaking provider detail: %v", err)
	}
}

func TestResolveComposerCredentialsRejectsInvalidOrLeakingValues(t *testing.T) {
	secret := "do-not-log-this"
	for _, provider := range []*fakeComposerSecretProvider{
		{secretValue: []byte(secret)},
		{err: errors.New("provider leaked " + secret)},
	} {
		_, err := resolveComposerCredentials(
			context.Background(),
			secretref.Reference{Kind: secretref.SecretsManager, ID: "composer"},
			provider,
		)
		if err == nil || strings.Contains(err.Error(), secret) {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

func TestLoadComposerCredentialsSkipsAWSWhenUnset(t *testing.T) {
	value, err := (&options{}).loadComposerCredentials(context.Background(), "", "eu-west-3")
	if err != nil || value != nil {
		t.Fatalf("value=%v error=%v", value, err)
	}
}

func TestSecretRegionUsesARNAndValidatesService(t *testing.T) {
	region, err := secretRegion(secretref.Reference{
		Kind: secretref.SecretsManager,
		ID:   "arn:aws:secretsmanager:us-east-2:123456789012:secret:composer-AbCd",
	}, "eu-west-3")
	if err != nil || region != "us-east-2" {
		t.Fatalf("region=%q error=%v", region, err)
	}
	if _, err := secretRegion(secretref.Reference{
		Kind: secretref.ParameterStore,
		ID:   "arn:aws:secretsmanager:us-east-2:123456789012:secret:composer-AbCd",
	}, "eu-west-3"); err == nil || !strings.Contains(err.Error(), "ssm") {
		t.Fatalf("unexpected service error: %v", err)
	}
	region, err = secretRegion(secretref.Reference{Kind: secretref.ParameterStore, ID: "/shop/composer"}, "eu-west-3")
	if err != nil || region != "eu-west-3" {
		t.Fatalf("default region=%q error=%v", region, err)
	}
}
