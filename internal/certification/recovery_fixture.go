package certification

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/magelift/magelift/internal/secretref"
)

var recoveryDigestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// RecoveryFixtureManifest is the portable identity of a scrubbed recovery
// fixture. It records expected facts, never the fixture's secret values.
// Provider adapters can materialize it in a database, object store, broker,
// and application configuration without changing the verification contract.
type RecoveryFixtureManifest struct {
	ID      string                 `json:"id" yaml:"id"`
	Source  string                 `json:"source" yaml:"source"`
	Classes []RecoveryFixtureClass `json:"classes" yaml:"classes"`
}

type RecoveryFixtureClass struct {
	Name                      string   `json:"name" yaml:"name"`
	ExpectedRecords           int64    `json:"expectedRecords" yaml:"expectedRecords"`
	ExpectedObjects           int64    `json:"expectedObjects" yaml:"expectedObjects"`
	ManifestDigest            string   `json:"manifestDigest" yaml:"manifestDigest"`
	ContentDigest             string   `json:"contentDigest" yaml:"contentDigest"`
	PermissionDigest          string   `json:"permissionDigest" yaml:"permissionDigest"`
	SecretReferences          []string `json:"secretReferences,omitempty" yaml:"secretReferences,omitempty"`
	MaxRestoreDurationSeconds int64    `json:"maxRestoreDurationSeconds" yaml:"maxRestoreDurationSeconds"`
}

// RecoveryFixtureMaterial is the non-secret known content used to derive one
// class of a fixture manifest. The material is normally loaded from scrubbed
// repository fixtures or generated in memory by a provider contract test.
// Values are hashed immediately and are never returned in the manifest.
type RecoveryFixtureMaterial struct {
	Name                      string
	ExpectedRecords           int64
	ExpectedObjects           int64
	Manifest                  []byte
	Content                   []byte
	Permissions               []byte
	SecretReferences          []string
	MaxRestoreDurationSeconds int64
}

// RecoveryFixtureFile describes one scrubbed repository file that belongs to
// a recovery class. The file content is hashed immediately; it is never
// copied into the manifest or evidence.
type RecoveryFixtureFile struct {
	Name                      string
	Path                      string
	ExpectedRecords           int64
	ExpectedObjects           int64
	SecretReferences          []string
	MaxRestoreDurationSeconds int64
}

// BuildRecoveryFixtureManifest derives a stable manifest from known content.
// It makes it difficult for a provider adapter to accidentally verify only a
// row count while ignoring media, permissions, or configuration material.
func BuildRecoveryFixtureManifest(id, source string, materials []RecoveryFixtureMaterial) (RecoveryFixtureManifest, error) {
	manifest := RecoveryFixtureManifest{ID: id, Source: source, Classes: make([]RecoveryFixtureClass, 0, len(materials))}
	for _, material := range materials {
		manifest.Classes = append(manifest.Classes, RecoveryFixtureClass{
			Name: material.Name, ExpectedRecords: material.ExpectedRecords, ExpectedObjects: material.ExpectedObjects,
			ManifestDigest: digestBytes(material.Manifest), ContentDigest: digestBytes(material.Content), PermissionDigest: digestBytes(material.Permissions),
			SecretReferences: append([]string(nil), material.SecretReferences...), MaxRestoreDurationSeconds: material.MaxRestoreDurationSeconds,
		})
	}
	if err := manifest.Validate(); err != nil {
		return RecoveryFixtureManifest{}, err
	}
	return manifest, nil
}

// BuildRecoveryFixtureManifestFromFS builds one deterministic manifest from
// scrubbed files. The caller supplies semantic counts because file formats
// differ between database dumps, object manifests, queues, and templates;
// the loader still hashes content, path, and permission mode consistently.
func BuildRecoveryFixtureManifestFromFS(fsys fs.FS, id, source, root string, files []RecoveryFixtureFile) (RecoveryFixtureManifest, error) {
	if fsys == nil {
		return RecoveryFixtureManifest{}, errors.New("recovery fixture filesystem is required")
	}
	if len(files) == 0 {
		return RecoveryFixtureManifest{}, errors.New("recovery fixture filesystem requires at least one file")
	}
	root = path.Clean(strings.TrimSpace(root))
	if root == "" {
		root = "."
	}
	if path.IsAbs(root) || root == ".." || strings.HasPrefix(root, "../") {
		return RecoveryFixtureManifest{}, fmt.Errorf("recovery fixture root must be relative: %q", root)
	}
	ordered := append([]RecoveryFixtureFile(nil), files...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Name != ordered[j].Name {
			return ordered[i].Name < ordered[j].Name
		}
		return ordered[i].Path < ordered[j].Path
	})
	materials := make([]RecoveryFixtureMaterial, 0, len(ordered))
	for _, file := range ordered {
		if strings.TrimSpace(file.Name) == "" || strings.TrimSpace(file.Path) == "" {
			return RecoveryFixtureManifest{}, errors.New("recovery fixture filesystem files require a name and path")
		}
		relative := path.Clean(strings.TrimSpace(file.Path))
		if path.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, "../") {
			return RecoveryFixtureManifest{}, fmt.Errorf("recovery fixture file path must stay below the root: %q", file.Path)
		}
		filePath := path.Join(root, relative)
		info, err := fs.Stat(fsys, filePath)
		if err != nil {
			return RecoveryFixtureManifest{}, fmt.Errorf("stat recovery fixture file %q: %w", filePath, err)
		}
		if !info.Mode().IsRegular() {
			return RecoveryFixtureManifest{}, fmt.Errorf("recovery fixture file %q is not regular", filePath)
		}
		content, err := fs.ReadFile(fsys, filePath)
		if err != nil {
			return RecoveryFixtureManifest{}, fmt.Errorf("read recovery fixture file %q: %w", filePath, err)
		}
		permissionMode := info.Mode().Perm().String()
		materials = append(materials, RecoveryFixtureMaterial{
			Name: file.Name, ExpectedRecords: file.ExpectedRecords, ExpectedObjects: file.ExpectedObjects,
			Manifest: []byte(relative + "\x00" + permissionMode), Content: content, Permissions: []byte(permissionMode),
			SecretReferences: append([]string(nil), file.SecretReferences...), MaxRestoreDurationSeconds: file.MaxRestoreDurationSeconds,
		})
	}
	return BuildRecoveryFixtureManifest(id, source, materials)
}

