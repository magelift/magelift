package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

var openSearchMinor = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)

// OpenSearchContainerTag translates a managed major.minor selection into
// the corresponding upstream release tag. Full tags pass through unchanged.
func OpenSearchContainerTag(version string) string {
	if openSearchMinor.MatchString(version) {
		return version + ".0"
	}
	return version
}

// ServiceBinding describes an operator-selected Magento dependency. Endpoint
// and Secret identify an existing service without transferring its ownership.
type ServiceBinding struct {
	Parameters     map[string]string `json:"parameters,omitempty" yaml:"parameters,omitempty" config:"Database server defaults for charset, collation and packet size"`
	Adapter        string            `json:"adapter,omitempty" yaml:"adapter,omitempty" config:"Explicitly linked service connection adapter ID"`
	Family         string            `json:"family" yaml:"family" config:"Service engine or implementation"`
	Version        string            `json:"version,omitempty" yaml:"version,omitempty" config:"Selected engine version"`
	Ownership      string            `json:"ownership,omitempty" yaml:"ownership,omitempty" config:"Provisioning ownership" schema:"enum=managed|self-hosted|existing|external,nullable"`
	Endpoint       string            `json:"endpoint,omitempty" yaml:"endpoint,omitempty" config:"Hostname or URL of an existing service"`
	Secret         string            `json:"secret,omitempty" yaml:"secret,omitempty" config:"Provider secret reference for a connection"`
	Image          string            `json:"image,omitempty" yaml:"image,omitempty" config:"Shop-selected service container image"`
	Port           int               `json:"port,omitempty" yaml:"port,omitempty" config:"Service connection port"`
	Username       string            `json:"username,omitempty" yaml:"username,omitempty" config:"Service connection username"`
	EvictionPolicy string            `json:"evictionPolicy,omitempty" yaml:"evictionPolicy,omitempty" config:"Eviction policy for the cache or session store"`
	Plugins        []string          `json:"plugins,omitempty" yaml:"plugins,omitempty" config:"OpenSearch plugins required by the shop"`
	MemoryMiB      int               `json:"memoryMiB,omitempty" yaml:"memoryMiB,omitempty" config:"Redis or Valkey eviction threshold in MiB"`
	StorageGiB     int               `json:"storageGiB,omitempty" yaml:"storageGiB,omitempty" config:"Persistent volume size in GiB for a self-hosted service"`
}

// VersionWithMinor expands a numeric major for APIs that require major.minor.
func (s ServiceBinding) VersionWithMinor() string {
	if s.Version != "" && strings.Trim(s.Version, "0123456789") == "" {
		return s.Version + ".0"
	}
	return s.Version
}

// ManagedBranch returns the major.minor selector used by cloud APIs that select their own patch.
func (s ServiceBinding) ManagedBranch() string {
	version := s.VersionWithMinor()
	parts := strings.Split(version, ".")
	if len(parts) == 3 && parts[2] != "" && strings.Trim(parts[2], "0123456789") == "" {
		return parts[0] + "." + parts[1]
	}
	return version
}

// ServiceAdapter translates one project-selected dependency into an attached
// connection understood by a target. Resolution must be deterministic and
// account-free, including during preview and destroy.
type ServiceAdapter interface {
	ServiceDescriptor() ServiceAdapterDescriptor
	ResolveService(context.Context, ServiceRequest) (ServiceBinding, error)
}

type ServiceAdapterDescriptor struct {
	APIVersion string             `json:"apiVersion" yaml:"apiVersion"`
	ID         string             `json:"id" yaml:"id"`
	Version    string             `json:"version" yaml:"version"`
	Source     string             `json:"source" yaml:"source"`
	Targets    []TargetDescriptor `json:"targets" yaml:"targets"`
	Roles      []string           `json:"roles" yaml:"roles"`
}

func ValidateServiceAdapterDescriptor(d ServiceAdapterDescriptor) error {
	if d.APIVersion != ExtensionAPIVersion || d.Source == "" || len(d.Targets) == 0 || len(d.Roles) == 0 {
		return errors.New("service adapter requires a supported API version, source, targets and roles")
	}
	if err := errors.Join(validateID("service adapter ID", d.ID), validateExtensionVersion(d.Version)); err != nil {
		return err
	}
	for _, target := range d.Targets {
		if err := ValidateTargetDescriptor(target); err != nil {
			return err
		}
	}
	for _, role := range d.Roles {
		if err := validateID("service role", role); err != nil {
			return err
		}
	}
	return nil
}

type ServiceRequest struct {
	Project     string
	Environment string
	Target      TargetDescriptor
	Role        string
	Binding     ServiceBinding
	// Options contains only extensions[adapter ID] from the project.
	Options json.RawMessage
}

func (s ServiceBinding) Attached() bool {
	return s.Ownership == "external" || s.Ownership == "existing"
}

// Address returns the configured endpoint with an explicit port, if supplied.
func (s ServiceBinding) Address() string {
	if s.Port == 0 {
		return s.Endpoint
	}
	if strings.Contains(s.Endpoint, "://") {
		u, err := url.Parse(s.Endpoint)
		if err == nil && u.Port() == "" {
			u.Host = net.JoinHostPort(u.Hostname(), fmt.Sprint(s.Port))
			return u.String()
		}
		return s.Endpoint
	}
	if _, _, err := net.SplitHostPort(s.Endpoint); err == nil {
		return s.Endpoint
	}
	return net.JoinHostPort(strings.Trim(s.Endpoint, "[]"), fmt.Sprint(s.Port))
}

// GCPPreviewPool references infrastructure shared by disposable previews.
// Its cluster and network remain owned by the operator.
type GCPPreviewPool struct {
	Cluster               string   `json:"cluster" yaml:"cluster" config:"Existing regional GKE cluster name"`
	Network               string   `json:"network" yaml:"network" config:"Existing VPC self link or resource ID"`
	Subnet                string   `json:"subnet" yaml:"subnet" config:"Existing regional subnet self link or resource ID"`
	NetworkPolicyEnforced bool     `json:"networkPolicyEnforced" yaml:"networkPolicyEnforced" config:"Confirm the shared cluster enforces Kubernetes NetworkPolicy"`
	IngressCIDRs          []string `json:"ingressCidrs" yaml:"ingressCidrs" config:"Source CIDRs reaching preview web pods, including load balancer and health checks"`
}
