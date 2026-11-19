package cli

const starterConfig = starterAWSConfig

const starterAWSConfig = `schemaVersion: 1
project:
  name: example-shop
application:
  edition: open-source
  version: 2.4.9
  mode: integrated
  webRuntime: nginx-fpm
  magento:
    frontName: admin
build:
  php: "8.5"
target:
  provider: aws
  runtime: ecs-fargate
  aws:
    catalog:
      # Certified cell: no managed search on first run. Managed search
      # proves in Phase 2; until then this avoids surprise AOSS bills.
      searchMode: disabled
defaults:
  region: eu-west-3
  preset: preview
environments:
  preview:
    account: "123456789012"
    class: preview
    preset: preview
    domain: preview.example.com
    expiresAt: "2026-12-31T23:59:59Z"
  staging:
    inherits: preview
    account: "123456789012"
    class: staging
    preset: standard
    domain: staging.example.com
  production:
    inherits: staging
    account: "210987654321"
    class: production
    preset: high-availability
    domain: example.com
    protection: true
extensions: {}
`

const starterGCPConfig = `# Alpha recipe starter (GCP GKE Autopilot preview). Fill in your GCP
# project and domains, then follow docs/onboarding.md (billing, domain,
# licenses, and SMTP relay prerequisites come first).
schemaVersion: 1
project:
  name: example-shop
application:
  edition: open-source
  version: 2.4.9
  mode: integrated
  webRuntime: nginx-fpm
  magento:
    frontName: admin
build:
  php: "8.5"
  staticContent:
    locales: [en_US]
    themes: [Magento/luma]
target:
  provider: gcp
  runtime: gke-autopilot
  gcp:
    project: example-gcp-project
    region: europe-west1
    # Search defaults to the recipe OpenSearch; pin openSearchMode: disabled
    # only to skip search on a first smoke run.
    # After magelift build, set imageDigest to the pushed shop digest.
    # The stack generates the Magento encryption key. Set
    # encryptionKeySecret only to reuse an existing Secret Manager secret.
# Outbound email: uncomment and point at your SMTP relay once the relay
# account exists (see docs/onboarding.md prerequisites). Preview
# environments stay disabled unless you set a mode explicitly.
#email:
#  mode: smtp
#  host: smtp.example.com
#  port: 587
#  username: mailer
#  from: shop@example.com
#  credential: gcp-secret-manager://projects/example-gcp-project/secrets/smtp-password/versions/latest
defaults:
  region: europe-west1
  preset: preview
environments:
  preview:
    class: preview
    preset: preview
    domain: preview.example.com
    expiresAt: "2026-12-31T23:59:59Z"
  staging:
    inherits: preview
    class: staging
    preset: standard
    domain: staging.example.com
  production:
    inherits: staging
    class: production
    preset: high-availability
    domain: example.com
    protection: true
extensions: {}
`
