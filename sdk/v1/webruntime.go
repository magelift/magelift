package v1

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// WebRuntimeID is the independently versioned Magento HTTP frontend plugin ID.
// It is not a stack Module ID and is not required to declare CoreOutputKeys.
type WebRuntimeID string

const (
	WebRuntimeNginxFPM          WebRuntimeID = "nginx-fpm"
	WebRuntimeFrankenPHPClassic WebRuntimeID = "frankenphp-classic"
	WebRuntimePHPApache         WebRuntimeID = "php-apache"
	WebRuntimeFrankenPHPWorker  WebRuntimeID = "frankenphp-worker"
)

// AdobeSupport is Adobe's current on-premises HTTP-server row for a Magento
// release. It is independent of MageLift certification.
type AdobeSupport string

const (
	AdobeSupported   AdobeSupport = "supported"
	AdobeUnsupported AdobeSupport = "unsupported"
)

const (
	WebRuntimePlacementProcess = "process"
	WebRuntimePlacementSidecar = "sidecar"
	WebRuntimeHealthPath       = "/health"
)

var magentoReleasePattern = regexp.MustCompile(`^2\.4\.\d+(?:-p[1-9][0-9]*)?$`)

// WebRuntimeComposeHints is the local Compose contract supplied by a plugin.
type WebRuntimeComposeHints struct {
	ImageFamily string `json:"imageFamily" yaml:"imageFamily"`
	HealthPath  string `json:"healthPath" yaml:"healthPath"`
}

// WebRuntimeCloudHints is the cloud HTTP process or sidecar contract.
type WebRuntimeCloudHints struct {
	Ports     []int  `json:"ports" yaml:"ports"`
	Placement string `json:"placement" yaml:"placement"`
}

// WebRuntimeDescriptor is the public metadata for a Magento HTTP frontend.
// First-party and community plugins version this independently of the CLI.
type WebRuntimeDescriptor struct {
	APIVersion      string                     `json:"apiVersion" yaml:"apiVersion"`
	ID              WebRuntimeID               `json:"id" yaml:"id"`
	Version         string                     `json:"version" yaml:"version"`
	Source          string                     `json:"source" yaml:"source"`
	Digest          string                     `json:"digest,omitempty" yaml:"digest,omitempty"`
	Tier            ExtensionCertificationTier `json:"tier" yaml:"tier"`
	Adobe           AdobeSupport               `json:"adobe" yaml:"adobe"`
	MagentoReleases []string                   `json:"magentoReleases" yaml:"magentoReleases"`
	LocalCompose    WebRuntimeComposeHints     `json:"localCompose" yaml:"localCompose"`
	Cloud           WebRuntimeCloudHints       `json:"cloud" yaml:"cloud"`
}

// WebRuntime is the public Magento HTTP port. Stack Modules own cloud graphs
// and CoreOutputKeys. This port owns the HTTP frontend only. MageLift never
// discovers or executes an unsigned file from the working directory.
type WebRuntime interface {
	Descriptor() WebRuntimeDescriptor
	Admit(magentoVersion string, allowUnsupported bool) (warnings []string, err error)
}

func ValidateWebRuntimeDescriptor(descriptor WebRuntimeDescriptor) error {
	var problems []error
	if descriptor.APIVersion != ExtensionAPIVersion {
		problems = append(problems, fmt.Errorf("web-runtime API version %q is not supported", descriptor.APIVersion))
	}
	problems = append(problems,
		validateID("web-runtime ID", string(descriptor.ID)),
		validateExtensionVersion(descriptor.Version),
	)
	if strings.TrimSpace(descriptor.Source) == "" {
		problems = append(problems, errors.New("web-runtime source is required"))
	}
	if descriptor.Digest != "" && !strings.HasPrefix(descriptor.Digest, "sha256:") {
		problems = append(problems, errors.New("web-runtime digest must use sha256: prefix"))
	}
	if descriptor.Tier != ExtensionTierCertified && descriptor.Tier != ExtensionTierExperimental {
		problems = append(problems, fmt.Errorf("invalid web-runtime certification tier %q", descriptor.Tier))
	}
	if descriptor.Adobe != AdobeSupported && descriptor.Adobe != AdobeUnsupported {
		problems = append(problems, fmt.Errorf("invalid web-runtime Adobe support %q", descriptor.Adobe))
	}
	if len(descriptor.MagentoReleases) == 0 {
		problems = append(problems, errors.New("web-runtime must declare at least one Magento release"))
	}
	seenReleases := make(map[string]struct{}, len(descriptor.MagentoReleases))
	for _, release := range descriptor.MagentoReleases {
		if !magentoReleasePattern.MatchString(release) {
			problems = append(problems, fmt.Errorf("web-runtime Magento release %q is not an exact 2.4 line or patch", release))
		}
		if _, exists := seenReleases[release]; exists {
			problems = append(problems, fmt.Errorf("duplicate web-runtime Magento release %q", release))
		}
		seenReleases[release] = struct{}{}
	}
	if strings.TrimSpace(descriptor.LocalCompose.ImageFamily) == "" {
		problems = append(problems, errors.New("web-runtime local Compose image family is required"))
	}
	if !strings.HasPrefix(descriptor.LocalCompose.HealthPath, "/") || strings.ContainsAny(descriptor.LocalCompose.HealthPath, " \t\r\n") {
		problems = append(problems, errors.New("web-runtime local Compose health path must be an absolute HTTP path"))
	}
	switch descriptor.Cloud.Placement {
	case WebRuntimePlacementProcess, WebRuntimePlacementSidecar:
	default:
		problems = append(problems, fmt.Errorf("invalid web-runtime cloud placement %q", descriptor.Cloud.Placement))
	}
	if len(descriptor.Cloud.Ports) == 0 {
		problems = append(problems, errors.New("web-runtime must declare at least one cloud port"))
	}
	seenPorts := make(map[int]struct{}, len(descriptor.Cloud.Ports))
	for _, port := range descriptor.Cloud.Ports {
		if port < 1 || port > 65535 {
			problems = append(problems, fmt.Errorf("web-runtime cloud port %d is out of range", port))
		}
		if _, exists := seenPorts[port]; exists {
			problems = append(problems, fmt.Errorf("duplicate web-runtime cloud port %d", port))
		}
		seenPorts[port] = struct{}{}
	}
	return errors.Join(problems...)
}

