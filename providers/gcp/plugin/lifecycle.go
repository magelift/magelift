package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"gopkg.in/yaml.v3"

	"github.com/magelift/magelift/internal/automation"
	"github.com/magelift/magelift/internal/secretsafe"
	gcpbootstrap "github.com/magelift/magelift/providers/gcp/bootstrap"
	"github.com/magelift/magelift/providers/gcp/naming"
	gcpops "github.com/magelift/magelift/providers/gcp/ops"
	gcpresilience "github.com/magelift/magelift/providers/gcp/resilience"
	providerschema "github.com/magelift/magelift/providers/gcp/schema"
	gcpstack "github.com/magelift/magelift/providers/gcp/stack"
	"github.com/magelift/magelift/sdk"
)

// Automation abstracts the Pulumi stack for apply/destroy/outputs. Update
// and Destroy route through the shared backend (preview ownership
// included); Outputs returns the rich output map so the server can report
// which keys are secrets.
type Automation interface {
	Update(context.Context, automation.Request, io.Writer) (map[string]int, error)
	Destroy(context.Context, automation.Request, io.Writer) (map[string]int, error)
	Outputs(context.Context) (auto.OutputMap, error)
}

type pulumiAutomation struct {
	backend *automation.PulumiBackend
	stack   *auto.Stack
}

func (a pulumiAutomation) Update(ctx context.Context, request automation.Request, diagnostics io.Writer) (map[string]int, error) {
	return a.backend.Update(ctx, request, diagnostics)
}

func (a pulumiAutomation) Destroy(ctx context.Context, request automation.Request, diagnostics io.Writer) (map[string]int, error) {
	return a.backend.Destroy(ctx, request, diagnostics)
}

func (a pulumiAutomation) Outputs(ctx context.Context) (auto.OutputMap, error) {
	return a.stack.Outputs(ctx)
}

func defaultNewStack(ctx context.Context, stackName string, spec gcpstack.Spec, backendURL string) (Automation, error) {
	stack, err := automation.NewInlineStackWithBackend(ctx, stackName, gcpstack.Program(spec), backendURL)
	if err != nil {
		return nil, err
	}
	return pulumiAutomation{backend: automation.NewPulumiBackend(stack), stack: stack}, nil
}

func (s *Server) newStack(ctx context.Context, stackName string, spec gcpstack.Spec, backendURL string) (Automation, error) {
	if s != nil && s.NewStack != nil {
		return s.NewStack(ctx, stackName, spec, backendURL)
	}
	return defaultNewStack(ctx, stackName, spec, backendURL)
}

// Describe identifies the plugin and negotiates operation versions.
func (s *Server) Describe(_ context.Context, req *sdk.DescribeRequest) (*sdk.DescribeResponse, *sdk.OperationError) {
	if req == nil {
		return nil, InvalidError("describe request is required")
	}
	operations := make([]sdk.OperationVersion, 0, len(sdk.PluginMethods))
	for operation := range sdk.PluginMethods {
		operations = append(operations, sdk.OperationVersion{Name: string(operation), Version: OperationVersion})
	}
	version := ""
	if s != nil {
		version = s.Version
	}
	return &sdk.DescribeResponse{
		ProtocolVersion: sdk.ProtocolV1,
		ProviderID:      "gcp",
		ProviderVersion: version,
		Operations:      operations,
		Runtimes:        append([]string(nil), Runtimes...),
	}, nil
}

// ValidateConfig validates a raw target block over the provider schema.
func (s *Server) ValidateConfig(_ context.Context, req *sdk.ValidateConfigRequest) (*sdk.ValidateConfigResult, *sdk.OperationError) {
	_ = s
	if req == nil || len(bytes.TrimSpace(req.TargetBlock)) == 0 {
		return nil, InvalidError("target block is required")
	}
	var target providerschema.GCPTarget
	if err := yaml.Unmarshal(req.TargetBlock, &target); err != nil {
		return nil, InvalidError("target block is not valid YAML: " + err.Error())
	}
	problems := providerschema.Validate(&target, req.Runtime)
	return &sdk.ValidateConfigResult{Valid: len(problems) == 0, Problems: problems}, nil
}

