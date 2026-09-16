package platform

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/magelift/magelift/sdk"
)

// BootstrapRequest is Magento-shaped account preparation input.
// Provider product fields stay inside adapters.
type BootstrapRequest struct {
	AccessLogBucket string
	GitHubOwner     string
	GitHubRepo      string
}

// GitHubIdentity returns the GitHub Actions identity Magelift should create.
// Laptop deploys omit both flags. CI must set both.
func (r BootstrapRequest) GitHubIdentity() (owner, repo string, requested bool, err error) {
	owner = strings.TrimSpace(r.GitHubOwner)
	repo = strings.TrimSpace(r.GitHubRepo)
	switch {
	case owner == "" && repo == "":
		return "", "", false, nil
	case owner == "" || repo == "":
		return "", "", false, errors.New("set both --github-owner and --github-repo for GitHub Actions, or omit both")
	default:
		return owner, repo, true, nil
	}
}

// BootstrapResult is opaque DIY-backend metadata for CLI display.
type BootstrapResult struct {
	BackendURL string         `json:"backendURL" yaml:"backendURL"`
	KeyRef     string         `json:"keyRef,omitempty" yaml:"keyRef,omitempty"`
	Details    map[string]any `json:"details,omitempty" yaml:"details,omitempty"`
}

// Bootstrap prepares the Pulumi DIY backend and optional CI identity.
type Bootstrap interface {
	VerifyAccount(ctx context.Context, planned PlannedStack) error
	Ensure(ctx context.Context, planned PlannedStack, req BootstrapRequest) (BootstrapResult, error)
}

// HasBootstrap is implemented by StackModules that own account bootstrap.
type HasBootstrap interface {
	Bootstrap() Bootstrap
}

// ModuleBootstrap returns Bootstrap when the module implements it.
func ModuleBootstrap(module StackModule) Bootstrap {
	if module == nil {
		return nil
	}
	if provider, ok := module.(HasBootstrap); ok {
		return provider.Bootstrap()
	}
	return nil
}

// LockInfo describes a DIY deployment lock.
type LockInfo struct {
	Project     string    `json:"project" yaml:"project"`
	Environment string    `json:"environment" yaml:"environment"`
	Owner       string    `json:"owner" yaml:"owner"`
	AcquiredAt  time.Time `json:"acquiredAt" yaml:"acquiredAt"`
}

// BackupResult is opaque backup metadata.
type BackupResult struct {
	ID             string `json:"id,omitempty" yaml:"id,omitempty"`
	Location       string `json:"location" yaml:"location"`
	ETag           string `json:"etag,omitempty" yaml:"etag,omitempty"`
	Objects        int    `json:"objects,omitempty" yaml:"objects,omitempty"`
	Bytes          int64  `json:"bytes,omitempty" yaml:"bytes,omitempty"`
	ManifestDigest string `json:"manifestDigest,omitempty" yaml:"manifestDigest,omitempty"`
}

// RestoreResult is opaque restore metadata.
type RestoreResult struct {
	ID             string `json:"id,omitempty" yaml:"id,omitempty"`
	Location       string `json:"location" yaml:"location"`
	Objects        int    `json:"objects,omitempty" yaml:"objects,omitempty"`
	Bytes          int64  `json:"bytes,omitempty" yaml:"bytes,omitempty"`
	ManifestDigest string `json:"manifestDigest,omitempty" yaml:"manifestDigest,omitempty"`
}

// State manages DIY Pulumi backend locks and snapshots for an environment.
type State interface {
	Status(ctx context.Context, planned PlannedStack) (locked bool, info *LockInfo, backend string, err error)
	Lock(ctx context.Context, planned PlannedStack, owner string) (release func(context.Context) error, err error)
	Unlock(ctx context.Context, planned PlannedStack) (*LockInfo, error)
	Backup(ctx context.Context, planned PlannedStack) (BackupResult, error)
	Restore(ctx context.Context, planned PlannedStack, location string) (RestoreResult, error)
}

// HasState is implemented by StackModules that expose DIY state ops.
type HasState interface {
	State() State
}

