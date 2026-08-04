// Single source of truth for landing-page copy.
// Voice rules: direct, technical, honest. No em dashes. Sentence case.
// Claims must stay verifiable against docs/ (capability-matrix, release-readiness).

export const docsUrl = '/docs/';
export const githubUrl = 'https://github.com/magelift/magelift';

export const nav = {
  links: [
    { label: 'Docs', href: docsUrl },
    { label: 'vs PaaS', href: `${docsUrl}compare-paas/` },
    { label: 'FAQ', href: `${docsUrl}faq/` },
    { label: 'GitHub', href: githubUrl },
  ],
  cta: { label: 'Install the CLI', href: `${docsUrl}install/` },
};

export const hero = {
  h1: 'Run Magento in your own AWS or GCP account.',
  lede: 'Open-source CLI with ACC/Upsun-shaped YAML. The store runs in your account, on your invoice.',
  primaryCta: { label: 'Install the CLI', href: `${docsUrl}install/` },
  secondaryCta: { label: 'Get started', href: `${docsUrl}getting-started/` },
};

export const terminal = {
  title: 'preview · eu-north-1',
  copyCommand: 'go install github.com/magelift/magelift/cmd/magelift@latest',
  lines: [
    { type: 'cmd', text: 'go install github.com/magelift/magelift/cmd/magelift@latest' },
    { type: 'cmd', text: 'magelift config validate --env preview' },
    { type: 'out', text: 'ok  magelift.yaml · compatibility checks passed', tone: 'ok' },
    { type: 'cmd', text: 'magelift preview --env preview' },
    { type: 'out', text: 'plan  ecs-fargate · digest promote · no mutate', tone: 'dim' },
    { type: 'cmd', text: 'magelift deploy --env preview --yes' },
    { type: 'out', text: 'deployed  https://preview.example.shop', tone: 'ok' },
    { type: 'cmd', text: 'magelift destroy --env preview --yes' },
    { type: 'out', text: 'destroyed · spend stopped', tone: 'dim' },
  ],
};

export const proof = [
  { k: 'License', v: 'Apache-2.0', note: 'Source, NOTICE, and a license check in CI.' },
  { k: 'AWS', v: 'ECS Fargate certified', note: 'Production path with acceptance evidence.' },
  { k: 'GCP', v: 'GKE Autopilot certified', note: 'Production path with acceptance evidence.' },
  { k: 'Measured', v: '10m31s', note: 'Time to preview on the documented eu-north-1 run.' },
];

export const personas = {
  heading: 'Who it is for',
  lede: 'MageLift is an open-source CLI and YAML config for Magento Open Source and Adobe Commerce in your AWS or GCP account. The YAML shape will feel familiar if you have used ACC or Upsun.',
  tabs: [
    {
      label: 'Agency tech lead',
      pain: 'Your PaaS bill grows with every shop you sign.',
      body: 'Each client gets previews you can spin up, share, and destroy when the work is done. Config follows the ACC/Upsun shape your team already reads, and magelift cost gives AWS shape estimates before you spend anything.',
      proof: 'Preview, share a URL, destroy. Protected destroys need --yes.',
    },
    {
      label: 'SME shop owner',
      pain: 'No DevOps hire, and no appetite for a blank Terraform repo.',
      body: 'The docs speak Magento, the certified targets are the only production paths, and your store lives in an AWS or GCP account that you own from day one. Weekend migration guides cover config, dump, media, and DNS cutover.',
      proof: 'Moving off ACC or Upsun starts with magelift init --from-acc or --from-upsun.',
    },
    {
      label: 'Platform engineer',
      pain: 'DIY stacks drift, and a PaaS hides the parts you need to audit.',
      body: 'IAM stays inside your account. Images build once and promote by digest. The capability matrix states in writing which cells are certified and which are experimental.',
      proof: 'Release archives ship with checksums and an SBOM.',
    },
  ],
};

