# rc19-proof HTTP 500 diagnosis — 2026-09-22

This is a development-loop diagnosis, not a certification. The capability
matrix is unchanged.

The retained proof in the acceptance project was labeled `rc19-proof`.
The proof VM `rsl1-cleanvm` carried
`magelift-proof=alpha-review-corrections-1-4`. A seven-day log search
found no `rc.22` or `v0.1.0-alpha.1-rc.22` string. The stack was created
on 2026-09-22, the same day the rc.22 report was written.

## Cause

`migration`. The deploy container ran `app:config:import` before
`setup:upgrade`. Import failed with `SQLSTATE[42S02]`: table
`magento.flag` does not exist (`flag_code=config_hash`). Three migrate
pods logged that error. No `deploy.done` marker was logged.

The shop image
`europe-west1-docker.pkg.dev/digital-lab-341608/shop/shop@sha256:9469338d69a4ab505eada974215aeed9932411d864685061f2dcc91f38ad9f89`
was pulled and the web and php-fpm containers started. Cloud SQL
`rc19-proof-preview-sql` was `RUNNABLE` and the error was a missing
table, not a refused connection. Ingress and an in-cluster request both
returned HTTP 500, so the load balancer was not the boundary that
failed.

`kubectl` could not be used: `gke-gcloud-auth-plugin` is not installed
on the workstation, so `exception.log` was not read. Cloud Logging did
not include a Magento exception class name.

## Fix

`build/src/Magento/LifecyclePlan.php` now runs `setup:upgrade` before
`app:config:import`. The Go migration shell golden was regenerated from
that plan. `LifecyclePlanTest::testDeployCreatesSchemaBeforeConfigImport`
failed on the old order and passed after the change.

## Cleanup

Deleted: GKE cluster `rc19-proof-preview-gke`, Cloud SQL
`rc19-proof-preview-sql`, VM `rsl1-cleanvm`, the state and media
buckets, Memorystore Valkey `rc19-proof-preview-valkey`, the service
account, and the service-connection policy.

The router and four subnets were deleted after Valkey was gone.
`gcloud services vpc-peerings delete` kept failing with
`FLOW_SN_DC_RESOURCE_PREVENTING_DELETE_CONNECTION` (subject `171113`)
even though no Cloud SQL, Memorystore, Redis, or Filestore instance
remained. `gcloud compute networks peerings delete
servicenetworking-googleapis-com` removed the peering. The reserved
range and VPC `rc19-proof-preview-net` were then deleted. A later
sweep found no `rc19-proof` cluster, instance, SQL instance, Valkey
instance, network, address, or service account.

`mldp7-*` networks, ranges, and the Valkey service-connection policy
were not deleted. They belong to that earlier run.
