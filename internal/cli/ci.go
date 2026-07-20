package cli

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/acourtiol/magelift/internal/config"
	"github.com/spf13/cobra"
)

const checkoutAction = "actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0 # v7.0.0"
const setupGoAction = "actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0"
const setupBuildxAction = "docker/setup-buildx-action@bb05f3f5519dd87d3ba754cc423b652a5edd6d2c # v4.2.0"
const setupQemuAction = "docker/setup-qemu-action@96fe6ef7f33517b61c61be40b68a1882f3264fb8 # v4.2.0"
const dockerLoginAction = "docker/login-action@af1e73f918a031802d376d3c8bbc3fe56130a9b0 # v4.4.0"
const configureAWSAction = "aws-actions/configure-aws-credentials@517a711dbcd0e402f90c77e7e2f81e849156e31d # v6.2.2"
const cosignInstallerAction = "sigstore/cosign-installer@6f9f17788090df1f26f669e9d70d6ae9567deba6 # v4.1.2"

var releaseVersionPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)

type ciFlags struct {
	path    string
	version string
}

func ciCommand(o *options) *cobra.Command {
	flags := &ciFlags{version: Version}
	command := &cobra.Command{Use: "ci", Short: "Generate and validate GitHub Actions workflows"}
	generate := &cobra.Command{Use: "generate", Short: "Generate the GitHub Actions workflow", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
		workflow, path, err := expectedWorkflow(o, flags)
		if err != nil {
			return invalid(err)
		}
		changed, err := writeWorkflow(path, workflow)
		if err != nil {
			return err
		}
		return o.write(map[string]any{"changed": changed, "path": path, "sha256": fmt.Sprintf("%x", sha256.Sum256(workflow))})
	}}
	validate := &cobra.Command{Use: "validate", Short: "Validate the GitHub Actions workflow", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
		expected, path, err := expectedWorkflow(o, flags)
		if err != nil {
			return invalid(err)
		}
		actual, err := os.ReadFile(path)
		if err != nil {
			return invalid(fmt.Errorf("read generated workflow: %w", err))
		}
		if !bytes.Equal(actual, expected) {
			return invalid(errors.New("generated GitHub Actions workflow is out of date; run magelift ci generate"))
		}
		return o.write(map[string]any{"path": path, "sha256": fmt.Sprintf("%x", sha256.Sum256(expected)), "valid": true})
	}}
	for _, subcommand := range []*cobra.Command{generate, validate} {
		subcommand.Flags().StringVar(&flags.path, "path", "", "workflow output path")
		subcommand.Flags().StringVar(&flags.version, "magelift-version", flags.version, "MageLift release version installed by CI")
		command.AddCommand(subcommand)
	}
	return command
}

func expectedWorkflow(o *options, flags *ciFlags) ([]byte, string, error) {
	if filepath.IsAbs(o.configPath) {
		// The generated workflow runs from the repository root, which is the
		// directory containing the single project configuration file.
		o.configPath = filepath.Clean(o.configPath)
	}
	file, err := o.load()
	if err != nil {
		return nil, "", err
	}
	environments := file.Environments()
	if len(environments) == 0 {
		return nil, "", errors.New("at least one environment is required")
	}
	for _, environment := range environments {
		effective, err := file.Resolve(environment, config.ResolveOptions{})
		if err != nil {
			return nil, "", fmt.Errorf("environment %s: %w", environment, err)
		}
		if err := requireAWSCITarget(effective.Config); err != nil {
			return nil, "", fmt.Errorf("environment %s: %w", environment, err)
		}
	}
	if !releaseVersionPattern.MatchString(flags.version) {
		return nil, "", errors.New("ci generation requires an immutable MageLift release version such as v1.2.3")
	}
	projectRoot := filepath.Dir(filepath.Clean(o.configPath))
	path := flags.path
	if path == "" {
		path = filepath.Join(projectRoot, ".github", "workflows", "magelift.yml")
	} else if !filepath.IsAbs(path) {
		path = filepath.Join(projectRoot, path)
	}
	workflow := renderWorkflow(filepath.Base(o.configPath), flags.version, environments)
	return workflow, filepath.Clean(path), nil
}

