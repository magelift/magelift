---
type: lesson
title: Pulumi AWS Go standalone security group rule names
description: In pulumi-aws Go v7.37.0, the Terraform-style VpcSecurityGroupIngressRuleArgs and VpcSecurityGroupEgressRuleArgs
  names are not exported from aws/ec2.
tags:
- pulumi
- aws
- security-groups
- failed-attempt
status: stable
generated:
  at: '2026-07-24'
---

In pulumi-aws Go v7.37.0, the Terraform-style VpcSecurityGroupIngressRuleArgs and VpcSecurityGroupEgressRuleArgs names are not exported from aws/ec2. Query the installed package for the generated Go names before implementation.
