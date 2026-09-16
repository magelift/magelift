package sdk

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// ServiceOwnership describes the operational boundary for a stateful or
// delivery service. It is deliberately provider-neutral; adapters translate
// a managed boundary to their provider API and community modules can add
// their own implementation without changing the core contract.
type ServiceOwnership string

const (
	ServiceManaged    ServiceOwnership = "managed"
	ServiceSelfHosted ServiceOwnership = "self-hosted"
	ServiceExisting   ServiceOwnership = "existing"
	ServiceExternal   ServiceOwnership = "external"
)

// ServiceBoundaryIntent is the smallest unit of architecture identity. A
// service family and major are semantic values (for example valkey/9), not
// provider API enum values such as VALKEY_9_0.
type ServiceBoundaryIntent struct {
	Role      string           `json:"role" yaml:"role"`
	Family    string           `json:"family" yaml:"family"`
	Major     string           `json:"major" yaml:"major"`
	Ownership ServiceOwnership `json:"ownership" yaml:"ownership"`
	// ResourceReference is an opaque provider-owned identity, such as an ARN,
	// project resource name, or community-provider handle. It is deliberately
	// not a provider SDK type so recovery adapters can locate an existing
	// service without leaking vendor schemas into the core.
	ResourceReference string   `json:"resourceReference,omitempty" yaml:"resourceReference,omitempty"`
	FailureDomains    []string `json:"failureDomains,omitempty" yaml:"failureDomains,omitempty"`
	BackupProfile     string   `json:"backupProfile,omitempty" yaml:"backupProfile,omitempty"`
	RecoveryProfile   string   `json:"recoveryProfile,omitempty" yaml:"recoveryProfile,omitempty"`
	CapabilityID      string   `json:"capabilityId,omitempty" yaml:"capabilityId,omitempty"`
}

// ArchitectureIntent is shared by all provider adapters. Provider modules
// may add opaque implementation details to their own plan, but they must not
// redefine these semantic boundaries.
type ArchitectureIntent struct {
	ProfileID           string     `json:"profileId" yaml:"profileId"`
	Provider            ProviderID `json:"provider" yaml:"provider"`
	Runtime             RuntimeID  `json:"runtime" yaml:"runtime"`
	AccountOrProjectRef string     `json:"accountOrProjectRef" yaml:"accountOrProjectRef"`
	Region              string     `json:"region" yaml:"region"`
	Regions             []string   `json:"regions,omitempty" yaml:"regions,omitempty"`
	Zones               []string   `json:"zones,omitempty" yaml:"zones,omitempty"`
	ComputeMode         string     `json:"computeMode" yaml:"computeMode"`
	KubernetesMode      string     `json:"kubernetesMode,omitempty" yaml:"kubernetesMode,omitempty"`
	NetworkMode         string     `json:"networkMode" yaml:"networkMode"`
	// NetworkProfile is an optional provider-neutral sub-boundary, such as
	// private-aws-fck-nat-multi-az-auto-scaling. It is a semantic identifier,
	// never a provider SDK argument bag.
	NetworkProfile string                  `json:"networkProfile,omitempty" yaml:"networkProfile,omitempty"`
	IngressMode    string                  `json:"ingressMode" yaml:"ingressMode"`
	Boundaries     []ServiceBoundaryIntent `json:"boundaries" yaml:"boundaries"`
	Edge           EdgeIntent              `json:"edge" yaml:"edge"`
	Observability  ObservabilityIntent     `json:"observability" yaml:"observability"`
	Resilience     ResilienceIntent        `json:"resilience" yaml:"resilience"`
	// ConfigurationFingerprint identifies the fully resolved, secret-safe
	// configuration consumed by the module. It keeps advanced YAML choices in
	// the core identity without exposing provider SDK types here.
	ConfigurationFingerprint string `json:"configurationFingerprint,omitempty" yaml:"configurationFingerprint,omitempty"`
	ArtifactDigest           string `json:"artifactDigest" yaml:"artifactDigest"`
	SchemaFingerprint        string `json:"schemaFingerprint" yaml:"schemaFingerprint"`
	MigrationFingerprint     string `json:"migrationFingerprint" yaml:"migrationFingerprint"`
	OwnershipMarker          string `json:"ownershipMarker" yaml:"ownershipMarker"`
}

