# Connect a storefront

Use the native Magento storefront with `application.mode` set to `integrated`. Build the installed themes, modules, static content and locales into the shop image. Set your domain on each environment. Check the storefront, Admin and media URLs after promoting the image.

For a separately deployed storefront, start with the frontend origin when initializing the Magento backend.

```sh
magelift init --provider aws --storefront-origin https://store.example.com
magelift --env staging config effective
magelift env status staging
```

MageLift configures Magento’s storefront origin and allowed CORS origins. The status output gives the configured Magento GraphQL endpoint. Add the corresponding Magento origin to the frontend’s configuration and deploy that frontend with its own tooling. Give previews their own frontend origin and Magento endpoint.

For an existing project, edit its application settings in PHP.

```php
$application['mode'] = 'headless';
$application['magento']['storefrontOrigin'] = 'https://store.example.com';
$application['magento']['corsOrigins'] = ['https://store.example.com'];
```

Check a real GraphQL query, a product media URL and a credentialed request from the authorized frontend origin. Cookies, TLS certificates, DNS, cache invalidation and customer session settings should match the domains used by the shop.
