package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/magelift/magelift/internal/certification"
	"github.com/spf13/cobra"
)

func certificationCommand(o *options) *cobra.Command {
	command := &cobra.Command{
		Use:   "certification",
		Short: "Inspect provider cells and verify acceptance evidence",
	}

	targets := &cobra.Command{
		Use:   "targets",
		Short: "List provider architecture descriptors",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return o.write(certification.CurrentTargetMatrix())
		},
	}

	var target, release, edition, preset string
	cells := &cobra.Command{
		Use:   "cells",
		Short: "Expand one provider target into architecture cells",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if target == "" {
				return invalid(fmt.Errorf("--target is required"))
			}
			values, err := certification.CellsForTarget(target, release, edition, preset)
			if err != nil {
				return invalid(err)
			}
			return o.write(values)
		},
	}
	cells.Flags().StringVar(&target, "target", "", "target ID, such as aws/ecs-fargate")
	cells.Flags().StringVar(&release, "release", "2.4.9", "exact Adobe Commerce release")
	cells.Flags().StringVar(&edition, "edition", "open-source", "Magento edition: open-source or commerce")
	cells.Flags().StringVar(&preset, "preset", "preview", "environment preset")

	plan := &cobra.Command{
		Use:   "plan",
		Short: "Print a side-effect-free certification execution plan",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if target == "" {
				return invalid(fmt.Errorf("--target is required"))
			}
			value, err := certification.PlanForTarget(target, release, edition, preset)
			if err != nil {
				return invalid(err)
			}
			return o.write(value)
		},
	}
	plan.Flags().StringVar(&target, "target", "", "target ID, such as gcp/gke-standard")
	plan.Flags().StringVar(&release, "release", "2.4.9", "exact Adobe Commerce release")
	plan.Flags().StringVar(&edition, "edition", "open-source", "Magento edition: open-source or commerce")
	plan.Flags().StringVar(&preset, "preset", "preview", "environment preset")

	var file, runID string
	var required []string
	verify := &cobra.Command{
		Use:   "verify",
		Short: "Verify sealed JSONL acceptance evidence",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if file == "" {
				return invalid(fmt.Errorf("--file is required"))
			}
			data, err := os.ReadFile(file)
			if err != nil {
				return invalid(err)
			}
			records, err := certification.Read(bytes.NewReader(data))
			if err != nil {
				return invalid(err)
			}
			selected, err := selectRun(records, runID)
			if err != nil {
				return invalid(err)
			}
			report, verifyErr := certification.VerifyRun(selected, required)
			if writeErr := o.write(report); writeErr != nil {
				return writeErr
			}
			if verifyErr != nil {
				return invalid(verifyErr)
			}
			return nil
		},
	}
	verify.Flags().StringVar(&file, "file", "", "JSONL evidence file")
	verify.Flags().StringVar(&runID, "run-id", "", "run ID when the file contains more than one run")
	verify.Flags().StringArrayVar(&required, "required-cell", nil, "required stable cell ID; repeat for multiple cells")

	var sealInput, sealOutput string
	seal := &cobra.Command{
		Use:   "seal",
		Short: "Seal unsealed acceptance candidates with the core evidence contract",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if sealInput == "" {
				return invalid(fmt.Errorf("--file is required"))
			}
			if sealOutput == "" {
				sealOutput = defaultSealedEvidencePath(sealInput)
			}
			if err := sealEvidenceFile(sealInput, sealOutput); err != nil {
				return invalid(err)
			}
			return o.write(map[string]string{"input": sealInput, "output": sealOutput})
		},
	}
	seal.Flags().StringVar(&sealInput, "file", "", "unsealed JSONL evidence candidates")
	seal.Flags().StringVar(&sealOutput, "output-file", "", "sealed JSONL output file (default: input with .sealed.jsonl suffix)")

	var docsOutputDir, docsEvidenceFile, docsRunID string
	docs := &cobra.Command{
		Use:   "docs",
		Short: "Generate source-dated release documents from the catalog and sealed evidence",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			var records []certification.Record
			if docsEvidenceFile != "" {
				data, err := os.ReadFile(docsEvidenceFile)
				if err != nil {
					return invalid(err)
				}
				parsed, err := certification.Read(bytes.NewReader(data))
				if err != nil {
					return invalid(err)
				}
				records, err = selectRun(parsed, docsRunID)
				if err != nil {
					return invalid(err)
				}
			}
			bundle, err := certification.GenerateDocumentation(certification.CurrentCapabilityCatalog(), records, time.Now().UTC())
			if err != nil {
				return invalid(err)
			}
			if docsOutputDir == "" {
				docsOutputDir = filepath.Join("docs", "generated")
			}
			if err := os.MkdirAll(docsOutputDir, 0o755); err != nil {
				return invalid(err)
			}
			files := make([]string, 0, len(bundle.Files))
			for name := range bundle.Files {
				files = append(files, name)
			}
			sort.Strings(files)
			for _, name := range files {
				if err := os.WriteFile(filepath.Join(docsOutputDir, name), []byte(bundle.Files[name]), 0o644); err != nil {
					return invalid(err)
				}
			}
			return o.write(map[string]any{"generatedAt": bundle.GeneratedAt, "outputDir": docsOutputDir, "files": files})
		},
	}
	docs.Flags().StringVar(&docsOutputDir, "output-dir", "", "directory for generated Markdown documents")
	docs.Flags().StringVar(&docsEvidenceFile, "evidence-file", "", "sealed JSONL evidence file to include")
	docs.Flags().StringVar(&docsRunID, "run-id", "", "run ID when the evidence file contains more than one run")

	command.AddCommand(targets, cells, plan, seal, verify, docs)
	return command
}

