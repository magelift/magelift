# syntax=docker/dockerfile:1.18

ARG FRANKENPHP_BASE=docker.io/dunglas/frankenphp:1.12.4-php8.5-trixie@sha256:a0ad00db3dd7f61b55a9951c9b6ef73ac8e1dd4c6de36979a0d1651b48da5f43
FROM ${FRANKENPHP_BASE}

ARG FRANKENPHP_BASE
ARG APP_UID=10001
ARG APP_GID=10001
ARG PHP_BRANCH=8.5

LABEL io.magelift.php.branch="${PHP_BRANCH}" \
      io.magelift.web-runtime="frankenphp-classic" \
      io.magelift.base.family="debian-trixie" \
      io.magelift.php.base="${FRANKENPHP_BASE}"

RUN cp "$PHP_INI_DIR/php.ini-production" "$PHP_INI_DIR/php.ini" \
    && groupadd --gid "${APP_GID}" magelift \
    && useradd --uid "${APP_UID}" --gid magelift --no-create-home --shell /usr/sbin/nologin magelift \
    && install -d -o magelift -g magelift /app /tmp/magelift \
    && chown -R magelift:magelift /config /data

COPY images/frankenphp-classic/Caddyfile /etc/frankenphp/Caddyfile
RUN install -d -o magelift -g magelift /app/app/etc
COPY --chown=magelift:magelift images/php-runtime/env.php /app/app/etc/env.php

WORKDIR /app
USER magelift
EXPOSE 8080 8443
