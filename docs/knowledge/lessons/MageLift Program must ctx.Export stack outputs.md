---
type: lesson
title: MageLift Program must ctx.Export stack outputs
description: RegisterResourceOutputs on magelift:aws:Stack does not populate Pulumi Automation stack.Outputs().
tags:
- pulumi
- outputs
- deploy
- e2e
status: stable
generated:
  at: '2026-07-24'
---

RegisterResourceOutputs on magelift:aws:Stack does not populate Pulumi Automation stack.Outputs(). Program() must ctx.Export the same keys (clusterName, serviceName, deployTaskDefinitionArn, securityGroupId, privateSubnetIds, applicationURL, mediaURL) or deploy fails after create with Pulumi output clusterName is required even when resources were created. Keep Component.Outputs() shared by RegisterResourceOutputs and Program exports.
