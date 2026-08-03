# syntax=docker/dockerfile:1.18

ARG RUNTIME_BASE=scratch
FROM ${RUNTIME_BASE}

ARG SOURCE_REVISION
ARG SOURCE_URI
ARG MAGELIFT_VERSION

LABEL org.opencontainers.image.revision="${SOURCE_REVISION}" \
      org.opencontainers.image.source="${SOURCE_URI}" \
      org.opencontainers.image.version="${MAGELIFT_VERSION}"

COPY --chown=10001:10001 rootfs/ /app/

RUN test -f composer.lock \
    && test -x bin/magento \
    && test ! -e auth.json \
    && test -f app/etc/env.php \
    && ! grep -Eiq 'password|secret|MAGELIFT' app/etc/env.php
