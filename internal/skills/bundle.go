package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/magelift/magelift/agents"
	"go.yaml.in/yaml/v4"
)

var skillNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

type ManifestEntry struct {
	Name        string   `json:"name" yaml:"name"`
	Description string   `json:"description" yaml:"description"`
	Version     string   `json:"version" yaml:"version"`
	Digest      string   `json:"digest" yaml:"digest"`
	Sources     []string `json:"sources" yaml:"sources"`
}

type InstallOptions struct {
	ProjectRoot string
	HomeRoot    string
	SourceRoot  string
	Agent       string
	Scope       string
	Mode        string
	Replace     bool
	Skills      []string
}

type InstallResult struct {
	Name string `json:"name" yaml:"name"`
	Path string `json:"path" yaml:"path"`
	Mode string `json:"mode" yaml:"mode"`
}

type VerificationItem struct {
	Name           string `json:"name" yaml:"name"`
	Path           string `json:"path" yaml:"path"`
	Status         string `json:"status" yaml:"status"`
	ExpectedDigest string `json:"expectedDigest" yaml:"expectedDigest"`
	ActualDigest   string `json:"actualDigest,omitempty" yaml:"actualDigest,omitempty"`
	Detail         string `json:"detail,omitempty" yaml:"detail,omitempty"`
}

type VerificationReport struct {
	Agent string             `json:"agent" yaml:"agent"`
	Scope string             `json:"scope" yaml:"scope"`
	Root  string             `json:"root" yaml:"root"`
	Clean bool               `json:"clean" yaml:"clean"`
	Items []VerificationItem `json:"items" yaml:"items"`
}

type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Version     string `yaml:"version"`
}

// BundledManifest returns the verified manifest derived from the embedded
// first-party skill files.
func BundledManifest() ([]ManifestEntry, error) {
	entries, err := fs.ReadDir(agents.Files, "skills")
	if err != nil {
		return nil, fmt.Errorf("read bundled skills: %w", err)
	}
	manifest := make([]ManifestEntry, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !skillNamePattern.MatchString(entry.Name()) {
			continue
		}
		name := entry.Name()
		data, err := embeddedSkill(name)
		if err != nil {
			return nil, err
		}
		metadata, err := parseFrontmatter(data)
		if err != nil {
			return nil, fmt.Errorf("skill %q: %w", name, err)
		}
		if metadata.Name != name {
			return nil, fmt.Errorf("skill %q declares frontmatter name %q", name, metadata.Name)
		}
		version := metadata.Version
		if version == "" {
			version = "1.0.0"
		}
		manifest = append(manifest, ManifestEntry{
			Name:        name,
			Description: metadata.Description,
			Version:     version,
			Digest:      digest(data),
			Sources:     []string{"embedded", "skills-cli"},
		})
	}
	sort.Slice(manifest, func(i, j int) bool { return manifest[i].Name < manifest[j].Name })
	return manifest, nil
}

