package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/magelift/magelift/internal/config"
	sdk "github.com/magelift/magelift/sdk/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type publicModuleAdapter struct {
	module     sdk.Module
	descriptor sdk.ExtensionDescriptor
	target     sdk.TargetDescriptor
}

type publicPlannedStack struct {
	module  sdk.Module
	request sdk.ModulePlanRequest
	plan    sdk.ModulePlan
	ctx     context.Context
}

func newPublicModuleAdapter(module sdk.Module) (publicModuleAdapter, error) {
	if module == nil {
		return publicModuleAdapter{}, errors.New("extension module is required")
	}
	descriptor := module.Descriptor()
	if err := sdk.ValidateExtensionDescriptor(descriptor); err != nil {
		return publicModuleAdapter{}, fmt.Errorf("validate extension module: %w", err)
	}
	if len(descriptor.Targets) != 1 {
		return publicModuleAdapter{}, errors.New("a deployable extension module must declare exactly one target")
	}
	return publicModuleAdapter{
		module:     module,
		descriptor: descriptor,
		target:     descriptor.Targets[0],
	}, nil
}

func (m publicModuleAdapter) Descriptor() sdk.TargetDescriptor { return m.target }

func (m publicModuleAdapter) CertificationTier() CertificationTier {
	if m.descriptor.Tier == sdk.ExtensionTierCertified {
		return TierCertified
	}
	return TierExperimental
}

func (m publicModuleAdapter) Plan(cfg config.Config, environment string, opts PlanOptions) (PlannedStack, error) {
	request, err := publicPlanRequest(cfg, environment, opts)
	if err != nil {
		return nil, err
	}
	plan, err := m.module.Plan(planContext(opts), request)
	if err != nil {
		return nil, fmt.Errorf("plan extension %q: %w", m.descriptor.ID, err)
	}
	if err := validatePublicPlan(m.target, m.descriptor.Tier, request, plan); err != nil {
		return nil, fmt.Errorf("extension %q returned an invalid plan: %w", m.descriptor.ID, err)
	}
	return publicPlannedStack{module: m.module, request: request, plan: plan, ctx: planContext(opts)}, nil
}

func (m publicModuleAdapter) Program(planned PlannedStack) (pulumi.RunFunc, error) {
	value, ok := planned.(publicPlannedStack)
	if !ok || value.plan.Target != m.target {
		return nil, fmt.Errorf("extension module %q received unexpected planned type %T", m.descriptor.ID, planned)
	}
	programValue, err := m.module.Program(value.plan)
	if err != nil {
		return nil, fmt.Errorf("create program for extension %q: %w", m.descriptor.ID, err)
	}
	program, ok := programValue.(pulumi.RunFunc)
	if !ok {
		return nil, fmt.Errorf("extension module %q returned %T, want pulumi.RunFunc", m.descriptor.ID, programValue)
	}
	if program == nil {
		return nil, fmt.Errorf("extension module %q returned a nil program", m.descriptor.ID)
	}
	return program, nil
}

func (m publicModuleAdapter) OutputKeys() []string {
	return append([]string(nil), m.descriptor.OutputKeys...)
}

// Resilience exposes the optional public recovery adapter without making resilience
// a mandatory method on every extension. Community providers can implement the
// same SDK interface when they own backup, restore, and fencing operations.
func (m publicModuleAdapter) Resilience() sdk.ResilienceAdapter {
	adapter, _ := m.module.(sdk.ResilienceAdapter)
	return adapter
}

func (m publicModuleAdapter) Edge() sdk.EdgeAdapter {
	adapter, _ := m.module.(sdk.EdgeAdapter)
	return adapter
}

func (m publicModuleAdapter) Observability() sdk.ObservabilityAdapter {
	adapter, _ := m.module.(sdk.ObservabilityAdapter)
	return adapter
}

func (m publicModuleAdapter) Certification() sdk.CertificationAdapter {
	adapter, _ := m.module.(sdk.CertificationAdapter)
	return adapter
}

func (m publicModuleAdapter) CertificationAdmission() sdk.CertificationAdmissionAdapter {
	adapter, _ := m.module.(sdk.CertificationAdmissionAdapter)
	return adapter
}

func (m publicModuleAdapter) CollectorDeployment() sdk.CollectorDeploymentAdapter {
	adapter, _ := m.module.(sdk.CollectorDeploymentAdapter)
	return adapter
}

func (m publicModuleAdapter) NewResilience(ctx context.Context, planned PlannedStack) (sdk.ResilienceAdapter, error) {
	value, ok := planned.(publicPlannedStack)
	if !ok || value.plan.Target != m.target {
		return nil, fmt.Errorf("extension module %q received unexpected planned type %T", m.descriptor.ID, planned)
	}
	if factory, ok := m.module.(sdk.ResilienceAdapterFactory); ok {
		return factory.NewResilience(ctx, value.plan)
	}
	return m.Resilience(), nil
}

