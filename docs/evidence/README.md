# Acceptance evidence (committed samples)

Local harness runs write full matrices under gitignored `.magelift/`. The files
here are **committed samples** so release claims stay auditable without private
scratch directories.

| File | What it proves |
| --- | --- |
| [aws-matrix-results-sample-2026-07-29.md](aws-matrix-results-sample-2026-07-29.md) | Free-tier AWS queue-mode cells PASS |
| [aws-brownfield-adopt-2026-08-02.md](aws-brownfield-adopt-2026-08-02.md) | VPC+RDS adopt: create children, destroy children, adopted resources intact |
| [gcp-matrix-results-2026-08-02.md](gcp-matrix-results-2026-08-02.md) | GCP harness cell rows (final PASS per required cell) |
| [gcp-certified-pass-2026-08-02.md](gcp-certified-pass-2026-08-02.md) | GCP create-once certify narrative + teardown |

Re-run acceptance with `make aws-acceptance-local` / `make gcp-acceptance-local`
and refresh these samples when claims change. See [release-readiness.md](../release-readiness.md).