func Install(options InstallOptions) ([]InstallResult, error) {
	root, err := installationRoot(options)
	if err != nil {
		return nil, err
	}
	if err := validateMode(options.Mode); err != nil {
		return nil, err
	}
	installMode := options.Mode
	if installMode == "" {
		installMode = "auto"
	}
	manifest, err := BundledManifest()
	if err != nil {
		return nil, err
	}
	selected, err := selectManifest(manifest, options.Skills)
	if err != nil {
		return nil, err
	}
	sourceRoot := options.SourceRoot
	if sourceRoot == "" {
		projectRoot, projectErr := projectRoot(options.ProjectRoot)
		if projectErr == nil {
			sourceRoot = FindSourceRoot(projectRoot)
		}
	}
	results := make([]InstallResult, 0, len(selected))
	for _, entry := range selected {
		data, err := embeddedSkill(entry.Name)
		if err != nil {
			return nil, err
		}
		if digest(data) != entry.Digest {
			return nil, fmt.Errorf("skill %q changed after manifest creation", entry.Name)
		}
		target := filepath.Join(root, entry.Name)
		if err := ensureWithin(root, target); err != nil {
			return nil, err
		}
		if err := checkExisting(target, entry.Digest, options.Replace); err != nil {
			return nil, err
		}
		if _, err := os.Lstat(target); err == nil {
			results = append(results, InstallResult{Name: entry.Name, Path: target, Mode: "unchanged"})
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("inspect skill destination %q: %w", target, err)
		}

		mode := installMode
		localPath := localSkillPath(sourceRoot, entry.Name)
		if mode == "auto" {
			mode = "copy"
			if localPath != "" {
				mode = "symlink"
			}
		}
		switch mode {
		case "copy":
			if err := writeSkillCopy(target, data); err != nil {
				return nil, err
			}
		case "symlink":
			if localPath == "" {
				return nil, fmt.Errorf("skill %q has no local source for symlink mode; use --mode copy", entry.Name)
			}
			localData, err := os.ReadFile(localPath)
			if err != nil {
				return nil, fmt.Errorf("read local skill %q: %w", entry.Name, err)
			}
			if digest(localData) != entry.Digest {
				return nil, fmt.Errorf("local skill %q does not match the bundled digest", entry.Name)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return nil, fmt.Errorf("create skill destination: %w", err)
			}
			if err := os.Symlink(filepath.Dir(localPath), target); err != nil {
				return nil, fmt.Errorf("link skill %q: %w", entry.Name, err)
			}
		default:
			return nil, fmt.Errorf("unsupported skill install mode %q", installMode)
		}
		results = append(results, InstallResult{Name: entry.Name, Path: target, Mode: mode})
	}
	return results, nil
}

func Verify(options InstallOptions) (VerificationReport, error) {
	root, err := installationRoot(options)
	if err != nil {
		return VerificationReport{}, err
	}
	manifest, err := BundledManifest()
	if err != nil {
		return VerificationReport{}, err
	}
	selected, err := selectManifest(manifest, options.Skills)
	if err != nil {
		return VerificationReport{}, err
	}
	report := VerificationReport{Agent: options.Agent, Scope: options.Scope, Root: root, Clean: true, Items: make([]VerificationItem, 0, len(selected))}
	for _, entry := range selected {
		path := filepath.Join(root, entry.Name)
		item := VerificationItem{Name: entry.Name, Path: path, ExpectedDigest: entry.Digest, Status: "ok"}
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			item.Status = "missing"
			item.Detail = "skill directory is not installed"
			report.Clean = false
			report.Items = append(report.Items, item)
			continue
		}
		if err != nil {
			return VerificationReport{}, fmt.Errorf("inspect installed skill %q: %w", entry.Name, err)
		}
		if !info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			item.Status = "drift"
			item.Detail = "skill destination is not a directory"
			report.Clean = false
			report.Items = append(report.Items, item)
			continue
		}
		data, err := os.ReadFile(filepath.Join(path, "SKILL.md"))
		if err != nil {
			item.Status = "drift"
			item.Detail = "SKILL.md cannot be read"
			report.Clean = false
			report.Items = append(report.Items, item)
			continue
		}
		item.ActualDigest = digest(data)
		if item.ActualDigest != entry.Digest {
			item.Status = "drift"
			item.Detail = "SKILL.md digest does not match the bundled manifest"
			report.Clean = false
		}
		if extra := unexpectedSkillFiles(path); len(extra) > 0 {
			item.Status = "drift"
			item.Detail = "unexpected files: " + strings.Join(extra, ", ")
			report.Clean = false
		}
		report.Items = append(report.Items, item)
	}
	return report, nil
}

// FindSourceRoot walks upward looking for the tracked agents/skills tree.
func FindSourceRoot(start string) string {
	directory, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "agents", "skills")); err == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return ""
		}
		directory = parent
	}
}

