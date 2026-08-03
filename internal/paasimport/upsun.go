package paasimport

import (
	"errors"
	"fmt"
	"os"

	"go.yaml.in/yaml/v4"
)

const (
	upsunAppFile      = ".platform.app.yaml"
	upsunServicesFile = ".platform/services.yaml"
	upsunRoutesFile   = ".platform/routes.yaml"
	upsunEnvFile      = ".platform.env.yaml"
)

// MapUpsun maps an Upsun / Platform.sh config root into MageLift YAML + residuals.
func MapUpsun(root string) (Result, error) {
	if err := DetectUpsun(root); err != nil {
		return Result{}, err
	}

	appData, err := readUnder(root, upsunAppFile)
	if err != nil {
		return Result{}, err
	}
	var raw accAppRaw
	if err := yaml.Unmarshal(appData, &raw); err != nil {
		return Result{}, fmt.Errorf("parse %s: %w", upsunAppFile, err)
	}

	doc, unmapped, err := mapAppTree(upsunAppFile, raw)
	if err != nil {
		return Result{}, err
	}

	if servicesData, err := readUnder(root, upsunServicesFile); err == nil {
		var services map[string]any
		if err := yaml.Unmarshal(servicesData, &services); err != nil {
			return Result{}, fmt.Errorf("parse %s: %w", upsunServicesFile, err)
		}
		unmapped = append(unmapped, mapServices(upsunServicesFile, services)...)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Result{}, err
	}

	if routesData, err := readUnder(root, upsunRoutesFile); err == nil {
		var routes map[string]accRoute
		if err := yaml.Unmarshal(routesData, &routes); err != nil {
			return Result{}, fmt.Errorf("parse %s: %w", upsunRoutesFile, err)
		}
		staging := doc.Environments["staging"]
		if staging.Domain == "" {
			staging.Domain = domainFromRoutes(routes)
		}
		doc.Environments["staging"] = staging
	} else if !errors.Is(err, os.ErrNotExist) {
		return Result{}, err
	}

	if envData, err := readUnder(root, upsunEnvFile); err == nil {
		var env accEnvDocument
		if err := yaml.Unmarshal(envData, &env); err != nil {
			return Result{}, fmt.Errorf("parse %s: %w", upsunEnvFile, err)
		}
		unmapped = append(unmapped, applyAllowlistEnv(&doc, upsunEnvFile, env)...)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Result{}, err
	}

	sortUnmapped(unmapped)
	yamlBytes, err := emitMagelift(doc)
	if err != nil {
		return Result{}, err
	}
	return Result{YAML: yamlBytes, Unmapped: unmapped}, nil
}
