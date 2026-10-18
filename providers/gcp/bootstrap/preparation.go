// Package bootstrap account preparation (APIs, Artifact Registry, state passphrase).
package bootstrap

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

const (
	purposeApplicationImages     = "application-images"
	purposePulumiStatePassphrase = "pulumi-state-passphrase"
	maxArtifactRepositoryIDLen   = 63
	maxOwnedResourceNameLen      = 63
)

var preparationOwnershipLabelKeys = []string{
	"magelift-managed-by",
	"magelift-project",
	"magelift-environment",
	"magelift-purpose",
}

const preparationOwnershipRefusalHint = "select a fresh MageLift project/environment name, or have the resource owner review recorded ownership before adoption"

// PreparationPlan is the deterministic account-prep recipe derived from bootstrap.Spec.
type PreparationPlan struct {
	Base              Plan              `json:"base" yaml:"base"`
	RegistryID        string            `json:"registryId" yaml:"registryId"`
	RegistryLocation  string            `json:"registryLocation" yaml:"registryLocation"`
	RegistryLabels    map[string]string `json:"registryLabels" yaml:"registryLabels"`
	ImageBase         string            `json:"imageBase" yaml:"imageBase"`
	StateSecretID     string            `json:"stateSecretId" yaml:"stateSecretId"`
	StateSecretLabels map[string]string `json:"stateSecretLabels" yaml:"stateSecretLabels"`
}

// RepositoryResult records an owned Docker Artifact Registry repository.
type RepositoryResult struct {
	RepositoryID string `json:"repositoryId" yaml:"repositoryId"`
	Location     string `json:"location" yaml:"location"`
	RegistryHost string `json:"registryHost" yaml:"registryHost"`
	ImageBase    string `json:"imageBase" yaml:"imageBase"`
}

// StatePassphraseRef is a Secret Manager version reference without secret material.
type StatePassphraseRef struct {
	SecretID    string `json:"secretId" yaml:"secretId"`
	VersionName string `json:"versionName" yaml:"versionName"`
}

// AccountPreparer ensures reference account-prep resources. Methods are separated so
// the parent can orchestrate APIs, bucket, passphrase, and identities independently.
type AccountPreparer struct {
	apis        ServiceEnablementAPI
	registry    ArtifactRegistryAPI
	secrets     StateSecretAPI
	checkpoints StateCheckpointAPI
	locks       StateLockAPI
}

// NewAccountPreparer wires injectable clients for tests and custom endpoints.
func NewAccountPreparer(apis ServiceEnablementAPI, registry ArtifactRegistryAPI, secrets StateSecretAPI, checkpoints StateCheckpointAPI, locks StateLockAPI) (*AccountPreparer, error) {
	if apis == nil || registry == nil || secrets == nil || checkpoints == nil || locks == nil {
		return nil, errors.New("account preparer requires APIs, registry, secrets, checkpoint, and lock clients")
	}
	return &AccountPreparer{
		apis:        apis,
		registry:    registry,
		secrets:     secrets,
		checkpoints: checkpoints,
		locks:       locks,
	}, nil
}

// BuildPreparationPlan derives registry, image base, and state passphrase metadata from Spec.
func BuildPreparationPlan(spec Spec) (PreparationPlan, error) {
	base, err := BuildPlan(spec)
	if err != nil {
		return PreparationPlan{}, err
	}
	envSegment := stateBucketEnvironment(spec.Environment)
	registrySeed := strings.Join([]string{
		"magelift", spec.GCPProject, spec.Region, spec.Project, envSegment, "images",
	}, "-")
	registryID := compactOwnedName(registrySeed, envSegment+"-images")
	secretSeed := strings.Join([]string{
		"magelift", spec.Project, envSegment, "pulumi", "state", "passphrase",
	}, "-")
	secretID := compactOwnedName(secretSeed, envSegment+"-pulumi-state-passphrase")
	registryLabels := preparationLabels(spec, purposeApplicationImages)
	stateLabels := preparationLabels(spec, purposePulumiStatePassphrase)
	registryHost := spec.Region + "-docker.pkg.dev"
	imageBase := registryHost + "/" + spec.GCPProject + "/" + registryID + "/" + spec.Project
	return PreparationPlan{
		Base:              base,
		RegistryID:        registryID,
		RegistryLocation:  spec.Region,
		RegistryLabels:    registryLabels,
		ImageBase:         imageBase,
		StateSecretID:     secretID,
		StateSecretLabels: stateLabels,
	}, nil
}

