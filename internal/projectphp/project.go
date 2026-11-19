// Package projectphp evaluates and writes MageLift PHP project definitions.
package projectphp

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

//go:embed sdk.php
var sdkSource string

var phpDiagnostic = regexp.MustCompile(`^[A-Za-z\\][A-Za-z0-9_\\]* in [A-Za-z0-9_.-]+:[0-9]+$`)

const bootstrap = `
namespace {
    try {
        ob_start();
        $definition = require $argv[1];
        $unexpectedOutput = ob_get_clean();
        if ($unexpectedOutput !== '') {
            throw new \UnexpectedValueException('Project configuration produced output');
        }
        if ($argv[2] === 'project' && !$definition instanceof \MageLift\Project) {
            throw new \UnexpectedValueException('Project configuration must return a MageLift\\Project');
        }
        if ($argv[2] === 'overlay' && !is_array($definition)) {
            throw new \UnexpectedValueException('Environment overlay must return an array');
        }
        echo json_encode($definition, JSON_THROW_ON_ERROR | JSON_UNESCAPED_SLASHES | JSON_INVALID_UTF8_SUBSTITUTE);
    } catch (\Throwable $error) {
        if (ob_get_level() > 0) {
            ob_end_clean();
        }
        fwrite(STDERR, get_class($error) . ' in ' . basename($error->getFile()) . ':' . $error->getLine() . PHP_EOL);
        exit(1);
    }
}
`

// SDKPath is an IDE-visible copy of the PHP types shipped with the CLI.
func SDKPath(configPath string) string {
	return filepath.Join(filepath.Dir(filepath.Clean(configPath)), ".magelift", "sdk.php")
}

// InstallSDK installs the exact SDK used by the evaluator for IDE completion.
func InstallSDK(configPath string) error {
	path := SDKPath(configPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// Keep generated configuration, local credentials and build output private
	// even when the shop has never used MageLift and has no root ignore rule.
	ignore, err := os.OpenFile(filepath.Join(filepath.Dir(path), ".gitignore"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err == nil {
		_, werr := ignore.WriteString("*\n")
		if err := errors.Join(werr, ignore.Close()); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrExist) {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".sdk-*.php")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if err = temp.Chmod(0644); err == nil {
		_, err = temp.WriteString(sdkSource)
	}
	if err = errors.Join(err, temp.Close()); err != nil {
		return err
	}
	return os.Rename(temp.Name(), path)
}

// OverlayPath is the tracked, tool-managed companion to the user project.
func OverlayPath(configPath string) string {
	clean := filepath.Clean(configPath)
	return strings.TrimSuffix(clean, filepath.Ext(clean)) + ".environments.php"
}

// Evaluate returns a normalized JSON project, including managed environment edits.
// PHP project files are executable code and run with the caller's privileges.
func Evaluate(ctx context.Context, configPath string) ([]byte, error) {
	document, err := EvaluateProject(ctx, configPath)
	if err != nil {
		return nil, err
	}
	overlay, err := ReadOverlay(ctx, configPath)
	if err != nil {
		return nil, err
	}
	return ApplyOverlay(document, overlay)
}

// EvaluateProject evaluates the authored project without its managed overlay.
func EvaluateProject(ctx context.Context, configPath string) ([]byte, error) {
	if strings.ToLower(filepath.Ext(configPath)) != ".php" {
		return nil, errors.New("project configuration must be a PHP file; import PaaS configuration with magelift init --from-acc or --from-upsun")
	}
	return evaluate(ctx, configPath, "project")
}

func evaluate(ctx context.Context, path, kind string) ([]byte, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(absolute); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	source := strings.TrimPrefix(sdkSource, "<?php") + bootstrap
	// A bracketed namespace cannot be mixed with the SDK's unbracketed form.
	source = strings.Replace(source, "namespace MageLift;", "namespace MageLift {", 1)
	source = strings.Replace(source, "\nnamespace {", "\n}\nnamespace {", 1)
	command := exec.CommandContext(ctx, "php", "-d", "display_errors=0", "-d", "log_errors=0", "-r", source, "--", absolute, kind)
	command.Dir = filepath.Dir(absolute)
	var stdout limitedBuffer
	var stderr limitedBuffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		diagnostic := strings.TrimSpace(stderr.String())
		if !stderr.exceeded && phpDiagnostic.MatchString(diagnostic) {
			return nil, fmt.Errorf("evaluate %s: %s", filepath.Base(path), diagnostic)
		}
		return nil, fmt.Errorf("evaluate %s: %w", filepath.Base(path), err)
	}
	if stdout.exceeded {
		return nil, errors.New("project configuration exceeds 16 MiB")
	}
	if !json.Valid(stdout.Bytes()) {
		return nil, fmt.Errorf("evaluate %s: PHP returned invalid JSON", filepath.Base(path))
	}
	return stdout.Bytes(), nil
}

// limitedBuffer prevents a misbehaving project from filling CLI memory.
type limitedBuffer struct {
	bytes.Buffer
	exceeded bool
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	const limit = 16 << 20
	if len(data) > limit-b.Len() {
		b.exceeded = true
		return len(data), nil
	}
	return b.Buffer.Write(data)
}

// Overlay is the data managed by environment commands.
type Overlay struct {
	Environments map[string]map[string]any `json:"environments"`
	Removed      []string                  `json:"removed"`
}

func ReadOverlay(ctx context.Context, configPath string) (Overlay, error) {
	path := OverlayPath(configPath)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return Overlay{Environments: map[string]map[string]any{}}, nil
	} else if err != nil {
		return Overlay{}, err
	}
	data, err := evaluate(ctx, path, "overlay")
	if err != nil {
		return Overlay{}, err
	}
	var overlay Overlay
	if err := json.Unmarshal(data, &overlay); err != nil {
		return Overlay{}, fmt.Errorf("decode environment overlay: %w", err)
	}
	if overlay.Environments == nil {
		overlay.Environments = map[string]map[string]any{}
	}
	return overlay, nil
}

// ApplyOverlay combines the authored configuration with tool-managed edits.
func ApplyOverlay(data []byte, overlay Overlay) ([]byte, error) {
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("decode project configuration: %w", err)
	}
	if document == nil {
		return nil, errors.New("project configuration must be an object")
	}
	environments, ok := document["environments"].(map[string]any)
	if !ok {
		return nil, errors.New("project environments must be a map")
	}
	for _, name := range overlay.Removed {
		delete(environments, name)
	}
	for name, values := range overlay.Environments {
		current, exists := environments[name]
		if !exists {
			current = map[string]any{}
		}
		base, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("environment %q must be a map", name)
		}
		merge(base, values)
		environments[name] = base
	}
	return json.Marshal(document)
}