// ModuleState returns State when the module implements it.
func ModuleState(module StackModule) State {
	if module == nil {
		return nil
	}
	if provider, ok := module.(HasState); ok {
		return provider.State()
	}
	return nil
}

// SecretMeta is a Magento application secret listing entry.
type SecretMeta struct {
	Name      string    `json:"name" yaml:"name"`
	UpdatedAt time.Time `json:"updatedAt,omitempty" yaml:"updatedAt,omitempty"`
}

// Secrets manages Magento application secrets (not Pulumi config).
type Secrets interface {
	List(ctx context.Context, planned PlannedStack) ([]SecretMeta, error)
	Set(ctx context.Context, planned PlannedStack, name string, value []byte) error
	Remove(ctx context.Context, planned PlannedStack, name string) error
}

// HasSecrets is implemented by StackModules that expose Secrets.
type HasSecrets interface {
	Secrets() Secrets
}

// ModuleSecrets returns Secrets when the module implements it.
func ModuleSecrets(module StackModule) Secrets {
	if module == nil {
		return nil
	}
	if provider, ok := module.(HasSecrets); ok {
		return provider.Secrets()
	}
	return nil
}

// ResilienceAdapter is the optional provider-neutral recovery port. Provider
// modules implement the SDK contract; the lifecycle core remains responsible
// for policy validation, scheduling, ownership, and evidence gates.
type ResilienceAdapter interface {
	Resilience() sdk.ResilienceAdapter
}

// ModuleResilience returns a module's recovery adapter when it exposes one.
// Absence is explicit: a target without this port cannot claim automated
// backup/restore or DR certification.
func ModuleResilience(module StackModule) sdk.ResilienceAdapter {
	if module == nil {
		return nil
	}
	if provider, ok := module.(ResilienceAdapter); ok {
		return provider.Resilience()
	}
	return nil
}

// EdgeAdapter is the optional public SDK edge lifecycle port. Provider
// modules implement it when they own native or external edge resources.
type EdgeAdapter interface {
	Edge() sdk.EdgeAdapter
}

// ModuleEdge returns an edge adapter when the module exposes one. Absence is
// explicit: an adapter without this port cannot claim automated edge lifecycle
// or cleanup certification.
func ModuleEdge(module StackModule) sdk.EdgeAdapter {
	if module == nil {
		return nil
	}
	if provider, ok := module.(EdgeAdapter); ok {
		return provider.Edge()
	}
	return nil
}

// ObservabilityAdapter is the optional public SDK telemetry lifecycle port.
type ObservabilityAdapter interface {
	Observability() sdk.ObservabilityAdapter
}

// ModuleObservability returns a telemetry adapter when the module exposes
// one. A configured exporter without this port remains unverified telemetry.
func ModuleObservability(module StackModule) sdk.ObservabilityAdapter {
	if module == nil {
		return nil
	}
	if provider, ok := module.(ObservabilityAdapter); ok {
		return provider.Observability()
	}
	return nil
}

// CertificationAdapter is the optional provider-neutral execution port for
// certification units. It is intentionally separate from deployment and from
// resilience/edge/observability lifecycle ports.
type CertificationAdapter interface {
	Certification() sdk.CertificationAdapter
}

// ModuleCertification returns a statically constructed certification adapter
// when a module exposes one. A nil result is explicit and cannot be promoted
// to certification support by the deployment graph alone.
func ModuleCertification(module StackModule) sdk.CertificationAdapter {
	if module == nil {
		return nil
	}
	if provider, ok := module.(CertificationAdapter); ok {
		return provider.Certification()
	}
	return nil
}

// CertificationAdmissionAdapter is the optional provider-neutral read-only
// admission port. It stays separate from certification execution so a
// provider can prove account, quota, network, state, fixture, and ownership
// facts before it exposes a paid mutation client.
type CertificationAdmissionAdapter interface {
	CertificationAdmission() sdk.CertificationAdmissionAdapter
}

// ModuleCertificationAdmission returns a statically constructed admission
// adapter when a module exposes one.
func ModuleCertificationAdmission(module StackModule) sdk.CertificationAdmissionAdapter {
	if module == nil {
		return nil
	}
	if provider, ok := module.(CertificationAdmissionAdapter); ok {
		return provider.CertificationAdmission()
	}
	return nil
}

