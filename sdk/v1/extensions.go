package v1

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const ExtensionAPIVersion = "v1"

var extensionVersionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.+_-]*$`)

type ExtensionCertificationTier string

const (
	ExtensionTierCertified    ExtensionCertificationTier = "certified"
	ExtensionTierExperimental ExtensionCertificationTier = "experimental"
)

var coreOutputKeys = []string{
	"applicationURL",
	"databaseWriter",
	"cacheEndpoint",
	"networkVpcId",
	"clusterName",
	"serviceName",
	"privateSubnetIds",
}

// CoreOutputKeys returns the stable output contract required by deployable
// targets. Provider-specific output keys may be added by an extension.
func CoreOutputKeys() []string {
	return append([]string(nil), coreOutputKeys...)
}

type ExtensionDescriptor struct {
	APIVersion   string                     `json:"apiVersion" yaml:"apiVersion"`
	ID           string                     `json:"id" yaml:"id"`
	Version      string                     `json:"version" yaml:"version"`
	Source       string                     `json:"source" yaml:"source"`
	Digest       string                     `json:"digest,omitempty" yaml:"digest,omitempty"`
	Build        string                     `json:"build,omitempty" yaml:"build,omitempty"`
	Tier         ExtensionCertificationTier `json:"tier" yaml:"tier"`
	Targets      []TargetDescriptor         `json:"targets" yaml:"targets"`
	Capabilities []CapabilityDescriptor     `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
	OutputKeys   []string                   `json:"outputKeys" yaml:"outputKeys"`
}

// Extension is the public metadata contract used by explicit custom binaries.
// Deployment implementation stays in the extension adapter and is not loaded
// from an arbitrary file at runtime.
type Extension interface {
	Descriptor() ExtensionDescriptor
}

func ValidateExtensionDescriptor(descriptor ExtensionDescriptor) error {
	var problems []error
	if descriptor.APIVersion != ExtensionAPIVersion {
		problems = append(problems, fmt.Errorf("extension API version %q is not supported", descriptor.APIVersion))
	}
	problems = append(problems,
		validateID("extension ID", descriptor.ID),
		validateExtensionVersion(descriptor.Version),
	)
	if strings.TrimSpace(descriptor.Source) == "" {
		problems = append(problems, errors.New("extension source is required"))
	}
	if descriptor.Digest != "" && !strings.HasPrefix(descriptor.Digest, "sha256:") {
		problems = append(problems, errors.New("extension digest must use sha256: prefix"))
	}
	if descriptor.Tier != ExtensionTierCertified && descriptor.Tier != ExtensionTierExperimental {
		problems = append(problems, fmt.Errorf("invalid extension certification tier %q", descriptor.Tier))
	}
	if len(descriptor.Targets) == 0 {
		problems = append(problems, errors.New("extension must declare at least one target"))
	} else {
		seenTargets := make(map[TargetID]struct{}, len(descriptor.Targets))
		for _, target := range descriptor.Targets {
			problems = append(problems, ValidateTargetDescriptor(target))
			if _, exists := seenTargets[target.ID]; exists {
				problems = append(problems, fmt.Errorf("duplicate extension target ID %q", target.ID))
			}
			seenTargets[target.ID] = struct{}{}
		}
	}
	problems = append(problems, ValidateCapabilityDescriptors(descriptor.Capabilities))
	seenKeys := make(map[string]struct{}, len(descriptor.OutputKeys))
	for _, key := range descriptor.OutputKeys {
		if strings.TrimSpace(key) == "" {
			problems = append(problems, errors.New("extension output key must not be empty"))
		}
		if _, exists := seenKeys[key]; exists {
			problems = append(problems, fmt.Errorf("duplicate extension output key %q", key))
		}
		seenKeys[key] = struct{}{}
	}
	for _, key := range coreOutputKeys {
		if _, exists := seenKeys[key]; !exists {
			problems = append(problems, fmt.Errorf("extension omits required output key %q", key))
		}
	}
	return errors.Join(problems...)
}

func validateExtensionVersion(version string) error {
	if !extensionVersionPattern.MatchString(version) {
		return fmt.Errorf("extension version %q is not a stable version", version)
	}
	return nil
}

func ExtensionDescriptorsByID(descriptors []ExtensionDescriptor) []ExtensionDescriptor {
	result := append([]ExtensionDescriptor(nil), descriptors...)
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}
