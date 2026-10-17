package cli

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v4"
)

type workflowWriteResult struct {
	Changed       bool
	CandidatePath string
}

func writeWorkflow(path string, contents []byte, replace bool) (workflowWriteResult, error) {
	path = filepath.Clean(path)
	dir := filepath.Dir(path)

	staged, err := stageWorkflowBytes(dir, contents, 0o644)
	if err != nil {
		return workflowWriteResult{}, err
	}
	defer func() { _ = os.Remove(staged) }()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return workflowWriteResult{}, err
	}
	if err := os.Link(staged, path); err == nil {
		_ = os.Remove(staged)
		return workflowWriteResult{Changed: true}, nil
	} else if !errors.Is(err, os.ErrExist) {
		return workflowWriteResult{}, err
	}

	info, err := os.Lstat(path)
	if err != nil {
		return workflowWriteResult{}, err
	}
	if !info.Mode().IsRegular() {
		return workflowWriteResult{}, invalid(errors.New("refusing to replace a non-regular workflow file; use a regular file path"))
	}
	current, err := os.ReadFile(path)
	if err != nil {
		return workflowWriteResult{}, err
	}
	if bytes.Equal(current, contents) {
		return workflowWriteResult{Changed: false}, nil
	}
	if !replace {
		candidate, err := publishWorkflowCandidate(dir, filepath.Base(path), contents)
		if err != nil {
			return workflowWriteResult{}, err
		}
		return workflowWriteResult{CandidatePath: candidate}, guided(
			"the GitHub Actions workflow already exists and differs from the MageLift template",
			"compare with diff -u "+shellQuote(path)+" "+shellQuote(candidate)+"; rerun your original ci generate command with --replace to adopt the template",
			"docs/bootstrap.md",
		)
	}
	mode := info.Mode().Perm()
	if err := replaceFile(path, contents, mode); err != nil {
		return workflowWriteResult{}, err
	}
	return workflowWriteResult{Changed: true}, nil
}

func stageWorkflowBytes(dir string, contents []byte, mode os.FileMode) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	temporary, err := os.CreateTemp(dir, ".magelift-workflow-*")
	if err != nil {
		return "", err
	}
	temporaryPath := temporary.Name()
	closeAndRemove := func() {
		if temporary != nil {
			_ = temporary.Close()
		}
		_ = os.Remove(temporaryPath)
	}
	if err := temporary.Chmod(mode); err != nil {
		closeAndRemove()
		return "", err
	}
	if _, err := temporary.Write(contents); err != nil {
		closeAndRemove()
		return "", err
	}
	if err := temporary.Sync(); err != nil {
		closeAndRemove()
		return "", err
	}
	if err := temporary.Close(); err != nil {
		closeAndRemove()
		return "", err
	}
	return temporaryPath, nil
}

func publishWorkflowCandidate(dir, workflowBase string, contents []byte) (string, error) {
	pattern := workflowBase + ".candidate-*"
	temporary, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	temporaryPath := temporary.Name()
	cleanup := func() { _ = os.Remove(temporaryPath) }
	if _, err := temporary.Write(contents); err != nil {
		_ = temporary.Close()
		cleanup()
		return "", err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		cleanup()
		return "", err
	}
	if err := temporary.Close(); err != nil {
		cleanup()
		return "", err
	}
	return temporaryPath, nil
}

func validateWorkflowFile(path string, expected []byte, checkTemplate bool) (map[string]any, error) {
	path = filepath.Clean(path)
	scope := []string{"project-config", "workflow-presence", "workflow-yaml-syntax"}

	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, invalid(errors.New("GitHub Actions workflow is missing; run magelift ci generate"))
		}
		return nil, invalid(fmt.Errorf("read generated workflow: %w", err))
	}
	if !info.Mode().IsRegular() {
		return nil, invalid(errors.New("workflow path must be a regular file"))
	}
	actual, err := os.ReadFile(path)
	if err != nil {
		return nil, invalid(fmt.Errorf("read generated workflow: %w", err))
	}
	if len(bytes.TrimSpace(actual)) == 0 {
		return nil, invalid(errors.New("workflow file is empty"))
	}
	if err := parseWorkflowYAML(actual); err != nil {
		return nil, invalid(fmt.Errorf("workflow YAML syntax is invalid: %w", err))
	}

	templateMatches := bytes.Equal(actual, expected)
	result := map[string]any{
		"path":            path,
		"sha256":          fmt.Sprintf("%x", sha256.Sum256(actual)),
		"valid":           true,
		"validationScope": scope,
		"templateMatches": templateMatches,
	}
	if checkTemplate {
		result["validationScope"] = append(scope, "workflow-template-freshness")
		if !templateMatches {
			return nil, guided(
				"the GitHub Actions workflow does not match the current MageLift template",
				"run magelift ci generate --replace to adopt the template, or keep your customized workflow without --check-template",
				"docs/bootstrap.md",
			)
		}
	}
	return result, nil
}

func parseWorkflowYAML(data []byte) error {
	var document any
	if err := yaml.Unmarshal(data, &document); err != nil {
		return err
	}
	if document == nil {
		return errors.New("workflow contains no YAML document")
	}
	return nil
}
