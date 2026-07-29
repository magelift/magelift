package paasimport

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"go.yaml.in/yaml/v4"
)

type accApp struct {
	Name          string            `yaml:"name"`
	Type          string            `yaml:"type"`
	Relationships map[string]string `yaml:"relationships"`
}

type accRoute struct {
	Type     string `yaml:"type"`
	Upstream string `yaml:"upstream"`
	To       string `yaml:"to"`
}

type accEnvDocument struct {
	Stage struct {
		Global map[string]string `yaml:"global"`
		Deploy map[string]string `yaml:"deploy"`
		Build  map[string]string `yaml:"build"`
	} `yaml:"stage"`
	Variables struct {
		Env map[string]string `yaml:"env"`
	} `yaml:"variables"`
}

// MapACC reads an ACC config root and returns MageLift YAML bytes.
// The supported fixture shape is fully mappable (exit 0 path without sidecar).
func MapACC(root string) ([]byte, error) {
	if err := DetectACC(root); err != nil {
		return nil, err
	}

	appData, err := readUnder(root, accAppFile)
	if err != nil {
		return nil, err
	}
	var app accApp
	if err := yaml.Unmarshal(appData, &app); err != nil {
		return nil, fmt.Errorf("parse %s: %w", accAppFile, err)
	}
	if strings.TrimSpace(app.Name) == "" {
		return nil, fmt.Errorf("%s: name is required", accAppFile)
	}
	php, err := parsePHPType(app.Type)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", accAppFile, err)
	}

	doc := baseDocument(app.Name, php)
	if catalog := catalogFromRelationships(app.Relationships); catalog != nil {
		if doc.Target.AWS == nil {
			doc.Target.AWS = &mageliftAWS{}
		}
		doc.Target.AWS.Catalog = catalog
	}

	if servicesData, err := readUnder(root, accServicesFile); err == nil {
		var services map[string]any
		if err := yaml.Unmarshal(servicesData, &services); err != nil {
			return nil, fmt.Errorf("parse %s: %w", accServicesFile, err)
		}
		_ = services // relationships already carry service intent for the tracer
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	if routesData, err := readUnder(root, accRoutesFile); err == nil {
		var routes map[string]accRoute
		if err := yaml.Unmarshal(routesData, &routes); err != nil {
			return nil, fmt.Errorf("parse %s: %w", accRoutesFile, err)
		}
		staging := doc.Environments["staging"]
		staging.Domain = domainFromRoutes(routes)
		doc.Environments["staging"] = staging
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	if envData, err := readUnder(root, accEnvFile); err == nil {
		var env accEnvDocument
		if err := yaml.Unmarshal(envData, &env); err != nil {
			return nil, fmt.Errorf("parse %s: %w", accEnvFile, err)
		}
		applyAllowlistEnv(&doc, env)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	return emitMagelift(doc)
}
