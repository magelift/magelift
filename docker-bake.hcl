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
      { branch = "8.3", base = "docker.io/library/php:8.3.37-fpm-trixie@sha256:a216054d3de38bc1e45704ef75402a6c43b659ef84f6adc0fa494b2233c195e0" },
      { branch = "8.4", base = "docker.io/library/php:8.4.28-fpm-trixie@sha256:9185c66c60705ef5775fc2f9f92bcda1a3645b3d195abaccecc37485c28eded8" },
      { branch = "8.5", base = "docker.io/library/php:8.5.13-fpm-trixie@sha256:52ba6f178d0a08f348578b03595661b7ff13defafacf4c823d045f2ecba8db8f" },
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
      { branch = "8.3", base = "docker.io/library/php:8.3.37-fpm-trixie@sha256:a216054d3de38bc1e45704ef75402a6c43b659ef84f6adc0fa494b2233c195e0" },
      { branch = "8.4", base = "docker.io/library/php:8.4.28-fpm-trixie@sha256:9185c66c60705ef5775fc2f9f92bcda1a3645b3d195abaccecc37485c28eded8" },
      { branch = "8.5", base = "docker.io/library/php:8.5.13-fpm-trixie@sha256:52ba6f178d0a08f348578b03595661b7ff13defafacf4c823d045f2ecba8db8f" },
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
      { branch = "8.3", base = "docker.io/dunglas/frankenphp:1.13.5-php8.3-trixie@sha256:5e17d7af9314b54213588803f117a2d482b6ba270b7d41381400ec78db45b36e" },
      { branch = "8.4", base = "docker.io/dunglas/frankenphp:1.13.5-php8.4-trixie@sha256:8f0d509ad824fa86da88b55c5ea9f1605380f423ab55ec3a72864cc223491bfd" },
      { branch = "8.5", base = "docker.io/dunglas/frankenphp:1.13.5-php8.5-trixie@sha256:667c5fffd348d9c3c260803d75956f1d9aace7e4b7331d8893de6f101d7e0e57" },
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
      { branch = "8.3", base = "docker.io/library/php:8.3.37-fpm-trixie@sha256:a216054d3de38bc1e45704ef75402a6c43b659ef84f6adc0fa494b2233c195e0" },
      { branch = "8.4", base = "docker.io/library/php:8.4.28-fpm-trixie@sha256:9185c66c60705ef5775fc2f9f92bcda1a3645b3d195abaccecc37485c28eded8" },
      { branch = "8.5", base = "docker.io/library/php:8.5.13-fpm-trixie@sha256:52ba6f178d0a08f348578b03595661b7ff13defafacf4c823d045f2ecba8db8f" },
    ]
  }
  name = "php-apache-${replace(php.branch, ".", "-")}"
  args = {
    PHP_BASE = php.base
    PHP_BRANCH = php.branch
  }
  tags = ["magelift/php-apache:${php.branch}-local"]
}
