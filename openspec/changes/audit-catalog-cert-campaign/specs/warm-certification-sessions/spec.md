## ADDED Requirements

### Requirement: Campaign KEEP is explicit and time-bounded

Warm reuse on GCP during `audit-catalog-cert-campaign` MUST fingerprint the Autopilot Magento cell and MAY retain it only while `MAGELIFT_GCP_ACCEPTANCE_KEEP=true`. AWS MUST fingerprint each compute family (Fargate, Managed Instances, EKS) and MAY retain them only while `MAGELIFT_AWS_ACCEPTANCE_KEEP=true` for this campaign. A crash MUST leave the KEEP cell in the checkpoint so the next invoke resumes instead of double-provisioning. Closing the campaign with KEEP disabled MUST destroy the shared stacks once. OVH and Scaleway MUST NOT set KEEP.

#### Scenario: Resume after interrupt

- **WHEN** the process stops after Autopilot is up with KEEP set
- **THEN** the next invoke with the same fingerprint attaches to that cluster and does not create a second Autopilot cluster

#### Scenario: AWS Fargate KEEP reuses the origin

- **WHEN** Fargate Magento is healthy with AWS KEEP set and the next cell is `amazon-mq`
- **THEN** the runner attaches to the same prefix and does not create a second Fargate cluster
