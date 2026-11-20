export const docsUrl = '/docs/';
export const githubUrl = 'https://github.com/magelift/magelift';

export const nav = {
  links: [
    { label: 'Docs', href: docsUrl },
    { label: 'Compare hosting', href: `${docsUrl}compare-paas/` },
    { label: 'FAQ', href: `${docsUrl}faq/` },
    { label: 'GitHub', href: githubUrl },
  ],
  cta: { label: 'Install the CLI', href: `${docsUrl}install/` },
};

export const hero = {
  h1: 'Deploy Magento 2 to your own cloud',
  lede: 'Run your Magento checkout with Docker on your laptop. Set up your environments in PHP and deploy to your AWS, Google Cloud, or Azure account.',
  primaryCta: { label: 'Install the CLI', href: `${docsUrl}install/` },
  secondaryCta: { label: 'Get started', href: `${docsUrl}getting-started/` },
};

export const terminal = {
  title: 'Inspect the sample project',
  copyCommand: 'git clone --depth 1 https://github.com/magelift/magelift.git && cd magelift && go install ./cmd/magelift',
  lines: [
    { type: 'cmd', text: 'git clone --depth 1 https://github.com/magelift/magelift.git' },
    { type: 'cmd', text: 'cd magelift && go install ./cmd/magelift' },
    { type: 'cmd', text: 'cd examples/sample-shop' },
    { type: 'cmd', text: 'magelift --env preview config validate' },
    { type: 'cmd', text: 'magelift env diff staging production' },
  ],
};

export const proof = [
  { k: 'License', v: 'Apache-2.0', note: 'Your cloud provider bills you directly.' },
  { k: 'AWS', v: 'ECS Fargate', note: 'Recorded Magento deployments.' },
  { k: 'Google Cloud', v: 'GKE Autopilot', note: 'Recorded Magento deployments.' },
  { k: 'Azure', v: 'AKS', note: 'Tested with Pulumi mocks and Floci.' },
];

export const personas = {
  heading: 'Work on your shop',
  lede: 'Keep staging, production, and preview settings in one PHP project.',
  tabs: [
    {
      label: 'Review a change',
      pain: 'Give a pull request its own URL',
      body: 'Apply magelift-preview after reviewing the pull request. The GitHub workflow deploys that revision. Close the pull request or remove the label to schedule teardown.',
      proof: 'magelift env diff staging production',
    },
    {
      label: 'Move a shop',
      pain: 'Start with your existing settings',
      body: 'Import settings from Upsun, Adobe Commerce Cloud, or Docker Compose. Rehearse the database and media transfer in staging before switching traffic.',
      proof: 'magelift init --from-upsun --provider gcp',
    },
    {
      label: 'Connect a storefront',
      pain: 'Use Magento as the backend',
      body: 'For a headless shop, set the storefront origin in PHP and use the Magento GraphQL URL for the environment.',
      proof: 'magelift env status staging',
    },
  ],
};

export const how = {
  heading: 'From your laptop to staging',
  lede: 'MageLift reads the Magento edition and version from your Composer lockfile.',
  steps: [
    'Install the CLI, then open your Magento repository.',
    'Run magelift init with your cloud choice, then start Magento locally with magelift dev.',
    'Connect the cloud account. Push the shop image to your registry and review the changes before deploying.',
  ],
  links: [
    { label: 'Getting started', href: `${docsUrl}getting-started/` },
    { label: 'Deploy to Google Cloud', href: `${docsUrl}onboarding/` },
    { label: 'Migration checklist', href: `${docsUrl}weekend-migrate/` },
  ],
  project: [
    { text: '<?php' },
    { text: 'use MageLift\\{Environment, Project, Service};' },
    { text: '' },
    { text: '$db = new Service(family: "mysql", version: "8.4");', hl: true },
    { text: 'return new Project([' },
    { text: '    "services" => ["database" => $db],' },
    { text: '    "environments" => [' },
    { text: '        "preview" => new Environment([' },
    { text: '            "previewTtl" => "168h",' },
    { text: '        ], class: "preview"),' },
    { text: '        "production" => new Environment([' },
    { text: '            "protection" => true,' },
    { text: '        ], preset: "high-availability"),', hl: true },
    { text: '    ],' },
    { text: ']);' },
  ],
  projectCaption: 'The preview expires after seven days. Both environments use MySQL 8.4. Production uses the high-availability preset.',
};