// Plan resolves inputs into a stored plan. Admission runs before anything
// is stored: a plan that fails admission is never persisted.
func (s *Server) Plan(ctx context.Context, req *sdk.PlanRequest) (*sdk.PlanResult, *sdk.OperationError) {
	if req == nil || len(bytes.TrimSpace(req.TargetBlock)) == 0 {
		return nil, InvalidError("target block is required")
	}
	var target providerschema.GCPTarget
	if err := yaml.Unmarshal(req.TargetBlock, &target); err != nil {
		return nil, InvalidError("target block is not valid YAML: " + err.Error())
	}
	if problems := providerschema.Validate(&target, req.Runtime); len(problems) > 0 {
		return nil, InvalidError("target block is invalid: " + strings.Join(problems, "; "))
	}
	spec, err := gcpstack.PlanFromInputs(gcpstack.PlanInputs{
		Envelope:            req.Envelope,
		Application:         req.Application,
		Target:              target,
		Edge:                req.Edge,
		Observability:       req.Observability,
		Runtime:             sdk.RuntimeID(req.Runtime),
		LiveQueueReplicas:   req.LiveQueueReplicas,
		ValkeyRequirement:   req.ValkeyRequirement,
		AllowUnsupported:    req.AllowUnsupported,
		AllowExpiredPreview: req.AllowExpiredPreview,
	})
	if err != nil {
		return nil, InvalidError(err.Error())
	}
	admitted, err := s.admission().AdmitSpec(ctx, spec)
	if err != nil {
		return nil, mapError(err)
	}
	inputs, err := gcpops.BuildDeployInputs(admitted)
	if err != nil {
		return nil, &sdk.OperationError{Code: sdk.ErrCodeInternal, Message: err.Error()}
	}
	backendURL, err := gcpbootstrap.StateBackendURL(gcpbootstrap.Spec{
		Project: admitted.Identity.Project, Environment: admitted.Identity.Environment,
		GCPProject: admitted.Identity.GCPProject, Region: admitted.Identity.Region,
	})
	if err != nil {
		return nil, &sdk.OperationError{Code: sdk.ErrCodeInternal, Message: err.Error()}
	}
	opaque, operr := marshalJSON(admitted)
	if operr != nil {
		return nil, operr
	}
	planned := gcpstack.SpecPlanned{Spec: admitted}
	return &sdk.PlanResult{Plan: sdk.StoredPlan{
		StackName:        planned.StackName(),
		Provider:         string(planned.Provider()),
		Runtime:          string(planned.Runtime()),
		ImageDigest:      admitted.Artifact.ImageDigest,
		StateBackendURL:  backendURL,
		DeployInputsJSON: inputs,
		Opaque:           opaque,
	}}, nil
}

// Apply runs the Pulumi update for a stored plan.
func (s *Server) Apply(ctx context.Context, req *sdk.StackCall) (*sdk.LifecycleResult, *sdk.OperationError) {
	return s.runStackMutation(ctx, req, false)
}

// Destroy runs the Pulumi destroy for a stored plan.
func (s *Server) Destroy(ctx context.Context, req *sdk.StackCall) (*sdk.LifecycleResult, *sdk.OperationError) {
	return s.runStackMutation(ctx, req, true)
}

func (s *Server) runStackMutation(ctx context.Context, req *sdk.StackCall, destroying bool) (*sdk.LifecycleResult, *sdk.OperationError) {
	if req == nil {
		return nil, InvalidError("stack call is required")
	}
	spec, operr := loadSpec(req.Plan.Opaque)
	if operr != nil {
		return nil, operr
	}
	if operr := checkEnvelope(req.Envelope, req.Plan, spec); operr != nil {
		return nil, operr
	}
	metadata, operr := previewMetadata(req.PreviewMetadataJSON)
	if operr != nil {
		return nil, operr
	}
	backend, err := s.newStack(ctx, req.Plan.StackName, spec, backendURLFor(req.Plan, req.Envelope))
	if err != nil {
		return nil, mapError(err)
	}
	planned := gcpstack.SpecPlanned{Spec: spec}
	request := automation.Request{Target: planned.TargetDescriptor(), Preview: metadata, Destroy: destroying}
	var diagnostics cappedBuffer
	var changes map[string]int
	if destroying {
		changes, err = backend.Destroy(ctx, request, &diagnostics)
	} else {
		changes, err = backend.Update(ctx, request, &diagnostics)
	}
	if err != nil {
		return nil, withDiagnostics(mapError(err), diagnostics.lines())
	}
	summary := summarizeChanges(changes)
	result := &sdk.LifecycleResult{Summary: summary, Diagnostics: diagnostics.lines()}
	return result, nil
}

