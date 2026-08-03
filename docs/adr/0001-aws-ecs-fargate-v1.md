# ADR 0001: AWS ECS Fargate is the first certified v1 runtime

> **Status note (2026-08):** The title used to say "only." GCP GKE Autopilot is now
> also **certified** (ADR 0007). This ADR still records why Fargate was the first path.

- Status: Accepted
- Date: 2026-07-17

## Context

Magento operations already span application build, networking, data services,
deployment safety, recovery, security, and cost. Implementing several providers before
one complete path is proven would dilute testing and leak lowest-common-denominator
abstractions into the user contract.

## Decision

MageLift v1 targets AWS and certifies ECS Fargate. Stable application lifecycle,
capability, build, and target interfaces preserve future extension points.
Multi-cloud marketing waits until a second provider passes the same suite.
GCP GKE Autopilot later met that bar (ADR 0007).

Pulumi owns durable AWS resources. The CLI uses AWS APIs directly only where Pulumi's
backend cannot bootstrap itself and for bounded deployment orchestration.

## Consequences

The project can test deeper AWS failure and recovery behavior and publish realistic
presets. Users needing Kubernetes or another cloud must wait or maintain an unsupported
extension. Provider differences remain explicit rather than entering the core YAML.

## Provenance

This is an original project decision informed by public Pulumi Automation API and AWS
ECS documentation. No third-party source code was used.
