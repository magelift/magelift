// Command aws-cloudfront-acceptance runs one disposable AWS CloudFront native
// edge cell: plan, apply with purge, optional origin-group failover, destroy,
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

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awsedge "github.com/magelift/magelift/internal/cloud/aws/edge"
	"github.com/magelift/magelift/internal/edge/waf"
	sdk "github.com/magelift/magelift/sdk/v1"
)

const (
	defaultTimeout     = 45 * time.Minute
	healthRefPrimary   = "health/primary"
	healthRefSecondary = "health/secondary"
)

func main() {
	if err := runMain(); err != nil {
		fmt.Fprintf(os.Stderr, "AWS CloudFront acceptance failed: %v\n", err)
		os.Exit(1)
	}
}

func runMain() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, os.Args[1:], os.Stdout)
}

func run(parent context.Context, args []string, output io.Writer) error {
	flags := flag.NewFlagSet("aws-cloudfront-acceptance", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	profile := flags.String("profile", "default", "AWS shared-config profile")
	marker := flags.String("marker", "", "single-line MageLift ownership marker")
	domain := flags.String("domain", "", "TLS alternate domain covered by the ACM certificate")
	certificateARN := flags.String("certificate-arn", "", "ACM certificate ARN in us-east-1")
	primaryOrigin := flags.String("primary-origin", "example.com", "primary custom HTTPS origin hostname")
	secondaryOrigin := flags.String("secondary-origin", "www.example.com", "secondary custom HTTPS origin hostname for origin-group failover")
	originGroup := flags.Bool("origin-group", true, "configure a two-origin CloudFront origin group")
	webACLID := flags.String("web-acl-id", "", "optional WAFv2 WebACL ARN for CloudFront scope")
	phase := flags.String("phase", "all", "lifecycle phase: all, apply, failover, or destroy")
	statePath := flags.String("state-path", "", "JSON state file written by apply and read by failover/destroy")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	switch strings.TrimSpace(*phase) {
	case "all", "apply", "failover", "destroy":
	default:
		return fmt.Errorf("unsupported phase %q", *phase)
	}
	if (*phase == "destroy" || *phase == "failover") && strings.TrimSpace(*statePath) == "" {
		return errors.New(*phase + " phase requires --state-path")
	}
	for _, part := range []struct {
		value string
		name  string
	}{
		{*profile, "profile"}, {*marker, "marker"}, {*domain, "domain"}, {*certificateARN, "certificate-arn"},
		{*primaryOrigin, "primary-origin"}, {*secondaryOrigin, "secondary-origin"},
	} {
		if err := validatePart(part.value, part.name); err != nil {
			return err
		}
	}
	if !strings.HasPrefix(*certificateARN, "arn:aws:acm:us-east-1:") {
		return errors.New("certificate-arn must reference an ACM certificate in us-east-1")
	}
	if *originGroup && *primaryOrigin == *secondaryOrigin {
		return errors.New("origin-group requires distinct primary and secondary origins")
	}

	request, probe, err := acceptanceRequest(*marker, *domain, *certificateARN, *primaryOrigin, *secondaryOrigin, *originGroup, *webACLID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(parent, defaultTimeout)
	defer cancel()

	fmt.Fprintf(output, "+ cloudfront: loading AWS SDK profile=%s region=us-east-1 marker=%s phase=%s trafficImpact=not-run\n", *profile, *marker, *phase)
	adapter, err := awsedge.NewNativeSDKLifecycleAdapter(ctx, probe, edgeOperationPolicy(), awsconfig.WithSharedConfigProfile(*profile), awsconfig.WithRegion("us-east-1"))
	if err != nil {
		return fmt.Errorf("construct CloudFront lifecycle adapter: %w", err)
	}

	fmt.Fprintf(output, "+ cloudfront: planning native distribution domain=%s originGroup=%t\n", *domain, *originGroup)
	plan, err := adapter.PlanEdge(ctx, request)
	if err != nil {
		return fmt.Errorf("plan CloudFront acceptance cell: %w", err)
	}

	var applied sdk.EdgeExecutionResult
	var distributionDomain string
	if *phase == "destroy" || *phase == "failover" {
		state, err := readEdgeState(*statePath)
		if err != nil {
			return err
		}
		if state.Marker != plan.OwnershipMarker {
			return errors.New("state ownership marker does not match the plan")
		}
		applied.ResourceRefs = state.ResourceRefs
		distributionDomain = state.DistributionDomain
		if *phase == "failover" {
			if !*originGroup {
				return errors.New("failover phase requires --origin-group")
			}
			fmt.Fprintf(output, "+ cloudfront: failing over origin-group members and waiting for Deployed\n")
			failedOver, err := adapter.ExecuteEdge(ctx, sdk.EdgeExecutionRequest{
				Plan: plan, Action: sdk.EdgeFailover, IdempotencyKey: "cloudfront-acceptance-failover",
				OwnershipMarker: plan.OwnershipMarker, ResourceReferences: applied.ResourceRefs,
			})
			if err != nil {
				return fmt.Errorf("failover CloudFront acceptance cell: %w", err)
			}
			if !containsProof(failedOver.ProofRefs, "aws.cloudfront.failover-control-plane-converged") {
				return fmt.Errorf("failover did not converge origin-group members: proofs=%#v", failedOver.ProofRefs)
			}
			fmt.Fprintf(output, "AWS CloudFront acceptance FAILOVER marker=%s domain=%s distributionDomainName=%s waf=%s trafficImpact=failover-control-plane\n",
				*marker, *domain, distributionDomain, wafStatus(*webACLID))
			return nil
		}
	} else {
		fmt.Fprintf(output, "+ cloudfront: applying distribution and waiting for Deployed with purge\n")
		applied, err = adapter.ExecuteEdge(ctx, sdk.EdgeExecutionRequest{
			Plan: plan, Action: sdk.EdgeApply, IdempotencyKey: "cloudfront-acceptance-apply", OwnershipMarker: plan.OwnershipMarker,
		})
		if err != nil {
			return fmt.Errorf("apply CloudFront acceptance cell: %w", err)
		}
		if !containsProof(applied.ProofRefs, "aws.cloudfront.purge-complete") {
			return fmt.Errorf("apply did not complete purge: proofs=%#v", applied.ProofRefs)
		}
		if strings.TrimSpace(*webACLID) != "" && !containsProof(applied.ProofRefs, "aws.cloudfront.waf") {
			return fmt.Errorf("apply did not attach WAF: proofs=%#v", applied.ProofRefs)
		}
		distributionDomain = outputValue(applied.Outputs, "distributionDomainName")
		if distributionDomain == "" {
			return errors.New("apply did not return distributionDomainName")
		}
		fmt.Fprintf(output, "+ cloudfront: deployed distributionDomainName=%s\n", distributionDomain)
		if strings.TrimSpace(*statePath) != "" {
			if err := writeEdgeState(*statePath, edgeState{
				Marker: plan.OwnershipMarker, Domain: *domain, DistributionDomain: distributionDomain, ResourceRefs: applied.ResourceRefs,
			}); err != nil {
				return err
			}
		}
		if *phase == "apply" {
			fmt.Fprintf(output, "AWS CloudFront acceptance APPLY marker=%s domain=%s distributionDomainName=%s waf=%s trafficImpact=not-run\n",
				*marker, *domain, distributionDomain, wafStatus(*webACLID))
			return nil
		}
	}

	fmt.Fprintf(output, "+ cloudfront: destroying distribution through adapter disable-then-delete path\n")
	destroyed, err := adapter.ExecuteEdge(ctx, sdk.EdgeExecutionRequest{
		Plan: plan, Action: sdk.EdgeDestroy, IdempotencyKey: "cloudfront-acceptance-destroy",
		OwnershipMarker: plan.OwnershipMarker, ResourceReferences: applied.ResourceRefs,
	})
	if err != nil {
		return fmt.Errorf("destroy CloudFront acceptance cell: %w", err)
	}
	if destroyed.Action != sdk.EdgeDestroy {
		return fmt.Errorf("unexpected destroy action: %#v", destroyed)
	}

	nativeAPI, err := awsedge.NewCloudFrontSDKClient(ctx, probe, awsconfig.WithSharedConfigProfile(*profile), awsconfig.WithRegion("us-east-1"))
	if err != nil {
		return fmt.Errorf("construct CloudFront inventory client: %w", err)
	}
	remaining, err := nativeAPI.Inventory(ctx, plan.OwnershipMarker)
	if err != nil {
		return fmt.Errorf("verify CloudFront ownership inventory: %w", err)
	}
	if len(remaining) != 0 {
		return fmt.Errorf("CloudFront ownership inventory not empty after destroy: %#v", remaining)
	}

	fmt.Fprintf(output, "AWS CloudFront acceptance PASS marker=%s domain=%s distributionDomainName=%s waf=%s trafficImpact=not-run cleanup=verified\n",
		*marker, *domain, distributionDomain, wafStatus(*webACLID))
	return nil
}

type edgeState struct {
	Marker             string   `json:"marker"`
	Domain             string   `json:"domain"`
	DistributionDomain string   `json:"distributionDomainName"`
	ResourceRefs       []string `json:"resourceRefs"`
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

func acceptanceRequest(marker, domain, certificateARN, primaryOrigin, secondaryOrigin string, originGroup bool, webACLID string) (sdk.EdgePlanRequest, *httpsOriginHealthProbe, error) {
	request := sdk.EdgePlanRequest{
		TargetProvider: "aws",
		TargetRuntime:  "cloudfront-live-cell",
		Intent: sdk.EdgeIntent{
			Mode:            "native",
			NativeProvider:  "cloudfront",
			Domains:         []string{domain},
			TLS:             true,
			TLSMode:         "managed",
			DNSMode:         "external",
			OriginHealthRef: healthRefPrimary,
			OwnershipMarker: marker,
			PurgeOnDeploy:   true,
		},
		Configuration: map[string]any{
			"certificateARN": certificateARN,
		},
	}
	if strings.TrimSpace(webACLID) != "" {
		if err := validatePart(webACLID, "web-acl-id"); err != nil {
			return sdk.EdgePlanRequest{}, nil, err
		}
		if !strings.HasPrefix(webACLID, "arn:aws:wafv2:us-east-1:") || !strings.Contains(webACLID, ":global/webacl/") {
			return sdk.EdgePlanRequest{}, nil, errors.New("web-acl-id must reference a CLOUDFRONT-scope WAFv2 WebACL in us-east-1")
		}
		request.Intent.WAFPolicyRef = waf.PolicyRef
		request.Configuration["webACLID"] = webACLID
	}
	probe := &httpsOriginHealthProbe{hosts: map[string]string{healthRefPrimary: primaryOrigin}}
	if originGroup {
		request.Intent.FailoverPolicyRef = "policy/cloudfront-origin-failover"
		request.Configuration["originGroup"] = map[string]any{
			"id": "magelift-origin-group",
			"primary": map[string]any{
				"kind": "custom", "domainName": primaryOrigin, "id": "primary-origin",
			},
			"secondary": map[string]any{
				"kind": "custom", "domainName": secondaryOrigin, "id": "secondary-origin",
			},
			"primaryHealthRef":   healthRefPrimary,
			"secondaryHealthRef": healthRefSecondary,
		}
		probe.hosts[healthRefSecondary] = secondaryOrigin
	} else {
		request.Configuration["origin"] = map[string]any{
			"kind": "custom", "domainName": primaryOrigin, "id": "primary-origin",
		}
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

func (probe *httpsOriginHealthProbe) httpClient() *http.Client {
	if probe.client != nil {
		return probe.client
	}
	return &http.Client{Timeout: 20 * time.Second}
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

func wafStatus(webACLID string) string {
	if strings.TrimSpace(webACLID) == "" {
		return "not-attached"
	}
	return "attached"
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
