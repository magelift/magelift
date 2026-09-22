---
status: accepted
slug: alpha-development-loop
---

# Intent: alpha development loop

## Problem

MageLift currently discovers ordinary Magento and integration defects through
published release candidates. The clean-machine proof combines distribution,
artifact construction, GCP deployment, Magento behavior, provider precedence,
generated CI, and cleanup. A failure such as the rc.22 Magento HTTP 500 forces
another release cycle before the product can be debugged again.

The same run left provider subprocesses resident and retained live GCP
resources. The project cannot reach alpha efficiently while its shortest
realistic feedback loop is a public release.

## Evidence

[The architecture report](../report.md) records the rc.22 state observed on
2026-09-22: published installation, provider resolution, bootstrap, image
construction, infrastructure creation, and workload startup completed before
the serving path returned HTTP 500. No deploy.done marker existed, multiple
provider children remained, and the proof VM was still running.

[The capability matrix](../../docs/capability-matrix.md) contains earlier live
GKE Autopilot Magento evidence, so the current failure does not establish that
the GCP topology is fundamentally unworkable.

## Proposed outcome

A maintainer can build a commit-addressed CLI, provider, and Magento artifact,
run the exact production provider boundary in the dedicated GCP acceptance
project, record distribution/artifact/product results independently, inspect a
failure, and repeat without creating a semantic-version release.

The rc.22 HTTP 500 has an evidence-backed root cause and regression test.
Provider processes terminate on success, error, and cancellation. All retained
rc.22 resources are destroyed after evidence capture and residual inventory is
empty.

## Affected users and systems

MageLift maintainers and contributors; release workflows; acceptance scripts;
the GCP acceptance project; provider process lifecycle; evidence records; the
reference Magento shop.

## Constraints

- Use the existing CLI, GCP provider protocol, artifact contracts, and GKE
  Autopilot topology.
- Canary artifacts are immutable and tied to the commit that produced them.
- Live work uses the dedicated acceptance project, the certification skill,
  destroy on exit, and no KEEP by default.
- The failure cause is determined from logs and runtime evidence, not guessed.
- No public release tag is created by this intent.
- No capability status changes without matching matrix and evidence updates.
- Secrets never enter logs or evidence.

## Out of scope

- Automatic provider installation for end users.
- Publishing the long-lived builder/runtime image catalog.
- The public alpha release.
- AWS provider extraction.
- New runtimes, providers, service combinations, edge vendors, or telemetry
  vendors.

## Open questions

- Which existing CI storage should hold commit-addressed canary artifacts while
  preserving the same verification path as releases? Decide in the spec; do not
  create a second trust implementation.
