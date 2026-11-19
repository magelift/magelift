package cli

import (
	"errors"
	"strings"

	"github.com/magelift/magelift/internal/config"
)

const azureLoginAction = "azure/login@a641126d1b8aa4d1fa005f4f92df94a3a4c4c906 # v3.1.0"
const azureKubeloginAction = "azure/use-kubelogin@0ce7c36141aa27d4934872cf00b0120804c98a29 # v1.3"

type azureCIWorkflowGenerator struct{}

func (azureCIWorkflowGenerator) Target() string { return "azure/aks" }
func (azureCIWorkflowGenerator) Validate(cfg config.Config) error {
	if cfg.Target.Provider != "azure" || cfg.Target.Runtime != "aks" || cfg.Target.Azure == nil || cfg.Target.Azure.SubscriptionID == "" {
		return errors.New("Azure CI requires an AKS target and target.azure.subscriptionId")
	}
	return nil
}
func writeAzureLogin(w *strings.Builder) {
	w.WriteString("      - name: Sign in to Azure with federated credentials\n        uses: " + azureLoginAction + "\n        with:\n          client-id: ${{ vars.MAGELIFT_AZURE_CLIENT_ID }}\n          tenant-id: ${{ vars.MAGELIFT_AZURE_TENANT_ID }}\n          subscription-id: ${{ vars.MAGELIFT_AZURE_SUBSCRIPTION_ID }}\n")
}
func (azureCIWorkflowGenerator) WriteBuildAuth(w *strings.Builder) {
	writeAzureLogin(w)
	w.WriteString("      - name: Authenticate Azure Container Registry\n        if: endsWith(vars.MAGELIFT_REGISTRY, '.azurecr.io')\n        env:\n          REGISTRY: ${{ vars.MAGELIFT_REGISTRY }}\n        run: az acr login --name \"${REGISTRY%%.*}\"\n")
}
func (azureCIWorkflowGenerator) WriteEnvironmentSetup(w *strings.Builder, _ config.Config, configFile string, version, _, _ string) {
	writeWorkflowCheckoutAndInstall(w, configFile, version)
	w.WriteString(`      - name: Validate Azure state configuration
        env:
          AZURE_STORAGE_ACCOUNT: ${{ vars.MAGELIFT_AZURE_STORAGE_ACCOUNT }}
          AZURE_TENANT_ID: ${{ vars.MAGELIFT_AZURE_TENANT_ID }}
          PULUMI_CONFIG_PASSPHRASE: ${{ secrets.MAGELIFT_PULUMI_CONFIG_PASSPHRASE }}
        run: |
          set -eu
          if ! [[ "$AZURE_STORAGE_ACCOUNT" =~ ^[a-z0-9]{3,24}$ ]]; then
            echo 'Set MAGELIFT_AZURE_STORAGE_ACCOUNT to the state account name.' >&2
            exit 1
          fi
          if ! [[ "$AZURE_TENANT_ID" =~ ^[a-zA-Z0-9.-]+$ ]]; then
            echo 'Set MAGELIFT_AZURE_TENANT_ID to the Azure tenant ID.' >&2
            exit 1
          fi
          if ! [[ "$PULUMI_CONFIG_PASSPHRASE" =~ [^[:space:]] ]]; then
            echo 'Set the MAGELIFT_PULUMI_CONFIG_PASSPHRASE secret.' >&2
            exit 1
          fi
          delimiter="MAGELIFT_$(openssl rand -hex 16)"
          if printf '%s\n' "$PULUMI_CONFIG_PASSPHRASE" | grep -Fxq "$delimiter"; then
            exit 1
          fi
          {
            printf 'AZURE_STORAGE_ACCOUNT=%s\n' "$AZURE_STORAGE_ACCOUNT"
            printf 'AZURE_TENANT_ID=%s\n' "$AZURE_TENANT_ID"
            printf 'PULUMI_CONFIG_PASSPHRASE<<%s\n%s\n%s\n' "$delimiter" "$PULUMI_CONFIG_PASSPHRASE" "$delimiter"
          } >> "$GITHUB_ENV"
`)
	writeAzureLogin(w)
	w.WriteString("      - name: Install kubelogin\n        uses: " + azureKubeloginAction + "\n        with:\n          kubelogin-version: 'v0.2.22'\n")
}
