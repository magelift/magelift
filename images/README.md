# Container images

`php-runtime` is the common PHP-FPM and nginx base for Magento application images. The
certified matrix covers PHP 8.2 through 8.5 on Debian Trixie. PHP 8.2 and 8.3
receive security fixes from PHP upstream, while PHP 8.4 and 8.5 receive active
support. Magento release lines still tied to PHP 8.1 are not available because
PHP 8.1 is no longer supported upstream.

| Magento line | MageLift PHP branches |
| --- | --- |
| 2.4.9 | 8.5 |
| 2.4.8 | 8.3, 8.4 |
| 2.4.7 | 8.2, 8.3 |
| 2.4.6 | 8.2 |

The images use Debian for glibc and third-party extension compatibility. Base
images are pinned by multi-platform digest and updated through reviewed dependency
changes.

The FrankenPHP classic adapter is published for the same PHP 8.2 through 8.5
branches with the `frankenphp-classic-supported` Bake target.

The image runs as UID 10001, writes logs to standard streams, and keeps application
code under `/app`. It embeds nginx 1.30.4 so the image matches the current Adobe
Commerce 2.4.9 requirement. An `nginx-fpm` ECS task runs two containers from the
same image: the `php-fpm` process and an nginx HTTP sidecar. They share the task
network namespace, so nginx proxies to PHP-FPM on localhost without exposing port
9000. Integrated ECS tasks add the pinned Varnish 8.0.2 sidecar on port 6081 and
send the load balancer there; headless tasks send traffic directly to nginx on
port 8080. Both nginx and FrankenPHP short-circuit `GET /health` with `200 OK`
(no Magento bootstrap). Integrated ALB health checks hit Varnish, which must
pass `/health` through to that short-circuit (see `images/varnish/default.vcl`).
Acceptance digests must be MageLift runtime images (`php-runtime` or
`frankenphp-classic`) — a bare Magento app image without `/health` will flap ALB
targets. Runtime images include `curl` for the ECS container health check.
Varnish keeps its root filesystem read-only and receives only a
task-scoped executable tmpfs at `/var/lib/varnish` for VSM and transient cache
data.
Deployments must mount writable Magento runtime directories and `/tmp` as task-scoped
storage when the root filesystem is read-only.

Build the local PHP 8.5 image:

```sh
docker buildx bake php-runtime --load
```

The integrated Varnish sidecar contract can be checked without an AWS account:

```sh
make varnish-test
```

Build the FrankenPHP classic adapter:

```sh
docker buildx bake frankenphp-classic --load
```

Build the supported multi-platform matrix for a registry exporter:

```sh
docker buildx bake php-runtime-supported php-builder-supported --push
```

Registry builds attach an SPDX SBOM and SLSA v1 provenance. Max provenance records
build arguments, so credentials must use BuildKit secret mounts and must never be
passed as build arguments. Release automation will sign and verify the resulting
digest after registry publication. Pull-request jobs neither push nor request an OIDC
token.

Tagged releases publish the supported matrix to GHCR as
`ghcr.io/acourtiol/magelift-runtime`, `ghcr.io/acourtiol/magelift-builder`, and
`ghcr.io/acourtiol/magelift-frankenphp-classic`. Each PHP branch receives a stable
branch tag and a release tag. Deployments should pin the digest printed by the
release workflow; tags are convenience aliases, not release identity.

The base contains no Magento source or credentials. A later application-image stage
will copy the validated build output and artifact manifest into this runtime.