func (m publicModuleAdapter) NewEdge(ctx context.Context, planned PlannedStack) (sdk.EdgeAdapter, error) {
	value, ok := planned.(publicPlannedStack)
	if !ok || value.plan.Target != m.target {
		return nil, fmt.Errorf("extension module %q received unexpected planned type %T", m.descriptor.ID, planned)
	}
	if factory, ok := m.module.(sdk.EdgeAdapterFactory); ok {
		return factory.NewEdge(ctx, value.plan)
	}
	return m.Edge(), nil
}

func (m publicModuleAdapter) NewObservability(ctx context.Context, planned PlannedStack) (sdk.ObservabilityAdapter, error) {
	value, ok := planned.(publicPlannedStack)
	if !ok || value.plan.Target != m.target {
		return nil, fmt.Errorf("extension module %q received unexpected planned type %T", m.descriptor.ID, planned)
	}
	if factory, ok := m.module.(sdk.ObservabilityAdapterFactory); ok {
		return factory.NewObservability(ctx, value.plan)
	}
	return m.Observability(), nil
}

func (m publicModuleAdapter) NewCollectorDeployment(ctx context.Context, planned PlannedStack) (sdk.CollectorDeploymentAdapter, error) {
	value, ok := planned.(publicPlannedStack)
	if !ok || value.plan.Target != m.target {
		return nil, fmt.Errorf("extension module %q received unexpected planned type %T", m.descriptor.ID, planned)
	}
	if factory, ok := m.module.(sdk.CollectorDeploymentAdapterFactory); ok {
		return factory.NewCollectorDeployment(ctx, value.plan)
	}
	return m.CollectorDeployment(), nil
}

func (m publicModuleAdapter) NewCertification(ctx context.Context, planned PlannedStack) (sdk.CertificationAdapter, error) {
	value, ok := planned.(publicPlannedStack)
	if !ok || value.plan.Target != m.target {
		return nil, fmt.Errorf("extension module %q received unexpected planned type %T", m.descriptor.ID, planned)
	}
	if factory, ok := m.module.(sdk.CertificationAdapterFactory); ok {
		return factory.NewCertification(ctx, value.plan)
	}
	return m.Certification(), nil
}

func (m publicModuleAdapter) NewCertificationAdmission(ctx context.Context, planned PlannedStack) (sdk.CertificationAdmissionAdapter, error) {
	value, ok := planned.(publicPlannedStack)
	if !ok || value.plan.Target != m.target {
		return nil, fmt.Errorf("extension module %q received unexpected planned type %T", m.descriptor.ID, planned)
	}
	if factory, ok := m.module.(sdk.CertificationAdmissionAdapterFactory); ok {
		return factory.NewCertificationAdmission(ctx, value.plan)
	}
	return m.CertificationAdmission(), nil
}

func (m publicModuleAdapter) NewPlanAdmission(ctx context.Context, planned PlannedStack) (PlanAdmission, error) {
	value, ok := planned.(publicPlannedStack)
	if !ok || value.plan.Target != m.target {
		return nil, fmt.Errorf("extension module %q received unexpected planned type %T", m.descriptor.ID, planned)
	}
	if factory, ok := m.module.(sdk.PlanAdmissionFactory); ok {
		admission, err := factory.NewPlanAdmission(ctx, value.plan)
		if err != nil {
			return nil, err
		}
		if admission == nil {
			return nil, nil
		}
		return publicPlanAdmission{admission: admission}, nil
	}
	if admission, ok := m.module.(sdk.PlanAdmission); ok {
		return publicPlanAdmission{admission: admission}, nil
	}
	return nil, nil
}

type publicPlanAdmission struct {
	admission sdk.PlanAdmission
}

func (a publicPlanAdmission) Admit(ctx context.Context, planned PlannedStack) (PlannedStack, error) {
	value, ok := planned.(publicPlannedStack)
	if !ok {
		return nil, fmt.Errorf("extension admission received unexpected planned type %T", planned)
	}
	next, err := a.admission.Admit(ctx, value.plan)
	if err != nil {
		return nil, err
	}
	if err := validatePublicPlan(value.plan.Target, value.plan.Tier, value.request, next); err != nil {
		return nil, fmt.Errorf("extension returned an invalid admitted plan: %w", err)
	}
	return publicPlannedStack{module: value.module, request: value.request, plan: next, ctx: value.ctx}, nil
}