// CollectorDeploymentAdapter is the optional workload collector lifecycle
// port associated with observability. It remains separate from signal export
// because ECS task definitions and Kubernetes releases have their own
// readiness, rollback, and owning-service inventory boundaries.
type CollectorDeploymentAdapter interface {
	CollectorDeployment() sdk.CollectorDeploymentAdapter
}

// ModuleCollectorDeployment returns an injected collector lifecycle adapter
// when the module exposes one. Absence is explicit and cannot be treated as
// collector delivery certification.
func ModuleCollectorDeployment(module StackModule) sdk.CollectorDeploymentAdapter {
	if module == nil {
		return nil
	}
	if provider, ok := module.(CollectorDeploymentAdapter); ok {
		return provider.CollectorDeployment()
	}
	return nil
}

// CollectorDeploymentFactory is the planned internal bridge for provider
// collector deployment clients whose ownership or credentials depend on the
// resolved target. Construction must not mutate provider state.
type CollectorDeploymentFactory interface {
	NewCollectorDeployment(context.Context, PlannedStack) (sdk.CollectorDeploymentAdapter, error)
}

// ResilienceFactory is the planned internal recovery port. It lets a
// provider construct its SDK-backed adapter from the validated planned stack
// without putting provider SDK types in PlannedStack or in the core.
type ResilienceFactory interface {
	NewResilience(context.Context, PlannedStack) (sdk.ResilienceAdapter, error)
}

// EdgeFactory is the planned internal edge lifecycle port.
type EdgeFactory interface {
	NewEdge(context.Context, PlannedStack) (sdk.EdgeAdapter, error)
}

// ObservabilityFactory is the planned internal telemetry lifecycle port.
type ObservabilityFactory interface {
	NewObservability(context.Context, PlannedStack) (sdk.ObservabilityAdapter, error)
}

// CertificationFactory is the planned internal bridge for provider modules
// whose certification client depends on the resolved target identity.
type CertificationFactory interface {
	NewCertification(context.Context, PlannedStack) (sdk.CertificationAdapter, error)
}

// CertificationAdmissionFactory is the planned internal bridge for
// provider-owned read-only admission clients.
type CertificationAdmissionFactory interface {
	NewCertificationAdmission(context.Context, PlannedStack) (sdk.CertificationAdmissionAdapter, error)
}

// LifecycleFactories is the small dependency-injection seam used by
// first-party modules. Provider packages may fill only the ports they own;
// the core remains responsible for resolving a planned module and validating
// every returned SDK descriptor. Functions must construct clients/adapters
// without mutating provider state.
type LifecycleFactories struct {
	Resilience             func(context.Context, PlannedStack) (sdk.ResilienceAdapter, error)
	Edge                   func(context.Context, PlannedStack) (sdk.EdgeAdapter, error)
	Observability          func(context.Context, PlannedStack) (sdk.ObservabilityAdapter, error)
	CollectorDeployment    func(context.Context, PlannedStack) (sdk.CollectorDeploymentAdapter, error)
	Certification          func(context.Context, PlannedStack) (sdk.CertificationAdapter, error)
	Admission              func(context.Context, PlannedStack) (PlanAdmission, error)
	CertificationAdmission func(context.Context, PlannedStack) (sdk.CertificationAdmissionAdapter, error)
}

func (f LifecycleFactories) NewResilience(ctx context.Context, planned PlannedStack) (sdk.ResilienceAdapter, error) {
	if f.Resilience == nil {
		return nil, nil
	}
	return f.Resilience(ctx, planned)
}

func (f LifecycleFactories) NewEdge(ctx context.Context, planned PlannedStack) (sdk.EdgeAdapter, error) {
	if f.Edge == nil {
		return nil, nil
	}
	return f.Edge(ctx, planned)
}

func (f LifecycleFactories) NewObservability(ctx context.Context, planned PlannedStack) (sdk.ObservabilityAdapter, error) {
	if f.Observability == nil {
		return nil, nil
	}
	return f.Observability(ctx, planned)
}

