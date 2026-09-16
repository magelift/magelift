package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/magelift/magelift/internal/config"
	fastlyedge "github.com/magelift/magelift/internal/external/fastly"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/toolchain"
	"github.com/magelift/magelift/sdk"
	"github.com/spf13/cobra"
)

type fastlyLifecycle interface {
	Plan(fastlyedge.Request) (fastlyedge.Plan, error)
	Apply(context.Context, fastlyedge.Request) (fastlyedge.Result, error)
	Destroy(context.Context, fastlyedge.Request, fastlyedge.Result) error
	Purge(context.Context, fastlyedge.Request) (fastlyedge.Result, error)
}

type fastlyEdgeState struct {
	Environment string            `json:"environment" yaml:"environment"`
	Result      fastlyedge.Result `json:"result" yaml:"result"`
}

func edgeCommand(o *options) *cobra.Command {
	command := &cobra.Command{
		Use:   "edge",
		Short: "Manage explicitly configured external edge services",
	}

	plan := &cobra.Command{
		Use:   "plan",
		Short: "Print the Fastly edge plan without changing provider state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireDependencies(cmd.Context(), o, toolchain.SpecsForFastly(), "Fastly edge lifecycle"); err != nil {
				return err
			}
			environment, request, client, err := o.fastlyRequestForSelectedEnvironment()
			if err != nil {
				return invalid(err)
			}
			value, err := client.Plan(request)
			if err != nil {
				return invalid(err)
			}
			return o.write(map[string]any{"environment": environment, "plan": value})
		},
	}

	apply := &cobra.Command{
		Use:   "apply",
		Short: "Apply the configured Fastly service and domains",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireDependencies(cmd.Context(), o, toolchain.SpecsForFastly(), "Fastly edge lifecycle"); err != nil {
				return err
			}
			environment, request, client, err := o.fastlyRequestForSelectedEnvironment()
			if err != nil {
				return invalid(err)
			}
			_, planned, err := o.planStack(false)
			if err != nil {
				return invalid(err)
			}
			if _, err := o.admitPlanned(cmd.Context(), planned); err != nil {
				return invalid(fmt.Errorf("provider plan admission: %w", err))
			}
			plan, err := client.Plan(request)
			if err != nil {
				return invalid(err)
			}
			result, err := client.Apply(cmd.Context(), request)
			if err != nil {
				return err
			}
			if err := o.saveFastlyEdgeState(environment, result); err != nil {
				return err
			}
			return o.write(map[string]any{"environment": environment, "plan": plan, "result": result})
		},
	}

	destroy := &cobra.Command{
		Use:   "destroy",
		Short: "Delete only Fastly resources owned by this project environment",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !o.yes {
				return invalid(errors.New("edge destroy requires --yes"))
			}
			if err := requireDependencies(cmd.Context(), o, toolchain.SpecsForFastly(), "Fastly edge lifecycle"); err != nil {
				return err
			}
			environment, request, client, err := o.fastlyRequestForSelectedEnvironment()
			if err != nil {
				return invalid(err)
			}
			state, err := o.loadFastlyEdgeState(environment)
			if errors.Is(err, os.ErrNotExist) {
				if o.stderr != nil {
					_, _ = fmt.Fprintf(o.stderr, "notice: no MageLift-managed Fastly state exists for %s; preserving the configured service\n", environment)
				}
				return o.write(map[string]any{"environment": environment, "destroyed": false, "reason": "no-managed-state"})
			}
			if err != nil {
				return err
			}
			if err := client.Destroy(cmd.Context(), request, state.Result); err != nil {
				return err
			}
			if err := os.Remove(fastlyEdgeStatePath(o.configPath, environment)); err != nil {
				return fmt.Errorf("remove Fastly state: %w", err)
			}
			return o.write(map[string]any{"environment": environment, "destroyed": true, "serviceId": state.Result.ServiceID})
		},
	}

	command.AddCommand(plan, apply, destroy, edgePurgeCommand(o))
	return command
}

func edgePurgeCommand(o *options) *cobra.Command {
	return &cobra.Command{
		Use:   "purge",
		Short: "Invalidate or purge the selected environment's Magento-facing edge cache",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			result, err := o.runEdgePurge(cmd.Context())
			if err != nil {
				return err
			}
			return o.write(result)
		},
	}
}