func (p publicPlannedStack) StackName() string        { return p.plan.StackName }
func (p publicPlannedStack) Provider() sdk.ProviderID { return p.plan.Provider }
func (p publicPlannedStack) Runtime() sdk.RuntimeID   { return p.plan.Runtime }
func (p publicPlannedStack) Project() string          { return p.plan.Project }
func (p publicPlannedStack) Environment() string      { return p.plan.Environment }
func (p publicPlannedStack) Region() string           { return p.plan.Region }
func (p publicPlannedStack) EnvironmentClass() string { return p.plan.EnvironmentClass }
func (p publicPlannedStack) Protected() bool          { return p.plan.Protected }
func (p publicPlannedStack) ImageDigest() string      { return p.plan.ImageDigest }
func (p publicPlannedStack) TargetDescriptor() sdk.TargetDescriptor {
	return p.plan.Target
}
func (p publicPlannedStack) CertificationTier() CertificationTier {
	if p.plan.Tier == sdk.ExtensionTierCertified {
		return TierCertified
	}
	return TierExperimental
}

func (p publicPlannedStack) Resilience() sdk.ResilienceAdapter {
	adapter, _ := p.module.(sdk.ResilienceAdapter)
	return adapter
}

func (p publicPlannedStack) Edge() sdk.EdgeAdapter {
	adapter, _ := p.module.(sdk.EdgeAdapter)
	return adapter
}

func (p publicPlannedStack) Observability() sdk.ObservabilityAdapter {
	adapter, _ := p.module.(sdk.ObservabilityAdapter)
	return adapter
}

func (p publicPlannedStack) CollectorDeployment() sdk.CollectorDeploymentAdapter {
	adapter, _ := p.module.(sdk.CollectorDeploymentAdapter)
	return adapter
}

func (p publicPlannedStack) WithImageDigest(digest string) (PlannedStack, error) {
	nextRequest := p.request
	nextRequest.Artifact.ImageDigest = digest
	nextRequest.Architecture.ArtifactDigest = digest
	nextPlan, err := p.module.Plan(planContextFrom(p.ctx), nextRequest)
	if err != nil {
		return nil, fmt.Errorf("replan extension %q with image digest: %w", p.plan.Target.ID, err)
	}
	if err := validatePublicPlan(p.plan.Target, p.plan.Tier, nextRequest, nextPlan); err != nil {
		return nil, fmt.Errorf("extension %q returned an invalid digest plan: %w", p.plan.Target.ID, err)
	}
	return publicPlannedStack{module: p.module, request: nextRequest, plan: nextPlan, ctx: p.ctx}, nil
}

func publicPlanRequest(cfg config.Config, environment string, opts PlanOptions) (sdk.ModulePlanRequest, error) {
	configuration, err := configMap(cfg)
	if err != nil {
		return sdk.ModulePlanRequest{}, fmt.Errorf("encode extension configuration: %w", err)
	}
	configurationFingerprint, err := config.ResolvedFingerprint(cfg)
	if err != nil {
		return sdk.ModulePlanRequest{}, fmt.Errorf("fingerprint resolved configuration: %w", err)
	}
	edge := sdk.EdgeIntent{
		ExternalProvider:  cfg.Edge.ExternalProvider,
		Lifecycle:         externalLifecycle(cfg.Edge.ExternalProvider),
		Certification:     externalCertification(cfg.Edge.ExternalProvider),
		Mode:              edgeMode(cfg.Edge),
		NativeProvider:    cfg.Edge.NativeProvider,
		CredentialRefs:    stringSlice(cfg.Edge.TokenSecret),
		ServiceReference:  cfg.Edge.ServiceID,
		Domains:           append([]string(nil), cfg.Edge.Domains...),
		TLS:               cfg.Edge.TLS,
		TLSMode:           cfg.Edge.TLSMode,
		DNSMode:           cfg.Edge.DNSMode,
		PurgeOnDeploy:     cfg.Edge.PurgeOnDeploy,
		PolicyReference:   cfg.Edge.VCLRef,
		OriginHealthRef:   cfg.Edge.OriginHealthRef,
		CachePolicyRef:    cfg.Edge.CachePolicyRef,
		PurgePolicyRef:    cfg.Edge.PurgePolicyRef,
		WAFPolicyRef:      cfg.Edge.WAFPolicyRef,
		FailoverPolicyRef: cfg.Edge.FailoverPolicyRef,
		OwnershipMarker:   edgeOwnershipMarker(cfg, environment),
	}
	if health := cfg.Edge.Health; health != nil {
		edge.Health = sdk.EdgeHealthIntent{
			OriginURL: health.OriginURL, OriginHost: health.OriginHost, ExpectedRouteTarget: health.ExpectedCNAME, RoutePath: health.RoutePath,
			ExpectedStatus: health.ExpectedStatus, RouteTimeoutSeconds: health.RouteTimeoutSeconds, RoutePollSeconds: health.RoutePollSeconds,
		}
	}
	observability, err := ObservabilityIntentFromConfig(cfg, environment)
	if err != nil {
		return sdk.ModulePlanRequest{}, err
	}
	if err := sdk.ValidateEdgeIntent(edge); err != nil {
		return sdk.ModulePlanRequest{}, fmt.Errorf("validate edge intent: %w", err)
	}
	resilience := resilienceIntent(cfg.Resilience)
	architecture := sdk.ArchitectureIntent{
		ProfileID:                stableProfileID(cfg.Target.Provider, cfg.Target.Runtime, environment),
		Provider:                 sdk.ProviderID(cfg.Target.Provider),
		Runtime:                  sdk.RuntimeID(architectureRuntime(cfg)),
		AccountOrProjectRef:      configuredAccountOrProject(cfg),
		Region:                   configuredRegion(cfg),
		Regions:                  configuredRegions(cfg),
		Zones:                    configuredZones(cfg),
		ComputeMode:              architectureComputeMode(cfg),
		KubernetesMode:           architectureKubernetesMode(cfg),
		NetworkMode:              architectureNetworkMode(cfg),
		NetworkProfile:           architectureNetworkProfile(cfg),
		IngressMode:              architectureIngressMode(cfg),
		Boundaries:               architectureBoundaries(cfg),
		ArtifactDigest:           configuredImageDigest(cfg),
		ConfigurationFingerprint: configurationFingerprint,
		SchemaFingerprint:        schemaFingerprint(),
		MigrationFingerprint:     migrationFingerprint(cfg),
		OwnershipMarker:          architectureOwnershipMarker(cfg, environment),
		Edge:                     edge,
		Observability:            observability,
		Resilience:               resilience,
	}
	return sdk.ModulePlanRequest{
		Application: sdk.Application{
			Edition: cfg.Application.Edition,
			Version: cfg.Application.Version,
			Mode:    cfg.Application.Mode,
		},
		Artifact:     sdk.BuildArtifact{ImageDigest: configuredImageDigest(cfg)},
		Architecture: architecture,
		Resilience:   resilience,
		Edge:         edge, Observability: observability,
		Project: cfg.Project.Name, Environment: environment,
		Region:           configuredRegion(cfg),
		EnvironmentClass: cfg.Class,
		Protected:        cfg.Protection,
		AllowExpired:     opts.AllowExpiredPreview,
		Configuration:    configuration,
	}, nil
}