func requireAWSCITarget(cfg config.Config) error {
	if cfg.Target.Provider != "aws" {
		return errors.New("ci generate currently supports AWS targets only")
	}
	runtime := cfg.Target.Runtime
	if runtime == "" {
		runtime = "ecs-fargate"
	}
	if runtime != "ecs-fargate" {
		return fmt.Errorf("ci generate does not support runtime %q yet", runtime)
	}
	return nil
}

func renderWorkflow(configFile, version string, environments []string) []byte {
	var workflow strings.Builder
	buildEnvironment := buildTargetEnvironment(environments)
	workflow.WriteString("name: MageLift CI\n\n")
	workflow.WriteString("on:\n  pull_request:\n    types: [opened, synchronize, reopened, labeled, closed]\n  push:\n    branches: [main]\n  schedule:\n    - cron: '17 * * * *'\n  workflow_dispatch:\n    inputs:\n      environment:\n        description: Environment to deploy\n        required: true\n        type: string\n\n")
	workflow.WriteString("permissions:\n  contents: read\n\n")
	workflow.WriteString("jobs:\n  validate:\n    name: Validate ${{ matrix.environment }}\n    runs-on: ubuntu-latest\n")
	workflow.WriteString("    strategy:\n      fail-fast: false\n      matrix:\n        environment:\n")
	for _, environment := range environments {
		workflow.WriteString("          - " + strconv.Quote(environment) + "\n")
	}
	workflow.WriteString("    steps:\n")
	workflow.WriteString("      - uses: " + checkoutAction + "\n")
	workflow.WriteString("      - uses: " + setupGoAction + "\n        with:\n          go-version: '1.26.5'\n          cache: false\n")
	workflow.WriteString("      - name: Install MageLift\n        run: go install github.com/acourtiol/magelift/cmd/magelift@" + version + "\n")
	workflow.WriteString("      - name: Validate configuration\n        run: magelift --config " + shellQuote(configFile) + " --no-interaction config validate\n")
	workflow.WriteString("      - name: Resolve environment\n        run: magelift --config " + shellQuote(configFile) + " --env \"${{ matrix.environment }}\" --no-interaction --output json config effective > /dev/null\n")
	workflow.WriteString("\n  build:\n    name: Build and sign immutable image\n    if: github.event_name == 'push' || github.event_name == 'workflow_dispatch'\n    needs: validate\n    runs-on: ubuntu-latest\n")
	if buildEnvironment != "" {
		workflow.WriteString("    environment: " + buildEnvironment + "\n")
	}
	workflow.WriteString("    permissions:\n      contents: read\n      packages: write\n      id-token: write\n      attestations: write\n    outputs:\n      digest: ${{ steps.build.outputs.digest }}\n    steps:\n")
	workflow.WriteString("      - uses: " + checkoutAction + "\n        with:\n          persist-credentials: false\n")
	workflow.WriteString("      - uses: " + setupGoAction + "\n        with:\n          go-version-file: go.mod\n          cache: false\n")
	workflow.WriteString("      - uses: " + setupQemuAction + "\n      - uses: " + setupBuildxAction + "\n      - uses: " + cosignInstallerAction + "\n")
	workflow.WriteString("      - uses: " + dockerLoginAction + "\n        with:\n          registry: ghcr.io\n          username: ${{ github.actor }}\n          password: ${{ secrets.GITHUB_TOKEN }}\n")
	workflow.WriteString("      - name: Configure AWS credentials\n        uses: " + configureAWSAction + "\n        with:\n          role-to-assume: ${{ vars.MAGELIFT_BUILD_ROLE_ARN }}\n          aws-region: ${{ vars.MAGELIFT_AWS_REGION }}\n          mask-aws-account-id: true\n")
	workflow.WriteString("      - name: Install MageLift\n        run: go install -ldflags=\"-X github.com/acourtiol/magelift/internal/cli.Version=" + version + "\" github.com/acourtiol/magelift/cmd/magelift@" + version + "\n")
	workflow.WriteString("      - name: Build and sign image\n        id: build\n        env:\n          MAGELIFT_RELEASE_IMAGE: ${{ vars.MAGELIFT_RELEASE_IMAGE }}\n          MAGELIFT_BUILDER_IMAGE: ${{ vars.MAGELIFT_BUILDER_IMAGE }}\n          MAGELIFT_RUNTIME_IMAGE: ${{ vars.MAGELIFT_RUNTIME_IMAGE }}\n        run: |\n          set -eu\n          for required in MAGELIFT_RELEASE_IMAGE MAGELIFT_BUILDER_IMAGE MAGELIFT_RUNTIME_IMAGE; do\n            test -n \"${!required}\"\n          done\n          magelift --config " + shellQuote(configFile) + " --no-interaction --output json build --push \\\n            --image \"$MAGELIFT_RELEASE_IMAGE\" \\\n            --builder-image \"$MAGELIFT_BUILDER_IMAGE\" \\\n            --runtime-image \"$MAGELIFT_RUNTIME_IMAGE\" > build-result.json\n          digest=\"$(jq -er '.image.imageReference + \"@\" + .image.digest' build-result.json)\"\n          printf 'digest=%s\\n' \"$digest\" >> \"$GITHUB_OUTPUT\"\n")
	if containsEnvironment(environments, "preview") {
		workflow.WriteString("\n  preview:\n    name: Preview infrastructure\n    if: github.event_name == 'pull_request' && contains(github.event.pull_request.labels.*.name, 'magelift-preview')\n    needs: validate\n    runs-on: ubuntu-latest\n    environment: preview\n    permissions:\n      contents: read\n      id-token: write\n    steps:\n")
		writeWorkflowAWSSetup(&workflow, checkoutAction, setupGoAction, configureAWSAction, configFile, version, "MAGELIFT_PREVIEW_ROLE_ARN", "MAGELIFT_PREVIEW_ENV")
		workflow.WriteString("      - name: Preview selected environment\n        env:\n          PREVIEW_ENV: ${{ vars.MAGELIFT_PREVIEW_ENV }}\n        run: |\n          test -n \"$PREVIEW_ENV\"\n          magelift --config " + shellQuote(configFile) + " --env \"$PREVIEW_ENV\" --no-interaction preview\n")
		workflow.WriteString("\n  preview-destroy:\n    name: Destroy closed preview\n    if: github.event_name == 'pull_request' && github.event.action == 'closed' && contains(github.event.pull_request.labels.*.name, 'magelift-preview')\n    needs: validate\n    runs-on: ubuntu-latest\n    environment: preview\n    permissions:\n      contents: read\n      id-token: write\n    steps:\n")
		writeWorkflowAWSSetup(&workflow, checkoutAction, setupGoAction, configureAWSAction, configFile, version, "MAGELIFT_PREVIEW_ROLE_ARN", "MAGELIFT_PREVIEW_ENV")
		workflow.WriteString("      - name: Destroy preview environment\n        env:\n          PREVIEW_ENV: ${{ vars.MAGELIFT_PREVIEW_ENV }}\n        run: magelift --config " + shellQuote(configFile) + " --env \"$PREVIEW_ENV\" --no-interaction --yes destroy\n")
		workflow.WriteString("\n  preview-sweep:\n    name: Sweep expired previews\n    if: github.event_name == 'schedule'\n    needs: validate\n    runs-on: ubuntu-latest\n    environment: preview\n    permissions:\n      contents: read\n      id-token: write\n    steps:\n")
		writeWorkflowAWSSetup(&workflow, checkoutAction, setupGoAction, configureAWSAction, configFile, version, "MAGELIFT_PREVIEW_ROLE_ARN", "")
		workflow.WriteString("      - name: Destroy expired previews\n        env:\n          PULUMI_BACKEND_URL: ${{ vars.MAGELIFT_PULUMI_BACKEND_URL }}\n        run: |\n          test -n \"$PULUMI_BACKEND_URL\"\n          magelift --config " + shellQuote(configFile) + " --no-interaction --yes env sweep\n")
	}
	if containsEnvironment(environments, "staging") {
		workflow.WriteString("\n  staging:\n    name: Deploy staging\n    if: (github.event_name == 'push' || (github.event_name == 'workflow_dispatch' && inputs.environment == 'staging')) && needs.build.result == 'success'\n    needs: [validate, build]\n    runs-on: ubuntu-latest\n    environment: staging\n    permissions:\n      contents: read\n      id-token: write\n    steps:\n")
		writeWorkflowAWSSetup(&workflow, checkoutAction, setupGoAction, configureAWSAction, configFile, version, "MAGELIFT_STAGING_ROLE_ARN", "MAGELIFT_STAGING_ENV")
		workflow.WriteString("      - name: Deploy immutable digest\n        env:\n          PULUMI_BACKEND_URL: ${{ vars.MAGELIFT_PULUMI_BACKEND_URL }}\n          DEPLOY_ENV: ${{ vars.MAGELIFT_STAGING_ENV }}\n          DIGEST: ${{ needs.build.outputs.digest }}\n        run: |\n          test -n \"$PULUMI_BACKEND_URL\"\n          test \"$DEPLOY_ENV\" = staging\n          magelift --config " + shellQuote(configFile) + " --env \"$DEPLOY_ENV\" --no-interaction --yes deploy --digest \"$DIGEST\"\n")
	}
	if containsEnvironment(environments, "production") {
		workflow.WriteString("\n  production:\n    name: Promote and deploy production\n    if: github.event_name == 'workflow_dispatch' && inputs.environment == 'production' && needs.build.result == 'success'\n    needs: [validate, build]\n    runs-on: ubuntu-latest\n    environment: production\n    permissions:\n      contents: read\n      id-token: write\n    steps:\n")
		writeWorkflowAWSSetup(&workflow, checkoutAction, setupGoAction, configureAWSAction, configFile, version, "MAGELIFT_PRODUCTION_ROLE_ARN", "MAGELIFT_PRODUCTION_ENV")
		workflow.WriteString("      - name: Verify and promote digest\n        env:\n          DIGEST: ${{ needs.build.outputs.digest }}\n          PULUMI_BACKEND_URL: ${{ vars.MAGELIFT_PULUMI_BACKEND_URL }}\n          RELEASE_IDENTITY: ${{ vars.MAGELIFT_RELEASE_IDENTITY }}\n        run: |\n          test -n \"$PULUMI_BACKEND_URL\"\n          test -n \"$RELEASE_IDENTITY\"\n          magelift --config " + shellQuote(configFile) + " --env production --no-interaction --yes promote \\\n            --digest \"$DIGEST\" \\\n            --from staging \\\n            --certificate-identity \"$RELEASE_IDENTITY\" \\\n            --certificate-oidc-issuer https://token.actions.githubusercontent.com\n")
		workflow.WriteString("      - name: Deploy promoted digest\n        env:\n          DIGEST: ${{ needs.build.outputs.digest }}\n        run: magelift --config " + shellQuote(configFile) + " --env production --no-interaction --yes deploy --digest \"$DIGEST\"\n")
	}
	return []byte(workflow.String())
}

