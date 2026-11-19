# Go lint and CI coverage

`make lint` checks the CLI, SDK and provider modules. MageLift enables `govet`, `staticcheck`, `ineffassign`, `misspell`, `unused`, `errcheck`, and selected rules from `revive` and `gocritic`. The repository’s golangci-lint configuration lists the active rules. [Lint configuration](https://github.com/magelift/magelift/blob/main/.golangci.yml)

Run `make fmt-check` to check formatting with the Go toolchain selected by the repository. A system installation of gofmt may format composite literals differently.

## GitHub Actions

| Job | Coverage |
| --- | --- |
| `lint`, `go-verify` | Root Go module, including CLI, shared packages and embedded adapters |
| `sdk-verify` | Public SDK module |
| `provider-aws`, `provider-gcp`, `provider-azure` | A separate job for each provider module, including vet, race tests, lint, dependency and vulnerability checks |
| `floci-aws`, `floci-gcp`, `floci-azure` | Cloud-specific API calls against local emulators |

The Go workspace makes local edits visible across modules. Go still treats each module as a separate test and lint target. Root lint coverage has its own test in `internal/lintcoverage`. To run every hosted check regardless of changed paths, use the CI workflow’s `all` input. [CI workflow](https://github.com/magelift/magelift/blob/main/.github/workflows/ci.yml)

Go checks set `GOMEMLIMIT` from available memory. Run module checks separately when diagnosing memory pressure. If a rule produces a false positive, scope the suppression to the affected code and explain why it is safe.
