package providerhost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"gopkg.in/yaml.v3"

	"github.com/magelift/magelift/internal/cloud/kube"
	"github.com/magelift/magelift/internal/config"
	deployflow "github.com/magelift/magelift/internal/deploy"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/sdk"
)

// ShimModule is a plugin-backed platform.StackModule for one GCP runtime.
// Planashes through the v2 client; deploy steps run core-side from the
// published deploy inputs; admission runs server-side during Plan (the shim
// deliberately exposes no PlanAdmission).
type ShimModule struct {
	platform.LifecycleFactories
	runtime  sdk.RuntimeID
	client   *Client
	describe *sdk.DescribeResponse
}

var (
	_ platform.StackModule       = (*ShimModule)(nil)
	_ platform.HasBootstrap      = (*ShimModule)(nil)
	_ platform.HasState          = (*ShimModule)(nil)
	_ platform.HasSecrets        = (*ShimModule)(nil)
	_ platform.HasRuntimeObserve = (*ShimModule)(nil)
	_ platform.HasCostEstimator  = (*ShimModule)(nil)
	_ platform.HasRuntimeTunnel  = (*ShimModule)(nil)
)

// NewShimModule binds a negotiated client to one runtime. The client must
// have completed Describe negotiation; the runtime must be advertised.
func NewShimModule(runtime sdk.RuntimeID, client *Client) (*ShimModule, error) {
	if client == nil {
		return nil, errors.New("provider client is required")
	}
	described := client.Describe()
	if described == nil {
		return nil, errors.New("provider client has not negotiated Describe")
	}
	served := false
	for _, advertised := range described.Runtimes {
		if advertised.Runtime == string(runtime) {
			served = true
			break
		}
	}
	if !served {
		return nil, fmt.Errorf("provider %q does not serve runtime %q", described.ProviderID, runtime)
	}
	module := &ShimModule{runtime: runtime, client: client, describe: described}
	module.Edge = func(ctx context.Context, planned platform.PlannedStack) (sdk.EdgeAdapter, error) {
		return NewShimEdgeAdapter(client, planned)
	}
	module.Resilience = func(ctx context.Context, planned platform.PlannedStack) (sdk.ResilienceAdapter, error) {
		return NewShimResilienceAdapter(client, planned)
	}
	return module, nil
}

func (m *ShimModule) Descriptor() sdk.TargetDescriptor {
	runtime := sdk.RuntimeID("gke-autopilot")
	if m != nil && m.runtime != "" {
		runtime = m.runtime
	}
	return sdk.TargetDescriptor{ID: sdk.TargetID("gcp." + string(runtime)), Provider: "gcp", Runtime: runtime}
}

func (m *ShimModule) CertificationTier() platform.CertificationTier {
	if m == nil || m.describe == nil {
		return platform.TierExperimental
	}
	for _, advertised := range m.describe.Runtimes {
		if advertised.Runtime == string(m.runtime) {
			return platformTier(advertised.Tier)
		}
	}
	return platform.TierExperimental
}

func platformTier(tier sdk.ExtensionCertificationTier) platform.CertificationTier {
	if tier == sdk.ExtensionTierCertified {
		return platform.TierCertified
	}
	return platform.TierExperimental
}

// retainedInputs carries everything a re-plan needs. WithImageDigest and
// WithLiveQueueReplicas mutate a copy and re-plan: plans are immutable
// values, never post-hoc mutations.
type retainedInputs struct {
	envelope            sdk.Envelope
	application         sdk.Application
	targetBlock         []byte
	edge                sdk.EdgeIntent
	observability       sdk.ObservabilityIntent
	runtime             string
	liveQueueReplicas   int
	valkeyRequirement   string
	allowUnsupported    bool
	allowExpiredPreview bool
}

