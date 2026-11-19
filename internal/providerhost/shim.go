package providerhost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"gopkg.in/yaml.v3"

	"github.com/magelift/magelift/internal/automation"
	"github.com/magelift/magelift/internal/cloud/kube"
	"github.com/magelift/magelift/internal/config"
	deployflow "github.com/magelift/magelift/internal/deploy"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/sdk"
)

// ShimModule sends plan and deployment requests to the provider that owns the runtime.
// The provider validates the request when it builds the plan.
type ShimModule struct {
	platform.LifecycleFactories
	provider   sdk.ProviderID
	runtime    sdk.RuntimeID
	tier       platform.CertificationTier
	outputKeys []string
	client     *Client
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

// NewLazyShimModule binds one runtime to a dialer. Registration spawns no
// processes and needs no credentials; the first fallible use dials once
// via the shared lazy client.
func NewLazyShimModule(runtime sdk.RuntimeID, dial DialFunc) (*ShimModule, error) {
	return NewLazyProviderShimModule("gcp", runtime, platformTierForRuntime(runtime), gcpOutputKeys(), dial)
}

// NewShimModule binds a client to one runtime. Tier and output keys are
// static pre-dial values matching the retired in-process module; the
// live-binary tripwire test fails loudly if they ever drift from the
// advertised values.
func NewShimModule(runtime sdk.RuntimeID, client *Client) (*ShimModule, error) {
	if runtime != "gke-autopilot" && runtime != "gke-standard" {
		return nil, fmt.Errorf("GCP provider shim cannot serve runtime %q", runtime)
	}
	return newShimModule("gcp", runtime, platformTierForRuntime(runtime), gcpOutputKeys(), client)
}

// NewLazyProviderShimModule binds one provider runtime to a verified plugin.
func NewLazyProviderShimModule(provider sdk.ProviderID, runtime sdk.RuntimeID, tier platform.CertificationTier, outputKeys []string, dial DialFunc) (*ShimModule, error) {
	if dial == nil {
		return nil, errors.New("provider dialer is required")
	}
	if provider == "" || runtime == "" {
		return nil, errors.New("provider and runtime are required")
	}
	return newShimModule(provider, runtime, tier, outputKeys, NewLazyClient(dial))
}

func newShimModule(provider sdk.ProviderID, runtime sdk.RuntimeID, tier platform.CertificationTier, outputKeys []string, client *Client) (*ShimModule, error) {
	if client == nil {
		return nil, errors.New("provider client is required")
	}
	if len(outputKeys) == 0 {
		outputKeys = platform.RequiredOutputKeys()
	}
	module := &ShimModule{
		provider:   provider,
		runtime:    runtime,
		tier:       tier,
		outputKeys: append([]string(nil), outputKeys...),
		client:     client,
	}
	if provider == "gcp" {
		module.Edge = func(ctx context.Context, planned platform.PlannedStack) (sdk.EdgeAdapter, error) {
			return NewShimEdgeAdapter(ctx, client, planned)
		}
		module.Resilience = func(ctx context.Context, planned platform.PlannedStack) (sdk.ResilienceAdapter, error) {
			return NewShimResilienceAdapter(ctx, client, planned)
		}
	}
	if provider == "aws" {
		module.Admission = func(_ context.Context, planned platform.PlannedStack) (platform.PlanAdmission, error) {
			shim, ok := AsShimPlanned(planned)
			if !ok || shim.module != module {
				return nil, errors.New("AWS admission requires a plan from this provider shim")
			}
			return shimPlanAdmission{module: module}, nil
		}
	}
	return module, nil
}

// NewLazyAzureShimModule binds the AKS runtime to an independently negotiated
// Azure provider. The session is opened only when a command needs the provider.
func NewLazyAzureShimModule(dial DialFunc) (*ShimModule, error) {
	return NewLazyProviderShimModule("azure", "aks", platform.TierExperimental, azureOutputKeys(), dial)
}

func platformTierForRuntime(runtime sdk.RuntimeID) platform.CertificationTier {
	if runtime == "gke-autopilot" {
		return platform.TierBuiltIn
	}
	return platform.TierExperimental
}

func gcpOutputKeys() []string {
	return append(append([]string(nil), platform.RequiredOutputKeys()...), "mediaURL", "mediaBucket", platform.OutputSearchEndpoint, platform.OutputQueueHost, platform.OutputQueueReplicas, platform.OutputDatabaseConnectionName, "queueMode", platform.OutputDatabaseSecretName, platform.OutputQueuePasswordSecretName, "securityPolicyName")
}

func azureOutputKeys() []string {
	return append(append([]string(nil), platform.RequiredOutputKeys()...), "clusterID", "databaseHost", "mediaAccount", "mediaShare", "vaultURI", "logAnalyticsWorkspaceId", "kubeconfig", "namespace", "imagePullSecret", "mediaClaim", "databaseTLSCA", platform.OutputSearchEndpoint, "queueMode", platform.OutputDatabaseSecretName, platform.OutputEncryptionKeySecretName)
}

func (m *ShimModule) Descriptor() sdk.TargetDescriptor {
	runtime := sdk.RuntimeID("gke-autopilot")
	if m != nil && m.runtime != "" {
		runtime = m.runtime
	}
	provider := sdk.ProviderID("gcp")
	if m != nil && m.provider != "" {
		provider = m.provider
	}
	return sdk.TargetDescriptor{ID: sdk.TargetID(string(provider) + "." + string(runtime)), Provider: provider, Runtime: runtime}
}

func (m *ShimModule) CertificationTier() platform.CertificationTier {
	if m != nil && m.tier != "" {
		return m.tier
	}
	return platformTierForRuntime(m.runtime)
}

func platformTier(tier sdk.ExtensionCertificationTier) platform.CertificationTier {
	if tier == sdk.ExtensionTierBuiltIn {
		return platform.TierBuiltIn
	}
	return platform.TierExperimental
}

// retainedInputs carries everything a re-plan needs. WithImageDigest and
// WithLiveQueueReplicas mutate a copy and re-plan: plans are immutable
// values, never post-hoc mutations.
type retainedInputs struct {
	stateEnvironment       string
	envelope               sdk.Envelope
	application            sdk.Application
	targetBlock            []byte
	services               map[string]sdk.ServiceBinding
	edge                   sdk.EdgeIntent
	observability          sdk.ObservabilityIntent
	runtime                string
	liveQueueReplicas      int
	valkeyRequirement      string
	allowUnsupported       bool
	allowExpiredPreview    bool
	accountOnly            bool
	runtimeObserve         bool
	useProviderDefaultEdge bool
	admitted               bool
}

func (m *ShimModule) Plan(cfg config.Config, environment string, opts platform.PlanOptions) (platform.PlannedStack, error) {
	if m == nil || m.client == nil {
		return nil, errors.New("provider client is required")
	}
	if sdk.ProviderID(cfg.Target.Provider) != m.provider {
		return nil, fmt.Errorf("%s provider shim cannot plan target %q", m.provider, cfg.Target.Provider)
	}
	if cfg.Target.Runtime != "" && cfg.Target.Runtime != string(m.runtime) {
		return nil, fmt.Errorf("%s %s shim cannot plan runtime %q", m.provider, m.runtime, cfg.Target.Runtime)
	}
	var target any
	switch m.provider {
	case "aws":
		if cfg.Target.AWS == nil {
			return nil, errors.New("target.aws is required for AWS deployment")
		}
		target = cfg.Target.AWS
	case "azure":
		if cfg.Target.Azure == nil {
			return nil, errors.New("target.azure is required for Azure deployment")
		}
		target = cfg.Target.Azure
	case "gcp":
		if cfg.Target.GCP == nil {
			return nil, errors.New("target.gcp is required for GCP deployment")
		}
		target = cfg.Target.GCP
	default:
		return nil, fmt.Errorf("unsupported provider shim %q", m.provider)
	}
	block, err := yaml.Marshal(target)
	if err != nil {
		return nil, fmt.Errorf("marshal %s target block: %w", m.provider, err)
	}
	inputs := retainedInputs{
		stateEnvironment: cfg.StateEnvironment, envelope: buildEnvelope(cfg, environment),
		application: buildApplication(cfg), targetBlock: block, services: maps.Clone(cfg.Services),
		runtime: string(m.runtime), allowUnsupported: true, allowExpiredPreview: opts.AllowExpiredPreview,
		accountOnly: opts.AccountOnly, runtimeObserve: opts.RuntimeObserve,
		useProviderDefaultEdge: cfg.Edge.Mode == "" && cfg.Edge.NativeProvider == "" && cfg.Edge.ExternalProvider == "",
	}
	if m.provider == "aws" {
		inputs.allowUnsupported = cfg.Compatibility.AllowUnsupported
	}
	if opts.AccountOnly || opts.AllowExpiredPreview {
		return m.replan(planContext(opts), inputs)
	}
	if m.provider == "gcp" {
		if err := platform.ValidateFirstPartyEdge(cfg); err != nil {
			return nil, err
		}
		if err := platform.ValidateFirstPartyObservability(cfg); err != nil {
			return nil, err
		}
		requirement, _ := config.ServiceRequirementForRelease(cfg.Application.Version, config.CompatibilityCache, "valkey")
		if len(requirement.Versions) > 0 {
			inputs.valkeyRequirement = requirement.Versions[0]
		} else {
			inputs.valkeyRequirement = cfg.Services["cache"].Version
			if inputs.valkeyRequirement == "" {
				inputs.valkeyRequirement = strings.ReplaceAll(strings.TrimPrefix(cfg.Target.GCP.MemorystoreEngineVersion, "VALKEY_"), "_", ".")
			}
		}
	}
	inputs.edge, err = platform.EdgeIntentFromConfig(cfg, environment)
	if err != nil {
		return nil, err
	}
	inputs.observability, err = platform.ObservabilityIntentFromConfig(cfg, environment)
	if err != nil {
		return nil, err
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
		Admit:                  inputs.admitted,
		UseProviderDefaultEdge: inputs.useProviderDefaultEdge,
		StateEnvironment:       inputs.stateEnvironment,
		ProtocolVersion:        sdk.ProtocolV1,
		Envelope:               inputs.envelope,
		Application:            inputs.application,
		Edge:                   inputs.edge,
		Observability:          inputs.observability,
		TargetBlock:            inputs.targetBlock,
		Services:               inputs.services,
		Runtime:                inputs.runtime,
		LiveQueueReplicas:      inputs.liveQueueReplicas,
		ValkeyRequirement:      inputs.valkeyRequirement,
		AllowUnsupported:       inputs.allowUnsupported,
		AllowExpiredPreview:    inputs.allowExpiredPreview,
		AccountOnly:            inputs.accountOnly,
		RuntimeObserve:         inputs.runtimeObserve,
	})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, AsPluginError(sdk.OpPlan, resp.Error)
	}
	if resp.Plan.Tier == "" {
		if m.tier == platform.TierBuiltIn {
			resp.Plan.Tier = sdk.ExtensionTierBuiltIn
		} else {
			resp.Plan.Tier = sdk.ExtensionTierExperimental
		}
	} else if resp.Plan.Tier != sdk.ExtensionTierBuiltIn && resp.Plan.Tier != sdk.ExtensionTierExperimental {
		return nil, fmt.Errorf("provider returned invalid plan tier %q", resp.Plan.Tier)
	}
	var outputKeys []string
	if described := m.client.Describe(); described != nil {
		outputKeys = append(outputKeys, described.OutputKeys...)
	}
	return &ShimPlanned{module: m, stored: resp.Plan, inputs: inputs, outputKeys: outputKeys}, nil
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
		Account:            cfg.Account,
		Project:            cfg.Project.Name,
		Environment:        environment,
		Region:             region,
		EnvironmentClass:   cfg.Class,
		StackName:          platform.FormatStackName(cfg.Project.Name, environment, sdk.ProviderID(cfg.Target.Provider), sdk.RuntimeID(cfg.Target.Runtime)),
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
	email := cfg.Email
	var managedDomain, managedHostedZoneID string
	if email.Managed != nil {
		managedDomain = email.Managed.Domain
		managedHostedZoneID = email.Managed.HostedZoneID
	}
	return sdk.Application{
		Edition:    cfg.Application.Edition,
		Version:    cfg.Application.Version,
		Mode:       cfg.Application.Mode,
		WebRuntime: cfg.Application.WebRuntime,
		Email: sdk.EmailSettings{
			ManagedDomain:       managedDomain,
			ManagedHostedZoneID: managedHostedZoneID,
			Mode:                email.Mode,
			Host:                email.Host,
			Port:                email.Port,
			Username:            email.Username,
			From:                email.From,
			Credential:          email.Credential,
		},
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

// Program directs callers to the provider backend that owns the Pulumi program.
func (m *ShimModule) Program(platform.PlannedStack) (pulumi.RunFunc, error) {
	return nil, errors.New("use the provider automation backend to run its Pulumi program")
}

func (m *ShimModule) OutputKeys() []string {
	if m != nil && len(m.outputKeys) > 0 {
		return append([]string(nil), m.outputKeys...)
	}
	return platform.RequiredOutputKeys()
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
	module     *ShimModule
	stored     sdk.StoredPlan
	inputs     retainedInputs
	outputKeys []string
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

// OutputKeys returns the keys advertised by the connected provider.
func (p *ShimPlanned) OutputKeys() []string {
	if p == nil {
		return nil
	}
	return append([]string(nil), p.outputKeys...)
}

type shimPlanAdmission struct{ module *ShimModule }

func (a shimPlanAdmission) Admit(ctx context.Context, planned platform.PlannedStack) (platform.PlannedStack, error) {
	shim, ok := AsShimPlanned(planned)
	if !ok || shim.module != a.module {
		return nil, errors.New("AWS admission requires a plan from this provider shim")
	}
	inputs := shim.inputs
	inputs.admitted = true
	return a.module.replan(ctx, inputs)
}

// EnvelopePlan exposes plan identity for plan-scoped plugin calls without
// coupling callers to the concrete shim type.
type EnvelopePlan interface {
	Envelope() sdk.Envelope
	StoredPlan() sdk.StoredPlan
}

// AsEnvelopePlan extracts plan identity for plugin calls.
func AsEnvelopePlan(planned platform.PlannedStack) (EnvelopePlan, bool) {
	value, ok := planned.(EnvelopePlan)
	return value, ok
}

func (p *ShimPlanned) StackName() string        { return p.stored.StackName }
func (p *ShimPlanned) Provider() sdk.ProviderID { return sdk.ProviderID(p.stored.Provider) }
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
	provider := p.Provider()
	return sdk.TargetDescriptor{ID: sdk.TargetID(string(provider) + "." + string(runtime)), Provider: provider, Runtime: runtime}
}

// StateBackendURL reports the plan-bound backend URL for CLI backend wiring.
func (p *ShimPlanned) StateBackendURL() string { return p.stored.StateBackendURL }

// Envelope reports the retained deployment envelope for backend wiring.
func (p *ShimPlanned) Envelope() sdk.Envelope { return p.inputs.envelope }

// StoredPlan reports the opaque stored plan for backend wiring.
func (p *ShimPlanned) StoredPlan() sdk.StoredPlan { return p.stored }

// WithImageDigest re-plans with a new image digest in the target block.
func (p *ShimPlanned) WithImageDigest(digest string) (platform.PlannedStack, error) {
	if p == nil || p.module == nil {
		return nil, errors.New("shim plan has no module")
	}
	var target map[string]any
	if err := yaml.Unmarshal(p.inputs.targetBlock, &target); err != nil {
		return nil, fmt.Errorf("decode retained target block: %w", err)
	}
	target["imageDigest"] = digest
	block, err := yaml.Marshal(target)
	if err != nil {
		return nil, fmt.Errorf("marshal target block: %w", err)
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

// ShimOps orchestrates Magento deploys core-side while the provider executes
// every application phase. Stored deploy inputs pass through as opaque
// bytes; core behavior never depends on provider internals.
type ShimOps struct {
	client        *Client
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
		return nil, fmt.Errorf("provider operations received unexpected plan type %T", planned)
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

func (o ShimOps) NewDeploySteps(_ context.Context, backend any, planned platform.PlannedStack, diagnostics io.Writer) (deployflow.Steps, error) {
	shim, ok := AsShimPlanned(planned)
	if !ok {
		return nil, fmt.Errorf("provider operations received unexpected plan type %T", planned)
	}
	typed, ok := backend.(kube.Backend)
	if !ok {
		return nil, fmt.Errorf("provider deployment requires an infrastructure backend with outputs, got %T", backend)
	}
	if o.client == nil {
		return nil, errors.New("provider client is required")
	}
	if diagnostics == nil {
		diagnostics = io.Discard
	}
	return &shimDeploySteps{
		client:      o.client,
		backend:     typed,
		envelope:    shim.inputs.envelope,
		stored:      shim.stored,
		diagnostics: diagnostics,
		record:      o.RecordRelease,
	}, nil
}

// shimDeploySteps implements deployflow.Steps for plugin-backed stacks.
// Infrastructure methods drive the Pulumi backend; application phases RPC
// to the provider, which decodes its own stored inputs and executes. The
// opaque candidate handle crosses back and forth; core never interprets it.
type shimDeploySteps struct {
	client      *Client
	backend     kube.Backend
	envelope    sdk.Envelope
	stored      sdk.StoredPlan
	diagnostics io.Writer
	record      func(context.Context, deployflow.Request, deployflow.Result) error
	state       []byte
}

var _ deployflow.Steps = (*shimDeploySteps)(nil)

func (s *shimDeploySteps) Validate(_ context.Context, request deployflow.Request) error {
	if s == nil || s.backend == nil {
		return errors.New("provider deployment steps are required")
	}
	if err := sdk.ValidateTargetDescriptor(request.Target); err != nil {
		return err
	}
	if request.ImageDigest != s.stored.ImageDigest {
		return fmt.Errorf("deployment digest %q does not match the planned artifact", request.ImageDigest)
	}
	return nil
}

func (s *shimDeploySteps) Preview(ctx context.Context, request deployflow.Request) (automation.ChangeSummary, error) {
	return automation.NewRunner(s.backend, s.diagnostics).Preview(ctx, automation.Request{Target: request.Target, Preview: request.Preview})
}

func (s *shimDeploySteps) RegisterCandidate(ctx context.Context, request deployflow.Request) error {
	outputs, err := s.backend.Outputs(ctx)
	if err != nil {
		return fmt.Errorf("read deployment outputs: %w", err)
	}
	// Greenfield stacks have no cluster yet. Create infrastructure first.
	if _, err := platform.RequireStringOutput(outputs, platform.OutputClusterName); err != nil {
		if _, updateErr := s.UpdateServices(ctx, request); updateErr != nil {
			return fmt.Errorf("create initial infrastructure: %w", updateErr)
		}
		outputs, err = s.backend.Outputs(ctx)
		if err != nil {
			return fmt.Errorf("read deployment outputs after initial create: %w", err)
		}
	}
	encoded, err := json.Marshal(outputs)
	if err != nil {
		return fmt.Errorf("encode deployment outputs: %w", err)
	}
	resp, err := Call[sdk.DeployAppPhaseCall, sdk.DeployAppPhaseResult](ctx, s.client, sdk.OpDeployAppPhase, &sdk.DeployAppPhaseCall{
		ProtocolVersion: sdk.ProtocolV1, Envelope: s.envelope, Plan: s.stored,
		Phase: sdk.DeployPhaseRegister, ImageDigest: request.ImageDigest, OutputsJSON: encoded,
	})
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return AsPluginError(sdk.OpDeployAppPhase, resp.Error)
	}
	s.state = resp.StateJSON
	s.report(resp.Message)
	return nil
}

func (s *shimDeploySteps) RunMigrations(ctx context.Context, request deployflow.Request) error {
	if len(s.state) == 0 {
		return errors.New("deployment candidate has not been registered")
	}
	resp, err := s.phase(ctx, sdk.DeployPhaseMigrate, request)
	if err != nil {
		return err
	}
	// Providers that need candidate verification may return an updated opaque
	// handle for stabilization and health. Empty state means their migration
	// phase completed the candidate lifecycle.
	s.state = append(s.state[:0], resp.StateJSON...)
	s.report(resp.Message)
	return nil
}

func (s *shimDeploySteps) CleanupCandidate(ctx context.Context, request deployflow.Request) error {
	if len(s.state) == 0 {
		return nil
	}
	resp, err := s.phase(ctx, sdk.DeployPhaseCleanup, request)
	if err != nil {
		return err
	}
	s.state = nil
	s.report(resp.Message)
	return nil
}

func (s *shimDeploySteps) UpdateServices(ctx context.Context, request deployflow.Request) (automation.ChangeSummary, error) {
	return automation.NewRunner(s.backend, s.diagnostics).Update(ctx, automation.Request{Target: request.Target, Preview: request.Preview})
}

func (s *shimDeploySteps) Stabilize(ctx context.Context, request deployflow.Request) error {
	resp, err := s.phase(ctx, sdk.DeployPhaseStabilize, request)
	if err != nil {
		return err
	}
	s.report(resp.Message)
	return nil
}

func (s *shimDeploySteps) Health(ctx context.Context, request deployflow.Request) error {
	resp, err := s.phase(ctx, sdk.DeployPhaseHealth, request)
	if err != nil {
		return err
	}
	s.state = nil
	s.report(resp.Message)
	return nil
}

func (s *shimDeploySteps) Record(ctx context.Context, request deployflow.Request, result deployflow.Result) error {
	if s.record == nil {
		return nil
	}
	return s.record(ctx, request, result)
}

func (s *shimDeploySteps) phase(ctx context.Context, phase sdk.DeployAppPhase, request deployflow.Request) (*sdk.DeployAppPhaseResult, error) {
	outputs, err := s.backend.Outputs(ctx)
	if err != nil {
		return nil, fmt.Errorf("read deployment outputs: %w", err)
	}
	encoded, err := json.Marshal(outputs)
	if err != nil {
		return nil, fmt.Errorf("encode deployment outputs: %w", err)
	}
	resp, err := Call[sdk.DeployAppPhaseCall, sdk.DeployAppPhaseResult](ctx, s.client, sdk.OpDeployAppPhase, &sdk.DeployAppPhaseCall{
		ProtocolVersion: sdk.ProtocolV1, Envelope: s.envelope, Plan: s.stored,
		Phase: phase, ImageDigest: request.ImageDigest, OutputsJSON: encoded, StateJSON: append([]byte(nil), s.state...),
	})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, AsPluginError(sdk.OpDeployAppPhase, resp.Error)
	}
	return resp, nil
}

func (s *shimDeploySteps) report(message string) {
	if s == nil || s.diagnostics == nil || strings.TrimSpace(message) == "" {
		return
	}
	fmt.Fprintln(s.diagnostics, message)
}
