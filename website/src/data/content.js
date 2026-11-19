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
  h1: 'Run Magento in your own cloud account.',
  lede: 'Define your Magento services in PHP. Run the shop locally, share a preview for review, and promote the same image to production.',
  primaryCta: { label: 'Install the CLI', href: `${docsUrl}install/` },
  secondaryCta: { label: 'Get started', href: `${docsUrl}getting-started/` },
};

export const terminal = {
  title: 'Try the sample shop locally',
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
  heading: 'Working with a shop',
  lede: 'The same PHP project describes staging, UAT, production and pull-request previews.',
  tabs: [
    {
      label: 'Review a change',
      pain: 'Give reviewers a shop of their own',
      body: 'Apply the magelift-preview label to a reviewed pull request in your repository. The generated workflow deploys that revision and removes the preview when you remove the label, close the PR or reach its expiry.',
      proof: 'magelift env diff staging production',
    },
    {
      label: 'Move a shop',
      pain: 'Bring an existing Magento project',
      body: 'Bring service versions, session settings and search plugins from Upsun or Adobe Commerce Cloud. For a Compose shop, import its service images. Rehearse the SQL and media transfer in staging.',
      proof: 'magelift init --from-upsun --provider gcp',
    },
    {
      label: 'Connect a storefront',
      pain: 'Use Magento behind your frontend',
      body: 'Keep the native Magento storefront or host a headless frontend separately. Configure its allowed origin in PHP and look up the Magento GraphQL endpoint for each environment.',
      proof: 'magelift env status staging',
    },
  ],
};

export const how = {
  heading: 'Set up a shop',
  lede: 'Start with your Magento checkout, or add --new to have Composer create a shop.',
  steps: [
    'Install the CLI from the MageLift checkout with go install ./cmd/magelift.',
    'Run magelift init --provider aws in your shop repository, or choose gcp or azure. Start Magento locally with magelift dev.',
    'Connect your cloud account, push a signed shop image to your registry and review the infrastructure changes with magelift preview.',
  ],
  links: [
    { label: 'Getting started', href: `${docsUrl}getting-started/` },
    { label: 'GCP onboarding', href: `${docsUrl}onboarding/` },
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
  projectCaption: 'Both environments inherit MySQL 8.4. Production uses the high-availability preset.',
};

export const targets = {
  heading: 'Cloud targets',
  lede: 'Compare the tests and recorded runs for each cloud adapter.',
  rows: [
    { target: 'AWS ECS Fargate', status: 'implemented' },
    { target: 'GCP GKE Autopilot', status: 'implemented' },
    { target: 'Azure AKS', status: 'experimental' },
    { target: 'AWS EKS · GCP GKE Standard · OVH MKS · Scaleway Kapsule', status: 'experimental' },
  ],
  links: [
    { label: 'Capability matrix', href: `${docsUrl}capability-matrix/` },
    { label: 'Compare to ACC / Upsun', href: `${docsUrl}compare-paas/` },
  ],
};

export const trust = {
  heading: 'Billing and releases',
  lede: 'You choose the registry and the cloud account that pay for each shop.',
  items: [
    { h: 'Apache-2.0', p: 'The open-source CLI carries no license fee.' },
    { h: 'Image promotion', p: 'Staging and production can deploy the same signed image digest.' },
    { h: 'Verifiable releases', p: 'The release workflow signs checksums and generates a software bill of materials.' },
    { h: 'Independent', p: 'MageLift is independent of Adobe.' },
  ],
};

export const faqs = [
  {
    q: 'What is MageLift?',
    a: 'MageLift runs Magento Open Source or licensed Adobe Commerce in your own cloud account. Your team defines services and environments in PHP.',
  },
  {
    q: 'Is MageLift a Magento hosting provider?',
    a: 'Your team owns the cloud account. MageLift uses a signed plugin for AWS, Google Cloud or Azure to deploy and operate Magento there.',
  },
  {
    q: 'How much does MageLift cost?',
    a: 'The CLI is free. Your cloud provider charges for resources while they exist. On AWS, magelift cost estimates the selected configuration before deployment.',
  },
  {
    q: 'Which cloud targets can I use?',
    a: 'AWS ECS Fargate and GCP GKE Autopilot have recorded Magento runs. Azure AKS has resource tests with Pulumi mocks and API tests with Floci. The capability matrix lists the evidence for each path.',
  },
  {
    q: 'Can I migrate from Adobe Commerce Cloud or Upsun?',
    a: 'Yes. Run magelift init --from-acc or --from-upsun and choose AWS, GCP or Azure. Review the generated PHP and the list of settings that need attention. The migration guide covers the encryption key, database, media and traffic switch.',
  },
  {
    q: 'Is MageLift affiliated with Adobe?',
    a: 'MageLift is an independent project. Magento and Adobe Commerce are trademarks of Adobe Inc.',
  },
];

export const finalCta = {
  heading: 'Try MageLift locally',
  body: 'Validate the sample project or start Magento in your existing shop with Docker Compose.',
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