func defaultSealedEvidencePath(input string) string {
	return strings.TrimSuffix(input, ".jsonl") + ".sealed.jsonl"
}

func sealEvidenceFile(inputPath, outputPath string) error {
	inputAbsolute, err := filepath.Abs(inputPath)
	if err != nil {
		return fmt.Errorf("resolve evidence input: %w", err)
	}
	outputAbsolute, err := filepath.Abs(outputPath)
	if err != nil {
		return fmt.Errorf("resolve evidence output: %w", err)
	}
	if inputAbsolute == outputAbsolute {
		return fmt.Errorf("sealed evidence output must differ from the unsealed input")
	}

	input, err := os.Open(inputAbsolute)
	if err != nil {
		return fmt.Errorf("open evidence candidates: %w", err)
	}
	defer input.Close()

	outputDirectory := filepath.Dir(outputAbsolute)
	if err := os.MkdirAll(outputDirectory, 0o755); err != nil {
		return fmt.Errorf("create sealed evidence directory: %w", err)
	}
	temporary, err := os.CreateTemp(outputDirectory, ".magelift-evidence-seal-*.jsonl")
	if err != nil {
		return fmt.Errorf("create temporary sealed evidence file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if err := certification.SealJSONL(input, temporary); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync sealed evidence: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close sealed evidence: %w", err)
	}
	if err := os.Rename(temporaryPath, outputAbsolute); err != nil {
		return fmt.Errorf("publish sealed evidence: %w", err)
	}
	if err := os.Chmod(outputAbsolute, 0o644); err != nil {
		return fmt.Errorf("set sealed evidence permissions: %w", err)
	}
	return nil
}

func selectRun(records []certification.Record, runID string) ([]certification.Record, error) {
	if runID != "" {
		selected := make([]certification.Record, 0)
		for _, record := range records {
			if record.RunID == runID {
				selected = append(selected, record)
			}
		}
		if len(selected) == 0 {
			return nil, fmt.Errorf("run %q is not present in the evidence file", runID)
		}
		return selected, nil
	}
	runs := make(map[string]struct{})
	for _, record := range records {
		runs[record.RunID] = struct{}{}
	}
	if len(runs) > 1 {
		ids := make([]string, 0, len(runs))
		for id := range runs {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		return nil, fmt.Errorf("evidence file contains multiple runs (%v); pass --run-id", ids)
	}
	return records, nil
}
