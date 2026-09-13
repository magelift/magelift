# syntax=docker/dockerfile:1.27

ARG PHP_BASE=docker.io/library/php:8.5-fpm-trixie@sha256:70076c1cae0cd0ba6761832417e3a1df3e5560f0544eb0fe40357373e54420fe
ARG APCU_VERSION=5.1.28
ARG REDIS_VERSION=6.3.0

FROM ${PHP_BASE} AS extensions

ARG APCU_VERSION
ARG REDIS_VERSION

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

FROM ${PHP_BASE} AS runtime

ARG APP_UID=10001
ARG APP_GID=10001
ARG PHP_BRANCH=8.5
ARG PHP_BASE

LABEL io.magelift.php.branch="${PHP_BRANCH}" \
      io.magelift.php.base="${PHP_BASE}" \
      io.magelift.web-runtime="php-apache" \
      io.magelift.base.family="debian-trixie"

RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
    apt-get update \
    && apt-get upgrade -y --no-install-recommends \
    && apt-get install -y --no-install-recommends \
        apache2 \
        curl \
        libfreetype6 \
        libicu76 \
        libjpeg62-turbo \
        libpng16-16t64 \
        libxslt1.1 \
        libzip5 \
    && groupadd --gid "${APP_GID}" magelift \
    && useradd --uid "${APP_UID}" --gid magelift --no-create-home --shell /usr/sbin/nologin magelift \
    && install -d -o magelift -g magelift /app /tmp/magelift /tmp/apache2 /var/log/apache2 \
    && sed -ri \
        -e 's/^export APACHE_RUN_USER=.*/export APACHE_RUN_USER=magelift/' \
        -e 's/^export APACHE_RUN_GROUP=.*/export APACHE_RUN_GROUP=magelift/' \
        -e 's%^export APACHE_RUN_DIR=.*%export APACHE_RUN_DIR=/tmp/apache2%' \
        -e 's%^export APACHE_PID_FILE=.*%export APACHE_PID_FILE=/tmp/apache2/apache2.pid%' \
        -e 's%^export APACHE_LOCK_DIR=.*%export APACHE_LOCK_DIR=/tmp/apache2%' \
        -e 's%^export APACHE_LOG_DIR=.*%export APACHE_LOG_DIR=/dev%' \
        /etc/apache2/envvars \
    && sed -ri 's/^Listen 80$/Listen 8080/' /etc/apache2/ports.conf \
    && a2enmod proxy proxy_fcgi rewrite \
    && rm -rf /var/lib/apt/lists/*

COPY --from=extensions /usr/local/lib/php/extensions/ /usr/local/lib/php/extensions/
COPY --from=extensions /usr/local/etc/php/conf.d/ /usr/local/etc/php/conf.d/
COPY images/php-runtime/php.ini /usr/local/etc/php/conf.d/zz-magelift.ini
COPY images/php-runtime/www.conf /usr/local/etc/php-fpm.d/zz-magelift.conf
COPY images/php-apache/apache.conf /etc/apache2/sites-available/000-default.conf

RUN install -d -o root -g root /usr/local/lib/magelift /usr/local/share/magelift \
    && printf 'OK\n' > /usr/local/share/magelift/health
COPY images/php-runtime/deployment-config.php /usr/local/lib/magelift/deployment-config.php

RUN install -d -o magelift -g magelift /app/app/etc
COPY --chown=magelift:magelift images/php-runtime/env.php /app/app/etc/env.php

WORKDIR /app
USER magelift
EXPOSE 8080 9000

CMD ["sh", "-eu", "-c", "php-fpm --daemonize && exec apache2ctl -D FOREGROUND"]
