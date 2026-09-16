package sdk

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// CoverageTransitionKind names the only changes that may be exercised from a
// retained cold baseline. A transition is still an independently executed
// unit; the scheduler must never mistake it for proof that the changed
// boundary was covered by the baseline.
type CoverageTransitionKind string

const (
	CoverageTransitionServiceMajor  CoverageTransitionKind = "service-major"
	CoverageTransitionRestore       CoverageTransitionKind = "restore"
	CoverageTransitionObservability CoverageTransitionKind = "observability"
	CoverageTransitionEdge          CoverageTransitionKind = "edge"
	CoverageTransitionNone          CoverageTransitionKind = "none"
	CoverageTransitionUnsupported   CoverageTransitionKind = "unsupported"
)

// CoverageBoundary is the provider-neutral identity of one certification
// boundary. Provider adapters can add opaque implementation details, but the
// fields here are the minimum set the core needs to decide whether a resource
// set may be reused or a transition must be admitted as a separate unit.
type CoverageBoundary struct {
	Provider                  ProviderID        `json:"provider" yaml:"provider"`
	Regions                   []string          `json:"regions" yaml:"regions"`
	ComputeMode               string            `json:"computeMode" yaml:"computeMode"`
	KubernetesTopology        string            `json:"kubernetesTopology" yaml:"kubernetesTopology"`
	NetworkProfile            string            `json:"networkProfile,omitempty" yaml:"networkProfile,omitempty"`
	ManagedSelfHostedBoundary string            `json:"managedSelfHostedBoundary" yaml:"managedSelfHostedBoundary"`
	ServiceMajors             map[string]string `json:"serviceMajors" yaml:"serviceMajors"`
	ResilienceProfileID       string            `json:"resilienceProfileId" yaml:"resilienceProfileId"`
	ObservabilityDestination  string            `json:"observabilityDestination" yaml:"observabilityDestination"`
	EdgePath                  string            `json:"edgePath" yaml:"edgePath"`
	ArtifactDigest            string            `json:"artifactDigest" yaml:"artifactDigest"`
	SchemaFingerprint         string            `json:"schemaFingerprint" yaml:"schemaFingerprint"`
	MigrationFingerprint      string            `json:"migrationFingerprint" yaml:"migrationFingerprint"`
	RecoveryFixtureID         string            `json:"recoveryFixtureId" yaml:"recoveryFixtureId"`
	RecoveryDestination       string            `json:"recoveryDestination" yaml:"recoveryDestination"`
	OwnershipMarker           string            `json:"ownershipMarker" yaml:"ownershipMarker"`
	ConfigurationFingerprint  string            `json:"configurationFingerprint,omitempty" yaml:"configurationFingerprint,omitempty"`
}

var coverageArtifactDigestPattern = regexp.MustCompile(`^[^@\s]+@sha256:[a-f0-9]{64}$`)
var coverageMajorPattern = regexp.MustCompile(`^[0-9][A-Za-z0-9._+-]*$`)
var coveragePathPattern = regexp.MustCompile(`^[a-z][a-z0-9._+:-]*$`)