func resilienceIntent(value config.ResilienceConfig) sdk.ResilienceIntent {
	result := sdk.ResilienceIntent{
		ProfileID: value.ProfileID, AvailabilityTarget: value.AvailabilityTarget,
		RPOSeconds: value.RPOSeconds, RTOSeconds: value.RTOSeconds, RetentionDays: value.RetentionDays,
		RecoveryScope: value.RecoveryScope, FailoverOwner: value.FailoverOwner, FencingPolicy: value.FencingPolicy,
		RecoveryDestinations: make([]sdk.RecoveryDestination, 0, len(value.RecoveryDestinations)),
		DataClasses:          make([]sdk.DataClassIntent, 0, len(value.DataClasses)),
	}
	if projection := value.Projection; projection != nil {
		result.Projection = &sdk.ProjectionTarget{
			Runtime: projection.Runtime, Cluster: projection.Cluster, Service: projection.Service,
			Task: projection.Task, Namespace: projection.Namespace, Workload: projection.Workload,
			Container: projection.Container,
		}
	}
	for _, destination := range value.RecoveryDestinations {
		result.RecoveryDestinations = append(result.RecoveryDestinations, sdk.RecoveryDestination(destination))
	}
	for _, dataClass := range value.DataClasses {
		result.DataClasses = append(result.DataClasses, sdk.DataClassIntent{
			Name: dataClass.Name, SourceOfTruth: dataClass.SourceOfTruth, FailureDomains: append([]string(nil), dataClass.FailureDomains...),
			BackupMethod: dataClass.BackupMethod, RestoreMethod: dataClass.RestoreMethod, IntegrityMethod: dataClass.IntegrityMethod,
			LossSemantics: dataClass.LossSemantics, RetentionDays: dataClass.RetentionDays, Encrypted: dataClass.Encrypted,
			Immutable: dataClass.Immutable, DeletionProtection: dataClass.DeletionProtection, OwnershipMarker: dataClass.OwnershipMarker,
		})
	}
	return result
}

func stableProfileID(provider, runtime, environment string) string {
	value := strings.ToLower(strings.TrimSpace(provider) + "-" + strings.TrimSpace(runtime) + "-" + strings.TrimSpace(environment))
	value = strings.NewReplacer("_", "-", "/", "-", ".", "-").Replace(value)
	return value
}