// withDiagnostics appends the redacted tail of captured progress to a typed
// error so failed mutations stay debuggable without a second call.
func withDiagnostics(operr *sdk.OperationError, lines []string) *sdk.OperationError {
	if operr == nil || len(lines) == 0 {
		return operr
	}
	tail := lines
	if len(tail) > 5 {
		tail = tail[len(tail)-5:]
	}
	redacted, _ := secretsafe.RedactSensitiveText(strings.Join(tail, "\n"))
	if strings.TrimSpace(redacted) == "" {
		return operr
	}
	operr.Message += "\nlast output:\n" + redacted
	return operr
}

// Outputs reads the stored stack outputs with secret-key flags.
func (s *Server) Outputs(ctx context.Context, req *sdk.StackCall) (*sdk.OutputsResult, *sdk.OperationError) {
	if req == nil {
		return nil, InvalidError("stack call is required")
	}
	spec, operr := loadSpec(req.Plan.Opaque)
	if operr != nil {
		return nil, operr
	}
	if operr := checkEnvelope(req.Envelope, req.Plan, spec); operr != nil {
		return nil, operr
	}
	backend, err := s.newStack(ctx, req.Plan.StackName, spec, backendURLFor(req.Plan, req.Envelope))
	if err != nil {
		return nil, mapError(err)
	}
	outputs, err := backend.Outputs(ctx)
	if err != nil {
		return nil, mapError(err)
	}
	values := make(map[string]any, len(outputs))
	var secrets []string
	for name, output := range outputs {
		values[name] = output.Value
		if output.Secret {
			secrets = append(secrets, name)
		}
	}
	encoded, operr := marshalJSON(values)
	if operr != nil {
		return nil, operr
	}
	return &sdk.OutputsResult{ValuesJSON: encoded, SecretKeys: secrets}, nil
}

// DestroyLeftoverBackups deletes Cloud SQL backups left after teardown.
func (s *Server) DestroyLeftoverBackups(ctx context.Context, req *sdk.DestroyLeftoverBackupsCall) (*sdk.DestroyLeftoverBackupsResult, *sdk.OperationError) {
	if req == nil {
		return nil, InvalidError("destroy leftover backups call is required")
	}
	spec, operr := loadSpec(req.Plan.Opaque)
	if operr != nil {
		return nil, operr
	}
	if operr := checkEnvelope(req.Envelope, req.Plan, spec); operr != nil {
		return nil, operr
	}
	project := strings.TrimSpace(spec.Identity.GCPProject)
	instance := naming.CloudSQLInstance(spec.Identity.Project, spec.Identity.Environment)
	if project == "" || instance == "" {
		return nil, InvalidError("stored plan lacks the GCP project or instance name")
	}
	sqlClient, err := s.cleanupSQL(ctx)
	if err != nil {
		return nil, mapError(err)
	}
	api, err := gcpresilience.NewNativeAPIWithSQLAndProjectionAndPubSub(nil, nil, sqlClient, nil, gcpresilience.NativeAPIConfig{Project: project}, nil)
	if err != nil {
		return nil, mapError(err)
	}
	destroyed, err := api.DeleteLeftoverCloudSQLBackupsForInstance(ctx, instance)
	if err != nil {
		return nil, mapError(err)
	}
	return &sdk.DestroyLeftoverBackupsResult{Destroyed: destroyed}, nil
}

func (s *Server) cleanupSQL(ctx context.Context) (gcpresilience.CloudSQLAPI, error) {
	if s != nil && s.NewCleanupSQL != nil {
		return s.NewCleanupSQL(ctx)
	}
	return gcpresilience.NewCloudSQLAPI(ctx)
}

func backendURLFor(plan sdk.StoredPlan, envelope sdk.Envelope) string {
	if plan.StateBackendURL != "" {
		return plan.StateBackendURL
	}
	return envelope.StateBackendURL
}

func previewMetadata(data []byte) (*automation.PreviewMetadata, *sdk.OperationError) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil
	}
	var metadata automation.PreviewMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, InvalidError("malformed preview metadata: " + err.Error())
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
		case "same", "unchanged":
			summary.Same += count
		case "read", "import":
			summary.Same += count
		}
	}
	return summary
}

// cappedBuffer keeps the tail of verbose progress streams so diagnostics
// stay bounded on the wire.
type cappedBuffer struct {
	kept []string
}

func (b *cappedBuffer) Write(data []byte) (int, error) {
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		b.kept = append(b.kept, line)
	}
	const maxLines = 50
	if len(b.kept) > maxLines {
		b.kept = append([]string(nil), b.kept[len(b.kept)-maxLines:]...)
	}
	return len(data), nil
}

func (b *cappedBuffer) lines() []string {
	return append([]string(nil), b.kept...)
}
