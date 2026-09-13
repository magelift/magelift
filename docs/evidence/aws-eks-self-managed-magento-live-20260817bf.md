# AWS EKS self-managed Magento 2.4.9 live baseline — 2026-08-17 (`20260817bf`)

This is a bounded PASS for a fresh self-managed EKS create, AL2023 node
registration, Kubernetes system-plane readiness, EBS CSI volume
write/read/delete, Magento seed/deploy/runtime health, direct ELB HTTP, and
exact teardown. It is a baseline cell, not a resilience or disaster-recovery
claim. Node interruption, node loss, zone loss, regional recovery, custom
domain HTTPS, CloudFront/WAF, and backup/restore remain untested here.

## Scope and admission

| Field | Value |
| --- | --- |
| Account | `<aws-account>` |
| Region | `eu-west-3` |
| Run ID | `20260817bf` |
| Project tag | `awsbf` |
| Acceptance owner | `magelift-acceptance-awsbf-preview` |
| Profile | `preview` |
| Runtime | `eks` (`computeMode:self-managed`) |
| Magento | Open Source `2.4.9`, headless |
| Kubernetes | `1.36` / EKS `v1.36.2-eks-254016e` |
| AMI | `ami-070bfbfeba67a8054` (`Amazon Linux 2023.12.20260803`) |
| Node shape | `m6i.large`, min/desired/max `2/2/2` |
| Availability zones | `eu-west-3b` and `eu-west-3c` |
| Network | `fck-nat`, VPC CIDR `10.103.0.0/16`, service CIDR `172.20.0.0/16` |
| Database/cache | RDS MySQL `8.4.10`, ElastiCache Valkey `9.0`, queue `database`, search disabled |
| Observability | EKS CloudWatch Observability add-on, logs and metrics |
| Artifact | Signed immutable ECR digest `sha256:8588b13fc390bc2b733ec3d03a0501ecc82a6ebad7fcca7758c933977fdb2be4` |
| Catalog | `scripts/acceptance/cells-aws-eks-architecture-self-managed.txt` |

The corrected run created 90 Pulumi resources. The run-specific prerequisite
secrets, regional ACM certificate, CloudFront-region ACM certificate, DNS
validation records, ECR repository, and Pulumi state bucket were all owned by
the exact project/run markers.

## Cold create and self-managed node boot

The launch template was `lt-00ed497a0ed24888f`, version `1`, and the Auto
Scaling group was `awsbf-preview-runtime-self-managed-nodes`. Both instances
were `InService` and healthy:

| Instance | Availability zone | Private address | Kubernetes node | Result |
| --- | --- | --- | --- | --- |
| `i-<redacted-c>` | `eu-west-3c` | `10.103.67.67` | `ip-10-103-67-67.eu-west-3.compute.internal` | `Ready` |
| `i-<redacted-b>` | `eu-west-3b` | `10.103.20.121` | `ip-10-103-20-121.eu-west-3.compute.internal` | `Ready` |

The live launch-template user data decoded to the AWS MIME multipart shape:

```text
MIME-Version: 1.0
Content-Type: multipart/mixed; boundary="//"
Content-Type: application/node.eks.aws
apiVersion: node.eks.aws/v1alpha1
kind: NodeConfig
Content-Type: text/x-shellscript
```