// SkillsCLIArgs builds argv for the optional official skills CLI backend.
// It never invokes a shell and accepts only paths discovered by FindSourceRoot.
func SkillsCLIArgs(sourceRoot, agent, scope, mode string, names []string) ([]string, error) {
	if sourceRoot == "" {
		return nil, errors.New("a local MageLift source root is required for the skills CLI backend")
	}
	if _, err := os.Stat(filepath.Join(sourceRoot, "agents", "skills")); err != nil {
		return nil, fmt.Errorf("invalid MageLift source root: %w", err)
	}
	if err := validateAgent(agent); err != nil {
		return nil, err
	}
	if err := validateMode(mode); err != nil {
		return nil, err
	}
	if scope != "project" && scope != "global" {
		return nil, fmt.Errorf("unsupported skill scope %q", scope)
	}
	cliAgent, err := skillsCLIAgent(agent)
	if err != nil {
		return nil, err
	}
	args := []string{"skills", "add", filepath.Join(sourceRoot, "agents", "skills"), "--agent", cliAgent, "--yes"}
	if scope == "global" {
		args = append(args, "--global")
	}
	if mode == "copy" {
		args = append(args, "--copy")
	}
	for _, name := range names {
		if !skillNamePattern.MatchString(name) {
			return nil, fmt.Errorf("invalid skill name %q", name)
		}
		args = append(args, "--skill", name)
	}
	return args, nil
}

func RunSkillsCLI(ctx context.Context, sourceRoot, agent, scope, mode string, names []string, stdout, stderr io.Writer) error {
	args, err := SkillsCLIArgs(sourceRoot, agent, scope, mode, names)
	if err != nil {
		return err
	}
	if _, err := exec.LookPath("npx"); err != nil {
		return errors.New("npx is required for the skills CLI backend; use the direct backend instead")
	}
	commandContext, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	command := exec.CommandContext(commandContext, "npx", args...)
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("run skills CLI: %w", err)
	}
	return nil
}

func embeddedSkill(name string) ([]byte, error) {
	if !skillNamePattern.MatchString(name) {
		return nil, fmt.Errorf("invalid skill name %q", name)
	}
	data, err := agents.Files.ReadFile(filepath.ToSlash(filepath.Join("skills", name, "SKILL.md")))
	if err != nil {
		return nil, fmt.Errorf("read bundled skill %q: %w", name, err)
	}
	return data, nil
}

func parseFrontmatter(data []byte) (frontmatter, error) {
	text := string(data)
	if !strings.HasPrefix(text, "---\n") {
		return frontmatter{}, errors.New("missing frontmatter")
	}
	rest := text[len("---\n"):]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		return frontmatter{}, errors.New("unterminated frontmatter")
	}
	var metadata frontmatter
	if err := yaml.Unmarshal([]byte(rest[:end]), &metadata); err != nil {
		return frontmatter{}, fmt.Errorf("parse frontmatter: %w", err)
	}
	if metadata.Name == "" || metadata.Description == "" {
		return frontmatter{}, errors.New("frontmatter needs name and description")
	}
	return metadata, nil
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func projectRoot(value string) (string, error) {
	if value == "" {
		var err error
		value, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("find project root: %w", err)
		}
	}
	root, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("resolve project root: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("project root %q is not a directory", root)
	}
	return root, nil
}

func installationRoot(options InstallOptions) (string, error) {
	if err := validateAgent(options.Agent); err != nil {
		return "", err
	}
	if options.Scope != "project" && options.Scope != "global" {
		return "", fmt.Errorf("unsupported skill scope %q", options.Scope)
	}
	project, err := projectRoot(options.ProjectRoot)
	if err != nil {
		return "", err
	}
	home := options.HomeRoot
	if home == "" {
		home, err = os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("find home directory: %w", err)
		}
	}
	home, err = filepath.Abs(home)
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	base := project
	if options.Scope == "global" {
		base = home
	}
	directory := agentDirectory(options.Agent, options.Scope)
	root := filepath.Join(base, directory)
	if err := ensureWithin(base, root); err != nil {
		return "", err
	}
	return root, nil
}

