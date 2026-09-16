group "default" {
  targets = ["php-nginx"]
}

target "php-nginx" {
  context    = "."
  dockerfile = "images/php-nginx/Dockerfile"
  target     = "runtime"
  tags       = ["magelift/php-nginx:local"]
}

target "php-builder" {
  inherits = ["php-nginx"]
  target   = "builder"
  tags     = ["magelift/php-builder:local"]
}

target "php-nginx-supported" {
  inherits = ["php-nginx"]
  platforms = ["linux/amd64", "linux/arm64"]
  attest = [
    "type=sbom",
    "type=provenance,mode=max,version=v1",
  ]
  matrix = {
    php = [
      { branch = "8.2", base = "docker.io/library/php:8.2-fpm-trixie@sha256:596175799ca76a93d5d0c3cda7d5f70fccfc86dfdaf97580c1c27e665aba991f" },
      { branch = "8.3", base = "docker.io/library/php:8.3-fpm-trixie@sha256:5ad27201cf5cdf1704e147218e64fa0db2155ecda26edbe2b3eeb7b90cbf9328" },
      { branch = "8.4", base = "docker.io/library/php:8.4-fpm-trixie@sha256:59fa733c9af643a122f8a9976119460e35ce76dd0a3f2b9c8f75af8e361a54e2" },
      { branch = "8.5", base = "docker.io/library/php:8.5-fpm-trixie@sha256:70076c1cae0cd0ba6761832417e3a1df3e5560f0544eb0fe40357373e54420fe" },
    ]
  }
  name = "php-nginx-${replace(php.branch, ".", "-")}"
  args = {
    PHP_BASE = php.base
    PHP_BRANCH = php.branch
  }
  tags = ["magelift/php-nginx:${php.branch}-local"]
}

target "php-builder-supported" {
  inherits = ["php-nginx"]
  platforms = ["linux/amd64", "linux/arm64"]
  attest = [
    "type=sbom",
    "type=provenance,mode=max,version=v1",
  ]
  matrix = {
    php = [
      { branch = "8.2", base = "docker.io/library/php:8.2-fpm-trixie@sha256:596175799ca76a93d5d0c3cda7d5f70fccfc86dfdaf97580c1c27e665aba991f" },
      { branch = "8.3", base = "docker.io/library/php:8.3-fpm-trixie@sha256:5ad27201cf5cdf1704e147218e64fa0db2155ecda26edbe2b3eeb7b90cbf9328" },
      { branch = "8.4", base = "docker.io/library/php:8.4-fpm-trixie@sha256:59fa733c9af643a122f8a9976119460e35ce76dd0a3f2b9c8f75af8e361a54e2" },
      { branch = "8.5", base = "docker.io/library/php:8.5-fpm-trixie@sha256:70076c1cae0cd0ba6761832417e3a1df3e5560f0544eb0fe40357373e54420fe" },
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
      { branch = "8.2", base = "docker.io/dunglas/frankenphp:1.12.7-php8.2-trixie@sha256:3c1ad970186bc0d3b47d0ff3212f9f47dfa0e6c7e5dbb68bec19ed9c6464c466" },
      { branch = "8.3", base = "docker.io/dunglas/frankenphp:1.12.7-php8.3-trixie@sha256:b0029775078a8434f74f7c694e0634e4a8e1a81b758132b22baaf42110a176b4" },
      { branch = "8.4", base = "docker.io/dunglas/frankenphp:1.12.7-php8.4-trixie@sha256:77bc2d40a58ace3a9425e4cbb0c40d044188dfd850e44a943ed043d743625df3" },
      { branch = "8.5", base = "docker.io/dunglas/frankenphp:1.12.7-php8.5-trixie@sha256:f92d81eb3fe4fd18b35d3d58192b7cc3acc8943817bbf39f2fcf0be02a3916dc" },
    ]
  }
  name = "frankenphp-classic-${replace(php.branch, ".", "-")}"
  args = {
    FRANKENPHP_BASE = php.base
    PHP_BRANCH = php.branch
  }
  tags = ["magelift/frankenphp-classic:${php.branch}-local"]
}

target "php-apache" {
  context    = "."
  dockerfile = "images/php-apache/Dockerfile"
  tags       = ["magelift/php-apache:8.5-local"]
}

target "php-apache-supported" {
  inherits = ["php-apache"]
  platforms = ["linux/amd64", "linux/arm64"]
  attest = [
    "type=sbom",
    "type=provenance,mode=max,version=v1",
  ]
  matrix = {
    php = [
      { branch = "8.2", base = "docker.io/library/php:8.2-fpm-trixie@sha256:596175799ca76a93d5d0c3cda7d5f70fccfc86dfdaf97580c1c27e665aba991f" },
      { branch = "8.3", base = "docker.io/library/php:8.3-fpm-trixie@sha256:5ad27201cf5cdf1704e147218e64fa0db2155ecda26edbe2b3eeb7b90cbf9328" },
      { branch = "8.4", base = "docker.io/library/php:8.4-fpm-trixie@sha256:59fa733c9af643a122f8a9976119460e35ce76dd0a3f2b9c8f75af8e361a54e2" },
      { branch = "8.5", base = "docker.io/library/php:8.5-fpm-trixie@sha256:70076c1cae0cd0ba6761832417e3a1df3e5560f0544eb0fe40357373e54420fe" },
    ]
  }
  name = "php-apache-${replace(php.branch, ".", "-")}"
  args = {
    PHP_BASE = php.base
    PHP_BRANCH = php.branch
  }
  tags = ["magelift/php-apache:${php.branch}-local"]
}
