package cli

import (
	"strings"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/edge/waf"
	"github.com/spf13/cobra"
)

const auditReportScope = "This report covers project settings. Organizational compliance requires its own assessment."

type auditControl struct {
	ID       string   `json:"id" yaml:"id"`
	Status   string   `json:"status" yaml:"status"`
	Pointers []string `json:"pointers" yaml:"pointers"`
}

type auditReport struct {
	Environment string         `json:"environment" yaml:"environment"`
	Disclaimer  string         `json:"disclaimer" yaml:"disclaimer"`
	Controls    []auditControl `json:"controls" yaml:"controls"`
}

func auditCommand(o *options) *cobra.Command {
	return &cobra.Command{
		Use:   "audit",
		Short: "Show where security, backups, and monitoring are configured",
		Long:  "Shows the project settings for encryption, access, logging, backups, web application firewall, and data residency. Secret values are omitted.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := o.runAudit()
			if err != nil {
				return err
			}
			return o.write(report)
		},
	}
}

func (o *options) runAudit() (auditReport, error) {
	effective, environment, err := o.resolveWithEnvironment()
	if err != nil {
		return auditReport{}, invalid(err)
	}
	cfg := effective.Config
	return auditReport{
		Environment: environment,
		Disclaimer:  auditReportScope,
		Controls: []auditControl{
			{ID: "encryption", Status: auditEncryptionStatus(cfg), Pointers: auditEncryptionPointers(cfg)},
			{ID: "iam", Status: auditIAMStatus(cfg), Pointers: auditIAMPointers(cfg, environment)},
			{ID: "logging", Status: auditLoggingStatus(cfg), Pointers: auditLoggingPointers(cfg)},
			{ID: "backup", Status: auditBackupStatus(cfg), Pointers: auditBackupPointers(cfg)},
			{ID: "waf", Status: auditWAFStatus(cfg), Pointers: auditWAFPointers(cfg)},
			{ID: "residency", Status: auditResidencyStatus(cfg), Pointers: auditResidencyPointers(cfg)},
		},
	}, nil
}

func auditPointerStatus(pointers []string) string {
	if len(pointers) == 0 {
		return "undeclared"
	}
	return "declared"
}

func auditEncryptionStatus(cfg config.Config) string {
	return auditPointerStatus(auditEncryptionPointers(cfg))
}

func auditEncryptionPointers(cfg config.Config) []string {
	var pointers []string
	if cfg.Target.AWS != nil {
		if arn := strings.TrimSpace(cfg.Target.AWS.KMSKeyARN); arn != "" {
			pointers = append(pointers, "target.aws.kmsKeyArn")
		}
		if arn := strings.TrimSpace(cfg.Target.AWS.EncryptionKeySecretARN); arn != "" {
			pointers = append(pointers, "target.aws.encryptionKeySecretArn")
		}
	}
	if cfg.Target.GCP != nil && strings.TrimSpace(cfg.Target.GCP.EncryptionKeySecret) != "" {
		pointers = append(pointers, "target.gcp.encryptionKeySecret")
	}
	return pointers
}

func auditIAMStatus(cfg config.Config) string {
	return auditPointerStatus(auditIAMPointers(cfg, ""))
}

func auditIAMPointers(cfg config.Config, environment string) []string {
	pointers := []string{"project.name", "target.provider", "target.runtime"}
	if environment != "" {
		pointers = append(pointers, "environments."+environment+".account")
	}
	if cfg.Account != "" {
		pointers = append(pointers, "account")
	}
	return pointers
}

func auditLoggingStatus(cfg config.Config) string {
	return auditPointerStatus(auditLoggingPointers(cfg))
}

func auditLoggingPointers(cfg config.Config) []string {
	var pointers []string
	if cfg.Target.AWS != nil && cfg.Target.AWS.Catalog.Retention.LogDays > 0 {
		pointers = append(pointers, "target.aws.catalog.retention.logDays")
	}
	if strings.TrimSpace(cfg.Observability.NativeProvider) != "" {
		pointers = append(pointers, "observability.nativeProvider")
	}
	if cfg.Observability.RetentionDays > 0 {
		pointers = append(pointers, "observability.retentionDays")
	}
	return pointers
}

func auditBackupStatus(cfg config.Config) string {
	return auditPointerStatus(auditBackupPointers(cfg))
}

func auditBackupPointers(cfg config.Config) []string {
	var pointers []string
	if cfg.Resilience.RetentionDays > 0 {
		pointers = append(pointers, "resilience.retentionDays")
	}
	if cfg.Target.AWS != nil && cfg.Target.AWS.Catalog.Retention.BackupDays > 0 {
		pointers = append(pointers, "target.aws.catalog.retention.backupDays")
	}
	if cfg.Target.GCP != nil && cfg.Target.GCP.CloudSQLBackupEnabled != nil {
		pointers = append(pointers, "target.gcp.cloudSqlBackupEnabled")
	}
	return pointers
}

func auditWAFStatus(cfg config.Config) string {
	return auditPointerStatus(auditWAFPointers(cfg))
}

func auditWAFPointers(cfg config.Config) []string {
	pointers := []string{"edge.wafPolicyRef=" + waf.PolicyRef}
	if ref := strings.TrimSpace(cfg.Edge.WAFPolicyRef); ref != "" {
		pointers = append(pointers, "edge.wafPolicyRef")
	}
	if frontName := strings.TrimSpace(cfg.Application.Magento.FrontName); frontName != "" {
		pointers = append(pointers, "application.magento.frontName")
	}
	return pointers
}

func auditResidencyStatus(cfg config.Config) string {
	if strings.TrimSpace(cfg.Resilience.DataRegion) == "" {
		return "undeclared"
	}
	return "declared"
}

func auditResidencyPointers(cfg config.Config) []string {
	var pointers []string
	if strings.TrimSpace(cfg.Resilience.DataRegion) != "" {
		pointers = append(pointers, "resilience.dataRegion")
	}
	if strings.TrimSpace(cfg.Defaults.Region) != "" {
		pointers = append(pointers, "defaults.region")
	}
	if cfg.Target.GCP != nil && strings.TrimSpace(cfg.Target.GCP.CloudSQLBackupLocation) != "" {
		pointers = append(pointers, "target.gcp.cloudSqlBackupLocation")
	}
	return pointers
}
