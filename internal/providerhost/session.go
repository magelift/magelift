package providerhost

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	sdk "github.com/magelift/magelift/sdk/v1"
)

// Session is a verified go-plugin gRPC connection. Close kills the subprocess.
type Session struct {
	client *plugin.Client
	api    API
}

// Dial starts a HashiCorp go-plugin gRPC subprocess. Callers must verify the
// binary with Load/VerifyLocal first. This does not switch Magento cells off
// the in-process path.
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
	return &Session{client: client, api: api}, nil
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

func (s *Session) Close() {
	if s == nil || s.client == nil {
		return
	}
	s.client.Kill()
}
