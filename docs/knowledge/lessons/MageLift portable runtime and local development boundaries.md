---
type: lesson
title: MageLift portable runtime and local development boundaries
description: MageLift v1 certifies AWS ECS Fargate and nginx with PHP-FPM.
tags:
- architecture
- local-development
- containers
- extensions
generated:
  at: '2026-07-24'
---

MageLift v1 certifies AWS ECS Fargate and nginx with PHP-FPM. The versioned Go SDK keeps Target and CapabilityProvider provider-neutral while provider topology stays outside magelift.yaml. FrankenPHP classic is a portable candidate runtime gated by the same acceptance suite; worker mode stays absent until Magento compatibility is proven. Local development is a separate Docker Compose execution context with local capability implementations, no Pulumi or AWS credentials. Debian Trixie is the certified container base; Nix may later pin contributor host tools, while Alpine and NixOS images remain future evaluations.
