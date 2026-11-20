# Deploy a staging shop on GCP

This walkthrough takes a Magento shop from its first local run to a staging deployment on GKE Autopilot. You’ll need a GCP project with billing enabled and a domain where you can edit DNS. Cloud SQL, Memorystore and GKE can incur charges while staging is idle.

## Start in the shop repository

Install the CLI from the MageLift checkout with `go install ./cmd/magelift`, then return to the Magento repository. A source build needs a matching provider binary and lock before it can run cloud commands. The install guide covers provider acquisition and offline setup. [Installation](install.md)

```sh
magelift init --provider gcp --cloud-account YOUR_PROJECT_ID \
  --region europe-west1 --domain shop.example.com
magelift config validate
magelift dev
```

Open the local URL and place a test order. The starter reads the Magento edition and release from your shop and writes `magelift.php`. Staging and pull-request previews start small, UAT uses standard capacity, and production uses the protected high-availability preset. Compare them with `magelift env diff staging production`. Review the service versions and commit the project file and Composer lock before building. [Service choices](service-bindings.md)

## Prepare GCP and the registry

Install the [Pulumi CLI](https://www.pulumi.com/docs/iac/download-install/) on the workstation for cloud infrastructure and stack operations. It is optional for local development. Authenticate with `gcloud auth application-default login` on the workstation. Run `magelift --env staging doctor` to check the operator credentials. The operator running bootstrap needs permission to enable service APIs, create the state bucket and secrets, create the Artifact Registry repository and grant access to the GKE node identity.

```sh
magelift --env staging bootstrap
```

Keep the image destination and registry hostname from the output. MageLift creates a versioned state bucket, a passphrase secret and a node identity that can pull images from the repository. For a migrated shop, configure the original Magento encryption key in Secret Manager before importing customer data. A new shop gets a key on its first deployment.

To deploy from GitHub, pass `--github-owner OWNER --github-repo REPO` to bootstrap. Read the returned `ciPermissionPlan` with the project administrator. Some deployment roles cover the entire GCP project. Once approved, repeat the command with `--apply-ci-permissions --configure-github` to grant the roles and write the repository variables. Put private Composer credentials in a GitHub secret. Configure GitHub environment reviewers separately. [Bootstrap and IAM](bootstrap.md)

## Build and deploy the image

Use Docker Engine and Buildx on the build host. Point Docker at the registry
named by bootstrap, then build from a clean shop checkout. Pass the manifest
path printed by the build to the deploy command.

```sh
gcloud auth configure-docker REGION-docker.pkg.dev
magelift build --push --image REGION-docker.pkg.dev/PROJECT/REPOSITORY/shop:candidate
magelift --env staging deploy --from-build /path/to/manifest.json --yes
magelift --env staging outputs --output json
```

Set `target.gcp.imageDigest` to the immutable image reference to reuse it in later runs. For private dependencies set `build.composer.credentials` to `env://COMPOSER_AUTH` or a supported secret reference. [Application builds](builds.md)

Point the staging hostname at the load balancer address from the outputs.
DNS propagation and issuance of the managed TLS certificate can take time.
Once HTTPS responds, open the storefront URL and run the health check.

```sh
magelift --env staging health --mode runtime
magelift --env staging logs --service web
magelift --env staging logs --service cron
```

- Open the storefront and fetch a stylesheet referenced by the page. For a headless shop, query the Magento GraphQL endpoint from the configured storefront origin.
- Add a product, reindex it and search for it. Upload a product image, open its storefront URL, and check the object under `media/` in the private media bucket named in the outputs.
- If the shop sends mail, configure an SMTP relay and secret reference, then test order and password-reset emails through to delivery. The starter leaves sending disabled.

## Give pull requests their own environments

After a signed release is available, run `magelift ci generate --magelift-version vX.Y.Z` with that tag and review the generated workflow. Configure the build, preview and stable-environment identities in your shop repository. Require approval on the `preview-build`, `preview` and `production` GitHub environments as appropriate.

Label the reviewed head of a same-repository pull request to `main` with `magelift-preview`. For a later commit, remove and reapply the label after review. Removing the label or closing the pull request schedules teardown. The hourly sweep also checks the configured preview lifetime. [Preview and promotion](operations.md)

Staging persists until you remove it. Check its planned capacity with `magelift --env staging cost`. A budget alert leaves resources running. When you finish the rehearsal, tear down the staging stack and review the reported retained resources.

```sh
magelift --env staging destroy --yes
```
