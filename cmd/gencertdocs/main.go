package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/magelift/magelift/internal/certification"
	"github.com/magelift/magelift/sdk"
)

const outputPath = "docs/evidence/current-capability-coverage.md"
const defaultEvidenceDir = "docs/evidence/runs"

func main() {
	check := flag.Bool("check", false, "fail if the generated capability report is stale")
	evidenceDir := flag.String("evidence-dir", defaultEvidenceDir, "relative directory containing sealed JSONL certification evidence")
	flag.Parse()
	if flag.NArg() != 0 {
		fatalf("usage: gencertdocs [--check] [--evidence-dir relative-directory]")
	}

	catalog := certification.CurrentCapabilityCatalog()
	now := catalogTime(catalog)
	coverage, err := catalog.Coverage(now)
	if err != nil {
		fatalf("generate capability coverage: %v", err)
	}
	architecture, err := certification.ArchitectureCoverage(catalog, now)
	if err != nil {
		fatalf("generate architecture coverage: %v", err)
	}
	bundle, err := certification.LoadEvidenceBundle(os.DirFS("."), *evidenceDir)
	if err != nil {
		fatalf("load certification evidence: %v", err)
	}
	contents := render(catalog, coverage, architecture, certification.CurrentResilienceProfiles(), bundle)
	if *check {
		current, err := os.ReadFile(outputPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				fatalf("%s is missing; run make certification-docs", outputPath)
			}
			fatalf("read %s: %v", outputPath, err)
		}
		if !bytes.Equal(current, contents) {
			fatalf("%s is stale; run make certification-docs", outputPath)
		}
		return
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		fatalf("create report directory: %v", err)
	}
	if err := os.WriteFile(outputPath, contents, 0o644); err != nil {
		fatalf("write %s: %v", outputPath, err)
	}
}

