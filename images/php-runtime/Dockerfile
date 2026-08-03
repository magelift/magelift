# syntax=docker/dockerfile:1.18

ARG PHP_BASE=docker.io/library/php:8.5-fpm-trixie@sha256:0dc450d0a0e81ba501973b8e303f5d45af2ed989e08730f597d8fc07fb289efd
ARG COMPOSER_BASE=docker.io/library/composer:2.10.2@sha256:5946476338742b200bb9ff88f8be56275ddae4b3949c72305cb0dbf10cfcb760
ARG NGINX_BASE=docker.io/library/nginx:1.30.4-trixie@sha256:5cf90903deda2c5981b8ad05e7617ac010e389f0dde0ac83487c02c509281de6
FROM ${COMPOSER_BASE} AS composer
FROM ${PHP_BASE} AS extensions

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
    && docker-php-ext-install -j"$(nproc)" \
        bcmath \
        gd \
        intl \
        pdo_mysql \
        soap \
        sockets \
        xsl \
        zip

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
      io.magelift.base.family="debian-trixie"

RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
    apt-get update \
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
COPY images/php-runtime/php.ini /usr/local/etc/php/conf.d/zz-magelift.ini
COPY images/php-runtime/www.conf /usr/local/etc/php-fpm.d/zz-magelift.conf
COPY images/php-runtime/nginx.conf /etc/nginx/nginx.conf

RUN install -d -o magelift -g magelift /var/cache/nginx /var/log/nginx

RUN install -d -o magelift -g magelift /app/app/etc
COPY --chown=magelift:magelift images/php-runtime/env.php /app/app/etc/env.php

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
