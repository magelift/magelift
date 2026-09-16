package providerhost

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/magelift/magelift/sdk"
)

type stubCaller struct {
	calls   []string
	args    []any
	respond func(method string, reply any) error
	block   chan struct{}
}

func (s *stubCaller) Call(method string, args any, reply any) error {
	s.calls = append(s.calls, method)
	s.args = append(s.args, args)
	if s.block != nil {
		<-s.block
	}
	if s.respond != nil {
		return s.respond(method, reply)
	}
	return errors.New("no response scripted for " + method)
}

func cannedDescribe() *sdk.DescribeResponse {
	operations := make([]sdk.OperationVersion, 0, len(sdk.PluginMethods))
	for operation := range sdk.PluginMethods {
		operations = append(operations, sdk.OperationVersion{Name: string(operation), Version: "1.0"})
	}
	return &sdk.DescribeResponse{
		ProtocolVersion: sdk.ProtocolV1, ProviderID: "gcp", ProviderVersion: "v9.9.9",
		Operations: operations,
		Runtimes: []sdk.RuntimeAdvertisement{
			{Runtime: "gke-autopilot", Tier: sdk.ExtensionTierCertified},
			{Runtime: "gke-standard", Tier: sdk.ExtensionTierExperimental},
		},
		OutputKeys: []string{"applicationURL", "kubeconfig"},
		Edge:       &sdk.EdgeAdapterDescriptor{APIVersion: sdk.ExtensionAPIVersion, ID: "gcp.edge.native", Provider: "gcp", Version: "1.0.0"},
		Resilience: &sdk.ResilienceAdapterDescriptor{APIVersion: sdk.ExtensionAPIVersion, ID: "gcp.resilience", Provider: "gcp", Version: "1.0.0"},
	}
}

