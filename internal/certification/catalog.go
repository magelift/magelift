package certification

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

// CapabilitySourceMaxAge is intentionally finite. Provider product catalogs,
// preview labels, regional availability, and service limits change often
// enough that a copied source link is not durable evidence by itself.
const CapabilitySourceMaxAge = 90 * 24 * time.Hour

type AdobeCapabilityStatus string

const (
	AdobeCapabilitySupported   AdobeCapabilityStatus = "adobe-supported"
	AdobeCapabilityMageLift    AdobeCapabilityStatus = "magelift-compatible"
	AdobeCapabilityUnsupported AdobeCapabilityStatus = "unsupported"
	AdobeCapabilityUnavailable AdobeCapabilityStatus = "unavailable"
	AdobeCapabilityUnknown     AdobeCapabilityStatus = "unknown"
)

// CapabilitySource is the source-of-truth pointer for one provider fact. The
// catalog stores a retrieval date rather than pretending that a URL is
// immutable documentation.
type CapabilitySource struct {
	URL           string `json:"url" yaml:"url"`
	RetrievedAt   string `json:"retrievedAt" yaml:"retrievedAt"`
	ReferenceDate string `json:"referenceDate,omitempty" yaml:"referenceDate,omitempty"`
}

// CapabilityRecord keeps four claims independent: Adobe compatibility,
// provider availability, MageLift implementation, and live certification.
// A nearby product may satisfy one claim without satisfying the others.
type CapabilityRecord struct {
	ID                  string                `json:"id" yaml:"id"`
	Provider            string                `json:"provider" yaml:"provider"`
	Region              string                `json:"region" yaml:"region"`
	Zones               []string              `json:"zones,omitempty" yaml:"zones,omitempty"`
	Service             string                `json:"service" yaml:"service"`
	Role                string                `json:"role" yaml:"role"`
	ServiceMajor        string                `json:"serviceMajor,omitempty" yaml:"serviceMajor,omitempty"`
	Lifecycle           string                `json:"lifecycle" yaml:"lifecycle"`
	Limits              string                `json:"limits,omitempty" yaml:"limits,omitempty"`
	AdobeStatus         AdobeCapabilityStatus `json:"adobeStatus" yaml:"adobeStatus"`
	ProviderStatus      CapabilityStatus      `json:"providerStatus" yaml:"providerStatus"`
	MageLiftStatus      CapabilityStatus      `json:"mageliftStatus" yaml:"mageliftStatus"`
	CertificationStatus CapabilityStatus      `json:"certificationStatus" yaml:"certificationStatus"`
	Source              CapabilitySource      `json:"source" yaml:"source"`
	EvidenceID          string                `json:"evidenceId,omitempty" yaml:"evidenceId,omitempty"`
	Reason              string                `json:"reason,omitempty" yaml:"reason,omitempty"`
}

type CapabilityCatalog struct {
	Version   string             `json:"version" yaml:"version"`
	UpdatedAt string             `json:"updatedAt" yaml:"updatedAt"`
	Records   []CapabilityRecord `json:"records" yaml:"records"`
}

type CapabilityCoverageRow struct {
	ID                  string           `json:"id" yaml:"id"`
	Provider            string           `json:"provider" yaml:"provider"`
	Region              string           `json:"region" yaml:"region"`
	Service             string           `json:"service" yaml:"service"`
	Role                string           `json:"role" yaml:"role"`
	Status              CapabilityStatus `json:"status" yaml:"status"`
	ProviderStatus      CapabilityStatus `json:"providerStatus" yaml:"providerStatus"`
	MageLiftStatus      CapabilityStatus `json:"mageliftStatus" yaml:"mageliftStatus"`
	CertificationStatus CapabilityStatus `json:"certificationStatus" yaml:"certificationStatus"`
	Limits              string           `json:"limits" yaml:"limits"`
	SourceURL           string           `json:"sourceUrl" yaml:"sourceUrl"`
	RetrievedAt         string           `json:"retrievedAt" yaml:"retrievedAt"`
	ReferenceDate       string           `json:"referenceDate,omitempty" yaml:"referenceDate,omitempty"`
	Reason              string           `json:"reason,omitempty" yaml:"reason,omitempty"`
}

type CapabilityCoverageReport struct {
	CatalogVersion string                   `json:"catalogVersion" yaml:"catalogVersion"`
	GeneratedAt    string                   `json:"generatedAt" yaml:"generatedAt"`
	Counts         map[CapabilityStatus]int `json:"counts" yaml:"counts"`
	Rows           []CapabilityCoverageRow  `json:"rows" yaml:"rows"`
}

const (
	CapabilitySupported CapabilityStatus = "supported"
	CapabilityCertified CapabilityStatus = "certified"
	CapabilityBlocked   CapabilityStatus = "blocked"
	CapabilityNotRun    CapabilityStatus = "not-run"
)

