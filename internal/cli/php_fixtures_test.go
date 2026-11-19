package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/magelift/magelift/internal/projectphp"
	"go.yaml.in/yaml/v4"
)

// writeTestProjectFile renders existing fixture descriptions as PHP when they
// are used as project files. Other test data is written unchanged.
func writeTestProjectFile(path string, data []byte, mode os.FileMode) error {
	if filepath.Base(path) != "magelift.php" || strings.HasPrefix(strings.TrimSpace(string(data)), "<?php") {
		return os.WriteFile(path, data, mode)
	}
	var document map[string]any
	if err := yaml.Unmarshal(data, &document); err != nil {
		return err
	}
	php, err := projectphp.Render(document)
	if err != nil {
		return err
	}
	return os.WriteFile(path, php, mode)
}

// readTestProjectYAML provides a mutable fixture representation for tests
// that exercise a sequence of configuration edits.
func readTestProjectYAML(path string) ([]byte, error) {
	data, err := projectphp.Evaluate(context.Background(), path)
	if err != nil {
		return nil, err
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	return yaml.Marshal(document)
}
