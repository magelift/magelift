group "default" {
  targets = ["php-runtime"]
}

target "php-runtime" {
  context    = "."
  dockerfile = "images/php-runtime/Dockerfile"
  target     = "runtime"
  tags       = ["magelift/php-runtime:local"]
}

target "php-builder" {
  inherits = ["php-runtime"]
  target   = "builder"
  tags     = ["magelift/php-builder:local"]
}

target "php-runtime-supported" {
  inherits = ["php-runtime"]
  platforms = ["linux/amd64", "linux/arm64"]
  attest = [
    "type=sbom",
    "type=provenance,mode=max,version=v1",
  ]
  matrix = {
    php = [
      { branch = "8.2", base = "docker.io/library/php:8.2-fpm-trixie@sha256:250c82f606f926bdbcc2baa83b2e6becf7ffec9944b9e6d308177274084e4321" },
      { branch = "8.3", base = "docker.io/library/php:8.3-fpm-trixie@sha256:4202966c9c1e2dc5416c845ec69617fd734c186f4acd31089a1dd5fa2218648d" },
      { branch = "8.4", base = "docker.io/library/php:8.4-fpm-trixie@sha256:a77f5f0ee8df9035f2a90d4f713e253bc374ed27534a30e5889e50e9346c5232" },
      { branch = "8.5", base = "docker.io/library/php:8.5-fpm-trixie@sha256:f56f4a81de6cd33ddfd6e99352889a53c94c3ffccce89e494563845a1c8ba75a" },
    ]
  }
  name = "php-runtime-${replace(php.branch, ".", "-")}"
  args = {
    PHP_BASE = php.base
    PHP_BRANCH = php.branch
  }
  tags = ["magelift/php-runtime:${php.branch}-local"]
}

target "php-builder-supported" {
  inherits = ["php-runtime"]
  platforms = ["linux/amd64", "linux/arm64"]
  attest = [
    "type=sbom",
    "type=provenance,mode=max,version=v1",
  ]
  matrix = {
    php = [
      { branch = "8.2", base = "docker.io/library/php:8.2-fpm-trixie@sha256:250c82f606f926bdbcc2baa83b2e6becf7ffec9944b9e6d308177274084e4321" },
      { branch = "8.3", base = "docker.io/library/php:8.3-fpm-trixie@sha256:4202966c9c1e2dc5416c845ec69617fd734c186f4acd31089a1dd5fa2218648d" },
      { branch = "8.4", base = "docker.io/library/php:8.4-fpm-trixie@sha256:a77f5f0ee8df9035f2a90d4f713e253bc374ed27534a30e5889e50e9346c5232" },
      { branch = "8.5", base = "docker.io/library/php:8.5-fpm-trixie@sha256:f56f4a81de6cd33ddfd6e99352889a53c94c3ffccce89e494563845a1c8ba75a" },
    ]
  }
  target   = "builder"
  args = {
    PHP_BASE = php.base
    PHP_BRANCH = php.branch
  }
  name     = "php-builder-${replace(php.branch, ".", "-")}"
  tags     = ["magelift/php-builder:${php.branch}-local"]
}

target "frankenphp-classic" {
  context    = "."
  dockerfile = "images/frankenphp-classic/Dockerfile"
  tags       = ["magelift/frankenphp-classic:8.5-local"]
}

target "frankenphp-classic-supported" {
  inherits = ["frankenphp-classic"]
  platforms = ["linux/amd64", "linux/arm64"]
  attest = [
    "type=sbom",
    "type=provenance,mode=max,version=v1",
  ]
  matrix = {
    php = [
      { branch = "8.2", base = "docker.io/dunglas/frankenphp:1.12.6-php8.2-trixie@sha256:f6b639e0c1445db0a08b2558b9fbf9d86d8b89eaaad734554c1fbd3854266672" },
      { branch = "8.3", base = "docker.io/dunglas/frankenphp:1.12.6-php8.3-trixie@sha256:032d1d77d88076d0290cb8d3d22e5645e8cd33b970644304c59a7a35d5218b93" },
      { branch = "8.4", base = "docker.io/dunglas/frankenphp:1.12.6-php8.4-trixie@sha256:d341f89bf44ff1d867cd6b9dc88a7a2942741cdddfbf92543ddf7e65c29f3412" },
      { branch = "8.5", base = "docker.io/dunglas/frankenphp:1.12.6-php8.5-trixie@sha256:f090c8edf61410f04f3e8533c7bc4896de68be0d363c8c70be200f2c17fab232" },
    ]
  }
  name = "frankenphp-classic-${replace(php.branch, ".", "-")}"
  args = {
    FRANKENPHP_BASE = php.base
    PHP_BRANCH = php.branch
  }
  tags = ["magelift/frankenphp-classic:${php.branch}-local"]
}
