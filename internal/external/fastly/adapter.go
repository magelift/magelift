// Package fastly contains the explicitly registered Fastly edge adapter.
//
// The adapter shells out to the authenticated Fastly CLI instead of accepting
// a token value from MageLift. This keeps credential resolution in the user's
// provider account or secret integration and gives tests a small command
// boundary to verify.
package fastly

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/magelift/magelift/internal/edge/waf"
	"github.com/magelift/magelift/internal/secretref"
	"github.com/magelift/magelift/sdk"
)

const ownershipPrefix = "magelift/"

// Fastly limits an individual custom VCL file to 1 MiB. Validate this in the
// shared request path so both the CLI and official-SDK lifecycles reject an
// invalid request before creating a service or version.
const maxFastlyVCLBytes = 1 << 20

// Fastly service IDs are opaque alphanumeric values, not UUIDs. Keep the
// parser narrow enough to avoid returning a flag value or a word from the
// human-readable create output.
var serviceIDPattern = regexp.MustCompile(`(?i)\b[a-z0-9]{20,32}\b`)
var genericIDPattern = regexp.MustCompile(`(?i)\b[a-z0-9][a-z0-9_-]{5,63}\b`)
var tlsSubscriptionOutputPattern = regexp.MustCompile(`(?i)(?:tls[ -]?)?subscription[^a-z0-9]+['"]?([a-z0-9]{20,32})`)
var fastlyVersionPattern = regexp.MustCompile(`^(?:latest|active|staged|[0-9]+)$`)
var genericHostPattern = regexp.MustCompile(`(?i)^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`)
var fastlyBackendNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)

type Runner interface {
	Run(context.Context, []string) ([]byte, error)
}

type CommandRunner func(context.Context, []string) ([]byte, error)

func (runner CommandRunner) Run(ctx context.Context, args []string) ([]byte, error) {
	return runner(ctx, args)
}

type ExecRunner struct {
	Binary string
}

func (runner ExecRunner) Run(ctx context.Context, args []string) ([]byte, error) {
	if ctx == nil {
		return nil, errors.New("Fastly command context is required")
	}
	binary := runner.Binary
	if binary == "" {
		binary = "fastly"
	}
	commandArgs, err := fastlyCLIArgs(args)
	if err != nil {
		return nil, err
	}
	command := exec.CommandContext(ctx, binary, commandArgs...)
	data, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("fastly %s: %w: %s", strings.Join(commandArgs, " "), err, strings.TrimSpace(string(data)))
	}
	return data, nil
}

// fastlyCLIArgs selects authentication without resolving a secret in MageLift.
// Precedence matches the Fastly CLI: FASTLY_API_TOKEN, then an explicit stored
// token name, then the CLI's current login session. Do not default the token
// name to "default": `--token default` selects a named stored token, which is
// a different credential from omitting `--token` after `fastly auth login`.
func fastlyCLIArgs(args []string) ([]string, error) {
	if strings.TrimSpace(os.Getenv("FASTLY_API_TOKEN")) != "" {
		return append([]string(nil), args...), nil
	}
	tokenName := strings.TrimSpace(os.Getenv("MAGELIFT_FASTLY_TOKEN_NAME"))
	if tokenName == "" {
		return append([]string(nil), args...), nil
	}
	if strings.ContainsAny(tokenName, "\r\n\x00") {
		return nil, errors.New("MAGELIFT_FASTLY_TOKEN_NAME must be a single-line token name")
	}
	return append([]string{"--token", tokenName}, args...), nil
}

type Request struct {
	ServiceID       string
	ServiceName     string
	CredentialRef   string
	Domains         []string
	OwnershipMarker string
	PurgeOnDeploy   bool
	CreateService   bool
	ProviderConfig  Config
}

// Config is Fastly-owned configuration. It is intentionally opaque to the
// portable core: the edge contract carries references and composition, while
// this adapter owns service-version, VCL, TLS, and DNS API details.
type Config struct {
	Version                   string `json:"version" yaml:"version"`
	ActivateVersion           bool   `json:"activateVersion,omitempty" yaml:"activateVersion,omitempty"`
	TLS                       bool   `json:"tls" yaml:"tls"`
	TLSMode                   string `json:"tlsMode,omitempty" yaml:"tlsMode,omitempty"`
	DNSMode                   string `json:"dnsMode,omitempty" yaml:"dnsMode,omitempty"`
	TLSSubscriptionID         string `json:"tlsSubscriptionId,omitempty" yaml:"tlsSubscriptionId,omitempty"`
	VCLName                   string `json:"vclName,omitempty" yaml:"vclName,omitempty"`
	VCLContent                string `json:"vclContent,omitempty" yaml:"vclContent,omitempty"`
	VCLRef                    string `json:"vclRef,omitempty" yaml:"vclRef,omitempty"`
	OriginHealthRef           string `json:"originHealthRef" yaml:"originHealthRef"`
	OriginAddress             string `json:"originAddress,omitempty" yaml:"originAddress,omitempty"`
	OriginHost                string `json:"originHost,omitempty" yaml:"originHost,omitempty"`
	OriginPort                int    `json:"originPort,omitempty" yaml:"originPort,omitempty"`
	OriginUseTLS              bool   `json:"originUseTls,omitempty" yaml:"originUseTls,omitempty"`
	BackendName               string `json:"backendName,omitempty" yaml:"backendName,omitempty"`
	RouteSnippetName          string `json:"routeSnippetName,omitempty" yaml:"routeSnippetName,omitempty"`
	CachePolicyRef            string `json:"cachePolicyRef,omitempty" yaml:"cachePolicyRef,omitempty"`
	PurgePolicyRef            string `json:"purgePolicyRef,omitempty" yaml:"purgePolicyRef,omitempty"`
	WAFPolicyRef              string `json:"wafPolicyRef,omitempty" yaml:"wafPolicyRef,omitempty"`
	FailoverPolicyRef         string `json:"failoverPolicyRef,omitempty" yaml:"failoverPolicyRef,omitempty"`
	HealthOriginURL           string `json:"healthOriginUrl,omitempty" yaml:"healthOriginUrl,omitempty"`
	HealthOriginHost          string `json:"healthOriginHost,omitempty" yaml:"healthOriginHost,omitempty"`
	HealthExpectedCNAME       string `json:"healthExpectedCname,omitempty" yaml:"healthExpectedCname,omitempty"`
	HealthRoutePath           string `json:"healthRoutePath,omitempty" yaml:"healthRoutePath,omitempty"`
	HealthExpectedStatus      int    `json:"healthExpectedStatus,omitempty" yaml:"healthExpectedStatus,omitempty"`
	HealthRouteTimeoutSeconds int    `json:"healthRouteTimeoutSeconds,omitempty" yaml:"healthRouteTimeoutSeconds,omitempty"`
	HealthRoutePollSeconds    int    `json:"healthRoutePollSeconds,omitempty" yaml:"healthRoutePollSeconds,omitempty"`
}

