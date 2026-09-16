// Command gcp-edge-acceptance runs one disposable GCP native edge control-plane
// cell: plan, apply with purge, optional origin-group failover, verify, destroy,
// and ownership inventory verification. Viewer HTTPS failover RTO is measured
// by the wrapper, not this binary.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	gcpedge "github.com/magelift/magelift/internal/cloud/gcp/edge"
	"github.com/magelift/magelift/internal/edge/waf"
	"github.com/magelift/magelift/sdk"
)

const (
	defaultTimeout     = 45 * time.Minute
	healthRef          = "health/origin"
	healthRefSecondary = "health/secondary"
)

func main() {
	if err := runMain(); err != nil {
		fmt.Fprintf(os.Stderr, "GCP edge acceptance failed: %v\n", err)
		os.Exit(1)
	}
}

func runMain() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, os.Args[1:], os.Stdout)
}

func run(parent context.Context, args []string, output io.Writer) error {
	flags := flag.NewFlagSet("gcp-edge-acceptance", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	project := flags.String("project", "", "GCP project ID")
	marker := flags.String("marker", "", "single-line MageLift ownership marker")
	domain := flags.String("domain", "", "TLS domain covered by the Google-managed certificate")
	sslCertificate := flags.String("ssl-certificate", "", "global SSL certificate name or https URL")
	backendService := flags.String("backend-service", "", "global backend service name or https URL")
	originHost := flags.String("origin-host", "example.com", "stable HTTPS origin hostname for the health probe")
	secondaryBackendService := flags.String("secondary-backend-service", "", "global secondary backend service name or https URL for origin-group failover")
	secondaryOriginHost := flags.String("secondary-origin-host", "developers.google.com", "secondary HTTPS origin hostname for origin-group health probes")
	originGroup := flags.Bool("origin-group", false, "configure a two-backend URL-map origin group")
	securityPolicy := flags.String("security-policy", "", "optional Cloud Armor security policy name or https URL")
	phase := flags.String("phase", "all", "lifecycle phase: all, apply, failover, or destroy")
	statePath := flags.String("state-path", "", "JSON state file written by apply and read by failover/destroy")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	for _, part := range []struct {
		value string
		name  string
	}{
		{*project, "project"}, {*marker, "marker"}, {*domain, "domain"},
		{*sslCertificate, "ssl-certificate"}, {*backendService, "backend-service"}, {*originHost, "origin-host"},
	} {
		if err := validatePart(part.value, part.name); err != nil {
			return err
		}
	}
	if strings.TrimSpace(*securityPolicy) != "" {
		if err := validatePart(*securityPolicy, "security-policy"); err != nil {
			return err
		}
	}
	if *originGroup {
		if err := validatePart(*secondaryBackendService, "secondary-backend-service"); err != nil {
			return err
		}
		if err := validatePart(*secondaryOriginHost, "secondary-origin-host"); err != nil {
			return err
		}
		if *backendService == *secondaryBackendService {
			return errors.New("origin-group requires distinct primary and secondary backend services")
		}
		if *originHost == *secondaryOriginHost {
			return errors.New("origin-group requires distinct primary and secondary origin hostnames")
		}
	}

	request, probe, err := acceptanceRequest(*marker, *domain, *sslCertificate, *backendService, *originHost, *secondaryBackendService, *secondaryOriginHost, *originGroup, *securityPolicy)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(parent, defaultTimeout)
	defer cancel()

	switch strings.TrimSpace(*phase) {
	case "all", "apply", "failover", "destroy":
	default:
		return fmt.Errorf("unsupported phase %q", *phase)
	}
	if (*phase == "destroy" || *phase == "failover") && strings.TrimSpace(*statePath) == "" {
		return errors.New(*phase + " phase requires --state-path")
	}
	if *phase == "failover" && !*originGroup {
		return errors.New("failover phase requires --origin-group")
	}

	fmt.Fprintf(output, "+ gcp-edge: loading Google Compute API project=%s marker=%s phase=%s trafficImpact=not-run\n", *project, *marker, *phase)
	adapter, err := gcpedge.NewNativeSDKLifecycleAdapter(ctx, *project, probe, edgeOperationPolicy())
	if err != nil {
		return fmt.Errorf("construct GCP edge lifecycle adapter: %w", err)
	}

	fmt.Fprintf(output, "+ gcp-edge: planning native Cloud CDN front end domain=%s\n", *domain)
	plan, err := adapter.PlanEdge(ctx, request)
	if err != nil {
		return fmt.Errorf("plan GCP edge acceptance cell: %w", err)
	}

	var applied sdk.EdgeExecutionResult
	var forwardingIP string
	if *phase == "destroy" || *phase == "failover" {
		state, err := readEdgeState(*statePath)
		if err != nil {
			return err
		}
		if state.Marker != plan.OwnershipMarker {
			return errors.New("state ownership marker does not match the plan")
		}
		applied.ResourceRefs = state.ResourceRefs
		forwardingIP = state.ForwardingIP
		if *phase == "failover" {
			fmt.Fprintf(output, "+ gcp-edge: failing over URL-map default backend after origin health gates\n")
			failedOver, err := adapter.ExecuteEdge(ctx, sdk.EdgeExecutionRequest{
				Plan: plan, Action: sdk.EdgeFailover, IdempotencyKey: "gcp-edge-acceptance-failover",
				OwnershipMarker: plan.OwnershipMarker, ResourceReferences: applied.ResourceRefs,
			})
			if err != nil {
				return fmt.Errorf("failover GCP edge acceptance cell: %w", err)
			}
			if !containsProof(failedOver.ProofRefs, "gcp.edge.failover-control-plane-converged") {
				return fmt.Errorf("failover did not converge URL-map backends: proofs=%#v", failedOver.ProofRefs)
			}
			fmt.Fprintf(output, "GCP edge acceptance FAILOVER marker=%s domain=%s forwardingIPAddress=%s waf=%s trafficImpact=failover-control-plane\n",
				*marker, *domain, forwardingIP, wafStatus(*securityPolicy))
			return nil
		}
	} else {
		fmt.Fprintf(output, "+ gcp-edge: applying URL map, target HTTPS proxy, forwarding rule, and purge\n")
		applied, err = adapter.ExecuteEdge(ctx, sdk.EdgeExecutionRequest{
			Plan: plan, Action: sdk.EdgeApply, IdempotencyKey: "gcp-edge-acceptance-apply", OwnershipMarker: plan.OwnershipMarker,
		})
		if err != nil {
			return fmt.Errorf("apply GCP edge acceptance cell: %w", err)
		}
		if !containsProof(applied.ProofRefs, "gcp.edge.cloud-cdn") {
			return fmt.Errorf("apply did not record Cloud CDN proof: proofs=%#v", applied.ProofRefs)
		}
		if !containsProof(applied.ProofRefs, "gcp.edge.purge-complete") {
			return fmt.Errorf("apply did not complete purge: proofs=%#v", applied.ProofRefs)
		}
		if *originGroup && !containsProof(applied.ProofRefs, "gcp.edge.origin-failover-configured") {
			return fmt.Errorf("apply did not record origin-group proof: proofs=%#v", applied.ProofRefs)
		}
		forwardingIP = outputValue(applied.Outputs, "forwardingIPAddress")
		if forwardingIP == "" {
			return errors.New("apply did not return forwardingIPAddress")
		}
		fmt.Fprintf(output, "+ gcp-edge: applied forwardingIPAddress=%s\n", forwardingIP)
		if name := outputValue(applied.Outputs, "urlMapName"); name != "" {
			fmt.Fprintf(output, "+ gcp-edge: applied urlMapName=%s\n", name)
		}

		fmt.Fprintf(output, "+ gcp-edge: verifying owned front-end resources\n")
		verified, err := adapter.ExecuteEdge(ctx, sdk.EdgeExecutionRequest{
			Plan: plan, Action: sdk.EdgeVerify, IdempotencyKey: "gcp-edge-acceptance-verify", OwnershipMarker: plan.OwnershipMarker,
			ResourceReferences: applied.ResourceRefs,
		})
		if err != nil {
			return fmt.Errorf("verify GCP edge acceptance cell: %w", err)
		}
		if !verified.OwnershipVerified || !verified.IdempotencyVerified {
			return fmt.Errorf("verify did not confirm ownership or idempotency: %#v", verified)
		}
		if strings.TrimSpace(*statePath) != "" {
			if err := writeEdgeState(*statePath, edgeState{
				Marker: plan.OwnershipMarker, Domain: *domain, ForwardingIP: forwardingIP, ResourceRefs: applied.ResourceRefs,
			}); err != nil {
				return err
			}
		}
		if *phase == "apply" {
			fmt.Fprintf(output, "GCP edge acceptance APPLY marker=%s domain=%s forwardingIPAddress=%s waf=%s trafficImpact=not-run\n",
				*marker, *domain, forwardingIP, wafStatus(*securityPolicy))
			return nil
		}
	}

	fmt.Fprintf(output, "+ gcp-edge: destroying URL map, target HTTPS proxy, and forwarding rule\n")
	destroyed, err := adapter.ExecuteEdge(ctx, sdk.EdgeExecutionRequest{
		Plan: plan, Action: sdk.EdgeDestroy, IdempotencyKey: "gcp-edge-acceptance-destroy",
		OwnershipMarker: plan.OwnershipMarker, ResourceReferences: applied.ResourceRefs,
	})
	if err != nil {
		return fmt.Errorf("destroy GCP edge acceptance cell: %w", err)
	}
	if destroyed.Action != sdk.EdgeDestroy {
		return fmt.Errorf("unexpected destroy action: %#v", destroyed)
	}
	if !containsProof(destroyed.ProofRefs, "gcp.edge.owning-service-inventory-empty") {
		return fmt.Errorf("destroy did not confirm empty ownership inventory: proofs=%#v", destroyed.ProofRefs)
	}

	fmt.Fprintf(output, "GCP edge acceptance PASS marker=%s domain=%s forwardingIPAddress=%s waf=%s trafficImpact=not-run cleanup=verified\n",
		*marker, *domain, forwardingIP, wafStatus(*securityPolicy))
	return nil
}

type edgeState struct {
	Marker       string   `json:"marker"`
	Domain       string   `json:"domain"`
	ForwardingIP string   `json:"forwardingIPAddress"`
	ResourceRefs []string `json:"resourceRefs"`
}

func readEdgeState(path string) (edgeState, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return edgeState{}, fmt.Errorf("read edge state %q: %w", path, err)
	}
	var state edgeState
	if err := json.Unmarshal(payload, &state); err != nil {
		return edgeState{}, fmt.Errorf("decode edge state %q: %w", path, err)
	}
	if strings.TrimSpace(state.Marker) == "" || len(state.ResourceRefs) == 0 {
		return edgeState{}, fmt.Errorf("edge state %q is incomplete", path)
	}
	return state, nil
}