func (f LifecycleFactories) NewCollectorDeployment(ctx context.Context, planned PlannedStack) (sdk.CollectorDeploymentAdapter, error) {
	if f.CollectorDeployment == nil {
		return nil, nil
	}
	return f.CollectorDeployment(ctx, planned)
}

func (f LifecycleFactories) NewCertification(ctx context.Context, planned PlannedStack) (sdk.CertificationAdapter, error) {
	if f.Certification == nil {
		return nil, nil
	}
	return f.Certification(ctx, planned)
}

func (f LifecycleFactories) NewPlanAdmission(ctx context.Context, planned PlannedStack) (PlanAdmission, error) {
	if f.Admission == nil {
		return nil, nil
	}
	return f.Admission(ctx, planned)
}

func (f LifecycleFactories) NewCertificationAdmission(ctx context.Context, planned PlannedStack) (sdk.CertificationAdmissionAdapter, error) {
	if f.CertificationAdmission == nil {
		return nil, nil
	}
	return f.CertificationAdmission(ctx, planned)
}

// ModuleResilienceFor resolves a recovery adapter after planning. The
// provider factory is preferred because it can use the concrete planned
// identity to construct provider SDK clients. The static port remains a
// compatibility path for modules whose adapter is already fully constructed.
func ModuleResilienceFor(ctx context.Context, module StackModule, planned PlannedStack) (sdk.ResilienceAdapter, error) {
	if err := validatePlannedModule(ctx, module, planned); err != nil {
		return nil, err
	}
	if factory, ok := module.(ResilienceFactory); ok {
		adapter, err := factory.NewResilience(ctx, planned)
		if err != nil {
			return nil, fmt.Errorf("create resilience adapter for %q: %w", module.Descriptor().ID, err)
		}
		if adapter == nil {
			adapter = ModuleResilience(module)
			if adapter == nil {
				return nil, nil
			}
		}
		if err := validatePlannedResilienceAdapter(module, planned, adapter); err != nil {
			return nil, err
		}
		return adapter, nil
	}
	adapter := ModuleResilience(module)
	if adapter == nil {
		return nil, nil
	}
	if err := validatePlannedResilienceAdapter(module, planned, adapter); err != nil {
		return nil, err
	}
	return adapter, nil
}

// ModuleEdgeFor resolves a planned edge adapter through the same optional
// factory/static fallback used by recovery.
func ModuleEdgeFor(ctx context.Context, module StackModule, planned PlannedStack) (sdk.EdgeAdapter, error) {
	if err := validatePlannedModule(ctx, module, planned); err != nil {
		return nil, err
	}
	if factory, ok := module.(EdgeFactory); ok {
		adapter, err := factory.NewEdge(ctx, planned)
		if err != nil {
			return nil, fmt.Errorf("create edge adapter for %q: %w", module.Descriptor().ID, err)
		}
		if adapter == nil {
			adapter = ModuleEdge(module)
			if adapter == nil {
				return nil, nil
			}
		}
		if err := validatePlannedEdgeAdapter(module, planned, adapter); err != nil {
			return nil, err
		}
		return adapter, nil
	}
	adapter := ModuleEdge(module)
	if adapter == nil {
		return nil, nil
	}
	if err := validatePlannedEdgeAdapter(module, planned, adapter); err != nil {
		return nil, err
	}
	return adapter, nil
}

// ModuleObservabilityFor resolves a planned telemetry adapter through the
// optional factory/static fallback.
func ModuleObservabilityFor(ctx context.Context, module StackModule, planned PlannedStack) (sdk.ObservabilityAdapter, error) {
	if err := validatePlannedModule(ctx, module, planned); err != nil {
		return nil, err
	}
	if factory, ok := module.(ObservabilityFactory); ok {
		adapter, err := factory.NewObservability(ctx, planned)
		if err != nil {
			return nil, fmt.Errorf("create observability adapter for %q: %w", module.Descriptor().ID, err)
		}
		if adapter == nil {
			adapter = ModuleObservability(module)
			if adapter == nil {
				return nil, nil
			}
		}
		if err := validatePlannedObservabilityAdapter(module, planned, adapter); err != nil {
			return nil, err
		}
		return adapter, nil
	}
	adapter := ModuleObservability(module)
	if adapter == nil {
		return nil, nil
	}
	if err := validatePlannedObservabilityAdapter(module, planned, adapter); err != nil {
		return nil, err
	}
	return adapter, nil
}

