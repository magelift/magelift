## 1. Planning close-out

- [x] 1.1 Write complete-operator-path proposal, specs, and design under `openspec/changes/complete-operator-path/` and verify `openspec status --change complete-operator-path` lists them.
- [x] 1.2 Replace `openspec/BACKLOG.md` so the single active objective is `magelift local` iso-prod plus Magento runtime config plus an integrated storefront example, and verify the file names that objective and lists P2 cells without starting them.

## 2. P0 — laptop path (nginx, local, runtime YAML)

- [x] 2.1 Rename `magelift dev` to `magelift local` in Cobra, tests, user skills, and docs; verify `magelift --help` lists `local` and not `dev`, and `go test ./internal/cli/ -count=1` passes.
- [x] 2.2 Restrict `application.webRuntime` to `nginx-fpm` in schema and config validation; verify FrankenPHP and Apache values fail `config.Load` before mutate.
- [x] 2.3 Add Magento runtime overlay YAML (URLs, cookies, frontName, CORS, consumers, secret-referenced CONFIG/MAGENTO_DC); verify unknown secret literals fail and `go test ./internal/config/ -count=1` passes.
- [x] 2.4 Read Quality Patch IDs only from `magelift.yaml` after import maps `.magento.env.yaml`; verify import and build tests in `build/tests` and `internal/paasimport`.
- [x] 2.5 Make `magelift local` iso-prod of the selected env (named substitutes, split cache when durable, Mailpit, no fake CloudFront); verify `go test ./internal/localdev/ -count=1` and doctor does not silently install software.
- [x] 2.6 Add an integrated PHP storefront example YAML under `examples/` and point `init` at it; verify `magelift config validate` on that file.
- [x] 2.7 Fail Adobe-unsupported combinations closed unless `compatibility.allowUnsupported`; warn and proceed for MageLift-experimental cells; verify config tests name Adobe vs MageLift vs provider and experimental does not require the hatch.

## 3. P1 — day-2 and production defaults

- [x] 3.1 Add `magelift audit` (or `evidence --controls`) distinct from the change journal; verify no secret values in output tests.
- [x] 3.2 Add Magelift edge purge/invalidate; verify typed unsupported when the target cannot purge.
- [x] 3.3 Drive Magento-safe WAF from configured `frontName`; verify tests reject unmodified CRS as Magento-protected.
- [x] 3.4 Encode split cache/session and refuse in-place RabbitMQ 1↔quorum; verify AWS/local tests.
- [x] 3.5 Hide Pulumi from doctor next-steps on the Magento happy path; verify deploy docs and skills never tell the user to run `pulumi up`.
- [x] 3.6 Add optional region residency fail-closed for DB/media/backups; verify config tests.

## 4. P2 — catalog honesty, not certified cells

- [x] 4.1 Document dense AWS profiles as experimental in catalog and examples; verify `config validate` warns and exits 0, does not require `allowUnsupported`, and does not claim certified without matrix plus evidence.
- [x] 4.2 Add Magento-module SQS/Pub/Sub integration (not `queueMode`); verify fail-closed without the locked Composer package.
- [x] 4.3 Specify brownfield Magento cutover (dump, media, crypt ref) without owning attached VPC/DB; verify destroy tests.
- [x] 4.4 Keep signed provider download and community catalog specified; do not wire Magento Dial in this slice. Verify existing `providerhost` tests still pass.

## 5. Verify

- [x] 5.1 Run `make generate` after schema/CLI changes and verify generated CLI reference and schema match `local` and nginx-only.
- [x] 5.2 Run `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cli/ ./internal/config/ ./internal/localdev/ -count=1` and record pass.