func render(catalog certification.CapabilityCatalog, coverage certification.CapabilityCoverageReport, architecture certification.ArchitectureCoverageReport, resilience []certification.ResilienceCapabilityProfile, bundle certification.EvidenceBundle) []byte {
	var output strings.Builder
	output.WriteString("# Current capability and resilience coverage\n\n")
	output.WriteString("This page is generated from the source-dated MageLift capability catalog. Do not edit it by hand.\n\n")
	output.WriteString("- Catalog: `" + catalog.Version + "`\n")
	output.WriteString("- Generated at: `" + coverage.GeneratedAt + "`\n\n")

	output.WriteString("## Provider capability records\n\n")
	output.WriteString("| ID | Provider | Service | Role | Major | Effective status | Limits | Source | Reason |\n| --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, row := range coverage.Rows {
		record, _ := catalog.FindID(row.ID)
		output.WriteString("| `" + row.ID + "` | " + record.Provider + " | " + record.Service + " | " + record.Role + " | " + record.ServiceMajor + " | **" + string(row.Status) + "** | " + markdownCell(row.Limits) + " | " + markdownSource(record.Source) + " | " + markdownCell(row.Reason) + " |\n")
	}

	output.WriteString("\n## Architecture families\n\n")
	output.WriteString("| Family | Provider/runtime | Status | Compute modes | Capability sources | Reason |\n| --- | --- | --- | --- | --- | --- |\n")
	for _, row := range architecture.Rows {
		output.WriteString("| `" + row.FamilyID + "` | " + row.Provider + " / `" + row.Runtime + "` | **" + string(row.Status) + "** | " + strings.Join(row.ComputeModes, ", ") + " | " + markdownSources(row.Sources) + " | " + markdownCell(strings.Join(row.Reasons, " ")) + " |\n")
	}

	output.WriteString("\n## Resilience policy profiles\n\n")
	output.WriteString("These are pre-mutation policy ceilings, not live backup/restore or disaster-recovery certification.\n\n")
	output.WriteString("| Profile | Provider/runtime | Status | Maximum RPO | Maximum RTO | Destinations | Source capability IDs | Reason |\n| --- | --- | --- | ---: | ---: | --- | --- | --- |\n")
	sort.Slice(resilience, func(i, j int) bool { return resilience[i].ID < resilience[j].ID })
	for _, profile := range resilience {
		output.WriteString("| `" + profile.ID + "` | " + profile.Provider + " / `" + profile.Runtime + "` | **" + string(profile.Status) + "** | " + fmt.Sprint(profile.MaxRPOSeconds) + "s | " + fmt.Sprint(profile.MaxRTOSeconds) + "s | " + strings.Join(stringDestinations(profile.AllowedDestinations), ", ") + " | " + strings.Join(profile.SourceCapabilityIDs, ", ") + " | " + markdownCell(profile.Reason) + " |\n")
	}

	output.WriteString("\n## Resilience data-class coverage\n\n")
	output.WriteString("A data class marked unavailable or unsupported is rejected before provider mutation; cache reconstruction and search rebuild are not durable-backup claims.\n\n")
	output.WriteString("| Profile | Data class | Status | Durable | Backup | Restore | Strategy | Destinations | Reason |\n| --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, profile := range resilience {
		classes := append([]certification.ResilienceDataClassCapability(nil), profile.DataClasses...)
		sort.Slice(classes, func(i, j int) bool { return classes[i].Name < classes[j].Name })
		for _, dataClass := range classes {
			output.WriteString("| `" + profile.ID + "` | `" + dataClass.Name + "` | **" + string(dataClass.Status) + "** | " + fmt.Sprint(dataClass.Durable) + " | " + fmt.Sprint(dataClass.BackupSupported) + " | " + fmt.Sprint(dataClass.RestoreSupported) + " | `" + dataClass.Strategy + "` | " + strings.Join(dataClass.Destinations, ", ") + " | " + markdownCell(dataClass.Reason) + " |\n")
		}
	}

	output.WriteString("\n## Observability coverage\n\n")
	output.WriteString("Native and external observability are separate destinations. A provider capability row does not prove that a signal was delivered, retained, redacted, or cleaned up.\n\n")
	output.WriteString("| ID | Provider | Destination | Lifecycle | Status | Source | Reason |\n| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, row := range coverage.Rows {
		record, _ := catalog.FindID(row.ID)
		if record.Role != "observability" {
			continue
		}
		output.WriteString("| `" + row.ID + "` | " + record.Provider + " | " + record.Service + " | " + record.Lifecycle + " | **" + string(row.Status) + "** | " + markdownSource(record.Source) + " | " + markdownCell(row.Reason) + " |\n")
	}

	output.WriteString("\n## Edge coverage\n\n")
	output.WriteString("Edge claims require independent origin-health, TLS, DNS, routing, purge, security-policy, failover, rollback, and cleanup evidence.\n\n")
	output.WriteString("| ID | Provider | Product | Lifecycle | Status | Source | Reason |\n| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, row := range coverage.Rows {
		record, _ := catalog.FindID(row.ID)
		if record.Role != "edge" && record.Role != "edge-security" {
			continue
		}
		output.WriteString("| `" + row.ID + "` | " + record.Provider + " | " + record.Service + " | " + record.Lifecycle + " | **" + string(row.Status) + "** | " + markdownSource(record.Source) + " | " + markdownCell(row.Reason) + " |\n")
	}

	output.WriteString("\n## Recorded certification evidence\n\n")
	output.WriteString("This section is generated only from sealed JSONL records loaded from `" + markdownCell(bundle.Directory) + "`. A validated run is evidence input, not an automatic certification claim; the release gate still evaluates the exact profile requirements and live provider boundaries.\n\n")
	if len(bundle.Files) == 0 {
		output.WriteString("No sealed JSONL evidence bundle was supplied. Live certification remains evidence-gated.\n")
	} else {
		output.WriteString("| Run | State | Records | Cells | PASS cells | Cleanup | Architecture rows | Resilience proofs | Observability proofs | Edge proofs | Generated at | Sources | Reason |\n| --- | --- | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | --- | --- | --- |\n")
		for _, run := range evidenceRuns(bundle.Records) {
			report, verifyErr := certification.VerifyRun(run.Records, nil)
			state := "VALIDATED"
			reason := ""
			if verifyErr != nil {
				state = "BLOCKED"
				reason = verifyErr.Error()
			}
			counts := evidenceProofCounts(run.Records)
			output.WriteString("| `" + markdownCell(run.ID) + "` | **" + state + "** | " + fmt.Sprint(len(run.Records)) + " | " + fmt.Sprint(report.CellCount) + " | " + fmt.Sprint(report.PassedCells) + " | " + fmt.Sprint(report.CleanupVerified) + " | " + fmt.Sprint(counts.architecture) + " | " + fmt.Sprint(counts.resilience) + " | " + fmt.Sprint(counts.observability) + " | " + fmt.Sprint(counts.edge) + " | " + markdownCell(run.GeneratedAt) + " | " + markdownCell(strings.Join(run.Sources, ", ")) + " | " + markdownCell(reason) + " |\n")
		}
		output.WriteString("\nValidated input files:\n\n")
		for _, file := range bundle.Files {
			output.WriteString("- `" + markdownCell(file.Path) + "` (" + fmt.Sprint(file.RecordCount) + " records)\n")
		}
	}

	output.WriteString("\n## Recovery runbook contract\n\n")
	output.WriteString("This generated runbook is the provider-neutral execution order. Provider adapters supply the native operation IDs; no provider API success is inferred from this document.\n\n")
	output.WriteString("| Stage | Required proof | Operator boundary |\n| --- | --- | --- |\n")
	for _, stage := range []struct{ name, proof, owner string }{
		{"admission", "account, quota, credential references, state backend, fixture, and ownership scope", "automation blocks mutation until ready"},
		{"backup", "independent backup ID, retention, encryption, immutability/deletion protection, and completed operation", "provider adapter executes and polls"},
		{"restore", "destination, restore operation ID, resource identity, and measured duration", "provider adapter executes; operator approves destructive restore-in-place"},
		{"integrity", "known-content manifest, checksums, counts, reads, permissions, and service health", "automation verifies; operator investigates mismatch"},
		{"fencing", "single-writer lease or provider fencing identity", "operator approval is required before a second runtime can accept writes"},
		{"failover", "route convergence, stale-origin prevention, measured RTO, and rollback/failback result", "operator owns regional or alternate-provider approval"},
		{"cleanup", "direct owning-service inventory, delayed tombstones, and protected-resource reconciliation", "automation cleans owned resources; operator resolves protected or ambiguous objects"},
	} {
		output.WriteString("| `" + stage.name + "` | " + stage.proof + " | " + stage.owner + " |\n")
	}

	output.WriteString("\n## Certification evidence classes\n\n")
	output.WriteString("Every claimed profile must carry independent evidence for architecture identity, immutable artifact, compatibility, backup, restore, integrity, HA, DR, fencing/failback, observability, edge behavior, cost/duration, and cleanup. Missing, stale, non-reproducible, or neighboring-architecture evidence remains non-certified.\n")
	return []byte(output.String())
}

