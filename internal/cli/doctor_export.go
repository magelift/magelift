package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/magelift/magelift/internal/config"
)

const doctorExportSchemaVersion = 1

// doctorExportDocument is the only JSON shape written by --export. It is not
// doctorReport and must never carry raw diagnostics.
type doctorExportDocument struct {
	SchemaVersion int                          `json:"schemaVersion"`
	CLIVersion    string                       `json:"cliVersion,omitempty"`
	Runtime       doctorExportRuntime          `json:"runtime"`
	Status        string                       `json:"status"`
	Target        *doctorExportTarget          `json:"target,omitempty"`
	Checks        doctorExportChecks           `json:"checks"`
	Omitted       doctorExportOmissionContract `json:"omitted"`
}

type doctorExportRuntime struct {
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
}

type doctorExportTarget struct {
	Provider string `json:"provider,omitempty"`
	Runtime  string `json:"runtime,omitempty"`
}

type doctorExportChecks struct {
	Summary doctorExportCheckSummary `json:"summary"`
	Items   []doctorExportCheckItem  `json:"items"`
}

type doctorExportCheckSummary struct {
	Total       int `json:"total"`
	OK          int `json:"ok"`
	Failed      int `json:"failed"`
	Skipped     int `json:"skipped"`
	Missing     int `json:"missing"`
	Unreachable int `json:"unreachable"`
}

type doctorExportCheckItem struct {
	Category    string `json:"category"`
	ID          string `json:"id"`
	Status      string `json:"status"`
	Requirement string `json:"requirement,omitempty"`
}

type doctorExportOmissionContract struct {
	Contract string   `json:"contract"`
	Fields   []string `json:"fields"`
}

var doctorExportOmittedFieldList = []string{
	"configPath",
	"projectName",
	"accountIds",
	"environmentNames",
	"hostnames",
	"next",
	"checkMessages",
	"checkPaths",
	"checkVersions",
	"installHints",
	"capabilities",
	"toolOutput",
	"rawErrors",
	"secrets",
	"tokens",
	"urls",
	"logs",
	"dumps",
}

func buildDoctorExport(report doctorReport, file *config.File) (doctorExportDocument, error) {
	doc := doctorExportDocument{
		SchemaVersion: doctorExportSchemaVersion,
		Runtime: doctorExportRuntime{
			GOOS:   runtime.GOOS,
			GOARCH: runtime.GOARCH,
		},
		Status: normalizeDoctorExportStatus(report.Status),
		Checks: summarizeDoctorExportChecks(report.Checks),
		Omitted: doctorExportOmissionContract{
			Contract: "Voluntary local metadata export uses this fixed allowlist only. Fields not listed in the export schema are omitted by construction, not redacted from a fuller report. Do not treat omitted data as safe to paste elsewhere.",
			Fields:   doctorExportOmittedFieldList,
		},
	}
	doc.CLIVersion = doctorExportVersion(Version)
	if file != nil {
		provider, runtimeID := dependencyTarget(file)
		doc.Target = exportableDoctorTarget(provider, runtimeID)
	}
	return doc, nil
}

func normalizeDoctorExportStatus(status string) string {
	switch status {
	case "ok", "failed":
		return status
	default:
		return "failed"
	}
}

// Only known built-in targets are exportable. Syntax validation alone would
// still allow private names or credential-like values from configuration.
func exportableDoctorTarget(provider, runtimeID string) *doctorExportTarget {
	switch provider + ":" + runtimeID {
	case "gcp:gke-autopilot", "gcp:gke-standard", "aws:ecs-fargate", "aws:eks", "ovh:mks", "scaleway:kapsule":
		return &doctorExportTarget{Provider: provider, Runtime: runtimeID}
	default:
		return nil
	}
}

var doctorExportVersionPattern = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(?:-(?:(?:alpha|beta|rc)(?:\.[0-9]+)+|canary\.[a-f0-9]{40}))?$`)

func doctorExportVersion(version string) string {
	if version == "dev" || doctorExportVersionPattern.MatchString(version) {
		return version
	}
	return ""
}

func doctorExportCheckID(id string) (category, safeID string) {
	switch id {
	case "config", "build", "environments":
		return "config", id
	case "credentials.gcp":
		return "credentials", id
	case "local.edge":
		return "local", id
	case "dependency.docker", "dependency.docker-compose", "dependency.docker-buildx", "dependency.kubectl", "dependency.cloud-sql-proxy", "dependency.gcloud", "dependency.aws", "dependency.session-manager-plugin", "dependency.dependency-runner":
		return "dependency", id
	default:
		if strings.HasPrefix(id, "environment.") {
			return "environment", "environment"
		}
		return "other", "other"
	}
}

func summarizeDoctorExportChecks(checks []doctorCheck) doctorExportChecks {
	summary := doctorExportCheckSummary{}
	items := make([]doctorExportCheckItem, 0, len(checks))
	for _, check := range checks {
		status := check.Status
		if status != "ok" && status != "skipped" && status != "missing" && status != "unreachable" {
			status = "failed"
		}
		summary.Total++
		switch status {
		case "ok":
			summary.OK++
		case "skipped":
			summary.Skipped++
		case "missing":
			summary.Missing++
		case "unreachable":
			summary.Unreachable++
		default:
			summary.Failed++
		}
		category, id := doctorExportCheckID(check.ID)
		requirement := ""
		if category == "dependency" && (check.Requirement == "required" || check.Requirement == "optional") {
			requirement = check.Requirement
		}
		items = append(items, doctorExportCheckItem{
			Category:    category,
			ID:          id,
			Status:      status,
			Requirement: requirement,
		})
	}
	return doctorExportChecks{Summary: summary, Items: items}
}

func writeDoctorExport(path string, report doctorReport, file *config.File) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	doc, err := buildDoctorExport(report, file)
	if err != nil {
		return err
	}
	payload, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	return writeDoctorExportFile(path, payload)
}

func writeDoctorExportFile(path string, payload []byte) error {
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
		return err
	}
	existing, err := os.Lstat(absolute)
	if err == nil {
		if existing.Mode()&os.ModeSymlink != 0 || !existing.Mode().IsRegular() {
			return invalid(errors.New("refusing to write export to a non-regular file"))
		}
		return invalid(errors.New("refusing to overwrite an existing export file"))
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := os.OpenFile(absolute, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(payload); err != nil {
		file.Close()
		os.Remove(absolute)
		return err
	}
	if err := file.Close(); err != nil {
		os.Remove(absolute)
		return err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return err
	}
	if info.Mode().Perm() != 0o600 {
		_ = os.Remove(absolute)
		return fmt.Errorf("export file mode is %#o, want 0600", info.Mode().Perm())
	}
	return nil
}

func buildDoctorReport(configPath string, file *config.File, loadErr error) doctorReport {
	if loadErr != nil {
		return doctorReport{
			Config:       configPath,
			Status:       "failed",
			Environments: []string{},
			Checks: []doctorCheck{{
				ID:      "config",
				Status:  "failed",
				Message: "configuration could not be read or validated; run magelift config validate",
			}},
		}
	}
	return inspectProject(configPath, file)
}

func localModeCredentialSkips(file *config.File) []doctorCheck {
	if file == nil {
		return nil
	}
	provider, _ := dependencyTarget(file)
	if provider != "gcp" {
		return nil
	}
	return []doctorCheck{{
		ID:      "credentials.gcp",
		Status:  "skipped",
		Message: "--local: cloud credential checks were not run",
	}}
}
