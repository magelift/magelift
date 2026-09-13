package automation

import (
	"context"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optdestroy"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optpreview"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
)

type ownershipTestStack struct {
	tags       map[string]string
	resources  int
	operations []string
}

func (s *ownershipTestStack) Preview(context.Context, ...optpreview.Option) (auto.PreviewResult, error) {
	s.operations = append(s.operations, "preview")
	return auto.PreviewResult{}, nil
}

func (s *ownershipTestStack) Up(context.Context, ...optup.Option) (auto.UpResult, error) {
	s.operations = append(s.operations, "update")
	return auto.UpResult{}, nil
}

func (s *ownershipTestStack) Destroy(context.Context, ...optdestroy.Option) (auto.DestroyResult, error) {
	s.operations = append(s.operations, "destroy")
	return auto.DestroyResult{}, nil
}

func (s *ownershipTestStack) Outputs(context.Context) (auto.OutputMap, error) {
	return auto.OutputMap{}, nil
}

func (s *ownershipTestStack) ListTags(context.Context) (map[string]string, error) {
	tags := make(map[string]string, len(s.tags))
	for key, value := range s.tags {
		tags[key] = value
	}
	return tags, nil
}

func (s *ownershipTestStack) SetTag(_ context.Context, key, value string) error {
	if s.tags == nil {
		s.tags = make(map[string]string)
	}
	s.tags[key] = value
	s.operations = append(s.operations, "set-tag")
	return nil
}

func (s *ownershipTestStack) Info(context.Context) (auto.StackSummary, error) {
	resources := s.resources
	return auto.StackSummary{ResourceCount: &resources}, nil
}

func testPreviewMetadata(generation uint64) PreviewMetadata {
	return PreviewMetadata{
		Project:      "shop",
		Repository:   "acme/store",
		PullRequest:  42,
		Environment:  "pr-42-abc123",
		StackKey:     "preview-abc123-pr-42",
		Owner:        "magelift-preview-abc123-pr-42",
		Branch:       "feature/cart",
		CommitDigest: "abcdef1",
		Domain:       "pr-42.example.test",
		ExpiresAt:    "2030-01-01T00:00:00Z",
		Generation:   generation,
	}
}

func TestPulumiBackendClaimsEmptyStackBeforePreview(t *testing.T) {
	stack := &ownershipTestStack{}
	backend := newPulumiBackend(stack)
	metadata := testPreviewMetadata(1)

	if _, err := backend.Preview(context.Background(), Request{Preview: &metadata}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stack.operations, []string{"set-tag", "preview"}) {
		t.Fatalf("operations = %#v, want metadata persistence before preview", stack.operations)
	}
	current, err := backend.ReadPreviewOwnership(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if current == nil || !reflect.DeepEqual(*current, metadata) {
		t.Fatalf("persisted metadata = %#v, want %#v", current, metadata)
	}
}

func TestPulumiBackendRejectsPopulatedUnownedStack(t *testing.T) {
	stack := &ownershipTestStack{resources: 1}
	metadata := testPreviewMetadata(1)

	_, err := newPulumiBackend(stack).Update(context.Background(), Request{Preview: &metadata}, io.Discard)
	if !errors.Is(err, ErrPreviewUnownedStack) {
		t.Fatalf("error = %v, want unowned-stack error", err)
	}
	if len(stack.operations) != 0 {
		t.Fatalf("provider operations = %#v, want none", stack.operations)
	}
}

func TestPulumiBackendRejectsForeignOwnerBeforeDestroy(t *testing.T) {
	stack := &ownershipTestStack{}
	current := testPreviewMetadata(1)
	backend := newPulumiBackend(stack)
	if err := backend.WritePreviewOwnership(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	requested := current
	requested.Repository = "other/store"

	_, err := backend.Destroy(context.Background(), Request{Preview: &requested, Destroy: true}, io.Discard)
	if !errors.Is(err, ErrPreviewOwnershipConflict) {
		t.Fatalf("error = %v, want ownership conflict", err)
	}
	if len(stack.operations) != 1 {
		t.Fatalf("provider operations = %#v, want only metadata setup", stack.operations)
	}
}

func TestPulumiBackendRejectsStaleCloseBeforeDestroy(t *testing.T) {
	stack := &ownershipTestStack{}
	current := testPreviewMetadata(2)
	backend := newPulumiBackend(stack)
	if err := backend.WritePreviewOwnership(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	requested := current
	requested.Generation = 1

	_, err := backend.Destroy(context.Background(), Request{Preview: &requested, Destroy: true}, io.Discard)
	if !errors.Is(err, ErrPreviewOwnershipConflict) {
		t.Fatalf("error = %v, want stale-generation conflict", err)
	}
	var ownershipErr *PreviewOwnershipError
	if !errors.As(err, &ownershipErr) || ownershipErr.Current == nil || ownershipErr.Current.Generation != 2 {
		t.Fatalf("error = %v, want current generation diagnostics", err)
	}
	if len(stack.operations) != 1 {
		t.Fatalf("provider operations = %#v, want only metadata setup", stack.operations)
	}
}

func TestPulumiBackendAllowsNewerCloseGenerationForSameOwner(t *testing.T) {
	stack := &ownershipTestStack{}
	current := testPreviewMetadata(2)
	backend := newPulumiBackend(stack)
	if err := backend.WritePreviewOwnership(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	requested := current
	requested.Generation = 3

	if _, err := backend.Destroy(context.Background(), Request{Preview: &requested, Destroy: true}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stack.operations, []string{"set-tag", "destroy"}) {
		t.Fatalf("operations = %#v, want destroy after same-owner generation check", stack.operations)
	}
}

func TestPulumiBackendRedeployAdvancesGeneration(t *testing.T) {
	stack := &ownershipTestStack{}
	backend := newPulumiBackend(stack)
	first := testPreviewMetadata(1)
	if err := backend.WritePreviewOwnership(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.Branch = "renamed/cart"
	second.CommitDigest = "abcdef2"
	second.Generation = 2

	if _, err := backend.Update(context.Background(), Request{Preview: &second}, io.Discard); err != nil {
		t.Fatal(err)
	}
	current, err := backend.ReadPreviewOwnership(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if current == nil || !reflect.DeepEqual(*current, second) {
		t.Fatalf("persisted metadata = %#v, want %#v", current, second)
	}
	if !reflect.DeepEqual(stack.operations, []string{"set-tag", "set-tag", "update"}) {
		t.Fatalf("operations = %#v, want generation persistence before update", stack.operations)
	}
}

func TestRunnerPreservesPreviewOwnershipConflict(t *testing.T) {
	stack := &ownershipTestStack{}
	backend := newPulumiBackend(stack)
	current := testPreviewMetadata(2)
	if err := backend.WritePreviewOwnership(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	requested := current
	requested.Generation = 1
	_, err := NewRunner(backend, io.Discard).Destroy(context.Background(), Request{Target: validRequest.Target, Preview: &requested, Destroy: true})
	if !errors.Is(err, ErrDestroyFailed) || !errors.Is(err, ErrPreviewOwnershipConflict) {
		t.Fatalf("error = %v, want operation and ownership errors", err)
	}
}

func TestRunnerRequiresOwnershipGuardForPreviewRequests(t *testing.T) {
	metadata := testPreviewMetadata(1)
	_, err := NewRunner(&mockBackend{}, io.Discard).Preview(context.Background(), Request{
		Target:  validRequest.Target,
		Preview: &metadata,
	})
	if !errors.Is(err, ErrPreviewGuardRequired) {
		t.Fatalf("error = %v, want ownership guard error", err)
	}
}