// Validate checks catalog shape and source freshness without contacting any
// provider. A caller that wants a historical report can pass the report's
// evidence timestamp as now.
func (c CapabilityCatalog) Validate(now time.Time) error {
	if strings.TrimSpace(c.Version) == "" {
		return errors.New("capability catalog version is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if strings.TrimSpace(c.UpdatedAt) == "" {
		return errors.New("capability catalog updatedAt is required")
	}
	if _, err := parseCatalogDate(c.UpdatedAt); err != nil {
		return fmt.Errorf("capability catalog updatedAt: %w", err)
	}
	if len(c.Records) == 0 {
		return errors.New("capability catalog has no records")
	}
	seen := make(map[string]struct{}, len(c.Records))
	for _, record := range c.Records {
		if _, exists := seen[record.ID]; exists {
			return fmt.Errorf("duplicate capability record %q", record.ID)
		}
		seen[record.ID] = struct{}{}
		if err := record.validate(now); err != nil {
			return fmt.Errorf("capability %q: %w", record.ID, err)
		}
	}
	return nil
}

func (r CapabilityRecord) validate(now time.Time) error {
	for field, value := range map[string]string{
		"id": r.ID, "provider": r.Provider, "region": r.Region,
		"service": r.Service, "role": r.Role, "lifecycle": r.Lifecycle,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", field)
		}
	}
	if r.AdobeStatus == "" || !validAdobeStatus(r.AdobeStatus) {
		return fmt.Errorf("invalid Adobe status %q", r.AdobeStatus)
	}
	for name, value := range map[string]CapabilityStatus{
		"provider": r.ProviderStatus, "MageLift": r.MageLiftStatus, "certification": r.CertificationStatus,
	} {
		if !validCapabilityStatus(value) {
			return fmt.Errorf("invalid %s status %q", name, value)
		}
	}
	if r.CertificationStatus != CapabilityCertified && strings.TrimSpace(r.Reason) == "" {
		return errors.New("non-certified records require a reason")
	}
	if strings.TrimSpace(r.Source.URL) == "" {
		return errors.New("source URL is required")
	}
	if strings.TrimSpace(r.Limits) == "" {
		return errors.New("limits are required")
	}
	parsedURL, err := url.Parse(r.Source.URL)
	if err != nil || parsedURL.Scheme != "https" || parsedURL.Host == "" {
		return fmt.Errorf("source URL must be an HTTPS URL: %q", r.Source.URL)
	}
	retrieved, err := parseCatalogDate(r.Source.RetrievedAt)
	if err != nil {
		return fmt.Errorf("source retrievedAt: %w", err)
	}
	if retrieved.After(now.Add(24 * time.Hour)) {
		return errors.New("source retrievedAt is in the future")
	}
	if now.Sub(retrieved) > CapabilitySourceMaxAge {
		return fmt.Errorf("source retrievedAt %s is older than %s", r.Source.RetrievedAt, CapabilitySourceMaxAge)
	}
	if strings.TrimSpace(r.Source.ReferenceDate) != "" {
		if _, err := parseCatalogDate(r.Source.ReferenceDate); err != nil {
			return fmt.Errorf("source referenceDate: %w", err)
		}
	}
	return nil
}

func (r CapabilityRecord) effectiveStatus() CapabilityStatus {
	if r.CertificationStatus == CapabilityCertified {
		return CapabilityCertified
	}
	if r.AdobeStatus == AdobeCapabilityUnsupported {
		return CapabilityUnsupported
	}
	if r.AdobeStatus == AdobeCapabilityUnavailable {
		return CapabilityUnavailable
	}
	if r.MageLiftStatus == CapabilityUnsupported || r.MageLiftStatus == CapabilityUnavailable || r.MageLiftStatus == CapabilityBlocked {
		return r.MageLiftStatus
	}
	if r.ProviderStatus == CapabilityUnavailable || r.ProviderStatus == CapabilityUnsupported || r.ProviderStatus == CapabilityBlocked {
		return r.ProviderStatus
	}
	if r.MageLiftStatus == CapabilityExperimental || r.CertificationStatus == CapabilityExperimental {
		return CapabilityExperimental
	}
	return CapabilitySupported
}

func (c CapabilityCatalog) Coverage(now time.Time) (CapabilityCoverageReport, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if err := c.Validate(now); err != nil {
		return CapabilityCoverageReport{}, err
	}
	report := CapabilityCoverageReport{
		CatalogVersion: c.Version,
		GeneratedAt:    now.UTC().Format(time.RFC3339),
		Counts:         make(map[CapabilityStatus]int),
		Rows:           make([]CapabilityCoverageRow, 0, len(c.Records)),
	}
	for _, record := range c.Records {
		status := record.effectiveStatus()
		report.Counts[status]++
		report.Rows = append(report.Rows, CapabilityCoverageRow{
			ID: record.ID, Provider: record.Provider, Region: record.Region,
			Service: record.Service, Role: record.Role, Status: status,
			ProviderStatus: record.ProviderStatus, MageLiftStatus: record.MageLiftStatus,
			CertificationStatus: record.CertificationStatus,
			Limits:              record.Limits,
			SourceURL:           record.Source.URL, RetrievedAt: record.Source.RetrievedAt,
			ReferenceDate: record.Source.ReferenceDate,
			Reason:        record.Reason,
		})
	}
	sort.Slice(report.Rows, func(i, j int) bool { return report.Rows[i].ID < report.Rows[j].ID })
	return report, nil
}