func TestCallUsesPluginMethodTable(t *testing.T) {
	t.Parallel()
	caller := &stubCaller{respond: func(method string, reply any) error {
		if method != "Plugin.ValidateConfig" {
			return errors.New("unexpected method " + method)
		}
		*(reply.(*sdk.ValidateConfigResult)) = sdk.ValidateConfigResult{Valid: true}
		return nil
	}}
	client := &Client{caller: caller}
	resp, err := Call[sdk.ValidateConfigRequest, sdk.ValidateConfigResult](context.Background(), client, sdk.OpValidateConfig, &sdk.ValidateConfigRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Valid {
		t.Fatal("valid is false")
	}
	if len(caller.calls) != 1 || caller.calls[0] != "Plugin.ValidateConfig" {
		t.Fatalf("calls = %#v", caller.calls)
	}
}

func TestCallFailsClosed(t *testing.T) {
	t.Parallel()
	if _, err := Call[sdk.DescribeRequest, sdk.DescribeResponse](context.Background(), nil, sdk.OpDescribe, &sdk.DescribeRequest{}); err == nil {
		t.Fatal("nil client was accepted")
	}
	if _, err := Call[sdk.DescribeRequest, sdk.DescribeResponse](context.Background(), &Client{}, sdk.OpDescribe, &sdk.DescribeRequest{}); err == nil {
		t.Fatal("disconnected client was accepted")
	}
	caller := &stubCaller{respond: func(string, any) error { return errors.New("transport boom") }}
	if _, err := Call[sdk.DescribeRequest, sdk.DescribeResponse](context.Background(), &Client{caller: caller}, sdk.OpDescribe, &sdk.DescribeRequest{}); err == nil || !strings.Contains(err.Error(), "transport boom") {
		t.Fatalf("transport error = %v", err)
	}
	if _, err := Call[sdk.DescribeRequest, sdk.DescribeResponse](context.Background(), &Client{caller: caller}, "nope", &sdk.DescribeRequest{}); err == nil {
		t.Fatal("unknown operation was accepted")
	}
}

func TestCallCancelKillsAndFails(t *testing.T) {
	t.Parallel()
	caller := &stubCaller{block: make(chan struct{})}
	client := &Client{caller: caller}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Call[sdk.DescribeRequest, sdk.DescribeResponse](ctx, client, sdk.OpDescribe, &sdk.DescribeRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error = %v", err)
	}
}

func TestAsPluginError(t *testing.T) {
	t.Parallel()
	if AsPluginError(sdk.OpApply, nil) != nil {
		t.Fatal("nil error converted to non-nil")
	}
	err := AsPluginError(sdk.OpApply, &sdk.OperationError{Code: sdk.ErrCodeConflict, Message: "busy", Retryable: true})
	pluginErr, ok := err.(*PluginError)
	if !ok || pluginErr.Operation != sdk.OpApply || pluginErr.Code != sdk.ErrCodeConflict || !pluginErr.Retryable {
		t.Fatalf("error = %#v", err)
	}
	if !strings.Contains(pluginErr.Error(), "apply") || !strings.Contains(pluginErr.Error(), "busy") {
		t.Fatalf("text = %q", pluginErr.Error())
	}
}

func TestDialV2RefusesBadBinary(t *testing.T) {
	t.Parallel()
	if _, err := DialV2(context.Background(), "", DialOptions{}); err == nil {
		t.Fatal("empty binary was accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := DialV2(ctx, "/bin/false", DialOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled dial error = %v", err)
	}
	if _, err := DialV2(context.Background(), "/bin/false", DialOptions{}); err == nil {
		t.Fatal("exiting binary was accepted")
	}
}

var (
	v2BinaryOnce sync.Once
	v2BinaryPath string
	v2BinaryErr  error
)

func v2ProviderBinary(t *testing.T) string {
	t.Helper()
	v2BinaryOnce.Do(func() {
		_, file, _, _ := runtime.Caller(0)
		root := filepath.Dir(filepath.Dir(filepath.Dir(file)))
		out := filepath.Join(os.TempDir(), "magelift-provider-gcp-v2test")
		cmd := exec.Command("go", "build", "-o", out, "./providers/gcp/cmd/magelift-provider-gcp")
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil {
			v2BinaryErr = errors.New("build provider: " + err.Error() + "\n" + string(output))
			return
		}
		v2BinaryPath = out
	})
	if v2BinaryErr != nil {
		t.Fatal(v2BinaryErr)
	}
	return v2BinaryPath
}

func TestDialV2NegotiatesLivePlugin(t *testing.T) {
	binary := v2ProviderBinary(t)
	var logs bytes.Buffer
	logger := hclog.New(&hclog.LoggerOptions{Level: hclog.Info, Output: &logs})
	client, err := DialV2(context.Background(), binary, DialOptions{Logger: logger})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	if client.ProviderID() != "gcp" || client.ProviderVersion() == "" {
		t.Fatalf("identity = %q %q", client.ProviderID(), client.ProviderVersion())
	}
	described := client.Describe()
	if described == nil || len(described.Operations) != len(sdk.PluginMethods) {
		t.Fatalf("describe = %#v", described)
	}
	if !strings.Contains(logs.String(), "using provider plugin") || !strings.Contains(logs.String(), "gcp") {
		t.Fatalf("selection log = %q", logs.String())
	}
	resp, err := Call[sdk.ValidateConfigRequest, sdk.ValidateConfigResult](context.Background(), client, sdk.OpValidateConfig, &sdk.ValidateConfigRequest{
		ProtocolVersion: sdk.ProtocolV1, Runtime: "gke-autopilot", TargetBlock: []byte("project: shop-prod\nregion: europe-west1\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatal(resp.Error)
	}
	if !resp.Valid {
		t.Fatalf("problems = %#v", resp.Problems)
	}
}

func TestDialV2RefusesMissingOperation(t *testing.T) {
	binary := v2ProviderBinary(t)
	if _, err := DialV2(context.Background(), binary, DialOptions{Required: []sdk.Operation{"nope"}}); err == nil {
		t.Fatal("missing operation was accepted")
	}
}

func TestLazyClientDialsOnce(t *testing.T) {
	t.Parallel()
	dials := 0
	connected := &Client{caller: &stubCaller{respond: func(string, any) error { return nil }}, describe: cannedDescribe()}
	lazy := NewLazyClient(func(context.Context) (*Client, error) {
		dials++
		return connected, nil
	})
	if _, err := Call[sdk.DescribeRequest, sdk.DescribeResponse](context.Background(), lazy, sdk.OpDescribe, &sdk.DescribeRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := Call[sdk.DescribeRequest, sdk.DescribeResponse](context.Background(), lazy, sdk.OpDescribe, &sdk.DescribeRequest{}); err != nil {
		t.Fatal(err)
	}
	if dials != 1 {
		t.Fatalf("dials = %d", dials)
	}
	if lazy.Describe() != connected.describe {
		t.Fatal("describe was not delegated")
	}
}

func TestLazyClientPropagatesDialFailure(t *testing.T) {
	t.Parallel()
	lazy := NewLazyClient(func(context.Context) (*Client, error) {
		return nil, errors.New("dial boom")
	})
	if _, err := Call[sdk.DescribeRequest, sdk.DescribeResponse](context.Background(), lazy, sdk.OpDescribe, &sdk.DescribeRequest{}); err == nil || !strings.Contains(err.Error(), "dial boom") {
		t.Fatalf("dial error = %v", err)
	}
	if _, err := lazy.DescribeWith(context.Background()); err == nil {
		t.Fatal("DescribeWith succeeded without a session")
	}
}

func TestStaticPreDialValuesMatchAdvertised(t *testing.T) {
	binary := v2ProviderBinary(t)
	client, err := DialV2(context.Background(), binary, DialOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	described := client.Describe()
	if described == nil {
		t.Fatal("no describe response")
	}
	for _, runtime := range []string{"gke-autopilot", "gke-standard"} {
		module, err := NewShimModule(sdk.RuntimeID(runtime), client)
		if err != nil {
			t.Fatal(err)
		}
		var want sdk.ExtensionCertificationTier
		for _, advertised := range described.Runtimes {
			if advertised.Runtime == runtime {
				want = advertised.Tier
			}
		}
		var got sdk.ExtensionCertificationTier = sdk.ExtensionTierExperimental
		if module.CertificationTier() == "certified" {
			got = sdk.ExtensionTierCertified
		}
		if got != want {
			t.Fatalf("runtime %q static tier = %q, advertised = %q", runtime, got, want)
		}
	}
	advertised := map[string]bool{}
	for _, key := range described.OutputKeys {
		advertised[key] = true
	}
	module, err := NewShimModule("gke-autopilot", client)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range module.OutputKeys() {
		if !advertised[key] {
			t.Fatalf("static key %q is not advertised", key)
		}
	}
	for _, key := range described.OutputKeys {
		found := false
		for _, static := range module.OutputKeys() {
			if static == key {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("advertised key %q is missing statically", key)
		}
	}
}