func configuredAccountOrProject(cfg config.Config) string {
	if strings.TrimSpace(cfg.Account) != "" {
		return cfg.Account
	}
	switch cfg.Target.Provider {
	case "gcp":
		if cfg.Target.GCP != nil {
			return cfg.Target.GCP.Project
		}
	case "ovh":
		if cfg.Target.OVH != nil {
			return cfg.Target.OVH.ServiceName
		}
	case "scaleway":
		if cfg.Target.Scaleway != nil {
			return cfg.Target.Scaleway.ProjectID
		}
	}
	return ""
}

func edgeMode(edge config.EdgeConfig) string {
	if edge.Mode != "" {
		return edge.Mode
	}
	if edge.NativeProvider != "" && edge.ExternalProvider != "" && edge.ExternalProvider != "none" {
		return "both"
	}
	if edge.NativeProvider != "" {
		return "native"
	}
	if edge.ExternalProvider != "" {
		return "external"
	}
	return "none"
}

func externalLifecycle(provider string) sdk.ExternalLifecycle {
	switch provider {
	case "", "none":
		return ""
	case "cloudwatch", "google-cloud-operations":
		return sdk.ExternalLifecycleManaged
	default:
		return sdk.ExternalLifecycleExtension
	}
}

func externalCertification(provider string) sdk.ExternalCertificationStatus {
	switch provider {
	case "", "none":
		return ""
	case "cloudwatch", "google-cloud-operations":
		return sdk.ExternalCertified
	default:
		return sdk.ExternalExperimental
	}
}

func nativeObservabilityProvider(cfg config.Config) string {
	return strings.TrimSpace(cfg.Observability.NativeProvider)
}

// ObservabilityIntentFromConfig is the one portable mapping from first-party
// configuration to the public SDK telemetry contract. Provider adapters may
// consume NativeReference, but they must not add vendor fields to the core
// intent.
func ObservabilityIntentFromConfig(cfg config.Config, environment string) (sdk.ObservabilityIntent, error) {
	intent := sdk.ObservabilityIntent{
		Lifecycle:          externalLifecycle(cfg.Observability.ExternalProvider),
		Certification:      externalCertification(cfg.Observability.ExternalProvider),
		OwnershipMarker:    architectureOwnershipMarker(cfg, environment),
		NativeProvider:     nativeObservabilityProvider(cfg),
		NativeReference:    strings.TrimSpace(cfg.Observability.NativeReference),
		ExternalProvider:   externalObservabilityProvider(cfg),
		CredentialRefs:     append([]string(nil), cfg.Observability.CredentialReferences...),
		Endpoint:           cfg.Observability.Endpoint,
		ServiceName:        cfg.Observability.ServiceName,
		Environment:        cfg.Observability.Environment,
		Logs:               cfg.Observability.Logs,
		Metrics:            cfg.Observability.Metrics,
		Traces:             cfg.Observability.Traces,
		Signals:            observabilitySignals(cfg.Observability),
		RetentionDays:      cfg.Observability.RetentionDays,
		SamplingRatio:      cfg.Observability.SamplingRatio,
		RedactionPolicyRef: cfg.Observability.RedactionPolicyRef,
		AlertRefs:          append([]string(nil), cfg.Observability.AlertReferences...),
		Labels:             cloneStringMap(cfg.Observability.Labels),
		DataResidency:      cfg.Observability.DataResidency,
		Alerts:             observabilityAlerts(cfg.Observability.Alerts),
		Dashboards:         observabilityDashboards(cfg.Observability.Dashboards),
		SLOs:               observabilitySLOs(cfg.Observability.SLOs),
	}
	if err := sdk.ValidateObservabilityIntent(intent); err != nil {
		return sdk.ObservabilityIntent{}, fmt.Errorf("validate observability intent: %w", err)
	}
	return intent, nil
}

func externalObservabilityProvider(cfg config.Config) string {
	return strings.TrimSpace(cfg.Observability.ExternalProvider)
}

func architectureOwnershipMarker(cfg config.Config, environment string) string {
	return "magelift/architecture/" + stableProfileID(cfg.Target.Provider, cfg.Project.Name, environment)
}

func edgeOwnershipMarker(cfg config.Config, environment string) string {
	if marker := strings.TrimSpace(cfg.Edge.OwnershipMarker); marker != "" {
		return marker
	}
	return "magelift/edge/" + stableProfileID(cfg.Target.Provider, cfg.Project.Name, environment)
}

func observabilitySignals(value config.ObservabilityConfig) []string {
	if len(value.Signals) > 0 {
		return append([]string(nil), value.Signals...)
	}
	var signals []string
	if value.Logs {
		signals = append(signals, "logs")
	}
	if value.Metrics {
		signals = append(signals, "metrics")
	}
	if value.Traces {
		signals = append(signals, "traces")
	}
	return signals
}