func (c CapabilityCatalog) Find(provider, region, service, major string) (CapabilityRecord, bool) {
	for _, record := range c.Records {
		if record.Provider != provider || record.Service != service || record.ServiceMajor != major {
			continue
		}
		if record.Region != region && record.Region != "*" {
			continue
		}
		return record, true
	}
	return CapabilityRecord{}, false
}

// CurrentCapabilityCatalog is the source-dated catalog used by local profile
// validation. The records intentionally contain unavailable and experimental
// rows; omission would make a coverage report look more complete than it is.
func CurrentCapabilityCatalog() CapabilityCatalog {
	catalog := CapabilityCatalog{Version: "2026-08-12", UpdatedAt: "2026-08-12", Records: currentCapabilityRecords()}
	catalog.Records = append([]CapabilityRecord(nil), catalog.Records...)
	for i := range catalog.Records {
		catalog.Records[i].Zones = append([]string(nil), catalog.Records[i].Zones...)
	}
	return catalog
}

func currentCapabilityRecords() []CapabilityRecord {
	const (
		awsECS        = "https://docs.aws.amazon.com/AmazonECS/latest/developerguide/capacity-launch-type-comparison.html"
		awsEKS        = "https://docs.aws.amazon.com/eks/latest/userguide/automode.html"
		awsEKSManaged = "https://docs.aws.amazon.com/eks/latest/userguide/managed-node-groups.html"
		awsEKSFargate = "https://docs.aws.amazon.com/eks/latest/userguide/fargate.html"
		awsEKSSelf    = "https://docs.aws.amazon.com/eks/latest/userguide/worker.html"
		awsRDS        = "https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/CHAP_MySQL.html"
		awsCache      = "https://docs.aws.amazon.com/AmazonElastiCache/latest/dg/engine-versions.html"
		awsSearch     = "https://docs.aws.amazon.com/opensearch-service/latest/developerguide/what-is.html"
		awsMQ         = "https://docs.aws.amazon.com/amazon-mq/latest/developer-guide/rabbitmq-version-management.html"
		awsS3         = "https://docs.aws.amazon.com/AmazonS3/latest/userguide/Welcome.html"
		awsSecrets    = "https://docs.aws.amazon.com/secretsmanager/latest/userguide/intro.html"
		awsCW         = "https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/WhatIsCloudWatch.html"
		awsEKSObs     = "https://docs.aws.amazon.com/eks/latest/userguide/cloudwatch.html"
		awsCF         = "https://docs.aws.amazon.com/AmazonCloudFront/latest/DeveloperGuide/Introduction.html"
		awsWAF        = "https://docs.aws.amazon.com/waf/latest/developerguide/waf-chapter.html"
		fckNat        = "https://fck-nat.dev/stable/deploying/"
		gcpGKE        = "https://docs.cloud.google.com/kubernetes-engine/docs/concepts/choose-cluster-mode"
		gcpSQL        = "https://cloud.google.com/sql/docs/mysql/"
		gcpValkey     = "https://docs.cloud.google.com/memorystore/docs/valkey/supported-versions"
		gcpPubSub     = "https://docs.cloud.google.com/pubsub/docs/replay-overview?hl=en"
		gcpStorage    = "https://cloud.google.com/storage/docs/introduction"
		gcpSecrets    = "https://cloud.google.com/secret-manager/docs/overview"
		gcpObs        = "https://docs.cloud.google.com/kubernetes-engine/docs/concepts/observability"
		gcpEdge       = "https://docs.cloud.google.com/load-balancing/docs/https"
		scwK8s        = "https://www.scaleway.com/en/docs/kubernetes/"
		scwDB         = "https://www.scaleway.com/en/docs/managed-databases/mysql/"
		scwRedis      = "https://www.scaleway.com/en/docs/managed-databases-for-redis/concepts/"
		scwStorage    = "https://www.scaleway.com/en/docs/object-storage/"
		scwSecrets    = "https://www.scaleway.com/en/developers/api/secret-manager"
		scwObs        = "https://www.scaleway.com/en/docs/cockpit/"
		scwEdge       = "https://www.scaleway.com/en/docs/edge-services/concepts/"
		scwLB         = "https://www.scaleway.com/en/docs/load-balancer/reference-content/kubernetes-load-balancer/"
		ovhK8s        = "https://docs.ovhcloud.com/en/guides/public-cloud/containers-orchestration/managed-kubernetes/mks-plans"
		ovhK8sNodes   = "https://docs.ovhcloud.com/en/guides/public-cloud/containers-orchestration/managed-kubernetes/managing-nodes"
		ovhMySQL      = "https://docs.ovhcloud.com/en/guides/public-cloud/databases/mysql-capabilities"
		ovhValkey     = "https://docs.ovhcloud.com/en/guides/public-cloud/databases/redis-capabilities"
		ovhStorage    = "https://docs.ovhcloud.com/en/guides/public-cloud/storage/object-storage/"
		ovhKMS        = "https://docs.ovhcloud.com/en/guides/manage-and-operate/kms/okms-authentication-methods"
		ovhSecretMgr  = "https://docs.ovhcloud.com/en/guides/manage-and-operate/secret-manager/rest-api"
		ovhObs        = "https://docs.ovhcloud.com/en/guides/public-cloud/containers-orchestration/managed-kubernetes/forwarding-audit-logs"
		ovhEdge       = "https://www.ovhcloud.com/en/web-hosting/options/cdn/"
		ovhLB         = "https://help.ovhcloud.com/csm/en-public-cloud-kubernetes-expose-applications-using-load-balancer?id=KB0062866"
		fastlyDocs    = "https://www.fastly.com/documentation/reference/cli/"
		newRelicOTLP  = "https://docs.newrelic.com/docs/opentelemetry/best-practices/opentelemetry-otlp/"
		newRelicECS   = "https://docs.newrelic.com/docs/opentelemetry/integrations/ecs-monitoring/overview/"
		newRelicK8s   = "https://docs.newrelic.com/docs/kubernetes-pixie/k8s-otel/install/"
	)
	var records []CapabilityRecord
	const retrievedAt = "2026-08-09"
	add := func(id, provider, region, service, role, major, lifecycle, source string, adobe AdobeCapabilityStatus, providerStatus, mageLiftStatus, certificationStatus CapabilityStatus, reason string) {
		records = append(records, CapabilityRecord{
			ID: id, Provider: provider, Region: region, Service: service, Role: role,
			ServiceMajor: major, Lifecycle: lifecycle, AdobeStatus: adobe,
			ProviderStatus: providerStatus, MageLiftStatus: mageLiftStatus,
			CertificationStatus: certificationStatus, Limits: capabilityLimits(id, provider, region, service, major, lifecycle, providerStatus),
			Source: CapabilitySource{URL: source, RetrievedAt: capabilityRetrievedAt(id, retrievedAt), ReferenceDate: capabilityReferenceDate(id)}, Reason: reason,
		})
	}
	records = append(records, CapabilityRecord{
		ID: "aws.fck-nat", Provider: "aws", Region: "*", Service: "fck-nat", Role: "network",
		ServiceMajor: "arm64-instance", Lifecycle: "ga", AdobeStatus: AdobeCapabilityUnknown,
		ProviderStatus: CapabilitySupported, MageLiftStatus: CapabilityExperimental,
		CertificationStatus: CapabilityNotRun,
		Limits:              capabilityLimits("aws.fck-nat", "aws", "*", "fck-nat", "arm64-instance", "ga", CapabilitySupported),
		Source:              CapabilitySource{URL: fckNat, RetrievedAt: "2026-08-10"},
		Reason:              "The official deployment path documents the public ARM64 AMI owner/name contract and the stable-ENI Auto Scaling pattern; cost, egress, replacement, failure-domain, and live cleanup evidence remain profile-specific.",
	})
	// AWS compute and stateful/service boundaries.
	add("aws.ecs.fargate", "aws", "*", "ecs", "compute", "fargate", "ga", awsECS, AdobeCapabilityUnknown, CapabilitySupported, CapabilityCertified, CapabilityExperimental, "Runtime evidence exists for ECS Fargate, but the complete resilience, native-edge, and New Relic claim is still open.")
	add("aws.ecs.fargate-spot", "aws", "*", "ecs", "compute", "fargate-spot", "ga", awsECS, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "Capacity mode is provider-available; interruption semantics require a separate MageLift profile and evidence.")
	add("aws.ecs.ec2-asg", "aws", "*", "ecs", "compute", "ec2-asg", "ga", awsECS, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "EC2 Auto Scaling Group capacity requires host lifecycle and capacity-provider evidence.")
	add("aws.ecs.managed-instances", "aws", "*", "ecs", "compute", "managed-instances", "ga", awsECS, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "ECS Managed Instances are provider-available; MageLift task placement and teardown are not yet certified.")
	add("aws.eks.managed-control-plane", "aws", "*", "eks", "control-plane", "managed", "ga", awsEKS, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "EKS control-plane ownership is managed by AWS; the full node, workload, storage, edge, and recovery profile needs evidence.")
	add("aws.eks.managed-node-groups", "aws", "*", "eks", "compute", "managed-node-groups", "ga", awsEKSManaged, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "Managed node groups are supported by EKS; multi-AZ workload scheduling and disruption evidence remains separate.")
	add("aws.eks.auto-mode", "aws", "*", "eks", "compute", "auto-mode", "ga", awsEKS, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "EKS Auto Mode is a distinct managed compute boundary and cannot reuse managed-node-group evidence.")
	add("aws.eks.fargate", "aws", "*", "eks", "compute", "fargate", "ga", awsEKSFargate, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "EKS Fargate requires pod execution profiles and cannot host the persistent OpenSearch or RabbitMQ workloads in this composition.")
	add("aws.eks.self-managed", "aws", "*", "eks", "compute", "self-managed", "ga", awsEKSSelf, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "Self-managed nodes require pinned EKS-compatible AMIs, bootstrap, node IAM, access entries, and customer-owned patching and disruption evidence.")
	add("aws.rds.mysql", "aws", "*", "rds", "database", "mysql-8.4", "ga", awsRDS, AdobeCapabilitySupported, CapabilitySupported, CapabilityCompatible, CapabilityExperimental, "Provider and Adobe intersections are available; backup/restore and full HA evidence remain profile-specific.")
	add("aws.elasticache.valkey", "aws", "*", "elasticache", "cache", "valkey-8", "ga", awsCache, AdobeCapabilitySupported, CapabilitySupported, CapabilityCompatible, CapabilityExperimental, "Managed cache is available; cache-loss and recovery semantics are not a database backup claim.")
	add("aws.opensearch", "aws", "*", "opensearch", "search", "3", "ga", awsSearch, AdobeCapabilitySupported, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "Search is rebuildable and must be certified independently from the durable database.")
	add("aws.amazon-mq.rabbitmq", "aws", "*", "amazon-mq", "queue", "4.2", "ga", awsMQ, AdobeCapabilitySupported, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "Queue failover and message-loss policy require a named resilience profile.")
	add("aws.s3", "aws", "*", "s3", "media", "object", "ga", awsS3, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "The AWS S3 recovery translator now has normalized ownership cleanup and one bounded three-object media backup/isolated-restore/integrity cell with exact bucket deletion; versioning, retention variants, application-integrity, HA, and regional DR evidence remain open.")
	add("aws.secrets-manager", "aws", "*", "secrets-manager", "secrets", "current", "ga", awsSecrets, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "Secret references are modeled; rotation and revocation evidence remains open.")
	add("aws.cloudwatch", "aws", "*", "cloudwatch", "observability", "current", "ga", awsCW, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "CloudWatch native signal coverage must be proven per ECS/EKS profile.")
	add("aws.eks.cloudwatch-observability", "aws", "*", "eks", "observability", "cloudwatch-observability", "ga", awsEKSObs, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "The AWS-managed EKS observability add-on covers node-backed modes; Fargate uses a separate ADOT/log-router boundary and all signal delivery still requires live evidence.")
	add("aws.cloudfront", "aws", "*", "cloudfront", "edge", "current", "ga", awsCF, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "CloudFront is provider-available; origin health, purge, WAF, TLS, failover, and cleanup remain separate evidence.")
	add("aws.waf", "aws", "*", "waf", "edge-security", "current", "ga", awsWAF, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "WAF policy is provider-available but not a substitute for an edge routing proof.")

	// GCP GKE and managed services. 9.0 is GA; 9.1 is explicitly Preview.
	add("gcp.gke.autopilot", "gcp", "*", "gke", "compute", "autopilot", "ga", gcpGKE, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "Autopilot manages nodes and scaling; workload restrictions differ from Standard.")
	add("gcp.gke.standard", "gcp", "*", "gke", "compute", "standard", "ga", gcpGKE, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "Standard exposes node pools and node-level controls; it is a separate architecture boundary.")
	add("gcp.cloud-sql.mysql", "gcp", "*", "cloud-sql", "database", "mysql-8.4", "ga", gcpSQL, AdobeCapabilitySupported, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "Cloud SQL version and availability are selected by release and resilience profile.")
	add("gcp.pubsub", "gcp", "*", "pubsub", "queue", "snapshot-seek", "ga", gcpPubSub, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "Pub/Sub snapshot and seek provide an ownership-labeled same-region queue recovery path with a documented seven-day snapshot boundary; same-region isolated restore seeks a new owned subscription. Alternate-region and unverified CMEK paths remain unsupported by the current adapter.")
	add("gcp.memorystore.valkey-9.0", "gcp", "*", "memorystore", "cache", "valkey-9.0", "ga", gcpValkey, AdobeCapabilitySupported, CapabilitySupported, CapabilityCompatible, CapabilityExperimental, "Current Adobe Commerce 2.4.9 Valkey 9 requirement maps to the provider API value VALKEY_9_0.")
	add("gcp.memorystore.valkey-9.1", "gcp", "*", "memorystore", "cache", "valkey-9.1", "preview", gcpValkey, AdobeCapabilitySupported, CapabilityExperimental, CapabilityExperimental, CapabilityNotRun, "Provider documentation labels Valkey 9.1 Preview; it is selectable as an experimental profile, not the default 2.4.9 GA target.")
	add("gcp.cloud-storage", "gcp", "*", "cloud-storage", "media", "object", "ga", gcpStorage, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "The GCP Cloud Storage recovery translator now has normalized ownership cleanup and one bounded three-object media backup/isolated-restore/integrity cell with exact bucket deletion; retention variants, application-integrity, HA, and regional DR evidence remain open.")
	add("gcp.secret-manager", "gcp", "*", "secret-manager", "secrets", "current", "ga", gcpSecrets, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "Secret references must remain redacted and rotation/revocation must be tested.")
	add("gcp.cloud-observability", "gcp", "*", "cloud-observability", "observability", "current", "ga", gcpObs, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "GKE natively provides Cloud Logging and Cloud Monitoring, including audit logs; the current MageLift graph provisions logs and metrics, while trace instrumentation and live signal delivery remain separate evidence.")
	add("gcp.load-balancing-cdn-armor", "gcp", "*", "load-balancing", "edge", "current", "ga", gcpEdge, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "Google edge is composed from load balancing, CDN, and Cloud Armor; no single product row proves the composition.")

	// Scaleway official product boundaries. There is no native OpenSearch or
	// RabbitMQ managed product in this catalog, so those rows remain explicit.
	add("scaleway.kapsule", "scaleway", "*", "kapsule", "compute", "managed-kubernetes", "ga", scwK8s, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "Kapsule manages the control plane and node pools; workload state and backup ownership remain customer responsibilities.")
	add("scaleway.managed-mysql", "scaleway", "*", "managed-database", "database", "mysql", "ga", scwDB, AdobeCapabilitySupported, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "Managed MySQL is provider-available; restore and cross-region evidence remain open.")
	add("scaleway.managed-redis", "scaleway", "*", "managed-database", "cache", "redis", "ga", scwRedis, AdobeCapabilityUnsupported, CapabilitySupported, CapabilityCompatible, CapabilityNotRun, "Scaleway documents Managed Databases for Redis, while the current Adobe latest-patch rows require Valkey for the supported release lines.")
	add("scaleway.object-storage", "scaleway", "*", "object-storage", "media", "s3-compatible", "ga", scwStorage, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "The Scaleway S3-compatible recovery translator is implemented with pagination, sealed manifests, encryption, Object Lock, restore, integrity, and direct inventory; one bounded three-object media restore cell passed live, while retention variants and provider-wide recovery evidence remain open.")
	add("scaleway.secret-manager", "scaleway", "*", "secret-manager", "secrets", "current", "ga", scwSecrets, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityExperimental, "One bounded live synthetic-secret backup, Object Lock archive, isolated restore, integrity, exact version purge, and scheduled-free source cleanup cell passed; CMEK, rotation/revocation, regional recovery, HA, and provider-wide evidence remain open.")
	add("scaleway.cockpit", "scaleway", "*", "cockpit", "observability", "current", "ga", scwObs, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "Current Cockpit documentation covers metrics, logs, traces, regional data sources, and alert manager; product mapping and live delivery still need evidence.")
	add("scaleway.load-balancer", "scaleway", "*", "load-balancer", "ingress", "current", "ga", scwLB, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "Kapsule LoadBalancer Services are reconciled by the cluster controller.")
	add("scaleway.edge-services", "scaleway", "*", "edge-services", "edge", "current", "ga", scwEdge, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "Provider Edge Services is available; the MageLift lifecycle adapter exists offline, while production routing, TLS/WAF/cache/failover, and cleanup proof remain live evidence gates.")
	add("scaleway.search", "scaleway", "*", "search", "search", "none", "unavailable", scwK8s, AdobeCapabilityUnknown, CapabilityUnavailable, CapabilityExperimental, CapabilityNotRun, "Scaleway has no managed search product in this catalog; the MageLift operation translator requires an injected Kapsule/workload projection adapter for self-hosted search rebuilds, and live certification remains open.")
	add("scaleway.queue", "scaleway", "*", "queue", "queue", "none", "unavailable", scwK8s, AdobeCapabilityUnknown, CapabilityUnavailable, CapabilityUnavailable, CapabilityUnavailable, "No source-dated native queue or MageLift queue adapter is declared for this profile.")

	// OVHcloud MKS and Public Cloud boundaries. The MKS Load Balancer changed
	// from the historical IOLB path to Octavia by Kubernetes version, so the
	// versioned edge identity is kept separate.
	add("ovh.mks", "ovh", "*", "mks", "compute", "managed-kubernetes", "ga", ovhK8sNodes, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "MKS manages the control plane while customers own worker and workload configuration; Standard multi-zone node pools require one documented availability zone per pool.")
	add("ovh.managed-database", "ovh", "*", "managed-database", "database", "mysql", "ga", ovhMySQL, AdobeCapabilitySupported, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "OVH documents MySQL 8.0/8.4, plan-controlled node counts and backup retention, and same-region nodes; 1-AZ versus 3-AZ service durability and MageLift restore/DR evidence remain profile-specific.")
	add("ovh.managed-valkey", "ovh", "*", "managed-database", "cache", "valkey", "ga", ovhValkey, AdobeCapabilitySupported, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "OVH's current capability page documents Valkey 7.2/8.0/8.1; the authenticated EU-WEST-PAR availability catalog returned 8.0/8.1/9.0/9.1 on 2026-08-11. The adapter admits only the exact private-network, region, plan, flavor, and node-count combination returned by that catalog; 8.1 remains the MageLift default while cache reconstruction and recovery evidence remain independent from database backup proof.")
	add("ovh.object-storage", "ovh", "*", "object-storage", "media", "s3-compatible", "ga", ovhStorage, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "The OVHcloud S3-compatible recovery translator is implemented with pagination, sealed manifests, encryption, Object Lock, restore, integrity, and direct inventory; one bounded unversioned three-object same-region media cell passed with disposable-user revocation and exact bucket deletion, while retention variants, HA, and regional DR evidence remain open.")
	add("ovh.secret-manager", "ovh", "*", "secret-manager", "secrets", "current", "ga", ovhSecretMgr, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "OVHcloud Secret Manager stores versioned key/value secrets through the regional OKMS API; the provider adapter archives only ownership-bound active versions, cleans only marker-owned restore outputs, and keeps values out of lifecycle evidence. The authenticated certification account had no OKMS service domain for a live cell; live credentials, CMEK/retention variants, and regional-DR proof remain open.")
	add("ovh.kms", "ovh", "*", "kms", "secrets", "current", "ga", ovhKMS, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "OKMS authentication and key ownership remain distinct from the Secret Manager value and version boundary.")
	add("ovh.logs-data-platform", "ovh", "*", "logs-data-platform", "observability", "current", "ga", ovhObs, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "OVH documents MKS audit-log forwarding to an existing Logs Data Platform stream; application pod logs, metrics, traces, recovery, and retention coverage remain separate adapter boundaries.")
	add("ovh.public-cloud-load-balancer", "ovh", "*", "load-balancer", "ingress", "octavia", "ga", ovhLB, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "Octavia is the current MKS path for supported Kubernetes versions; historical IOLB is a separate migration boundary.")
	add("ovh.cdn", "ovh", "*", "cdn", "edge", "current", "ga", ovhEdge, AdobeCapabilityUnknown, CapabilitySupported, CapabilityUnavailable, CapabilityUnavailable, "OVH Web Hosting CDN is not the MKS Public Cloud edge path; no native OVH MKS CDN adapter is claimed. Use the OVH MKS Octavia Load Balancer adapter for L4 ingress or Fastly/external edge for CDN, TLS, WAF, and cache.")
	add("ovh.search", "ovh", "*", "search", "search", "none", "unavailable", ovhK8s, AdobeCapabilityUnknown, CapabilityUnavailable, CapabilityExperimental, CapabilityNotRun, "OVHcloud has no managed search product in this catalog; the MageLift operation translator requires an injected MKS/workload projection adapter for self-hosted search rebuilds, and live certification remains open.")
	add("ovh.queue", "ovh", "*", "queue", "queue", "none", "unavailable", ovhK8s, AdobeCapabilityUnknown, CapabilityUnavailable, CapabilityUnavailable, CapabilityUnavailable, "No source-dated native queue or MageLift queue adapter is declared for this profile.")
	for i := range records {
		switch records[i].ID {
		case "ovh.mks", "ovh.managed-database", "ovh.managed-valkey":
			records[i].Source.RetrievedAt = "2026-08-11"
		}
	}

	// Independent edge and observability implementations. Their records are
	// intentionally separate from each origin provider; a profile may compose
	// one of these with AWS, GCP, Scaleway, or OVHcloud.
	add("fastly.edge", "fastly", "*", "fastly", "edge", "current", "ga", fastlyDocs, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "Fastly service/domain/purge lifecycle is adapter-backed; TLS, origin health, routing, rollback, and complete cleanup remain independent gates.")
	add("newrelic.opentelemetry", "newrelic", "*", "newrelic", "observability", "otlp", "ga", newRelicOTLP, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "New Relic accepts portable OTLP signals; the provider-owned adapter verifies exact Log, Metric, or Span ownership markers through NerdGraph/NRQL, and the EU live cell passed logs, metrics, and traces with separate ingest and query credentials. Collector, native-provider composition, redaction, retention, alert, replacement-credential, and post-cleanup evidence remain profile-specific.")
	add("newrelic.ecs", "newrelic", "*", "newrelic", "observability", "ecs", "ga", newRelicECS, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "New Relic documents ECS monitoring for both EC2 and Fargate through OpenTelemetry Collector paths.")
	add("newrelic.kubernetes", "newrelic", "*", "newrelic", "observability", "kubernetes", "ga", newRelicK8s, AdobeCapabilityUnknown, CapabilitySupported, CapabilityExperimental, CapabilityNotRun, "New Relic documents the provider-agnostic NRDOT Kubernetes Helm chart and names managed-cloud examples including EKS and GKE; MageLift maps the same provider-neutral Helm lifecycle to Kapsule and MKS, while cluster compatibility, logs, traces, alerting, and cleanup remain separate evidence.")

	sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })
	return records
}

