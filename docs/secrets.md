# Secret references

Composer credentials must not appear as plaintext in YAML. Use a scheme that
matches `target.provider`.

## AWS (`target.provider: aws`)

```yaml
build:
  composer:
    credentials: aws-secrets-manager://magelift/composer
```

```yaml
build:
  composer:
    credentials: ssm:///magelift/composer
```

The value must be a non-empty Composer `auth.json` object. SSM parameters are
fetched with decryption. Names and paths use `defaults.region`; an ARN supplies
its own region.

To pick one string field from a JSON Secrets Manager secret:

```yaml
build:
  composer:
    credentials: aws-secrets-manager://magelift/shared?jsonField=composer
```

That field must hold the full Composer auth object.

## GCP (`target.provider: gcp`)

```yaml
build:
  composer:
    credentials: gcp-secret-manager://projects/PROJECT/secrets/NAME/versions/latest
```

`magelift build` resolves `gcp-secret-manager://` on a GCP target the same way
it resolves Secrets Manager and SSM on AWS: the value is written to a private
temp file for the prepare container, then deleted. Cross-provider schemes still
fail validation.

## How resolution works

The CLI writes the secret to a private temp file for the prepare container, then
deletes it. It never puts the value in process args, build metadata, the app
image, the artifact manifest, or Pulumi state.

Unknown schemes, fragments, userinfo, and stray query keys fail validation.
Cross-provider schemes (e.g. `gcp-secret-manager://` on an AWS target) fail too.
