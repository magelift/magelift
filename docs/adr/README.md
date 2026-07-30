# Architecture decision records

ADRs capture decisions that materially constrain public contracts, infrastructure,
security, or maintenance. Copy the next numeric filename, state the context, decision,
consequences, alternatives, and provenance, then submit it with the implementation.

Accepted ADRs are immutable except for status and links. A new ADR supersedes an old
decision and explains migration implications.

## Accepted decisions

- [ADR 0001](0001-aws-ecs-fargate-v1.md): AWS ECS Fargate is the only certified v1 runtime.
- [ADR 0002](0002-provider-runtime-extension-boundary.md): Provider topology stays outside portable contracts.
- [ADR 0003](0003-external-final-artifact-manifest.md): The finalized artifact manifest is external to the image.
- [ADR 0004](0004-provider-package-layout.md): Provider-specific packages stay behind stable extension boundaries.
- [ADR 0005](0005-local-development-compose.md): Local development uses a generated Docker Compose execution context.
- [ADR 0006](0006-github-oidc-role-separation.md): GitHub CI and state recovery use separate trusted roles.
- [ADR 0007](0007-multi-provider-community-targets.md): Multi-provider roadmap and community targets.
- [ADR 0008](0008-ports-and-adapters-multi-provider.md): Ports and adapters for multi-provider stacks.
- [ADR 0009](0009-day2-magento-ports.md): Day-2 Magento ports on StackModule.
- [ADR 0010](0010-database-dump-seed.md): Database dump seed for new/ephemeral envs (attach-out-of-scope consequence superseded by Phase 8 / ATTACH-02 — [brownfield-attach](../brownfield-attach.md)).
