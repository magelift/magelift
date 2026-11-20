---
name: magelift-dependencies
description: Diagnose missing MageLift tools, Docker connectivity, and cloud authentication on developer machines and CI runners.
version: 1.0.0
---

# Check MageLift dependencies

Run `magelift doctor --local` before starting a shop locally. It evaluates the
project and checks PHP and Docker without contacting the cloud. Run
`magelift doctor` after cloud login to include credential checks.

Install tools only when the user requests it. Review the proposed package
manager command before running `magelift doctor --install-dependencies`.
If Docker is installed but unreachable, check its daemon and context first.
Cloud stack commands need Pulumi on `PATH`. Azure AKS may need `kubelogin`
when the cluster uses Entra authentication.

For a report you can review before sharing, run:

```sh
magelift doctor --local --export doctor-metadata.json
```

The new file has mode 0600 and contains check statuses, tool requirements,
and target categories. Keep raw command output private because it can
include project and account details.

Cloud commands load the verified provider for the selected target. Run
`magelift providers install` online to prepare for offline use or repair a
cache entry. Keep the project pin and check any digest or signature error
against it. [Install and verify providers](https://magelift.dev/docs/install/#providers)
