# AGENTS.md

CLI that deploys Magento Open Source / Adobe Commerce in the user's AWS or GCP
account. Not a host. The supported user path is YAML-only.

Always-on layer only. Load one skill from the tables below. Do not paste skill bodies,
`.agents/knowledge/`, or CONTRIBUTING. Longer rules belong in a skill.

## Never

- Copy ece-tools, Cloud Patches, Quality Patches databases, ACC CLI, or
  employer source/docs/IDs/secrets, and never name the employer. Gate: `make check-clean-room`. Ledger:
  `docs/provenance.md`.
- Call a cell certified unless `docs/capability-matrix.md` plus
  `docs/evidence/README.md` say so. Today: AWS ECS Fargate and GCP GKE Autopilot.
  EKS, OVH, Scaleway, and GKE Standard stay experimental.
- Freelance `magelift deploy`/`destroy` against a non-acceptance account.
  Prefer `make local-gates`. Live GCP is thorough E2E; AWS, OVH, Scaleway,
  Cloudflare, New Relic, and Fastly are light smoke. Load
  `magelift-certify`; destroy on exit; no KEEP unless a retained debug cell.
- Commit `.cursor/`, `.claude/`, `.agents/` caches, credentials, Pulumi state,
  or third-party skill packs. Do commit `.agents/knowledge/`.
- Share Pulumi components behind `if provider ==`. Topology stays in
  `internal/cloud/<provider>/` (ADR 0003, 0004).
- Put secret values in YAML, logs, or evidence. References only.
- Run local GoReleaser multi-target matrices or unbounded `go test -race ./...`.
  Do not pin `GOMAXPROCS` or `-p`. `GOMEMLIMIT` is 75% of available RAM
  (`scripts/go-memlimit.sh`).

## Ask first: protected/production destroy, release tags, history rewrite.

## Do

- Work comes from `intent/ROADMAP.md`, one intent at a time: accept, spec,
  plan, implement, verify, archive. No code before the plan is approved.
- Smallest correct change. Wire CLI modules through `cmd/magelift` /
  `platform.ModuleRegistry`. `infra.RegisterTarget` alone does not ship
  `magelift deploy`.
- After clone, once: `composer install --working-dir=build`. Gate: `make verify`.
  Iterate: `go test ./internal/<pkg>/ -count=1`.
- Do not hand-edit generated schema, CLI reference, certification coverage, or
  `agents/manifest.json`. Use `make generate`.
- Recall: read `.agents/knowledge/index.md`, grep, open only matching notes.
  Default UPDATE or SKIP. Do not file session resumes, KEEP run IDs, or facts
  the code, CBM, or Serena already answer.
- When a public contract, certification tier, or topology rule changes, update
  the ADR and the human page together. Human docs and website copy go through
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

User track — dogfooding and skills acceptance only:

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
| `intent/` | Ordered work: ROADMAP plus one dir per change |
| `scripts/` | Acceptance harnesses (per-provider local runners) |
| `examples/` | Sample shop plus clean-room custom CLI |
