# syntax=docker/dockerfile:1.27

ARG PHP_BASE=docker.io/library/php:8.5-fpm-trixie@sha256:70076c1cae0cd0ba6761832417e3a1df3e5560f0544eb0fe40357373e54420fe
ARG COMPOSER_BASE=docker.io/library/composer:2.10.3@sha256:d8f6343d3fae98107426bc49163ccad46ef85aabd4a27d80a74401fab4aba332
ARG NGINX_BASE=docker.io/library/nginx:1.30.4-trixie@sha256:d5792f71a9496b833bc08ea834a758c46e2b6a6306c10f4be926f38a656cdc1c
ARG APCU_VERSION=5.1.28
ARG REDIS_VERSION=6.3.0
FROM ${COMPOSER_BASE} AS composer
# Keep extension compilation bounded on developer machines.
FROM ${PHP_BASE} AS extensions

ARG APCU_VERSION
ARG REDIS_VERSION

# The official PHP image sets PHP_LDFLAGS to include -pie for the PHP binary.
# libtool misroutes that flag while linking out-of-tree shared extensions on
# Debian Trixie/GCC 14, turning -soname into a compiler option. The PHP binary
# remains PIE; only extension linking needs the narrower linker flags.
ENV PHP_LDFLAGS="-Wl,-O1"

RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
    apt-get update \
    && apt-get install -y --no-install-recommends \
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
    && rm -rf /tmp/pear

FROM ${NGINX_BASE} AS nginx

FROM ${PHP_BASE} AS runtime

ARG APP_UID=10001
ARG APP_GID=10001
ARG PHP_BRANCH=8.5
ARG PHP_BASE
ARG NGINX_BASE

LABEL io.magelift.php.branch="${PHP_BRANCH}" \
      io.magelift.php.base="${PHP_BASE}" \
      io.magelift.nginx.version="1.30.4" \
      io.magelift.web-runtime="php-nginx" \
      io.magelift.base.family="debian-trixie"

RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
    apt-get update \
    && apt-get upgrade -y --no-install-recommends \
    && apt-get install -y --no-install-recommends \
        curl \
        libfreetype6 \
        libicu76 \
        libjpeg62-turbo \
        libpng16-16t64 \
        libxslt1.1 \
        libzip5 \
    && groupadd --gid "${APP_GID}" magelift \
    && useradd --uid "${APP_UID}" --gid magelift --no-create-home --shell /usr/sbin/nologin magelift \
    && install -d -o magelift -g magelift /app /tmp/magelift

COPY --from=nginx /usr/sbin/nginx /usr/sbin/nginx
COPY --from=nginx /usr/lib/nginx /usr/lib/nginx
COPY --from=nginx /etc/nginx/mime.types /etc/nginx/mime.types
COPY --from=nginx /etc/nginx/fastcgi_params /etc/nginx/fastcgi_params
COPY --from=extensions /usr/local/lib/php/extensions/ /usr/local/lib/php/extensions/
COPY --from=extensions /usr/local/etc/php/conf.d/ /usr/local/etc/php/conf.d/
COPY images/php-nginx/php.ini /usr/local/etc/php/conf.d/zz-magelift.ini
COPY images/php-nginx/www.conf /usr/local/etc/php-fpm.d/zz-magelift.conf
COPY images/php-nginx/nginx.conf /etc/nginx/nginx.conf

RUN install -d -o magelift -g magelift /var/cache/nginx /var/log/nginx

RUN install -d -o root -g root /usr/local/lib/magelift
COPY images/php-nginx/deployment-config.php /usr/local/lib/magelift/deployment-config.php

RUN install -d -o magelift -g magelift /app/app/etc
COPY --chown=magelift:magelift images/php-nginx/env.php /app/app/etc/env.php

WORKDIR /app
USER magelift
EXPOSE 8080 9000

CMD ["php-fpm", "--nodaemonize"]

FROM runtime AS builder

USER root
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
    apt-get update \
    && apt-get install -y --no-install-recommends git unzip \
    && install -d -o magelift -g magelift /opt/magelift/build /workspace /output

COPY --from=composer /usr/bin/composer /usr/local/bin/composer
COPY --chown=magelift:magelift build/composer.json build/composer.lock /opt/magelift/build/
COPY --chown=magelift:magelift build/src /opt/magelift/build/src
COPY --chown=magelift:magelift build/bin /opt/magelift/build/bin

RUN composer install \
    --working-dir=/opt/magelift/build \
    --no-dev \
    --no-interaction \
    --no-plugins \
    --no-progress \
    --no-scripts \
    --classmap-authoritative

USER magelift
WORKDIR /workspace
ENTRYPOINT ["php", "/opt/magelift/build/bin/magelift-build-runner"]
