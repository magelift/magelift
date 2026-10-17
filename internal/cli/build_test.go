package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	buildkit "github.com/magelift/magelift/internal/build/kit"
	buildpipeline "github.com/magelift/magelift/internal/build/pipeline"
	"github.com/magelift/magelift/internal/secretref"
	"github.com/magelift/magelift/internal/source"
	"github.com/magelift/magelift/internal/toolchain"
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
		ImageReference: "ghcr.io/magelift/shop:revision",
		BuilderImage:   "registry.example.invalid/team/shop-builder@sha256:" + strings.Repeat("b", 64),
		RuntimeImage:   "registry.example.invalid/team/shop-runtime@sha256:" + strings.Repeat("c", 64),
		Repository: source.Repository{
			Revision:  revision,
			OriginURL: "https://github.com/acourtiol/shop.git",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if request.Output != buildkit.OutputPush || len(request.Platforms) != 1 || request.Platforms[0] != "linux/amd64" {
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
		ImageReference: "ghcr.io/magelift/shop:revision",
		BuilderImage:   "ghcr.io/magelift/builder@sha256:" + strings.Repeat("b", 64),
		RuntimeImage:   "ghcr.io/magelift/runtime@sha256:" + strings.Repeat("c", 64),
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
	if _, err := buildPipelineRequest(base); err != nil {
		t.Fatalf("push request without base images should be accepted: %v", err)
	}
}

func TestBuildPipelineRequestRejectsReleaseFlagsWithoutPush(t *testing.T) {
	_, err := buildPipelineRequest(buildRequestOptions{
		ImageReference: "ghcr.io/magelift/shop:revision",
		Repository:     source.Repository{Revision: strings.Repeat("a", 40)},
	})
	if err == nil || !strings.Contains(err.Error(), "require --push") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildPipelineRequestDefaultsLocalPlatformToLinuxAMD64(t *testing.T) {
	request, err := buildPipelineRequest(buildRequestOptions{
		Repository:  source.Repository{Revision: strings.Repeat("a", 40)},
		ProjectName: "boutique",
	})
	if err != nil {
		t.Fatal(err)
	}
	if request.Platform != "linux/amd64" {
		t.Fatalf("platform=%q", request.Platform)
	}
	if request.ImageReference != "magelift/boutique:"+strings.Repeat("a", 12) {
		t.Fatalf("image reference=%q", request.ImageReference)
	}
}

func TestBuildPipelineRequestAllowsAdvancedBaseImagesWithoutPush(t *testing.T) {
	builder := "registry.example.invalid/team/shop-builder@sha256:" + strings.Repeat("b", 64)
	runtime := "registry.example.invalid/team/shop-runtime@sha256:" + strings.Repeat("c", 64)
	request, err := buildPipelineRequest(buildRequestOptions{
		BuilderImage: builder,
		RuntimeImage: runtime,
		Platforms:    []string{"linux/amd64"},
		Repository:   source.Repository{Revision: strings.Repeat("a", 40)},
		ProjectName:  "boutique",
	})
	if err != nil {
		t.Fatal(err)
	}
	if request.BuilderImage != builder || request.RuntimeImage != runtime {
		t.Fatalf("unexpected images: %#v", request)
	}
}

func TestBuildPipelineRequestPushDoesNotRequireExplicitBaseImages(t *testing.T) {
	revision := strings.Repeat("a", 40)
	request, err := buildPipelineRequest(buildRequestOptions{
		Push:           true,
		ImageReference: "ghcr.io/magelift/shop:revision",
		Repository: source.Repository{
			Revision:  revision,
			OriginURL: "https://github.com/acourtiol/shop.git",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if request.BuilderImage != "" || request.RuntimeImage != "" {
		t.Fatalf("base images should be resolved later: %#v", request)
	}
}

func TestApplyResolvedImagesUsesUserRegistryReferences(t *testing.T) {
	builderRef := "registry.example.invalid/team/shop-builder@sha256:" + strings.Repeat("a", 64)
	runtimeRef := "registry.example.invalid/team/shop-runtime@sha256:" + strings.Repeat("b", 64)
	request := buildpipeline.Request{}
	applyResolvedImages(&request, toolchain.Images{
		BuilderReference: builderRef,
		BuilderID:        "sha256:" + strings.Repeat("a", 64),
		RuntimeReference: runtimeRef,
		RuntimeID:        "sha256:" + strings.Repeat("b", 64),
	})
	if request.BuilderImage != builderRef || request.RuntimeImage != runtimeRef || request.RuntimeImageID != "" {
		t.Fatalf("unexpected request images: %#v", request)
	}
}

func TestApplyResolvedImagesUsesBuilderIDForLocalTags(t *testing.T) {
	request := buildpipeline.Request{}
	builderID := "sha256:" + strings.Repeat("a", 64)
	runtimeRef := "magelift/php-nginx@sha256:" + strings.Repeat("b", 64)
	applyResolvedImages(&request, toolchain.Images{
		BuilderID:        builderID,
		RuntimeReference: runtimeRef,
		RuntimeID:        "sha256:" + strings.Repeat("b", 64),
	})
	if request.BuilderImage != builderID || request.RuntimeImage != runtimeRef {
		t.Fatalf("unexpected resolved images: %#v", request)
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

func TestNormalizePublicVersion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "", want: "0.0.0-dev"},
		{input: "dev", want: "0.0.0-dev"},
		{input: "DEV", want: "0.0.0-dev"},
		{input: "v1.2.3", want: "1.2.3"},
		{input: "0.1.0-alpha.1", want: "0.1.0-alpha.1"},
	}
	for _, test := range tests {
		got, err := normalizePublicVersion(test.input)
		if err != nil || got != test.want {
			t.Fatalf("normalizePublicVersion(%q) = (%q, %v), want %q", test.input, got, err, test.want)
		}
	}
	if _, err := normalizePublicVersion("main"); err == nil || !strings.Contains(err.Error(), "semantic version") {
		t.Fatalf("unexpected invalid version error: %v", err)
	}
}

func TestApplyResolvedLocalOverridesKeepRuntimeDriftGuard(t *testing.T) {
	request := buildpipeline.Request{BuilderImage: "magelift/php-builder:qualification", RuntimeImage: "magelift/php-nginx:qualification"}
	images := toolchain.Images{BuilderID: "sha256:" + strings.Repeat("a", 64), RuntimeID: "sha256:" + strings.Repeat("b", 64)}
	applyResolvedImages(&request, images)
	if request.BuilderImage != images.BuilderID || request.RuntimeImage != "magelift/php-nginx:qualification" || request.RuntimeImageID != images.RuntimeID {
		t.Fatalf("request = %+v", request)
	}
	changed := images
	changed.RuntimeID = "sha256:" + strings.Repeat("c", 64)
	if sameResolvedRuntime(images, changed) {
		t.Fatal("runtime drift was accepted")
	}
}

func TestDefaultImageRecipeRejectsWrongPHPOrRuntimeBeforePull(t *testing.T) {
	for _, tc := range []struct {
		php, runtime, builder, image string
		allowed                      bool
	}{
		{"8.5", "nginx-fpm", "", "", true},
		{"8.4", "nginx-fpm", "", "", false},
		{"8.5", "frankenphp-classic", "", "", false},
		{"8.4", "nginx-fpm", "builder-override", "runtime-override", true},
	} {
		if err := validateDefaultImageRecipe(tc.php, tc.runtime, tc.builder, tc.image); (err == nil) != tc.allowed {
			t.Fatalf("%+v: %v", tc, err)
		}
	}
}

func TestExplicitARMOverrideRemainsOutsideDefaultQualification(t *testing.T) {
	options := buildRequestOptions{Repository: source.Repository{Revision: strings.Repeat("a", 40)}, ProjectName: "shop", Platforms: []string{"linux/arm64"}}
	if _, err := buildPipelineRequest(options); err == nil || !strings.Contains(err.Error(), "explicit") {
		t.Fatalf("unqualified defaults accepted: %v", err)
	}
	options.BuilderImage = "ghcr.io/example/builder@sha256:" + strings.Repeat("b", 64)
	options.RuntimeImage = "ghcr.io/example/runtime@sha256:" + strings.Repeat("c", 64)
	request, err := buildPipelineRequest(options)
	if err != nil || request.Platform != "linux/arm64" {
		t.Fatalf("explicit advanced override lost: %+v, %v", request, err)
	}
}

func TestApplicationImageRepositoryKeepsUserRegistry(t *testing.T) {
	for _, tc := range []struct{ image, repository string }{
		{"ghcr.io/team/shop:candidate", "ghcr.io/team/shop"},
		{"123456789012.dkr.ecr.us-east-1.amazonaws.com/shop:alpha", "123456789012.dkr.ecr.us-east-1.amazonaws.com/shop"},
		{"us-central1-docker.pkg.dev/project/repository/shop:alpha", "us-central1-docker.pkg.dev/project/repository/shop"},
		{"registry.example.invalid:5000/team/shop:v1", "registry.example.invalid:5000/team/shop"},
	} {
		got, err := applicationImageRepository(tc.image)
		if err != nil || got != tc.repository {
			t.Fatalf("%q: %q %v", tc.image, got, err)
		}
	}
	for _, image := range []string{"shop:alpha", "team/shop:alpha", "ghcr.io/team/shop", "ghcr.io/team/shop@sha256:" + strings.Repeat("a", 64), "https://ghcr.io/team/shop:x", "ghcr.io/team/../shop:x", "ghcr.io/team/shop:x\nother", "ghcr.io/team/shop:"} {
		if _, err := applicationImageRepository(image); err == nil {
			t.Fatalf("accepted %q", image)
		}
	}
}

func TestApplyResolvedImagesPreservesGeneratedLocalRuntimeTag(t *testing.T) {
	images := toolchain.Images{BuilderID: "sha256:" + strings.Repeat("a", 64), RuntimeID: "sha256:" + strings.Repeat("b", 64), RuntimeReference: "magelift/php-nginx:recipe-unique"}
	request := buildpipeline.Request{}
	applyResolvedImages(&request, images)
	if request.RuntimeImage != images.RuntimeReference || request.RuntimeImageID != images.RuntimeID {
		t.Fatalf("lost generated runtime: %+v", request)
	}
	after := images
	after.RuntimeReference = "magelift/php-nginx@" + images.RuntimeID
	if !sameResolvedRuntime(images, after) {
		t.Fatal("unchanged local runtime rejected")
	}
	after.RuntimeID = "sha256:" + strings.Repeat("c", 64)
	if sameResolvedRuntime(images, after) {
		t.Fatal("changed local runtime accepted")
	}
}
