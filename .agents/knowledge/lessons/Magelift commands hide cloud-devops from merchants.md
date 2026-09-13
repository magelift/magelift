---
type: lesson
title: Magelift commands hide cloud-devops from merchants
description: The merchant path is doctor, bootstrap, deploy, health, destroy. Do not send operators to Pulumi, Cosign, WIF, kubeconfig, or GitHub bootstrap flags.
tags: [cli, ux, yaml, bootstrap, merchant]
status: stable
generated:
  by: cursor-grok-4.6/darwin
  at: 2026-08-19
---

# Magelift commands hide cloud-devops from merchants

Merchants have little or no cloud-devops knowledge. The supported laptop path is
Magelift commands only:

```sh
magelift doctor
magelift bootstrap --env preview
magelift deploy --env preview --yes
magelift health --mode runtime
magelift destroy --env preview --yes
```

AWS bootstrap also needs `--access-log-bucket` pointing at an existing log
bucket Magelift does not create. GitHub `--github-owner` / `--github-repo` are
optional and only for Actions CI.

Magelift derives stack state from `magelift.yaml` after bootstrap. Generated
GitHub workflows must not require `PULUMI_BACKEND_URL` or Cosign identity
flags. `PULUMI_BACKEND_URL` remains an advanced override. Signing and promote
use the current `aws` / `gcloud` or Actions login.

Errors, `doctor`, and `bootstrap` output must name the next Magelift command
and `docs/getting-started.md`. `doctor` prints `magelift bootstrap --env …`;
`bootstrap` prints `magelift deploy --env … --yes`. Do not mention Pulumi,
Cosign, impersonation, kubeconfig, or Workload Identity Federation on that path.

* Relates to: [Cosign signing identity is Sigstore OIDC not the Magento cloud provider](Cosign%20signing%20identity%20is%20Sigstore%20OIDC%20not%20the%20Magento%20cloud%20provider.md)