func (o *options) runEdgePurge(ctx context.Context) (map[string]any, error) {
	effective, environment, err := o.resolveWithEnvironment()
	if err != nil {
		return nil, invalid(err)
	}
	cfg := effective.Config
	if strings.EqualFold(strings.TrimSpace(cfg.Edge.ExternalProvider), "fastly") {
		if err := requireDependencies(ctx, o, toolchain.SpecsForFastly(), "Fastly edge lifecycle"); err != nil {
			return nil, err
		}
		_, request, client, err := o.fastlyRequestForSelectedEnvironment()
		if err != nil {
			return nil, invalid(err)
		}
		result, err := client.Purge(ctx, request)
		if err != nil {
			return nil, err
		}
		return map[string]any{"environment": environment, "provider": "fastly", "purged": result.PurgeRequested, "serviceId": result.ServiceID}, nil
	}
	_, planned, err := o.planStack(false)
	if err != nil {
		return nil, invalid(err)
	}
	module, found := o.modules.Module(planned.Provider(), planned.Runtime())
	if found {
		adapter, err := platform.ModuleEdgeFor(ctx, module, planned)
		if err != nil {
			return nil, invalid(err)
		}
		if adapter != nil {
			return o.executeNativeEdgePurge(ctx, environment, cfg, planned, adapter)
		}
	}
	reason := edgePurgeUnsupportedReason(cfg)
	return nil, &exitError{code: 3, err: sdk.EdgeCapabilityError{
		AdapterID: cfg.Target.Provider + ".edge",
		Action:    sdk.EdgePurge,
		Status:    sdk.EdgeCapabilityUnsupported,
		Reason:    reason,
	}}
}

func edgePurgeUnsupportedReason(cfg config.Config) string {
	provider := strings.TrimSpace(cfg.Target.Provider)
	native := strings.TrimSpace(cfg.Edge.NativeProvider)
	if provider == "ovh" || strings.EqualFold(native, "cdn") || strings.EqualFold(native, "ovh-cdn") {
		return "OVH native edge cannot purge Magento cache; use Fastly or another provider that advertises purge"
	}
	if native == "" && strings.TrimSpace(cfg.Edge.ExternalProvider) == "" {
		return "selected target has no Magento-facing CDN cache to purge; configure Fastly or a native CloudFront/Cloud CDN edge"
	}
	return "selected target does not advertise Magento edge purge"
}

func (o *options) executeNativeEdgePurge(ctx context.Context, environment string, cfg config.Config, planned platform.PlannedStack, adapter sdk.EdgeAdapter) (map[string]any, error) {
	intent, err := platform.EdgeIntentFromConfig(cfg, environment)
	if err != nil {
		return nil, invalid(err)
	}
	plan, err := adapter.PlanEdge(ctx, sdk.EdgePlanRequest{
		TargetProvider: planned.Provider(),
		TargetRuntime:  planned.Runtime(),
		Intent:         intent,
	})
	if err != nil {
		var capability sdk.EdgeCapabilityError
		if errors.As(err, &capability) {
			return nil, &exitError{code: 3, err: capability}
		}
		return nil, err
	}
	result, err := adapter.ExecuteEdge(ctx, sdk.EdgeExecutionRequest{
		Plan:            plan,
		Action:          sdk.EdgePurge,
		IdempotencyKey:  "purge/" + environment,
		OwnershipMarker: plan.OwnershipMarker,
	})
	if err != nil {
		var capability sdk.EdgeCapabilityError
		if errors.As(err, &capability) {
			return nil, &exitError{code: 3, err: capability}
		}
		return nil, err
	}
	return map[string]any{"environment": environment, "provider": string(planned.Provider()), "purged": true, "operationId": result.OperationID}, nil
}

func (o *options) fastlyRequestForSelectedEnvironment() (string, fastlyedge.Request, fastlyLifecycle, error) {
	effective, environment, err := o.resolveWithEnvironment()
	if err != nil {
		return "", fastlyedge.Request{}, nil, err
	}
	request, err := fastlyRequest(effective.Config, environment)
	if err != nil {
		return "", fastlyedge.Request{}, nil, err
	}
	if o.newFastly == nil {
		return "", fastlyedge.Request{}, nil, errors.New("Fastly lifecycle adapter is not configured")
	}
	client, err := o.newFastly(request)
	if err != nil {
		return "", fastlyedge.Request{}, nil, err
	}
	if client == nil {
		return "", fastlyedge.Request{}, nil, errors.New("Fastly lifecycle adapter is unavailable")
	}
	return environment, request, client, nil
}