func writeEdgeState(path string, state edgeState) error {
	payload, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode edge state: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return fmt.Errorf("write edge state %q: %w", path, err)
	}
	return nil
}

func acceptanceRequest(marker, domain, sslCertificate, backendService, originHost, secondaryBackendService, secondaryOriginHost string, originGroup bool, securityPolicy string) (sdk.EdgePlanRequest, *httpsOriginHealthProbe, error) {
	request := sdk.EdgePlanRequest{
		TargetProvider: "gcp",
		TargetRuntime:  "gcp-edge-live-cell",
		Intent: sdk.EdgeIntent{
			Mode:            "native",
			NativeProvider:  "cloud-cdn",
			Domains:         []string{domain},
			TLS:             true,
			TLSMode:         "managed",
			DNSMode:         "external",
			OriginHealthRef: healthRef,
			OwnershipMarker: marker,
			PurgeOnDeploy:   true,
		},
		Configuration: map[string]any{
			"backendService": backendService,
			"sslCertificate": sslCertificate,
			"cloudCDN":       true,
		},
	}
	if strings.TrimSpace(securityPolicy) != "" {
		request.Intent.WAFPolicyRef = waf.PolicyRef
		request.Configuration["securityPolicy"] = securityPolicy
	}
	probe := &httpsOriginHealthProbe{hosts: map[string]string{healthRef: originHost}}
	if originGroup {
		request.Intent.FailoverPolicyRef = "gcp/failover-policy"
		request.Configuration["originGroup"] = map[string]any{
			"primary":            backendService,
			"secondary":          secondaryBackendService,
			"primaryHealthRef":   healthRef,
			"secondaryHealthRef": healthRefSecondary,
		}
		probe.hosts[healthRefSecondary] = secondaryOriginHost
	}
	if err := sdk.ValidateEdgePlanRequest(request); err != nil {
		return sdk.EdgePlanRequest{}, nil, err
	}
	return request, probe, nil
}