type RecoveryDestination string

const (
	RecoverySameRegion         RecoveryDestination = "same-region"
	RecoverySameRegionIsolated RecoveryDestination = "same-region-isolated"
	RecoveryAlternateRegion    RecoveryDestination = "alternate-region"
	RecoveryAlternateProvider  RecoveryDestination = "alternate-provider"
)

// DataClassIntent makes durable and reconstructible state explicit. Cache and
// search may legitimately use rebuild semantics, but they must not be
// mistaken for database or media backup proof.
type DataClassIntent struct {
	Name               string   `json:"name" yaml:"name"`
	SourceOfTruth      string   `json:"sourceOfTruth" yaml:"sourceOfTruth"`
	FailureDomains     []string `json:"failureDomains" yaml:"failureDomains"`
	BackupMethod       string   `json:"backupMethod" yaml:"backupMethod"`
	RestoreMethod      string   `json:"restoreMethod" yaml:"restoreMethod"`
	IntegrityMethod    string   `json:"integrityMethod" yaml:"integrityMethod"`
	LossSemantics      string   `json:"lossSemantics" yaml:"lossSemantics"`
	RetentionDays      int      `json:"retentionDays" yaml:"retentionDays"`
	Encrypted          bool     `json:"encrypted" yaml:"encrypted"`
	Immutable          bool     `json:"immutable" yaml:"immutable"`
	DeletionProtection bool     `json:"deletionProtection" yaml:"deletionProtection"`
	OwnershipMarker    string   `json:"ownershipMarker" yaml:"ownershipMarker"`
}

// ProjectionTarget identifies the application workload used to rebuild a
// reconstructible search or cache projection. It is intentionally semantic:
// provider adapters translate the target into ECS or Kubernetes APIs, while
// the core keeps credentials, clients, and native request shapes out of the
// public contract.
type ProjectionTarget struct {
	Runtime   string `json:"runtime" yaml:"runtime"`
	Cluster   string `json:"cluster,omitempty" yaml:"cluster,omitempty"`
	Service   string `json:"service,omitempty" yaml:"service,omitempty"`
	Task      string `json:"task,omitempty" yaml:"task,omitempty"`
	Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Workload  string `json:"workload,omitempty" yaml:"workload,omitempty"`
	Container string `json:"container,omitempty" yaml:"container,omitempty"`
}

func (p ProjectionTarget) Validate() error {
	var problems []error
	for name, value := range map[string]string{
		"runtime": p.Runtime, "cluster": p.Cluster, "service": p.Service,
		"task": p.Task, "namespace": p.Namespace, "workload": p.Workload,
		"container": p.Container,
	} {
		if strings.ContainsAny(value, "\r\n\x00") {
			problems = append(problems, fmt.Errorf("projection %s must be single-line and NUL-free", name))
		}
	}
	switch p.Runtime {
	case "ecs":
		if strings.TrimSpace(p.Cluster) == "" || strings.TrimSpace(p.Container) == "" {
			problems = append(problems, errors.New("ECS projection requires cluster and container"))
		}
		if (strings.TrimSpace(p.Service) == "") == (strings.TrimSpace(p.Task) == "") {
			problems = append(problems, errors.New("ECS projection requires exactly one of service or task"))
		}
		if p.Namespace != "" || p.Workload != "" {
			problems = append(problems, errors.New("ECS projection cannot contain Kubernetes namespace or workload"))
		}
	case "kubernetes":
		if strings.TrimSpace(p.Workload) == "" {
			problems = append(problems, errors.New("Kubernetes projection requires workload"))
		}
		if p.Cluster != "" || p.Service != "" || p.Task != "" {
			problems = append(problems, errors.New("Kubernetes projection cannot contain ECS cluster, service, or task"))
		}
	default:
		problems = append(problems, fmt.Errorf("projection runtime must be ecs or kubernetes (got %q)", p.Runtime))
	}
	return errors.Join(problems...)
}

