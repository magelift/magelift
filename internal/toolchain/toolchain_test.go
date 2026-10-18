package toolchain

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestResolveUsesDefaultTagsAndExactInspectArgv(t *testing.T) {
	idA := "sha256:" + strings.Repeat("a", 64)
	idB := "sha256:" + strings.Repeat("b", 64)
	runtimeReference := DefaultRuntimeTag
	command := &fakeCommand{results: []commandResult{{output: idA + "\n"}, {output: idB + "\r\n"}}}

	images, err := NewResolver(command, DefaultBuilderTag, DefaultRuntimeTag).Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if images.BuilderID != idA || images.RuntimeID != idB || images.RuntimeReference != runtimeReference {
		t.Fatalf("unexpected images: %#v", images)
	}
	want := [][]string{
		{"docker", "image", "inspect", "--format={{.Id}}", DefaultBuilderTag},
		{"docker", "image", "inspect", "--format={{.Id}}", DefaultRuntimeTag},
	}
	if !reflect.DeepEqual(command.calls, want) {
		t.Fatalf("calls = %#v, want %#v", command.calls, want)
	}
}

func TestResolveStopsWhenBuilderIsUnavailable(t *testing.T) {
	command := &fakeCommand{results: []commandResult{{err: errors.New("No such image")}}}

	_, err := NewResolver(command, DefaultBuilderTag, DefaultRuntimeTag).Resolve(context.Background())
	if !errors.Is(err, ErrImageUnavailable) || !strings.Contains(err.Error(), DefaultBuilderTag) || !strings.Contains(err.Error(), "build or load") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(command.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(command.calls))
	}
	for _, call := range command.calls {
		if call[0] == "docker" && len(call) > 1 && call[1] == "image" && len(call) > 2 && call[2] == "pull" {
			t.Fatalf("local tag resolution must not pull: %#v", command.calls)
		}
	}
}

func TestResolvePullsMissingImmutableReferences(t *testing.T) {
	builderRef := "registry.example.invalid/team/shop-builder@sha256:" + strings.Repeat("a", 64)
	runtimeRef := "registry.example.invalid/team/shop-runtime@sha256:" + strings.Repeat("b", 64)
	builderID := "sha256:" + strings.Repeat("a", 64)
	runtimeID := "sha256:" + strings.Repeat("b", 64)
	command := &fakeCommand{results: []commandResult{
		{err: errors.New("missing")},
		{output: ""},
		{output: builderID + "\n"},
		{err: errors.New("missing")},
		{output: ""},
		{output: runtimeID + "\n"},
	}}

	images, err := NewResolver(command, builderRef, runtimeRef).Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if images.BuilderReference != builderRef || images.RuntimeReference != runtimeRef {
		t.Fatalf("unexpected references: %#v", images)
	}
	if !reflect.DeepEqual(command.pulls(), []string{builderRef, runtimeRef}) {
		t.Fatalf("pulls = %#v", command.pulls())
	}
}

func TestResolveReusesCachedImmutableReferencesWithoutPull(t *testing.T) {
	builderRef := "registry.example.invalid/team/shop-builder@sha256:" + strings.Repeat("a", 64)
	runtimeRef := "registry.example.invalid/team/shop-runtime@sha256:" + strings.Repeat("b", 64)
	builderID := "sha256:" + strings.Repeat("a", 64)
	runtimeID := "sha256:" + strings.Repeat("b", 64)
	command := &fakeCommand{results: []commandResult{
		{output: builderID + "\n"},
		{output: builderID + "\n"},
		{output: runtimeID + "\n"},
		{output: runtimeID + "\n"},
	}}

	_, err := NewResolver(command, builderRef, runtimeRef).Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(command.pulls()) != 0 {
		t.Fatalf("unexpected pulls: %#v", command.pulls())
	}
}

func TestResolveRejectsAmbiguousOrInvalidIDs(t *testing.T) {
	valid := "sha256:" + strings.Repeat("b", 64)
	tests := map[string]string{
		"empty":         "",
		"uppercase":     "sha256:" + strings.Repeat("A", 64) + "\n",
		"leading space": " sha256:" + strings.Repeat("a", 64) + "\n",
		"multiple":      "sha256:" + strings.Repeat("a", 64) + "\n" + valid + "\n",
		"oversized":     strings.Repeat("x", maxImageIDBytes+1),
	}
	for name, output := range tests {
		t.Run(name, func(t *testing.T) {
			command := &fakeCommand{results: []commandResult{{output: output}}}
			_, err := NewResolver(command, DefaultBuilderTag, DefaultRuntimeTag).Resolve(context.Background())
			if err == nil || !strings.Contains(err.Error(), "Docker image ID") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestResolveValidatesReferencesBeforeExecutingDocker(t *testing.T) {
	command := &fakeCommand{}
	_, err := NewResolver(command, "invalid tag", DefaultRuntimeTag).Resolve(context.Background())
	if err == nil || !strings.Contains(err.Error(), "invalid builder image reference") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(command.calls) != 0 {
		t.Fatal("Docker was invoked for an invalid reference")
	}
}

type commandResult struct {
	output string
	err    error
}

type fakeCommand struct {
	results []commandResult
	calls   [][]string
}

func (command *fakeCommand) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	command.calls = append(command.calls, append([]string{name}, args...))
	if len(command.results) == 0 {
		return nil, errors.New("unexpected docker invocation")
	}
	result := command.results[0]
	command.results = command.results[1:]
	return []byte(result.output), result.err
}

func (command *fakeCommand) pulls() []string {
	pulls := make([]string, 0)
	for _, call := range command.calls {
		if len(call) >= 4 && call[0] == "docker" && call[1] == "image" && call[2] == "pull" {
			pulls = append(pulls, call[3])
		}
	}
	return pulls
}

func TestDefaultResolverUsesLocalTags(t *testing.T) {
	idA := "sha256:" + strings.Repeat("a", 64)
	idB := "sha256:" + strings.Repeat("b", 64)
	command := &fakeCommand{results: []commandResult{{output: idA + "\n"}, {output: idB + "\r\n"}}}

	images, err := DefaultResolver(command).Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if images.BuilderID != idA || images.RuntimeID != idB {
		t.Fatalf("unexpected images: %#v", images)
	}
}