func (o *options) fastlyRequestForEnvironment(environment, originURL string) (fastlyedge.Request, bool, error) {
	file, err := o.load()
	if err != nil {
		return fastlyedge.Request{}, false, err
	}
	selectedEnvironment := environment
	if o.previewIdentityRequested() && strings.TrimSpace(o.environment) != "" {
		selectedEnvironment = o.environment
	}
	effective, resolvedEnvironment, err := o.resolveEnvironment(file, selectedEnvironment)
	if err != nil {
		return fastlyedge.Request{}, false, err
	}
	if effective.Config.Edge.ExternalProvider == "" {
		return fastlyedge.Request{}, false, nil
	}
	request, err := fastlyRequest(effective.Config, resolvedEnvironment)
	if err == nil && strings.TrimSpace(originURL) != "" && strings.TrimSpace(request.ProviderConfig.HealthOriginURL) == "" {
		request.ProviderConfig.HealthOriginURL = strings.TrimSpace(originURL)
	}
	return request, true, err
}

func fastlyRequest(cfg config.Config, environment string) (fastlyedge.Request, error) {
	if cfg.Edge.ExternalProvider != "fastly" {
		return fastlyedge.Request{}, fmt.Errorf("edge.externalProvider must be fastly for external edge operations (got %q)", cfg.Edge.ExternalProvider)
	}
	providerConfig := fastlyedge.Config{
		TLS:               cfg.Edge.TLS,
		TLSMode:           cfg.Edge.TLSMode,
		DNSMode:           cfg.Edge.DNSMode,
		VCLRef:            cfg.Edge.VCLRef,
		OriginHealthRef:   cfg.Edge.OriginHealthRef,
		CachePolicyRef:    cfg.Edge.CachePolicyRef,
		PurgePolicyRef:    cfg.Edge.PurgePolicyRef,
		WAFPolicyRef:      cfg.Edge.WAFPolicyRef,
		FailoverPolicyRef: cfg.Edge.FailoverPolicyRef,
	}
	if advanced, ok := cfg.Extensions["fastly.edge"]; ok {
		var overlayErr error
		providerConfig, overlayErr = fastlyedge.ApplyAdvancedConfig(providerConfig, advanced)
		if overlayErr != nil {
			return fastlyedge.Request{}, fmt.Errorf("decode Fastly advanced configuration: %w", overlayErr)
		}
	}
	if health := cfg.Edge.Health; health != nil {
		providerConfig.HealthOriginURL = health.OriginURL
		providerConfig.HealthOriginHost = health.OriginHost
		providerConfig.HealthExpectedCNAME = health.ExpectedCNAME
		providerConfig.HealthRoutePath = health.RoutePath
		providerConfig.HealthExpectedStatus = health.ExpectedStatus
		providerConfig.HealthRouteTimeoutSeconds = health.RouteTimeoutSeconds
		providerConfig.HealthRoutePollSeconds = health.RoutePollSeconds
	}
	if providerConfig.HealthOriginHost == "" && len(cfg.Edge.Domains) == 1 {
		// MageLift's default ALB/application certificate is issued for the
		// configured edge hostname, not the generated load-balancer DNS name.
		// Use that hostname for origin SNI/Host unless advanced YAML overrides it.
		providerConfig.HealthOriginHost = cfg.Edge.Domains[0]
	}
	return fastlyedge.Request{
		ServiceID:       cfg.Edge.ServiceID,
		CredentialRef:   cfg.Edge.TokenSecret,
		Domains:         append([]string(nil), cfg.Edge.Domains...),
		OwnershipMarker: fastlyOwnershipMarker(cfg.Project.Name, environment),
		PurgeOnDeploy:   cfg.Edge.PurgeOnDeploy,
		ProviderConfig:  providerConfig,
	}, nil
}

func fastlyHealthProbe(request fastlyedge.Request) (fastlyedge.HealthProbe, error) {
	config := request.ProviderConfig
	// Deploy can derive the origin URL from the freshly-created stack's
	// applicationURL output. A standalone `edge plan` can therefore remain
	// useful before that output is known; `edge apply` still fails closed when
	// no probe can be constructed.
	if strings.TrimSpace(config.HealthOriginURL) == "" {
		return nil, nil
	}
	probe, err := fastlyedge.NewHTTPHealthProbe(fastlyedge.HTTPHealthProbeConfig{
		OriginURL: config.HealthOriginURL, OriginHost: config.HealthOriginHost,
		ExpectedCNAME:  config.HealthExpectedCNAME,
		RoutePath:      config.HealthRoutePath,
		ExpectedStatus: config.HealthExpectedStatus,
		RouteTimeout:   secondsDuration(config.HealthRouteTimeoutSeconds),
		RoutePoll:      secondsDuration(config.HealthRoutePollSeconds),
	})
	if err != nil {
		return nil, fmt.Errorf("configure Fastly edge health probe: %w", err)
	}
	return probe, nil
}

