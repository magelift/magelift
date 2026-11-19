package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/magelift/magelift/internal/automation"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/sdk"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func openStack(ctx context.Context, plan sdk.StoredPlan, m platform.StackModule, p platform.PlannedStack, selectOnly bool) (*auto.Stack, error) {
	backend := strings.TrimSpace(os.Getenv("PULUMI_BACKEND_URL"))
	if backend == "" {
		backend = plan.StateBackendURL
	}
	if selectOnly {
		// Reads and destroys use the existing snapshot and cannot create an empty stack.
		selected, err := auto.SelectStackInlineSource(ctx, p.StackName(), "magelift", func(*pulumi.Context) error { return nil }, auto.EnvVars(map[string]string{"PULUMI_BACKEND_URL": backend}))
		if err != nil {
			return nil, err
		}
		return &selected, nil
	}
	program, err := m.Program(p)
	if err != nil {
		return nil, err
	}
	return automation.NewInlineStackWithBackend(ctx, p.StackName(), program, backend)
}

func (s *Server) Preview(req *sdk.StackCall, resp *sdk.LifecycleResult) error {
	return s.mutate(sdk.OpPreview, req, resp)
}
func (s *Server) Apply(req *sdk.StackCall, resp *sdk.LifecycleResult) error {
	return s.mutate(sdk.OpApply, req, resp)
}
func (s *Server) Destroy(req *sdk.StackCall, resp *sdk.LifecycleResult) error {
	return s.mutate(sdk.OpDestroy, req, resp)
}

func (s *Server) mutate(op sdk.Operation, req *sdk.StackCall, resp *sdk.LifecycleResult) error {
	return respond(req.ProtocolVersion, &resp.Error, 2*time.Hour, func(ctx context.Context) error {
		b, m, p, err := checked(req.Envelope, req.Plan)
		if err != nil {
			return err
		}
		if b.AccountOnly || b.ObserveOnly || op == sdk.OpDestroy && (!b.DestroyOnly || p.Protected()) || op == sdk.OpApply && b.DestroyOnly {
			return invalid(errors.New("AWS lifecycle operation does not match the plan or the environment is protected"))
		}
		if !b.DestroyOnly && !b.Admitted {
			return invalid(errors.New("AWS infrastructure changes require account and catalog admission"))
		}
		request := automation.Request{Target: m.Descriptor(), Destroy: b.DestroyOnly}
		if len(req.PreviewMetadataJSON) > 0 {
			var metadata automation.PreviewMetadata
			if err := json.Unmarshal(req.PreviewMetadataJSON, &metadata); err != nil {
				return invalid(err)
			}
			if err := metadata.Validate(); err != nil {
				return invalid(err)
			}
			request.Preview = &metadata
		}
		st, err := openStack(ctx, req.Plan, m, p, b.DestroyOnly)
		if err != nil {
			return err
		}
		backend := automation.NewPulumiBackend(st)
		var progress automation.Diagnostics
		defer func() { resp.Diagnostics = progress.Lines() }()
		var counts map[string]int
		switch op {
		case sdk.OpPreview:
			counts, err = backend.Preview(ctx, request, &progress)
		case sdk.OpApply:
			counts, err = backend.Update(ctx, request, &progress)
		case sdk.OpDestroy:
			counts, err = backend.Destroy(ctx, request, &progress)
		}
		if err != nil {
			return err
		}
		resp.Summary = sdk.ChangeSummary{Create: counts["create"], Update: counts["update"], Delete: counts["delete"], Replace: counts["replace"], Same: counts["same"]}
		return nil
	})
}

func (s *Server) Outputs(req *sdk.StackCall, resp *sdk.OutputsResult) error {
	return withPlan(req.ProtocolVersion, &resp.Error, req.Envelope, req.Plan, func(ctx context.Context, m platform.StackModule, p platform.PlannedStack) error {
		st, err := openStack(ctx, req.Plan, m, p, true)
		if err != nil {
			// An initial deployment has no outputs yet. Keep reads side-effect free.
			if auto.IsSelectStack404Error(err) {
				return encode(&resp.ValuesJSON, map[string]any{})
			}
			return err
		}
		outputs, err := st.Outputs(ctx)
		if err != nil {
			return err
		}
		values := make(map[string]any, len(outputs))
		for name, out := range outputs {
			values[name] = out.Value
			if out.Secret {
				resp.SecretKeys = append(resp.SecretKeys, name)
			}
		}
		slices.Sort(resp.SecretKeys)
		return encode(&resp.ValuesJSON, values)
	})
}
