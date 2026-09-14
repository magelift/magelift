package providerhost

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/automation"
	"github.com/magelift/magelift/internal/providerhost/hostproto"
	sdk "github.com/magelift/magelift/sdk/v1"
)

func TestExecuteRequestValidate(t *testing.T) {
	validPreview := &automation.PreviewMetadata{
		Project:     "shop",
		Repository:  "example/shop",
		PullRequest: 7,
		Environment: "preview-7",
		StackKey:    "shop-preview-7",
		Owner:       "team",
		Generation:  3,
	}
	cases := []struct {
		name    string
		request ExecuteRequest
		wantErr string
	}{
		{
			name:    "valid preview",
			request: ExecuteRequest{Operation: ExecutePreview, Plan: sdk.ModulePlan{StackName: "shop-preview"}},
		},
		{
			name: "valid with preview metadata",
			request: ExecuteRequest{
				Operation: ExecuteUp,
				Plan:      sdk.ModulePlan{StackName: "shop-preview"},
				Preview:   validPreview,
			},
		},
		{
			name:    "valid redacted outputs",
			request: ExecuteRequest{Operation: ExecuteRedactedOutputs, Plan: sdk.ModulePlan{StackName: "shop-preview"}},
		},
		{
			name:    "unknown operation",
			request: ExecuteRequest{Operation: "launch", Plan: sdk.ModulePlan{StackName: "shop-preview"}},
			wantErr: `execute operation "launch" is not supported`,
		},
		{
			name:    "missing stack name",
			request: ExecuteRequest{Operation: ExecuteDestroy},
			wantErr: "execute plan stack name is required",
		},
		{
			name: "invalid preview metadata",
			request: ExecuteRequest{
				Operation: ExecutePreview,
				Plan:      sdk.ModulePlan{StackName: "shop-preview"},
				Preview:   &automation.PreviewMetadata{},
			},
			wantErr: "execute preview metadata",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.request.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate() = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestOwnershipConflictRoundTrip(t *testing.T) {
	requested := automation.PreviewMetadata{
		Project:     "shop",
		Repository:  "example/shop",
		PullRequest: 7,
		Environment: "preview-7",
		StackKey:    "shop-preview-7",
		Owner:       "team",
		Generation:  2,
	}
	original := &automation.PreviewOwnershipError{
		Cause:     automation.ErrPreviewOwnershipConflict,
		Operation: "preview",
		Reason:    "stale generation",
		Requested: requested,
		Current:   &automation.PreviewMetadata{Generation: 3},
	}
	wire := OwnershipConflictFromError(original)
	if wire == nil {
		t.Fatal("OwnershipConflictFromError() = nil")
	}
	if wire.Operation != "preview" || wire.Reason != "stale generation" || wire.CurrentGeneration != 3 || !wire.HasCurrent {
		t.Fatalf("wire = %+v", wire)
	}
	rebuilt := wire.ToOwnershipError(requested)
	var ownershipErr *automation.PreviewOwnershipError
	if !errors.As(rebuilt, &ownershipErr) {
		t.Fatalf("rebuilt = %T %v, want *PreviewOwnershipError", rebuilt, rebuilt)
	}
	if ownershipErr.Operation != "preview" || ownershipErr.Current == nil || ownershipErr.Current.Generation != 3 {
		t.Fatalf("rebuilt = %+v", ownershipErr)
	}
	if !errors.Is(rebuilt, automation.ErrPreviewOwnershipConflict) {
		t.Fatalf("rebuilt %v does not unwrap to conflict", rebuilt)
	}
	if OwnershipConflictFromError(errors.New("boom")) != nil {
		t.Fatal("plain error should not convert")
	}
	var nilConflict *OwnershipConflict
	if nilConflict.ToOwnershipError(requested) != nil {
		t.Fatal("nil conflict should rebuild to nil")
	}
}

func TestTruncateDiagnostics(t *testing.T) {
	short := []string{"a", "b"}
	kept, truncated := TruncateDiagnostics(short)
	if truncated || len(kept) != 2 {
		t.Fatalf("short diagnostics truncated: %v %v", kept, truncated)
	}
	long := make([]string, 0, maxDiagnostics+10)
	for i := 0; i < maxDiagnostics+10; i++ {
		long = append(long, "line")
	}
	kept, truncated = TruncateDiagnostics(long)
	if !truncated || len(kept) != maxDiagnostics {
		t.Fatalf("long diagnostics kept %d, truncated %v", len(kept), truncated)
	}
}

type fakeExecuteAPI struct {
	execute func(ExecuteRequest) (ExecuteResult, error)
	calls   int
	last    ExecuteRequest
}

func (f *fakeExecuteAPI) Ping(context.Context) (string, error) {
	return SDKAPIVersion, nil
}

func (f *fakeExecuteAPI) Describe(context.Context) (Identity, error) {
	return Identity{APIVersion: SDKAPIVersion}, nil
}

func (f *fakeExecuteAPI) Plan(_ context.Context, _ sdk.ModulePlanRequest) (sdk.ModulePlan, error) {
	return sdk.ModulePlan{}, errors.New("plan not implemented")
}

func (f *fakeExecuteAPI) Program(_ context.Context, _ sdk.ModulePlan) (ProgramResult, error) {
	return ProgramResult{}, errors.New("program not implemented")
}

func (f *fakeExecuteAPI) Execute(_ context.Context, request ExecuteRequest) (ExecuteResult, error) {
	f.calls++
	f.last = request
	if f.execute == nil {
		return ExecuteResult{Operation: request.Operation}, nil
	}
	return f.execute(request)
}

func TestExecuteServerRoundTrip(t *testing.T) {
	fake := &fakeExecuteAPI{}
	server := &grpcProvider{Impl: fake}
	resp, err := server.Execute(context.Background(), &hostproto.ExecuteRequest{
		ExecuteJson: `{"operation":"preview","plan":{"StackName":"shop-preview"}}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if fake.calls != 1 || fake.last.Operation != ExecutePreview || fake.last.Plan.StackName != "shop-preview" {
		t.Fatalf("calls=%d last=%+v", fake.calls, fake.last)
	}
	if !strings.Contains(resp.GetResultJson(), `"operation":"preview"`) {
		t.Fatalf("result = %s", resp.GetResultJson())
	}
}

func TestExecuteServerRefusesOversizedRequest(t *testing.T) {
	fake := &fakeExecuteAPI{}
	server := &grpcProvider{Impl: fake}
	huge := strings.Repeat("x", executeRequestLimit+1)
	_, err := server.Execute(context.Background(), &hostproto.ExecuteRequest{ExecuteJson: huge})
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("err = %v, want size refusal", err)
	}
	if fake.calls != 0 {
		t.Fatal("oversized request reached the implementation")
	}
}

func TestExecuteServerValidatesOperation(t *testing.T) {
	fake := &fakeExecuteAPI{}
	server := &grpcProvider{Impl: fake}
	_, err := server.Execute(context.Background(), &hostproto.ExecuteRequest{
		ExecuteJson: `{"operation":"launch","plan":{"StackName":"shop-preview"}}`,
	})
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("err = %v, want operation refusal", err)
	}
	if fake.calls != 0 {
		t.Fatal("invalid operation reached the implementation")
	}
}
