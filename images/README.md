# Bundled build recipe

`magelift build` builds the shop's OCI image from the embedded nginx/PHP-FPM
recipe in [`php-nginx/`](php-nginx/). The builder and runtime use the same
PHP branch. The shop pushes the resulting application image to its own
registry and deploys its digest. `magelift dev` uses the runtime recipe locally.

Run `make image-test` to build and test the PHP runtime. Run
`make builder-image-test` to check the builder's user and Composer binary.
`make varnish-test` tests the Varnish configuration in [`varnish/`](varnish/).
[Application builds](../docs/builds.md)