type Plan struct {
	ServiceID       string   `json:"serviceId" yaml:"serviceId"`
	ServiceName     string   `json:"serviceName,omitempty" yaml:"serviceName,omitempty"`
	OwnershipMarker string   `json:"ownershipMarker" yaml:"ownershipMarker"`
	CreateService   bool     `json:"createService" yaml:"createService"`
	Domains         []string `json:"domains" yaml:"domains"`
	PurgeOnDeploy   bool     `json:"purgeOnDeploy" yaml:"purgeOnDeploy"`
	CredentialRef   string   `json:"credentialRef" yaml:"credentialRef"`
	ProviderConfig  Config   `json:"providerConfig" yaml:"providerConfig"`
	OutputKeys      []string `json:"outputKeys" yaml:"outputKeys"`
}

type Output struct {
	Key   string `json:"key" yaml:"key"`
	Value string `json:"value" yaml:"value"`
}

type Result struct {
	ServiceID           string         `json:"serviceId" yaml:"serviceId"`
	CreatedService      bool           `json:"createdService" yaml:"createdService"`
	CreatedDomains      []string       `json:"createdDomains" yaml:"createdDomains"`
	OwnershipMarker     string         `json:"ownershipMarker" yaml:"ownershipMarker"`
	Version             string         `json:"version" yaml:"version"`
	TLSSubscriptionID   string         `json:"tlsSubscriptionId,omitempty" yaml:"tlsSubscriptionId,omitempty"`
	CreatedTLS          bool           `json:"createdTls" yaml:"createdTls"`
	TLSChallenges       []TLSChallenge `json:"tlsChallenges,omitempty" yaml:"tlsChallenges,omitempty"`
	VCLName             string         `json:"vclName,omitempty" yaml:"vclName,omitempty"`
	CreatedVCL          bool           `json:"createdVcl" yaml:"createdVcl"`
	VCLApplied          bool           `json:"vclApplied" yaml:"vclApplied"`
	BackendName         string         `json:"backendName,omitempty" yaml:"backendName,omitempty"`
	CreatedBackend      bool           `json:"createdBackend" yaml:"createdBackend"`
	RouteSnippetName    string         `json:"routeSnippetName,omitempty" yaml:"routeSnippetName,omitempty"`
	CreatedRouteSnippet bool           `json:"createdRouteSnippet" yaml:"createdRouteSnippet"`
	VersionActivated    bool           `json:"versionActivated" yaml:"versionActivated"`
	PurgeRequested      bool           `json:"purgeRequested" yaml:"purgeRequested"`
	FailoverApplied     bool           `json:"failoverApplied" yaml:"failoverApplied"`
	RollbackApplied     bool           `json:"rollbackApplied" yaml:"rollbackApplied"`
	Outputs             []Output       `json:"outputs" yaml:"outputs"`
}

// TLSChallenge is the provider-owned DNS proof Fastly returns for a managed
// certificate. The challenge is an output, not a DNS mutation: a separate DNS
// adapter (for example Cloudflare in acceptance) owns the record lifecycle.
type TLSChallenge struct {
	RecordName string   `json:"recordName" yaml:"recordName"`
	RecordType string   `json:"recordType" yaml:"recordType"`
	Type       string   `json:"type,omitempty" yaml:"type,omitempty"`
	Values     []string `json:"values" yaml:"values"`
}

// HealthObservation is the small provider-neutral safety result required by
// the Fastly adapter before and after a route mutation. The probe owns the
// origin protocol, DNS resolver, and application smoke request; the adapter
// never treats a non-empty OriginHealthRef as proof.
type HealthObservation struct {
	OriginHealthy        bool
	RouteHealthy         bool
	DNSOwnershipVerified bool
	TLSVerified          bool
	PurgeVerified        bool
	CachePolicyVerified  bool
	WAFVerified          bool
	FailoverVerified     bool
	RollbackVerified     bool
}

type HealthProbe interface {
	Preflight(context.Context, Request, string) (HealthObservation, error)
	Verify(context.Context, Request, Result) (HealthObservation, error)
}

// DeletionObservation is the owning-service inventory result after a delete
// request. A successful delete command is not enough because Fastly control
// plane deletion can be asynchronous.
type DeletionObservation struct {
	Complete     bool
	ResourceRefs []string
	Detail       string
}

type DeletionProbe interface {
	PollDeletion(context.Context, Request, Result) (DeletionObservation, error)
}

type DeletionPolicy struct {
	Timeout      time.Duration
	PollInterval time.Duration
	MaxAttempts  int
}

type domainRecord struct {
	ID          string
	FQDN        string
	Description string
}

type Adapter struct {
	Runner        Runner
	Probe         HealthProbe
	DeletionProbe DeletionProbe
}

func New(runner Runner) Adapter {
	return Adapter{Runner: runner}
}

func NewWithHealthProbe(runner Runner, probe HealthProbe) Adapter {
	return Adapter{Runner: runner, Probe: probe}
}

func NewWithDeletionProbe(runner Runner, probe DeletionProbe) Adapter {
	return Adapter{Runner: runner, DeletionProbe: probe}
}

func NewWithProbes(runner Runner, health HealthProbe, deletion DeletionProbe) Adapter {
	return Adapter{Runner: runner, Probe: health, DeletionProbe: deletion}
}

func (adapter Adapter) Plan(request Request) (Plan, error) {
	return planRequest(request, false)
}

// planRequest is shared with the native lifecycle so an explicitly injected
// failover controller can opt into the one policy operation it actually owns.
// The default CLI adapter has no failover mutation surface and therefore stays
// fail-closed.
func planRequest(request Request, allowFailover bool) (Plan, error) {
	if err := validateRequest(request); err != nil {
		return Plan{}, err
	}
	providerConfig := normalizeConfig(request.ProviderConfig)
	if err := validateFastlyPolicyCapabilities(providerConfig, allowFailover); err != nil {
		return Plan{}, err
	}
	domains := append([]string(nil), request.Domains...)
	sort.Strings(domains)
	return Plan{
		ServiceID:       request.ServiceID,
		ServiceName:     request.ServiceName,
		OwnershipMarker: request.OwnershipMarker,
		CreateService:   request.CreateService || request.ServiceID == "",
		Domains:         domains,
		PurgeOnDeploy:   request.PurgeOnDeploy,
		CredentialRef:   request.CredentialRef,
		ProviderConfig:  providerConfig,
		OutputKeys:      fastlyOutputKeys(),
	}, nil
}

