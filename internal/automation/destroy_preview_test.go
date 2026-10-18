package automation

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"testing"
)

func TestPulumiBackendDestroyPreviewUsesPreviewDestroy(t *testing.T) {
	stack := &ownershipTestStack{}
	backend := newPulumiBackend(stack)
	metadata := testPreviewMetadata(1)
	if err := backend.WritePreviewOwnership(context.Background(), metadata); err != nil {
		t.Fatal(err)
	}
	stack.operations = nil

	var diagnostics bytes.Buffer
	changes, err := backend.Preview(context.Background(), Request{Preview: &metadata, Destroy: true}, &diagnostics)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stack.operations, []string{"preview-destroy"}) {
		t.Fatalf("operations = %#v, want only PreviewDestroy", stack.operations)
	}
	if changes["delete"] != 2 {
		t.Fatalf("changes = %#v, want delete summary from PreviewDestroy", changes)
	}
	if !bytes.Contains(diagnostics.Bytes(), []byte("destroy-preview")) {
		t.Fatalf("diagnostics = %q, want the destroy preview progress stream", diagnostics.String())
	}
	if stack.lastDestroyPreview == nil || (stack.lastDestroyPreview.RunProgram == nil || *stack.lastDestroyPreview.RunProgram) || stack.lastDestroyPreview.Refresh {
		t.Fatalf("destroy preview options = %#v, want progress streams and an explicit RunProgram=false", stack.lastDestroyPreview)
	}
	if len(stack.lastDestroyPreview.ProgressStreams) != 1 || len(stack.lastDestroyPreview.ErrorProgressStreams) != 1 {
		t.Fatalf("destroy preview streams = progress %d error %d", len(stack.lastDestroyPreview.ProgressStreams), len(stack.lastDestroyPreview.ErrorProgressStreams))
	}
}

func TestPulumiBackendDestroyPreviewWithoutMetadataUsesPreviewDestroy(t *testing.T) {
	stack := &ownershipTestStack{}
	backend := newPulumiBackend(stack)

	if _, err := backend.Preview(context.Background(), Request{Destroy: true}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stack.operations, []string{"preview-destroy"}) {
		t.Fatalf("operations = %#v, want PreviewDestroy without an ownership write", stack.operations)
	}
}

func TestPulumiBackendDestroyPreviewRefusesOwnershipMismatchBeforeCalls(t *testing.T) {
	stack := &ownershipTestStack{}
	current := testPreviewMetadata(2)
	backend := newPulumiBackend(stack)
	if err := backend.WritePreviewOwnership(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	requested := current
	requested.Repository = "other/store"
	stack.operations = nil

	_, err := backend.Preview(context.Background(), Request{Preview: &requested, Destroy: true}, io.Discard)
	if !errors.Is(err, ErrPreviewOwnershipConflict) {
		t.Fatalf("error = %v, want ownership conflict", err)
	}
	if len(stack.operations) != 0 {
		t.Fatalf("operations = %#v, want no Preview, PreviewDestroy, Up, or Destroy", stack.operations)
	}
}

func TestPulumiBackendDestroyExplicitlyDisablesProgram(t *testing.T) {
	stack := &ownershipTestStack{}
	backend := newPulumiBackend(stack)
	if _, err := backend.Destroy(context.Background(), Request{Destroy: true}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stack.operations, []string{"destroy"}) {
		t.Fatalf("operations = %v", stack.operations)
	}
	if stack.lastDestroy == nil || stack.lastDestroy.RunProgram == nil || *stack.lastDestroy.RunProgram {
		t.Fatal("destroy must explicitly disable the program")
	}
}