The `NodeConfig` contained the run's cluster name, EKS API endpoint, cluster
CA, and service CIDR. The shell part is only a legacy-AMI fallback and exits
when `nodeadm` or `/etc/eks/nodeadm.d` is present. This is the correction for
the failed `20260817be` cell, where AL2023 `nodeadm-config` attempted to parse
raw shell text as YAML. The renderer now follows the AWS [node bootstrapping
guide](https://docs.aws.amazon.com/eks/latest/eksctl/node-bootstrapping.html)
and the [AL2023 EKS user-data contract](https://docs.aws.amazon.com/eks/latest/userguide/al2023.html).

Both nodes reported:

```text
STATUS=Ready
OS=Amazon Linux 2023.12.20260803
KERNEL=6.18.39-79.141.amzn2023.x86_64
CONTAINER_RUNTIME=containerd://2.2.5+unknown
```

There were no `nodeadm` parse errors, no pending node-registration state, and
no node restarts during the observed baseline.

## Kubernetes system plane, IAM, and storage

The cluster was `ACTIVE`; `aws-node`, CoreDNS, and kube-proxy were all ready
on both nodes with zero restarts. The EBS CSI controller ran two `6/6`
replicas and the EBS CSI node daemonset ran `3/3` containers on each node,
also with zero restarts.

The live EKS add-ons were:

| Add-on | Version | Status |
| --- | --- | --- |
| `amazon-cloudwatch-observability` | `v6.5.0-eksbuild.1` | `ACTIVE` |
| `aws-ebs-csi-driver` | `v1.63.1-eksbuild.1` | `ACTIVE` |

The EBS add-on used
`arn:aws:iam::<aws-account>:role/awsbf-preview-runtime-ebs-csi-role-d929328`.
Its trust policy was restricted to the cluster OIDC provider,
audience `sts.amazonaws.com`, and subject
`system:serviceaccount:kube-system:ebs-csi-controller-sa`; its only attached
policy was the AWS-managed `AmazonEBSCSIDriverPolicy`. This proves the
run-scoped IRSA identity path used by the add-on; it does not claim a
provider-wide identity policy.

The disposable storage cell used StorageClass
`awsbf-preview-runtime-ebs` and a 1 GiB RWO PVC. The first probe manifest
created and attached an EBS volume but compared a marker containing a literal
escaped newline, so its shell assertion failed. The corrected probe wrote and
read the exact marker `awsbf-ebs-marker-20260817bf` and completed `1/1`:

```text
2f1700739e7bee0a7231c3d4896bd24816914e37c2bfcdbefdfa77ce5b938a35  /data/marker
awsbf-ebs-marker-20260817bf
```

The corrected probe's volume was deleted by the StorageClass `Delete`
reclaim policy before the stack teardown. The probe assertion correction is
not an infrastructure failure and is retained as a tooling deviation.

## Magento application evidence

The database-grant Job completed `1/1`. Seed import completed against
`database=magento` with the sanitized definer-free Magento dump:

```text
{"environment":"preview","seedDump":"...magento-249-sanitized-definer-free.sql.gz","seedDumpStatus":"imported"}
AWS EKS seed import completed database=magento
```

The web Deployment was `1/1` with both `php-fpm` and `web` containers, and the
cron Deployment was `1/1`. The final pods used the exact signed immutable
image digest recorded above. Runtime health returned `healthy` with one ready
web replica of one desired replica. The acceptance matrix passed:

| Cell | Result | Duration |
| --- | --- | --- |
| `computeMode:self-managed` | PASS | `1375s` |
| `queueMode:database` | PASS | `15s` |
| `searchMode:disabled` | PASS | `15s` |

The Kubernetes LoadBalancer Service exposed
`aea0846cc49cd4d3c853f8df8e9034e9-1542054818.eu-west-3.elb.amazonaws.com`.
After the run-owned seeded-database mutation set both Magento secure and
unsecure `base_url` values to that direct ELB URL and flushed the cache, an
HTTP GET returned `200 OK`, the rendered title was `Home page`, and the body
contained 48 references to the same ELB hostname. This is direct origin
HTTP proof after an explicit acceptance mutation; it is not custom-domain
TLS or edge proof.

The final image was signed with the configured keyless identity and verified
against the Google OIDC issuer before deployment. The first signing command
omitted `--include-email` from the identity-token request and Fulcio returned
HTTP 400; the corrected identity-token command passed before the live run.

## Cleanup and independent audit

Pulumi destroy removed all 90 recorded resources in `16m1s`, followed by
exact removal of stack
`organization/magelift/awsbf-preview-aws-eks`. The versioned state bucket
passed exact project/managed-by/purpose/run tag checks; 776 object versions
and 19 delete markers (795 total) were removed before deleting the bucket.

The three run-tagged Secrets Manager records, two `ISSUED` ACM certificates,
two ACM validation CNAMEs with marker
`magelift/acceptance/acm-dns/20260817bf`, six run log groups, and the
run-scoped ECR repository (including its signature artifacts) were each
identity-checked and deleted. The shared cleanup assertion returned:

```text
assert_clean ok
```

An independent owning-service inventory after teardown returned zero for:

| Inventory | Count |
| --- | ---: |
| EKS cluster | 0 |
| RDS instance / subnet group | 0 / 0 |
| ElastiCache replication group / subnet group | 0 / 0 |
| VPC / tagged subnets | 0 / 0 |
| Active cluster EC2 instances | 0 |
| EBS volumes / EIPs | 0 / 0 |
| IAM roles with the run cluster prefix | 0 |
| CloudWatch log groups | 0 |
| Secrets Manager records | 0 |
| Pulumi state bucket | 0 |
| ECR repository | 0 |
| ELBv2 load balancers | 0 |
| ACM certificates (`eu-west-3` / `us-east-1`) | 0 / 0 |
| Cloudflare validation records | 0 |

The customer-managed KMS key supplied as an external prerequisite was not
owned by this run and was intentionally retained.

## Non-claims

This cell does not certify:

- node interruption, node loss, pod disruption budgets, or automatic
  self-managed-node replacement under failure;
- physical AZ outage, zone loss, regional recovery, failover/failback, RPO,
  or RTO;
- EBS snapshot/restore, database restore, cache reconstruction, or media
  recovery;
- custom-domain HTTPS, CloudFront, WAF, origin authentication, purge, or
  edge failover;
- CloudWatch signal delivery beyond the live add-on/controller and agent
  readiness observed here.