type ResilienceIntent struct {
	ProfileID            string                `json:"profileId" yaml:"profileId"`
	AvailabilityTarget   string                `json:"availabilityTarget" yaml:"availabilityTarget"`
	RPOSeconds           int64                 `json:"rpoSeconds" yaml:"rpoSeconds"`
	RTOSeconds           int64                 `json:"rtoSeconds" yaml:"rtoSeconds"`
	RetentionDays        int                   `json:"retentionDays" yaml:"retentionDays"`
	RecoveryScope        string                `json:"recoveryScope" yaml:"recoveryScope"`
	FailoverOwner        string                `json:"failoverOwner" yaml:"failoverOwner"`
	FencingPolicy        string                `json:"fencingPolicy" yaml:"fencingPolicy"`
	RecoveryDestinations []RecoveryDestination `json:"recoveryDestinations" yaml:"recoveryDestinations"`
	DataClasses          []DataClassIntent     `json:"dataClasses" yaml:"dataClasses"`
	Projection           *ProjectionTarget     `json:"projection,omitempty" yaml:"projection,omitempty"`
}

// Validate checks only semantic invariants. It never calls a provider and it
// does not decide whether a particular provider can implement the intent.
func (a ArchitectureIntent) Validate() error {
	var problems []error
	problems = append(problems,
		validateID("architecture profile ID", a.ProfileID),
		validateID("provider ID", string(a.Provider)),
		validateID("runtime ID", string(a.Runtime)),
	)
	if strings.TrimSpace(a.AccountOrProjectRef) == "" {
		problems = append(problems, errors.New("architecture account or project reference is required"))
	}
	if strings.TrimSpace(a.Region) == "" {
		problems = append(problems, errors.New("architecture region is required"))
	}
	for name, values := range map[string][]string{"architecture region list": a.Regions, "architecture zones": a.Zones} {
		seen := make(map[string]struct{}, len(values))
		for _, value := range values {
			if err := validateID(name, value); err != nil {
				problems = append(problems, err)
			}
			if _, exists := seen[value]; exists {
				problems = append(problems, fmt.Errorf("duplicate %s %q", name, value))
			}
			seen[value] = struct{}{}
		}
	}
	for name, value := range map[string]string{"compute mode": a.ComputeMode, "network mode": a.NetworkMode, "ingress mode": a.IngressMode, "artifact digest": a.ArtifactDigest, "schema fingerprint": a.SchemaFingerprint, "migration fingerprint": a.MigrationFingerprint, "ownership marker": a.OwnershipMarker} {
		if strings.TrimSpace(value) == "" {
			problems = append(problems, fmt.Errorf("architecture %s is required", name))
		}
	}
	if strings.TrimSpace(a.ConfigurationFingerprint) != "" && strings.ContainsAny(a.ConfigurationFingerprint, "\r\n\x00") {
		problems = append(problems, errors.New("architecture configuration fingerprint must be single-line and NUL-free"))
	}
	if strings.TrimSpace(a.NetworkProfile) != "" {
		problems = append(problems, validateID("architecture network profile", a.NetworkProfile))
	}
	seenRoles := make(map[string]struct{}, len(a.Boundaries))
	for _, boundary := range a.Boundaries {
		problems = append(problems, validateServiceBoundary(boundary))
		if _, exists := seenRoles[boundary.Role]; exists {
			problems = append(problems, fmt.Errorf("duplicate architecture service role %q", boundary.Role))
		}
		seenRoles[boundary.Role] = struct{}{}
	}
	for _, dataClass := range a.Resilience.DataClasses {
		if !ownershipWithin(a.OwnershipMarker, dataClass.OwnershipMarker) {
			problems = append(problems, fmt.Errorf("resilience data class %q ownership marker must be scoped by architecture ownership", dataClass.Name))
		}
	}
	problems = append(problems, a.Resilience.Validate(), ValidateEdgeIntent(a.Edge), ValidateObservabilityIntent(a.Observability))
	return errors.Join(problems...)
}