func (m *ShimModule) Plan(cfg config.Config, environment string, opts platform.PlanOptions) (platform.PlannedStack, error) {
	if m == nil || m.client == nil {
		return nil, errors.New("provider client is required")
	}
	if cfg.Target.Provider != "gcp" {
		return nil, fmt.Errorf("GCP provider shim cannot plan target %q", cfg.Target.Provider)
	}
	if cfg.Target.Runtime != "" && cfg.Target.Runtime != string(m.runtime) {
		return nil, fmt.Errorf("GCP %s shim cannot plan runtime %q", m.runtime, cfg.Target.Runtime)
	}
	if err := platform.ValidateFirstPartyEdge(cfg); err != nil {
		return nil, err
	}
	if err := platform.ValidateFirstPartyObservability(cfg); err != nil {
		return nil, err
	}
	edge, err := platform.EdgeIntentFromConfig(cfg, environment)
	if err != nil {
		return nil, err
	}
	observability, err := platform.ObservabilityIntentFromConfig(cfg, environment)
	if err != nil {
		return nil, err
	}
	if cfg.Target.GCP == nil {
		return nil, errors.New("target.gcp is required for GCP deployment")
	}
	targetBlock, err := yaml.Marshal(cfg.Target.GCP)
	if err != nil {
		return nil, fmt.Errorf("marshal GCP target block: %w", err)
	}
	requirement, err := config.ServiceRequirementForRelease(cfg.Application.Version, config.CompatibilityCache, "valkey")
	if err != nil {
		return nil, fmt.Errorf("resolve GCP Valkey compatibility: %w", err)
	}
	if len(requirement.Versions) == 0 || strings.TrimSpace(requirement.Versions[0]) == "" {
		return nil, fmt.Errorf("Adobe Valkey requirement for Magento %s has no service major", cfg.Application.Version)
	}
	inputs := retainedInputs{
		envelope:            buildEnvelope(cfg, environment),
		application:         buildApplication(cfg),
		targetBlock:         targetBlock,
		edge:                edge,
		observability:       observability,
		runtime:             string(m.runtime),
		valkeyRequirement:   requirement.Versions[0],
		allowUnsupported:    cfg.Compatibility.AllowUnsupported,
		allowExpiredPreview: opts.AllowExpiredPreview,
	}
	return m.replan(planContext(opts), inputs)
}

func planContext(opts platform.PlanOptions) context.Context {
	if opts.Context != nil {
		return opts.Context
	}
	return context.Background()
}

func (m *ShimModule) replan(ctx context.Context, inputs retainedInputs) (*ShimPlanned, error) {
	resp, err := Call[sdk.PlanRequest, sdk.PlanResult](ctx, m.client, sdk.OpPlan, &sdk.PlanRequest{
		ProtocolVersion:     sdk.ProtocolV1,
		Envelope:            inputs.envelope,
		Application:         inputs.application,
		Edge:                inputs.edge,
		Observability:       inputs.observability,
		TargetBlock:         inputs.targetBlock,
		Runtime:             inputs.runtime,
		LiveQueueReplicas:   inputs.liveQueueReplicas,
		ValkeyRequirement:   inputs.valkeyRequirement,
		AllowUnsupported:    inputs.allowUnsupported,
		AllowExpiredPreview: inputs.allowExpiredPreview,
	})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, AsPluginError(sdk.OpPlan, resp.Error)
	}
	return &ShimPlanned{module: m, stored: resp.Plan, inputs: inputs}, nil
}

func buildEnvelope(cfg config.Config, environment string) sdk.Envelope {
	region := cfg.Defaults.Region
	preset := cfg.Defaults.Preset
	if cfg.Target.GCP != nil && strings.TrimSpace(cfg.Target.GCP.Region) != "" {
		region = cfg.Target.GCP.Region
	}
	if strings.TrimSpace(cfg.Preset) != "" {
		preset = cfg.Preset
	}
	return sdk.Envelope{
		Project:            cfg.Project.Name,
		Environment:        environment,
		Region:             region,
		EnvironmentClass:   cfg.Class,
		StackName:          platform.FormatStackName(cfg.Project.Name, environment, "gcp", sdk.RuntimeID(cfg.Target.Runtime)),
		Preset:             preset,
		MonthlyBudgetCents: cfg.MonthlyBudgetCents,
		AppVersion:         cfg.Application.Version,
		ExpiresAt:          cfg.ExpiresAt,
		Protected:          cfg.Protection,
		Domain:             cfg.Domain,
	}
}

func buildApplication(cfg config.Config) sdk.Application {
	magento := cfg.Application.Magento
	return sdk.Application{
		Edition:    cfg.Application.Edition,
		Version:    cfg.Application.Version,
		Mode:       cfg.Application.Mode,
		WebRuntime: cfg.Application.WebRuntime,
		Magento: sdk.MagentoSettings{
			FrontName:        magento.FrontName,
			CookieDomain:     magento.CookieDomain,
			UnsecureBaseURL:  magento.UnsecureBaseURL,
			SecureBaseURL:    magento.SecureBaseURL,
			CORSOrigins:      append([]string(nil), magento.CORSOrigins...),
			StorefrontOrigin: magento.StorefrontOrigin,
			ConsumersMode:    magento.Consumers.Mode,
			ConsumerNames:    append([]string(nil), magento.Consumers.Names...),
			QueueTransport:   magento.QueueTransport,
			QueueModule:      magento.QueueModule,
			Variables:        appendMap(magento.Variables),
		},
	}
}

func appendMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

// Program always fails: GCP programs execute inside the provider plugin.
// The CLI builds a plugin-backed automation backend instead of a core
// Pulumi program for GCP targets.
func (m *ShimModule) Program(platform.PlannedStack) (pulumi.RunFunc, error) {
	return nil, errors.New("GCP programs execute inside the provider plugin; use the plugin automation backend")
}

func (m *ShimModule) OutputKeys() []string {
	if m == nil || m.describe == nil {
		return nil
	}
	return append([]string(nil), m.describe.OutputKeys...)
}

func (m *ShimModule) Ops() platform.Ops {
	if m == nil {
		return ShimOps{}
	}
	return ShimOps{client: m.client}
}

func (m *ShimModule) Bootstrap() platform.Bootstrap {
	if m == nil {
		return ShimBootstrap{}
	}
	return ShimBootstrap{client: m.client}
}

func (m *ShimModule) State() platform.State {
	if m == nil {
		return ShimState{}
	}
	return ShimState{client: m.client}
}

func (m *ShimModule) Secrets() platform.Secrets {
	if m == nil {
		return ShimSecrets{}
	}
	return ShimSecrets{client: m.client}
}

func (m *ShimModule) RuntimeObserve() platform.RuntimeObserve {
	if m == nil {
		return &ShimObserve{}
	}
	return &ShimObserve{client: m.client}
}

func (m *ShimModule) CostEstimator() platform.CostEstimator {
	if m == nil {
		return ShimCostEstimator{}
	}
	return ShimCostEstimator{client: m.client}
}

func (m *ShimModule) RuntimeTunnel() platform.RuntimeTunnel {
	if m == nil {
		return ShimTunnel{}
	}
	return ShimTunnel{client: m.client}
}

// ShimPlanned is a plugin-backed planned stack. The stored plan is opaque;
// plan identity fields come from the retained envelope.
type ShimPlanned struct {
	module *ShimModule
	stored sdk.StoredPlan
	inputs retainedInputs
}

var (
	_ platform.PlannedStack           = (*ShimPlanned)(nil)
	_ platform.LiveQueueReplicaBinder = (*ShimPlanned)(nil)
)

// AsShimPlanned extracts the plugin-backed plan when present.
func AsShimPlanned(planned platform.PlannedStack) (*ShimPlanned, bool) {
	value, ok := planned.(*ShimPlanned)
	return value, ok
}

func (p *ShimPlanned) StackName() string        { return p.stored.StackName }
func (p *ShimPlanned) Provider() sdk.ProviderID { return "gcp" }
func (p *ShimPlanned) Runtime() sdk.RuntimeID   { return sdk.RuntimeID(p.stored.Runtime) }
func (p *ShimPlanned) Project() string          { return p.inputs.envelope.Project }
func (p *ShimPlanned) Environment() string      { return p.inputs.envelope.Environment }
func (p *ShimPlanned) Region() string           { return p.inputs.envelope.Region }
func (p *ShimPlanned) CertificationTier() platform.CertificationTier {
	return platformTier(p.stored.Tier)
}
func (p *ShimPlanned) EnvironmentClass() string { return p.inputs.envelope.EnvironmentClass }
func (p *ShimPlanned) Protected() bool          { return p.inputs.envelope.Protected }
func (p *ShimPlanned) ImageDigest() string      { return p.stored.ImageDigest }
func (p *ShimPlanned) TargetDescriptor() sdk.TargetDescriptor {
	runtime := p.Runtime()
	return sdk.TargetDescriptor{ID: sdk.TargetID("gcp." + string(runtime)), Provider: "gcp", Runtime: runtime}
}

// StateBackendURL reports the plan-bound backend URL for CLI backend wiring.
func (p *ShimPlanned) StateBackendURL() string { return p.stored.StateBackendURL }

// WithImageDigest re-plans with a new image digest in the target block.
func (p *ShimPlanned) WithImageDigest(digest string) (platform.PlannedStack, error) {
	if p == nil || p.module == nil {
		return nil, errors.New("shim plan has no module")
	}
	var target config.GCPTarget
	if err := yaml.Unmarshal(p.inputs.targetBlock, &target); err != nil {
		return nil, fmt.Errorf("decode retained GCP target block: %w", err)
	}
	target.ImageDigest = digest
	block, err := yaml.Marshal(&target)
	if err != nil {
		return nil, fmt.Errorf("marshal GCP target block: %w", err)
	}
	next := p.inputs
	next.targetBlock = block
	return p.module.replan(context.Background(), next)
}

