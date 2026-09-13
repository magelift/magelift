# syntax=docker/dockerfile:1.27

ARG RUNTIME_BASE=scratch
FROM ${RUNTIME_BASE}

ARG SOURCE_REVISION
ARG SOURCE_URI
ARG MAGELIFT_VERSION

LABEL org.opencontainers.image.revision="${SOURCE_REVISION}" \
      org.opencontainers.image.source="${SOURCE_URI}" \
      org.opencontainers.image.version="${MAGELIFT_VERSION}"

# Magento's build commands may create app/etc/env.php with only build-time
# cache metadata. Keep the deployment scaffold from the pinned runtime base;
# runtime MAGENTO_DC_* values are resolved when the container starts.
RUN test -f /app/app/etc/env.php \
    && cp /app/app/etc/env.php /tmp/magelift-runtime-env.php

COPY --chown=10001:10001 rootfs/ /app/

RUN cp /tmp/magelift-runtime-env.php /app/app/etc/env.php \
    && rm -f /tmp/magelift-runtime-env.php \
    && test -f composer.lock \
    && test -x bin/magento \
    && test ! -e auth.json \
    && test -f app/etc/env.php \
    && ! grep -Eiq 'MAGELIFT' app/etc/env.php \
    && php -r '$data = require "app/etc/env.php"; array_walk_recursive($data, static function ($value, $key): void { if (preg_match("/password|secret|token/i", (string) $key) === 1 && is_string($value) && $value !== "" && !str_starts_with($value, "#env(")) { exit(1); } });'
