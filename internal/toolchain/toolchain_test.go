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
	runtimeReference := "magelift/php-runtime@" + idB
	command := &fakeCommand{results: []commandResult{{output: idA + "\n"}, {output: idB + "\r\n"}, {output: runtimeReference + "\n"}}}

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
		{"docker", "image", "inspect", "--format={{index .RepoDigests 0}}", DefaultRuntimeTag},
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

func TestResolveValidatesTagsBeforeExecutingDocker(t *testing.T) {
	command := &fakeCommand{}
	_, err := NewResolver(command, "invalid tag", DefaultRuntimeTag).Resolve(context.Background())
	if err == nil || !strings.Contains(err.Error(), "invalid builder image tag") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(command.calls) != 0 {
		t.Fatal("Docker was invoked for an invalid tag")
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
	result := command.results[0]
	command.results = command.results[1:]
	return []byte(result.output), result.err
}
