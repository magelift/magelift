# Implementation notes

This file records live-certification decisions and deviations that are not
expressed by the provider-neutral source contract.

## Deviations

### GCP URL-map failover destination ownership check `20260817bk`

`Start` set `ExpectedBackendServiceURL` to the secondary backend before
`Transition`. The first ownership predicate then required the still-primary URL
map to already equal that destination, so a healthy origin-group map was
refused as drift. The pre-mutation check now clears `ExpectedBackendServiceURL`;
the post-update check still requires the destination. Focused comparison tests
pass. Do not reuse `bk`.

### AWS EKS managed-node-group live cell `20260817bd`

The planned disposable configuration started with one `m6i.large` node
(`min/desired/max=1/1/1`). The node reported about 1,930m allocatable CPU
while the existing workload requests consumed about 1,900m. The 500m/1Gi
Magento migration Job could not schedule and Kubernetes reported
`0/1 nodes available: Insufficient cpu`.

The live run used the exact EKS operation
`bea65b0c-99e7-3c10-9588-396b89621ec3` to change the managed node group to
`min/desired/max=2/2/2`. The retained live config was then edited to the same
2/2/2 shape, and a normal Pulumi queue-cell update reconciled the scaling
configuration (`scalingConfig` changed; 86 resources unchanged). This is a
capacity correction for the disposable Magento workload, not a claim that
one-node managed groups are unsupported.

### Immutable application artifact follow-up

The original acceptance digest was retained in the source configuration but
was not used for the final HTTP claim: its PHP-FPM `clear_env = yes` prevented
the runtime resolver from seeing endpoint environment variables. The first
run-scoped replacement digest corrected that behavior but lacked Magento's
generated `pub/static/deployed_version.txt`. The final HTTP probe used the
second signed run-scoped digest
`669890779205.dkr.ecr.eu-west-3.amazonaws.com/magelift-acceptance-awsbd-20260817@sha256:8588b13fc390bc2b733ec3d03a0501ecc82a6ebad7fcca7758c933977fdb2be4`.
Both candidates were deleted with their exact run-scoped ECR repository after
the probe.

### Seeded database URL mutation

The seeded Magento database initially redirected to `http://localhost:8080/`.
For the bounded direct ELB HTTP probe, the run set
`web/unsecure/base_url` and `web/secure/base_url` to the run's ELB URL through
the Magento CLI and flushed the Magento cache. This proves the deployed
application path after an explicit run-owned configuration mutation; it does
not imply that the EKS component currently derives public URL settings from
the domain input or cert resources.

### AWS EKS self-managed AL2023 failed cell `20260817be`

The first self-managed cold run used the current EKS-optimized AL2023 AMI
`ami-070bfbfeba67a8054` and a 2/2/2 `m6i.large` Auto Scaling group. Both EC2
instances reached `InService`, but Kubernetes registered zero nodes. The EC2
console showed `nodeadm-config` failing with `yaml: line 7: mapping values are
not allowed in this context` because the launch template supplied a shell
script as raw user data and attempted to write a complete NodeConfig from
that script. AL2023 parses IMDS user data as NodeConfig before the shell part
runs, so the shell text was never a valid node configuration.

The live evidence is [the failed self-managed attempt](../../../docs/evidence/aws-eks-self-managed-magento-attempt-20260817be.md).
`eksNodeUserData` now emits the AWS-documented MIME multipart form with an
`application/node.eks.aws` NodeConfig part containing the cluster endpoint,
CA, and service CIDR, plus a shell-script part that falls back to
`/etc/eks/bootstrap.sh` only on legacy AMIs. Focused graph tests pass; a new
cold self-managed run is required before this architecture can be counted.

The same preflight exposed a harness bug: a nonexistent state bucket was
reported as an unowned leftover before the harness could create it. The
cleanup library now treats only an explicit S3 not-found response as the new
run path and continues to fail closed on authorization, redirect, or tag
ownership errors.

### AWS EKS self-managed AL2023 live baseline `20260817bf`

The corrected cold cell passed the self-managed architecture baseline after
the `20260817be` user-data failure. The launch template used the AWS MIME
multipart form with an `application/node.eks.aws` NodeConfig containing the
cluster metadata required by AL2023. Two `m6i.large` instances in
`eu-west-3b`/`eu-west-3c` registered `Ready`; `aws-node`, CoreDNS,
kube-proxy, CloudWatch, and the EBS CSI controller/node pods were ready.

The EBS add-on used a dedicated OIDC/IRSA role restricted to
`system:serviceaccount:kube-system:ebs-csi-controller-sa`, and a disposable
PVC/Job completed the write/read assertion before its Delete-reclaim volume
was removed. The database grant, sanitized Magento seed import, runtime health,
queue/database cell, disabled-search cell, and direct ELB homepage HTTP `200`
also passed. The exact stack and run-owned artifacts were deleted and both
the shared `assert_clean` assertion and independent owning-service inventory
were empty. The direct ELB probe required the same explicit seeded-database
base-URL mutation documented for `20260817bd`; this does not claim custom
domain or edge behavior.

The first EBS probe assertion used a literal escaped newline and failed after
volume provisioning/attach; the corrected no-newline assertion passed. The
first keyless signing command omitted `--include-email`, causing Fulcio to
reject the identity token with HTTP 400; the corrected token request and
Cosign verification passed before deployment. These are retained as tooling
deviations, not provider failures. See [the live evidence](../../../docs/evidence/aws-eks-self-managed-magento-live-20260817bf.md).

### AWS EKS self-managed node-loss resilience cell `20260817bh`

