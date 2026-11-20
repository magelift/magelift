package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/config"
	deployflow "github.com/magelift/magelift/internal/deploy"
	"github.com/magelift/magelift/internal/platform"
)

func TestDeployFromBuildWritesDeploymentEndpoints(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	work := t.TempDir()
	manifest := filepath.Join(work, "manifest.json")
	report := filepath.Join(work, "endpoints.json")
	imageDigest := "sha256:" + strings.Repeat("a", 64)
	reference := "registry.example.test:5000/shops/demo@" + imageDigest
	if err := os.WriteFile(manifest, []byte(`{"imageDigest":"`+imageDigest+`","imageDigestReference":"`+reference+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(report, []byte("an older deployment"), 0o600); err != nil {
		t.Fatal(err)
	}

	backend := &fakeInfrastructureBackend{outputs: map[string]any{
		platform.OutputApplicationURL: "https://magento.example.test/shop/",
		"privateValue":                "test-secret",
	}}
	var out bytes.Buffer
	var request deployflow.Request
	var order []string
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.newBackend = func(context.Context, platform.PlannedStack, string) (infrastructureBackend, error) {
		return backend, nil
	}
	o.newDeploySteps = func(context.Context, infrastructureBackend, platform.PlannedStack, io.Writer) (deployflow.Steps, error) {
		return fakeDeploymentSteps{order: &order, request: &request}, nil
	}
	o.newLock = func(context.Context, platform.PlannedStack) (func(context.Context) error, error) {
		return func(context.Context) error { return nil }, nil
	}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "--output", "json", "deploy", "--from-build", manifest, "--outputs-file", report})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if request.ImageDigest != reference {
		t.Fatalf("deployed %q instead of manifest digest %q", request.ImageDigest, reference)
	}
	var result infrastructureResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Endpoints == nil || result.Endpoints.StorefrontURL != "https://magento.example.test/shop/" ||
		!strings.Contains(result.LogsCommand, "logs --env staging") || !strings.HasSuffix(result.LogsCommand, "--service web") {
		t.Fatalf("unexpected deployment summary: %s", &out)
	}
	data, err := os.ReadFile(report)
	if err != nil {
		t.Fatal(err)
	}
	var outputs map[string]string
	if err := json.Unmarshal(data, &outputs); err != nil {
		t.Fatal(err)
	}
	if len(outputs) != 4 || outputs["environment"] != "staging" ||
		outputs["graphqlUrl"] != "https://magento.example.test/shop/graphql" ||
		outputs["storefrontUrl"] != result.Endpoints.StorefrontURL ||
		outputs["magentoUrl"] != result.Endpoints.MagentoURL || strings.Contains(out.String(), "test-secret") {
		t.Fatalf("unexpected deployment outputs: %s", data)
	}
}

func TestDeployFromBuildRejectsLocalAndInconsistentManifests(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	for _, tc := range []struct {
		name, contents, message string
		args                    []string
	}{
		{"local", `{"imageDigest":"` + digest + `"}`, "no registry digest", nil},
		{"mismatch", `{"imageDigest":"sha256:inconsistent","imageDigestReference":"registry.example.test/shop@` + digest + `"}`, "differs from its registry digest", nil},
		{"conflict", "", "none of the others can be", []string{"--digest", "registry.example.test/shop@" + digest}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "manifest.json")
			if err := os.WriteFile(path, []byte(tc.contents), 0o600); err != nil {
				t.Fatal(err)
			}
			o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
			cmd := newCommandWithOptions(o)
			cmd.SetArgs(append([]string{"deploy", "--from-build", path}, tc.args...))
			if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("deploy error = %v, want %q", err, tc.message)
			}
		})
	}
}

func TestDeploymentLinksUseSeparateHeadlessOrigin(t *testing.T) {
	application := config.Application{Mode: "headless", Magento: config.MagentoRuntime{
		SecureBaseURL:    "https://magento.staging.example.test/",
		StorefrontOrigin: "https://store.staging.example.test",
	}}
	links := deploymentLinks(application, "http://load-balancer.example.test")
	if links.MagentoURL != "https://magento.staging.example.test/" ||
		links.GraphQLURL != "https://magento.staging.example.test/graphql" ||
		links.StorefrontURL != "https://store.staging.example.test" {
		t.Fatalf("headless endpoints = %#v", links)
	}
	application.Magento.SecureBaseURL = ""
	application.Magento.StorefrontOrigin = ""
	if links := deploymentLinks(application, "https://user:password@magento.example.test/graphql"); links != nil {
		t.Fatalf("credentialed URL was exported: %#v", links)
	}
}
