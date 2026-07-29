package paasimport

import (
	"fmt"
	"net/url"
	"strings"

	"go.yaml.in/yaml/v4"
)

// mageliftDocument is the subset of MageLift YAML emitted by the importer.
// Fields must already exist on starterConfig / schema (IMPORT-04).
type mageliftDocument struct {
	SchemaVersion int                    `yaml:"schemaVersion"`
	Project       mageliftProject        `yaml:"project"`
	Application   mageliftApplication    `yaml:"application"`
	Build         mageliftBuild          `yaml:"build"`
	Target        mageliftTarget         `yaml:"target"`
	Defaults      mageliftDefaults       `yaml:"defaults"`
	Environments  map[string]mageliftEnv `yaml:"environments"`
	Extensions    map[string]any         `yaml:"extensions"`
}

type mageliftProject struct {
	Name string `yaml:"name"`
}

type mageliftApplication struct {
	Edition string `yaml:"edition"`
	Version string `yaml:"version"`
	Mode    string `yaml:"mode"`
}

type mageliftBuild struct {
	PHP           string         `yaml:"php"`
	StaticContent map[string]any `yaml:"staticContent,omitempty"`
}

type mageliftTarget struct {
	Provider string       `yaml:"provider"`
	Runtime  string       `yaml:"runtime"`
	AWS      *mageliftAWS `yaml:"aws,omitempty"`
}

type mageliftAWS struct {
	EncryptionKeySecretARN string         `yaml:"encryptionKeySecretArn,omitempty"`
	Catalog                map[string]any `yaml:"catalog,omitempty"`
}

type mageliftDefaults struct {
	Region string `yaml:"region"`
	Preset string `yaml:"preset"`
}

type mageliftEnv struct {
	Account string `yaml:"account"`
	Domain  string `yaml:"domain,omitempty"`
}

// defaultEncryptionKeyPlaceholder is a secret-ref-shaped ARN placeholder.
// Crypt plaintext from PaaS env must never be written into magelift.yaml (T-04-02).
const defaultEncryptionKeyPlaceholder = "arn:aws:secretsmanager:eu-west-3:123456789012:secret:magelift/encryption-key"

func emitMagelift(doc mageliftDocument) ([]byte, error) {
	data, err := yaml.Marshal(&doc)
	if err != nil {
		return nil, fmt.Errorf("encode magelift.yaml: %w", err)
	}
	return data, nil
}

func parsePHPType(typeValue string) (string, error) {
	typeValue = strings.TrimSpace(typeValue)
	if typeValue == "" {
		return "", fmt.Errorf("app type is required")
	}
	branch, ok := strings.CutPrefix(typeValue, "php:")
	if !ok || branch == "" {
		return "", fmt.Errorf("unsupported app type %q (want php:<version>)", typeValue)
	}
	return branch, nil
}

func domainFromRoutes(routes map[string]accRoute) string {
	for pattern := range routes {
		if strings.Contains(pattern, "{default}") {
			continue
		}
		u, err := url.Parse(pattern)
		if err != nil || u.Host == "" {
			continue
		}
		return u.Hostname()
	}
	return "staging.example.com"
}

func catalogFromRelationships(rels map[string]string) map[string]any {
	catalog := map[string]any{}
	for name, value := range rels {
		service := strings.ToLower(value)
		key := strings.ToLower(name)
		switch {
		case strings.Contains(service, "rabbitmq") || strings.Contains(service, "amqp") ||
			key == "rabbitmq" || key == "amqp" || key == "queue":
			catalog["queueMode"] = "ecs-rabbitmq"
		case strings.Contains(service, "opensearch") || strings.Contains(service, "elasticsearch") ||
			key == "opensearch" || key == "elasticsearch" || key == "search":
			catalog["searchMode"] = "serverless"
		}
	}
	if len(catalog) == 0 {
		return nil
	}
	return catalog
}

func applyAllowlistEnv(doc *mageliftDocument, env accEnvDocument) {
	vars := collectEnvVars(env)
	if strategy, ok := vars["SCD_STRATEGY"]; ok && strategy != "" {
		if doc.Build.StaticContent == nil {
			doc.Build.StaticContent = map[string]any{}
		}
		doc.Build.StaticContent["strategy"] = strategy
	}
	if threads, ok := vars["SCD_THREADS"]; ok && threads != "" {
		if doc.Build.StaticContent == nil {
			doc.Build.StaticContent = map[string]any{}
		}
		doc.Build.StaticContent["threads"] = threads
	}
	if _, ok := vars["CRYPT_KEY"]; ok {
		if doc.Target.AWS == nil {
			doc.Target.AWS = &mageliftAWS{}
		}
		doc.Target.AWS.EncryptionKeySecretARN = defaultEncryptionKeyPlaceholder
	}
}

func collectEnvVars(env accEnvDocument) map[string]string {
	out := map[string]string{}
	mergeStringMap(out, env.Stage.Global)
	mergeStringMap(out, env.Stage.Deploy)
	mergeStringMap(out, env.Stage.Build)
	mergeStringMap(out, env.Variables.Env)
	return out
}

func mergeStringMap(dst map[string]string, src map[string]string) {
	for k, v := range src {
		dst[k] = v
	}
}

func baseDocument(name, php string) mageliftDocument {
	return mageliftDocument{
		SchemaVersion: 1,
		Project:       mageliftProject{Name: name},
		Application: mageliftApplication{
			Edition: "open-source",
			Version: "2.4.9",
			Mode:    "integrated",
		},
		Build: mageliftBuild{PHP: php},
		Target: mageliftTarget{
			Provider: "aws",
			Runtime:  "ecs-fargate",
		},
		Defaults: mageliftDefaults{
			Region: "eu-west-3",
			Preset: "preview",
		},
		Environments: map[string]mageliftEnv{
			"staging": {Account: "123456789012"},
		},
		Extensions: map[string]any{},
	}
}