func normalizeConfig(config Config) Config {
	if config.Version == "" {
		config.Version = "latest"
	}
	// The portable edge contract uses "external" when certificate ownership
	// and renewal stay outside MageLift. Fastly's equivalent is a customer-
	// provided certificate, for which this adapter must not create or renew a
	// TLS subscription.
	if config.TLSMode == "external" {
		config.TLSMode = "customer-managed"
	}
	return config
}

func (adapter Adapter) Apply(ctx context.Context, request Request) (result Result, err error) {
	if ctx == nil {
		return Result{}, errors.New("Fastly apply context is required")
	}
	if adapter.Runner == nil {
		return Result{}, errors.New("Fastly adapter command runner is required")
	}
	plan, err := adapter.Plan(request)
	if err != nil {
		return Result{}, err
	}
	if adapter.Probe == nil {
		return Result{}, errors.New("Fastly origin safety probe is required before apply")
	}
	if !plan.CreateService {
		if err := verifyOwnedService(ctx, adapter.Runner, plan.ServiceID, plan.OwnershipMarker); err != nil {
			return Result{}, err
		}
	}
	if !plan.CreateService && plan.ProviderConfig.OriginAddress != "" {
		return Result{}, errors.New("Fastly CLI origin mutations on an existing service require the version lifecycle adapter")
	}
	preflight, err := adapter.Probe.Preflight(ctx, request, plan.ServiceID)
	if err != nil {
		return Result{}, fmt.Errorf("Fastly origin preflight: %w", err)
	}
	if !preflight.OriginHealthy {
		return Result{}, errors.New("Fastly origin preflight did not verify a healthy origin")
	}
	result = Result{ServiceID: plan.ServiceID, CreatedService: plan.CreateService, OwnershipMarker: plan.OwnershipMarker, Version: plan.ProviderConfig.Version, Outputs: fastlyOutputs(plan, nil)}
	cleanupResult := Result{ServiceID: result.ServiceID, CreatedService: result.CreatedService, OwnershipMarker: result.OwnershipMarker, Version: result.Version}
	updateCleanupResult := func() {
		// Keep every successfully discovered child identity in the deferred
		// cleanup scope. A later verification or activation failure must not
		// turn a partially-created TLS subscription or domain into an orphan.
		cleanupResult = result
	}
	cleanupRequired := false
	defer func() {
		if err == nil || !cleanupRequired {
			return
		}
		cleanupContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		defer cancel()
		if cleanupErr := adapter.cleanupFailedApply(cleanupContext, request, cleanupResult); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("cleanup Fastly apply failure: %w", cleanupErr))
		}
	}()
	if plan.CreateService {
		cleanupRequired = true
		name := request.ServiceName
		if name == "" {
			name = request.OwnershipMarker
		}
		data, runErr := adapter.Runner.Run(ctx, []string{"-i", "-q", "service", "create", "--name", name, "--comment", request.OwnershipMarker, "--type=vcl"})
		if runErr != nil {
			return Result{}, fmt.Errorf("create Fastly service: %w", runErr)
		}
		result.ServiceID, err = parseServiceID(data)
		if err != nil {
			return Result{}, fmt.Errorf("read Fastly service ID: %w", err)
		}
		updateCleanupResult()
	}
	if plan.ProviderConfig.OriginAddress != "" {
		if err := waitForFastlyServiceVersion(ctx, adapter.Runner, result.ServiceID, plan.ProviderConfig.Version); err != nil {
			return Result{}, err
		}
		backendName := fastlyBackendName(plan.ProviderConfig)
		cleanupRequired = true
		cleanupResult.BackendName = backendName
		cleanupResult.CreatedBackend = true
		if _, runErr := adapter.Runner.Run(ctx, fastlyBackendCreateArgs(plan.ProviderConfig, result.ServiceID, plan.OwnershipMarker)); runErr != nil {
			return Result{}, fmt.Errorf("create Fastly origin backend %q: %w", backendName, runErr)
		}
		result.BackendName = backendName
		result.CreatedBackend = true
		updateCleanupResult()
		if plan.ProviderConfig.VCLContent == "" {
			snippetName := plan.ProviderConfig.RouteSnippetName
			if snippetName == "" {
				snippetName = "magelift-route"
			}
			snippetContent := "set req.backend = F_" + backendName + ";"
			cleanupResult.RouteSnippetName = snippetName
			cleanupResult.CreatedRouteSnippet = true
			if _, runErr := adapter.Runner.Run(ctx, []string{"-i", "-q", "service", "vcl", "snippet", "create", "--version", plan.ProviderConfig.Version, "--name", snippetName, "--content", snippetContent, "--type", "recv", "--priority", "10", "--service-id", result.ServiceID}); runErr != nil {
				return Result{}, fmt.Errorf("create Fastly route snippet %q: %w", snippetName, runErr)
			}
			result.RouteSnippetName = snippetName
			result.CreatedRouteSnippet = true
			updateCleanupResult()
		}
	}
	existing, err := adapter.listDomains(ctx, result.ServiceID)
	if err != nil {
		return Result{}, fmt.Errorf("list Fastly domains: %w", err)
	}
	owned := make(map[string]domainRecord, len(existing))
	for _, domain := range existing {
		if domain.Description == plan.OwnershipMarker {
			owned[domain.FQDN] = domain
		}
	}
	for _, domain := range plan.Domains {
		if _, exists := owned[domain]; exists {
			result.CreatedDomains = append(result.CreatedDomains, domain)
			continue
		}
		cleanupRequired = true
		cleanupResult.CreatedDomains = append(cleanupResult.CreatedDomains, domain)
		_, err := adapter.Runner.Run(ctx, []string{"-i", "-q", "domain", "create", "--fqdn", domain, "--description", request.OwnershipMarker, "--service-id", result.ServiceID})
		if err != nil {
			return Result{}, fmt.Errorf("create Fastly domain %q: %w", domain, err)
		}
		result.CreatedDomains = append(result.CreatedDomains, domain)
		updateCleanupResult()
	}
	if plan.ProviderConfig.VCLContent != "" {
		if _, err := adapter.Runner.Run(ctx, []string{"-i", "-q", "service", "vcl", "custom", "create", "--version", plan.ProviderConfig.Version, "--name", plan.ProviderConfig.VCLName, "--content", plan.ProviderConfig.VCLContent, "--main", "--service-id", result.ServiceID}); err != nil {
			return Result{}, fmt.Errorf("upload Fastly VCL: %w", err)
		}
		result.VCLName = plan.ProviderConfig.VCLName
		result.CreatedVCL = true
		result.VCLApplied = true
	}
	if plan.ProviderConfig.TLSMode == "fastly-managed" && plan.ProviderConfig.TLSSubscriptionID == "" {
		args := []string{"-i", "-q", "tls-subscription", "create"}
		for _, domain := range plan.Domains {
			args = append(args, "--domain", domain)
		}
		data, err := adapter.Runner.Run(ctx, args)
		if err != nil {
			return Result{}, fmt.Errorf("create Fastly TLS subscription: %w", err)
		}
		result.TLSSubscriptionID, err = parseTLSSubscriptionID(data)
		if err != nil {
			return Result{}, fmt.Errorf("read Fastly TLS subscription ID: %w", err)
		}
		result.CreatedTLS = true
		updateCleanupResult()
		challengeData, runErr := adapter.Runner.Run(ctx, []string{"-i", "-q", "tls-subscription", "describe", "--id", result.TLSSubscriptionID, "--include=tls_authorizations", "--json"})
		if runErr != nil {
			return Result{}, fmt.Errorf("describe Fastly TLS challenges: %w", runErr)
		}
		result.TLSChallenges, err = parseTLSChallenges(challengeData)
		if err != nil {
			return Result{}, fmt.Errorf("read Fastly TLS challenges: %w", err)
		}
		updateCleanupResult()
	} else {
		result.TLSSubscriptionID = plan.ProviderConfig.TLSSubscriptionID
		updateCleanupResult()
	}
	if plan.ProviderConfig.ActivateVersion {
		if _, err := adapter.Runner.Run(ctx, []string{"-i", "-q", "service", "version", "activate", "--version", plan.ProviderConfig.Version, "--service-id", result.ServiceID}); err != nil {
			return Result{}, fmt.Errorf("activate Fastly service version: %w", err)
		}
		result.VersionActivated = true
		updateCleanupResult()
	}
	if plan.PurgeOnDeploy {
		if _, err := adapter.Runner.Run(ctx, []string{"-i", "-q", "service", "purge", "--service-id", result.ServiceID, "--all"}); err != nil {
			return Result{}, fmt.Errorf("purge Fastly service: %w", err)
		}
		result.PurgeRequested = true
		updateCleanupResult()
	}
	if err := adapter.verifyResult(ctx, request, plan, result); err != nil {
		return Result{}, err
	}
	result.Outputs = fastlyOutputs(plan, &result)
	return result, nil
}

