package plugin

import (
	"context"
	"errors"
	"strings"
	"testing"

	gcpbootstrap "github.com/magelift/magelift/providers/gcp/bootstrap"
)

type fakeAccountPreparation struct {
	t         *testing.T
	buckets   *memBuckets
	apisReady bool
	keyReady  bool
}

func (p *fakeAccountPreparation) EnsureAPIs(context.Context, gcpbootstrap.PreparationPlan, gcpbootstrap.Spec) error {
	p.apisReady = true
	return nil
}
func (p *fakeAccountPreparation) EnsureStatePassphrase(ctx context.Context, plan gcpbootstrap.PreparationPlan, spec gcpbootstrap.Spec, owner string) (gcpbootstrap.StatePassphraseRef, error) {
	exists, _, err := p.buckets.BucketExists(ctx, plan.Base.StateBucket)
	if err != nil {
		return gcpbootstrap.StatePassphraseRef{}, err
	}
	if !exists || !p.apisReady || !strings.HasPrefix(owner, "bootstrap-") {
		return gcpbootstrap.StatePassphraseRef{}, errors.New("state initialization preceded prerequisites")
	}
	p.keyReady = true
	return gcpbootstrap.StatePassphraseRef{SecretID: plan.StateSecretID, VersionName: "projects/" + spec.GCPProject + "/secrets/" + plan.StateSecretID + "/versions/1"}, nil
}
func (p *fakeAccountPreparation) EnsureRepository(_ context.Context, plan gcpbootstrap.PreparationPlan, _ gcpbootstrap.Spec) (gcpbootstrap.RepositoryResult, error) {
	if !p.keyReady {
		p.t.Fatal("registry preparation preceded state initialization")
	}
	return gcpbootstrap.RepositoryResult{RepositoryID: plan.RegistryID, Location: plan.RegistryLocation, ImageBase: plan.ImageBase, RegistryHost: plan.RegistryLocation + "-docker.pkg.dev"}, nil
}
func (p *fakeAccountPreparation) Close() error { return nil }