export const targets = {
  heading: 'Choose a cloud',
  lede: 'The run records cover Magento on ECS Fargate and GKE Autopilot. Local tests check the AKS resources and Azure API calls.',
  rows: [
    { target: 'AWS ECS Fargate', status: 'implemented' },
    { target: 'Google Cloud GKE Autopilot', status: 'implemented' },
    { target: 'Azure AKS', status: 'experimental' },
    { target: 'AWS EKS', status: 'experimental' },
    { target: 'Google Cloud GKE Standard', status: 'experimental' },
  ],
  links: [
    { label: 'Provider support', href: `${docsUrl}capability-matrix/` },
    { label: 'Compare hosting', href: `${docsUrl}compare-paas/` },
  ],
};

export const trust = {
  heading: 'Billing and releases',
  lede: 'Your team keeps the application image in its own registry. The cloud provider bills your account.',
  items: [
    { h: 'License', p: 'The CLI is free under the Apache 2.0 license.' },
    { h: 'Image promotion', p: 'Staging and production can deploy the same signed image digest.' },
    { h: 'Signed releases', p: 'The release workflow signs checksums and lists the dependencies in each build.' },
    { h: 'Adobe affiliation', p: 'MageLift is independent of Adobe.' },
  ],
};

export const faqs = [
  {
    q: 'What is MageLift?',
    a: 'MageLift runs Magento Open Source or licensed Adobe Commerce in your own cloud account. Your team defines services and environments in PHP.',
  },
  {
    q: 'Is MageLift a Magento hosting provider?',
    a: 'The store runs in your cloud account. MageLift creates the infrastructure there and deploys Magento.',
  },
  {
    q: 'How much does MageLift cost?',
    a: 'The CLI is free. Your cloud provider charges for resources while they exist. On AWS, magelift cost estimates the selected configuration before deployment.',
  },
  {
    q: 'Which cloud targets can I use?',
    a: 'Start with ECS Fargate on AWS, GKE Autopilot on Google Cloud, or experimental AKS support on Azure. Consult the provider guide for the tests and operations available on each runtime.',
  },
  {
    q: 'Can I migrate from Adobe Commerce Cloud or Upsun?',
    a: 'Run magelift init --from-acc or --from-upsun and choose AWS, GCP or Azure. Review the generated PHP and any settings the importer could not map. Follow the migration guide to move the encryption key, database and media before switching traffic.',
  },
  {
    q: 'Is MageLift affiliated with Adobe?',
    a: 'MageLift is an independent project. Magento and Adobe Commerce are trademarks of Adobe Inc.',
  },
];

export const finalCta = {
  heading: 'Try MageLift locally',
  body: 'Use an existing Magento checkout with PHP and Docker. You can connect the cloud account later.',
  primaryCta: { label: 'Install the CLI', href: `${docsUrl}install/` },
  secondaryCta: { label: 'Get started', href: `${docsUrl}getting-started/` },
};

export const footer = {
  links: [
    { label: 'Docs', href: docsUrl },
    { label: 'Install', href: `${docsUrl}install/` },
    { label: 'FAQ', href: `${docsUrl}faq/` },
    { label: 'Pricing', href: '/pricing.md' },
    { label: 'llms.txt', href: '/llms.txt' },
    { label: 'GitHub', href: githubUrl },
    { label: 'Security', href: `${githubUrl}/blob/main/SECURITY.md` },
    { label: 'Support', href: `${githubUrl}/blob/main/SUPPORT.md` },
    { label: 'Code of Conduct', href: `${githubUrl}/blob/main/CODE_OF_CONDUCT.md` },
    { label: 'Contributing', href: `${githubUrl}/blob/main/CONTRIBUTING.md` },
    { label: 'License', href: `${githubUrl}/blob/main/LICENSE` },
  ],
  trademark:
    'MageLift is independent of Adobe Inc. Magento and Adobe Commerce are Adobe trademarks, used only to describe compatibility.',
};