func containsEnvironment(environments []string, wanted string) bool {
	for _, environment := range environments {
		if environment == wanted {
			return true
		}
	}
	return false
}

func buildTargetEnvironment(environments []string) string {
	for _, preferred := range []string{"staging", "preview", "production"} {
		if containsEnvironment(environments, preferred) {
			return preferred
		}
	}
	if len(environments) > 0 {
		return environments[0]
	}
	return ""
}

func writeWorkflowAWSSetup(workflow *strings.Builder, checkout, setupGo, configureAWS, configFile, version, roleVariable, environmentVariable string) {
	workflow.WriteString("      - uses: " + checkout + "\n        with:\n          persist-credentials: false\n")
	workflow.WriteString("      - uses: " + setupGo + "\n        with:\n          go-version-file: go.mod\n          cache: false\n")
	workflow.WriteString("      - name: Configure AWS credentials\n        uses: " + configureAWS + "\n        with:\n          role-to-assume: ${{ vars." + roleVariable + " }}\n          aws-region: ${{ vars.MAGELIFT_AWS_REGION }}\n          mask-aws-account-id: true\n")
	workflow.WriteString("      - name: Install MageLift\n        run: go install -ldflags=\"-X github.com/acourtiol/magelift/internal/cli.Version=" + version + "\" github.com/acourtiol/magelift/cmd/magelift@" + version + "\n")
	if environmentVariable != "" {
		workflow.WriteString("      - name: Validate environment variable\n        env:\n          DEPLOY_ENV: ${{ vars." + environmentVariable + " }}\n        run: test -n \"$DEPLOY_ENV\"\n")
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func writeWorkflow(path string, contents []byte) (bool, error) {
	current, err := os.ReadFile(path)
	if err == nil && bytes.Equal(current, contents) {
		return false, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	mode := os.FileMode(0o644)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	}
	if err := replaceFile(path, contents, mode); err != nil {
		return false, err
	}
	return true, nil
}