func observabilityAlerts(values []config.ObservabilityAlert) []sdk.AlertIntent {
	result := make([]sdk.AlertIntent, 0, len(values))
	for _, value := range values {
		result = append(result, sdk.AlertIntent{
			ID: value.ID, Signal: value.Signal, Severity: value.Severity, Operator: value.Operator,
			Threshold: value.Threshold, WindowSeconds: value.WindowSeconds, Owner: value.Owner,
			RunbookURL: value.RunbookURL, DeduplicationKey: value.DeduplicationKey,
			MaintenancePolicyRef: value.MaintenancePolicyRef,
		})
	}
	return result
}

func observabilityDashboards(values []config.ObservabilityDashboard) []sdk.DashboardIntent {
	result := make([]sdk.DashboardIntent, 0, len(values))
	for _, value := range values {
		result = append(result, sdk.DashboardIntent{ID: value.ID, Signals: append([]string(nil), value.Signals...), Owner: value.Owner})
	}
	return result
}

func observabilitySLOs(values []config.ObservabilitySLO) []sdk.SLOIntent {
	result := make([]sdk.SLOIntent, 0, len(values))
	for _, value := range values {
		result = append(result, sdk.SLOIntent{
			ID: value.ID, Signal: value.Signal, Target: value.Target, WindowSeconds: value.WindowSeconds,
			Owner: value.Owner, RunbookURL: value.RunbookURL, ErrorBudgetPolicy: value.ErrorBudgetPolicy,
		})
	}
	return result
}

func architectureRuntime(cfg config.Config) string {
	switch {
	case cfg.Target.Provider == "aws" && cfg.Target.Runtime == "eks":
		return "eks"
	case cfg.Target.Provider == "gcp" && (cfg.Target.Runtime == "gke-autopilot" || cfg.Target.Runtime == "gke-standard"):
		return "gke"
	default:
		return cfg.Target.Runtime
	}
}

func architectureComputeMode(cfg config.Config) string {
	switch cfg.Target.Runtime {
	case "ecs-fargate":
		if cfg.Target.AWS != nil {
			if mode := strings.TrimSpace(cfg.Target.AWS.Catalog.Fargate.ComputeMode); mode != "" {
				return mode
			}
		}
		return "fargate"
	case "eks":
		if cfg.Target.AWS != nil {
			if mode := strings.TrimSpace(cfg.Target.AWS.Catalog.EKS.ComputeMode); mode != "" {
				return mode
			}
		}
		return "auto-mode"
	case "gke-autopilot":
		return "autopilot"
	case "gke-standard":
		return "standard"
	case "mks", "kapsule":
		return "managed-node-pools"
	default:
		return cfg.Target.Runtime
	}
}

func architectureKubernetesMode(cfg config.Config) string {
	switch cfg.Target.Runtime {
	case "eks", "mks", "kapsule":
		if len(configuredZones(cfg)) > 1 {
			return "multi-zone"
		}
		return "managed-control-plane"
	case "gke-autopilot", "gke-standard":
		if len(configuredZones(cfg)) > 1 {
			return "regional"
		}
		return "zonal"
	default:
		return ""
	}
}

func architectureNetworkMode(config.Config) string { return "private" }

func architectureNetworkProfile(cfg config.Config) string {
	if cfg.Target.Provider != "aws" || cfg.Target.AWS == nil {
		return "private"
	}
	aws := cfg.Target.AWS
	if aws.Existing.Network != nil {
		return "private-existing-network"
	}
	mode := strings.TrimSpace(aws.NatMode)
	if mode == "" {
		mode = "nat-gateway"
	}
	preset := strings.TrimSpace(cfg.Preset)
	if preset == "" {
		preset = strings.TrimSpace(cfg.Defaults.Preset)
	}
	topology := strings.TrimSpace(aws.NatTopology)
	if topology == "" {
		if preset == "preview" {
			topology = "single-az"
		} else {
			topology = "multi-az"
		}
	}
	replacement := strings.TrimSpace(aws.NatReplacementMode)
	if replacement == "" {
		if mode == "fck-nat" && topology == "multi-az" {
			replacement = "auto-scaling"
		} else {
			replacement = "none"
		}
	}
	if mode == "fck-nat" {
		instanceType := strings.TrimSpace(aws.NatInstanceType)
		if instanceType == "" {
			instanceType = "t4g.nano"
		}
		return "private-" + mode + "-" + topology + "-" + replacement + "-arm64-" + strings.ReplaceAll(instanceType, ".", "-")
	}
	return "private-" + mode + "-" + topology + "-" + replacement
}

func architectureIngressMode(cfg config.Config) string {
	if cfg.Target.Provider == "" {
		return "declared-by-target"
	}
	return "load-balancer"
}

func configuredRegions(cfg config.Config) []string {
	region := configuredRegion(cfg)
	if region == "" {
		return nil
	}
	return []string{region}
}