func capabilityLimits(id, provider, region, service, major, lifecycle string, providerStatus CapabilityStatus) string {
	switch id {
	case "gcp.memorystore.valkey-9.0":
		return "GA and the documented default; supported version and feature availability must still be checked in the selected region and project."
	case "gcp.memorystore.valkey-9.1":
		return "Preview under Pre-GA terms; not a default or production certification target; verify project and region eligibility before mutation."
	case "gcp.pubsub":
		return "Snapshot retention is bounded by the documented seven-day maximum; snapshot/seek restore targets the existing subscription and requires a same-region application verifier."
	case "aws.ecs.fargate-spot":
		return "Spare capacity can be unavailable; interrupted tasks receive a two-minute warning and are not automatically converted to Fargate On-Demand."
	case "aws.ecs.managed-instances":
		return "Capacity-provider strategy cannot mix Managed Instances with Auto Scaling group or Fargate provider types; IAM roles and regional capacity are required."
	case "aws.eks.auto-mode":
		return "AWS-managed nodes have a maximum 21-day lifetime and do not allow SSH or SSM access; regional service limits and workload policy eligibility still apply."
	case "aws.eks.managed-node-groups":
		return "Managed node groups span the selected subnets/AZs, are unavailable on Outposts and Wavelength, and require ECR/private-network reachability in private subnets."
	case "ovh.managed-database":
		return "MySQL and PostgreSQL use continuous PITR with a documented few-minute RPO; backup locations accept one or two regions with one matching the service region."
	case "ovh.managed-valkey":
		return "The current Valkey capability page documents 7.2/8.0/8.1, while the authenticated EU-WEST-PAR availability catalog returned 8.0/8.1/9.0/9.1 on 2026-08-11. Low-cost Discovery/Essential one-node and Business/Production two-node shapes require private-network, region, flavor, and node-count admission; backup frequency, retention, and the 12-hour RPO boundary remain plan/provider controlled and require live evidence."
	case "scaleway.edge-services":
		return "Edge Services provides caching and WAF; the WAF was documented as Public Beta, and product/region availability must be checked before mutation."
	case "scaleway.cockpit":
		return "Cockpit data-source types and retention are product/region controlled; signal delivery and retention must be verified for the selected data source."
	case "ovh.logs-data-platform":
		return "MKS forwarding currently supports audit subscriptions to an existing Logs Data Platform stream; application pod logs require a separate Fluent Bit path."
	case "scaleway.search", "ovh.search":
		return "No managed search product is declared; self-hosted Kapsule/MKS search recovery requires an injected workload projection adapter, and live certification remains open."
	case "scaleway.queue", "ovh.queue":
		return "No current provider-managed or MageLift adapter path is declared; this row is explicitly unavailable for the current catalog."
	}
	if lifecycle == "preview" {
		return "Preview lifecycle; support, regions, quotas, and production eligibility must be verified in the target account before mutation."
	}
	if providerStatus == CapabilityUnavailable || providerStatus == CapabilityUnsupported {
		return "Unavailable or unsupported in the current MageLift catalog; no provider mutation is permitted."
	}
	return fmt.Sprintf("Regional availability, account/project quotas, service limits, and feature eligibility for %s %s must be checked during admission before mutation.", provider, service)
}