// AdobeSupportFor reports Adobe's HTTP-server table for a plugin on one
// Magento release. nginx-fpm is the Adobe-supported default. FrankenPHP has
// no Adobe row. Apache is Adobe-unsupported on 2.4.8-p3+ and 2.4.9.
func AdobeSupportFor(id WebRuntimeID, magentoVersion string) AdobeSupport {
	switch id {
	case WebRuntimeNginxFPM:
		return AdobeSupported
	case WebRuntimePHPApache:
		if adobeListsNginxOnly(magentoVersion) {
			return AdobeUnsupported
		}
		return AdobeSupported
	default:
		return AdobeUnsupported
	}
}

// AdmitWebRuntime is the public Adobe hatch. It never calls an Adobe-unsupported
// plugin Adobe-supported or MageLift-certified.
func AdmitWebRuntime(id WebRuntimeID, magentoVersion string, allowUnsupported bool) ([]string, error) {
	if AdobeSupportFor(id, magentoVersion) == AdobeSupported {
		return nil, nil
	}
	if !allowUnsupported {
		return nil, fmt.Errorf("web-runtime plugin %q is Adobe-unsupported on Magento %s; Adobe lists nginx only; set compatibility.allowUnsupported: true to record the Adobe hatch", id, magentoVersion)
	}
	return []string{
		fmt.Sprintf("web-runtime plugin %q is Adobe-unsupported on Magento %s and MageLift-community experimental; compatibility.allowUnsupported records the Adobe hatch", id, magentoVersion),
	}, nil
}

func WebRuntimeDescriptorsByID(descriptors []WebRuntimeDescriptor) []WebRuntimeDescriptor {
	result := make([]WebRuntimeDescriptor, 0, len(descriptors))
	for _, descriptor := range descriptors {
		result = append(result, cloneWebRuntimeDescriptor(descriptor))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func cloneWebRuntimeDescriptor(descriptor WebRuntimeDescriptor) WebRuntimeDescriptor {
	descriptor.MagentoReleases = append([]string(nil), descriptor.MagentoReleases...)
	descriptor.Cloud.Ports = append([]int(nil), descriptor.Cloud.Ports...)
	return descriptor
}

func adobeListsNginxOnly(magentoVersion string) bool {
	release, ok := parseMagentoRelease(magentoVersion)
	if !ok {
		return true
	}
	if release.major != 2 || release.minor != 4 {
		return true
	}
	if release.patch > 8 {
		return true
	}
	return release.patch == 8 && release.security >= 3
}

type magentoRelease struct {
	major, minor, patch, security int
}

func parseMagentoRelease(version string) (magentoRelease, bool) {
	match := magentoExactVersionPattern.FindStringSubmatch(strings.TrimSpace(version))
	if match == nil {
		return magentoRelease{}, false
	}
	majorMinorPatch := strings.Split(match[1], ".")
	if len(majorMinorPatch) != 3 {
		return magentoRelease{}, false
	}
	major, errMajor := strconv.Atoi(majorMinorPatch[0])
	minor, errMinor := strconv.Atoi(majorMinorPatch[1])
	patch, errPatch := strconv.Atoi(majorMinorPatch[2])
	if errMajor != nil || errMinor != nil || errPatch != nil {
		return magentoRelease{}, false
	}
	security := 0
	if match[2] != "" {
		var err error
		security, err = strconv.Atoi(match[2])
		if err != nil {
			return magentoRelease{}, false
		}
	}
	return magentoRelease{major: major, minor: minor, patch: patch, security: security}, true
}

var magentoExactVersionPattern = regexp.MustCompile(`^(2\.4\.\d+)(?:-p([1-9][0-9]*))?$`)
