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
      { branch = "8.2", base = "docker.io/library/php:8.2-fpm-trixie@sha256:ecc8e257cfc08404b00909e560656df4b5461f21099fd645b507862f84dce358" },
      { branch = "8.3", base = "docker.io/library/php:8.3-fpm-trixie@sha256:32b6cb4cf9f41d3e62424e64959bbf824125ca7b7869c18fb54d4bafc9d990b4" },
      { branch = "8.4", base = "docker.io/library/php:8.4-fpm-trixie@sha256:3661dc717d73bdb0ec58bcdc96af36af90f8894c66c09ac74721a012b01da6f4" },
      { branch = "8.5", base = "docker.io/library/php:8.5-fpm-trixie@sha256:0dc450d0a0e81ba501973b8e303f5d45af2ed989e08730f597d8fc07fb289efd" },
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
      { branch = "8.2", base = "docker.io/library/php:8.2-fpm-trixie@sha256:ecc8e257cfc08404b00909e560656df4b5461f21099fd645b507862f84dce358" },
      { branch = "8.3", base = "docker.io/library/php:8.3-fpm-trixie@sha256:32b6cb4cf9f41d3e62424e64959bbf824125ca7b7869c18fb54d4bafc9d990b4" },
      { branch = "8.4", base = "docker.io/library/php:8.4-fpm-trixie@sha256:3661dc717d73bdb0ec58bcdc96af36af90f8894c66c09ac74721a012b01da6f4" },
      { branch = "8.5", base = "docker.io/library/php:8.5-fpm-trixie@sha256:0dc450d0a0e81ba501973b8e303f5d45af2ed989e08730f597d8fc07fb289efd" },
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
      { branch = "8.2", base = "docker.io/dunglas/frankenphp:1.12.4-php8.2-trixie@sha256:e9552431aa69ce4b6da625c14bd19b4b127b8d8a37252369efb3a5ea05074d2a" },
      { branch = "8.3", base = "docker.io/dunglas/frankenphp:1.12.4-php8.3-trixie@sha256:28c061d337d6b9e893190e4d8d2d1112d6ad18f840574e22630a73ad7156adbb" },
      { branch = "8.4", base = "docker.io/dunglas/frankenphp:1.12.4-php8.4-trixie@sha256:a6b2340a4a9b70fc25d3afe663052d64da949721ff081399533856fba86fa3f4" },
      { branch = "8.5", base = "docker.io/dunglas/frankenphp:1.12.4-php8.5-trixie@sha256:a0ad00db3dd7f61b55a9951c9b6ef73ac8e1dd4c6de36979a0d1651b48da5f43" },
    ]
  }
  name = "frankenphp-classic-${replace(php.branch, ".", "-")}"
  args = {
    FRANKENPHP_BASE = php.base
    PHP_BRANCH = php.branch
  }
  tags = ["magelift/frankenphp-classic:${php.branch}-local"]
}
