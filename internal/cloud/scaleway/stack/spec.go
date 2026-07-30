package stack

import (
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strings"
	"time"

	sdk "github.com/acourtiol/magelift/sdk/v1"
)

var (
	stableName     = regexp.MustCompile(`^[a-z][a-z0-9-]{0,30}$`)
	digest         = regexp.MustCompile(`^[^@\s]+@sha256:[a-f0-9]{64}$`)
	scalewayRegion = regexp.MustCompile(`^[a-z]{2}-[a-z]{3}$`)
	scalewayZone   = regexp.MustCompile(`^[a-z]{2}-[a-z]{3}-[1-9]$`)
)

type Spec struct {
	Identity     Identity
	Application  Application
	Artifact     Artifact
	Lifecycle    Lifecycle
	Policy       NetworkPolicy
	Catalog      CatalogSelection
	Dependencies Dependencies
}

type Identity struct {
	Project          string // Magento project name
	ScalewayProject  string
	Environment      string
	Region           string
	Zone             string
	EnvironmentClass string
	Preset           sdk.PresetID
	Labels           map[string]string
}

type Application struct {
	Edition    string
	Version    string
	Mode       string
	WebRuntime string
}

type Artifact struct {
	ImageDigest string
}

type Lifecycle struct {
	ExpiresAt  time.Time
	Protection bool
}

type NetworkPolicy struct {
	NetworkCIDR string
	Zones       []string
}

// CatalogSelection captures managed-service shape choices. CacheMode is an
// explicit escape hatch: Scaleway does not offer managed Valkey yet, so
// "redis" is the only supported value (see internal/cloud/scaleway/cache).
type CatalogSelection struct {
	DatabaseNodeType   string
	RedisNodeType      string
	CacheMode          string
	KapsuleVersion     string
	NodeType           string
	NodeCount          int
	CPURequest         string
	MemoryRequest      string
	DesiredWebReplicas int
	QueueConsumerCount int
}

type Dependencies struct {
	DatabaseName        string
	MasterUsername      string
	EncryptionKeySecret string
	StateBucket         string
	StateEndpoint       string
	StateRegion         string
}

func (s Spec) Validate() error {
	var problems []error
	if !stableName.MatchString(s.Identity.Project) {
		problems = append(problems, errors.New("project name must be a stable lowercase identifier"))
	}
	if strings.TrimSpace(s.Identity.ScalewayProject) == "" {
		problems = append(problems, errors.New("Scaleway project ID is required"))
	}
	if !stableName.MatchString(s.Identity.Environment) {
		problems = append(problems, errors.New("environment name must be a stable lowercase identifier"))
	}
	if !scalewayRegion.MatchString(s.Identity.Region) {
		problems = append(problems, fmt.Errorf("invalid Scaleway region %q", s.Identity.Region))
	}
	if !scalewayZone.MatchString(s.Identity.Zone) {
		problems = append(problems, fmt.Errorf("invalid Scaleway zone %q", s.Identity.Zone))
	}
	if s.Identity.EnvironmentClass == "" {
		problems = append(problems, errors.New("environment class is required"))
	}
	if s.Identity.Preset != sdk.PresetPreview && s.Identity.Preset != sdk.PresetStandard && s.Identity.Preset != sdk.PresetHighAvailability {
		problems = append(problems, fmt.Errorf("invalid preset %q", s.Identity.Preset))
	}
	if !digest.MatchString(s.Artifact.ImageDigest) {
		problems = append(problems, errors.New("artifact image digest must be repository@sha256:..."))
	}
	if _, err := netip.ParsePrefix(s.Policy.NetworkCIDR); err != nil {
		problems = append(problems, fmt.Errorf("network CIDR: %w", err))
	}
	if len(s.Policy.Zones) == 0 {
		problems = append(problems, errors.New("at least one zone is required"))
	}
	if s.Catalog.CacheMode != "redis" {
		problems = append(problems, fmt.Errorf("invalid cache mode %q; Scaleway only supports \"redis\" until Valkey ships", s.Catalog.CacheMode))
	}
	if s.Dependencies.DatabaseName == "" || s.Dependencies.MasterUsername == "" {
		problems = append(problems, errors.New("database name and master username are required"))
	}
	if s.Catalog.DesiredWebReplicas < 1 {
		problems = append(problems, errors.New("desired web replicas must be at least 1"))
	}
	if !s.Lifecycle.ExpiresAt.IsZero() && time.Now().After(s.Lifecycle.ExpiresAt) {
		problems = append(problems, errors.New("preview environment has expired"))
	}
	return errors.Join(problems...)
}
