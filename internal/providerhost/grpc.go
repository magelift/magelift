package providerhost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hashicorp/go-plugin"
	"github.com/magelift/magelift/internal/providerhost/hostproto"
	sdk "github.com/magelift/magelift/sdk/v1"
	"google.golang.org/grpc"
)

const PluginName = "provider"

var Handshake = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "MAGELIFT_PROVIDER",
	MagicCookieValue: "magelift-provider-v1",
}

type providerGRPCPlugin struct {
	plugin.NetRPCUnsupportedPlugin
	Impl API
}

func (p *providerGRPCPlugin) GRPCServer(_ *plugin.GRPCBroker, server *grpc.Server) error {
	hostproto.RegisterProviderServer(server, &grpcProvider{Impl: p.Impl})
	return nil
}

func (p *providerGRPCPlugin) GRPCClient(_ context.Context, _ *plugin.GRPCBroker, conn *grpc.ClientConn) (interface{}, error) {
	return &grpcProviderClient{client: hostproto.NewProviderClient(conn)}, nil
}

type grpcProvider struct {
	hostproto.UnimplementedProviderServer
	Impl API
}

func (s *grpcProvider) Ping(ctx context.Context, _ *hostproto.PingRequest) (*hostproto.PingResponse, error) {
	if s.Impl == nil {
		return nil, errors.New("provider plugin implementation is nil")
	}
	version, err := s.Impl.Ping(ctx)
	if err != nil {
		return nil, err
	}
	return &hostproto.PingResponse{SdkApiVersion: version}, nil
}

func (s *grpcProvider) Describe(ctx context.Context, _ *hostproto.DescribeRequest) (*hostproto.DescribeResponse, error) {
	if s.Impl == nil {
		return nil, errors.New("provider plugin implementation is nil")
	}
	identity, err := s.Impl.Describe(ctx)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(identity)
	if err != nil {
		return nil, fmt.Errorf("encode provider identity: %w", err)
	}
	return &hostproto.DescribeResponse{IdentityJson: string(payload)}, nil
}

type grpcProviderClient struct {
	client hostproto.ProviderClient
}

func (c *grpcProviderClient) Ping(ctx context.Context) (string, error) {
	resp, err := c.client.Ping(ctx, &hostproto.PingRequest{})
	if err != nil {
		return "", err
	}
	return resp.GetSdkApiVersion(), nil
}

func (c *grpcProviderClient) Describe(ctx context.Context) (Identity, error) {
	resp, err := c.client.Describe(ctx, &hostproto.DescribeRequest{})
	if err != nil {
		return Identity{}, err
	}
	payload := resp.GetIdentityJson()
	if len(payload) > 64<<10 {
		return Identity{}, errors.New("provider identity exceeds 64KiB")
	}
	var identity Identity
	if err := json.Unmarshal([]byte(payload), &identity); err != nil {
		return Identity{}, fmt.Errorf("decode provider identity: %w", err)
	}
	return identity, nil
}

func (s *grpcProvider) Plan(ctx context.Context, req *hostproto.PlanRequest) (*hostproto.PlanResponse, error) {
	if s.Impl == nil {
		return nil, errors.New("provider plugin implementation is nil")
	}
	var request sdk.ModulePlanRequest
	if err := decodeJSON("plan request", req.GetRequestJson(), 1<<20, &request); err != nil {
		return nil, err
	}
	plan, err := s.Impl.Plan(ctx, request)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(plan)
	if err != nil {
		return nil, fmt.Errorf("encode module plan: %w", err)
	}
	return &hostproto.PlanResponse{PlanJson: string(payload)}, nil
}

func (s *grpcProvider) Program(ctx context.Context, req *hostproto.ProgramRequest) (*hostproto.ProgramResponse, error) {
	if s.Impl == nil {
		return nil, errors.New("provider plugin implementation is nil")
	}
	var plan sdk.ModulePlan
	if err := decodeJSON("module plan", req.GetPlanJson(), 1<<20, &plan); err != nil {
		return nil, err
	}
	result, err := s.Impl.Program(ctx, plan)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("encode program result: %w", err)
	}
	return &hostproto.ProgramResponse{ResultJson: string(payload)}, nil
}

func (c *grpcProviderClient) Plan(ctx context.Context, request sdk.ModulePlanRequest) (sdk.ModulePlan, error) {
	payload, err := json.Marshal(request)
	if err != nil {
		return sdk.ModulePlan{}, fmt.Errorf("encode plan request: %w", err)
	}
	resp, err := c.client.Plan(ctx, &hostproto.PlanRequest{RequestJson: string(payload)})
	if err != nil {
		return sdk.ModulePlan{}, err
	}
	var plan sdk.ModulePlan
	if err := decodeJSON("module plan", resp.GetPlanJson(), 1<<20, &plan); err != nil {
		return sdk.ModulePlan{}, err
	}
	return plan, nil
}

func (c *grpcProviderClient) Program(ctx context.Context, plan sdk.ModulePlan) (ProgramResult, error) {
	payload, err := json.Marshal(plan)
	if err != nil {
		return ProgramResult{}, fmt.Errorf("encode module plan: %w", err)
	}
	resp, err := c.client.Program(ctx, &hostproto.ProgramRequest{PlanJson: string(payload)})
	if err != nil {
		return ProgramResult{}, err
	}
	var result ProgramResult
	if err := decodeJSON("program result", resp.GetResultJson(), 64<<10, &result); err != nil {
		return ProgramResult{}, err
	}
	return result, nil
}

func decodeJSON(label, payload string, limit int, dest any) error {
	if len(payload) > limit {
		return fmt.Errorf("%s exceeds %d bytes", label, limit)
	}
	if err := json.Unmarshal([]byte(payload), dest); err != nil {
		return fmt.Errorf("decode %s: %w", label, err)
	}
	return nil
}

func pluginSet(impl API) plugin.PluginSet {
	return plugin.PluginSet{
		PluginName: &providerGRPCPlugin{Impl: impl},
	}
}
