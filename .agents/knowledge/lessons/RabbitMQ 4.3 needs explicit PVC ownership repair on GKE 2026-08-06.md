---
type: lesson
title: RabbitMQ 4.3 needs explicit PVC ownership repair on GKE
description: The RabbitMQ 4.3 image can fail to read its Erlang cookie when a GKE PVC is mounted without the image user owning the data directory.
tags: [gcp, gke, rabbitmq, kubernetes, magento]
status: stable
generated:
  by: codex
  at: 2026-08-06
---

The first GCP 2.4.6 standard run with the pinned RabbitMQ 4.3 image pulled the
image successfully, but the broker exited with `eacces` while reading
`/var/lib/rabbitmq/.erlang.cookie`. MageLift's configuration init container
mounted and repaired `/etc/rabbitmq`, but did not mount the persistent data
volume. The GKE PVC therefore did not have the RabbitMQ image user's ownership
when the broker started.

The queue template now mounts the data PVC in the configuration init container
and repairs both the configuration and data directory ownership using the
pinned image's numeric `100:101` account. That ownership is deliberate for the
first-party image. A third-party RabbitMQ image must provide a compatible user
or an explicit provider-specific image contract before it can be certified.

For future broker image changes, certify the full startup path, not only image
pullability: PVC mount, cookie readability, readiness probe, and broker health.