type evidenceRun struct {
	ID          string
	Records     []certification.Record
	GeneratedAt string
	Sources     []string
}

func evidenceRuns(records []certification.Record) []evidenceRun {
	byID := make(map[string]*evidenceRun)
	for _, record := range records {
		run := byID[record.RunID]
		if run == nil {
			run = &evidenceRun{ID: record.RunID}
			byID[record.RunID] = run
		}
		run.Records = append(run.Records, record)
		if run.GeneratedAt == "" || record.Provenance.GeneratedAt > run.GeneratedAt {
			run.GeneratedAt = record.Provenance.GeneratedAt
		}
		if !contains(run.Sources, record.Provenance.Source) {
			run.Sources = append(run.Sources, record.Provenance.Source)
		}
	}
	runs := make([]evidenceRun, 0, len(byID))
	for _, run := range byID {
		sort.Strings(run.Sources)
		runs = append(runs, *run)
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].ID < runs[j].ID })
	return runs
}

type evidenceProofCount struct {
	architecture  int
	resilience    int
	observability int
	edge          int
}

func evidenceProofCounts(records []certification.Record) evidenceProofCount {
	var count evidenceProofCount
	for _, record := range records {
		if record.Type != certification.RecordCell {
			continue
		}
		if record.Architecture.ProfileID != "" {
			count.architecture++
		}
		count.resilience += len(record.Resilience)
		count.observability += len(record.Observability)
		count.edge += len(record.Edge)
	}
	return count
}

func contains(values []string, value string) bool {
	for _, existing := range values {
		if existing == value {
			return true
		}
	}
	return false
}

func catalogTime(catalog certification.CapabilityCatalog) time.Time {
	if parsed, err := time.Parse(time.RFC3339, catalog.UpdatedAt); err == nil {
		return parsed.UTC()
	}
	parsed, err := time.Parse("2006-01-02", catalog.UpdatedAt)
	if err != nil {
		return time.Unix(0, 0).UTC()
	}
	return parsed.UTC()
}

func stringDestinations(values []sdk.RecoveryDestination) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, string(value))
	}
	return result
}

func markdownSource(source certification.CapabilitySource) string {
	result := "[official](" + source.URL + ") — retrieved " + source.RetrievedAt
	if source.ReferenceDate != "" {
		result += ", reference " + source.ReferenceDate
	}
	return result
}

func markdownSources(sources []certification.CapabilitySource) string {
	links := make([]string, 0, len(sources))
	for _, source := range sources {
		links = append(links, markdownSource(source))
	}
	return strings.Join(links, "<br>")
}

func markdownCell(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "|", "\\|"), "\n", " ")
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