// DecodeRecoveryFixtureManifest reads a generated manifest and rejects trailing
// data so a concatenated or partially written file cannot be accepted.
func DecodeRecoveryFixtureManifest(reader io.Reader) (RecoveryFixtureManifest, error) {
	if reader == nil {
		return RecoveryFixtureManifest{}, errors.New("recovery fixture manifest reader is required")
	}
	decoder := json.NewDecoder(reader)
	var manifest RecoveryFixtureManifest
	if err := decoder.Decode(&manifest); err != nil {
		return RecoveryFixtureManifest{}, fmt.Errorf("decode recovery fixture manifest: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return RecoveryFixtureManifest{}, errors.New("recovery fixture manifest contains trailing data")
		}
		return RecoveryFixtureManifest{}, fmt.Errorf("read recovery fixture manifest trailer: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return RecoveryFixtureManifest{}, err
	}
	return manifest, nil
}

func digestBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

// RecoveryFixtureObservation contains only post-restore observations. A
// provider must resolve secret references and report permissions, but it must
// never put the resolved secret values into this structure.
type RecoveryFixtureObservation struct {
	Name                     string   `json:"name" yaml:"name"`
	Records                  int64    `json:"records" yaml:"records"`
	Objects                  int64    `json:"objects" yaml:"objects"`
	ManifestDigest           string   `json:"manifestDigest" yaml:"manifestDigest"`
	ContentDigest            string   `json:"contentDigest" yaml:"contentDigest"`
	PermissionDigest         string   `json:"permissionDigest" yaml:"permissionDigest"`
	SecretReferences         []string `json:"secretReferences,omitempty" yaml:"secretReferences,omitempty"`
	PermissionsOK            bool     `json:"permissionsOk" yaml:"permissionsOk"`
	ApplicationReadsOK       bool     `json:"applicationReadsOk" yaml:"applicationReadsOk"`
	SecretReferencesResolved bool     `json:"secretReferencesResolved" yaml:"secretReferencesResolved"`
	ServiceHealthy           bool     `json:"serviceHealthy" yaml:"serviceHealthy"`
	RestoreDurationSeconds   int64    `json:"restoreDurationSeconds" yaml:"restoreDurationSeconds"`
}

// Validate checks the fixture identity and known-content expectations before
// any cloud resource is created. It rejects inline secrets by requiring every
// secret input to use the repository's typed reference parser.
func (m RecoveryFixtureManifest) Validate() error {
	if strings.TrimSpace(m.ID) == "" || strings.TrimSpace(m.Source) == "" {
		return errors.New("recovery fixture ID and source are required")
	}
	if len(m.Classes) == 0 {
		return errors.New("recovery fixture requires at least one data class")
	}
	seen := make(map[string]struct{}, len(m.Classes))
	for _, class := range m.Classes {
		if strings.TrimSpace(class.Name) == "" {
			return errors.New("recovery fixture data class name is required")
		}
		if _, exists := seen[class.Name]; exists {
			return fmt.Errorf("recovery fixture contains duplicate data class %q", class.Name)
		}
		seen[class.Name] = struct{}{}
		if class.ExpectedRecords < 0 || class.ExpectedObjects < 0 {
			return fmt.Errorf("recovery fixture data class %q has a negative expected count", class.Name)
		}
		if class.MaxRestoreDurationSeconds < 0 {
			return fmt.Errorf("recovery fixture data class %q has a negative restore duration limit", class.Name)
		}
		for name, digest := range map[string]string{"manifest": class.ManifestDigest, "content": class.ContentDigest, "permission": class.PermissionDigest} {
			if !recoveryDigestPattern.MatchString(strings.TrimSpace(digest)) {
				return fmt.Errorf("recovery fixture data class %q has an invalid %s digest", class.Name, name)
			}
		}
		for _, reference := range class.SecretReferences {
			if _, err := secretref.Parse(reference); err != nil {
				return fmt.Errorf("recovery fixture data class %q secret reference: %w", class.Name, err)
			}
		}
	}
	return nil
}

// VerifyRecoveryFixture compares every declared class, count, checksum,
// permission result, and service-health result. Missing or extra classes are
// failures; a partial restore cannot pass by matching only the database.
func VerifyRecoveryFixture(manifest RecoveryFixtureManifest, observations []RecoveryFixtureObservation) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	expected := make(map[string]RecoveryFixtureClass, len(manifest.Classes))
	for _, class := range manifest.Classes {
		expected[class.Name] = class
	}
	seen := make(map[string]struct{}, len(observations))
	var problems []string
	for _, observation := range observations {
		if err := validateRecoveryFixtureObservation(observation); err != nil {
			problems = append(problems, fmt.Sprintf("data class %q observation: %v", observation.Name, err))
			continue
		}
		class, ok := expected[observation.Name]
		if !ok {
			problems = append(problems, fmt.Sprintf("unexpected recovered data class %q", observation.Name))
			continue
		}
		if _, exists := seen[observation.Name]; exists {
			problems = append(problems, fmt.Sprintf("duplicate recovered data class %q", observation.Name))
			continue
		}
		seen[observation.Name] = struct{}{}
		if observation.Records != class.ExpectedRecords {
			problems = append(problems, fmt.Sprintf("data class %q record count %d does not match %d", observation.Name, observation.Records, class.ExpectedRecords))
		}
		if observation.Objects != class.ExpectedObjects {
			problems = append(problems, fmt.Sprintf("data class %q object count %d does not match %d", observation.Name, observation.Objects, class.ExpectedObjects))
		}
		if observation.ManifestDigest != class.ManifestDigest || observation.ContentDigest != class.ContentDigest || observation.PermissionDigest != class.PermissionDigest {
			problems = append(problems, fmt.Sprintf("data class %q checksum does not match the fixture manifest", observation.Name))
		}
		if !sameRecoveryReferences(observation.SecretReferences, class.SecretReferences) {
			problems = append(problems, fmt.Sprintf("data class %q secret references do not match the fixture manifest", observation.Name))
		}
		if observation.RestoreDurationSeconds <= 0 {
			problems = append(problems, fmt.Sprintf("data class %q has no measured restore duration", observation.Name))
		}
		if class.MaxRestoreDurationSeconds > 0 && observation.RestoreDurationSeconds > class.MaxRestoreDurationSeconds {
			problems = append(problems, fmt.Sprintf("data class %q restore duration %d seconds exceeds %d second limit", observation.Name, observation.RestoreDurationSeconds, class.MaxRestoreDurationSeconds))
		}
		if !observation.PermissionsOK {
			problems = append(problems, fmt.Sprintf("data class %q permission verification failed", observation.Name))
		}
		if !observation.ApplicationReadsOK {
			problems = append(problems, fmt.Sprintf("data class %q application read verification failed", observation.Name))
		}
		if !observation.SecretReferencesResolved {
			problems = append(problems, fmt.Sprintf("data class %q secret-reference verification failed", observation.Name))
		}
		if !observation.ServiceHealthy {
			problems = append(problems, fmt.Sprintf("data class %q service health verification failed", observation.Name))
		}
	}
	for name := range expected {
		if _, exists := seen[name]; !exists {
			problems = append(problems, fmt.Sprintf("missing recovered data class %q", name))
		}
	}
	sort.Strings(problems)
	return errors.Join(stringErrors(problems)...)
}

func validateRecoveryFixtureObservation(observation RecoveryFixtureObservation) error {
	if strings.TrimSpace(observation.Name) == "" {
		return errors.New("data class name is required")
	}
	if strings.ContainsAny(observation.Name, "\r\n\x00") {
		return errors.New("data class name must be single-line and NUL-free")
	}
	for _, reference := range observation.SecretReferences {
		if _, err := secretref.Parse(reference); err != nil {
			return fmt.Errorf("secret reference: %w", err)
		}
	}
	if err := ValidateSecretSafeValue(observation); err != nil {
		return fmt.Errorf("secret-safety check: %w", err)
	}
	return nil
}

func sameRecoveryReferences(left, right []string) bool {
	left = append([]string(nil), left...)
	right = append([]string(nil), right...)
	sort.Strings(left)
	sort.Strings(right)
	return slicesEqual(left, right)
}

func slicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func stringErrors(messages []string) []error {
	result := make([]error, 0, len(messages))
	for _, message := range messages {
		result = append(result, errors.New(message))
	}
	return result
}
