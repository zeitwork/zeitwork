FROM php:8.3-cli AS base
WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
    unzip libpq-dev libzip-dev libpng-dev libjpeg-dev libfreetype6-dev && \
    docker-php-ext-configure gd --with-freetype --with-jpeg && \
    docker-php-ext-install pdo pdo_mysql pdo_pgsql zip gd bcmath opcache && \
    rm -rf /var/lib/apt/lists/*

COPY --from=composer:2 /usr/bin/composer /usr/bin/composer

FROM base AS build

COPY composer.json composer.lock ./
RUN composer install --no-dev --no-scripts --no-autoloader

COPY . .
RUN composer dump-autoload --optimize && \
    php artisan config:cache || true && \
    php artisan route:cache || true && \
    php artisan view:cache || true

FROM base AS runtime

RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - && \
    apt-get install -y nodejs && \
    rm -rf /var/lib/apt/lists/*

COPY --from=build /app /app

RUN if [ -f package.json ]; then npm ci && npm run build || true; fi

RUN groupadd --system --gid 1000 laravel && \
    useradd laravel --uid 1000 --gid 1000 --create-home --shell /bin/bash && \
    chown -R laravel:laravel /app/storage /app/bootstrap/cache

USER 1000:1000

EXPOSE 3000
CMD ["php", "artisan", "serve", "--host=0.0.0.0", "--port=3000"]
