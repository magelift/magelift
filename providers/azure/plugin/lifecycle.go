package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	"github.com/magelift/magelift/internal/automation"
	"github.com/magelift/magelift/internal/secretsafe"
	"github.com/magelift/magelift/providers/azure/stack"
	"github.com/magelift/magelift/sdk"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func (s *Server) Apply(req *sdk.StackCall, resp *sdk.LifecycleResult) error {
	return s.mutateStack(req, resp, false)
}

func (s *Server) Destroy(req *sdk.StackCall, resp *sdk.LifecycleResult) error {
	return s.mutateStack(req, resp, true)
}

func (s *Server) mutateStack(req *sdk.StackCall, resp *sdk.LifecycleResult, destroy bool) error {
	if req == nil || resp == nil {
		return errors.New("Azure lifecycle request and response are required")
	}
	if err := sdk.ValidateProtocolVersion(req.ProtocolVersion); err != nil {
		return err
	}
	plan, err := checkedStoredPlan(req)
	if err != nil {
		resp.Error = &sdk.OperationError{Code: sdk.ErrCodeInvalid, Message: err.Error()}
		return nil
	}
	if plan.AccountOnly || plan.Spec.ObserveOnly || plan.DestroyPreview != destroy || (destroy && plan.Envelope.Protected) {
		resp.Error = &sdk.OperationError{Code: sdk.ErrCodeInvalid, Message: "Azure operation does not match the plan or the environment is protected"}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	runner := s.mutate
	if runner == nil {
		runner = runMutation
	}
	summary, err := runner(ctx, plan, req.PreviewMetadataJSON, destroy)
	if err != nil {
		message, _ := secretsafe.RedactSensitiveText(err.Error())
		resp.Error = &sdk.OperationError{Code: sdk.ErrCodeUpstream, Message: message}
		return nil
	}
	resp.Summary = summary
	return nil
}

func runMutation(ctx context.Context, plan storedPlan, rawMetadata []byte, destroy bool) (sdk.ChangeSummary, error) {
	metadata, err := parsePreviewMetadata(rawMetadata)
	if err != nil {
		return sdk.ChangeSummary{}, err
	}
	pulumiStack, err := openStack(ctx, plan, destroy)
	if err != nil {
		return sdk.ChangeSummary{}, err
	}
	backend := automation.NewPulumiBackend(pulumiStack)
	request := automation.Request{Target: sdk.TargetDescriptor{ID: "azure.aks", Provider: "azure", Runtime: "aks"}, Preview: metadata, Destroy: destroy}
	var changes map[string]int
	if destroy {
		changes, err = backend.Destroy(ctx, request, io.Discard)
	} else {
		changes, err = backend.Update(ctx, request, io.Discard)
	}
	if err != nil {
		return sdk.ChangeSummary{}, err
	}
	return summarizeChanges(changes), nil
}

func openStack(ctx context.Context, plan storedPlan, selectOnly bool) (*auto.Stack, error) {
	if strings.TrimSpace(os.Getenv("PULUMI_CONFIG_PASSPHRASE")) == "" && strings.TrimSpace(os.Getenv("PULUMI_CONFIG_PASSPHRASE_FILE")) == "" {
		return nil, errors.New("set the Azure Pulumi state passphrase before opening the stack")
	}
	if !selectOnly && plan.Spec.EncryptionKeySecret != "" {
		vault, name, version, err := parseEncryptionSecretID(plan.Spec.EncryptionKeySecret)
		if err != nil {
			return nil, err
		}
		credential, err := azidentity.NewDefaultAzureCredential(nil)
		if err != nil {
			return nil, fmt.Errorf("create Azure credential: %w", err)
		}
		client, err := azsecrets.NewClient(vault, credential, nil)
		if err != nil {
			return nil, fmt.Errorf("create Key Vault client: %w", err)
		}
		result, err := client.GetSecret(ctx, name, version, nil)
		if err != nil {
			return nil, fmt.Errorf("read configured Magento encryption key from Key Vault: %w", err)
		}
		if result.Value == nil || strings.TrimSpace(*result.Value) == "" {
			return nil, errors.New("configured Magento encryption key is empty")
		}
		plan.Spec.EncryptionKey = pulumi.ToSecret(*result.Value).(pulumi.StringOutput)
	}
	if !selectOnly {
		return automation.NewInlineStackWithBackend(ctx, plan.Envelope.StackName, stack.Program(plan.Spec), plan.BackendURL)
	}
	selected, err := auto.SelectStackInlineSource(ctx, plan.Envelope.StackName, "magelift", stack.Program(plan.Spec), auto.EnvVars(map[string]string{"PULUMI_BACKEND_URL": plan.BackendURL}))
	if err != nil {
		return nil, fmt.Errorf("select Azure Pulumi stack: %w", err)
	}
	return &selected, nil
}

func (s *Server) Outputs(req *sdk.StackCall, resp *sdk.OutputsResult) error {
	if req == nil || resp == nil {
		return errors.New("Azure outputs request and response are required")
	}
	if err := sdk.ValidateProtocolVersion(req.ProtocolVersion); err != nil {
		return err
	}
	plan, err := checkedStoredPlan(req)
	if err != nil || plan.AccountOnly {
		if err == nil {
			err = errors.New("account preparation plans have no stack outputs")
		}
		resp.Error = &sdk.OperationError{Code: sdk.ErrCodeInvalid, Message: err.Error()}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	reader := s.outputs
	if reader == nil {
		reader = readOutputs
	}
	values, secretKeys, err := reader(ctx, plan)
	if err != nil {
		message, _ := secretsafe.RedactSensitiveText(err.Error())
		resp.Error = &sdk.OperationError{Code: sdk.ErrCodeUpstream, Message: message}
		return nil
	}
	resp.ValuesJSON, err = json.Marshal(values)
	if err != nil {
		resp.Error = &sdk.OperationError{Code: sdk.ErrCodeInternal, Message: "encode Azure stack outputs"}
		return nil
	}
	sort.Strings(secretKeys)
	resp.SecretKeys = secretKeys
	return nil
}

func readOutputs(ctx context.Context, plan storedPlan) (map[string]any, []string, error) {
	pulumiStack, err := openStack(ctx, plan, true)
	if err != nil {
		return nil, nil, err
	}
	outputs, err := pulumiStack.Outputs(ctx)
	if err != nil {
		return nil, nil, err
	}
	values := make(map[string]any, len(outputs))
	var secretKeys []string
	for key, value := range outputs {
		values[key] = value.Value
		if value.Secret {
			secretKeys = append(secretKeys, key)
		}
	}
	return values, secretKeys, nil
}
