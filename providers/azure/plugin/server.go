// Package plugin serves the Azure provider's versioned discovery contract.
package plugin

import (
	"context"
	"errors"
	"fmt"
	"net/rpc"
	"sort"
	"strings"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/hashicorp/go-plugin"
	"github.com/magelift/magelift/internal/cloud/kube"
	azurestate "github.com/magelift/magelift/providers/azure/state"
	"github.com/magelift/magelift/sdk"
	"gopkg.in/yaml.v3"
)

var HandshakeConfig = plugin.HandshakeConfig{
	ProtocolVersion:  sdk.HandshakeProtocolVersion,
	MagicCookieKey:   sdk.HandshakeCookieKey,
	MagicCookieValue: sdk.HandshakeCookieValue,
}

// PluginMap exposes the Azure provider through MageLift's normal RPC protocol.
func PluginMap(version string) map[string]plugin.Plugin {
	return map[string]plugin.Plugin{sdk.PluginName: &rpcPlugin{version: version}}
}

type rpcPlugin struct{ version string }

func (p *rpcPlugin) Server(*plugin.MuxBroker) (any, error) {
	return &Server{Version: p.version}, nil
}

func (*rpcPlugin) Client(*plugin.MuxBroker, *rpc.Client) (any, error) {
	return nil, errors.New("Azure provider does not implement the host client")
}

// Server implements the Azure operations advertised through Describe.
type Server struct {
	Version         string
	KubeClients     kube.ClientFactory
	NewDeployStores func(kube.ClientFactory, map[string]any) (*kube.CandidateStore, *kube.DeploymentRuntime, error)
	HTTPDoer        kube.HTTPDoer
	preview         func(context.Context, storedPlan, []byte) (sdk.ChangeSummary, error)
	mutate          func(context.Context, storedPlan, []byte, bool) (sdk.ChangeSummary, error)
	outputs         func(context.Context, storedPlan) (map[string]any, []string, error)
	stateFactory    func(context.Context, storedPlan) (*azurestate.Manager, error)
	blobFactory     func(context.Context, storedPlan) (*azblob.Client, error)
	lockMu          sync.Mutex
	locks           map[string]*azurestate.Handle
}

func (s *Server) Describe(req *sdk.DescribeRequest, resp *sdk.DescribeResponse) error {
	if req == nil || resp == nil {
		return errors.New("describe request and response are required")
	}
	if err := sdk.ValidateProtocolVersion(req.ProtocolVersion); err != nil {
		return err
	}
	*resp = sdk.DescribeResponse{
		ProtocolVersion: sdk.ProtocolV1, ProviderID: "azure", ProviderVersion: s.Version,
		Operations: []sdk.OperationVersion{
			{Name: string(sdk.OpDescribe), Version: "1.0"},
			{Name: string(sdk.OpValidateConfig), Version: "1.0"},
			{Name: string(sdk.OpPlan), Version: "1.0"},
			{Name: string(sdk.OpPreview), Version: "1.0"},
			{Name: string(sdk.OpApply), Version: "1.0"},
			{Name: string(sdk.OpDestroy), Version: "1.0"},
			{Name: string(sdk.OpOutputs), Version: "1.0"},
			{Name: string(sdk.OpTailLogs), Version: "1.0"},
			{Name: string(sdk.OpCheckRuntime), Version: "1.0"},
			{Name: string(sdk.OpPrepareExec), Version: "1.0"},
			{Name: string(sdk.OpPrepareTunnel), Version: "1.0"},
			{Name: string(sdk.OpDeployAppPhase), Version: "1.0"},
			{Name: string(sdk.OpStateStatus), Version: "1.0"},
			{Name: string(sdk.OpStateLock), Version: "1.0"},
			{Name: string(sdk.OpStateUnlock), Version: "1.0"},
			{Name: string(sdk.OpBootstrapVerify), Version: "1.0"},
			{Name: string(sdk.OpBootstrapEnsure), Version: "1.0"},
		},
		Runtimes: []sdk.RuntimeAdvertisement{{Runtime: "aks", Tier: sdk.ExtensionTierExperimental}},
	}
	return nil
}

type target struct {
	SubscriptionID     string       `yaml:"subscriptionId"`
	ResourceGroup      string       `yaml:"resourceGroup"`
	TenantID           string       `yaml:"tenantId,omitempty"`
	ImageDigest        string       `yaml:"imageDigest,omitempty"`
	StateBackendURL    string       `yaml:"stateBackendUrl,omitempty"`
	ImagePullSecret    string       `yaml:"imagePullSecret,omitempty"`
	ACRResourceID      string       `yaml:"acrResourceId,omitempty"`
	IngressClass       string       `yaml:"ingressClass,omitempty"`
	TLSSecret          string       `yaml:"tlsSecret,omitempty"`
	MonitorWorkspaceID string       `yaml:"monitorWorkspaceId,omitempty"`
	PreviewPool        *previewPool `yaml:"previewPool,omitempty"`
}

type previewPool struct {
	ClusterName    string `yaml:"clusterName"`
	NodeSubnetID   string `yaml:"nodeSubnetId"`
	MySQLSubnetID  string `yaml:"mysqlSubnetId"`
	MySQLDNSZoneID string `yaml:"mysqlDnsZoneId"`
}

func (s *Server) ValidateConfig(req *sdk.ValidateConfigRequest, resp *sdk.ValidateConfigResult) error {
	if req == nil || resp == nil {
		return errors.New("validate request and response are required")
	}
	if err := sdk.ValidateProtocolVersion(req.ProtocolVersion); err != nil {
		return err
	}
	var config target
	decoder := yaml.NewDecoder(strings.NewReader(string(req.TargetBlock)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		resp.Error = &sdk.OperationError{Code: sdk.ErrCodeInvalid, Message: fmt.Sprintf("invalid Azure target: %v", err)}
		return nil
	}
	var problems []string
	if req.Runtime != "aks" {
		problems = append(problems, "Azure runtime must be aks")
	}
	if strings.TrimSpace(config.SubscriptionID) == "" {
		problems = append(problems, "subscriptionId is required")
	}
	if strings.TrimSpace(config.ResourceGroup) == "" {
		problems = append(problems, "resourceGroup is required")
	}
	sort.Strings(problems)
	*resp = sdk.ValidateConfigResult{Valid: len(problems) == 0, Problems: problems}
	return nil
}
