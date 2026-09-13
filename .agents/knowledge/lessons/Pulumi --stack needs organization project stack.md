---
type: lesson
title: Pulumi --stack needs organization/project/stack
description: Magelift outputs print an unqualified stack name; pulumi stack output --stack rejects it and needs org/project/stack.
tags: [pulumi, aws, eks, acceptance, kubeconfig]
status: stable
generated:
  by: cursor-grok-4.6/darwin
  at: 2026-08-14
sources:
  - id: pulumi-stack-output
    resource: https://www.pulumi.com/docs/iac/cli/commands/pulumi_stack_output/
    title: pulumi stack output
  - id: pulumi-automation-inline
    resource: https://www.pulumi.com/docs/iac/packages-and-automation/automation-api/concepts/
    title: Automation API concepts
---

# Pulumi --stack needs organization/project/stack

`pulumi stack output --stack NAME` requires a fully qualified stack
(`organization/project/stack`) when the backend has an organization. Magelift
`outputs` JSON `.stack` is the unqualified Automation API name
(`project-environment-aws-eks`).

Magento EKS Auto Mode `20260813av` created 85 resources, then seed import
failed: `pulumi stack output kubeconfig --stack awsav-preview-aws-eks`
returned "if you're using the --stack flag, pass the fully qualified name
(organization/project/stack)". EXIT destroyed 85/85.

The GCP harness already resolves this with `pulumi_fq_stack_ref` (`pulumi
stack ls`, else `whoami`/`organization` + project `magelift`). AWS EKS seed
must do the same. Do not pass Magelift `.stack` to `--stack` unchanged.

Inline stacks are created as project `magelift`
(`NewInlineStackWithBackend(..., stackName, "magelift", backendURL)`), so
the DIY fallback is `organization/magelift/<stack>`.
