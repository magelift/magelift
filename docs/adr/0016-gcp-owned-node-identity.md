# ADR 0016. Give GKE nodes a dedicated identity

Accepted on 2026-10-18.

Bootstrap creates an environment-owned node service account. The default
Compute service account may have unrelated permissions and is used by other
workloads in the project.

After checking the registry's ownership, bootstrap grants the node account
Artifact Registry reader on that repository and the
`roles/container.defaultNodeServiceAccount` project role. This preparation
also runs for a laptop deployment. It requires permission to manage IAM.
The separate CI permission plan grants the deployment identity permission to
act as the node account.

Autopilot sets the account and OAuth scope in
`clusterAutoscaling.autoProvisioningDefaults`. GKE computes the top-level
`nodeConfig` block for Autopilot. Sending it can cause Pulumi to plan a cluster
replacement. Standard clusters and node pools use their explicit
`nodeConfig`.

Pulumi treats the selected identity as a creation-time setting. Create a
fresh environment to migrate a cluster using the default account. The node
account belongs to bootstrap and survives environment teardown.

API fixtures and Pulumi mock tests check permissions, resource ownership, initial
cluster settings and replacement guards. [Cloud setup](../bootstrap.md) · [GKE Autopilot creation](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/creating-an-autopilot-cluster)