func merge(base map[string]any, values map[string]any) {
	for key, value := range values {
		child, isMap := value.(map[string]any)
		current, currentIsMap := base[key].(map[string]any)
		if isMap && currentIsMap {
			merge(current, child)
		} else {
			base[key] = value
		}
	}
}

// Render emits an editable, typed PHP project from normalized project data.
func Render(document map[string]any) ([]byte, error) {
	value, err := normalize(document)
	if err != nil {
		return nil, err
	}
	return []byte("<?php\n\ndeclare(strict_types=1);\n\nuse MageLift\\Project;\n\nreturn new Project(" + phpValue(value, 0) + ");\n"), nil
}

// RenderOverlay emits only the CLI-managed changes, leaving project code untouched.
func RenderOverlay(overlay Overlay) ([]byte, error) {
	if overlay.Environments == nil {
		overlay.Environments = map[string]map[string]any{}
	}
	if overlay.Removed == nil {
		overlay.Removed = []string{}
	}
	value, err := normalize(overlay)
	if err != nil {
		return nil, err
	}
	return []byte("<?php\n\ndeclare(strict_types=1);\n\n// Managed by MageLift environment commands.\nreturn " + phpValue(value, 0) + ";\n"), nil
}

func normalize(value any) (any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var result any
	if err := decoder.Decode(&result); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return result, nil
}

func phpValue(value any, level int) string {
	switch v := value.(type) {
	case nil:
		return "null"
	case bool:
		return strconv.FormatBool(v)
	case json.Number:
		return v.String()
	case string:
		return "'" + strings.ReplaceAll(strings.ReplaceAll(v, "\\", "\\\\"), "'", "\\'") + "'"
	case []any:
		if len(v) == 0 {
			return "[]"
		}
		lines := make([]string, 0, len(v))
		for _, child := range v {
			lines = append(lines, strings.Repeat("    ", level+1)+phpValue(child, level+1)+",")
		}
		return "[\n" + strings.Join(lines, "\n") + "\n" + strings.Repeat("    ", level) + "]"
	case map[string]any:
		if len(v) == 0 {
			return "(object) []"
		}
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		lines := make([]string, 0, len(keys))
		for _, key := range keys {
			lines = append(lines, strings.Repeat("    ", level+1)+phpValue(key, level+1)+" => "+phpValue(v[key], level+1)+",")
		}
		return "[\n" + strings.Join(lines, "\n") + "\n" + strings.Repeat("    ", level) + "]"
	default:
		panic(fmt.Sprintf("unsupported normalized PHP value %T", value))
	}
}
