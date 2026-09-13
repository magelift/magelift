---
type: lesson
title: EKS Auto Mode stateful workloads need explicit storage and claim cleanup
description: EKS Auto Mode does not provide a suitable default StorageClass for MageLift stateful workloads, and StatefulSet claims need an explicit deletion policy for disposable cells.
tags: [aws, eks, kubernetes, storage, pvc, acceptance, cleanup]
status: stable
generated:
  by: codex
  at: 2026-08-07
---

# Observation

The first EKS RabbitMQ attempt failed because no StorageClass existed for the
Auto Mode cluster. After MageLift created an encrypted gp3 StorageClass using
the EBS CSI Auto Mode provisioner and `WaitForFirstConsumer`, RabbitMQ and
OpenSearch claims bound successfully.

The first warm queue transition also showed that deleting a StatefulSet does
not automatically delete its volume claim. The retained claim kept an EBS
volume after RabbitMQ had been removed from the graph.

# Rule

Create an explicit cluster-owned StorageClass for EKS Auto Mode stateful
services. Set StatefulSet claim retention to delete claims when the StatefulSet
is deleted, while retaining claims during replica scale-down. Use a deleting
reclaim policy for disposable acceptance storage and verify both the PVC and PV
after each capability removal and final teardown.

# MageLift

The shared Kubernetes RabbitMQ and OpenSearch components now render the claim
retention policy. The live EKS stack was updated once before removing
OpenSearch, and the search PVC and PV disappeared during the warm transition.
The earlier RabbitMQ claim was removed explicitly because it was created before
the policy change.