func (r ResilienceIntent) Validate() error {
	var problems []error
	for name, value := range map[string]string{"profile ID": r.ProfileID, "availability target": r.AvailabilityTarget, "recovery scope": r.RecoveryScope, "failover owner": r.FailoverOwner, "fencing policy": r.FencingPolicy} {
		if strings.TrimSpace(value) == "" {
			problems = append(problems, fmt.Errorf("resilience %s is required", name))
		}
	}
	if r.RPOSeconds <= 0 || r.RTOSeconds <= 0 {
		problems = append(problems, errors.New("resilience RPO and RTO must be positive"))
	}
	if r.RetentionDays <= 0 {
		problems = append(problems, errors.New("resilience retention must be positive"))
	}
	if len(r.RecoveryDestinations) == 0 {
		problems = append(problems, errors.New("resilience requires at least one recovery destination"))
	}
	seenDestinations := make(map[RecoveryDestination]struct{}, len(r.RecoveryDestinations))
	for _, destination := range r.RecoveryDestinations {
		switch destination {
		case RecoverySameRegion, RecoverySameRegionIsolated, RecoveryAlternateRegion, RecoveryAlternateProvider:
		default:
			problems = append(problems, fmt.Errorf("invalid resilience recovery destination %q", destination))
		}
		if _, exists := seenDestinations[destination]; exists {
			problems = append(problems, fmt.Errorf("duplicate resilience recovery destination %q", destination))
		}
		seenDestinations[destination] = struct{}{}
	}
	if len(r.DataClasses) == 0 {
		problems = append(problems, errors.New("resilience requires data-class policies"))
	}
	if r.Projection != nil {
		problems = append(problems, r.Projection.Validate())
	}
	seenClasses := make(map[string]struct{}, len(r.DataClasses))
	for _, dataClass := range r.DataClasses {
		if strings.TrimSpace(dataClass.Name) == "" || strings.TrimSpace(dataClass.SourceOfTruth) == "" || strings.TrimSpace(dataClass.BackupMethod) == "" || strings.TrimSpace(dataClass.RestoreMethod) == "" || strings.TrimSpace(dataClass.IntegrityMethod) == "" || strings.TrimSpace(dataClass.LossSemantics) == "" || strings.TrimSpace(dataClass.OwnershipMarker) == "" {
			problems = append(problems, fmt.Errorf("resilience data class %q is incomplete", dataClass.Name))
		}
		if dataClass.RetentionDays <= 0 {
			problems = append(problems, fmt.Errorf("resilience data class %q retention must be positive", dataClass.Name))
		}
		if _, exists := seenClasses[dataClass.Name]; exists {
			problems = append(problems, fmt.Errorf("duplicate resilience data class %q", dataClass.Name))
		}
		seenClasses[dataClass.Name] = struct{}{}
	}
	return errors.Join(problems...)
}

func ownershipWithin(parent, child string) bool {
	parent = strings.TrimSuffix(strings.TrimSpace(parent), "/")
	child = strings.TrimSpace(child)
	return parent != "" && child != "" && (child == parent || strings.HasPrefix(child, parent+"/"))
}

func validateServiceBoundary(boundary ServiceBoundaryIntent) error {
	var problems []error
	for name, value := range map[string]string{"role": boundary.Role, "family": boundary.Family, "major": boundary.Major, "backup profile": boundary.BackupProfile, "recovery profile": boundary.RecoveryProfile} {
		if strings.TrimSpace(value) == "" {
			problems = append(problems, fmt.Errorf("service boundary %s is required", name))
		}
	}
	switch boundary.Ownership {
	case ServiceManaged, ServiceSelfHosted, ServiceExisting, ServiceExternal:
	default:
		problems = append(problems, fmt.Errorf("invalid service boundary ownership %q", boundary.Ownership))
	}
	if boundary.CapabilityID != "" {
		problems = append(problems, validateID("service boundary capability ID", boundary.CapabilityID))
	}
	if strings.ContainsAny(boundary.ResourceReference, "\r\n\x00") {
		problems = append(problems, errors.New("service boundary resource reference must be single-line and NUL-free"))
	}
	return errors.Join(problems...)
}

