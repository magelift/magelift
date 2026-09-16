package providerhost

import (
	"context"
	"errors"
	"fmt"
	"net/rpc"
	"os/exec"
	"strings"
	"sync"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"github.com/magelift/magelift/sdk"
)

// rpcCaller abstracts net/rpc for tests. Production uses *rpc.Client.
type rpcCaller interface {
	Call(serviceMethod string, args any, reply any) error
}

// RPCCaller is the exported test seam for scripted v2 sessions. Test
// binaries in other packages implement it to drive shims and backends
// without spawning a plugin process.
type RPCCaller interface {
	Call(serviceMethod string, args any, reply any) error
}

// NewTestClient builds a client over a scripted caller for tests.
func NewTestClient(caller RPCCaller, described *sdk.DescribeResponse) *Client {
	return &Client{caller: caller, describe: described}
}

// V2HandshakeConfig gates transport compatibility. It mirrors the plugin
// side built from the same SDK constants; the SDK stays dependency-free.
var V2HandshakeConfig = plugin.HandshakeConfig{
	ProtocolVersion:  sdk.HandshakeProtocolVersion,
	MagicCookieKey:   sdk.HandshakeCookieKey,
	MagicCookieValue: sdk.HandshakeCookieValue,
}

// v2HostPlugin serves the host side of the v2 plugin. The host never serves
// methods; Client wraps the raw rpc connection for typed calls. Asymmetric
// plugin implementations are standard go-plugin: both sides agree on the
// plugin name, never on the Go type.
type v2HostPlugin struct {
	client *Client
}

var _ plugin.Plugin = (*v2HostPlugin)(nil)

func (p *v2HostPlugin) Server(*plugin.MuxBroker) (any, error) {
	return nil, errors.New("provider host never serves the v2 plugin")
}

func (p *v2HostPlugin) Client(_ *plugin.MuxBroker, c *rpc.Client) (any, error) {
	if p == nil || p.client == nil {
		return nil, errors.New("v2 host plugin has no client")
	}
	p.client.caller = c
	return p.client, nil
}

// DialFunc starts and negotiates a v2 provider session.
type DialFunc func(context.Context) (*Client, error)

// Client is a negotiated v2 provider session. Close kills the subprocess.
// A lazy client dials on first use; use NewLazyClient for registry-held
// modules that must exist without credentials or processes.
type Client struct {
	caller  rpcCaller
	process *plugin.Client
	logger  hclog.Logger

	provider        string
	providerVersion string
	describe        *sdk.DescribeResponse

	mu       sync.Mutex
	dial     DialFunc
	delegate *Client
}

// NewLazyClient builds a client that dials once on first use and caches
// the session. The first caller supplies the dial context.
func NewLazyClient(dial DialFunc) *Client {
	return &Client{dial: dial}
}

// ensure dials a lazy client. Connected clients return themselves.
func (c *Client) ensure(ctx context.Context) (*Client, error) {
	if c == nil {
		return nil, errors.New("provider client is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.caller != nil {
		return c, nil
	}
	if c.delegate != nil {
		return c.delegate, nil
	}
	if c.dial == nil {
		return nil, errors.New("provider client is not connected")
	}
	delegate, err := c.dial(ctx)
	if err != nil {
		return nil, err
	}
	if delegate == nil {
		return nil, errors.New("provider dial returned no client")
	}
	c.delegate = delegate
	return delegate, nil
}

// DialOptions configures v2 dialing.
type DialOptions struct {
	// Logger receives the selection line plus plugin stderr. Nil silences.
	Logger hclog.Logger
	// Required lists the operations negotiation must find. Nil requires all
	// known operations: a partial plugin fails closed.
	Required []sdk.Operation
}

// DialV2 starts a verified v2 provider subprocess and negotiates the
// protocol: handshake transport compat, then Describe semantics (protocol
// major plus required operations). Callers must verify the binary with
// Load/VerifyLocal first. Every failure kills the subprocess; nothing
// falls back.
func DialV2(ctx context.Context, binaryPath string, opts DialOptions) (*Client, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(binaryPath) == "" {
		return nil, errors.New("provider binary path is required")
	}
	logger := opts.Logger
	if logger == nil {
		logger = hclog.NewNullLogger()
	}
	client := &Client{logger: logger}
	host := &v2HostPlugin{client: client}
	// NetRPC is plaintext localhost IPC: go-plugin offers mTLS for gRPC
	// only. The peer runs as the same user from a digest plus Cosign
	// verified binary, so localhost carries no wider trust than the CLI
	// process itself, which already holds the stack passphrase and secret
	// values in memory.
	process := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig:  V2HandshakeConfig,
		Plugins:          map[string]plugin.Plugin{sdk.PluginName: host},
		Cmd:              exec.Command(binaryPath),
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolNetRPC},
		Logger:           logger,
	})
	rpcClient, err := process.Client()
	if err != nil {
		process.Kill()
		return nil, fmt.Errorf("start provider plugin: %w", err)
	}
	raw, err := rpcClient.Dispense(sdk.PluginName)
	if err != nil {
		process.Kill()
		return nil, fmt.Errorf("dispense provider plugin: %w", err)
	}
	negotiated, ok := raw.(*Client)
	if !ok || negotiated.caller == nil {
		process.Kill()
		return nil, errors.New("dispensed plugin is not a v2 client")
	}
	negotiated.process = process
	described, err := negotiated.describeOnce(ctx)
	if err != nil {
		process.Kill()
		return nil, fmt.Errorf("negotiate provider protocol: %w", err)
	}
	if err := sdk.ValidateProtocolVersion(described.ProtocolVersion); err != nil {
		process.Kill()
		return nil, fmt.Errorf("negotiate provider protocol: %w", err)
	}
	required := opts.Required
	if required == nil {
		for operation := range sdk.PluginMethods {
			required = append(required, operation)
		}
	}
	for _, operation := range required {
		if err := sdk.RequireOperation(described.Operations, operation, "1.0"); err != nil {
			process.Kill()
			return nil, fmt.Errorf("negotiate provider protocol: %w", err)
		}
	}
	negotiated.describe = described
	negotiated.provider = described.ProviderID
	negotiated.providerVersion = described.ProviderVersion
	logger.Info("using provider plugin", "provider", described.ProviderID, "version", described.ProviderVersion, "protocol", described.ProtocolVersion, "operations", len(described.Operations))
	return negotiated, nil
}

