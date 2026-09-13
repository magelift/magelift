## Purpose

Captures composition rules that are easy to miss when capabilities are implemented one at a time. Each rule is an invariant, not a new product area.

## ADDED Requirements

### Requirement: Destroy honors backup retention

Destroying an environment MUST NOT delete retained backups that the backup policy still requires, unless the operator passes an explicit flag that the command documents as destroying backups too. Preview teardown MAY delete preview-owned backups when the preview policy says they are disposable.

#### Scenario: Production destroy keeps required backups

- **WHEN** a production environment is destroyed without a destroy-backups flag
- **THEN** provider backups still inside retention remain, and the command reports what was retained

### Requirement: Preview DNS is ownership-scoped and disposable

Preview hostnames MUST use ownership-scoped DNS records. Destroy MUST remove those records and MUST NOT delete a pre-existing zone. Fixed-environment DNS MUST NOT be reused as a preview hostname.

#### Scenario: Preview destroy leaves the zone

- **WHEN** a preview environment that created a hostname record is destroyed
- **THEN** only the ownership-scoped record is removed and the parent zone remains

### Requirement: CI authentication is non-interactive

CI deploys MUST use short-lived federation or injected credentials. They MUST NOT open a browser login. Interactive `magelift login` remains valid for operators.

#### Scenario: Generated GitHub Actions workflow

- **WHEN** `magelift ci generate` writes a preview workflow for GCP
- **THEN** the job uses workload identity federation or the documented short-lived credential path and does not call an interactive login

### Requirement: Secrets never appear in logs or YAML

Configuration, plans, logs, health output, and evidence MUST store secret references, not secret values. Log redaction MUST apply to application, web, PHP, Magento, and infrastructure logs retrieved through MageLift.

#### Scenario: Log line contains a token

- **WHEN** retrieved logs include a known credential or token shape
- **THEN** the CLI redacts it before writing output or evidence

### Requirement: Preview spend is bounded

Preview environments MUST use preview cost presets and MUST NOT inherit production budget limits as if they were already spent. Unexpectedly expensive catalog choices (for example managed HA message brokers on a preview) MUST be reported at plan time.

#### Scenario: Preview selects Amazon MQ

- **WHEN** a preview plan would provision a production-priced managed broker
- **THEN** planning warns or fails closed according to the catalog, and `cost` labels the estimate as preview-incompatible or high

### Requirement: Patch application is not skipped by cache

A cache hit on a build layer MUST NOT skip patch application when the patch set, Magento version, or hotfix directory changed. Repeat builds with the same inputs MAY reuse work; changed patches MUST rebuild the affected layer.

#### Scenario: Hotfix added after a cached build

- **WHEN** a new file appears under `m2-hotfixes` and the operator rebuilds
- **THEN** patches are applied for the new set and the previous cached layer is not reused as the final Magento tree

### Requirement: Application rollback does not imply database rollback

Rolling back an application digest MUST leave database schema and data as they are unless a restore is explicitly requested. If the previous digest cannot run on the current schema, MageLift MUST refuse and name restore or forward-fix options.

#### Scenario: Rollback blocked by schema

- **WHEN** rollback targets a digest that required an older schema than the live database
- **THEN** the command fails before cutover and names the schema mismatch

### Requirement: PHP presets follow Magento compatibility

A Magento-optimized PHP preset MUST only enable extensions and ini values allowed for that Magento release. Selecting a Magento version MUST NOT keep a previous release's PHP preset if that preset is incompatible.

#### Scenario: Magento version changes PHP

- **WHEN** a project moves from a release that allowed PHP 8.3 to one that requires PHP 8.4+
- **THEN** validation fails until `build.php` matches the new catalog row

### Requirement: Autoscaling keeps shared media coherent

Horizontal scaling MUST use shared media (object storage or equivalent) and MUST NOT leave per-instance local `pub/media` as the only copy. Cache and generated code MAY stay local when Magento's contract allows it.

#### Scenario: Second replica serves media

- **WHEN** a production profile scales to more than one web replica
- **THEN** media requests resolve from the shared media backend, not from replica-local disks

### Requirement: Restore needs the encryption keys

A restore MUST fail if the encryption or KMS key required to read backups or Magento crypt key is missing. MageLift MUST NOT write recovered data that cannot be decrypted.

#### Scenario: Backup key is gone

- **WHEN** restore runs and the referenced KMS or crypt-key secret is missing
- **THEN** restore stops, names the missing key reference, and does not mark the environment recovered

### Requirement: Auto-healing stays observable

Restarts, replacements, and autoscaling MAY recover instance failure. They MUST emit health and event records. They MUST NOT retry indefinitely in a way that hides a systemic failure (bad digest, exhausted quota, failing Magento setup). After the documented retry budget, MageLift or the provider adapter MUST surface the failure.

#### Scenario: Bad digest crash-loops

- **WHEN** a new release crash-loops beyond the retry budget
- **THEN** health is unhealthy, deploy is failed, and auto-restart is not reported as success