// ValidateEdgeIntent validates composition and secret-reference shape while
// leaving DNS, TLS, WAF, and origin semantics to the edge adapter.
func ValidateEdgeIntent(intent EdgeIntent) error {
	if err := ValidateExternalIntent(intent.ExternalProvider, intent.Lifecycle, intent.Certification, intent.CredentialRefs); err != nil {
		return err
	}
	nativeProvider := strings.TrimSpace(intent.NativeProvider)
	externalProvider := strings.TrimSpace(intent.ExternalProvider)
	hasNative := nativeProvider != "" && nativeProvider != "none"
	hasExternal := externalProvider != "" && externalProvider != "none"
	if hasNative {
		if err := validateID("native edge provider ID", nativeProvider); err != nil {
			return err
		}
	}
	mode := strings.TrimSpace(intent.Mode)
	if mode == "" {
		if !hasExternal {
			mode = "none"
		} else {
			mode = "external"
		}
	}
	switch mode {
	case "none", "native", "external", "both":
	default:
		return fmt.Errorf("invalid edge mode %q", intent.Mode)
	}
	if mode == "native" || mode == "both" {
		if !hasNative {
			return errors.New("native edge mode requires a native provider")
		}
	}
	if mode == "external" || mode == "both" {
		if !hasExternal {
			return errors.New("external edge mode requires a provider")
		}
	}
	if mode == "none" && (hasNative || hasExternal) {
		return errors.New("edge mode none cannot declare a provider")
	}
	if mode == "native" && hasExternal {
		return errors.New("native edge mode cannot declare an external provider")
	}
	if mode == "external" && hasNative {
		return errors.New("external edge mode cannot declare a native provider")
	}
	if mode != "none" && strings.TrimSpace(intent.OriginHealthRef) == "" {
		return errors.New("edge mode requires an origin health reference")
	}
	if mode != "none" && strings.TrimSpace(intent.OwnershipMarker) == "" {
		return errors.New("edge mode requires an ownership marker")
	}
	if intent.TLS {
		if strings.TrimSpace(intent.TLSMode) == "" {
			return errors.New("edge TLS requires an explicit certificate ownership mode")
		}
		if strings.TrimSpace(intent.DNSMode) == "" {
			return errors.New("edge TLS requires an explicit DNS ownership mode")
		}
	}
	if health := intent.Health; health.OriginURL != "" {
		parsed, err := url.Parse(strings.TrimSpace(health.OriginURL))
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return errors.New("edge health origin URL must use http or https")
		}
	}
	if strings.ContainsAny(healthString(intent.Health), "\r\n\x00") {
		return errors.New("edge health values must not contain control characters")
	}
	if health := intent.Health; health.RoutePath != "" && !strings.HasPrefix(health.RoutePath, "/") {
		return errors.New("edge health route path must start with /")
	}
	if health := intent.Health; health.ExpectedStatus < 0 || health.ExpectedStatus > 599 || (health.ExpectedStatus > 0 && health.ExpectedStatus < 100) {
		return errors.New("edge health expected status must be between 100 and 599 when set")
	}
	if health := intent.Health; health.RouteTimeoutSeconds < 0 || health.RouteTimeoutSeconds > 3600 {
		return errors.New("edge health route timeout must be between 1 and 3600 seconds when set")
	}
	if health := intent.Health; health.RoutePollSeconds < 0 || health.RoutePollSeconds > 300 {
		return errors.New("edge health route poll interval must be between 1 and 300 seconds when set")
	}
	for _, domain := range intent.Domains {
		if strings.TrimSpace(domain) == "" || strings.ContainsAny(domain, "\r\n /:") {
			return fmt.Errorf("edge domain %q is invalid", domain)
		}
	}
	return nil
}

func healthString(health EdgeHealthIntent) string {
	return health.OriginURL + health.OriginHost + health.ExpectedRouteTarget + health.RoutePath
}

