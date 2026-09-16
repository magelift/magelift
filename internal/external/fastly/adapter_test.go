package fastly

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/magelift/magelift/internal/edge/waf"
	"github.com/magelift/magelift/sdk"
)

type fakeRunner struct {
	outputs [][]byte
	args    [][]string
	failAt  int
	err     error
}

func TestFastlyCLIArgsSelectsStoredTokenWithoutResolvingIt(t *testing.T) {
	t.Setenv("FASTLY_API_TOKEN", "")
	t.Setenv("MAGELIFT_FASTLY_TOKEN_NAME", "certification")
	args, err := fastlyCLIArgs([]string{"-i", "-q", "service", "list", "--json"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--token", "certification", "-i", "-q", "service", "list", "--json"}
	if strings.Join(args, " ") != strings.Join(want, " ") {
		t.Fatalf("stored-token args = %#v, want %#v", args, want)
	}
	if strings.Contains(strings.Join(args, " "), "secret") {
		t.Fatalf("token secret entered CLI args: %#v", args)
	}
}

func TestFastlyCLIArgsPreserveRawEnvironmentTokenPrecedence(t *testing.T) {
	t.Setenv("FASTLY_API_TOKEN", "raw-token-is-not-returned")
	t.Setenv("MAGELIFT_FASTLY_TOKEN_NAME", "certification")
	args, err := fastlyCLIArgs([]string{"service", "list"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(args, " ") != "service list" {
		t.Fatalf("raw environment args = %#v, want no stored-token flag", args)
	}
}

func TestFastlyCLIArgsOmitTokenFlagWhenStoredNameUnset(t *testing.T) {
	t.Setenv("FASTLY_API_TOKEN", "")
	t.Setenv("MAGELIFT_FASTLY_TOKEN_NAME", "")
	args, err := fastlyCLIArgs([]string{"service", "list"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(args, " ") != "service list" {
		t.Fatalf("unset stored-token args = %#v, want the CLI login session", args)
	}
}

func TestFastlyCLIArgsSelectsExplicitDefaultNamedToken(t *testing.T) {
	t.Setenv("FASTLY_API_TOKEN", "")
	t.Setenv("MAGELIFT_FASTLY_TOKEN_NAME", "default")
	args, err := fastlyCLIArgs([]string{"service", "list"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--token", "default", "service", "list"}
	if strings.Join(args, " ") != strings.Join(want, " ") {
		t.Fatalf("explicit default named-token args = %#v, want %#v", args, want)
	}
}

func TestFastlyCLIArgsRejectInvalidStoredTokenName(t *testing.T) {
	t.Setenv("FASTLY_API_TOKEN", "")
	t.Setenv("MAGELIFT_FASTLY_TOKEN_NAME", "bad\nname")
	if _, err := fastlyCLIArgs([]string{"service", "list"}); err == nil || !strings.Contains(err.Error(), "single-line") {
		t.Fatalf("invalid stored token name error = %v", err)
	}
}

func (runner *fakeRunner) Run(_ context.Context, args []string) ([]byte, error) {
	runner.args = append(runner.args, append([]string(nil), args...))
	if runner.failAt > 0 && len(runner.args) == runner.failAt {
		if runner.err != nil {
			return nil, runner.err
		}
		return nil, fmt.Errorf("fake Fastly command failed at call %d", runner.failAt)
	}
	if len(runner.outputs) == 0 {
		return nil, nil
	}
	output := runner.outputs[0]
	runner.outputs = runner.outputs[1:]
	return output, nil
}

type fakeHealthProbe struct {
	preflight HealthObservation
	verify    HealthObservation
	calls     []string
}

type fakeDeletionProbe struct {
	completeAfter int
	calls         int
}

func (probe *fakeDeletionProbe) PollDeletion(_ context.Context, _ Request, _ Result) (DeletionObservation, error) {
	probe.calls++
	if probe.calls < probe.completeAfter {
		return DeletionObservation{ResourceRefs: []string{"service:pending"}}, nil
	}
	return DeletionObservation{Complete: true}, nil
}

func (probe *fakeHealthProbe) Preflight(_ context.Context, _ Request, _ string) (HealthObservation, error) {
	probe.calls = append(probe.calls, "preflight")
	return probe.preflight, nil
}

func (probe *fakeHealthProbe) Verify(_ context.Context, _ Request, _ Result) (HealthObservation, error) {
	probe.calls = append(probe.calls, "verify")
	return probe.verify, nil
}

func healthyFastlyProbe() *fakeHealthProbe {
	healthy := HealthObservation{
		OriginHealthy: true, RouteHealthy: true, DNSOwnershipVerified: true,
		TLSVerified: true, PurgeVerified: true, WAFVerified: true,
		FailoverVerified: true, RollbackVerified: true,
	}
	return &fakeHealthProbe{preflight: healthy, verify: healthy}
}

func completeFastlyDeletionProbe() *fakeDeletionProbe {
	return &fakeDeletionProbe{completeAfter: 1}
}

func fastlyRequest() Request {
	return Request{
		ServiceName:     "magelift-test",
		CredentialRef:   "aws-secrets-manager://magelift/fastly-token",
		Domains:         []string{"preview.example.com"},
		OwnershipMarker: "magelift/acceptance/test-1",
		PurgeOnDeploy:   true,
		ProviderConfig:  Config{OriginHealthRef: "health/magento"},
	}
}

func TestPlanRequiresSecretReferenceAndExactMarker(t *testing.T) {
	request := fastlyRequest()
	plan, err := New(&fakeRunner{}).Plan(request)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.CreateService || plan.CredentialRef != request.CredentialRef {
		t.Fatalf("plan = %#v", plan)
	}
	request.CredentialRef = "raw-token"
	if _, err := New(&fakeRunner{}).Plan(request); err == nil || !strings.Contains(err.Error(), "credential") {
		t.Fatalf("plaintext token accepted: %v", err)
	}
	request = fastlyRequest()
	request.OwnershipMarker = "test/unsafe"
	if _, err := New(&fakeRunner{}).Plan(request); err == nil || !strings.Contains(err.Error(), "ownership") {
		t.Fatalf("unsafe marker accepted: %v", err)
	}
}

func TestPlanRejectsUnsupportedPoliciesBeforeMutation(t *testing.T) {
	tests := []struct {
		name   string
		config Config
	}{
		{name: "cache policy", config: Config{CachePolicyRef: "cache/policy"}},
		{name: "purge policy", config: Config{PurgePolicyRef: "purge/policy"}},
		{name: "WAF policy", config: Config{WAFPolicyRef: "waf/policy"}},
		{name: "failover policy", config: Config{FailoverPolicyRef: "failover/policy"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &fakeRunner{}
			request := fastlyRequest()
			request.ProviderConfig = test.config
			request.ProviderConfig.OriginHealthRef = "health/magento"
			_, err := New(runner).Plan(request)
			var capabilityErr sdk.EdgeCapabilityError
			if !errors.As(err, &capabilityErr) || capabilityErr.Status != sdk.EdgeCapabilityUnsupported || capabilityErr.Action != sdk.EdgeApply {
				t.Fatalf("unsupported policy error = %v, typed = %#v", err, capabilityErr)
			}
			if len(runner.args) != 0 {
				t.Fatalf("unsupported policy reached Fastly before rejection: %#v", runner.args)
			}
		})
	}
}

func TestPlanRejectsMagentoSafeWAFUntilNGWAFMutation(t *testing.T) {
	runner := &fakeRunner{}
	request := fastlyRequest()
	request.ProviderConfig.WAFPolicyRef = waf.PolicyRef
	request.ProviderConfig.OriginHealthRef = "health/magento"
	_, err := New(runner).Plan(request)
	var capabilityErr sdk.EdgeCapabilityError
	if !errors.As(err, &capabilityErr) || capabilityErr.Status != sdk.EdgeCapabilityUnsupported {
		t.Fatalf("error = %v", err)
	}
	if !strings.Contains(capabilityErr.Reason, "does not yet mutate a Fastly NGWAF workspace") {
		t.Fatalf("reason = %q", capabilityErr.Reason)
	}
	if len(runner.args) != 0 {
		t.Fatalf("Magento-safe WAF reached Fastly before rejection: %#v", runner.args)
	}
}

func TestApplyRefusesMutationsWithoutOriginSafetyProbe(t *testing.T) {
	runner := &fakeRunner{}
	if _, err := New(runner).Apply(context.Background(), fastlyRequest()); err == nil || !strings.Contains(err.Error(), "safety probe") {
		t.Fatalf("apply without safety probe error = %v", err)
	}
	if len(runner.args) != 0 {
		t.Fatalf("Fastly mutated before safety probe: %#v", runner.args)
	}
}

func TestApplyRejectsUnownedExistingServiceBeforeMutation(t *testing.T) {
	runner := &fakeRunner{outputs: [][]byte{[]byte(`{"id":"existing-service","comment":"user-owned"}`)}}
	request := fastlyRequest()
	request.ServiceID = "existing-service"
	request.CreateService = false
	if _, err := NewWithHealthProbe(runner, healthyFastlyProbe()).Apply(context.Background(), request); err == nil || !strings.Contains(err.Error(), "ownership") {
		t.Fatalf("unowned existing service error = %v", err)
	}
	if len(runner.args) != 1 || hasFastlyCommand(runner.args, "service", "create") || hasFastlyCommand(runner.args, "domain", "create") {
		t.Fatalf("unowned existing service was mutated: %#v", runner.args)
	}
}

func TestApplyRequiresContext(t *testing.T) {
	var missingContext context.Context
	if _, err := New(&fakeRunner{}).Apply(missingContext, fastlyRequest()); err == nil || !strings.Contains(err.Error(), "context") {
		t.Fatalf("apply without context error = %v", err)
	}
}

func TestOriginBackendConfigurationIsProviderOwned(t *testing.T) {
	request := fastlyRequest()
	request.ProviderConfig = Config{
		OriginHealthRef: "health/live", OriginAddress: "origin.example.com", OriginHost: "origin.example.com",
		OriginPort: 443, OriginUseTLS: true, BackendName: "magelift_origin",
	}
	plan, err := New(&fakeRunner{}).Plan(request)
	if err != nil {
		t.Fatal(err)
	}
	args := fastlyBackendCreateArgs(plan.ProviderConfig, "service-123", request.OwnershipMarker)
	joined := strings.Join(args, " ")
	for _, expected := range []string{"service backend create", "--address origin.example.com", "--override-host origin.example.com", "--use-ssl", "--ssl-check-cert", "--ssl-sni-hostname origin.example.com", "--ssl-cert-hostname origin.example.com"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("backend args missing %q: %s", expected, joined)
		}
	}
}

func TestHTTPHealthProbeChecksOriginStatusAndFastlyHeaders(t *testing.T) {
	server := httptest.NewServer(httpHandler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Via", "1.1 varnish, 1.1 fastly")
		w.WriteHeader(200)
	}))
	defer server.Close()
	probe, err := NewHTTPHealthProbe(HTTPHealthProbeConfig{OriginURL: server.URL, ExpectedCNAME: "dualstack.nonssl.global.fastly.net"})
	if err != nil {
		t.Fatal(err)
	}
	preflight, err := probe.Preflight(context.Background(), fastlyRequest(), "")
	if err != nil || !preflight.OriginHealthy {
		t.Fatalf("origin preflight = %#v, err=%v", preflight, err)
	}
	ok, headers, err := probe.checkRoute(context.Background(), server.URL)
	if err != nil || !ok || headers.Get("Via") == "" {
		t.Fatalf("route probe = ok:%v headers:%#v err:%v", ok, headers, err)
	}
}

func TestHTTPHealthProbeUsesConfiguredOriginHost(t *testing.T) {
	var receivedHost string
	server := httptest.NewServer(httpHandler(func(w http.ResponseWriter, r *http.Request) {
		receivedHost = r.Host
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	probe, err := NewHTTPHealthProbe(HTTPHealthProbeConfig{OriginURL: server.URL, OriginHost: "origin.example.com", ExpectedCNAME: "dualstack.nonssl.global.fastly.net"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := probe.Preflight(context.Background(), fastlyRequest(), ""); err != nil {
		t.Fatal(err)
	}
	if receivedHost != "origin.example.com" {
		t.Fatalf("origin Host = %q, want origin.example.com", receivedHost)
	}
}

type delayedFastlyRouteTransport struct {
	calls int
}

func (transport *delayedFastlyRouteTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	transport.calls++
	status := http.StatusInternalServerError
	if transport.calls >= 3 {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Via": []string{"1.1 varnish"}},
		Body:       io.NopCloser(strings.NewReader("ok")),
		Request:    request,
	}, nil
}

func TestHTTPHealthProbeRetriesTransientFastlyRoutePropagation(t *testing.T) {
	transport := &delayedFastlyRouteTransport{}
	probe, err := NewHTTPHealthProbe(HTTPHealthProbeConfig{
		OriginURL: "https://origin.example.com/", ExpectedCNAME: "dualstack.nonssl.global.fastly.net",
		RouteTimeout: 100 * time.Millisecond, RoutePoll: time.Millisecond,
		Client: &http.Client{Transport: transport},
	})
	if err != nil {
		t.Fatal(err)
	}
	ok, _, err := probe.checkRouteUntilReady(context.Background(), "https://edge.example.com/")
	if err != nil || !ok || transport.calls != 3 {
		t.Fatalf("route convergence = ok:%v calls:%d err:%v", ok, transport.calls, err)
	}
}

type httpHandler func(http.ResponseWriter, *http.Request)

func (handler httpHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	handler(writer, request)
}

func TestApplyAndDestroyOnlyDeletesMarkedOwnedResources(t *testing.T) {
	runner := &fakeRunner{outputs: [][]byte{
		[]byte("Created service yrrMKM93YoJjtikP1OGfXO\n"),
		[]byte(`{"data":[]}`),
		[]byte("domain created"),
		[]byte("purged"),
		[]byte(`{"data":[{"id":"gvPYujf2oAjk98PAgli9Vw","fqdn":"preview.example.com","description":"magelift/acceptance/test-1"},{"id":"other-domain","fqdn":"existing.example.com","description":"user-owned"}]}`),
		[]byte("deleted"),
		[]byte(`{"comment":"magelift/acceptance/test-1"}`),
		[]byte("deleted service"),
	}}
	probe := healthyFastlyProbe()
	deletion := completeFastlyDeletionProbe()
	adapter := NewWithProbes(runner, probe, deletion)
	request := fastlyRequest()
	result, err := adapter.Apply(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !result.CreatedService || result.ServiceID == "" || len(result.CreatedDomains) != 1 {
		t.Fatalf("result = %#v", result)
	}
	if strings.Join(probe.calls, ",") != "preflight,verify" {
		t.Fatalf("health probe calls = %#v", probe.calls)
	}
	if err := adapter.Destroy(context.Background(), request, result); err != nil {
		t.Fatal(err)
	}
	if deletion.calls != 1 {
		t.Fatalf("deletion inventory calls = %d, want 1", deletion.calls)
	}
	joined := ""
	for _, args := range runner.args {
		joined += strings.Join(args, " ") + "\n"
	}
	if strings.Contains(joined, "existing.example.com") {
		t.Fatalf("cleanup touched an unmarked domain: %s", joined)
	}
	if !strings.Contains(joined, "service delete --force") {
		t.Fatalf("owned service was not deleted: %s", joined)
	}
}

func TestApplyCleansPartialResourcesAfterVerificationFailure(t *testing.T) {
	runner := &fakeRunner{outputs: [][]byte{
		[]byte("Created service yrrMKM93YoJjtikP1OGfXO\n"),
		[]byte(`{"data":[]}`),
		[]byte("domain created"),
		[]byte("purged"),
		[]byte(`{"data":[{"id":"gvPYujf2oAjk98PAgli9Vw","fqdn":"preview.example.com","description":"magelift/acceptance/test-1"}]}`),
		[]byte("deleted domain"),
		[]byte(`{"comment":"magelift/acceptance/test-1"}`),
		[]byte("deleted service"),
	}}
	probe := healthyFastlyProbe()
	probe.verify.RouteHealthy = false
	adapter := NewWithProbes(runner, probe, completeFastlyDeletionProbe())

	_, err := adapter.Apply(context.Background(), fastlyRequest())
	if err == nil || !strings.Contains(err.Error(), "route verification") {
		t.Fatalf("partial apply error = %v", err)
	}
	commands := runner.args
	if !hasFastlyCommand(commands, "service", "delete") {
		t.Fatalf("partial apply did not delete the created service: %#v", commands)
	}
	if !hasFastlyCommand(commands, "domain", "delete") {
		t.Fatalf("partial apply did not delete the created domain: %#v", commands)
	}
}

func hasFastlyCommand(commands [][]string, values ...string) bool {
	for _, command := range commands {
		if len(command) < len(values) {
			continue
		}
		for i := range command {
			if i+len(values) > len(command) {
				break
			}
			matched := true
			for j, value := range values {
				if command[i+j] != value {
					matched = false
					break
				}
			}
			if matched {
				return true
			}
		}
	}
	return false
}

func TestDestroyPreservesExistingService(t *testing.T) {
	runner := &fakeRunner{outputs: [][]byte{
		[]byte(`{"id":"yrrMKM93YoJjtikP1OGfXO","comment":"magelift/acceptance/test-1"}`),
		[]byte(`{"data":[{"id":"gvPYujf2oAjk98PAgli9Vw","fqdn":"preview.example.com","description":"magelift/acceptance/test-1"}]}`),
	}}
	request := fastlyRequest()
	request.ServiceID = "yrrMKM93YoJjtikP1OGfXO"
	result := Result{ServiceID: request.ServiceID, CreatedDomains: []string{"preview.example.com"}, OwnershipMarker: request.OwnershipMarker}
	if err := NewWithDeletionProbe(runner, completeFastlyDeletionProbe()).Destroy(context.Background(), request, result); err != nil {
		t.Fatal(err)
	}
	if len(runner.args) != 3 || strings.Contains(strings.Join(runner.args[2], " "), "service delete") {
		t.Fatalf("existing service cleanup = %#v", runner.args)
	}
}

func TestDestroyRejectsExistingServiceOwnershipDriftBeforeMutation(t *testing.T) {
	runner := &fakeRunner{outputs: [][]byte{[]byte(`{"id":"existing-service","comment":"user-owned"}`)}}
	request := fastlyRequest()
	request.ServiceID = "existing-service"
	request.CreateService = false
	result := Result{ServiceID: request.ServiceID, OwnershipMarker: request.OwnershipMarker, CreatedDomains: []string{"preview.example.com"}}
	if err := NewWithDeletionProbe(runner, completeFastlyDeletionProbe()).Destroy(context.Background(), request, result); err == nil || !strings.Contains(err.Error(), "ownership") {
		t.Fatalf("ownership drift cleanup error = %v", err)
	}
	if len(runner.args) != 1 {
		t.Fatalf("ownership drift cleanup mutated resources: %#v", runner.args)
	}
}

func TestDestroyRemovesManagedTLSBeforeVersionlessDomain(t *testing.T) {
	runner := &fakeRunner{outputs: [][]byte{
		[]byte(`{"id":"existing-service","comment":"magelift/acceptance/test-1"}`),
		[]byte(`{"data":[{"id":"domain-1","fqdn":"preview.example.com","description":"magelift/acceptance/test-1"}]}`),
	}}
	request := fastlyRequest()
	request.ServiceID = "existing-service"
	result := Result{
		ServiceID: "existing-service", CreatedDomains: []string{"preview.example.com"}, OwnershipMarker: request.OwnershipMarker,
		CreatedTLS: true, TLSSubscriptionID: "tls-subscription-123",
	}
	if err := NewWithDeletionProbe(runner, completeFastlyDeletionProbe()).Destroy(context.Background(), request, result); err != nil {
		t.Fatal(err)
	}
	if len(runner.args) < 3 {
		t.Fatalf("cleanup commands = %#v", runner.args)
	}
	if !hasFastlyCommand(runner.args[2:3], "tls-subscription", "delete") {
		t.Fatalf("TLS subscription was not deleted: %#v", runner.args)
	}
	if !hasFastlyCommand(runner.args[3:4], "domain", "delete") {
		t.Fatalf("versionless domain was not deleted: %#v", runner.args)
	}
	if strings.Contains(strings.Join(runner.args[2], " "), "domain delete") {
		t.Fatalf("domain was deleted before TLS subscription: %#v", runner.args)
	}
}

func TestCLIInventoryDeletionProbeChecksVersionlessDomainsForCreatedService(t *testing.T) {
	runner := &fakeRunner{outputs: [][]byte{
		[]byte(`[]`),
		[]byte(`{"data":[{"id":"domain-1","fqdn":"preview.example.com","description":"magelift/acceptance/test-1"}]}`),
	}}
	request := fastlyRequest()
	result := Result{ServiceID: "created-service", CreatedService: true, CreatedDomains: []string{"preview.example.com"}, OwnershipMarker: request.OwnershipMarker}
	observation, err := NewCLIInventoryDeletionProbe(runner).PollDeletion(context.Background(), request, result)
	if err != nil {
		t.Fatal(err)
	}
	if observation.Complete || !strings.Contains(strings.Join(observation.ResourceRefs, ","), "domain:preview.example.com") {
		t.Fatalf("deletion observation = %#v", observation)
	}
}

type liveCloudflareDNSRecord struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Content string `json:"content"`
	Comment string `json:"comment"`
}

// liveTLSChallengeRunner is acceptance-only glue. The Fastly adapter exposes
// the managed-TLS DNS challenge; this test-owned runner applies that record
// through the separately owned Cloudflare DNS boundary before the adapter
// verifies the route. Production edge code never shells out to Cloudflare.
type liveTLSChallengeRunner struct {
	base    Runner
	profile string
	zone    string
	domain  string
	marker  string
	owned   []string
}

func (runner *liveTLSChallengeRunner) Run(ctx context.Context, args []string) ([]byte, error) {
	if runner.base == nil {
		return nil, fmt.Errorf("live Fastly runner base is required")
	}
	data, err := runner.base.Run(ctx, args)
	if err != nil {
		return nil, err
	}
	if !isFastlyTLSDescribe(args) {
		return data, nil
	}
	challenges, err := parseTLSChallenges(data)
	if err != nil {
		return nil, err
	}
	expectedCNAME := strings.TrimSuffix(os.Getenv("MAGELIFT_FASTLY_LIVE_CNAME_TARGET"), ".")
	for _, challenge := range challenges {
		if challenge.RecordType == "CNAME" && challenge.Type == "managed-http-cname" {
			if len(challenge.Values) != 1 || !fastlyCNAMEMatches(expectedCNAME, challenge.Values[0]) {
				return nil, fmt.Errorf("Fastly managed TLS route CNAME %q does not match the configured exact target %q", firstTLSChallengeValue(challenge), expectedCNAME)
			}
		}
		if challenge.RecordType != "CNAME" || challenge.Type != "managed-dns" {
			continue
		}
		if !strings.EqualFold(challenge.RecordName, "_acme-challenge."+runner.domain) || len(challenge.Values) != 1 {
			return nil, fmt.Errorf("unexpected Fastly managed TLS DNS challenge for %q", runner.domain)
		}
		if err := runner.ensureRecord(ctx, challenge.RecordName, challenge.Values[0]); err != nil {
			return nil, err
		}
	}
	return data, nil
}

func (runner *liveTLSChallengeRunner) Cleanup(ctx context.Context) error {
	var cleanupErr error
	for _, id := range runner.owned {
		if _, err := runner.cloudflare(ctx, "dns", "records", "delete", id, "--force", "--quiet"); err != nil {
			cleanupErr = errors.Join(cleanupErr, err)
		}
	}
	return cleanupErr
}

func (runner *liveTLSChallengeRunner) ensureRecord(ctx context.Context, name, target string) error {
	data, err := runner.cloudflare(ctx, "dns", "records", "list", "--name-exact", name, "--type", "CNAME", "--per-page", "100", "--quiet")
	if err != nil {
		return err
	}
	records, err := parseLiveCloudflareRecords(data)
	if err != nil {
		return err
	}
	if len(records) > 1 {
		return fmt.Errorf("multiple Cloudflare CNAME records exist at %q", name)
	}
	if len(records) == 1 {
		record := records[0]
		if !strings.EqualFold(record.Comment, runner.marker) || !strings.EqualFold(strings.TrimSuffix(record.Content, "."), strings.TrimSuffix(target, ".")) {
			return fmt.Errorf("refusing foreign or drifted Cloudflare TLS challenge record at %q", name)
		}
		runner.owned = append(runner.owned, record.ID)
		return nil
	}
	body, err := json.Marshal(map[string]any{
		"type": "CNAME", "name": name, "content": strings.TrimSuffix(target, "."),
		"ttl": 60, "proxied": false, "comment": runner.marker,
	})
	if err != nil {
		return fmt.Errorf("marshal Cloudflare TLS challenge record: %w", err)
	}
	created, err := runner.cloudflare(ctx, "dns", "records", "create", "--body", string(body), "--quiet")
	if err != nil {
		return err
	}
	createdID, err := parseLiveCloudflareCreateID(created)
	if err != nil {
		return err
	}
	if createdID == "" {
		return fmt.Errorf("Cloudflare TLS challenge create returned no record ID")
	}
	runner.owned = append(runner.owned, createdID)
	return nil
}

func (runner *liveTLSChallengeRunner) cloudflare(ctx context.Context, args ...string) ([]byte, error) {
	commandArgs := append([]string{"--profile", runner.profile, "--zone", runner.zone}, args...)
	data, err := exec.CommandContext(ctx, "cf", commandArgs...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("Cloudflare %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(data)))
	}
	return data, nil
}

func parseLiveCloudflareRecords(data []byte) ([]liveCloudflareDNSRecord, error) {
	if strings.HasPrefix(strings.TrimSpace(string(data)), "[") {
		var records []liveCloudflareDNSRecord
		if err := json.Unmarshal(data, &records); err != nil {
			return nil, fmt.Errorf("decode Cloudflare DNS record list: %w", err)
		}
		return records, nil
	}
	var envelope struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("decode Cloudflare DNS records: %w", err)
	}
	if len(envelope.Result) == 0 || string(envelope.Result) == "null" {
		return nil, nil
	}
	var records []liveCloudflareDNSRecord
	if err := json.Unmarshal(envelope.Result, &records); err != nil {
		return nil, fmt.Errorf("decode Cloudflare DNS record list: %w", err)
	}
	return records, nil
}

func parseLiveCloudflareCreateID(data []byte) (string, error) {
	if strings.HasPrefix(strings.TrimSpace(string(data)), "[") {
		var records []liveCloudflareDNSRecord
		if err := json.Unmarshal(data, &records); err != nil {
			return "", fmt.Errorf("decode Cloudflare DNS create response: %w", err)
		}
		if len(records) > 0 {
			return records[0].ID, nil
		}
		return "", nil
	}
	var envelope struct {
		ID     string          `json:"id"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return "", fmt.Errorf("decode Cloudflare DNS create response: %w", err)
	}
	if envelope.ID != "" {
		return envelope.ID, nil
	}
	if len(envelope.Result) == 0 || string(envelope.Result) == "null" {
		var records []liveCloudflareDNSRecord
		if json.Unmarshal(data, &records) == nil && len(records) > 0 {
			return records[0].ID, nil
		}
		return "", nil
	}
	var object struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(envelope.Result, &object) == nil && object.ID != "" {
		return object.ID, nil
	}
	var records []liveCloudflareDNSRecord
	if json.Unmarshal(envelope.Result, &records) == nil && len(records) > 0 {
		return records[0].ID, nil
	}
	return "", nil
}

func isFastlyTLSDescribe(args []string) bool {
	return hasFastlyCommand([][]string{args}, "tls-subscription", "describe")
}

func firstTLSChallengeValue(challenge TLSChallenge) string {
	if len(challenge.Values) == 0 {
		return ""
	}
	return challenge.Values[0]
}

func fastlyCNAMEMatches(configured, assigned string) bool {
	configured = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(configured)), ".")
	assigned = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(assigned)), ".")
	return configured == assigned || configured == "dualstack."+assigned
}

func TestApplyReusesMarkedDomain(t *testing.T) {
	runner := &fakeRunner{outputs: [][]byte{
		[]byte(`{"id":"yrrMKM93YoJjtikP1OGfXO","comment":"magelift/acceptance/test-1"}`),
		[]byte(`{"data":[{"id":"gvPYujf2oAjk98PAgli9Vw","fqdn":"preview.example.com","description":"magelift/acceptance/test-1"}]}`),
		[]byte("purged"),
	}}
	request := fastlyRequest()
	request.ServiceID = "yrrMKM93YoJjtikP1OGfXO"
	result, err := NewWithHealthProbe(runner, healthyFastlyProbe()).Apply(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.CreatedDomains) != 1 || result.CreatedDomains[0] != "preview.example.com" {
		t.Fatalf("result = %#v", result)
	}
	joined := ""
	for _, args := range runner.args {
		joined += strings.Join(args, " ") + "\n"
	}
	if strings.Contains(joined, "domain create") {
		t.Fatalf("idempotent apply created an existing domain: %s", joined)
	}
}

func TestApplyMapsProviderOwnedVCLTLSAndPurgeOutputs(t *testing.T) {
	runner := &fakeRunner{outputs: [][]byte{
		[]byte("Created service yrrMKM93YoJjtikP1OGfXO\n"),
		[]byte(`{"data":[]}`),
		[]byte("domain created"),
		[]byte("vcl created"),
		[]byte(`{"id":"tls-subscription-123"}`),
		[]byte(`{"Authorizations":[{"Challenges":[{"RecordName":"_acme-challenge.preview.example.com","RecordType":"CNAME","Type":"managed-dns","Values":["token.fastly-validations.com"]}]}]}`),
		[]byte("purged"),
		[]byte(`{"data":[{"id":"gvPYujf2oAjk98PAgli9Vw","fqdn":"preview.example.com","description":"magelift/acceptance/test-1"}]}`),
		[]byte("domain deleted"),
		[]byte("vcl deleted"),
		[]byte("tls deleted"),
		[]byte(`{"comment":"magelift/acceptance/test-1"}`),
		[]byte("service deleted"),
	}}
	request := fastlyRequest()
	request.ProviderConfig = Config{
		Version: "latest", TLS: true, TLSMode: "fastly-managed", DNSMode: "external",
		VCLName: "magelift-main", VCLContent: "sub vcl_recv { return (hash); }", OriginHealthRef: "health/magento",
	}
	adapter := NewWithProbes(runner, healthyFastlyProbe(), completeFastlyDeletionProbe())
	result, err := adapter.Apply(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !result.CreatedTLS || result.TLSSubscriptionID != "tls-subscription-123" || len(result.TLSChallenges) != 1 || result.TLSChallenges[0].RecordName != "_acme-challenge.preview.example.com" || !result.VCLApplied || !result.PurgeRequested || result.Version != "latest" {
		t.Fatalf("provider result = %#v", result)
	}
	if !hasFastlyOutput(result.Outputs, "edge.service_id", result.ServiceID) || !hasFastlyOutput(result.Outputs, "edge.tls_subscription_id", result.TLSSubscriptionID) || !hasFastlyOutput(result.Outputs, "edge.tls.challenges", "[{\"recordName\":\"_acme-challenge.preview.example.com\",\"recordType\":\"CNAME\",\"type\":\"managed-dns\",\"values\":[\"token.fastly-validations.com\"]}]") || !hasFastlyOutput(result.Outputs, "edge.vcl_name", "magelift-main") || !hasFastlyOutput(result.Outputs, "edge.purge_requested", "true") {
		t.Fatalf("stable outputs = %#v", result.Outputs)
	}
	if err := adapter.Destroy(context.Background(), request, result); err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, args := range runner.args {
		joined += strings.Join(args, " ") + "\n"
	}
	for _, required := range []string{"service vcl custom create", "tls-subscription create", "service purge", "service vcl custom delete", "tls-subscription delete"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("Fastly command %q missing from %s", required, joined)
		}
	}
}

func TestParseTLSSubscriptionIDIgnoresCLIStatusWords(t *testing.T) {
	got, err := parseTLSSubscriptionID([]byte("SUCCESS: Created TLS Subscription 'zY9sqmd4fGQRuqAlmFhlUQ'\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got != "zY9sqmd4fGQRuqAlmFhlUQ" {
		t.Fatalf("TLS subscription ID = %q", got)
	}
}

func TestPlanRejectsUnscopedTLSAndVCLConfiguration(t *testing.T) {
	request := fastlyRequest()
	request.ProviderConfig = Config{TLS: true, OriginHealthRef: "health/magento"}
	if _, err := New(&fakeRunner{}).Plan(request); err == nil || !strings.Contains(err.Error(), "TLS mode") {
		t.Fatalf("TLS without ownership mode was accepted: %v", err)
	}
	request = fastlyRequest()
	request.ProviderConfig = Config{VCLContent: "sub vcl_recv {}", OriginHealthRef: "health/magento"}
	if _, err := New(&fakeRunner{}).Plan(request); err == nil || !strings.Contains(err.Error(), "VCL name") {
		t.Fatalf("unnamed VCL was accepted: %v", err)
	}
}

func TestPlanNormalizesPortableExternalTLSMode(t *testing.T) {
	request := fastlyRequest()
	request.ProviderConfig = Config{TLS: true, TLSMode: "external", OriginHealthRef: "health/magento"}

	plan, err := New(&fakeRunner{}).Plan(request)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ProviderConfig.TLSMode != "customer-managed" {
		t.Fatalf("TLS mode = %q, want customer-managed", plan.ProviderConfig.TLSMode)
	}
}

func hasFastlyOutput(outputs []Output, key, want string) bool {
	for _, output := range outputs {
		if output.Key == key && output.Value == want {
			return true
		}
	}
	return false
}

func TestLiveFastlyAdapter(t *testing.T) {
	if os.Getenv("MAGELIFT_FASTLY_LIVE") != "1" {
		t.Skip("set MAGELIFT_FASTLY_LIVE=1 to run the authenticated Fastly lifecycle probe")
	}
	marker := os.Getenv("MAGELIFT_FASTLY_LIVE_MARKER")
	if marker == "" {
		marker = "magelift/acceptance/live-" + time.Now().UTC().Format("20060102-150405")
	}
	domain := os.Getenv("MAGELIFT_FASTLY_LIVE_DOMAIN")
	if domain == "" {
		domain = "m249p-fastly-live-20260807.acourtiol.com"
	}
	cnameTarget := os.Getenv("MAGELIFT_FASTLY_LIVE_CNAME_TARGET")
	originAddress := os.Getenv("MAGELIFT_FASTLY_LIVE_ORIGIN_ADDRESS")
	originHost := os.Getenv("MAGELIFT_FASTLY_LIVE_ORIGIN_HOST")
	originURL := os.Getenv("MAGELIFT_FASTLY_LIVE_ORIGIN_URL")
	if cnameTarget == "" || originAddress == "" || originURL == "" {
		t.Fatal("live Fastly route requires CNAME target, origin address, and origin URL")
	}
	if originHost == "" {
		originHost = originAddress
	}
	originUseTLS := os.Getenv("MAGELIFT_FASTLY_LIVE_ORIGIN_USE_TLS") != "0"
	tlsEnabled := os.Getenv("MAGELIFT_FASTLY_LIVE_TLS") == "1"
	if !tlsEnabled {
		t.Fatalf("the current Fastly versionless-domain acceptance path requires managed TLS; use the classic adapter only on an account that explicitly supports it")
	}
	expectedStatus := http.StatusOK
	if value := os.Getenv("MAGELIFT_FASTLY_LIVE_EXPECTED_STATUS"); value != "" {
		parsedStatus, parseErr := strconv.Atoi(value)
		if parseErr != nil {
			t.Fatalf("invalid MAGELIFT_FASTLY_LIVE_EXPECTED_STATUS: %v", parseErr)
		}
		expectedStatus = parsedStatus
	}
	routePath := os.Getenv("MAGELIFT_FASTLY_LIVE_ROUTE_PATH")
	if routePath == "" {
		routePath = "/"
	}
	routeTimeout := 5 * time.Minute
	if value := os.Getenv("MAGELIFT_FASTLY_LIVE_ROUTE_TIMEOUT_SECONDS"); value != "" {
		seconds, parseErr := strconv.Atoi(value)
		if parseErr != nil || seconds <= 0 {
			t.Fatalf("invalid MAGELIFT_FASTLY_LIVE_ROUTE_TIMEOUT_SECONDS: %q", value)
		}
		routeTimeout = time.Duration(seconds) * time.Second
	}
	probe, err := NewHTTPHealthProbe(HTTPHealthProbeConfig{OriginURL: originURL, ExpectedCNAME: cnameTarget, RoutePath: routePath, ExpectedStatus: expectedStatus, RouteTimeout: routeTimeout, RoutePoll: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	request := Request{
		ServiceName:     "MageLift live Fastly adapter probe",
		Domains:         []string{domain},
		OwnershipMarker: marker,
		PurgeOnDeploy:   true,
		CreateService:   true,
		ProviderConfig: Config{
			OriginHealthRef: "health/live", ActivateVersion: true, OriginAddress: originAddress, OriginHost: originHost,
			OriginUseTLS: originUseTLS, TLS: tlsEnabled, BackendName: "magelift_origin",
		},
	}
	if tlsEnabled {
		request.ProviderConfig.TLSMode = "fastly-managed"
		request.ProviderConfig.DNSMode = "external"
	}
	runner := Runner(ExecRunner{})
	var challengeRunner *liveTLSChallengeRunner
	if tlsEnabled {
		challengeRunner = &liveTLSChallengeRunner{
			base: ExecRunner{}, profile: os.Getenv("MAGELIFT_FASTLY_LIVE_CLOUDFLARE_PROFILE"),
			zone: os.Getenv("MAGELIFT_FASTLY_LIVE_CLOUDFLARE_ZONE"), domain: domain,
			marker: os.Getenv("MAGELIFT_FASTLY_LIVE_TLS_DNS_MARKER"),
		}
		if challengeRunner.profile == "" {
			challengeRunner.profile = "default"
		}
		if challengeRunner.zone == "" {
			challengeRunner.zone = "acourtiol.com"
		}
		if challengeRunner.marker == "" {
			t.Fatalf("MAGELIFT_FASTLY_LIVE_TLS_DNS_MARKER is required for managed TLS acceptance")
		}
		runner = challengeRunner
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			if err := challengeRunner.Cleanup(ctx); err != nil {
				t.Errorf("destroy live Fastly TLS challenge DNS: %v", err)
			}
		})
	}
	adapter := NewWithProbes(runner, probe, NewCLIInventoryDeletionProbe(runner))
	result, err := adapter.Apply(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := adapter.Destroy(ctx, request, result); err != nil {
			t.Errorf("destroy live Fastly probe: %v", err)
		}
	})
	if result.ServiceID == "" || len(result.CreatedDomains) != 1 {
		t.Fatalf("live result = %#v", result)
	}
	if tlsEnabled && (!result.CreatedTLS || len(result.TLSChallenges) == 0) {
		t.Fatalf("managed TLS result did not expose its DNS challenge: %#v", result)
	}
}

func TestDestroyRefusesToClaimCleanupWithoutOwningInventoryProbe(t *testing.T) {
	request := fastlyRequest()
	request.ServiceID = "yrrMKM93YoJjtikP1OGfXO"
	result := Result{ServiceID: request.ServiceID, CreatedDomains: []string{"preview.example.com"}, OwnershipMarker: request.OwnershipMarker}
	if err := New(&fakeRunner{}).Destroy(context.Background(), request, result); err == nil || !strings.Contains(err.Error(), "deletion inventory probe") {
		t.Fatalf("cleanup without deletion probe error = %v", err)
	}
}

func TestWaitForDeletionRetriesTransientInventory(t *testing.T) {
	probe := &fakeDeletionProbe{completeAfter: 3}
	observation, err := WaitForDeletion(context.Background(), probe, fastlyRequest(), Result{ServiceID: "service", OwnershipMarker: "magelift/acceptance/test-1"}, DeletionPolicy{Timeout: time.Second, PollInterval: time.Millisecond, MaxAttempts: 3})
	if err != nil {
		t.Fatal(err)
	}
	if !observation.Complete || probe.calls != 3 {
		t.Fatalf("deletion observation = %#v after %d calls", observation, probe.calls)
	}
}

func TestSDKAdapterUsesSharedEdgeLifecycleAndOpaqueResourceReferences(t *testing.T) {
	runner := &fakeRunner{outputs: [][]byte{
		[]byte("Created service yrrMKM93YoJjtikP1OGfXO\n"),
		[]byte(`{"data":[]}`),
		[]byte("domain created"),
		[]byte("purged"),
	}}
	adapter := NewSDKAdapter(NewWithProbes(runner, healthyFastlyProbe(), completeFastlyDeletionProbe()))
	intent := sdk.EdgeIntent{
		ExternalProvider: "fastly", Lifecycle: sdk.ExternalLifecycleExtension, Certification: sdk.ExternalExperimental,
		CredentialRefs: []string{"aws-secrets-manager://magelift/fastly-token"}, Mode: "external", Domains: []string{"preview.example.com"},
		PurgeOnDeploy: true, OriginHealthRef: "health/magento", OwnershipMarker: "magelift/acceptance/sdk-fastly",
	}
	request := sdk.EdgePlanRequest{TargetProvider: "aws", TargetRuntime: "ecs-fargate", Intent: intent}
	_, applied, err := sdk.RunEdgeLifecycle(context.Background(), adapter, request, sdk.EdgeApply, "apply-1", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(applied.ResourceRefs) == 0 || !applied.OwnershipVerified || !applied.IdempotencyVerified {
		t.Fatalf("apply result = %#v", applied)
	}
	_, verified, err := sdk.RunEdgeLifecycle(context.Background(), adapter, request, sdk.EdgeVerify, "verify-1", applied.ResourceRefs, "")
	if err != nil {
		t.Fatal(err)
	}
	if verified.Action != sdk.EdgeVerify || len(verified.ProofRefs) == 0 {
		t.Fatalf("verify result = %#v", verified)
	}
}

type fakeSDKLifecycle struct {
	plan         Plan
	applyCalls   int
	verifyCalls  int
	destroyCalls int
}

type fakeSDKFailoverLifecycle struct {
	fakeSDKLifecycle
	failoverCalls int
	rollbackCalls int
}

func (lifecycle *fakeSDKFailoverLifecycle) SupportsFailover() bool { return true }

func (lifecycle *fakeSDKFailoverLifecycle) Plan(request Request) (Plan, error) {
	plan, err := planRequest(request, true)
	if err != nil {
		return Plan{}, err
	}
	lifecycle.plan = plan
	return plan, nil
}

func (lifecycle *fakeSDKFailoverLifecycle) Failover(_ context.Context, _ Request, result Result) (Result, error) {
	lifecycle.failoverCalls++
	result.FailoverApplied = true
	return result, nil
}

func (lifecycle *fakeSDKFailoverLifecycle) Rollback(_ context.Context, _ Request, result Result) (Result, error) {
	lifecycle.rollbackCalls++
	result.RollbackApplied = true
	return result, nil
}

func (lifecycle *fakeSDKLifecycle) Plan(request Request) (Plan, error) {
	plan, err := (Adapter{}).Plan(request)
	if err != nil {
		return Plan{}, err
	}
	lifecycle.plan = plan
	return plan, nil
}

func (lifecycle *fakeSDKLifecycle) Apply(_ context.Context, request Request) (Result, error) {
	lifecycle.applyCalls++
	return Result{ServiceID: "service-native", CreatedService: true, OwnershipMarker: request.OwnershipMarker, Version: "1"}, nil
}

func (lifecycle *fakeSDKLifecycle) Verify(_ context.Context, _ Request, result Result) error {
	lifecycle.verifyCalls++
	if result.ServiceID != "service-native" {
		return fmt.Errorf("unexpected service %q", result.ServiceID)
	}
	return nil
}

func (lifecycle *fakeSDKLifecycle) Destroy(_ context.Context, _ Request, result Result) error {
	lifecycle.destroyCalls++
	if result.ServiceID != "service-native" {
		return fmt.Errorf("unexpected service %q", result.ServiceID)
	}
	return nil
}

func TestSDKLifecycleAdapterBridgesProviderLifecycleWithoutCLI(t *testing.T) {
	lifecycle := &fakeSDKLifecycle{}
	adapter := NewSDKLifecycleAdapter(lifecycle)
	request := sdk.EdgePlanRequest{
		TargetProvider: "aws",
		TargetRuntime:  "ecs-fargate",
		Intent: sdk.EdgeIntent{
			ExternalProvider: "fastly", Lifecycle: sdk.ExternalLifecycleExtension, Certification: sdk.ExternalExperimental,
			CredentialRefs: []string{"aws-secrets-manager://magelift/fastly-token"}, Mode: "external",
			Domains: []string{"preview.example.com"}, OriginHealthRef: "health/magento",
			OwnershipMarker: "magelift/acceptance/sdk-fastly-native",
		},
	}
	plan, err := adapter.PlanEdge(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := plan.Opaque.(Plan); !ok {
		t.Fatalf("opaque plan type = %T", plan.Opaque)
	}

	applied, err := adapter.ExecuteEdge(context.Background(), sdk.EdgeExecutionRequest{Action: sdk.EdgeApply, Plan: plan, OwnershipMarker: request.Intent.OwnershipMarker})
	if err != nil {
		t.Fatal(err)
	}
	if lifecycle.applyCalls != 1 || len(applied.ResourceRefs) != 2 {
		t.Fatalf("apply calls/result = %d/%#v", lifecycle.applyCalls, applied)
	}

	verified, err := adapter.ExecuteEdge(context.Background(), sdk.EdgeExecutionRequest{
		Action: sdk.EdgeVerify, Plan: plan, ResourceReferences: applied.ResourceRefs,
		OwnershipMarker: request.Intent.OwnershipMarker,
	})
	if err != nil {
		t.Fatal(err)
	}
	if lifecycle.verifyCalls != 1 || verified.Action != sdk.EdgeVerify {
		t.Fatalf("verify calls/result = %d/%#v", lifecycle.verifyCalls, verified)
	}

	_, err = adapter.ExecuteEdge(context.Background(), sdk.EdgeExecutionRequest{
		Action: sdk.EdgeDestroy, Plan: plan, ResourceReferences: applied.ResourceRefs,
		OwnershipMarker: request.Intent.OwnershipMarker,
	})
	if err != nil {
		t.Fatal(err)
	}
	if lifecycle.destroyCalls != 1 {
		t.Fatalf("destroy calls = %d", lifecycle.destroyCalls)
	}
}

func TestSDKLifecycleAdapterAdvertisesAndExecutesOptionalFailoverLifecycle(t *testing.T) {
	lifecycle := &fakeSDKFailoverLifecycle{}
	adapter := NewSDKLifecycleAdapter(lifecycle)
	if !containsFastlyEdgeAction(adapter.EdgeDescriptor().Capabilities, sdk.EdgeFailover) || !containsFastlyEdgeAction(adapter.EdgeDescriptor().Capabilities, sdk.EdgeRollback) {
		t.Fatalf("descriptor = %#v", adapter.EdgeDescriptor())
	}
	request := sdk.EdgePlanRequest{TargetProvider: "aws", TargetRuntime: "ecs-fargate", Intent: sdk.EdgeIntent{
		ExternalProvider: "fastly", Lifecycle: sdk.ExternalLifecycleExtension, Certification: sdk.ExternalExperimental,
		CredentialRefs: []string{"aws-secrets-manager://magelift/fastly-token"}, Mode: "external", Domains: []string{"preview.example.com"},
		OriginHealthRef: "health/magento", FailoverPolicyRef: "fastly-policy://magelift/primary-secondary", OwnershipMarker: "magelift/acceptance/sdk-fastly-failover",
	}}
	plan, err := adapter.PlanEdge(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	apply, err := adapter.ExecuteEdge(context.Background(), sdk.EdgeExecutionRequest{Action: sdk.EdgeApply, Plan: plan, OwnershipMarker: plan.OwnershipMarker})
	if err != nil {
		t.Fatal(err)
	}
	failedOver, err := adapter.ExecuteEdge(context.Background(), sdk.EdgeExecutionRequest{Action: sdk.EdgeFailover, Plan: plan, ResourceReferences: apply.ResourceRefs, OwnershipMarker: plan.OwnershipMarker})
	if err != nil {
		t.Fatal(err)
	}
	if lifecycle.failoverCalls != 1 || !containsString(failedOver.ResourceRefs, "failover:applied") {
		t.Fatalf("failover calls/result = %d/%#v", lifecycle.failoverCalls, failedOver)
	}
	rolledBack, err := adapter.ExecuteEdge(context.Background(), sdk.EdgeExecutionRequest{Action: sdk.EdgeRollback, Plan: plan, ResourceReferences: failedOver.ResourceRefs, OwnershipMarker: plan.OwnershipMarker})
	if err != nil {
		t.Fatal(err)
	}
	if lifecycle.rollbackCalls != 1 || !containsString(rolledBack.ResourceRefs, "rollback:applied") {
		t.Fatalf("rollback calls/result = %d/%#v", lifecycle.rollbackCalls, rolledBack)
	}
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func containsFastlyEdgeAction(values []sdk.EdgeAction, wanted sdk.EdgeAction) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
