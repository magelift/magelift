# Secret references

In `magelift.php`, set `build.composer.credentials` to a reference. The secret
must contain a non-empty Composer `auth.json` object.

## Environment variable

Use `env://COMPOSER_AUTH` to read an environment variable on the machine or CI
runner that builds the image. This works with every provider. Populate
`COMPOSER_AUTH` from your CI secret or workstation secret store.

```php
$build['composer']['credentials'] = 'env://COMPOSER_AUTH';
```

## AWS (`target.provider: aws`)

```php
$build['composer']['credentials'] = 'aws-secrets-manager://magelift/composer';
```

```php
$build['composer']['credentials'] = 'ssm:///magelift/composer';
```

MageLift decrypts SSM parameters when it fetches them. A parameter name or
path uses `defaults.region`. A parameter ARN supplies its own region.

To pick one string field from a JSON Secrets Manager secret:

```php
$build['composer']['credentials'] = 'aws-secrets-manager://magelift/shared?jsonField=composer';
```

That field must hold the full Composer auth object.

## GCP (`target.provider: gcp`)

```php
$build['composer']['credentials'] = 'gcp-secret-manager://projects/PROJECT/secrets/NAME/versions/latest';
```

A GCP reference includes the project, secret and version. The build identity
needs permission to read that version.

## How resolution works

MageLift passes the value to the prepare container in a private temporary file
and deletes the file after the build. Use a scheme supported by the selected
provider. [Application builds](builds.md#composer)
