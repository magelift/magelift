package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestClientUsesStdinForRequestsAndDecodesResponse(t *testing.T) {
	client := NewClient("magelift-build", io.Discard)
	client.command = runnerHelperCommand(t, "prepare")

	response, err := client.RoundTrip(context.Background(), validPrepareRequest())
	if err != nil {
		t.Fatal(err)
	}
	if response.Prepare == nil || response.Prepare.PreparedArtifact != "dist/rootfs.tar" {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestClientRejectsMismatchedStageAndOversizedOutput(t *testing.T) {
	for _, test := range []struct {
		mode    string
		message string
	}{
		{mode: "wrong-stage", message: "response for"},
		{mode: "oversized", message: "exceeds"},
	} {
		t.Run(test.mode, func(t *testing.T) {
			client := NewClient("magelift-build", io.Discard)
			client.command = runnerHelperCommand(t, test.mode)
			_, err := client.RoundTrip(context.Background(), validPrepareRequest())
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func runnerHelperCommand(t *testing.T, mode string) clientCommandFactory {
	t.Helper()
	return func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestRunnerHelperProcess", "--", mode)
		cmd.Env = append(os.Environ(), "MAGELIFT_RUNNER_HELPER=1")
		return cmd
	}
}

func TestRunnerHelperProcess(t *testing.T) {
	if os.Getenv("MAGELIFT_RUNNER_HELPER") != "1" {
		return
	}
	request, err := DecodeRequest(os.Stdin)
	if err != nil {
		os.Exit(2)
	}
	mode := os.Args[len(os.Args)-1]
	if mode == "oversized" {
		_, _ = fmt.Fprint(os.Stdout, strings.Repeat("x", int(MaxResponseSize)+1))
		os.Exit(0)
	}
	stage := request.Stage
	if mode == "wrong-stage" {
		stage = StageFinalize
	}
	response := Response{ProtocolVersion: ProtocolVersion, Stage: stage}
	if stage == StagePrepare {
		response.Prepare = &PrepareResponse{
			PreparedArtifact: "dist/rootfs.tar", PHPVersion: "8.5.1",
			PHPExtensions: []string{"intl"}, EnabledModules: []string{"Magento_Catalog"},
			Checksums:                   []FileChecksum{{Path: "composer.lock", SHA256: strings.Repeat("a", 64)}},
			RequiredRuntimeCapabilities: []string{"database.mysql"},
		}
	} else {
		response.Finalize = &FinalizeResponse{ImageDigest: "sha256:" + strings.Repeat("b", 64), ManifestPath: "dist/manifest.json", ManifestSHA256: strings.Repeat("c", 64)}
	}
	encoded, err := EncodeResponse(response)
	if err != nil {
		os.Exit(3)
	}
	_, _ = os.Stdout.Write(encoded)
	os.Exit(0)
}