func (adapter Adapter) Purge(ctx context.Context, request Request) (Result, error) {
	if ctx == nil {
		return Result{}, errors.New("Fastly purge context is required")
	}
	if adapter.Runner == nil {
		return Result{}, errors.New("Fastly runner is required")
	}
	if err := validateRequest(request); err != nil {
		return Result{}, err
	}
	serviceID := strings.TrimSpace(request.ServiceID)
	if serviceID == "" {
		return Result{}, errors.New("Fastly purge requires a service ID")
	}
	if _, err := adapter.Runner.Run(ctx, []string{"-i", "-q", "service", "purge", "--service-id", serviceID, "--all"}); err != nil {
		return Result{}, fmt.Errorf("purge Fastly service: %w", err)
	}
	return Result{ServiceID: serviceID, PurgeRequested: true, OwnershipMarker: request.OwnershipMarker}, nil
}

func waitForFastlyServiceVersion(ctx context.Context, runner Runner, serviceID, requestedVersion string) error {
	if ctx == nil {
		return errors.New("Fastly service version wait context is required")
	}
	if runner == nil || strings.TrimSpace(serviceID) == "" {
		return errors.New("Fastly service version wait requires a runner and service ID")
	}
	requestedVersion = strings.TrimSpace(requestedVersion)
	deadline := time.Now().Add(45 * time.Second)
	for {
		data, err := runner.Run(ctx, []string{"-i", "-q", "service", "describe", "--json", "--service-id", serviceID})
		if err == nil && fastlyServiceVersionAvailable(data, requestedVersion) {
			return nil
		}
		if time.Now().After(deadline) {
			if err != nil {
				return fmt.Errorf("wait for Fastly service version: %w", err)
			}
			return errors.New("wait for Fastly service version: no requested version became available")
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("wait for Fastly service version: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

func fastlyServiceVersionAvailable(data []byte, requested string) bool {
	var value any
	if json.Unmarshal(data, &value) != nil {
		return false
	}
	available := false
	walkJSON(value, func(object map[string]any) {
		if available {
			return
		}
		number := objectString(object, "number")
		if number == "" {
			for name, value := range object {
				if !strings.EqualFold(name, "number") {
					continue
				}
				if numeric, ok := value.(float64); ok {
					number = strconv.Itoa(int(numeric))
				}
				break
			}
		}
		if number == "" {
			return
		}
		available = requested == "" || requested == "latest" || requested == "active" || requested == "staged" || requested == number
	})
	return available
}

func (adapter Adapter) cleanupFailedApply(ctx context.Context, request Request, result Result) error {
	if result.CreatedService && result.ServiceID == "" {
		serviceID, err := adapter.findOwnedServiceID(ctx, request.OwnershipMarker)
		if err != nil {
			return err
		}
		if serviceID == "" {
			return errors.New("could not rediscover the partially created Fastly service by ownership marker")
		}
		result.ServiceID = serviceID
	}
	if result.ServiceID == "" {
		return nil
	}
	return adapter.Destroy(ctx, request, result)
}

func (adapter Adapter) findOwnedServiceID(ctx context.Context, marker string) (string, error) {
	data, err := adapter.Runner.Run(ctx, []string{"-i", "-q", "service", "list", "--json"})
	if err != nil {
		return "", fmt.Errorf("list Fastly services for partial cleanup: %w", err)
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return "", fmt.Errorf("decode Fastly services for partial cleanup: %w", err)
	}
	var serviceID string
	walkJSON(value, func(object map[string]any) {
		if serviceID != "" || objectString(object, "comment") != marker {
			return
		}
		serviceID = objectString(object, "id")
		if serviceID == "" {
			serviceID = objectString(object, "service_id")
		}
	})
	return serviceID, nil
}

// Verify checks the current route and origin through the injected safety
// probe without applying configuration. It is the provider-specific half of
// the public SDK EdgeVerify lifecycle action.
func (adapter Adapter) Verify(ctx context.Context, request Request, result Result) error {
	if adapter.Probe == nil {
		return errors.New("Fastly origin safety probe is required before verification")
	}
	plan, err := adapter.Plan(request)
	if err != nil {
		return err
	}
	return adapter.verifyResult(ctx, request, plan, result)
}

func (adapter Adapter) verifyResult(ctx context.Context, request Request, plan Plan, result Result) error {
	if adapter.Probe == nil {
		return errors.New("Fastly origin safety probe is required before verification")
	}
	postflight, err := adapter.Probe.Verify(ctx, request, result)
	if err != nil {
		return fmt.Errorf("Fastly route verification: %w", err)
	}
	if !postflight.OriginHealthy || !postflight.RouteHealthy {
		return fmt.Errorf("Fastly route verification did not verify a healthy origin and route (origin=%t route=%t dns=%t tls=%t purge=%t)", postflight.OriginHealthy, postflight.RouteHealthy, postflight.DNSOwnershipVerified, postflight.TLSVerified, postflight.PurgeVerified)
	}
	if len(plan.Domains) > 0 && !postflight.DNSOwnershipVerified {
		return errors.New("Fastly route verification did not verify DNS ownership")
	}
	if plan.ProviderConfig.TLS && !postflight.TLSVerified {
		return errors.New("Fastly route verification did not verify TLS")
	}
	if plan.PurgeOnDeploy && !postflight.PurgeVerified {
		return errors.New("Fastly route verification did not verify purge completion")
	}
	if plan.ProviderConfig.CachePolicyRef != "" && !postflight.CachePolicyVerified {
		return errors.New("Fastly route verification did not verify the cache policy")
	}
	if plan.ProviderConfig.WAFPolicyRef != "" && !postflight.WAFVerified {
		return errors.New("Fastly route verification did not verify the WAF or security policy")
	}
	if plan.ProviderConfig.FailoverPolicyRef != "" && (!postflight.FailoverVerified || !postflight.RollbackVerified) {
		return errors.New("Fastly route verification did not verify failover and rollback")
	}
	return nil
}

// Destroy removes only domains whose current Fastly comment exactly matches
// the run marker. An existing service is never deleted; a newly created one is
// deleted only after its service comment is checked through describe.
func (adapter Adapter) Destroy(ctx context.Context, request Request, result Result) error {
	if ctx == nil {
		return errors.New("Fastly cleanup context is required")
	}
	if adapter.Runner == nil {
		return errors.New("Fastly adapter command runner is required")
	}
	if strings.TrimSpace(result.ServiceID) == "" || result.OwnershipMarker != request.OwnershipMarker {
		return errors.New("Fastly cleanup ownership proof does not match the request")
	}
	if adapter.DeletionProbe == nil {
		return errors.New("Fastly deletion inventory probe is required before cleanup")
	}
	plan, err := adapter.Plan(request)
	if err != nil {
		return err
	}
	if !result.CreatedService {
		if err := verifyOwnedService(ctx, adapter.Runner, result.ServiceID, request.OwnershipMarker); err != nil {
			return err
		}
	}
	var cleanupErr error
	recordCleanupError := func(format string, args ...any) {
		cleanupErr = errors.Join(cleanupErr, fmt.Errorf(format, args...))
	}
	owned, err := adapter.listDomains(ctx, result.ServiceID)
	if err != nil {
		// Versionless domains survive service deletion. Fall back to the
		// account-level Domain Management inventory when the service-scoped
		// lookup is unavailable, but still filter by the exact marker and
		// domains recorded in the result before mutating anything.
		fallbackOwned, fallbackErr := listAllDomainsWithRunner(ctx, adapter.Runner)
		if fallbackErr != nil {
			recordCleanupError("list Fastly domains for cleanup: %w (account inventory fallback: %v)", err, fallbackErr)
			owned = nil
		} else {
			owned = fallbackOwned
		}
	}
	owned = filterOwnedDomains(owned, request.OwnershipMarker, result.CreatedDomains)
	// Fastly-managed TLS owns the domain activation. Remove that dependency
	// before deleting the versionless domain; deleting in the opposite order
	// leaves a pending/active TLS subscription behind and the domain delete is
	// rejected by the API.
	if result.CreatedTLS && result.TLSSubscriptionID != "" {
		if _, err := adapter.Runner.Run(ctx, []string{"-i", "-q", "tls-subscription", "delete", "--id", result.TLSSubscriptionID, "--force"}); err != nil {
			if !isFastlyNotFound(err) {
				recordCleanupError("delete Fastly TLS subscription: %w", err)
			}
		}
	}
	for _, domain := range owned {
		if _, err := adapter.Runner.Run(ctx, []string{"-i", "-q", "domain", "delete", "--domain-id", domain.ID}); err != nil {
			if !isFastlyNotFound(err) {
				recordCleanupError("delete Fastly domain %q: %w", domain.FQDN, err)
			}
		}
	}
	if result.CreatedBackend && result.BackendName != "" && !result.CreatedService {
		if _, err := adapter.Runner.Run(ctx, fastlyBackendDeleteArgs(plan.ProviderConfig, result.BackendName, result.ServiceID)); err != nil {
			if !isFastlyNotFound(err) {
				recordCleanupError("delete Fastly origin backend %q: %w", result.BackendName, err)
			}
		}
	}
	if result.CreatedRouteSnippet && result.RouteSnippetName != "" && !result.CreatedService {
		if _, err := adapter.Runner.Run(ctx, []string{"-i", "-q", "service", "vcl", "snippet", "delete", "--version", plan.ProviderConfig.Version, "--name", result.RouteSnippetName, "--service-id", result.ServiceID}); err != nil && !isFastlyNotFound(err) {
			recordCleanupError("delete Fastly route snippet %q: %w", result.RouteSnippetName, err)
		}
	}
	if result.VCLApplied && plan.ProviderConfig.VCLName != "" {
		if _, err := adapter.Runner.Run(ctx, []string{"-i", "-q", "service", "vcl", "custom", "delete", "--version", plan.ProviderConfig.Version, "--name", plan.ProviderConfig.VCLName, "--service-id", result.ServiceID}); err != nil {
			if !isFastlyNotFound(err) {
				recordCleanupError("delete Fastly VCL %q: %w", plan.ProviderConfig.VCLName, err)
			}
		}
	}
	if result.CreatedService {
		serviceData, err := adapter.Runner.Run(ctx, []string{"-i", "-q", "service", "describe", "--json", "--service-id", result.ServiceID})
		if err != nil {
			if !isFastlyNotFound(err) {
				recordCleanupError("describe Fastly service for cleanup: %w", err)
			}
		} else if !jsonHasString(serviceData, "comment", request.OwnershipMarker) {
			recordCleanupError("refusing to delete Fastly service without an exact ownership comment")
		} else if _, err := adapter.Runner.Run(ctx, []string{"-i", "-q", "service", "delete", "--force", "--service-id", result.ServiceID}); err != nil && !isFastlyNotFound(err) {
			recordCleanupError("delete Fastly service: %w", err)
		}
	}
	observation, err := WaitForDeletion(ctx, adapter.DeletionProbe, request, result, DeletionPolicy{Timeout: 2 * time.Minute, PollInterval: 2 * time.Second, MaxAttempts: 60})
	if err != nil {
		recordCleanupError("verify Fastly cleanup: %w", err)
	} else if !observation.Complete {
		recordCleanupError("verify Fastly cleanup: %s", deletionDetail(observation))
	}
	if cleanupErr != nil {
		return cleanupErr
	}
	return nil
}

func isFastlyNotFound(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "404 not found") || strings.Contains(message, "record not found")
}

func verifyOwnedService(ctx context.Context, runner Runner, serviceID, marker string) error {
	if strings.TrimSpace(serviceID) == "" {
		return errors.New("Fastly service identity is required for ownership verification")
	}
	data, err := runner.Run(ctx, []string{"-i", "-q", "service", "describe", "--json", "--service-id", serviceID})
	if err != nil {
		return fmt.Errorf("get Fastly service for ownership verification: %w", err)
	}
	if !jsonHasString(data, "id", serviceID) || !jsonHasString(data, "comment", marker) {
		return errors.New("refusing to mutate Fastly service without an exact ownership comment")
	}
	return nil
}

// WaitForDeletion keeps cleanup bounded and treats an unfinished owning
// service inventory as failure. It is intentionally separate from Apply so a
// later retry can resume cleanup without reapplying edge configuration.
func WaitForDeletion(ctx context.Context, probe DeletionProbe, request Request, result Result, policy DeletionPolicy) (DeletionObservation, error) {
	if ctx == nil {
		return DeletionObservation{}, errors.New("Fastly deletion context is required")
	}
	if probe == nil {
		return DeletionObservation{}, errors.New("Fastly deletion probe is required")
	}
	if policy.Timeout <= 0 || policy.PollInterval <= 0 || policy.MaxAttempts <= 0 {
		return DeletionObservation{}, errors.New("Fastly deletion policy requires positive timeout, poll interval, and max attempts")
	}
	observation, err := sdk.WaitForDeletion(ctx, fastlyDeletionClient{probe: probe, request: request, result: result}, request.OwnershipMarker, sdk.ResilienceOperationPolicy{
		Timeout: policy.Timeout, PollInterval: policy.PollInterval, MaxAttempts: policy.MaxAttempts,
	})
	converted := DeletionObservation{ResourceRefs: append([]string(nil), observation.ResourceRefs...)}
	converted.Complete = observation.Status == sdk.ResilienceOperationSucceeded && len(observation.ResourceRefs) == 0
	if err != nil {
		return converted, fmt.Errorf("Fastly deletion: %w", err)
	}
	return converted, nil
}

type fastlyDeletionClient struct {
	probe   DeletionProbe
	request Request
	result  Result
}

func (client fastlyDeletionClient) PollDeletion(ctx context.Context, _ string) (sdk.DeletionObservation, error) {
	observation, err := client.probe.PollDeletion(ctx, client.request, client.result)
	if err != nil {
		return sdk.DeletionObservation{}, err
	}
	status := sdk.ResilienceOperationPending
	if observation.Complete {
		status = sdk.ResilienceOperationSucceeded
	}
	return sdk.DeletionObservation{Status: status, ResourceRefs: append([]string(nil), observation.ResourceRefs...), Detail: observation.Detail}, nil
}

func deletionDetail(observation DeletionObservation) string {
	if strings.TrimSpace(observation.Detail) != "" {
		return observation.Detail
	}
	if len(observation.ResourceRefs) > 0 {
		return "owned resources remain: " + strings.Join(observation.ResourceRefs, ", ")
	}
	return "owning-service inventory did not confirm deletion"
}

// CLIInventoryDeletionProbe verifies deletion through the Fastly owning
// service inventories. It intentionally uses list commands rather than a tag
// index, because a delayed index entry is not proof that a billable object is
// still live and an absent index entry is not proof that it is gone.
type CLIInventoryDeletionProbe struct {
	Runner Runner
}

func NewCLIInventoryDeletionProbe(runner Runner) DeletionProbe {
	return CLIInventoryDeletionProbe{Runner: runner}
}

func (probe CLIInventoryDeletionProbe) PollDeletion(ctx context.Context, request Request, result Result) (DeletionObservation, error) {
	if probe.Runner == nil {
		return DeletionObservation{}, errors.New("Fastly inventory runner is required")
	}
	remaining := make([]string, 0)
	if result.CreatedService {
		data, err := probe.Runner.Run(ctx, []string{"-i", "-q", "service", "list", "--json"})
		if err != nil {
			return DeletionObservation{}, fmt.Errorf("list Fastly services: %w", err)
		}
		if jsonHasString(data, "id", result.ServiceID) {
			remaining = append(remaining, "service:"+result.ServiceID)
		}
	}
	domains, err := listAllDomainsWithRunner(ctx, probe.Runner)
	if err != nil {
		return DeletionObservation{}, fmt.Errorf("list Fastly domains: %w", err)
	}
	for _, domain := range filterOwnedDomains(domains, request.OwnershipMarker, result.CreatedDomains) {
		remaining = append(remaining, "domain:"+domain.FQDN)
	}
	if result.CreatedTLS && result.TLSSubscriptionID != "" {
		data, err := probe.Runner.Run(ctx, []string{"-i", "-q", "tls-subscription", "list", "--json"})
		if err != nil {
			return DeletionObservation{}, fmt.Errorf("list Fastly TLS subscriptions: %w", err)
		}
		if jsonHasString(data, "id", result.TLSSubscriptionID) {
			remaining = append(remaining, "tls-subscription:"+result.TLSSubscriptionID)
		}
	}
	return DeletionObservation{Complete: len(remaining) == 0, ResourceRefs: remaining}, nil
}

func validateRequest(request Request) error {
	if request.ServiceID == "" && request.ServiceName == "" && !request.CreateService {
		return errors.New("Fastly service ID or service name is required")
	}
	if request.CredentialRef != "" {
		if _, err := secretref.Parse(request.CredentialRef); err != nil {
			return fmt.Errorf("Fastly credential reference: %w", err)
		}
	}
	if !strings.HasPrefix(request.OwnershipMarker, ownershipPrefix) || strings.ContainsAny(request.OwnershipMarker, "\r\n ") {
		return fmt.Errorf("Fastly ownership marker must start with %q and contain no whitespace", ownershipPrefix)
	}
	if len(request.Domains) == 0 {
		return errors.New("at least one Fastly domain is required")
	}
	if err := validateConfig(request.ProviderConfig); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(request.Domains))
	for _, domain := range request.Domains {
		if domain == "" || strings.ContainsAny(domain, "\r\n /:") {
			return fmt.Errorf("invalid Fastly domain %q", domain)
		}
		if _, exists := seen[domain]; exists {
			return fmt.Errorf("duplicate Fastly domain %q", domain)
		}
		seen[domain] = struct{}{}
	}
	return nil
}

func validateFastlyPolicyCapabilities(config Config, allowFailover bool) error {
	if strings.TrimSpace(config.CachePolicyRef) != "" {
		return sdk.EdgeCapabilityError{
			AdapterID: "fastly.edge",
			Action:    sdk.EdgeApply,
			Status:    sdk.EdgeCapabilityUnsupported,
			Reason:    "Fastly cache policy lifecycle is not implemented by this adapter",
		}
	}
	if strings.TrimSpace(config.PurgePolicyRef) != "" {
		return sdk.EdgeCapabilityError{
			AdapterID: "fastly.edge",
			Action:    sdk.EdgeApply,
			Status:    sdk.EdgeCapabilityUnsupported,
			Reason:    "Fastly purge policy lifecycle is not implemented by this adapter",
		}
	}
	if policyRef := strings.TrimSpace(config.WAFPolicyRef); policyRef != "" {
		reason := "Fastly WAF policy lifecycle requires provider-owned Magento-safe NGWAF translation; opaque WAFPolicyRef is not executable"
		if policyRef == waf.PolicyRef {
			reason = "Fastly Magento-safe NGWAF translation is encoded (static/media/health_check exclusions, admin/GraphQL/REST in scope) but this adapter does not yet mutate a Fastly NGWAF workspace"
		}
		return sdk.EdgeCapabilityError{
			AdapterID: "fastly.edge",
			Action:    sdk.EdgeApply,
			Status:    sdk.EdgeCapabilityUnsupported,
			Reason:    reason,
		}
	}
	if strings.TrimSpace(config.FailoverPolicyRef) != "" && !allowFailover {
		return sdk.EdgeCapabilityError{
			AdapterID: "fastly.edge",
			Action:    sdk.EdgeApply,
			Status:    sdk.EdgeCapabilityUnsupported,
			Reason:    "Fastly failover lifecycle requires a provider-owned failover controller",
		}
	}
	return nil
}

func fastlyBackendName(config Config) string {
	if strings.TrimSpace(config.BackendName) != "" {
		return config.BackendName
	}
	return "magelift_origin"
}

func fastlyBackendVersion(config Config) string {
	if strings.TrimSpace(config.Version) == "" {
		return "latest"
	}
	return config.Version
}

func fastlyBackendPort(config Config) int {
	if config.OriginPort != 0 {
		return config.OriginPort
	}
	if config.OriginUseTLS {
		return 443
	}
	return 80
}

func fastlyBackendCreateArgs(config Config, serviceID, marker string) []string {
	originHost := config.OriginHost
	if originHost == "" {
		originHost = config.OriginAddress
	}
	args := []string{
		"-i", "-q", "service", "backend", "create",
		"--version", fastlyBackendVersion(config),
		"--name", fastlyBackendName(config),
		"--address", config.OriginAddress,
		"--port", strconv.Itoa(fastlyBackendPort(config)),
		"--service-id", serviceID,
		"--comment", marker,
		"--override-host", originHost,
	}
	if config.OriginUseTLS {
		args = append(args, "--use-ssl", "--ssl-check-cert", "--ssl-sni-hostname", originHost, "--ssl-cert-hostname", originHost)
	}
	return args
}

func fastlyBackendDeleteArgs(config Config, backendName, serviceID string) []string {
	return []string{
		"-i", "-q", "service", "backend", "delete",
		"--version", fastlyBackendVersion(config),
		"--name", backendName,
		"--service-id", serviceID,
	}
}

func validateConfig(config Config) error {
	if config.Version == "" {
		config.Version = "latest"
	}
	if !fastlyVersionPattern.MatchString(config.Version) {
		return fmt.Errorf("Fastly service version must be latest, active, staged, or a numeric version (got %q)", config.Version)
	}
	if config.TLS {
		switch config.TLSMode {
		case "fastly-managed":
		case "existing", "customer-managed", "external":
			if config.TLSSubscriptionID == "" && config.TLSMode == "existing" {
				return errors.New("Fastly existing TLS mode requires a TLS subscription ID")
			}
		case "":
			return errors.New("Fastly TLS requires an explicit TLS mode")
		default:
			return fmt.Errorf("unsupported Fastly TLS mode %q", config.TLSMode)
		}
	} else if config.TLSMode != "" && config.TLSMode != "existing" && config.TLSSubscriptionID != "" {
		return errors.New("Fastly TLS subscription metadata requires TLS to be enabled")
	}
	if config.DNSMode != "" && config.DNSMode != "external" && config.DNSMode != "fastly-managed" && config.DNSMode != "customer-managed" {
		return fmt.Errorf("unsupported Fastly DNS mode %q", config.DNSMode)
	}
	if config.VCLContent != "" && config.VCLName == "" {
		return errors.New("Fastly VCL content requires a VCL name")
	}
	if len([]byte(config.VCLContent)) > maxFastlyVCLBytes {
		return fmt.Errorf("Fastly VCL content exceeds the %d-byte per-file limit", maxFastlyVCLBytes)
	}
	if strings.TrimSpace(config.OriginHealthRef) == "" {
		return errors.New("Fastly origin health reference is required")
	}
	if config.OriginAddress == "" {
		if config.OriginHost != "" || config.OriginPort != 0 || config.OriginUseTLS || config.BackendName != "" {
			return errors.New("Fastly origin address is required when origin backend settings are provided")
		}
		return nil
	}
	if !validFastlyHost(config.OriginAddress) {
		return fmt.Errorf("invalid Fastly origin address %q", config.OriginAddress)
	}
	if config.OriginHost != "" && !validFastlyHost(config.OriginHost) {
		return fmt.Errorf("invalid Fastly origin host %q", config.OriginHost)
	}
	if config.OriginPort < 0 || config.OriginPort > 65535 {
		return fmt.Errorf("invalid Fastly origin port %d", config.OriginPort)
	}
	if config.BackendName != "" && !fastlyBackendNamePattern.MatchString(config.BackendName) {
		return fmt.Errorf("invalid Fastly backend name %q", config.BackendName)
	}
	return nil
}

func validFastlyHost(value string) bool {
	if strings.ContainsAny(value, "\r\n /:") {
		return net.ParseIP(strings.Trim(value, "[]")) != nil
	}
	return genericHostPattern.MatchString(value)
}

func fastlyOutputKeys() []string {
	return []string{
		"edge.service_id", "edge.service_version", "edge.domain.0", "edge.origin_health_ref",
		"edge.origin_backend", "edge.version_activated", "edge.tls_subscription_id", "edge.tls.challenges", "edge.vcl_name", "edge.purge_requested",
	}
}

func fastlyOutputs(plan Plan, result *Result) []Output {
	serviceID := plan.ServiceID
	version := plan.ProviderConfig.Version
	if result != nil {
		serviceID = result.ServiceID
		version = result.Version
	}
	outputs := []Output{{Key: "edge.service_id", Value: serviceID}, {Key: "edge.service_version", Value: version}, {Key: "edge.origin_health_ref", Value: plan.ProviderConfig.OriginHealthRef}}
	backendName := plan.ProviderConfig.BackendName
	if backendName == "" && plan.ProviderConfig.OriginAddress != "" {
		backendName = fastlyBackendName(plan.ProviderConfig)
	}
	if result != nil && result.BackendName != "" {
		backendName = result.BackendName
	}
	if backendName != "" {
		outputs = append(outputs, Output{Key: "edge.origin_backend", Value: backendName})
	}
	if result != nil {
		outputs = append(outputs, Output{Key: "edge.version_activated", Value: fmt.Sprintf("%t", result.VersionActivated)})
	}
	for index, domain := range plan.Domains {
		outputs = append(outputs, Output{Key: fmt.Sprintf("edge.domain.%d", index), Value: domain})
	}
	tlsSubscriptionID := plan.ProviderConfig.TLSSubscriptionID
	if result != nil && result.TLSSubscriptionID != "" {
		tlsSubscriptionID = result.TLSSubscriptionID
	}
	if tlsSubscriptionID != "" {
		outputs = append(outputs, Output{Key: "edge.tls_subscription_id", Value: tlsSubscriptionID})
	}
	if result != nil && len(result.TLSChallenges) > 0 {
		if data, err := json.Marshal(result.TLSChallenges); err == nil {
			outputs = append(outputs, Output{Key: "edge.tls.challenges", Value: string(data)})
		}
	}
	if plan.ProviderConfig.VCLName != "" {
		outputs = append(outputs, Output{Key: "edge.vcl_name", Value: plan.ProviderConfig.VCLName})
	}
	if result != nil {
		outputs = append(outputs, Output{Key: "edge.purge_requested", Value: fmt.Sprintf("%t", result.PurgeRequested)})
	}
	return outputs
}

func parseServiceID(data []byte) (string, error) {
	if id := serviceIDPattern.Find(data); id != nil {
		return string(id), nil
	}
	return "", errors.New("CLI output did not contain an opaque Fastly service ID")
}

func parseTLSSubscriptionID(data []byte) (string, error) {
	var value any
	if json.Unmarshal(data, &value) == nil {
		var found string
		walkJSON(value, func(object map[string]any) {
			if found == "" {
				found = objectString(object, "id")
			}
		})
		if found != "" {
			return found, nil
		}
	}
	if id := tlsSubscriptionOutputPattern.FindSubmatch(data); len(id) == 2 {
		return string(id[1]), nil
	}
	trimmed := strings.TrimSpace(string(data))
	if serviceIDPattern.MatchString(trimmed) && serviceIDPattern.FindString(trimmed) == trimmed {
		return trimmed, nil
	}
	if id := genericIDPattern.Find(data); len(id) >= 20 {
		return string(id), nil
	}
	return "", errors.New("CLI output did not contain a TLS subscription ID")
}

func parseTLSChallenges(data []byte) ([]TLSChallenge, error) {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("decode TLS challenge response: %w", err)
	}
	challenges := make([]TLSChallenge, 0)
	walkJSON(value, func(object map[string]any) {
		recordName := objectString(object, "record_name")
		if recordName == "" {
			recordName = objectString(object, "recordName")
		}
		recordType := objectString(object, "record_type")
		if recordType == "" {
			recordType = objectString(object, "recordType")
		}
		if recordName == "" || recordType == "" {
			return
		}
		values := objectStrings(object, "values")
		if len(values) == 0 {
			return
		}
		challenges = append(challenges, TLSChallenge{RecordName: recordName, RecordType: recordType, Type: objectString(object, "type"), Values: values})
	})
	if len(challenges) == 0 {
		return nil, errors.New("TLS subscription response contained no DNS challenges")
	}
	sort.Slice(challenges, func(i, j int) bool {
		if challenges[i].RecordName == challenges[j].RecordName {
			return challenges[i].RecordType < challenges[j].RecordType
		}
		return challenges[i].RecordName < challenges[j].RecordName
	})
	return challenges, nil
}

func (adapter Adapter) listDomains(ctx context.Context, serviceID string) ([]domainRecord, error) {
	return listDomainsWithRunner(ctx, adapter.Runner, serviceID)
}

func listDomainsWithRunner(ctx context.Context, runner Runner, serviceID string) ([]domainRecord, error) {
	data, err := runner.Run(ctx, []string{"-i", "-q", "domain", "list", "--json", "--service-id", serviceID})
	if err != nil {
		return nil, err
	}
	return parseDomains(data)
}

func listAllDomainsWithRunner(ctx context.Context, runner Runner) ([]domainRecord, error) {
	data, err := runner.Run(ctx, []string{"-i", "-q", "domain", "list", "--json"})
	if err != nil {
		return nil, err
	}
	return parseDomains(data)
}

func parseDomains(data []byte) ([]domainRecord, error) {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("decode domain list: %w", err)
	}
	var result []domainRecord
	walkJSON(value, func(object map[string]any) {
		id := objectString(object, "id")
		fqdn := objectString(object, "fqdn")
		description := objectString(object, "description")
		if id != "" && fqdn != "" {
			result = append(result, domainRecord{ID: id, FQDN: fqdn, Description: description})
		}
	})
	sort.Slice(result, func(i, j int) bool { return result[i].FQDN < result[j].FQDN })
	return result, nil
}