func configuredZones(cfg config.Config) []string {
	switch cfg.Target.Provider {
	case "aws":
		if cfg.Target.AWS != nil {
			return append([]string(nil), cfg.Target.AWS.AvailabilityZones...)
		}
	case "gcp":
		if cfg.Target.GCP != nil {
			return append([]string(nil), cfg.Target.GCP.Zones...)
		}
	case "ovh":
		if cfg.Target.OVH != nil {
			return append([]string(nil), cfg.Target.OVH.Zones...)
		}
	case "scaleway":
		if cfg.Target.Scaleway != nil {
			zones := append([]string(nil), cfg.Target.Scaleway.Zones...)
			if len(zones) == 0 && cfg.Target.Scaleway.Zone != "" {
				zones = []string{cfg.Target.Scaleway.Zone}
			}
			return zones
		}
	}
	return nil
}

func architectureBoundaries(cfg config.Config) []sdk.ServiceBoundaryIntent {
	failureDomains := configuredZones(cfg)
	if len(failureDomains) == 0 {
		failureDomains = []string{"region"}
	}
	boundary := func(role, family, major, ownership, capability string) sdk.ServiceBoundaryIntent {
		return sdk.ServiceBoundaryIntent{
			Role: role, Family: family, Major: major, Ownership: sdk.ServiceOwnership(ownership),
			FailureDomains: append([]string(nil), failureDomains...), BackupProfile: "declared-by-resilience",
			RecoveryProfile: "declared-by-resilience", CapabilityID: capability,
		}
	}
	var result []sdk.ServiceBoundaryIntent
	switch cfg.Target.Provider {
	case "aws":
		if cfg.Target.AWS == nil {
			return nil
		}
		databaseFamily, databaseCapability := "mysql", "aws.rds.mysql"
		var databaseMajor string
		switch cfg.Target.AWS.Catalog.DatabaseEngine {
		case "aurora-mysql":
			databaseMajor = valueOrDefault(cfg.Target.AWS.Catalog.Versions.AuroraMySQL, "8.4")
		case "rds-mariadb":
			databaseFamily, databaseCapability, databaseMajor = "mariadb", "", valueOrDefault(cfg.Target.AWS.Catalog.Versions.MariaDB, "11")
		default:
			databaseMajor = valueOrDefault(cfg.Target.AWS.Catalog.Versions.MySQL, "8.4")
		}
		result = append(result, boundary("database", databaseFamily, databaseMajor, "managed", databaseCapability))
		result = append(result, boundary("cache", "valkey", valueOrDefault(cfg.Target.AWS.Catalog.Versions.Valkey, "8"), "managed", "aws.elasticache.valkey"))
		if cfg.Target.AWS.Catalog.SearchMode != "disabled" {
			result = append(result, boundary("search", "opensearch", valueOrDefault(cfg.Target.AWS.Catalog.Versions.OpenSearch, "3"), "managed", "aws.opensearch"))
		}
		if cfg.Target.AWS.Catalog.QueueMode == "amazon-mq" {
			result = append(result, boundary("queue", "rabbitmq", valueOrDefault(cfg.Target.AWS.Catalog.Versions.RabbitMQ, "4.2"), "managed", "aws.amazon-mq.rabbitmq"))
		} else if cfg.Target.AWS.Catalog.QueueMode != "db" {
			result = append(result, boundary("queue", "database", "current", "managed", ""))
		}
		result = append(result, boundary("media", "object-storage", "current", "managed", "aws.s3"), boundary("secrets", "secrets-manager", "current", "managed", "aws.secrets-manager"))
	case "gcp":
		if cfg.Target.GCP == nil {
			return nil
		}
		result = append(result, boundary("database", "mysql", "8.4", "managed", "gcp.cloud-sql.mysql"))
		cacheMajor, cacheCapability := gcpValkeyMajor(cfg.Target.GCP.MemorystoreEngineVersion)
		result = append(result, boundary("cache", "valkey", cacheMajor, "managed", cacheCapability))
		if cfg.Target.GCP.OpenSearchImage != "" {
			result = append(result, boundary("search", "opensearch", "3", "self-hosted", ""))
		}
		if cfg.Target.GCP.RabbitMQImage != "" {
			result = append(result, boundary("queue", "rabbitmq", "current", "self-hosted", ""))
		}
		result = append(result, boundary("media", "object-storage", "current", "managed", "gcp.cloud-storage"), boundary("secrets", "secret-manager", "current", "managed", "gcp.secret-manager"))
	case "scaleway":
		redisVersion := "current"
		if cfg.Target.Scaleway != nil && cfg.Target.Scaleway.RedisVersion != "" {
			redisVersion = cfg.Target.Scaleway.RedisVersion
		}
		result = append(result, boundary("database", "mysql", "current", "managed", "scaleway.managed-mysql"), boundary("cache", "redis", redisVersion, "managed", "scaleway.managed-redis"), boundary("search", "search", "current", "self-hosted", ""), boundary("queue", "queue", "current", "self-hosted", ""), boundary("media", "object-storage", "current", "managed", "scaleway.object-storage"), boundary("secrets", "secret-manager", "current", "managed", "scaleway.secret-manager"))
	case "ovh":
		valkeyVersion := "current"
		if cfg.Target.OVH != nil && cfg.Target.OVH.ValkeyVersion != "" {
			valkeyVersion = cfg.Target.OVH.ValkeyVersion
		}
		result = append(result, boundary("database", "mysql", "current", "managed", "ovh.managed-database"), boundary("cache", "valkey", valkeyVersion, "managed", ""), boundary("search", "search", "current", "self-hosted", ""), boundary("queue", "queue", "current", "self-hosted", ""), boundary("media", "object-storage", "current", "managed", "ovh.object-storage"), boundary("secrets", "kms", "current", "managed", "ovh.kms"))
	}
	return result
}