func secondsDuration(seconds int) time.Duration {
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func fastlyOwnershipMarker(project, environment string) string {
	return "magelift/edge/" + fastlyMarkerPart(project) + "/" + fastlyMarkerPart(environment)
}

func fastlyMarkerPart(value string) string {
	var builder strings.Builder
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z', char >= '0' && char <= '9', char == '-', char == '_', char == '.':
			builder.WriteRune(char)
		default:
			builder.WriteByte('-')
		}
	}
	value = strings.Trim(builder.String(), "-")
	if value == "" {
		return "unknown"
	}
	return value
}

func (o *options) applyFastlyEdge(ctx context.Context, environment, originURL string) error {
	request, configured, err := o.fastlyRequestForEnvironment(environment, originURL)
	if err != nil || !configured {
		return err
	}
	if err := requireDependencies(ctx, o, toolchain.SpecsForFastly(), "Fastly edge lifecycle"); err != nil {
		return err
	}
	if o.newFastly == nil {
		return errors.New("Fastly lifecycle adapter is not configured")
	}
	client, err := o.newFastly(request)
	if err != nil {
		return err
	}
	if client == nil {
		return errors.New("Fastly lifecycle adapter is unavailable")
	}
	result, err := client.Apply(ctx, request)
	if err != nil {
		return fmt.Errorf("apply Fastly edge: %w", err)
	}
	if err := o.saveFastlyEdgeState(environment, result); err != nil {
		return fmt.Errorf("save Fastly edge state: %w", err)
	}
	return nil
}

func (o *options) destroyFastlyEdge(ctx context.Context, environment string) error {
	request, configured, err := o.fastlyRequestForEnvironment(environment, "")
	if err != nil || !configured {
		return err
	}
	if err := requireDependencies(ctx, o, toolchain.SpecsForFastly(), "Fastly edge lifecycle"); err != nil {
		return err
	}
	state, err := o.loadFastlyEdgeState(environment)
	if errors.Is(err, os.ErrNotExist) {
		if o.stderr != nil {
			_, _ = fmt.Fprintf(o.stderr, "notice: no MageLift-managed Fastly state exists for %s; preserving the configured service\n", environment)
		}
		return nil
	}
	if err != nil {
		return err
	}
	if o.newFastly == nil {
		return errors.New("Fastly lifecycle adapter is not configured")
	}
	client, err := o.newFastly(request)
	if err != nil {
		return err
	}
	if client == nil {
		return errors.New("Fastly lifecycle adapter is unavailable")
	}
	if err := client.Destroy(ctx, request, state.Result); err != nil {
		return fmt.Errorf("destroy Fastly edge: %w", err)
	}
	if err := os.Remove(fastlyEdgeStatePath(o.configPath, environment)); err != nil {
		return fmt.Errorf("remove Fastly state: %w", err)
	}
	return nil
}

func fastlyEdgeStatePath(configPath, environment string) string {
	return filepath.Join(filepath.Dir(filepath.Clean(configPath)), ".magelift", "edge", fastlyMarkerPart(environment), "fastly.json")
}

func (o *options) saveFastlyEdgeState(environment string, result fastlyedge.Result) error {
	path := fastlyEdgeStatePath(o.configPath, environment)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create Fastly state directory: %w", err)
	}
	data, err := json.MarshalIndent(fastlyEdgeState{Environment: environment, Result: result}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode Fastly state: %w", err)
	}
	data = append(data, '\n')
	if err := replaceFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write Fastly state: %w", err)
	}
	return nil
}

func (o *options) loadFastlyEdgeState(environment string) (fastlyEdgeState, error) {
	data, err := os.ReadFile(fastlyEdgeStatePath(o.configPath, environment))
	if err != nil {
		return fastlyEdgeState{}, err
	}
	var state fastlyEdgeState
	if err := json.Unmarshal(data, &state); err != nil {
		return fastlyEdgeState{}, fmt.Errorf("decode Fastly state: %w", err)
	}
	if state.Environment != environment || state.Result.ServiceID == "" || state.Result.OwnershipMarker == "" {
		return fastlyEdgeState{}, errors.New("Fastly state ownership proof is incomplete")
	}
	return state, nil
}
