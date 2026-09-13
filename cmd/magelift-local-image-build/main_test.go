package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

type dependencyRunner struct {
	missing map[string]bool
}

func (r dependencyRunner) LookPath(command string) (string, error) {
	if r.missing[command] {
		return "", errors.New("not found")
	}
	return "/test/bin/" + command, nil
}

func (dependencyRunner) Run(context.Context, string, ...string) ([]byte, error) {
	return []byte("test-version\n"), nil
}

func TestLocalImageBuildPreflightRequiresDockerAndGit(t *testing.T) {
	if err := checkLocalImageBuildDependencies(context.Background(), dependencyRunner{}); err != nil {
		t.Fatal(err)
	}
	err := checkLocalImageBuildDependencies(context.Background(), dependencyRunner{missing: map[string]bool{"docker": true}})
	if err == nil || err.Error() != "local image build dependencies unavailable: docker: docker is not installed" {
		t.Fatalf("error = %v", err)
	}
}

func TestRunHelpDoesNotRequireBuildDependencies(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run(context.Background(), []string{"--help"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != localImageBuildUsage+"\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunRejectsInvalidArgumentCount(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(context.Background(), nil, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), localImageBuildUsage) {
		t.Fatalf("error = %v", err)
	}
}
