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

	"github.com/magelift/magelift/internal/config"
	"github.com/spf13/cobra"
)

const checkoutAction = "actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1"
const setupBuildxAction = "docker/setup-buildx-action@37fe631027851001ddb9b187196cc803df7f5f0e # v4.3.0"
const setupQemuAction = "docker/setup-qemu-action@1f40c72289eff860ee54a304f1438e3cff362e0a # v4.3.0"
const dockerLoginAction = "docker/login-action@dbcb813823bdd20940b903addbd779551569679f # v4.6.0"
const configureAWSAction = "aws-actions/configure-aws-credentials@cbe3b392738ccf3f987d68400dafcf4b0624a56c # v6.2.4"
const gcpAuthAction = "google-github-actions/auth@7c6bc770dae815cd3e89ee6cdf493a5fab2cc093 # v3.0.0"
const gcpSetupAction = "google-github-actions/setup-gcloud@aa5489c8933f4cc7a4f7d45035b3b1440c9c10db # v3.0.1"
const cosignInstallerAction = "sigstore/cosign-installer@6f9f17788090df1f26f669e9d70d6ae9567deba6 # v4.1.2"
const cosignRelease = "v3.1.3"

var releaseVersionPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)

type ciFlags struct {
	path    string
	version string
}

type ciWorkflowGenerator interface {
	Target() string
	Validate(config.Config) error
	WriteBuildAuth(*strings.Builder)
	WriteEnvironmentSetup(*strings.Builder, config.Config, string, string, string, string)
}

type awsCIWorkflowGenerator struct{}

func (awsCIWorkflowGenerator) Target() string { return "aws/ecs-fargate" }

func (awsCIWorkflowGenerator) Validate(cfg config.Config) error {
	if cfg.Target.Provider != "aws" {
		return fmt.Errorf("ci generator target is aws/ecs-fargate, got %q/%q", cfg.Target.Provider, cfg.Target.Runtime)
	}
	if cfg.Target.Runtime != "ecs-fargate" {
		return fmt.Errorf("ci generate does not support runtime %q for provider aws", cfg.Target.Runtime)
	}
	return nil
}

type gcpCIWorkflowGenerator struct{}

func (gcpCIWorkflowGenerator) Target() string { return "gcp/gke" }

func (gcpCIWorkflowGenerator) Validate(cfg config.Config) error {
	if cfg.Target.Provider != "gcp" {
		return fmt.Errorf("ci generator target is gcp/gke, got %q/%q", cfg.Target.Provider, cfg.Target.Runtime)
	}
	if cfg.Target.Runtime != "gke-autopilot" && cfg.Target.Runtime != "gke-standard" {
		return fmt.Errorf("ci generate does not support runtime %q for provider gcp", cfg.Target.Runtime)
	}
	if cfg.Target.GCP == nil || strings.TrimSpace(cfg.Target.GCP.Project) == "" {
		return errors.New("GCP CI requires target.gcp.project")
	}
	if strings.TrimSpace(gcpRegion(cfg)) == "" {
		return errors.New("GCP CI requires target.gcp.region or defaults.region")
	}
	return nil
}

func ciWorkflowGeneratorFor(cfg config.Config) (ciWorkflowGenerator, error) {
	switch cfg.Target.Provider + "/" + cfg.Target.Runtime {
	case "aws/ecs-fargate":
		return awsCIWorkflowGenerator{}, nil
	case "gcp/gke-autopilot", "gcp/gke-standard":
		return gcpCIWorkflowGenerator{}, nil
	default:
		return nil, fmt.Errorf("ci generator is not registered for target %q/%q", cfg.Target.Provider, cfg.Target.Runtime)
	}
}

