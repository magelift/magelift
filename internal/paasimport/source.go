package paasimport

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SourceKind identifies a detected PaaS config root.
type SourceKind string

const (
	SourceACC   SourceKind = "acc"
	SourceUpsun SourceKind = "upsun"
)

const (
	accAppFile      = ".magento.app.yaml"
	accServicesFile = ".magento/services.yaml"
	accRoutesFile   = ".magento/routes.yaml"
	accEnvFile      = ".magento.env.yaml"
)

// DetectACC verifies root looks like an Adobe Commerce Cloud config tree.
func DetectACC(root string) error {
	root = filepath.Clean(root)
	info, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("ACC config root: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("ACC config root must be a directory: %s", root)
	}
	if _, err := os.Stat(filepath.Join(root, accAppFile)); err != nil {
		return fmt.Errorf("ACC config root requires %s: %w", accAppFile, err)
	}
	return nil
}

// DetectUpsun verifies root looks like an Upsun / Platform.sh config tree.
func DetectUpsun(root string) error {
	root = filepath.Clean(root)
	info, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("Upsun config root: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("Upsun config root must be a directory: %s", root)
	}
	if _, err := os.Stat(filepath.Join(root, upsunAppFile)); err != nil {
		return fmt.Errorf("Upsun config root requires %s: %w", upsunAppFile, err)
	}
	return nil
}

// ConfineConfigRoot cleans a maintainer-provided soak path and rejects escapes (T-04-07).
func ConfineConfigRoot(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("config root is empty")
	}
	if strings.Contains(path, "..") {
		return "", fmt.Errorf("config root must not contain ..")
	}
	cleaned := filepath.Clean(path)
	info, err := os.Stat(cleaned)
	if err != nil {
		return "", fmt.Errorf("config root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("config root must be a directory: %s", cleaned)
	}
	return cleaned, nil
}

// readUnder reads a file relative to root, rejecting path escapes.
func readUnder(root, rel string) ([]byte, error) {
	root = filepath.Clean(root)
	if rel == "" || strings.Contains(rel, "..") {
		return nil, fmt.Errorf("invalid config path %q", rel)
	}
	path := filepath.Clean(filepath.Join(root, rel))
	sep := string(os.PathSeparator)
	if path != root && !strings.HasPrefix(path, root+sep) {
		return nil, fmt.Errorf("path escapes config root: %s", rel)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", rel, err)
	}
	return data, nil
}
