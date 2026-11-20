# Rehearse a PaaS migration

Run a complete rehearsal before scheduling a cutover. Use a separate cloud account or project and a disposable environment with a sanitized database and media copy. Check the sanitized copy for remaining personal data before granting access, and delete it under your retention policy.

## Prepare the shop and the data

1. Import the source configuration with `magelift init --from-upsun` or `magelift init --from-acc`. Pass `--provider`, `--cloud-account` and `--region` for your destination. The examples below use `--config-out rehearsal.php`. Pass your production hostname with `--domain` to generate the staging and UAT hostnames, or set each hostname in the PHP file. In a multi-app source, select the Magento application with `--app`. Review service versions, Composer credentials, extension modules, cron and consumers in the generated PHP and unmapped report. [Move a Magento shop to MageLift](migrating-from-paas.md)
2. Obtain a consistent SQL dump and the matching media tree. Record the source Magento version, enabled modules, time of the snapshot and original Magento encryption key. The key is needed to decrypt existing encrypted values. Put it in the destination's supported secret store. Keep the key, database password and dump outside Git.
3. Verify that the destination can accept the dump. Check the SQL version and collation, `DEFINER` clauses, database names, privileges and incompatible extensions. Take a backup of every destination you might touch and test that backup separately. Back up SQL and media separately from Pulumi state.
4. Select a dedicated staging or preview environment. Check its account, region, database, media store and estimated running cost.
5. Bootstrap the target, build the shop image and record its immutable digest. Configure the original encryption key before applying the stack. AKS accepts a versioned Key Vault URL under `target.azure.encryptionKeySecret` or an existing namespace-local Secret under `encryptionKeySecretName`. The Kubernetes Secret needs a `key` entry in the destination namespace. [Bootstrap](bootstrap.md) · [Application builds](builds.md)

## Create infrastructure before importing

Use a **new empty database** and an empty destination media store. Use the reviewed image digest from the build.

```sh
magelift --config rehearsal.php --env staging preview
magelift --config rehearsal.php --env staging deploy --infra-only --digest "$IMAGE_DIGEST"
magelift --config rehearsal.php --env staging outputs --output json
```

`--infra-only` applies the infrastructure graph and skips Magento migration, cutover and health checks. The provider may start web, cron or consumer workloads while provisioning. Keep traffic away and stop writers before you restore. Confirm the database identity and contents against the stack outputs.

Restore into the empty destination before the first application deploy. On a private host, `env import-dump staging --from <dump>` records the outcome for `env status staging`. On Kubernetes, `migrate restore` reports SQL and media progress directly. If the target already holds data, review the overwrite behavior for the command you chose before continuing.

## AWS ECS: private MySQL and S3 media

`magelift migrate restore` uses Kubernetes and does not run on ECS. For an ECS/RDS rehearsal, run the host MySQL or MariaDB client on an operator-controlled machine with private network reachability to the RDS writer and permission to use the selected database. Do not assume that the default import transport reaches the cloud: without an explicit host it targets local MySQL or local Compose. The ECS adapter does not offer a MageLift database tunnel.

For the existing staging environment above, use the explicit host transport:

```sh
# Set these to the reviewed RDS writer DNS name, database, user, and CA bundle.
export MAGELIFT_DUMPIMPORT_HOST="$RDS_WRITER_DNS"
export MAGELIFT_DUMPIMPORT_PORT=3306
export MAGELIFT_DUMPIMPORT_DATABASE=magento
export MAGELIFT_DUMPIMPORT_USER="$DB_USER"
export MAGELIFT_DUMPIMPORT_TLS=verify
export MAGELIFT_DUMPIMPORT_CA=/secure/rds-ca-bundle.pem
unset MAGELIFT_DUMPIMPORT_RUNNER
read -r -s -p 'Database password: ' MAGELIFT_DUMPIMPORT_PASSWORD; echo
export MAGELIFT_DUMPIMPORT_PASSWORD
magelift --config rehearsal.php env import-dump staging --from /secure/rehearsal.sql.gz
unset MAGELIFT_DUMPIMPORT_PASSWORD
```

The TLS setting requires the CA file and a client that identifies itself as MySQL or MariaDB. The connection verifies the server certificate and hostname. Use the writer DNS name so the hostname matches the certificate. Install the [RDS CA bundle](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/UsingWithRDS.SSL.html) on the import host. The first import into an empty database does not require `--yes`. If the import is interrupted, inspect the target and restore from backup or use a new empty database. `--yes` on a later import drops and recreates a nonempty schema before loading the dump.

On ECS Fargate, MageLift configures Magento's native S3 driver with the task role. Media sync reads `mediaBucket` from the stack outputs and writes the contents of the local media directory under `media/` in that bucket. Run the sync with a role in the bucket's AWS account. If you work from another account, assume that role before uploading.

```sh
magelift --config rehearsal.php env media-sync staging --source /secure/rehearsal-media
```