// WithLiveQueueReplicas re-plans with the observed live replica count so
// validation keeps refusing in-place RabbitMQ topology changes.
func (p *ShimPlanned) WithLiveQueueReplicas(replicas int) (platform.PlannedStack, error) {
	if p == nil || p.module == nil {
		return nil, errors.New("shim plan has no module")
	}
	next := p.inputs
	next.liveQueueReplicas = replicas
	return p.module.replan(context.Background(), next)
}

// ShimOps runs Magento deploy steps core-side from published deploy inputs.
type ShimOps struct {
	client        *Client
	NewCandidate  func(context.Context, kube.Backend) (kube.CandidateRunner, error)
	NewRuntime    func(context.Context, kube.Backend) (kube.RuntimeChecker, error)
	RecordRelease func(context.Context, deployflow.Request, deployflow.Result) error
}

var _ platform.Ops = ShimOps{}

// WithRecordRelease implements platform.HasRecordRelease.
func (o ShimOps) WithRecordRelease(fn func(context.Context, deployflow.Request, deployflow.Result) error) platform.Ops {
	o.RecordRelease = fn
	return o
}

func (o ShimOps) AcquireLock(ctx context.Context, planned platform.PlannedStack) (func(context.Context) error, error) {
	shim, ok := AsShimPlanned(planned)
	if !ok {
		return nil, fmt.Errorf("GCP ops received unexpected planned type %T", planned)
	}
	if o.client == nil {
		return nil, errors.New("provider client is required")
	}
	host, _ := os.Hostname()
	owner := fmt.Sprintf("magelift-cli-%s-%d", host, os.Getpid())
	resp, err := Call[sdk.StateLockCall, sdk.StateLockResult](ctx, o.client, sdk.OpStateLock, &sdk.StateLockCall{
		ProtocolVersion: sdk.ProtocolV1, Envelope: shim.inputs.envelope, Plan: shim.stored, Owner: owner,
	})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, AsPluginError(sdk.OpStateLock, resp.Error)
	}
	return func(releaseCtx context.Context) error {
		unlock, err := Call[sdk.StateUnlockCall, sdk.StateUnlockResult](releaseCtx, o.client, sdk.OpStateUnlock, &sdk.StateUnlockCall{
			ProtocolVersion: sdk.ProtocolV1, Envelope: shim.inputs.envelope, Plan: shim.stored,
		})
		if err != nil {
			return err
		}
		if unlock.Error != nil {
			return AsPluginError(sdk.OpStateUnlock, unlock.Error)
		}
		return nil
	}, nil
}

func (o ShimOps) NewDeploySteps(ctx context.Context, backend any, planned platform.PlannedStack, diagnostics io.Writer) (deployflow.Steps, error) {
	shim, ok := AsShimPlanned(planned)
	if !ok {
		return nil, fmt.Errorf("GCP ops received unexpected planned type %T", planned)
	}
	typed, ok := backend.(kube.Backend)
	if !ok {
		return nil, fmt.Errorf("GCP deploy steps require an infrastructure backend with outputs, got %T", backend)
	}
	var deploySpec kube.DeploySpec
	if err := json.Unmarshal(shim.stored.DeployInputsJSON, &deploySpec); err != nil {
		return nil, fmt.Errorf("decode GCP deploy inputs: %w", err)
	}
	newCandidate := o.NewCandidate
	if newCandidate == nil {
		newCandidate = func(context.Context, kube.Backend) (kube.CandidateRunner, error) {
			return kube.NewCandidateFromFactory(kube.ClientFromOutputs), nil
		}
	}
	newRuntime := o.NewRuntime
	if newRuntime == nil {
		newRuntime = func(_ context.Context, b kube.Backend) (kube.RuntimeChecker, error) {
			return kube.NewRuntimeFromFactory(b, kube.ClientFromOutputs)
		}
	}
	candidate, err := newCandidate(ctx, typed)
	if err != nil {
		return nil, err
	}
	runtime, err := newRuntime(ctx, typed)
	if err != nil {
		return nil, err
	}
	return kube.New(typed, deploySpec, candidate, runtime, diagnostics, o.RecordRelease)
}
