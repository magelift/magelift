# AGENTS.md

CLI that deploys Magento Open Source / Adobe Commerce in the user's AWS or GCP
account. Not a host. The supported user path is YAML-only.

Current priority is milestone A, the GCP community alpha. Public gates
are in `docs/release-readiness.md`. AWS stays second and targeted.
Experimental providers stay at the level `docs/capability-matrix.md` and
`docs/evidence/README.md` already record. They do not block milestone A.

Always-on layer only. Load one skill from the tables below. Do not paste skill bodies,
`.agents/knowledge/`, or CONTRIBUTING. Longer rules belong in a skill.

## Never

- Copy ece-tools, Cloud Patches, Quality Patches databases, ACC CLI, or
  employer source/docs/IDs/secrets, and never name the employer. Gate: `make check-clean-room`. Ledger:
  `docs/provenance.md`.
- Call a cell certified unless `docs/capability-matrix.md` plus
  `docs/evidence/README.md` say so for that exact cell. Historical evidence
  stays as written. Milestone A does not certify every catalog intersection
  or frozen provider. EKS, OVH, Scaleway, and GKE Standard stay experimental.
- Treat `NOT_RUN`, a mock, or an untested path as a live pass. A startup
  path that is known to fail stays failed until a real run says otherwise.
- Freelance `magelift deploy`/`destroy` against a non-acceptance account.
  Live GCP uses the dedicated acceptance project. The community-alpha
  campaign teardown is recorded in `intent/community-alpha/verification.md`;
  do not recreate that campaign for docs, an install smoke, or a tag.
  AWS stays targeted: about USD 150 of announced credits, at most USD 100
  of planned test spend, USD 50 held back, and no card spend after credits
  run out. OVH, Scaleway, Cloudflare, New Relic, and Fastly stay light
  smoke and do not block the alpha. Load `magelift-certify` before a live
  mutation. Other cells still destroy on exit. No KEEP unless a retained
  debug cell records an owner, expiry, inventory, and destroy procedure.
  CLI exit is not the only cleanup for an interrupted run.
- Commit `.cursor/`, `.claude/`, `.agents/` caches, credentials, Pulumi state,
  or third-party skill packs. Do commit `.agents/knowledge/`.
- Share Pulumi components behind `if provider ==`. Topology stays in
  `internal/cloud/<provider>/` (ADR 0003, 0004).
- Put secret values in YAML, logs, or evidence. References only.
- Run local GoReleaser multi-target matrices, `make test-race-full`, or
  unbounded `go test -race ./...` as the default local loop. Do not pin
  `GOMAXPROCS` or `-p`. `GOMEMLIMIT` is 75% of available RAM
  (`scripts/go-memlimit.sh`).

## Ask first

Protected or production destroy, release tags, public publish, history
rewrite, and cloud spend outside the acceptance envelope above.

## Do

- Active work is the GCP community alpha. S0-S2 stay delivered; S4/S5
  follow pilots. Do not reopen a closed lot or add another plan.
  Historical evidence stays as written.
- Smallest correct change. Fix a capability that already exists before adding
  a second version of it. Wire CLI modules through `cmd/magelift` /
  `platform.ModuleRegistry`. `infra.RegisterTarget` alone does not ship
  `magelift deploy`.
- After clone, once: `composer install --working-dir=build`.
  Community alpha gate: `make verify-community`.
  Fast loop: `make test-fast`.
  Targeted race: `make test-race`.
  `make verify-community` also runs that targeted race.
  Package iteration: `go test ./internal/<pkg>/ -count=1`.
  `make test-fast` runs the root module, `sdk/` with `GOWORK=off`, and
  `providers/gcp/` on the workspace. Root `go test ./...` does not cover the
  nested modules. `make verify` still runs `test-race-full` and is not the
  alpha gate. `make local-gates` and live acceptance targets stay available
  for the jobs they name.
- `make php-test` uses host PHP when `dom`, `filter`, `json`, `libxml`,
  `mbstring`, `tokenizer`, and `xmlwriter` are present, otherwise the pinned
  Composer image. `lifecycle-golden-check` uses the same PHP runner.
- Do not hand-edit generated schema, CLI reference, certification coverage, or
  `agents/manifest.json`. Use `make generate`.
- Recall: read `.agents/knowledge/index.md`, grep, open only matching notes.
  Default UPDATE or SKIP. Do not file session resumes, KEEP run IDs, or facts
  the code, CBM, or Serena already answer.
- When a public contract, certification tier, or topology rule changes, update
  the ADR and the human page together. Do not rewrite historical evidence
  conclusions. Human docs and website copy go through
  humanizer, then remove-ai-marks. Humans start at `README.md`.

## Skills

Read the `SKILL.md`. Default to the contributor track under `contrib/skills/`
(never shipped). The user track under `agents/skills/` ships in the binary
and is only for dogfooding user flows or the skills acceptance run.

| When | Skill |
| --- | --- |
| Code, docs, PR, honesty | `contrib/skills/magelift-contribute` |
| New `internal/cloud/<provider>/` | `contrib/skills/magelift-provider` |
| Public extension boundary | `contrib/skills/magelift-extend` |
| Live matrix cell + evidence | `contrib/skills/magelift-certify` |
| Tag, GoReleaser, Cosign, GHCR | `contrib/skills/magelift-release` |
| `website/` + MkDocs | `contrib/skills/magelift-site` |

User track: dogfooding and skills acceptance only:

| When | Skill |
| --- | --- |
| YAML, catalog, PHP/DB/search/queue/edge | `agents/skills/magelift-configure` |
| Local Compose / `magelift local` | `agents/skills/magelift-local-runtime` |
| `magelift doctor` / workstation tools | `agents/skills/magelift-dependencies` |
| Status, logs, rollback, teardown | `agents/skills/magelift-operate` |
| ACC / Upsun import | `agents/skills/magelift-migrate` |

## Map

| Path | Role |
| --- | --- |
| `cmd/magelift` | Production CLI registration |
| `internal/cli` | Cobra; keep Pulumi SDKs out |
| `internal/platform` | Cross-provider ports |
| `internal/cloud/<p>/` | IaaS adapter + Pulumi (aws/ovh/scaleway only) |
| `providers/gcp/` | Autonomous GCP plugin (own module, net/rpc protocol) |
| `internal/external/<v>/` | SaaS edge/observability adapters (fastly/newrelic/edge/observability) |
| `internal/shared/<port>/` | Provider-neutral ports (recovery/resilience/statearchive; no SDKs) |
| `sdk` | Typed contracts |
| `build/` | Composer Magento package, not the Go tree |
| `schema/` | Generated JSON Schema |
| `docs/` | MkDocs; ADRs; short evidence pack |
| `.agents/knowledge/` | Repo OKF bundle (agents) |
| `tests/` | Floci and fixtures |
| `intent/` | Community-alpha verification record |
| `scripts/` | Acceptance harnesses (per-provider local runners) |
| `examples/` | Sample shop plus clean-room custom CLI |
