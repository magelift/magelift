package certification

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

// DocumentationBundle is the deterministic release-document output produced
// from the source-dated capability catalog and sealed evidence. The generator
// deliberately returns values instead of writing files so CI, the CLI, and a
// community release tool can choose their own storage backend.
type DocumentationBundle struct {
	GeneratedAt string            `json:"generatedAt" yaml:"generatedAt"`
	Files       map[string]string `json:"files" yaml:"files"`
}

// GenerateDocumentation renders the release-facing capability, architecture,
// resilience, observability, edge, certification, and recovery documents.
// Catalog facts are always rendered; evidence-dependent claims are rendered
// only from records that have passed the sealed-record validator.
func GenerateDocumentation(catalog CapabilityCatalog, records []Record, now time.Time) (DocumentationBundle, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if err := catalog.Validate(now); err != nil {
		return DocumentationBundle{}, fmt.Errorf("validate documentation catalog: %w", err)
	}
	for index, record := range records {
		if err := record.Validate(); err != nil {
			return DocumentationBundle{}, fmt.Errorf("validate documentation evidence %d: %w", index, err)
		}
	}
	coverage, err := ArchitectureCoverage(catalog, now)
	if err != nil {
		return DocumentationBundle{}, fmt.Errorf("build documentation architecture coverage: %w", err)
	}
	generatedAt := now.UTC().Format(time.RFC3339)
	return DocumentationBundle{
		GeneratedAt: generatedAt,
		Files: map[string]string{
			"capability-matrix.md":   renderCapabilityMatrix(catalog, now),
			"architecture-matrix.md": renderArchitectureMatrix(coverage),
			"resilience.md":          renderResilience(records, generatedAt),
			"observability.md":       renderObservability(records, generatedAt),
			"edge.md":                renderEdge(records, generatedAt),
			"certification.md":       renderCertification(records, generatedAt),
			"recovery-runbook.md":    renderRecoveryRunbook(catalog, generatedAt),
		},
	}, nil
}

func renderCapabilityMatrix(catalog CapabilityCatalog, now time.Time) string {
	report, _ := catalog.Coverage(now)
	var builder strings.Builder
	fmt.Fprintf(&builder, "# Capability matrix\n\nCatalog `%s`, generated `%s`. Every row is source-dated; a status is not a certification claim.\n\n", catalog.Version, report.GeneratedAt)
	builder.WriteString("| ID | Provider | Service | Role | Status | Limits | Source |\n|---|---|---|---|---|---|---|\n")
	for _, row := range report.Rows {
		fmt.Fprintf(&builder, "| `%s` | `%s` | `%s` | `%s` | `%s` | %s | %s |\n", row.ID, row.Provider, row.Service, row.Role, row.Status, markdown(row.Limits), sourceMarkdown(row.SourceURL, row.RetrievedAt))
	}
	return builder.String()
}

func renderArchitectureMatrix(report ArchitectureCoverageReport) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "# Architecture matrix\n\nCatalog `%s`, generated `%s`. Non-certified rows retain their provider reason and source links.\n\n", report.CatalogVersion, report.GeneratedAt)
	builder.WriteString("| Family | Provider | Runtime | Status | Compute modes | Sources | Reason |\n|---|---|---|---|---|---|---|\n")
	for _, row := range report.Rows {
		sources := make([]string, 0, len(row.Sources))
		for _, source := range row.Sources {
			sources = append(sources, sourceMarkdown(source.URL, source.RetrievedAt))
		}
		fmt.Fprintf(&builder, "| `%s` | `%s` | `%s` | `%s` | %s | %s | %s |\n", row.FamilyID, row.Provider, row.Runtime, row.Status, markdown(strings.Join(row.ComputeModes, ", ")), strings.Join(sources, "<br>"), markdown(strings.Join(row.Reasons, " ")))
	}
	return builder.String()
}

func renderResilience(records []Record, generatedAt string) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "# Resilience evidence\n\nGenerated `%s` from sealed evidence only.\n\n", generatedAt)
	builder.WriteString("| Run | Cell | Data class | Action | Status | Operation/backup/restore | RPO/RTO seconds | Reason |\n|---|---|---|---|---|---|---|---|\n")
	count := 0
	for _, record := range sortedRecords(records) {
		for _, proof := range record.Resilience {
			identity := firstNonEmpty(proof.OperationID, proof.BackupID, proof.RestoreID, proof.IntegrityDigest)
			fmt.Fprintf(&builder, "| `%s` | `%s` | `%s` | `%s` | `%s` | `%s` | %d/%d | %s |\n", record.RunID, record.CellID, proof.DataClass, proof.Action, proof.Status, markdown(identity), proof.MeasuredRPOSeconds, proof.MeasuredRTOSeconds, markdown(proof.Reason))
			count++
		}
	}
	if count == 0 {
		builder.WriteString("No sealed resilience proof was supplied; no backup, restore, HA, DR, fencing, or failback claim is published.\n")
	}
	return builder.String()
}

