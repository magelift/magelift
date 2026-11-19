# syntax=docker/dockerfile:1.27

ARG FRANKENPHP_BASE=docker.io/dunglas/frankenphp:1.13.5-php8.5-trixie@sha256:667c5fffd348d9c3c260803d75956f1d9aace7e4b7331d8893de6f101d7e0e57
FROM ${FRANKENPHP_BASE}

ARG FRANKENPHP_BASE
ARG APP_UID=10001
ARG APP_GID=10001
ARG PHP_BRANCH=8.5
ARG APCU_VERSION=5.1.28
ARG REDIS_VERSION=6.3.0

LABEL io.magelift.php.branch="${PHP_BRANCH}" \
      io.magelift.web-runtime="frankenphp-classic" \
      io.magelift.base.family="debian-trixie" \
      io.magelift.php.base="${FRANKENPHP_BASE}"

RUN cp "$PHP_INI_DIR/php.ini-production" "$PHP_INI_DIR/php.ini" \
    && groupadd --gid "${APP_GID}" magelift \
    && useradd --uid "${APP_UID}" --gid magelift --no-create-home --shell /usr/sbin/nologin magelift \
    && install -d -o magelift -g magelift /app /tmp/magelift \
    && chown -R magelift:magelift /config /data \
    && apt-get update \
    && apt-get upgrade -y --no-install-recommends \
    && apt-get install -y --no-install-recommends \
        curl \
        libfreetype-dev \
        libicu-dev \
        libjpeg62-turbo-dev \
        libpng-dev \
        libxml2-dev \
        libxslt1-dev \
        libzip-dev \
    && docker-php-ext-configure gd --with-freetype --with-jpeg \
    && docker-php-ext-install -j1 \
        bcmath \
        gd \
        ftp \
        intl \
        pdo_mysql \
        soap \
        sockets \
        xsl \
        zip \
    && pecl install \
        "apcu-${APCU_VERSION}" \
        "redis-${REDIS_VERSION}" \
    && docker-php-ext-enable apcu redis \
    && rm -rf /tmp/pear \
    && rm -rf /var/lib/apt/lists/*

COPY images/php-nginx/php.ini /usr/local/etc/php/conf.d/zz-magelift.ini
COPY images/frankenphp-classic/Caddyfile /etc/frankenphp/Caddyfile
RUN install -d -o root -g root /usr/local/lib/magelift
COPY images/php-nginx/deployment-config.php /usr/local/lib/magelift/deployment-config.php
RUN install -d -o magelift -g magelift /app/app/etc
COPY --chown=magelift:magelift images/php-nginx/env.php /app/app/etc/env.php

WORKDIR /app
USER magelift
EXPOSE 8080 8443
