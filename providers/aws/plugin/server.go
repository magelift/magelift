// Package plugin exposes AWS planning and operations through MageLift's provider protocol.
package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/rpc"
	"slices"
	"sync"
	"time"

	"github.com/aws/smithy-go"
	"github.com/hashicorp/go-plugin"
	"github.com/magelift/magelift/internal/automation"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/s3state"
	"github.com/magelift/magelift/internal/secretsafe"
	"github.com/magelift/magelift/providers/aws/edge"
	"github.com/magelift/magelift/providers/aws/eksops"
	"github.com/magelift/magelift/providers/aws/ops"
	"github.com/magelift/magelift/sdk"
)

var HandshakeConfig = plugin.HandshakeConfig{
	ProtocolVersion: sdk.HandshakeProtocolVersion, MagicCookieKey: sdk.HandshakeCookieKey, MagicCookieValue: sdk.HandshakeCookieValue,
}

type rpcPlugin struct{ server *Server }

func (p rpcPlugin) Server(*plugin.MuxBroker) (any, error) { return p.server, nil }
func (rpcPlugin) Client(*plugin.MuxBroker, *rpc.Client) (any, error) {
	return nil, errors.New("AWS provider serves RPC requests")
}
func PluginMap(server *Server) map[string]plugin.Plugin {
	return map[string]plugin.Plugin{sdk.PluginName: rpcPlugin{server}}
}

type Server struct {
	Version    string
	mu         sync.Mutex
	candidates map[string]*deploymentCandidate
	admit      func(context.Context, platform.StackModule, platform.PlannedStack) (platform.PlannedStack, error)
}

func module(runtime string) (platform.StackModule, error) {
	switch runtime {
	case "ecs-fargate":
		return ops.Module{}, nil
	case "eks":
		return eksops.Module{}, nil
	default:
		return nil, fmt.Errorf("unknown AWS runtime %q", runtime)
	}
}

func (s *Server) Describe(req *sdk.DescribeRequest, resp *sdk.DescribeResponse) error {
	if err := sdk.ValidateProtocolVersion(req.ProtocolVersion); err != nil {
		return err
	}
	operations := []sdk.Operation{
		sdk.OpDescribe, sdk.OpValidateConfig, sdk.OpPlan, sdk.OpPreview, sdk.OpApply, sdk.OpOutputs, sdk.OpDestroy,
		sdk.OpBootstrapVerify, sdk.OpBootstrapEnsure, sdk.OpStateStatus, sdk.OpStateLock, sdk.OpStateUnlock, sdk.OpStateBackup, sdk.OpStateRestore,
		sdk.OpSecretList, sdk.OpSecretSet, sdk.OpSecretRemove, sdk.OpSecretRead, sdk.OpCheckRuntime, sdk.OpPrepareExec, sdk.OpPrepareTunnel, sdk.OpTailLogs,
		sdk.OpMediaExport, sdk.OpMediaImport, sdk.OpCostInputs, sdk.OpDeployAppPhase, sdk.OpEdgePlan, sdk.OpEdgeExecute,
	}
	for _, operation := range operations {
		resp.Operations = append(resp.Operations, sdk.OperationVersion{Name: string(operation), Version: sdk.ProtocolV1})
	}
	desc := edge.Descriptor()
	resp.ProtocolVersion, resp.ProviderID, resp.ProviderVersion = sdk.ProtocolV1, "aws", s.Version
	resp.Runtimes = []sdk.RuntimeAdvertisement{{Runtime: "ecs-fargate", Tier: sdk.ExtensionTierBuiltIn}, {Runtime: "eks", Tier: sdk.ExtensionTierExperimental}}
	keys := append(ops.Module{}.OutputKeys(), eksops.Module{}.OutputKeys()...)
	slices.Sort(keys)
	resp.OutputKeys, resp.Edge = slices.Compact(keys), &desc
	return nil
}

func invalid(err error) error {
	return &sdk.OperationError{Code: sdk.ErrCodeInvalid, Message: err.Error()}
}
func integrity(message string) error {
	return &sdk.OperationError{Code: sdk.ErrCodeIntegrity, Message: message}
}

func operationError(err error) *sdk.OperationError {
	message, _ := secretsafe.RedactSensitiveText(err.Error())
	result := &sdk.OperationError{Code: sdk.ErrCodeUpstream, Message: message}
	var typed *sdk.OperationError
	var ownership *automation.PreviewOwnershipError
	var api smithy.APIError
	switch {
	case errors.As(err, &typed):
		result.Code = typed.Code
		result.Retryable = typed.Retryable
		result.Message, _ = secretsafe.RedactSensitiveText(typed.Message)
	case errors.Is(err, context.DeadlineExceeded):
		result.Code = sdk.ErrCodeTimeout
	case errors.Is(err, s3state.ErrLocked), errors.Is(err, s3state.ErrLockReplaced), errors.As(err, &ownership), automation.IsConcurrentUpdate(err):
		result.Code = sdk.ErrCodeConflict
	case errors.As(err, &api):
		switch api.ErrorCode() {
		case "UnauthorizedOperation", "AccessDenied", "AccessDeniedException", "ExpiredToken", "ExpiredTokenException", "InvalidClientTokenId", "UnrecognizedClientException", "InvalidAccessKeyId", "SignatureDoesNotMatch":
			result.Code = sdk.ErrCodeCredential
		case "NoSuchBucket", "NoSuchKey", "ResourceNotFoundException", "ParameterNotFound":
			result.Code = sdk.ErrCodeNotFound
		case "PreconditionFailed", "ConditionalRequestConflict":
			result.Code = sdk.ErrCodeConflict
		case "Throttling", "ThrottlingException", "TooManyRequestsException", "ServiceUnavailable":
			result.Retryable = true
		}
	}
	return result
}

func respond(version string, failure **sdk.OperationError, timeout time.Duration, run func(context.Context) error) error {
	if err := sdk.ValidateProtocolVersion(version); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := run(ctx); err != nil {
		*failure = operationError(err)
	}
	return nil
}

func encode(dst *[]byte, data any) error {
	body, err := json.Marshal(data)
	if err == nil {
		*dst = body
	}
	return err
}
func decode[T any](data []byte) (T, error) {
	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		return result, invalid(errors.New("invalid JSON in AWS provider request"))
	}
	return result, nil
}