// validatePreparationPlanScope rejects plan/spec drift before locks or mutations run.
func validatePreparationPlanScope(plan PreparationPlan, spec Spec) error {
	expected, err := BuildPreparationPlan(spec)
	if err != nil {
		return err
	}
	if !preparationPlansEqual(plan, expected) {
		return fmt.Errorf(
			"preparation plan does not match spec for project %q environment %q: rebuild the plan from the same spec before account preparation",
			spec.Project, spec.Environment,
		)
	}
	return nil
}

func preparationPlansEqual(a, b PreparationPlan) bool {
	if a.RegistryID != b.RegistryID ||
		a.RegistryLocation != b.RegistryLocation ||
		a.ImageBase != b.ImageBase ||
		a.StateSecretID != b.StateSecretID {
		return false
	}
	if !reflect.DeepEqual(a.RegistryLabels, b.RegistryLabels) ||
		!reflect.DeepEqual(a.StateSecretLabels, b.StateSecretLabels) {
		return false
	}
	return preparationBasePlansEqual(a.Base, b.Base)
}

func preparationBasePlansEqual(a, b Plan) bool {
	if a.StateBucket != b.StateBucket || a.GCPProject != b.GCPProject || a.Region != b.Region {
		return false
	}
	return reflect.DeepEqual(a.Labels, b.Labels)
}

func preparationLabels(spec Spec, purpose string) map[string]string {
	return map[string]string{
		"magelift-managed-by":  "magelift",
		"magelift-project":     spec.Project,
		"magelift-environment": spec.Environment,
		"magelift-purpose":     purpose,
	}
}

func compactOwnedName(seed, tail string) string {
	if len(seed) <= maxOwnedResourceNameLen {
		return seed
	}
	digest := sha256.Sum256([]byte(seed))
	suffix := hex.EncodeToString(digest[:])[:10]
	prefixLength := maxOwnedResourceNameLen - len(suffix) - len(tail) - 2
	if prefixLength < 1 {
		return strings.TrimRight(seed[:maxOwnedResourceNameLen-len(suffix)-1], "-") + "-" + suffix
	}
	prefix := strings.TrimRight(seed[:prefixLength], "-")
	return prefix + "-" + suffix + "-" + tail
}

func validatePreparationOwnership(resource string, want, got map[string]string) error {
	for _, key := range preparationOwnershipLabelKeys {
		wantValue := strings.TrimSpace(want[key])
		gotValue := strings.TrimSpace(got[key])
		if gotValue == "" {
			return fmt.Errorf(
				"refusing to mutate %s: missing ownership label %q; %s",
				resource, key, preparationOwnershipRefusalHint,
			)
		}
		if gotValue != wantValue {
			return fmt.Errorf(
				"refusing to mutate %s: label %q is %q, want %q; %s",
				resource, key, gotValue, wantValue, preparationOwnershipRefusalHint,
			)
		}
	}
	return nil
}

func preparationSecretVersionName(project, secretID string) string {
	return fmt.Sprintf("projects/%s/secrets/%s/versions/1", project, secretID)
}

func preparationSecretResourceName(project, secretID string) string {
	return fmt.Sprintf("projects/%s/secrets/%s", project, secretID)
}

// Close releases native clients when supported.
func (p *AccountPreparer) Close() error {
	if p == nil {
		return nil
	}
	var first error
	if p.apis != nil {
		if err := p.apis.Close(); err != nil && first == nil {
			first = err
		}
	}
	if p.registry != nil {
		if err := p.registry.Close(); err != nil && first == nil {
			first = err
		}
	}
	if p.secrets != nil {
		if err := p.secrets.Close(); err != nil && first == nil {
			first = err
		}
	}
	if p.checkpoints != nil {
		if err := p.checkpoints.Close(); err != nil && first == nil {
			first = err
		}
	}
	if p.locks != nil {
		if err := p.locks.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}