func capabilityReferenceDate(id string) string {
	switch id {
	case "gcp.memorystore.valkey-9.0":
		return "2026-07-27"
	case "gcp.memorystore.valkey-9.1":
		return "2026-07-27"
	case "gcp.pubsub":
		return "2026-07-17"
	case "ovh.secret-manager":
		return "2026-07-16"
	default:
		return ""
	}
}

func capabilityRetrievedAt(id, defaultDate string) string {
	switch id {
	case "gcp.memorystore.valkey-9.0", "gcp.memorystore.valkey-9.1", "scaleway.secret-manager", "ovh.secret-manager", "newrelic.kubernetes":
		return "2026-08-12"
	default:
		return defaultDate
	}
}

func parseCatalogDate(value string) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.UTC(), nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, fmt.Errorf("want RFC3339 or YYYY-MM-DD: %w", err)
	}
	return parsed.UTC(), nil
}

func validAdobeStatus(status AdobeCapabilityStatus) bool {
	switch status {
	case AdobeCapabilitySupported, AdobeCapabilityMageLift, AdobeCapabilityUnsupported, AdobeCapabilityUnavailable, AdobeCapabilityUnknown:
		return true
	default:
		return false
	}
}

func validCapabilityStatus(status CapabilityStatus) bool {
	switch status {
	case CapabilitySupported, CapabilityCertified, CapabilityCompatible, CapabilityExperimental, CapabilityUnsupported, CapabilityUnavailable, CapabilityBlocked, CapabilityNotRun:
		return true
	default:
		return false
	}
}