export const how = {
  heading: 'From install to preview',
  lede: 'Certified path today: AWS ECS Fargate and GCP GKE Autopilot. Stay on free-tier-safe shapes until you mean to spend.',
  steps: [
    'Install with go install until the first release tag, then run magelift version.',
    'Copy examples/sample-shop/magelift.yaml, fill in account and secret refs, then magelift config validate.',
    'Bootstrap once, then preview and deploy on a certified target. Destroy when the spike is done.',
  ],
  links: [
    { label: 'Getting started', href: `${docsUrl}getting-started/` },
    { label: 'Leave PaaS in a weekend', href: `${docsUrl}weekend-migrate/` },
  ],
  yaml: [
    { text: 'project:' },
    { text: '  name: sample-shop' },
    { text: 'application:' },
    { text: '  edition: open-source' },
    { text: '  version: 2.4.9' },
    { text: 'target:' },
    { text: '  provider: aws' },
    { text: '  runtime: ecs-fargate' },
    { text: 'environments:' },
    { text: '  preview:' },
    { text: '    class: preview' },
    { text: '    domain: preview.example.com' },
    { text: '    monthlyBudgetCents: 5000', hl: true },
    { text: '    expiresAt: "2026-12-31T23:59:59Z"', hl: true },
    { text: '  production:' },
    { text: '    inherits: staging' },
    { text: '    preset: high-availability' },
    { text: '    protection: true' },
  ],
  yamlCaption: 'Trimmed from examples/sample-shop/magelift.yaml. Preview environments carry a budget cap and an expiry date.',
};

export const targets = {
  heading: 'Certified targets',
  lede: 'Only certified cells are production-supported. Experimental targets are labeled, and they stay labeled until they pass acceptance.',
  rows: [
    { target: 'AWS ECS Fargate', status: 'certified' },
    { target: 'GCP GKE Autopilot', status: 'certified' },
    { target: 'AWS EKS · OVH MKS · Scaleway Kapsule', status: 'experimental' },
  ],
  links: [
    { label: 'Capability matrix', href: `${docsUrl}capability-matrix/` },
    { label: 'Compare to ACC / Upsun', href: `${docsUrl}compare-paas/` },
  ],
};

export const trust = {
  heading: 'Boring where it counts',
  lede: 'There is no SaaS subscription for the CLI. The software is free; the cloud bill is yours.',
  items: [
    { h: 'Apache-2.0', p: 'Source and NOTICE in the repo. The license check runs in CI.' },
    { h: 'Digest promote', p: 'Build once, promote by digest. Mutable tags are not the default path.' },
    { h: 'Verifiable releases', p: 'The first tag ships archives with checksums and an SBOM.' },
    { h: 'Independent', p: 'Not affiliated with Adobe. Product names appear only to describe compatibility.' },
  ],
};

export const faqs = [
  {
    q: 'What is MageLift?',
    a: 'An open-source CLI plus ACC/Upsun-shaped YAML for Magento Open Source and Adobe Commerce in your AWS or GCP account. You run it against your own cloud; MageLift does not host stores.',
  },
  {
    q: 'Is MageLift a Magento hosting provider?',
    a: 'No. It is Apache-2.0 software you run in your account. ACC and Upsun rent you a platform; MageLift targets infrastructure you already control.',
  },
  {
    q: 'How much does MageLift cost?',
    a: 'The CLI is free under Apache-2.0. AWS or GCP bills you for compute, data, and network. Run magelift cost for AWS shape estimates, and destroy preview stacks when you are done with them.',
  },
  {
    q: 'Which clouds are certified for production?',
    a: 'AWS ECS Fargate and GCP GKE Autopilot. AWS EKS, OVH MKS, and Scaleway Kapsule are experimental and not production-supported yet.',
  },
  {
    q: 'Can I migrate from Adobe Commerce Cloud or Upsun?',
    a: 'Yes. Import config with magelift init --from-acc or --from-upsun. Dump, media, and DNS cutover are covered in the migration guides.',
  },
  {
    q: 'Is MageLift affiliated with Adobe?',
    a: 'No. MageLift is independent of Adobe Inc. Magento and Adobe Commerce are Adobe trademarks; the names are used only to describe compatibility.',
  },
];

export const finalCta = {
  heading: 'Read the docs first',
  body: 'Install takes one command. Validate the sample shop, then decide whether a certified preview belongs in your account.',
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
