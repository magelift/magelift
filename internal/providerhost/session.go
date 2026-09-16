package providerhost

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"github.com/magelift/magelift/sdk"
)

// Session is a verified go-plugin gRPC connection. Close kills the subprocess.
type Session struct {
	client *plugin.Client
	api    API
}

var _ API = (*Session)(nil)

// Dial starts a HashiCorp go-plugin gRPC subprocess. Callers must verify the
// binary with Load/VerifyLocal first. Dial pings the subprocess and refuses
// SDK API versions other than the host version before returning the session.
// This does not switch Magento cells off the in-process path.
func Dial(ctx context.Context, binaryPath string) (*Session, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(binaryPath) == "" {
		return nil, errors.New("provider binary path is required")
	}
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig:  Handshake,
		Plugins:          pluginSet(nil),
		Cmd:              exec.Command(binaryPath),
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		Logger:           hclog.NewNullLogger(),
		AutoMTLS:         true,
	})
	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("start provider plugin: %w", err)
	}
	raw, err := rpcClient.Dispense(PluginName)
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("dispense provider plugin: %w", err)
	}
	api, ok := raw.(API)
	if !ok {
		client.Kill()
		return nil, errors.New("dispensed plugin does not implement provider API")
	}
	version, err := api.Ping(ctx)
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("ping provider plugin: %w", err)
	}
	if err := checkAPIVersion(version); err != nil {
		client.Kill()
		return nil, err
	}
	return &Session{client: client, api: api}, nil
}

// checkAPIVersion refuses a plugin whose SDK API version differs from the
// host. A mismatch means the JSON contracts may have drifted.
func checkAPIVersion(version string) error {
	if strings.TrimSpace(version) != SDKAPIVersion {
		return fmt.Errorf("%w: plugin reports %q, host requires %q", ErrUnsupportedAPI, version, SDKAPIVersion)
	}
	return nil
}

func (s *Session) Ping(ctx context.Context) (string, error) {
	if s == nil || s.api == nil {
		return "", errors.New("provider plugin session is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return s.api.Ping(ctx)
}

func (s *Session) Describe(ctx context.Context) (Identity, error) {
	if s == nil || s.api == nil {
		return Identity{}, errors.New("provider plugin session is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return s.api.Describe(ctx)
}

func (s *Session) Plan(ctx context.Context, request sdk.ModulePlanRequest) (sdk.ModulePlan, error) {
	if s == nil || s.api == nil {
		return sdk.ModulePlan{}, errors.New("provider plugin session is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return s.api.Plan(ctx, request)
}

func (s *Session) Program(ctx context.Context, plan sdk.ModulePlan) (ProgramResult, error) {
	if s == nil || s.api == nil {
		return ProgramResult{}, errors.New("provider plugin session is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return s.api.Program(ctx, plan)
}

func (s *Session) Execute(ctx context.Context, request ExecuteRequest) (ExecuteResult, error) {
	if s == nil || s.api == nil {
		return ExecuteResult{}, errors.New("provider plugin session is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return s.api.Execute(ctx, request)
}

func (s *Session) Close() {
	if s == nil || s.client == nil {
		return
	}
	s.client.Kill()
}