// ModuleCollectorDeploymentFor resolves a planned collector deployment
// adapter through the optional provider factory, falling back to a static
// adapter only when it is safe to construct at module registration time.
func ModuleCollectorDeploymentFor(ctx context.Context, module StackModule, planned PlannedStack) (sdk.CollectorDeploymentAdapter, error) {
	if err := validatePlannedModule(ctx, module, planned); err != nil {
		return nil, err
	}
	if factory, ok := module.(CollectorDeploymentFactory); ok {
		adapter, err := factory.NewCollectorDeployment(ctx, planned)
		if err != nil {
			return nil, fmt.Errorf("create collector deployment adapter for %q: %w", module.Descriptor().ID, err)
		}
		if adapter == nil {
			adapter = ModuleCollectorDeployment(module)
		}
		return adapter, nil
	}
	return ModuleCollectorDeployment(module), nil
}

// ModuleCertificationFor resolves a provider certification adapter after the
// target has been planned. The provider factory is preferred; a static module
// adapter is the fallback for clients that are safe to construct at registry
// time. The returned descriptor is checked against both the module and the
// planned target before it can execute a unit.
func ModuleCertificationFor(ctx context.Context, module StackModule, planned PlannedStack) (sdk.CertificationAdapter, error) {
	if err := validatePlannedModule(ctx, module, planned); err != nil {
		return nil, err
	}
	if factory, ok := module.(CertificationFactory); ok {
		adapter, err := factory.NewCertification(ctx, planned)
		if err != nil {
			return nil, fmt.Errorf("create certification adapter for %q: %w", module.Descriptor().ID, err)
		}
		if adapter == nil {
			adapter = ModuleCertification(module)
			if adapter == nil {
				return nil, nil
			}
		}
		if err := validatePlannedCertificationAdapter(module, planned, adapter); err != nil {
			return nil, err
		}
		return adapter, nil
	}
	adapter := ModuleCertification(module)
	if adapter == nil {
		return nil, nil
	}
	if err := validatePlannedCertificationAdapter(module, planned, adapter); err != nil {
		return nil, err
	}
	return adapter, nil
}

// ModuleCertificationAdmissionFor resolves a provider admission adapter
// after planning. The provider factory is preferred; a static adapter is the
// fallback for clients that are safe to construct before planning.
func ModuleCertificationAdmissionFor(ctx context.Context, module StackModule, planned PlannedStack) (sdk.CertificationAdmissionAdapter, error) {
	if err := validatePlannedModule(ctx, module, planned); err != nil {
		return nil, err
	}
	if factory, ok := module.(CertificationAdmissionFactory); ok {
		adapter, err := factory.NewCertificationAdmission(ctx, planned)
		if err != nil {
			return nil, fmt.Errorf("create certification admission adapter for %q: %w", module.Descriptor().ID, err)
		}
		if adapter == nil {
			adapter = ModuleCertificationAdmission(module)
			if adapter == nil {
				return nil, nil
			}
		}
		if err := validatePlannedCertificationAdmissionAdapter(module, planned, adapter); err != nil {
			return nil, err
		}
		return adapter, nil
	}
	adapter := ModuleCertificationAdmission(module)
	if adapter == nil {
		return nil, nil
	}
	if err := validatePlannedCertificationAdmissionAdapter(module, planned, adapter); err != nil {
		return nil, err
	}
	return adapter, nil
}

func validatePlannedModule(ctx context.Context, module StackModule, planned PlannedStack) error {
	if ctx == nil {
		return fmt.Errorf("lifecycle adapter context is required")
	}
	if module == nil {
		return fmt.Errorf("stack module is required")
	}
	if planned == nil {
		return fmt.Errorf("planned stack is required")
	}
	descriptor := module.Descriptor()
	plannedTarget := planned.TargetDescriptor()
	if plannedTarget != descriptor {
		return fmt.Errorf("planned target %q/%q/%q does not match stack module %q/%q/%q", plannedTarget.Provider, plannedTarget.Runtime, plannedTarget.ID, descriptor.Provider, descriptor.Runtime, descriptor.ID)
	}
	return nil
}

