---
name: magelift-contribute
description: Make and review MageLift changes, run local checks and describe what each test covered.
version: 1.0.0
---

# Contributing to MageLift

Start with [AGENTS.md](../../../AGENTS.md) for setup and checks. [CONTRIBUTING.md](../../../CONTRIBUTING.md) covers the PR workflow.

- Keep project configuration in `magelift.php`. Update the matching shop skill in the same change when CLI behavior or configuration changes.
- Keep provider resource graphs in their adapters. A new first-party target needs a stack module in the CLI registry. Read the [provider guide](../../../docs/adding-a-provider.md) before wiring one.
- Record changes to public contracts in the relevant ADR and guide. Use [Humanizer](https://github.com/blader/humanizer) for human-facing copy.
- In a PR, describe the change, the checks you ran and any behavior you could not exercise. Use a synthetic fixture when an example comes from a private shop.
- Report Go, Pulumi mock, Floci, act and live results by name. Include the configuration and date for a live result. The [capability matrix](../../../docs/capability-matrix.md) links to the historical evidence.
