## ADDED Requirements

### Requirement: GCP campaign cells may stay warm until close

A named GCP Autopilot Magento campaign MAY set `MAGELIFT_GCP_ACCEPTANCE_KEEP=true` so long-running Autopilot, Cloud SQL, and Memorystore resources stay up while Magento cells iterate. KEEP MUST appear in evidence. When the campaign ends, the operator MUST destroy the cell and run the provider orphan assertion unless the user explicitly retains a debug cell.

#### Scenario: Second Magento cell reuses Autopilot

- **WHEN** the first Autopilot Magento preview is healthy and KEEP is set
- **THEN** a later queue or cache cell updates the same stack instead of creating a second GKE cluster

### Requirement: Non-GCP providers pack one origin per compute family

AWS campaign runs MAY KEEP one stack per compute family (Fargate, Managed Instances, EKS), mutate catalog cells in place, and destroy at session end. OVH and Scaleway MUST create at most one preview stack each and MUST NOT set KEEP. Parallelism MUST be across providers (separate worktrees and credentials), not N Magento shops in one bill.

#### Scenario: Worktrees overlap GCP and AWS waits

- **WHEN** GCP Autopilot KEEP is active and AWS credentials are isolated in another worktree
- **THEN** the AWS KEEP origin may provision while GCP Magento tests run, and each worktree uses a unique resource prefix

#### Scenario: AWS Amazon MQ is not a second shop

- **WHEN** Fargate Magento KEEP is healthy and the next cell is `amazon-mq`
- **THEN** the same prefix is updated and no second Fargate cluster is created
