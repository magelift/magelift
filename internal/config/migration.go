package config

import (
	"bytes"
	"fmt"

	"go.yaml.in/yaml/v4"
)

const CurrentSchemaVersion = 1

type Migration struct {
	FromSchemaVersion int      `json:"fromSchemaVersion" yaml:"fromSchemaVersion"`
	ToSchemaVersion   int      `json:"toSchemaVersion" yaml:"toSchemaVersion"`
	Changed           bool     `json:"changed" yaml:"changed"`
	Diagnostics       []string `json:"diagnostics" yaml:"diagnostics"`
	Data              []byte   `json:"-" yaml:"-"`
}

// Migrate returns a canonical representation of a supported configuration.
// Field transformations belong in explicit version-to-version migrations once
// a second schema has shipped.
func Migrate(data []byte) (Migration, error) {
	var header struct {
		SchemaVersion int `yaml:"schemaVersion"`
	}
	if err := yaml.Unmarshal(data, &header); err != nil {
		return Migration{}, fmt.Errorf("read schema version: %w", err)
	}
	if header.SchemaVersion != CurrentSchemaVersion {
		return Migration{}, fmt.Errorf(
			"cannot migrate schema version %d: supported version is %d",
			header.SchemaVersion,
			CurrentSchemaVersion,
		)
	}

	if _, err := Load(data); err != nil {
		return Migration{}, err
	}
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return Migration{}, fmt.Errorf("decode config for migration: %w", err)
	}
	canonical, err := yaml.Marshal(&node)
	if err != nil {
		return Migration{}, fmt.Errorf("encode migrated config: %w", err)
	}

	changed := !bytes.Equal(data, canonical)
	diagnostic := "schema version 1 is current"
	if changed {
		diagnostic = "schema version 1 is current; normalized YAML formatting"
	}
	return Migration{
		FromSchemaVersion: CurrentSchemaVersion,
		ToSchemaVersion:   CurrentSchemaVersion,
		Changed:           changed,
		Diagnostics:       []string{diagnostic},
		Data:              canonical,
	}, nil
}
