package platform

import (
	"fmt"
	"strings"

	"github.com/magelift/magelift/internal/config"
	sdk "github.com/magelift/magelift/sdk/v1"
)

// EdgeIntentFromConfig is the single portable mapping from first-party
// configuration to the public edge contract. Provider modules consume the
// resulting intent; they must not reintroduce vendor-shaped configuration.
func EdgeIntentFromConfig(cfg config.Config, environment string) (sdk.EdgeIntent, error) {
	intent := sdk.EdgeIntent{
		ExternalProvider: cfg.Edge.ExternalProvider, Lifecycle: externalLifecycle(cfg.Edge.ExternalProvider), Certification: externalCertification(cfg.Edge.ExternalProvider),
		Mode: edgeMode(cfg.Edge), NativeProvider: cfg.Edge.NativeProvider, CredentialRefs: stringSlice(cfg.Edge.TokenSecret), ServiceReference: cfg.Edge.ServiceID,
		Domains: append([]string(nil), cfg.Edge.Domains...), TLS: cfg.Edge.TLS, TLSMode: cfg.Edge.TLSMode, DNSMode: cfg.Edge.DNSMode, PurgeOnDeploy: cfg.Edge.PurgeOnDeploy,
		PolicyReference: cfg.Edge.VCLRef, OriginHealthRef: cfg.Edge.OriginHealthRef, CachePolicyRef: cfg.Edge.CachePolicyRef, PurgePolicyRef: cfg.Edge.PurgePolicyRef,
		WAFPolicyRef: cfg.Edge.WAFPolicyRef, FailoverPolicyRef: cfg.Edge.FailoverPolicyRef, OwnershipMarker: edgeOwnershipMarker(cfg, environment),
	}
	if health := cfg.Edge.Health; health != nil {
		intent.Health = sdk.EdgeHealthIntent{
			OriginURL: health.OriginURL, OriginHost: health.OriginHost, ExpectedRouteTarget: health.ExpectedCNAME, RoutePath: health.RoutePath,
			ExpectedStatus: health.ExpectedStatus, RouteTimeoutSeconds: health.RouteTimeoutSeconds, RoutePollSeconds: health.RoutePollSeconds,
		}
	}
	if err := sdk.ValidateEdgeIntent(intent); err != nil {
		return sdk.EdgeIntent{}, fmt.Errorf("validate edge intent: %w", err)
	}
	return intent, nil
}

// ValidateFirstPartyEdge rejects edge providers that MageLift cannot manage.
// Fastly is handled by the explicit external-edge lifecycle in the CLI after
// the origin deployment has passed its candidate and health gates.
func ValidateFirstPartyEdge(cfg config.Config) error {
	native := strings.TrimSpace(cfg.Edge.NativeProvider)
	if native == "" {
		return nil
	}
	if !nativeEdgeMatchesTarget(native, cfg.Target.Provider) {
		return fmt.Errorf("native edge provider %q does not match target.provider %q", native, cfg.Target.Provider)
	}
	return nil
}

func nativeEdgeMatchesTarget(provider, target string) bool {
	switch target {
	case "aws":
		return provider == "cloudfront" || provider == "cloudfront-waf"
	case "gcp":
		return provider == "cloud-cdn" || provider == "cloud-armor" || provider == "google-cloud-load-balancing"
	case "scaleway":
		return provider == "scaleway-edge-services" || provider == "scaleway-load-balancer"
	case "ovh":
		return provider == "ovh-cdn" || provider == "ovh-public-cloud-load-balancer"
	default:
		return false
	}
}