Media sync overwrites matching keys and leaves other objects in place. Finish the sync before switching to fresh ECS tasks. Tasks that cached an old file can keep serving it until replaced. To serve images through the media CDN, the KMS key policy must allow the CloudFront service to decrypt objects for that distribution. Use the `mediaDistributionId` stack output in the distribution ARN. Verify a storefront image through the CDN and a fresh upload through Admin. [CloudFront KMS key policy](https://docs.aws.amazon.com/AmazonCloudFront/latest/DeveloperGuide/private-content-restricting-access-to-s3.html#private-content-s3-origin-configure-s3-bucket)

## GCP GKE and Azure AKS: Kubernetes restore

`magelift migrate restore` creates temporary helper pods in the selected namespace. It can stream a local `.sql` or `.sql.gz` file to a reachable MySQL-compatible database, and it can merge a local directory into an existing PVC. It does not create the database, its credentials, or the PVC. The operator needs Kubernetes permissions to inspect the namespace and create, exec into and delete helper pods, plus network access from those pods to the database. Private clusters also need a way for the helpers to pull their pinned images.

Set `KUBE_CONTEXT` to your operator context for the cluster in the stack outputs. Set `NAMESPACE` to the `namespace` output. Stable GKE and AKS environments use `default` unless the stack assigns another namespace. Check both values against the cluster before reading the namespace UID.

```sh
kubectl --context "$KUBE_CONTEXT" get namespace "$NAMESPACE"
NAMESPACE_UID=$(kubectl --context "$KUBE_CONTEXT" get namespace "$NAMESPACE" -o jsonpath='{.metadata.uid}')
```

The examples below use a Secret containing `username` and `password`. For an attached database whose Secret contains only `password`, add `--database-user "$DB_USER"` to **both** the plan and execution commands. The helper connects to `--database-host` on port 3306. The default is verified TLS. Supply `--ca-secret "$DB_CA_SECRET"` when the namespace Secret contains a `ca.crt` for the server. The server certificate must match the supplied host. Use a hostname in the server certificate or the provider's import service. Select `--tls disabled` only when the operator has explicitly accepted an unencrypted connection in the isolated rehearsal network.

Inspect the printed destination before adding `--execute --yes`.

```sh
magelift --output json migrate restore \
  --context "$KUBE_CONTEXT" --namespace "$NAMESPACE" --namespace-uid "$NAMESPACE_UID" \
  --database-host "$DB_HOST" --database magento --database-secret "$DB_SECRET" \
  --tls verify --ca-secret "$DB_CA_SECRET" --sql /secure/rehearsal.sql.gz

magelift --output json --yes migrate restore \
  --context "$KUBE_CONTEXT" --namespace "$NAMESPACE" --namespace-uid "$NAMESPACE_UID" \
  --database-host "$DB_HOST" --database magento --database-secret "$DB_SECRET" \
  --tls verify --ca-secret "$DB_CA_SECRET" --sql /secure/rehearsal.sql.gz --execute
```

Omit `--ca-secret` if the helper image's system CA bundle validates the server. The helpers run for up to one hour by default. Set `--timeout` on both commands if the transfer takes longer. The command checks the namespace UID and refuses a database containing tables or views unless `--allow-nonempty` is explicitly supplied. Review stored routines and other schema objects separately. `--allow-nonempty` does not clear the schema. It permits the SQL to run against it. If SQL or media import fails partway through, inspect both destinations before retrying. If a restore is interrupted, check for any remaining `app.kubernetes.io/name=magelift-restore` pods.

### GCP media and managed Cloud SQL

GKE stores media in Google Cloud Storage. Use your Google credentials to upload the contents of the source media directory under `media/` in the environment's `mediaBucket`.

```sh
magelift --config rehearsal.php env media-sync staging --source /secure/rehearsal-media
```

The transfer overwrites matching objects and leaves other objects in place. Compare the source and destination counts and open representative files.

For Cloud SQL, check whether the server certificate covers the private address or hostname reachable from the helper. If it doesn't, upload the dump to a private GCS bucket and use Cloud SQL's import service. Grant the instance service account access to the object, review the dump's `USE` statements and confirm the empty destination before starting the import. [Cloud SQL import procedure](https://cloud.google.com/sql/docs/mysql/import-export/import-export-sql)

### Azure AKS media

The AKS stack exports `namespace`, `databaseWriter`, `databaseSecretName` and `mediaClaim`. Read the Azure Files claim name from `mediaClaim`. MageLift creates it as `magelift-media`. After restoring SQL, plan and execute the media merge.

```sh
magelift --output json migrate restore \
  --context "$KUBE_CONTEXT" --namespace "$NAMESPACE" --namespace-uid "$NAMESPACE_UID" \
  --media /secure/rehearsal-media --media-claim "$MEDIA_CLAIM"
magelift --output json --yes migrate restore \
  --context "$KUBE_CONTEXT" --namespace "$NAMESPACE" --namespace-uid "$NAMESPACE_UID" \
  --media /secure/rehearsal-media --media-claim "$MEDIA_CLAIM" --execute
```

The merge overwrites matching files and leaves other files in the claim. Supply regular files and directories. The helper rejects symbolic links and writes ownership for Magento on POSIX volumes. Test an upload into a restored folder as the application user. Azure managed MySQL requires TLS.

## Deploy and assess the rehearsal

After SQL and media are in place, run the normal deploy with the same reviewed image digest. It runs Magento migration and health steps against the imported database. For a production-class deployment, follow the maintenance, writer-drain and signed-release requirements. [Operations](operations.md)

```sh
magelift --config rehearsal.php --env staging deploy --digest "$IMAGE_DIGEST"
magelift --config rehearsal.php --env staging health --mode runtime
magelift --config rehearsal.php --env staging logs --service web
```

Verify the effective search adapter, reindex, check a known product and media asset, and exercise login, checkout, email, cron and consumers as applicable. For a headless shop, test the Magento APIs and the separately deployed storefront. Rehearse DNS and TLS with an operator-owned test hostname. Record timings, source freeze and rollback steps, including how to handle writes after a real cutover.

Destroy only the disposable environment when its owner approves, and verify its resources and rehearsal data are removed or retained according to policy. Keep the source and any backups needed for recovery.
