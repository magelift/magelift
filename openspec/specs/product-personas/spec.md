# Product Personas Specification

## Purpose

Indexes Magento SME and agency operator journeys so gaps stay attached to a named persona instead of restating deploy, WAF, or health requirements that already live in other capabilities.

## Requirements

### Requirement: Personas are an index, not a second product contract

The persona catalog MUST list each in-scope operator and the Magelift commands, YAML surfaces, and other spec IDs that close their journey. It MUST record a gap ID when a journey is not green. It MUST NOT copy Magento-safe WAF, deploy DAG, or health-layer requirements into this capability.

#### Scenario: An agent looks up the in-house Magento PHP SME

- **WHEN** an implementer reads this capability for the in-house Magento PHP SME who left Adobe Commerce Cloud or Upsun
- **THEN** the index names `init` or import, `doctor`, `local`, `bootstrap`, `deploy`, `health`, `logs`, `exec`, dump, and `destroy`, points at `magento-runtime-config`, `local-runtime-and-preflight`, and `cli-contract`, and does not restate those specs' requirements

### Requirement: Primary no-DevOps journeys are listed

The index MUST include: in-house Magento PHP SME; agency Magento developer; agency tech lead; integrated Magento PHP storefront team; headless backend team; QA using previews; on-call Magento developer; merchant who hired a freelancer; Magento Open Source-only SME; Adobe Commerce licensee.

#### Scenario: Headless backend team does not deploy a JS storefront

- **WHEN** the headless backend persona runs Magelift
- **THEN** Magento GraphQL, REST, admin, and media are in scope, Next.js and PWA Studio remain outside the CLI, and CORS plus dual hostnames are specified in `magento-runtime-config`

#### Scenario: MOS-only SME cannot claim Commerce

- **WHEN** the project is Magento Open Source and no Commerce artifact credentials exist
- **THEN** Open Source deploy may proceed on a certified target and no Adobe Commerce certification row is generated

### Requirement: Additional journeys are listed with gap IDs

The index MUST also include: EU merchant or DPO; multi-website merchandiser; generic Magento B2B extra consumers; ACC import versus Upsun import as two maps; brownfield inheritor; agency FinOps; Magento module vendor; coding agent; CI; security reviewer; power user; community provider author; MageLift contributor.

#### Scenario: Power user hits an uncertified catalog knob

- **WHEN** a power user selects an experimental cell such as ECS Managed Instances
- **THEN** planning warns that the cell is MageLift-experimental as specified by `compatibility-catalog`, proceeds, and does not claim certified

#### Scenario: Coding agent uses only user skills

- **WHEN** a coding agent installs Magelift skills into a Magento repository
- **THEN** only `agents/skills/` user skills install, and contributor skills are refused as specified in `skill-installation`

### Requirement: The laptop-to-production path stays Magelift-only

For the in-house Magento PHP SME, the documented happy path MUST be YAML plus Magelift commands from first clone through local Docker, preview, staging or UAT, and production, without Pulumi, Terraform, or raw cloud CLI recipes.

#### Scenario: First-run Magento PHP developer

- **WHEN** the user runs `magelift init` or import, `doctor`, `local up`, `bootstrap`, and `deploy` on a certified target
- **THEN** each step is a Magelift command, Pulumi is not required on the command line, and a missing Docker install is offered only through confirmed `doctor --install-dependencies`
