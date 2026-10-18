# AGENTS.md

Always-on layer. Load one skill from the table. Do not paste skill bodies,
`.agents/knowledge/`, or CONTRIBUTING into the session.

## Never

- Copy ece-tools, Cloud Patches, Quality Patches databases, ACC CLI, or
  employer source, docs, IDs, or secrets, and never name the employer.
  Gate: `make check-clean-room`. Ledger: `docs/provenance.md`.
- Call a cell certified unless `docs/capability-matrix.md` and
  `docs/evidence/README.md` say so for that cell. Leave historical evidence
  as written. A live pass is a real run of that path. Record `NOT_RUN`, a
  mock, or a known-failing startup path as a failure.
- Run `magelift deploy` or `destroy` outside the acceptance account. Live
  GCP uses the dedicated acceptance project. The community-alpha campaign
  was torn down; leave it down for docs, an install smoke, or a tag. AWS
  stays inside about USD 150 of announced credits, at most USD 100 planned,
  USD 50 held back, and no card spend after credits run out. OVH, Scaleway,
  Cloudflare, New Relic, and Fastly stay light smoke. Load `magelift-certify`
  before a live mutation. Destroy on exit. A KEEP cell needs an owner,
  expiry, inventory, and destroy procedure. An interrupted run still needs
  its own cleanup.
- Hand-edit generated schema, the CLI reference, certification coverage, or
  `agents/manifest.json`. Use `make generate`.
- File session resumes, KEEP run IDs, or facts the code already answers
  into `.agents/knowledge/`. Commit that bundle. Leave `.cursor/`,
  `.claude/`, other `.agents/` caches, credentials, Pulumi state, and
  third-party skill packs untracked.

## Ask first

Protected or production destroy, release tags, public publish, history
rewrite, and cloud spend outside the envelope above.

## Skills

Contributor skills under `contrib/skills/` stay out of the binary. User
skills under `agents/skills/` ship in the binary for dogfooding and the
skills acceptance run. Layout and the local gate are in `CONTRIBUTING.md`.

| When | Skill |
| --- | --- |
| Code, docs, PR, honesty | `contrib/skills/magelift-contribute` |
| New `internal/cloud/<provider>/` | `contrib/skills/magelift-provider` |
| Public extension boundary | `contrib/skills/magelift-extend` |
| Live matrix cell and evidence | `contrib/skills/magelift-certify` |
| Tag, GoReleaser, Cosign, GHCR | `contrib/skills/magelift-release` |
| `website/` and MkDocs | `contrib/skills/magelift-site` |
| YAML, catalog, PHP/DB/search/queue/edge | `agents/skills/magelift-configure` |
| Local Compose / `magelift local` | `agents/skills/magelift-local-runtime` |
| `magelift doctor` / workstation tools | `agents/skills/magelift-dependencies` |
| Status, logs, rollback, teardown | `agents/skills/magelift-operate` |
| ACC / Upsun import | `agents/skills/magelift-migrate` |