// ValidateObservabilityIntent validates the generic signal contract. Provider
// adapters decide which signals are native and whether OTLP is required.
func ValidateObservabilityIntent(intent ObservabilityIntent) error {
	if err := ValidateExternalIntent(intent.ExternalProvider, intent.Lifecycle, intent.Certification, intent.CredentialRefs); err != nil {
		return err
	}
	nativeProvider := strings.TrimSpace(intent.NativeProvider)
	externalProvider := strings.TrimSpace(intent.ExternalProvider)
	hasNative := nativeProvider != "" && nativeProvider != "none"
	hasExternal := externalProvider != "" && externalProvider != "none"
	if hasNative {
		if err := validateID("native observability provider ID", nativeProvider); err != nil {
			return err
		}
	}
	if hasNative || hasExternal {
		if err := validateOwnershipMarker("observability ownership marker", intent.OwnershipMarker); err != nil {
			return err
		}
	}
	if intent.RetentionDays < 0 || intent.RetentionDays > 3650 {
		return errors.New("observability retention must be between 0 and 3650 days")
	}
	if intent.SamplingRatio < 0 || intent.SamplingRatio > 1 {
		return errors.New("observability sampling ratio must be between 0 and 1")
	}
	if len(intent.Signals) == 0 && (intent.Logs || intent.Metrics || intent.Traces) {
		return errors.New("observability signals must be named when telemetry is enabled")
	}
	if len(intent.Signals) > 0 && !hasNative && !hasExternal {
		return errors.New("observability signals require a native or external destination")
	}
	seenSignals := make(map[string]struct{}, len(intent.Signals))
	for _, signal := range intent.Signals {
		if err := validateID("observability signal", signal); err != nil {
			return err
		}
		if _, exists := seenSignals[signal]; exists {
			return fmt.Errorf("duplicate observability signal %q", signal)
		}
		seenSignals[signal] = struct{}{}
	}
	for name, value := range intent.Labels {
		if strings.TrimSpace(name) == "" || strings.ContainsAny(name, "\r\n") || sensitiveField.MatchString(name) {
			return fmt.Errorf("observability label key %q is empty, unsafe, or sensitive", name)
		}
		if strings.ContainsAny(value, "\r\n") || strings.Contains(value, "://") || sensitiveField.MatchString(value) {
			return fmt.Errorf("observability label %q contains a secret-like or unsafe value", name)
		}
	}
	if hasExternal && len(intent.CredentialRefs) == 0 && strings.TrimSpace(intent.Endpoint) == "" {
		return errors.New("external observability requires a credential reference or endpoint")
	}
	if externalProvider == "newrelic" && len(intent.CredentialRefs) == 0 {
		return errors.New("New Relic observability requires an opaque license-key credential reference")
	}
	if strings.ContainsAny(intent.DataResidency, "\r\n") {
		return errors.New("observability data residency must not contain line breaks")
	}
	if strings.ContainsAny(intent.NativeReference, "\r\n\x00") {
		return errors.New("observability native reference must be single-line and NUL-free")
	}
	problems := make([]error, 0, len(intent.Alerts)+len(intent.Dashboards)+len(intent.SLOs))
	seenAlerts := make(map[string]struct{}, len(intent.Alerts))
	for _, alert := range intent.Alerts {
		problems = append(problems, validateAlertIntent(alert))
		if _, exists := seenAlerts[alert.ID]; exists {
			problems = append(problems, fmt.Errorf("duplicate observability alert %q", alert.ID))
		}
		seenAlerts[alert.ID] = struct{}{}
	}
	seenDashboards := make(map[string]struct{}, len(intent.Dashboards))
	for _, dashboard := range intent.Dashboards {
		problems = append(problems, validateDashboardIntent(dashboard))
		if _, exists := seenDashboards[dashboard.ID]; exists {
			problems = append(problems, fmt.Errorf("duplicate observability dashboard %q", dashboard.ID))
		}
		seenDashboards[dashboard.ID] = struct{}{}
	}
	seenSLOs := make(map[string]struct{}, len(intent.SLOs))
	for _, slo := range intent.SLOs {
		problems = append(problems, validateSLOIntent(slo))
		if _, exists := seenSLOs[slo.ID]; exists {
			problems = append(problems, fmt.Errorf("duplicate observability SLO %q", slo.ID))
		}
		seenSLOs[slo.ID] = struct{}{}
	}
	if err := errors.Join(problems...); err != nil {
		return err
	}
	return nil
}