// describeOnce performs the negotiation Describe call.
func (c *Client) describeOnce(ctx context.Context) (*sdk.DescribeResponse, error) {
	resp, err := Call[sdk.DescribeRequest, sdk.DescribeResponse](ctx, c, sdk.OpDescribe, &sdk.DescribeRequest{ProtocolVersion: sdk.ProtocolV1})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, AsPluginError(sdk.OpDescribe, resp.Error)
	}
	return resp, nil
}

// Describe returns the cached negotiation response.
func (c *Client) Describe() *sdk.DescribeResponse {
	if c == nil {
		return nil
	}
	if c.describe == nil && c.delegate != nil {
		return c.delegate.describe
	}
	return c.describe
}

// DescribeWith dials a lazy client and returns the negotiation response.
func (c *Client) DescribeWith(ctx context.Context) (*sdk.DescribeResponse, error) {
	client, err := c.ensure(ctx)
	if err != nil {
		return nil, err
	}
	if client.describe == nil {
		return nil, errors.New("provider client has not negotiated Describe")
	}
	return client.describe, nil
}

// Call invokes one typed operation. The caller checks the typed result
// error: a nil Go error means transport success, and the response carries
// the operation outcome. Cancelling the context kills the plugin process
// (client abort).
func Call[Req any, Resp any](ctx context.Context, c *Client, operation sdk.Operation, req *Req) (*Resp, error) {
	client, err := c.ensure(ctx)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	method, err := sdk.PluginMethod(operation)
	if err != nil {
		return nil, err
	}
	resp := new(Resp)
	type outcome struct {
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		done <- outcome{err: client.caller.Call(method, req, resp)}
	}()
	select {
	case out := <-done:
		if out.err != nil {
			return nil, fmt.Errorf("provider %s: %w", operation, out.err)
		}
		return resp, nil
	case <-ctx.Done():
		client.Kill()
		return nil, fmt.Errorf("provider %s: %w", operation, ctx.Err())
	}
}

// ProviderID reports the negotiated provider ID.
func (c *Client) ProviderID() string {
	if c == nil {
		return ""
	}
	if c.provider == "" && c.delegate != nil {
		return c.delegate.provider
	}
	return c.provider
}

// ProviderVersion reports the negotiated provider version.
func (c *Client) ProviderVersion() string {
	if c == nil {
		return ""
	}
	if c.providerVersion == "" && c.delegate != nil {
		return c.delegate.providerVersion
	}
	return c.providerVersion
}

// Close kills the plugin subprocess.
func (c *Client) Close() {
	c.Kill()
}

// Kill terminates the plugin subprocess.
func (c *Client) Kill() {
	if c == nil {
		return
	}
	if c.process != nil {
		c.process.Kill()
		return
	}
	if c.delegate != nil {
		c.delegate.Kill()
	}
}

// PluginError is a typed provider operation failure.
type PluginError struct {
	Operation sdk.Operation
	Code      sdk.OperationErrorCode
	Message   string
	Retryable bool
}

func (e *PluginError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("provider %s: %s: %s", e.Operation, e.Code, e.Message)
}

// AsPluginError converts a typed operation error, preserving code and
// retryability for core callers that switch on failure kind.
func AsPluginError(operation sdk.Operation, operr *sdk.OperationError) error {
	if operr == nil {
		return nil
	}
	return &PluginError{Operation: operation, Code: operr.Code, Message: operr.Message, Retryable: operr.Retryable}
}
