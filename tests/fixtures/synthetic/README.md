# Synthetic shop fixtures

These invented projects use example domains and placeholder IDs. The tagged
suite checks the fixtures for credentials and exercises config validation,
provider routing, PaaS imports and local Compose planning.

- `aws-preview/` and `gcp-preview/` contain valid preview configurations.
- `invalid/` contains configurations that should fail with a specific error.
- `imports/` contains sample Adobe Commerce Cloud and Upsun settings.

Run the tests from the repository root.

```sh
go test -tags synthetic ./tests/synthetic/
```
