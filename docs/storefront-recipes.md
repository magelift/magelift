# Connect a storefront

## Use the native Magento storefront

Set `application.mode` to `integrated` to deploy Magento and its storefront
together through MageLift. Build your themes and modules, including static
assets for the locales you serve, into the shop image. Set the domain for
each environment and check the storefront, Admin and media URLs after
deploying. [Application builds](builds.md)

## Connect a headless storefront

A separate storefront uses Magento's GraphQL API. When setting up MageLift
in the Magento checkout, supply the origin where that frontend will run.
This example uses AWS for Magento and a staging frontend.

```sh
magelift init --provider aws --storefront-origin https://store.staging.example.com
magelift --env staging config effective
magelift env status staging
```

MageLift sets the storefront origin and the browser origins Magento allows
through CORS. After a successful rollout, `magelift deploy` prints the full
GraphQL URL. Give that URL to the frontend.
[Deploy Magento to staging](getting-started.md#build-and-deploy-to-staging)

For an existing project, set the mode in its PHP application settings.
Set the origin and CORS entries for each environment's frontend.

```php
$application['mode'] = 'headless';
$application['magento']['storefrontOrigin'] = 'https://store.staging.example.com';
$application['magento']['corsOrigins'] = ['https://store.staging.example.com'];
```

### Deploy the frontend with SST

MageLift manages the Magento backend, including its database, search and
media. SST can deploy the Next.js or Nuxt storefront alongside it. Configure
SST in the frontend checkout and give each frontend stage the GraphQL URL
of its corresponding MageLift environment. [SST stages](https://sst.dev/docs/basics/) · [SST Next.js component](https://sst.dev/docs/component/aws/nextjs/) · [SST Nuxt component](https://sst.dev/docs/component/aws/nuxt/)

If your Next.js storefront reads the endpoint at build time, set it in
the SST component's environment for that stage. Use the variable name
your frontend expects. This example uses a staging URL.

```ts
// Inside run() in sst.config.ts
new sst.aws.Nextjs("Storefront", {
  domain: "store.staging.example.com",
  environment: {
    MAGENTO_GRAPHQL_URL: "https://magento.staging.example.com/graphql",
  },
});
```

Run `sst deploy --stage staging` from the frontend checkout once its account
and domain are configured. For a Nuxt storefront, use the Nuxt component
and pass the endpoint through the app's configuration.

In CI, append `--outputs-file endpoints.json` to the MageLift deploy command
and pass the file to the frontend job. Its `graphqlUrl` field contains the
endpoint for the deployed environment. Read it into `MAGENTO_GRAPHQL_URL`
if the SST config uses that variable. Share one file per environment or PR.
The file contains the environment name and URLs.

Point each frontend preview at its own Magento preview and allow that
frontend's origin in Magento. MageLift manages Magento preview expiry.
Use the frontend's SST workflow to deploy and remove its stage.

Check a product query, a media URL and a credentialed browser request from
the configured frontend origin. Test checkout in staging if the storefront
handles it. Check DNS, TLS, cookies and customer sessions for the domains
the shop uses. If the frontend caches Magento responses, check how it
invalidates them after a change.
