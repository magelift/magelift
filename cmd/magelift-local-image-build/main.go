package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	buildkit "github.com/magelift/magelift/internal/build/kit"
	buildpipeline "github.com/magelift/magelift/internal/build/pipeline"
	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/source"
	"github.com/magelift/magelift/internal/toolchain"
)

const localImageBuildUsage = "usage: magelift-local-image-build SOURCE CONFIG IMAGE BUILDER_IMAGE_ID RUNTIME_IMAGE RUNTIME_IMAGE_ID"

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		_, err := fmt.Fprintln(stdout, localImageBuildUsage)
		return err
	}
	if len(args) != 6 {
		return fmt.Errorf("%s", localImageBuildUsage)
	}

	if err := checkLocalImageBuildDependencies(ctx, toolchain.SystemDependencyRunner()); err != nil {
		return err
	}
	sourceRoot := filepath.Clean(args[0])
	configBytes, err := os.ReadFile(filepath.Clean(args[1]))
	if err != nil {
		return err
	}
	file, err := config.Load(configBytes)
	if err != nil {
		return err
	}
	repository, err := (source.GitInspector{}).Inspect(ctx, sourceRoot)
	if err != nil {
		return err
	}
	artifactDirectory, err := os.MkdirTemp("", ".magelift-image-artifacts-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(artifactDirectory)
	tempRoot, err := os.MkdirTemp("", ".magelift-image-build-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempRoot)

	pipeline := buildpipeline.New(
		buildpipeline.ContainerAdapter{TempRoot: tempRoot, Stderr: stderr, Network: "bridge"},
		buildkit.NewRunner(stdout),
	)
	result, err := pipeline.Run(ctx, buildpipeline.Request{
		Repository:        repository,
		Config:            file,
		BuilderImage:      args[3],
		RuntimeImage:      args[4],
		RuntimeImageID:    args[5],
		ImageReference:    args[2],
		Platform:          "linux/amd64",
		Output:            buildkit.OutputLoad,
		MageLiftVersion:   "1.0.0-rc1",
		ArtifactDirectory: artifactDirectory,
	})
	if err != nil {
		return fmt.Errorf("build image: %w", err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, string(encoded))
	return err
}

func checkLocalImageBuildDependencies(ctx context.Context, runner toolchain.DependencyRunner) error {
	report := toolchain.CheckDependencies(ctx, runner, toolchain.SpecsForLocalImageBuild())
	missing := make([]string, 0)
	for _, check := range report.Checks {
		if check.Requirement != toolchain.DependencyRequired || check.Status == toolchain.DependencyAvailable {
			continue
		}
		missing = append(missing, check.ID+": "+check.Message)
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("local image build dependencies unavailable: %s", strings.Join(missing, "; "))
}
