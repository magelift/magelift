# Secret references

MageLift does not accept Composer credentials as plaintext YAML. Set
`build.composer.credentials` to one of these references:

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

The resolved value must be a non-empty Composer `auth.json` object. MageLift requests
encrypted SSM parameters with decryption enabled. It uses `defaults.region` for names
and parameter paths. A Secrets Manager or SSM ARN uses the region from the ARN.

Secrets Manager references may select a string field from a JSON secret with the
`jsonField` query parameter. The selected string must contain the complete Composer
authentication object:

```yaml
build:
  composer:
    credentials: aws-secrets-manager://magelift/shared?jsonField=composer
```

The CLI sends the value to the isolated prepare container through a private temporary
file. It does not place the value in process arguments, build metadata, the application
image, the artifact manifest, or Pulumi state. The temporary file is removed after the
runner exits.

Only AWS Secrets Manager and SSM Parameter Store are supported in v1. Unknown schemes,
fragments, user information, and provider-specific query options fail configuration
validation.