func gcpValkeyMajor(value string) (string, string) {
	switch value {
	case "VALKEY_8_0":
		return "8.0", ""
	case "VALKEY_9_1":
		return "9.1", "gcp.memorystore.valkey-9.1"
	default:
		return "9.0", "gcp.memorystore.valkey-9.0"
	}
}

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func schemaFingerprint() string { return digestString("magelift-config-schema-v1") }

func migrationFingerprint(cfg config.Config) string {
	return digestString(struct {
		Application config.Application
		Build       config.Build
	}{Application: cfg.Application, Build: cfg.Build})
}

func digestString(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "sha256:unavailable"
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func stringSlice(value string) []string {
	if value == "" {
		return nil
	}
	return []string{value}
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func configMap(cfg config.Config) (map[string]any, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func configuredImageDigest(cfg config.Config) string {
	switch cfg.Target.Provider {
	case "aws":
		if cfg.Target.AWS != nil {
			return cfg.Target.AWS.ImageDigest
		}
	case "gcp":
		if cfg.Target.GCP != nil {
			return cfg.Target.GCP.ImageDigest
		}
	case "ovh":
		if cfg.Target.OVH != nil {
			return cfg.Target.OVH.ImageDigest
		}
	case "scaleway":
		if cfg.Target.Scaleway != nil {
			return cfg.Target.Scaleway.ImageDigest
		}
	}
	return ""
}

func configuredRegion(cfg config.Config) string {
	region := cfg.Defaults.Region
	switch cfg.Target.Provider {
	case "gcp":
		if cfg.Target.GCP != nil && cfg.Target.GCP.Region != "" {
			region = cfg.Target.GCP.Region
		}
	case "ovh":
		if cfg.Target.OVH != nil && cfg.Target.OVH.Region != "" {
			region = cfg.Target.OVH.Region
		}
	case "scaleway":
		if cfg.Target.Scaleway != nil && cfg.Target.Scaleway.Region != "" {
			region = cfg.Target.Scaleway.Region
		}
	}
	return region
}

func validatePublicPlan(target sdk.TargetDescriptor, tier sdk.ExtensionCertificationTier, request sdk.ModulePlanRequest, plan sdk.ModulePlan) error {
	var problems []error
	if strings.TrimSpace(plan.StackName) == "" {
		problems = append(problems, errors.New("stack name is required"))
	}
	if plan.Provider != target.Provider || plan.Runtime != target.Runtime || plan.Target != target {
		problems = append(problems, fmt.Errorf("plan target %q/%q does not match registered target %q/%q", plan.Provider, plan.Runtime, target.Provider, target.Runtime))
	}
	if plan.Tier != tier {
		problems = append(problems, fmt.Errorf("plan tier %q does not match registered tier %q", plan.Tier, tier))
	}
	if strings.TrimSpace(plan.Project) == "" {
		problems = append(problems, errors.New("plan project is required"))
	}
	if strings.TrimSpace(plan.Environment) == "" {
		problems = append(problems, errors.New("plan environment is required"))
	}
	if plan.Region != request.Region {
		problems = append(problems, fmt.Errorf("plan region %q does not match request region %q", plan.Region, request.Region))
	}
	if plan.EnvironmentClass != request.EnvironmentClass {
		problems = append(problems, fmt.Errorf("plan environment class %q does not match request %q", plan.EnvironmentClass, request.EnvironmentClass))
	}
	if plan.Protected != request.Protected {
		problems = append(problems, errors.New("plan protection does not match request"))
	}
	if strings.TrimSpace(plan.ImageDigest) != strings.TrimSpace(request.Artifact.ImageDigest) {
		problems = append(problems, fmt.Errorf("plan image digest does not match request"))
	}
	return errors.Join(problems...)
}

func planContext(opts PlanOptions) context.Context {
	return planContextFrom(opts.Context)
}

func planContextFrom(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	return context.Background()
}