// Validate checks a complete boundary before it reaches a provider adapter.
// It intentionally rejects an incomplete boundary instead of silently
// treating unknown dimensions as reusable.
func (b CoverageBoundary) Validate() error {
	var problems []error
	problems = append(problems,
		validateID("coverage provider", string(b.Provider)),
		validateID("coverage compute mode", b.ComputeMode),
		validateID("coverage Kubernetes topology", b.KubernetesTopology),
		validateID("coverage managed/self-hosted boundary", b.ManagedSelfHostedBoundary),
		validateID("coverage resilience profile", b.ResilienceProfileID),
		validateID("coverage recovery fixture", b.RecoveryFixtureID),
	)
	if strings.TrimSpace(b.NetworkProfile) != "" {
		problems = append(problems, validateID("coverage network profile", b.NetworkProfile))
	}
	for name, value := range map[string]string{
		"coverage observability destination": b.ObservabilityDestination,
		"coverage edge path":                 b.EdgePath,
		"coverage recovery destination":      b.RecoveryDestination,
	} {
		if !coveragePathPattern.MatchString(value) {
			problems = append(problems, fmt.Errorf("%s %q is invalid", name, value))
		}
	}
	if len(b.Regions) == 0 {
		problems = append(problems, errors.New("coverage boundary requires at least one region"))
	}
	seenRegions := make(map[string]struct{}, len(b.Regions))
	for _, region := range b.Regions {
		problems = append(problems, validateID("coverage region", region))
		if _, exists := seenRegions[region]; exists {
			problems = append(problems, fmt.Errorf("duplicate coverage region %q", region))
		}
		seenRegions[region] = struct{}{}
	}
	switch b.ManagedSelfHostedBoundary {
	case string(ServiceManaged), string(ServiceSelfHosted), "mixed", string(ServiceExisting), string(ServiceExternal):
	default:
		problems = append(problems, fmt.Errorf("invalid managed/self-hosted boundary %q", b.ManagedSelfHostedBoundary))
	}
	if len(b.ServiceMajors) == 0 {
		problems = append(problems, errors.New("coverage boundary requires service majors"))
	}
	for role, major := range b.ServiceMajors {
		problems = append(problems, validateID("coverage service role", role))
		if !coverageMajorPattern.MatchString(major) {
			problems = append(problems, fmt.Errorf("coverage service major %q is invalid", major))
		}
	}
	if !coverageArtifactDigestPattern.MatchString(b.ArtifactDigest) {
		problems = append(problems, errors.New("coverage boundary requires an immutable artifact digest"))
	}
	for name, value := range map[string]string{
		"schema fingerprint":    b.SchemaFingerprint,
		"migration fingerprint": b.MigrationFingerprint,
		"ownership marker":      b.OwnershipMarker,
	} {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
			problems = append(problems, fmt.Errorf("coverage boundary %s is required and must be single-line", name))
		}
	}
	if strings.TrimSpace(b.ConfigurationFingerprint) != "" && strings.ContainsAny(b.ConfigurationFingerprint, "\r\n\x00") {
		problems = append(problems, errors.New("coverage configuration fingerprint must be single-line and NUL-free"))
	}
	return errors.Join(problems...)
}