func agentDirectory(agent, scope string) string {
	if scope == "global" {
		switch agent {
		case "codex":
			return filepath.Join(".codex", "skills")
		case "claude", "claude-code":
			return filepath.Join(".claude", "skills")
		case "cursor":
			return filepath.Join(".cursor", "skills")
		default:
			return filepath.Join(".agents", "skills")
		}
	}
	switch agent {
	case "claude", "claude-code":
		return filepath.Join(".claude", "skills")
	case "cursor":
		return filepath.Join(".cursor", "skills")
	default:
		return filepath.Join(".agents", "skills")
	}
}

func validateAgent(agent string) error {
	switch agent {
	case "codex", "claude", "claude-code", "cursor", "generic":
		return nil
	default:
		return fmt.Errorf("unsupported agent %q; use codex, claude-code, cursor, or generic", agent)
	}
}

func skillsCLIAgent(agent string) (string, error) {
	switch agent {
	case "codex", "cursor":
		return agent, nil
	case "claude", "claude-code":
		return "claude-code", nil
	case "generic":
		return "", errors.New("the skills CLI backend needs a named agent; use codex, claude-code, or cursor")
	default:
		return "", fmt.Errorf("unsupported skills CLI agent %q", agent)
	}
}

func validateMode(mode string) error {
	if mode == "" {
		return nil
	}
	switch mode {
	case "auto", "copy", "symlink":
		return nil
	default:
		return fmt.Errorf("unsupported skill install mode %q; use auto, copy, or symlink", mode)
	}
}

func selectManifest(manifest []ManifestEntry, names []string) ([]ManifestEntry, error) {
	if len(names) == 0 {
		return manifest, nil
	}
	byName := make(map[string]ManifestEntry, len(manifest))
	for _, entry := range manifest {
		byName[entry.Name] = entry
	}
	selected := make([]ManifestEntry, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if _, exists := seen[name]; exists {
			continue
		}
		entry, found := byName[name]
		if !found {
			return nil, fmt.Errorf("skill %q is not bundled", name)
		}
		selected = append(selected, entry)
		seen[name] = struct{}{}
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].Name < selected[j].Name })
	return selected, nil
}

func localSkillPath(sourceRoot, name string) string {
	if sourceRoot == "" {
		return ""
	}
	path := filepath.Join(sourceRoot, "agents", "skills", name, "SKILL.md")
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}

func checkExisting(path, expectedDigest string, replace bool) error {
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect skill destination %q: %w", path, err)
	}
	data, err := os.ReadFile(filepath.Join(path, "SKILL.md"))
	if err == nil && digest(data) == expectedDigest {
		return nil
	}
	if !replace {
		return fmt.Errorf("skill destination %q already exists and does not match MageLift; pass --replace to replace it", path)
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("replace skill destination %q: %w", path, err)
	}
	return nil
}

func writeSkillCopy(target string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create skill destination: %w", err)
	}
	temporary, err := os.MkdirTemp(filepath.Dir(target), ".magelift-skill-")
	if err != nil {
		return fmt.Errorf("stage skill: %w", err)
	}
	defer os.RemoveAll(temporary)
	if err := os.WriteFile(filepath.Join(temporary, "SKILL.md"), data, 0o644); err != nil {
		return fmt.Errorf("write staged skill: %w", err)
	}
	if err := os.Rename(temporary, target); err != nil {
		return fmt.Errorf("install skill: %w", err)
	}
	return nil
}

func unexpectedSkillFiles(path string) []string {
	entries, err := os.ReadDir(path)
	if err != nil {
		return []string{"<unreadable>"}
	}
	extra := make([]string, 0)
	for _, entry := range entries {
		if entry.Name() != "SKILL.md" {
			extra = append(extra, entry.Name())
		}
	}
	sort.Strings(extra)
	return extra
}

func ensureWithin(root, target string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolve destination: %w", err)
	}
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("skill destination %q is outside %q", target, root)
	}
	return nil
}