func validateDashboardIntent(dashboard DashboardIntent) error {
	if err := validateID("observability dashboard ID", dashboard.ID); err != nil {
		return err
	}
	if strings.TrimSpace(dashboard.Owner) == "" {
		return fmt.Errorf("observability dashboard %q requires an owner", dashboard.ID)
	}
	if len(dashboard.Signals) == 0 {
		return fmt.Errorf("observability dashboard %q requires at least one signal", dashboard.ID)
	}
	seen := make(map[string]struct{}, len(dashboard.Signals))
	for _, signal := range dashboard.Signals {
		if err := validateID("observability dashboard signal", signal); err != nil {
			return err
		}
		if _, exists := seen[signal]; exists {
			return fmt.Errorf("observability dashboard %q contains duplicate signal %q", dashboard.ID, signal)
		}
		seen[signal] = struct{}{}
	}
	return nil
}

func validateAlertIntent(alert AlertIntent) error {
	if err := validateID("observability alert ID", alert.ID); err != nil {
		return err
	}
	if err := validateID("observability alert signal", alert.Signal); err != nil {
		return err
	}
	switch alert.Severity {
	case "info", "warning", "critical":
	default:
		return fmt.Errorf("observability alert %q has invalid severity %q", alert.ID, alert.Severity)
	}
	switch alert.Operator {
	case "gt", "gte", "lt", "lte", "eq":
	default:
		return fmt.Errorf("observability alert %q has invalid operator %q", alert.ID, alert.Operator)
	}
	if alert.Threshold < 0 || alert.WindowSeconds <= 0 {
		return fmt.Errorf("observability alert %q requires a non-negative threshold and positive window", alert.ID)
	}
	if strings.TrimSpace(alert.Owner) == "" || strings.TrimSpace(alert.RunbookURL) == "" || strings.TrimSpace(alert.DeduplicationKey) == "" {
		return fmt.Errorf("observability alert %q requires an owner, runbook URL, and deduplication key", alert.ID)
	}
	if err := validateHTTPSURL("observability alert runbook URL", alert.RunbookURL); err != nil {
		return err
	}
	return nil
}

func validateSLOIntent(slo SLOIntent) error {
	if err := validateID("observability SLO ID", slo.ID); err != nil {
		return err
	}
	if err := validateID("observability SLO signal", slo.Signal); err != nil {
		return err
	}
	if slo.Target <= 0 || slo.Target > 1 || slo.WindowSeconds <= 0 {
		return fmt.Errorf("observability SLO %q requires a target in (0,1] and a positive window", slo.ID)
	}
	if strings.TrimSpace(slo.Owner) == "" || strings.TrimSpace(slo.RunbookURL) == "" || strings.TrimSpace(slo.ErrorBudgetPolicy) == "" {
		return fmt.Errorf("observability SLO %q requires an owner, runbook URL, and error-budget policy", slo.ID)
	}
	return validateHTTPSURL("observability SLO runbook URL", slo.RunbookURL)
}

func validateHTTPSURL(name, value string) error {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return fmt.Errorf("%s must be an HTTPS URL", name)
	}
	return nil
}

func validateOwnershipMarker(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", name)
	}
	if len(value) > 256 || strings.ContainsAny(value, "\r\n\x00") || sensitiveField.MatchString(value) {
		return fmt.Errorf("%s must be a non-secret marker without line breaks", name)
	}
	return nil
}

// SortedStrings is shared by adapters when producing deterministic fingerprints
// without exposing provider-specific map ordering.
func SortedStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}
