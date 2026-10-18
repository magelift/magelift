package ops

import (
	"context"
	"errors"
	"strconv"
	"strings"

	gcpbootstrap "github.com/magelift/magelift/providers/gcp/bootstrap"
)

type CIGrantPreparation interface {
	Ensure(context.Context, gcpbootstrap.CIGrantInputs) error
	Close() error
}

func ciGrantInputs(prep gcpbootstrap.PreparationPlan, identities []gcpbootstrap.IdentityPlan, provider string) (gcpbootstrap.CIGrantInputs, error) {
	parts := strings.Split(provider, "/")
	if len(parts) < 4 || parts[0] != "projects" {
		return gcpbootstrap.CIGrantInputs{}, errors.New("federation result is missing its verified GCP project number")
	}
	number, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || number <= 0 || len(identities) != 2 {
		return gcpbootstrap.CIGrantInputs{}, errors.New("federation result has an invalid project number or CI identities")
	}
	expected := "projects/" + parts[1] + "/locations/global/workloadIdentityPools/" + identities[0].PoolID + "/providers/" + identities[0].ProviderID
	if provider != expected {
		return gcpbootstrap.CIGrantInputs{}, errors.New("federation resource does not match the planned CI identity")
	}
	return gcpbootstrap.CIGrantInputs{Preparation: prep, Deploy: identities[0], Build: identities[1], ProjectNumber: parts[1]}, nil
}

func ciVariables(prep gcpbootstrap.PreparationPlan, identities []gcpbootstrap.IdentityPlan, provider string) map[string]string {
	deploy, build := identities[0], identities[1]
	serviceAccountVar := "MAGELIFT_GCP_" + strings.ToUpper(strings.ReplaceAll(deploy.Environment, "-", "_")) + "_SERVICE_ACCOUNT"
	return map[string]string{
		"MAGELIFT_GCP_PROJECT_ID":                 prep.Base.GCPProject,
		"MAGELIFT_GCP_REGION":                     prep.Base.Region,
		"MAGELIFT_GCP_WORKLOAD_IDENTITY_PROVIDER": provider,
		"MAGELIFT_GCP_BUILD_SERVICE_ACCOUNT":      build.ServiceAccountEmail,
		serviceAccountVar:                         deploy.ServiceAccountEmail,
		"MAGELIFT_REGISTRY":                       prep.RegistryLocation + "-docker.pkg.dev",
		"MAGELIFT_RELEASE_IMAGE":                  prep.ImageBase,
		"MAGELIFT_IMAGE_SIGNING_IDENTITY":         "https://github.com/" + deploy.Repository + "/.github/workflows/magelift.yml@refs/heads/main",
		"MAGELIFT_IMAGE_SIGNING_ISSUER":           "https://token.actions.githubusercontent.com",
	}
}