func filterOwnedDomains(domains []domainRecord, marker string, expected []string) []domainRecord {
	allowed := make(map[string]struct{}, len(expected))
	for _, domain := range expected {
		allowed[domain] = struct{}{}
	}
	result := make([]domainRecord, 0, len(domains))
	for _, domain := range domains {
		if domain.Description != marker {
			continue
		}
		if _, exists := allowed[domain.FQDN]; exists {
			result = append(result, domain)
		}
	}
	return result
}

func jsonHasString(data []byte, key, want string) bool {
	var value any
	if json.Unmarshal(data, &value) != nil {
		return false
	}
	found := false
	walkJSON(value, func(object map[string]any) {
		if value := objectString(object, key); value == want {
			found = true
		}
	})
	return found
}

func objectString(object map[string]any, key string) string {
	for name, value := range object {
		if !strings.EqualFold(name, key) {
			continue
		}
		text, _ := value.(string)
		return text
	}
	return ""
}

func objectStrings(object map[string]any, key string) []string {
	for name, value := range object {
		if !strings.EqualFold(name, key) {
			continue
		}
		values, ok := value.([]any)
		if !ok {
			return nil
		}
		result := make([]string, 0, len(values))
		for _, value := range values {
			if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
				result = append(result, text)
			}
		}
		return result
	}
	return nil
}

func walkJSON(value any, visit func(map[string]any)) {
	switch typed := value.(type) {
	case map[string]any:
		visit(typed)
		for _, child := range typed {
			walkJSON(child, visit)
		}
	case []any:
		for _, child := range typed {
			walkJSON(child, visit)
		}
	}
}
