---
name: magelift-operate
description: >-
  Operate a MageLift environment after deployment. Use for status, health,
  logs, safe Magento commands, release promotion, rollback, and teardown.
version: 1.0.0
---

# Operate MageLift

Use this skill when the infrastructure already exists and you need to inspect
or change the selected environment.

## Working rules

- Select the environment explicitly with `--env` in automation.
- Run `magelift health --mode config` before a live operation.
- Use `magelift health --mode runtime` after deploy, rollback, or a provider change.
- Treat `outputs` as data. Do not paste secrets into tickets or agent prompts.
- Use `destroy --yes` only for the exact environment you intend to remove.
- `exec --service deploy` is rejected by design; read deploy logs instead.
  Expired previews refuse deploy but always allow destroy.
- Production deploys need `--ack-maintenance-drain` plus the incompatible-schema
  runbook in `docs/operations.md`; deploy success means the intended rollout
  plus a passing Magento probe, never scheduler settlement alone.
- If cleanup fails, keep the evidence and inspect resources by the MageLift run prefix.

## Useful commands

Laptop Magento path:

```sh
magelift --env staging doctor
magelift --env staging bootstrap
magelift --env staging deploy --yes
magelift --env staging health --mode runtime
magelift --env staging logs --service web
magelift --env staging exec --service web -- php -v
magelift --env staging cost --live
magelift --env staging cleanup plan --dir .magelift/cleanup
magelift --env staging destroy --yes
```

AWS bootstrap also needs `--access-log-bucket`. `sign` and `promote` use the
current cloud or CI login; omit Cosign flags.

```sh
magelift sign --digest IMAGE@sha256:...
magelift --env staging promote --digest IMAGE@sha256:...
magelift --env staging rollback
```

## Report

Record the command, environment, image digest, result, and any resources left
after teardown. A green CLI exit code is not proof that an external DNS record
or manually created resource was removed.
