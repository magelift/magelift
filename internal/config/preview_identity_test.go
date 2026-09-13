package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildPreviewIdentityIsStableAcrossBranchRenames(t *testing.T) {
	first, err := BuildPreviewIdentity(PreviewIdentityInput{
		Project: "shop", Repository: "Acme/Magento", PullRequest: 41,
		Branch: "feature/cart", Commit: strings.Repeat("a", 40), Generation: 1,
		Domain: "pr-41.example.com", ExpiresAt: "2026-08-15T12:00:00+02:00",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildPreviewIdentity(PreviewIdentityInput{
		Project: "shop", Repository: "acme/magento", PullRequest: 41,
		Branch: "feature/cart-v2", Commit: strings.Repeat("b", 40), Generation: 2,
		Domain: "pr-41.example.com", ExpiresAt: "2026-08-15T10:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Repository != "acme/magento" || first.Environment != second.Environment || first.StackKey != second.StackKey || first.OwnershipMarker != second.OwnershipMarker {
		t.Fatalf("identity changed across branch rename: first=%#v second=%#v", first, second)
	}
	if second.Branch != "feature/cart-v2" || second.CommitDigest != strings.Repeat("b", 40) || second.Generation != 2 || second.ExpiresAt != "2026-08-15T10:00:00Z" {
		t.Fatalf("diagnostic metadata was not updated: %#v", second)
	}
}

func TestBuildPreviewIdentitySeparatesPullRequestsAndRepositories(t *testing.T) {
	first, err := BuildPreviewIdentity(PreviewIdentityInput{Project: "shop", Repository: "acme/magento", PullRequest: 41})
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildPreviewIdentity(PreviewIdentityInput{Project: "shop", Repository: "acme/magento", PullRequest: 42})
	if err != nil {
		t.Fatal(err)
	}
	third, err := BuildPreviewIdentity(PreviewIdentityInput{Project: "shop", Repository: "other/magento", PullRequest: 41})
	if err != nil {
		t.Fatal(err)
	}
	if first.Environment == second.Environment || first.StackKey == second.StackKey || first.OwnershipMarker == second.OwnershipMarker {
		t.Fatalf("pull requests collided: %#v %#v", first, second)
	}
	if first.Environment == third.Environment || first.StackKey == third.StackKey || first.OwnershipMarker == third.OwnershipMarker {
		t.Fatalf("repositories collided: %#v %#v", first, third)
	}
}

func TestBuildPreviewIdentityCanonicalizesRepositoryFormsDeterministically(t *testing.T) {
	inputs := []string{"Acme/Magento", "https://github.com/acme/magento.git", "git@github.com:acme/magento.git"}
	var first PreviewIdentity
	for index, repository := range inputs {
		identity, err := BuildPreviewIdentity(PreviewIdentityInput{Project: "shop", Repository: repository, PullRequest: 41})
		if err != nil {
			t.Fatalf("repository %q: %v", repository, err)
		}
		if index == 0 {
			first = identity
			continue
		}
		if identity != first {
			t.Fatalf("repository form %q changed identity: first=%#v got=%#v", repository, first, identity)
		}
	}
}

func TestBuildPreviewIdentityRejectsLengthAndTampering(t *testing.T) {
	baseInput := PreviewIdentityInput{Project: "shop", Repository: "acme/magento", PullRequest: 41}
	tooLongBranch := baseInput
	tooLongBranch.Branch = strings.Repeat("b", 256)
	tooLongDomain := baseInput
	tooLongDomain.Domain = strings.Repeat("d", 254)
	for _, test := range []struct {
		name  string
		input PreviewIdentityInput
	}{
		{name: "branch length", input: tooLongBranch},
		{name: "domain length", input: tooLongDomain},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := BuildPreviewIdentity(test.input); err == nil {
				t.Fatal("overlong metadata was accepted")
			}
		})
	}

	identity, err := BuildPreviewIdentity(baseInput)
	if err != nil {
		t.Fatal(err)
	}
	identity.Environment = "pr-foreign"
	if err := identity.Validate(); err == nil {
		t.Fatal("tampered environment was accepted")
	}
}

func TestPreviewIdentityMetadataDoesNotExposeCredentialFields(t *testing.T) {
	identity, err := BuildPreviewIdentity(PreviewIdentityInput{
		Project: "shop", Repository: "acme/magento", PullRequest: 41,
		Branch: "feature/checkout", Commit: strings.Repeat("a", 40),
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(identity)
	if err != nil {
		t.Fatal(err)
	}
	serialized := strings.ToLower(string(data))
	for _, forbidden := range []string{"password", "secret", "token", "privatekey", "accesskey", "credential"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("preview identity exposes credential field %q: %s", forbidden, serialized)
		}
	}
}

func TestBuildPreviewIdentityRejectsUnsafeOrInvalidMetadata(t *testing.T) {
	tests := []struct {
		name  string
		input PreviewIdentityInput
	}{
		{name: "missing pull request", input: PreviewIdentityInput{Project: "shop", Repository: "acme/magento"}},
		{name: "invalid repository", input: PreviewIdentityInput{Project: "shop", Repository: "https://example.com/acme/magento", PullRequest: 1}},
		{name: "control branch", input: PreviewIdentityInput{Project: "shop", Repository: "acme/magento", PullRequest: 1, Branch: "feature\nset"}},
		{name: "invalid commit", input: PreviewIdentityInput{Project: "shop", Repository: "acme/magento", PullRequest: 1, Commit: "not-a-digest"}},
		{name: "invalid expiration", input: PreviewIdentityInput{Project: "shop", Repository: "acme/magento", PullRequest: 1, ExpiresAt: "tomorrow"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := BuildPreviewIdentity(test.input); err == nil {
				t.Fatal("unsafe or invalid preview metadata was accepted")
			}
		})
	}
}

func TestResolvePreviewIdentityContributesToFingerprint(t *testing.T) {
	file, err := Load([]byte(base))
	if err != nil {
		t.Fatal(err)
	}
	identity, err := BuildPreviewIdentity(PreviewIdentityInput{Project: "shop", Repository: "acme/magento", PullRequest: 41})
	if err != nil {
		t.Fatal(err)
	}
	without, err := file.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	with, err := file.Resolve("staging", ResolveOptions{PreviewIdentity: &identity})
	if err != nil {
		t.Fatal(err)
	}
	if without.Config.PreviewIdentity != nil {
		t.Fatal("fixed environment unexpectedly received preview identity")
	}
	if with.Config.PreviewIdentity == nil || with.Config.PreviewIdentity.Environment != identity.Environment {
		t.Fatalf("resolved preview identity = %#v", with.Config.PreviewIdentity)
	}
	if without.Fingerprint == with.Fingerprint {
		t.Fatal("preview identity did not change resolved fingerprint")
	}
	if got := with.Provenance["previewIdentity"].Source; got != "derived preview identity" {
		t.Fatalf("preview identity provenance = %q", got)
	}
}
