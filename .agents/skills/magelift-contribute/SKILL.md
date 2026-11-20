---
name: magelift-contribute
description: Change or review MageLift code, examples, and documentation and select the contributor checks that exercise the change.
metadata:
  version: "1.0.0"
---

# Contribute to MageLift

Read [AGENTS.md](../../../AGENTS.md) and [CONTRIBUTING.md](../../../CONTRIBUTING.md).
Write setup instructions for a Magento PHP developer using their own cloud
account. Review prose with the current Humanizer skill. Keep commands and
technical claims accurate.

Run `make verify-community` before submitting. Use `make local-gates` for
provider changes and `make ci-act-project` for generated shop workflows.
After changing the project schema, a command, or a bundled skill, run
`make generate` and review its diff. Run `make docs` for guide changes.

Report the method, configuration, and result for each test. Preserve dated
run records. Use a synthetic example when a bug comes from a private shop.
