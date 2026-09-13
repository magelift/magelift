---
type: lesson
title: Kubernetes nginx-fpm needs an HTTP sidecar and readiness probe
description: A PHP-FPM-only pod can look alive while the Kubernetes Service has no HTTP listener and deployment health is only a false positive.
tags: [kubernetes, runtime, nginx, php-fpm, gcp, ovh, scaleway]
status: stable
---

# Rule

For the shared nginx-fpm image, run PHP-FPM and nginx as two containers in the same Kubernetes pod. Put Magento environment and Secret references on PHP-FPM only, expose port 8080 from nginx, and make `/health` the nginx readiness probe.

# Why

The image defaults to PHP-FPM on port 9000. The first Scaleway smoke created the pod but observed zero ready replicas during node startup; the existing Kubernetes graph also had no HTTP sidecar or readiness probe. GCP's earlier deployment count therefore proved process readiness, not that the configured Service could serve HTTP.

# MageLift

The provider-neutral Kubernetes helper now matches the AWS ECS nginx-fpm shape for GKE, MKS, and Kapsule. Live acceptance also retries runtime health while the node pool and readiness probe settle, with a bounded timeout.