func edgeOperationPolicy() sdk.EdgeOperationPolicy {
	return sdk.EdgeOperationPolicy{Timeout: 40 * time.Minute, PollInterval: 15 * time.Second, MaxAttempts: 200}
}

type httpsOriginHealthProbe struct {
	hosts  map[string]string
	client *http.Client
}

func (probe *httpsOriginHealthProbe) VerifyOrigin(ctx context.Context, reference string) error {
	host, ok := probe.hosts[strings.TrimSpace(reference)]
	if !ok || host == "" {
		return fmt.Errorf("unknown origin health reference %q", reference)
	}
	client := probe.httpClient()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+host+"/", nil)
	if err != nil {
		return fmt.Errorf("build origin health request for %q: %w", host, err)
	}
	req.Host = host
	req.Header.Set("User-Agent", "MageLift-origin-health/1")
	response, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("probe origin %q: %w", host, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 400 {
		return fmt.Errorf("origin %q returned status %d", host, response.StatusCode)
	}
	return nil
}

func (probe *httpsOriginHealthProbe) httpClient() *http.Client {
	if probe.client != nil {
		return probe.client
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func outputValue(outputs []sdk.AdapterOutput, key string) string {
	for _, output := range outputs {
		if output.Key == key {
			return output.Value
		}
	}
	return ""
}

func containsProof(proofRefs []string, want string) bool {
	for _, proof := range proofRefs {
		if proof == want {
			return true
		}
	}
	return false
}

func validatePart(value, name string) error {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("%s is required and must be single-line", name)
	}
	return nil
}

func wafStatus(securityPolicy string) string {
	if strings.TrimSpace(securityPolicy) == "" {
		return "not-attached"
	}
	return "attached"
}
