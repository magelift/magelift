# Secret references

In `magelift.php`, set the Composer credential reference on the build array. Keep the contents of `auth.json` in the secret store for your target.

## AWS (`target.provider: aws`)

```php
$build['composer']['credentials'] = 'aws-secrets-manager://magelift/composer';
```

```php
$build['composer']['credentials'] = 'ssm:///magelift/composer';
```

The value must be a non-empty Composer `auth.json` object. SSM parameters are
fetched with decryption. Names and paths use `defaults.region`; an ARN supplies
its own region.

To pick one string field from a JSON Secrets Manager secret:

```php
$build['composer']['credentials'] = 'aws-secrets-manager://magelift/shared?jsonField=composer';
```

That field must hold the full Composer auth object.

## GCP (`target.provider: gcp`)

```php
$build['composer']['credentials'] = 'gcp-secret-manager://projects/PROJECT/secrets/NAME/versions/latest';
```

`magelift build` resolves `gcp-secret-manager://` on a GCP target the same way
it resolves Secrets Manager and SSM on AWS: the value is written to a private
temp file for the prepare container, then deleted. Cross-provider schemes still
fail validation.

## How resolution works

Only the prepare container needs these credentials. The CLI removes its private temporary file after the build and keeps the value out of the image, artifact manifest, process arguments and Pulumi state.

Unknown schemes, fragments, userinfo, and stray query keys fail validation.
Cross-provider schemes (e.g. `gcp-secret-manager://` on an AWS target) fail too.