// Fingerprint returns the canonical identity of every boundary dimension.
// Map and slice ordering cannot change the result.
func (b CoverageBoundary) Fingerprint() (string, error) {
	if err := b.Validate(); err != nil {
		return "", err
	}
	canonical := b
	canonical.Regions = SortedStrings(b.Regions)
	canonical.ServiceMajors = make(map[string]string, len(b.ServiceMajors))
	for role, major := range b.ServiceMajors {
		canonical.ServiceMajors[role] = major
	}
	data, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("encode coverage boundary: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// ValidateCoverageTransition allows only a named, independently evidenced
// transition from a compatible cold boundary. Provider, region, topology,
// ownership, resilience, network, configuration, artifact, schema, migration, and fixture changes
// remain cold-boundary changes and are never inferred from a warm run.
func ValidateCoverageTransition(baseline, next CoverageBoundary, kind CoverageTransitionKind) error {
	if err := baseline.Validate(); err != nil {
		return fmt.Errorf("validate baseline coverage boundary: %w", err)
	}
	if err := next.Validate(); err != nil {
		return fmt.Errorf("validate next coverage boundary: %w", err)
	}
	if baseline.Provider != next.Provider || !sameStrings(baseline.Regions, next.Regions) || baseline.ComputeMode != next.ComputeMode || baseline.KubernetesTopology != next.KubernetesTopology || baseline.NetworkProfile != next.NetworkProfile || baseline.ManagedSelfHostedBoundary != next.ManagedSelfHostedBoundary || baseline.ResilienceProfileID != next.ResilienceProfileID || baseline.ConfigurationFingerprint != next.ConfigurationFingerprint || baseline.ArtifactDigest != next.ArtifactDigest || baseline.SchemaFingerprint != next.SchemaFingerprint || baseline.MigrationFingerprint != next.MigrationFingerprint || baseline.RecoveryFixtureID != next.RecoveryFixtureID || baseline.OwnershipMarker != next.OwnershipMarker {
		return errors.New("coverage transition crosses a cold boundary")
	}
	differences := coverageDifferences(baseline, next)
	if len(differences) == 0 {
		if kind == "" || kind == CoverageTransitionNone {
			return nil
		}
		return fmt.Errorf("coverage transition kind %q has no boundary change", kind)
	}
	switch kind {
	case CoverageTransitionServiceMajor:
		if len(differences) != 1 || differences[0] != "serviceMajors" {
			return fmt.Errorf("service-major transition changes %s", strings.Join(differences, ", "))
		}
	case CoverageTransitionRestore:
		if len(differences) != 1 || differences[0] != "recoveryDestination" {
			return fmt.Errorf("restore transition changes %s; recovery fixture and all other boundary dimensions remain cold", strings.Join(differences, ", "))
		}
	case CoverageTransitionObservability:
		if len(differences) != 1 || differences[0] != "observabilityDestination" {
			return fmt.Errorf("observability transition changes %s", strings.Join(differences, ", "))
		}
	case CoverageTransitionEdge:
		if len(differences) != 1 || differences[0] != "edgePath" {
			return fmt.Errorf("edge transition changes %s", strings.Join(differences, ", "))
		}
	case CoverageTransitionNone, CoverageTransitionUnsupported, "":
		return fmt.Errorf("coverage transition kind %q is not permitted for changes in %s", kind, strings.Join(differences, ", "))
	default:
		return fmt.Errorf("unsupported coverage transition kind %q", kind)
	}
	return nil
}

func CoverageBoundaryFromArchitecture(intent ArchitectureIntent, recoveryFixtureID string) (CoverageBoundary, error) {
	if err := intent.Validate(); err != nil {
		return CoverageBoundary{}, fmt.Errorf("validate architecture intent: %w", err)
	}
	regions := append([]string(nil), intent.Regions...)
	if len(regions) == 0 {
		regions = []string{intent.Region}
	}
	serviceMajors := make(map[string]string, len(intent.Boundaries))
	ownerships := make(map[ServiceOwnership]struct{}, len(intent.Boundaries))
	for _, boundary := range intent.Boundaries {
		serviceMajors[boundary.Role] = boundary.Major
		ownerships[boundary.Ownership] = struct{}{}
	}
	serviceOwnership := "mixed"
	if len(ownerships) == 1 {
		for ownership := range ownerships {
			serviceOwnership = string(ownership)
		}
	}
	observability := "none"
	if activeProvider(intent.Observability.NativeProvider) {
		observability = intent.Observability.NativeProvider
	}
	if activeProvider(intent.Observability.ExternalProvider) {
		if observability == "none" {
			observability = intent.Observability.ExternalProvider
		} else {
			observability += "+" + intent.Observability.ExternalProvider
		}
	}
	edge := intent.Edge.Mode
	if edge == "" {
		edge = "none"
	}
	boundary := CoverageBoundary{
		Provider: intent.Provider, Regions: regions, ComputeMode: intent.ComputeMode,
		KubernetesTopology: valueOrNone(intent.KubernetesMode), NetworkProfile: valueOrNone(intent.NetworkProfile), ManagedSelfHostedBoundary: serviceOwnership,
		ServiceMajors: serviceMajors, ResilienceProfileID: intent.Resilience.ProfileID,
		ObservabilityDestination: observability, EdgePath: edge, ArtifactDigest: intent.ArtifactDigest,
		SchemaFingerprint: intent.SchemaFingerprint, MigrationFingerprint: intent.MigrationFingerprint,
		RecoveryFixtureID: recoveryFixtureID, RecoveryDestination: "none", OwnershipMarker: intent.OwnershipMarker,
		ConfigurationFingerprint: valueOrNone(intent.ConfigurationFingerprint),
	}
	if err := boundary.Validate(); err != nil {
		return CoverageBoundary{}, err
	}
	return boundary, nil
}

func coverageDifferences(a, b CoverageBoundary) []string {
	var result []string
	if !sameStringMap(a.ServiceMajors, b.ServiceMajors) {
		result = append(result, "serviceMajors")
	}
	if a.ObservabilityDestination != b.ObservabilityDestination {
		result = append(result, "observabilityDestination")
	}
	if a.EdgePath != b.EdgePath {
		result = append(result, "edgePath")
	}
	if a.RecoveryDestination != b.RecoveryDestination {
		result = append(result, "recoveryDestination")
	}
	if a.NetworkProfile != b.NetworkProfile {
		result = append(result, "networkProfile")
	}
	if a.ConfigurationFingerprint != b.ConfigurationFingerprint {
		result = append(result, "configurationFingerprint")
	}
	return result
}

func sameStrings(a, b []string) bool {
	return strings.Join(SortedStrings(a), "\x00") == strings.Join(SortedStrings(b), "\x00")
}

func sameStringMap(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if b[key] != value {
			return false
		}
	}
	return true
}

func valueOrNone(value string) string {
	if strings.TrimSpace(value) == "" {
		return "none"
	}
	return value
}