func validatePlannedResilienceAdapter(module StackModule, planned PlannedStack, adapter sdk.ResilienceAdapter) error {
	descriptor := adapter.ResilienceDescriptor()
	if err := sdk.ValidateResilienceAdapterDescriptor(descriptor); err != nil {
		return fmt.Errorf("validate planned resilience adapter for %q: %w", module.Descriptor().ID, err)
	}
	if descriptor.Provider != module.Descriptor().Provider || descriptor.Provider != planned.Provider() {
		return fmt.Errorf("planned resilience adapter provider %q does not match target provider %q", descriptor.Provider, planned.Provider())
	}
	return nil
}

func validatePlannedEdgeAdapter(module StackModule, planned PlannedStack, adapter sdk.EdgeAdapter) error {
	descriptor := adapter.EdgeDescriptor()
	if err := sdk.ValidateEdgeAdapterDescriptor(descriptor); err != nil {
		return fmt.Errorf("validate planned edge adapter for %q: %w", module.Descriptor().ID, err)
	}
	if descriptor.Provider != module.Descriptor().Provider || descriptor.Provider != planned.Provider() {
		return fmt.Errorf("planned edge adapter provider %q does not match target provider %q", descriptor.Provider, planned.Provider())
	}
	return nil
}

func validatePlannedObservabilityAdapter(module StackModule, planned PlannedStack, adapter sdk.ObservabilityAdapter) error {
	descriptor := adapter.ObservabilityDescriptor()
	if err := sdk.ValidateObservabilityAdapterDescriptor(descriptor); err != nil {
		return fmt.Errorf("validate planned observability adapter for %q: %w", module.Descriptor().ID, err)
	}
	if descriptor.Provider != module.Descriptor().Provider || descriptor.Provider != planned.Provider() {
		return fmt.Errorf("planned observability adapter provider %q does not match target provider %q", descriptor.Provider, planned.Provider())
	}
	return nil
}

func validatePlannedCertificationAdapter(module StackModule, planned PlannedStack, adapter sdk.CertificationAdapter) error {
	if adapter == nil {
		return errors.New("planned certification adapter is required")
	}
	descriptor := adapter.CertificationDescriptor()
	if err := sdk.ValidateCertificationAdapterDescriptor(descriptor); err != nil {
		return fmt.Errorf("validate planned certification adapter for %q: %w", module.Descriptor().ID, err)
	}
	if descriptor.Provider != module.Descriptor().Provider || descriptor.Provider != planned.Provider() {
		return fmt.Errorf("planned certification adapter provider %q does not match target provider %q", descriptor.Provider, planned.Provider())
	}
	if descriptor.Runtime != module.Descriptor().Runtime || descriptor.Runtime != planned.Runtime() {
		return fmt.Errorf("planned certification adapter runtime %q does not match target runtime %q", descriptor.Runtime, planned.Runtime())
	}
	return nil
}

func validatePlannedCertificationAdmissionAdapter(module StackModule, planned PlannedStack, adapter sdk.CertificationAdmissionAdapter) error {
	if adapter == nil {
		return errors.New("planned certification admission adapter is required")
	}
	descriptor := adapter.CertificationAdmissionDescriptor()
	if err := sdk.ValidateCertificationAdmissionAdapterDescriptor(descriptor); err != nil {
		return fmt.Errorf("validate planned certification admission adapter for %q: %w", module.Descriptor().ID, err)
	}
	if descriptor.Provider != module.Descriptor().Provider || descriptor.Provider != planned.Provider() {
		return fmt.Errorf("planned certification admission adapter provider %q does not match target provider %q", descriptor.Provider, planned.Provider())
	}
	if descriptor.Runtime != module.Descriptor().Runtime || descriptor.Runtime != planned.Runtime() {
		return fmt.Errorf("planned certification admission adapter runtime %q does not match target runtime %q", descriptor.Runtime, planned.Runtime())
	}
	return nil
}