func gcpRegion(cfg config.Config) string {
	if cfg.Target.GCP != nil && strings.TrimSpace(cfg.Target.GCP.Region) != "" {
		return strings.TrimSpace(cfg.Target.GCP.Region)
	}
	return strings.TrimSpace(cfg.Defaults.Region)
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
	var generator ciWorkflowGenerator
	var targetKey string
	var targetConfig config.Config
	for _, environment := range environments {
		effective, err := file.Resolve(environment, config.ResolveOptions{})
		if err != nil {
			return nil, "", fmt.Errorf("environment %s: %w", environment, err)
		}
		currentGenerator, err := ciWorkflowGeneratorFor(effective.Config)
		if err != nil {
			return nil, "", fmt.Errorf("environment %s: %w", environment, err)
		}
		if generator == nil {
			generator = currentGenerator
			targetKey = effective.Config.Target.Provider + "/" + effective.Config.Target.Runtime
			targetConfig = effective.Config
		} else if target := effective.Config.Target.Provider + "/" + effective.Config.Target.Runtime; target != targetKey {
			return nil, "", fmt.Errorf("environment %s resolves to target %q, but workflow generation already selected %q; mixed provider/runtime targets are not supported", environment, target, targetKey)
		}
		if err := currentGenerator.Validate(effective.Config); err != nil {
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
	workflow := renderWorkflow(filepath.Base(o.configPath), flags.version, environments, generator, targetConfig, targetKey)
	return workflow, filepath.Clean(path), nil
}

func renderWorkflow(configFile, version string, environments []string, generator ciWorkflowGenerator, targetConfig config.Config, targetKey string) []byte {
	var workflow strings.Builder
	buildEnvironment := buildTargetEnvironment(environments)
	workflow.WriteString("name: MageLift CI\n\n")
	workflow.WriteString("# target: " + targetKey + "\n\n")
	workflow.WriteString("on:\n  pull_request:\n    types: [opened, synchronize, reopened, labeled, closed]\n  push:\n    branches: [main]\n  schedule:\n    - cron: '17 * * * *'\n  workflow_dispatch:\n    inputs:\n      environment:\n        description: Environment to deploy\n        required: true\n        type: string\n\n")
	workflow.WriteString("permissions:\n  contents: read\n\n")
	workflow.WriteString("jobs:\n  validate:\n    name: Validate ${{ matrix.environment }}\n    runs-on: ubuntu-latest\n")
	workflow.WriteString("    strategy:\n      fail-fast: false\n      matrix:\n        environment:\n")
	for _, environment := range environments {
		workflow.WriteString("          - " + strconv.Quote(environment) + "\n")
	}
	workflow.WriteString("    steps:\n")
	workflow.WriteString("      - uses: " + checkoutAction + "\n")
	writeVerifiedInstall(&workflow, version, true)
	workflow.WriteString("      - name: Validate configuration\n        run: magelift --config " + shellQuote(configFile) + " --no-interaction config validate\n")
	workflow.WriteString("      - name: Resolve environment\n        run: magelift --config " + shellQuote(configFile) + " --env \"${{ matrix.environment }}\" --no-interaction --output json config effective > /dev/null\n")
	workflow.WriteString("\n  build:\n    name: Build and sign immutable image\n    if: github.event_name == 'push' || github.event_name == 'workflow_dispatch'\n    needs: validate\n    runs-on: ubuntu-latest\n")
	if buildEnvironment != "" {
		workflow.WriteString("    environment: " + buildEnvironment + "\n")
	}
	workflow.WriteString("    permissions:\n      contents: read\n      packages: write\n      id-token: write\n      attestations: write\n    outputs:\n      digest: ${{ steps.build.outputs.digest }}\n    steps:\n")
	workflow.WriteString("      - uses: " + checkoutAction + "\n        with:\n          persist-credentials: false\n")
	workflow.WriteString("      - uses: " + setupQemuAction + "\n      - uses: " + setupBuildxAction + "\n      - uses: " + cosignInstallerAction + "\n        with:\n          cosign-release: " + cosignRelease + "\n")
	workflow.WriteString("      - uses: " + dockerLoginAction + "\n        with:\n          registry: ghcr.io\n          username: ${{ github.actor }}\n          password: ${{ secrets.GITHUB_TOKEN }}\n")
	generator.WriteBuildAuth(&workflow)
	writeVerifiedInstall(&workflow, version, false)
	workflow.WriteString("      - name: Build and sign image\n        id: build\n        env:\n          MAGELIFT_RELEASE_IMAGE: ${{ vars.MAGELIFT_RELEASE_IMAGE }}\n          MAGELIFT_BUILDER_IMAGE: ${{ vars.MAGELIFT_BUILDER_IMAGE }}\n          MAGELIFT_RUNTIME_IMAGE: ${{ vars.MAGELIFT_RUNTIME_IMAGE }}\n        run: |\n          set -eu\n          for required in MAGELIFT_RELEASE_IMAGE MAGELIFT_BUILDER_IMAGE MAGELIFT_RUNTIME_IMAGE; do\n            test -n \"${!required}\"\n          done\n          magelift --config " + shellQuote(configFile) + " --no-interaction --output json build --push \\\n            --image \"$MAGELIFT_RELEASE_IMAGE\" \\\n            --builder-image \"$MAGELIFT_BUILDER_IMAGE\" \\\n            --runtime-image \"$MAGELIFT_RUNTIME_IMAGE\" > build-result.json\n          digest=\"$(jq -er '.image.imageReference + \"@\" + .image.digest' build-result.json)\"\n          printf 'digest=%s\\n' \"$digest\" >> \"$GITHUB_OUTPUT\"\n")
	if containsEnvironment(environments, "preview") {
		workflow.WriteString("\n  preview:\n    name: Preview infrastructure\n    if: github.event_name == 'pull_request' && contains(github.event.pull_request.labels.*.name, 'magelift-preview')\n    needs: validate\n    runs-on: ubuntu-latest\n    environment: preview\n    concurrency:\n      group: magelift-preview-${{ github.repository }}-${{ github.event.pull_request.number }}\n      cancel-in-progress: false\n    permissions:\n      contents: read\n      id-token: write\n    steps:\n")
		generator.WriteEnvironmentSetup(&workflow, targetConfig, configFile, version, "MAGELIFT_PREVIEW_ROLE_ARN", "MAGELIFT_PREVIEW_ENV")
		writePreviewApplyStep(&workflow, configFile)
		workflow.WriteString("\n  preview-destroy:\n    name: Destroy closed preview\n    if: github.event_name == 'pull_request' && github.event.action == 'closed' && contains(github.event.pull_request.labels.*.name, 'magelift-preview')\n    needs: validate\n    runs-on: ubuntu-latest\n    environment: preview\n    concurrency:\n      group: magelift-preview-${{ github.repository }}-${{ github.event.pull_request.number }}\n      cancel-in-progress: false\n    permissions:\n      contents: read\n      id-token: write\n    steps:\n")
		generator.WriteEnvironmentSetup(&workflow, targetConfig, configFile, version, "MAGELIFT_PREVIEW_ROLE_ARN", "MAGELIFT_PREVIEW_ENV")
		writePreviewDestroyStep(&workflow, configFile)
		workflow.WriteString("\n  preview-sweep:\n    name: Sweep expired previews\n    if: github.event_name == 'schedule'\n    needs: validate\n    runs-on: ubuntu-latest\n    environment: preview\n    permissions:\n      contents: read\n      id-token: write\n    steps:\n")
		generator.WriteEnvironmentSetup(&workflow, targetConfig, configFile, version, "MAGELIFT_PREVIEW_ROLE_ARN", "")
		workflow.WriteString("      - name: Destroy expired previews\n        run: |\n          test -n \"${{ vars.MAGELIFT_PREVIEW_ENV }}\"\n          magelift --config " + shellQuote(configFile) + " --env \"${{ vars.MAGELIFT_PREVIEW_ENV }}\" --preview-repository \"${{ github.repository }}\" --no-interaction --yes env sweep\n")
	}
	if containsEnvironment(environments, "staging") {
		workflow.WriteString("\n  staging:\n    name: Deploy staging\n    if: (github.event_name == 'push' || (github.event_name == 'workflow_dispatch' && inputs.environment == 'staging')) && needs.build.result == 'success'\n    needs: [validate, build]\n    runs-on: ubuntu-latest\n    environment: staging\n    permissions:\n      contents: read\n      id-token: write\n    steps:\n")
		generator.WriteEnvironmentSetup(&workflow, targetConfig, configFile, version, "MAGELIFT_STAGING_ROLE_ARN", "MAGELIFT_STAGING_ENV")
		workflow.WriteString("      - name: Deploy immutable digest\n        env:\n          DEPLOY_ENV: ${{ vars.MAGELIFT_STAGING_ENV }}\n          DIGEST: ${{ needs.build.outputs.digest }}\n        run: |\n          test \"$DEPLOY_ENV\" = staging\n          magelift --config " + shellQuote(configFile) + " --env \"$DEPLOY_ENV\" --no-interaction --yes deploy --digest \"$DIGEST\"\n")
	}
	if containsEnvironment(environments, "production") {
		workflow.WriteString("\n  production:\n    name: Promote and deploy production\n    if: github.event_name == 'workflow_dispatch' && inputs.environment == 'production' && needs.build.result == 'success'\n    needs: [validate, build]\n    runs-on: ubuntu-latest\n    environment: production\n    permissions:\n      contents: read\n      id-token: write\n    steps:\n")
		generator.WriteEnvironmentSetup(&workflow, targetConfig, configFile, version, "MAGELIFT_PRODUCTION_ROLE_ARN", "MAGELIFT_PRODUCTION_ENV")
		workflow.WriteString("      - name: Verify and promote digest\n        env:\n          DIGEST: ${{ needs.build.outputs.digest }}\n        run: |\n          magelift --config " + shellQuote(configFile) + " --env production --no-interaction --yes promote \\\n            --digest \"$DIGEST\" \\\n            --from staging\n")
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

func (awsCIWorkflowGenerator) WriteBuildAuth(workflow *strings.Builder) {
	workflow.WriteString("      - name: Configure AWS credentials\n        uses: " + configureAWSAction + "\n        with:\n          role-to-assume: ${{ vars.MAGELIFT_BUILD_ROLE_ARN }}\n          aws-region: ${{ vars.MAGELIFT_AWS_REGION }}\n          mask-aws-account-id: true\n")
}

func (awsCIWorkflowGenerator) WriteEnvironmentSetup(workflow *strings.Builder, _ config.Config, _ string, version, roleVariable, environmentVariable string) {
	writeWorkflowCheckoutAndInstall(workflow, version)
	workflow.WriteString("      - name: Configure AWS credentials\n        uses: " + configureAWSAction + "\n        with:\n          role-to-assume: ${{ vars." + roleVariable + " }}\n          aws-region: ${{ vars.MAGELIFT_AWS_REGION }}\n          mask-aws-account-id: true\n")
	if environmentVariable != "" {
		workflow.WriteString("      - name: Validate environment variable\n        env:\n          DEPLOY_ENV: ${{ vars." + environmentVariable + " }}\n        run: test -n \"$DEPLOY_ENV\"\n")
	}
}

func (gcpCIWorkflowGenerator) WriteBuildAuth(workflow *strings.Builder) {
	workflow.WriteString("      - name: Validate GCP federation variables\n        env:\n          GCP_PROJECT_ID: ${{ vars.MAGELIFT_GCP_PROJECT_ID }}\n          GCP_WIF_PROVIDER: ${{ vars.MAGELIFT_GCP_WORKLOAD_IDENTITY_PROVIDER }}\n          GCP_SERVICE_ACCOUNT: ${{ vars.MAGELIFT_GCP_SERVICE_ACCOUNT }}\n        run: |\n          test -n \"$GCP_PROJECT_ID\"\n          test -n \"$GCP_WIF_PROVIDER\"\n          test -n \"$GCP_SERVICE_ACCOUNT\"\n")
	writeGCPAuth(workflow)
}

func (gcpCIWorkflowGenerator) WriteEnvironmentSetup(workflow *strings.Builder, cfg config.Config, _ string, version, _, environmentVariable string) {
	writeWorkflowCheckoutAndInstall(workflow, version)
	project := cfg.Target.GCP.Project
	region := gcpRegion(cfg)
	workflow.WriteString("      - name: Validate GCP deployment variables\n        env:\n          GCP_PROJECT_ID: ${{ vars.MAGELIFT_GCP_PROJECT_ID }}\n          GCP_REGION: ${{ vars.MAGELIFT_GCP_REGION }}\n          GCP_WIF_PROVIDER: ${{ vars.MAGELIFT_GCP_WORKLOAD_IDENTITY_PROVIDER }}\n          GCP_SERVICE_ACCOUNT: ${{ vars.MAGELIFT_GCP_SERVICE_ACCOUNT }}\n")
	if environmentVariable != "" {
		workflow.WriteString("          DEPLOY_ENV: ${{ vars." + environmentVariable + " }}\n")
	}
	workflow.WriteString("        run: |\n          set -eu\n          test -n \"$GCP_PROJECT_ID\"\n          test \"$GCP_PROJECT_ID\" = " + shellQuote(project) + "\n          test -n \"$GCP_REGION\"\n          test \"$GCP_REGION\" = " + shellQuote(region) + "\n          test -n \"$GCP_WIF_PROVIDER\"\n          test -n \"$GCP_SERVICE_ACCOUNT\"\n")
	if environmentVariable != "" {
		workflow.WriteString("          test -n \"$DEPLOY_ENV\"\n")
	}
	writeGCPAuth(workflow)
}

func writeGCPAuth(workflow *strings.Builder) {
	workflow.WriteString("      - name: Authenticate to Google Cloud\n        uses: " + gcpAuthAction + "\n        with:\n          project_id: ${{ vars.MAGELIFT_GCP_PROJECT_ID }}\n          workload_identity_provider: ${{ vars.MAGELIFT_GCP_WORKLOAD_IDENTITY_PROVIDER }}\n          service_account: ${{ vars.MAGELIFT_GCP_SERVICE_ACCOUNT }}\n          create_credentials_file: true\n          export_environment_variables: true\n          cleanup_credentials: true\n")
	workflow.WriteString("      - name: Set up Google Cloud SDK\n        uses: " + gcpSetupAction + "\n        with:\n          project_id: ${{ vars.MAGELIFT_GCP_PROJECT_ID }}\n          version: '>= 363.0.0'\n")
}

func writeWorkflowCheckoutAndInstall(workflow *strings.Builder, version string) {
	workflow.WriteString("      - uses: " + checkoutAction + "\n        with:\n          persist-credentials: false\n")
	writeVerifiedInstall(workflow, version, true)
}

// writeVerifiedInstall installs the pinned release binary: Sigstore bundle
// on checksums.txt, archive checksum, extract to PATH. Every step fails
// closed; no toolchain compile. cosignInstaller adds the cosign-installer
// step for jobs that do not already install cosign (generated jobs all run
// ubuntu-latest, so the linux/amd64 archive is correct).
func writeVerifiedInstall(workflow *strings.Builder, version string, cosignInstaller bool) {
	if cosignInstaller {
		workflow.WriteString("      - uses: " + cosignInstallerAction + "\n        with:\n          cosign-release: " + cosignRelease + "\n")
	}
	workflow.WriteString("      - name: Install MageLift\n        env:\n          MAGELIFT_VERSION: " + version + "\n        run: |\n")
	workflow.WriteString("          set -eu\n")
	workflow.WriteString("          archive=\"magelift_${MAGELIFT_VERSION#v}_linux_amd64.tar.gz\"\n")
	workflow.WriteString("          base=\"https://github.com/magelift/magelift/releases/download/$MAGELIFT_VERSION\"\n")
	workflow.WriteString("          tmp=\"$(mktemp -d)\"\n")
	workflow.WriteString("          trap 'rm -rf \"$tmp\"' EXIT\n")
	workflow.WriteString("          curl -fsSL -o \"$tmp/$archive\" \"$base/$archive\"\n")
	workflow.WriteString("          curl -fsSL -o \"$tmp/checksums.txt\" \"$base/checksums.txt\"\n")
	workflow.WriteString("          curl -fsSL -o \"$tmp/checksums.txt.sigstore.json\" \"$base/checksums.txt.sigstore.json\"\n")
	workflow.WriteString("          cosign verify-blob --bundle \"$tmp/checksums.txt.sigstore.json\" --certificate-identity \"https://github.com/magelift/magelift/.github/workflows/release.yml@refs/tags/$MAGELIFT_VERSION\" --certificate-oidc-issuer https://token.actions.githubusercontent.com \"$tmp/checksums.txt\"\n")
	workflow.WriteString("          (cd \"$tmp\" && grep \" $archive$\" checksums.txt | sha256sum -c -)\n")
	workflow.WriteString("          tar -xzf \"$tmp/$archive\" -C \"$tmp\" magelift\n")
	workflow.WriteString("          sudo install -m 0755 \"$tmp/magelift\" /usr/local/bin/magelift\n")
	workflow.WriteString("          magelift version\n")
}

func writePreviewApplyStep(workflow *strings.Builder, configFile string) {
	workflow.WriteString("      - name: Preview selected environment\n        env:\n          PREVIEW_ENV: ${{ vars.MAGELIFT_PREVIEW_ENV }}\n          PREVIEW_REPOSITORY: ${{ github.repository }}\n          PREVIEW_NUMBER: ${{ github.event.pull_request.number }}\n          PREVIEW_BRANCH: ${{ github.head_ref }}\n          PREVIEW_COMMIT: ${{ github.event.pull_request.head.sha }}\n          PREVIEW_GENERATION: ${{ github.run_id }}\n        run: |\n          set -eu\n          test -n \"$PREVIEW_ENV\"\n          test -n \"$PREVIEW_REPOSITORY\"\n          test -n \"$PREVIEW_NUMBER\"\n          test -n \"$PREVIEW_COMMIT\"\n          magelift --config " + shellQuote(configFile) + " --env \"$PREVIEW_ENV\" --preview-repository \"$PREVIEW_REPOSITORY\" --preview-number \"$PREVIEW_NUMBER\" --preview-branch \"$PREVIEW_BRANCH\" --preview-commit \"$PREVIEW_COMMIT\" --preview-generation \"$PREVIEW_GENERATION\" --no-interaction preview\n")
}

func writePreviewDestroyStep(workflow *strings.Builder, configFile string) {
	workflow.WriteString("      - name: Destroy preview environment\n        env:\n          PREVIEW_ENV: ${{ vars.MAGELIFT_PREVIEW_ENV }}\n          PREVIEW_REPOSITORY: ${{ github.repository }}\n          PREVIEW_NUMBER: ${{ github.event.pull_request.number }}\n          PREVIEW_BRANCH: ${{ github.head_ref }}\n          PREVIEW_COMMIT: ${{ github.event.pull_request.head.sha }}\n          PREVIEW_GENERATION: ${{ github.run_id }}\n        run: |\n          set -eu\n          test -n \"$PREVIEW_ENV\"\n          test -n \"$PREVIEW_REPOSITORY\"\n          test -n \"$PREVIEW_NUMBER\"\n          test -n \"$PREVIEW_COMMIT\"\n          magelift --config " + shellQuote(configFile) + " --env \"$PREVIEW_ENV\" --preview-repository \"$PREVIEW_REPOSITORY\" --preview-number \"$PREVIEW_NUMBER\" --preview-branch \"$PREVIEW_BRANCH\" --preview-commit \"$PREVIEW_COMMIT\" --preview-generation \"$PREVIEW_GENERATION\" --no-interaction --yes destroy\n")
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