The fresh node-loss cell used the self-managed ASG's owning EC2 API rather
than deleting a Kubernetes Node object. The harness selected
`ip-10-104-24-137.eu-west-3.compute.internal`, verified provider ID
`aws:///eu-west-3b/i-0162e28d0b0a7d17a` against the running, cluster-tagged
instance and `awsbh-preview-runtime-self-managed-nodes`, then terminated that
exact instance. The ASG launched `i-067e1bc97e6941c5c` in `eu-west-3b`; the
replacement received a new provider ID and node name, became Ready, and the
pre-existing `eu-west-3c` node stayed Ready. The proof preserved 2 Ready
nodes across 2 zones and the post-replacement runtime-health probe returned
healthy with a measured 71-second replacement/runtime-health interval.

The cell's matrix duration was 76 seconds. Direct post-cell AWS queries
confirmed the old instance was terminated and the replacement plus surviving
instance were running and `InService`/`Healthy` in the exact ASG. The old
Kubernetes Node object remained present but NotReady at the proof timestamp;
the evidence therefore does not claim immediate node-object garbage
collection. Pulumi destroyed 90 resources in 14m23s and exact state,
Secrets Manager, ACM, Cloudflare DNS, CloudWatch Logs, ECR, and independent
owning-service cleanup passed. See [the node-loss evidence](../../../docs/evidence/aws-eks-self-managed-node-loss-live-20260817bh.md).

### AWS EKS self-managed declared-zone loss simulation `20260817bi`

The next bounded cell selected every Ready self-managed node in the
deterministic first declared zone, `eu-west-3b`. The selected node's provider
ID, EC2 placement/state, `aws:eks:cluster-name` tag, and
`awsbi-preview-runtime-self-managed-nodes` membership were checked before the
exact `ec2:TerminateInstances` call. The cell requires and records an actual
zero-Ready gap in the selected zone before accepting recovery; it does not
equate deletion of run-owned instances with a physical AWS Availability Zone
outage.

The 2/2/2 ASG had one Ready target in `eu-west-3b`. After termination of
`i-0317b83a6dcfbfe0e`, the ASG launched distinct replacement
`i-0d8b4a892e21f54d1` in the same zone and Kubernetes registered
`ip-10-105-31-189.eu-west-3.compute.internal` Ready. The existing
`eu-west-3c` instance `i-0f7b3cc1c1868cd0d` remained Ready. Ready count and
zone count returned to `2/2`, runtime health passed, and the measured
replacement/runtime-health interval was 71 seconds. The old Kubernetes Node
object was still present but Unknown in the direct post-cell query; immediate
node-object garbage collection is not claimed.

The cell recorded 75 seconds in the matrix, destroyed all 90 resources in
13m37s, and passed exact state-bucket, prerequisite-secret, ACM, Cloudflare
DNS, log-group, ECR, and independent owning-service cleanup. The source
harness now also writes `cell-zone-loss-targets-before.json` so the exact
pre-injection target set survives alongside the injection/proof records. See
[the zone-loss evidence](../../../docs/evidence/aws-eks-self-managed-zone-loss-live-20260817bi.md).

### AWS EKS self-managed CloudWatch observability cell `20260817bj`

The fresh retained-run stack created 90 resources and passed the self-managed
baseline and database-queue warm cell. The bounded observability cell then
verified the EKS-managed `amazon-cloudwatch-observability` add-on as ACTIVE
(`v6.5.0-eksbuild.1`), the active Linux `cloudwatch-agent` and `fluent-bit`
DaemonSets as `2/2` Ready, one exact marker event in the Container Insights
application log group, one `ContainerInsights/cluster_node_count` data point,
and two `kube-apiserver-audit` streams containing 380 audit events. The cell
measured 59 seconds from probe readiness to the complete delivery proof and
recorded a 65-second matrix duration.

The first assertion was wrong: the add-on also creates optional Windows
DaemonSets with desired/Ready `0`, and the broad name filter treated those
disabled DaemonSets as failures. The source now selects only the active Linux
names `cloudwatch-agent` and `fluent-bit`. The first resume implementation
also ran its retained-stack check before the later function definitions and
before the checkpoint fingerprint/stack ID existed; moving the check into the
main sequence fixed that ordering. Its first ownership predicate then treated
resources without an `acceptance-run` tag as foreign even though the AWS
provider propagates the project tag more broadly than the run tag. The final
predicate rejects only an explicit different run marker while requiring exact
marker-bearing resources, an owned state bucket, a matching fingerprint and
digest, and a non-empty Pulumi stack.

The three predeclared prerequisite secret records initially lacked the
`magelift:purpose=acceptance` tag, so the fail-closed preflight correctly
stopped before mutation; exact tag correction admitted the run. The first
live observability attempt was retained, resumed after the predicate fix, and
the final stack/artifact teardown passed `assert_clean` plus independent
owning-service inventories. The evidence intentionally does not claim alert
notifications, redaction, SLO delivery, custom OTel configuration, or
performance/host/dataplane delivery.

See [the live evidence](../../../docs/evidence/aws-eks-self-managed-cloudwatch-observability-live-20260817bj.md).

## Dead ends retained for future work

The first corrected live retry used the wrong `MAGELIFT_AWS_ACCEPTANCE_DIR`
variable and then a source/workdir collision; neither created stack resources.
The initial fixed-image deploy used the wrong temporary passphrase path and
failed before mutation. A first ECR push used zsh's `$uri:tag` parameter
expansion and addressed a nonexistent repository; the corrected push used the
run-scoped repository and preserved the source digest. These are recorded so
future retries do not reinterpret them as provider failures.
