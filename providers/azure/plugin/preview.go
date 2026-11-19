package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"

	"github.com/magelift/magelift/internal/automation"
	"github.com/magelift/magelift/internal/secretsafe"
	"github.com/magelift/magelift/sdk"
)

// Preview evaluates the Pulumi graph using the selected Azure state backend.
func (s *Server) Preview(req *sdk.StackCall, resp *sdk.LifecycleResult) error {
	if req == nil || resp == nil {
		return errors.New("preview request and response are required")
	}
	if err := sdk.ValidateProtocolVersion(req.ProtocolVersion); err != nil {
		return err
	}
	plan, err := checkedStoredPlan(req)
	if err != nil {
		resp.Error = &sdk.OperationError{Code: sdk.ErrCodeInvalid, Message: err.Error()}
		return nil
	}
	if plan.AccountOnly || plan.Spec.ObserveOnly {
		resp.Error = &sdk.OperationError{Code: sdk.ErrCodeInvalid, Message: "account and observation plans cannot preview infrastructure"}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	runner := s.preview
	if runner == nil {
		runner = previewStack
	}
	summary, err := runner(ctx, plan, req.PreviewMetadataJSON)
	if err != nil {
		message, _ := secretsafe.RedactSensitiveText(err.Error())
		resp.Error = &sdk.OperationError{Code: sdk.ErrCodeUpstream, Message: message}
		return nil
	}
	resp.Summary = summary
	return nil
}

func checkedStoredPlan(req *sdk.StackCall) (storedPlan, error) {
	var plan storedPlan
	if err := json.Unmarshal(req.Plan.Opaque, &plan); err != nil {
		return storedPlan{}, errors.New("invalid Azure stored plan")
	}
	if req.Plan.Provider != "azure" || req.Plan.Runtime != "aks" || req.Plan.StackName == "" ||
		req.Plan.StackName != plan.Envelope.StackName || req.Plan.ImageDigest != plan.Spec.ApplicationImage ||
		req.Plan.StateBackendURL != plan.BackendURL || !reflect.DeepEqual(req.Envelope, plan.Envelope) {
		return storedPlan{}, errors.New("Azure stored plan identity does not match the requested environment")
	}
	if plan.AccountOnly {
		if plan.Spec.SubscriptionID == "" || plan.Spec.TenantID == "" || plan.Spec.ResourceGroup == "" || plan.Spec.Region == "" || plan.Spec.Project == "" || plan.Spec.Environment == "" {
			return storedPlan{}, errors.New("Azure account plan is incomplete")
		}
		return plan, nil
	}
	if plan.Spec.DestroyOnly != plan.DestroyPreview {
		return storedPlan{}, errors.New("Azure teardown plan does not match its operation")
	}
	if err := plan.Spec.Validate(); err != nil {
		return storedPlan{}, err
	}
	return plan, nil
}

func previewStack(ctx context.Context, plan storedPlan, rawMetadata []byte) (sdk.ChangeSummary, error) {
	metadata, err := parsePreviewMetadata(rawMetadata)
	if err != nil {
		return sdk.ChangeSummary{}, err
	}
	pulumiStack, err := openStack(ctx, plan, plan.DestroyPreview)
	if err != nil {
		return sdk.ChangeSummary{}, fmt.Errorf("open Azure Pulumi stack: %w", err)
	}
	changes, err := automation.NewPulumiBackend(pulumiStack).Preview(ctx, automation.Request{
		Target: sdk.TargetDescriptor{ID: "azure.aks", Provider: "azure", Runtime: "aks"}, Preview: metadata, Destroy: plan.DestroyPreview,
	}, io.Discard)
	if err != nil {
		return sdk.ChangeSummary{}, err
	}
	return summarizeChanges(changes), nil
}

func parsePreviewMetadata(raw []byte) (*automation.PreviewMetadata, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var metadata automation.PreviewMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return nil, errors.New("invalid preview ownership metadata")
	}
	if err := metadata.Validate(); err != nil {
		return nil, err
	}
	return &metadata, nil
}

func summarizeChanges(changes map[string]int) sdk.ChangeSummary {
	var summary sdk.ChangeSummary
	for operation, count := range changes {
		switch strings.ToLower(strings.TrimSpace(operation)) {
		case "create", "add":
			summary.Create += count
		case "update", "change", "modify":
			summary.Update += count
		case "delete", "remove":
			summary.Delete += count
		case "replace":
			summary.Replace += count
		case "same", "unchanged", "read", "import":
			summary.Same += count
		}
	}
	return summary
}
