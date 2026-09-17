# Spec: alpha review corrections

Each section resolves one finding. Proofs are the review's,
narrowed only where noted with reasoning.

## R01 — Connected installation

- Lockfiles generated with absolute `URL` plus absolute bundle
  URL under the release download base. No empty-URL entries
  from the generator; empty URL keeps meaning beside-CLI
  bundled (dev path only, never generated).
- First-party publisher pinned in the CLI (release workflow
  identity pattern plus OIDC issuer). Downloaded lockfiles
  whose identity/issuer differ are refused: metadata can
  never redefine the trusted publisher. Content trust stays
  digest plus bundle-over-pinned-identity, so lockfiles need
  no separate signature; the reasoning is documented.
- One resolver shared by registry, backend, hooks, and
  `providers install`: lock source order is `--lockfile`
  flag, `./magelift.providers.lock`, beside-CLI lock;
  binary order is beside-CLI (version must match the
  selected lock), then versioned cache. Project locks
  control the version run. `dialGCP` uses it (cache-aware).
- Release-metadata bootstrap: `providers install` with no
  lockfile fetches the platform lockfile for an explicit
  version (default: the CLI's own version) from the release
  base over HTTPS, enforces the publisher pin, then
  installs. No silent version choice.
- Verifier provisioning: `providers install` persists the
  pinned Cosign bootstrap into the cache; runtime
  verification prefers the cached verifier, then system
  Cosign. Clean machines need nothing preinstalled.
- Generated CI installs the provider the same way after
  the CLI (same pinned version).
- Installer harness honors `TMPDIR` (review's quota block).
- Proof: the review's clean-machine procedure (fresh GCE
  micro VM, destroyed after): published script, GCP YAML,
  provider install, real command through the production
  registry; repeat project-pinned; repeat via generated CI
  in a scratch org repo. No sidecars.

## R02 — Fresh credentials on production paths

- Kube client construction moves under the provider with the
  `auth` token source as the default factory (no nil
  default to saved kubeconfig). Exec/tunnel launching uses
  it too; the saved-kubeconfig temp-file path is removed
  or made refreshable.
- Production `main` constructs the server with the factory;
  tests cover the production constructor shape.
- Through plugin plus core: expired stored token with valid
  ambient credentials succeeds and never sends the stale
  bearer (asserted); refresh failure and restart covered.
- The order-8 expiry phase omits `MAGELIFT_KUBECONFIG`.

## R03 — Real media persistence and restore

- Lead mechanism: Magento S3-compatible remote storage
  against GCS (HMAC keys provisioned by the stack),
  configured through the env template's remote_storage
  section. Delivery is a single-purpose world-readable
  bucket: IAM rejects conditions on allUsers bindings, so
  prefix-scoped public reads are not expressible, and the
  bucket holds only public-by-design storefront assets
  under media/ (writes stay HMAC-gated). Paid downloadable
  content is out of alpha scope. Verified live or the
  mechanism changes (documented).
- CLI media operations route through the provider: new
  typed plugin ops for media export (to operator disk;
  the plugin runs locally) and import. Core drops the
  unconditional S3 client for non-AWS stacks.
- Encryption-key continuity: restore keeps the secret
  reference; proof compares crypt keys and exercises
  encrypted data.
- Proof: the review's upload/view/replace/export/restore/
  compare sequence, from a machine without AWS credentials.

## R04 — Serving-path deploy health

- Keep scheduler checks; add a bounded request through the
  serving nginx-to-PHP path (not a job) as a deploy gate.
- Search proved through Magento's effective configuration
  (engine plus host as Magento resolves them), not a
  separately supplied URL.
- `deploymentHealth` checks all serving containers' resolved
  images, or the claim is scoped to exactly what is checked.
- Tests: broken FPM/upstream fails deploy despite a healthy
  job; wrong effective search host fails despite a reachable
  server; stale rollout fails.

## R05 — True provider boundary

- Provider-specific deploy-step execution moves into the
  plugin (new typed ops); core stops decoding provider
  deploy inputs into kube types. Needed operation contracts
  go in the SDK with schemas, not Go-type-name comments.
- Provider drops root-`internal` imports; minimal helpers
  become provider-local (duplication consciously chosen
  over coupling for one provider; a versioned support
  module stays a noted future option).
- Alpha core registration is GCP plus AWS ECS Fargate;
  EKS, OVH, and Scaleway register behind
  `MAGELIFT_EXPERIMENTAL_PROVIDERS`. Cleanup/recovery
  providers stay available (recovery reads old ledgers).
- ADR 0013 reconciled with the as-built boundary; the
  leanness gate matches the ADR wording (no GCP provider
  code or GCP SDKs in the CLI; AWS/OVH/Scaleway SDKs stay
  linked by design until parity).
- Import decoupling explicitly deferred: the provider
  still imports root-internal helpers and the core still
  links deferred-provider SDKs. Removing that coupling is
  a hard prerequisite to adding the second autonomous
  provider (recorded in `aws-provider-parity`). Neither
  the core nor the provider is described as SDK-free.
- Proof: `make core-leanness` green; registry plus plugin
  suites green; no claim of full SDK independence in
  shipped docs.

## R06 — Real provider publication

- Coherent fixtures: identical manifest content in `.mod`
  and zip for every staged module.
- The provider test resolves and COMPILES the real
  provider with `GOWORK=off`: file proxy carries the
  synthetic magelift modules, upstream carries
  third-party (documented network use).
- The first require-bump lands WITH rc.2 (requires name rc.2,
  matching that tree). Bumping to rc.1 was attempted and
  reverted: the tree is already ahead of rc.1's SDK, so rc.1
  requires help nothing. After rc.2 tags: bump root plus
  provider requires to rc.2, record sums from the proxy, and
  verify `GOWORK=off` root plus provider builds. Published
  tags are never rewritten.

## R07 — Honest release channels

- Release config sets `prerelease: true`, `make_latest:
  false`, `draft: true`; a publish step flips draft off
  only after lockfiles upload plus bundle/checksum
  verification pass. Stays until the first stable release
  (documented flip).
- `/releases/latest` behavior against all-prerelease repos
  verified empirically first; the installer keeps stable
  default behavior (clean failure while no stable exists)
  with explicit `MAGELIFT_VERSION` alpha opt-in.
- Lockfiles remain uploaded post-signing; R01's
  pinned-publisher design is their authentication policy
  (documented at the upload step).

## R08 — One lifecycle authority

- The PHP lifecycle plan emits the deploy sequence
  (machine-readable); Go executes the emitted steps with
  no hardcoded duplicate. A checked-in golden plus a
  regeneration target keep them in sync; drift fails CI.
- Incompatible flows: explicitly rejected or explicitly
  scoped to preview-compatible only; multi-replica
  quiescence claims removed until automated and verified.
  Production stays outside the alpha proof.

## R09 — CI covers the modules

- CI enumerates root, SDK, and provider modules: provider
  suite, provider lint, provider vuln check, synthetic
  suite, with correct path filters and cache keys.
- Draft-to-publish promotion gates on same-commit checks
  via the checks API (scripted, unit-tested with mocked
  responses).
- Proof: a failing provider-only test blocks promotion
  (demonstrated against the gate script).

## R10 — Correct timeout policy

- Every op classified by actual effect; mutating timeouts
  require reconciliation (non-retryable) unless a tested
  idempotency/ownership mechanism says otherwise.
- All mutating ops asserted, not just Apply/Destroy.

## R11 — Crash-safe updater

- Backup becomes copy-not-move: the executable never
  leaves its path until the atomic replacement rename.
  Crash at any point leaves old or new, never missing.
- Filesystem transitions injectable for fault tests at
  each step; self-check failure still restores.
- Windows replace-running limitation documented honestly.

## R12 — Proved onboarding and pins

- Onboarding rewritten to the shipped path: installer,
  provider install, application build, deploy; ADC (not
  `gcloud auth login`) with missing-credential detection;
  correct mail recipient (test customer) with delivery
  evidence — or mail explicitly unproved for the loop.
- Recipe gains exact pins (container digests, source
  revision, effective config, scale, region) from loop
  evidence as a machine-readable fixture. Until the loop
  runs it stays labeled proposed.
- Autonomous expiry proved through the scheduled sweep
  path, not a manual call alone.

## Amendments and roadmap

- Archived orders 4-7 reports gain dated correction notes
  naming the R-items and where they were fixed. History
  is appended, never rewritten.
- ROADMAP drops stale "rewritten draft" labels, links
  archived records, and shows the correction intent plus
  the order-8 re-run.