func renderObservability(records []Record, generatedAt string) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "# Observability evidence\n\nGenerated `%s` from sealed evidence only. Native provider and New Relic rows remain independent.\n\n", generatedAt)
	builder.WriteString("| Run | Cell | Provider | Signal | Status | Identity | Retention | Labels | Redaction | Alert | Reason |\n|---|---|---|---|---|---|---:|---|---|---|---|\n")
	count := 0
	for _, record := range sortedRecords(records) {
		for _, proof := range record.Observability {
			fmt.Fprintf(&builder, "| `%s` | `%s` | `%s` | `%s` | `%s` | `%s` | %d | %t | %t | %t | %s |\n", record.RunID, record.CellID, proof.Provider, proof.Signal, proof.Status, markdown(proof.Identity), proof.RetentionDays, proof.LabelsVerified, proof.RedactionVerified, proof.AlertVerified, markdown(proof.Reason))
			count++
		}
	}
	if count == 0 {
		builder.WriteString("No sealed observability proof was supplied; no native or New Relic delivery claim is published.\n")
	}
	return builder.String()
}

func renderEdge(records []Record, generatedAt string) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "# Edge evidence\n\nGenerated `%s` from sealed evidence only. Native edge and Fastly rows remain independent.\n\n", generatedAt)
	builder.WriteString("| Run | Cell | Provider | Feature | Status | Identity | Origin | TLS | Purge | Route | Reason |\n|---|---|---|---|---|---|---|---|---|---|---|\n")
	count := 0
	for _, record := range sortedRecords(records) {
		for _, proof := range record.Edge {
			fmt.Fprintf(&builder, "| `%s` | `%s` | `%s` | `%s` | `%s` | `%s` | %t | %t | %t | %t | %s |\n", record.RunID, record.CellID, proof.Provider, proof.Feature, proof.Status, markdown(proof.Identity), proof.OriginHealthVerified, proof.TLSVerified, proof.PurgeVerified, proof.RouteVerified, markdown(proof.Reason))
			count++
		}
	}
	if count == 0 {
		builder.WriteString("No sealed edge proof was supplied; no CloudFront, Google Cloud, Scaleway, OVHcloud, or Fastly edge claim is published.\n")
	}
	return builder.String()
}

func renderCertification(records []Record, generatedAt string) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "# Certification report\n\nGenerated `%s` from sealed evidence only.\n\n", generatedAt)
	counts := make(map[Status]int)
	for _, record := range records {
		if record.Type == RecordCell {
			counts[record.Status]++
		}
	}
	statuses := make([]string, 0, len(counts))
	for status := range counts {
		statuses = append(statuses, string(status))
	}
	sort.Strings(statuses)
	if len(statuses) == 0 {
		builder.WriteString("No sealed cell evidence was supplied; the release has no certified cells.\n")
		return builder.String()
	}
	builder.WriteString("| Status | Count |\n|---|---:|\n")
	for _, status := range statuses {
		fmt.Fprintf(&builder, "| `%s` | %d |\n", status, counts[Status(status)])
	}
	builder.WriteString("\n| Run | Cell | Status | Required | Record digest | Reason |\n|---|---|---|---|---|---|\n")
	for _, record := range sortedRecords(records) {
		if record.Type != RecordCell {
			continue
		}
		fmt.Fprintf(&builder, "| `%s` | `%s` | `%s` | %t | `%s` | %s |\n", record.RunID, record.CellID, record.Status, record.Required, record.RecordDigest, markdown(record.Reason))
	}
	return builder.String()
}

func renderRecoveryRunbook(catalog CapabilityCatalog, generatedAt string) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "# Recovery runbook\n\nGenerated `%s` from catalog `%s`. Provider adapters must supply the opaque operation and inventory implementations; the core never substitutes a neighboring provider.\n\n", generatedAt, catalog.Version)
	builder.WriteString("1. Freeze the active writer and obtain the declared failover approval.\n2. Verify the selected source, backup identity, retention, encryption, protection, and ownership markers.\n3. Execute the provider-specific restore or rebuild stages through the normalized SDK port and poll the owning service until terminal.\n4. Verify fixture manifest, checksums, counts, application reads, permissions, secret references, service health, measured RPO/RTO, and stale-origin fencing.\n5. Reopen traffic only after the alternate runtime is healthy and route convergence is measured.\n6. Fail back through the same approval and fencing boundary; clean up only resources with exact ownership evidence.\n\n")
	builder.WriteString("Provider-specific gaps and source links are maintained in the generated capability and architecture matrices. A row marked unavailable, unsupported, blocked, experimental, or not-run is not a recovery guarantee.\n")
	return builder.String()
}

func sortedRecords(records []Record) []Record {
	result := append([]Record(nil), records...)
	sort.Slice(result, func(i, j int) bool {
		if result[i].RunID != result[j].RunID {
			return result[i].RunID < result[j].RunID
		}
		if result[i].CellID != result[j].CellID {
			return result[i].CellID < result[j].CellID
		}
		return result[i].Type < result[j].Type
	})
	return result
}

func sourceMarkdown(rawURL, retrievedAt string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return markdown(rawURL)
	}
	return fmt.Sprintf("[%s](%s) (%s)", markdown(parsed.Host), rawURL, markdown(retrievedAt))
}

func markdown(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.ReplaceAll(value, "|", "\\|")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
