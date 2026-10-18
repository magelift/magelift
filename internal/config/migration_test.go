package config

import (
	"bytes"
	"strings"
	"testing"
)

func TestMigrateCurrentSchemaIsDeterministic(t *testing.T) {
	first, err := Migrate([]byte(base))
	if err != nil {
		t.Fatal(err)
	}
	if first.FromSchemaVersion != 1 || first.ToSchemaVersion != 1 {
		t.Fatalf("unexpected versions: %d to %d", first.FromSchemaVersion, first.ToSchemaVersion)
	}
	if !first.Changed {
		t.Fatal("expected compact input to be normalized")
	}

	second, err := Migrate(first.Data)
	if err != nil {
		t.Fatal(err)
	}
	if second.Changed {
		t.Fatal("canonical migration was not idempotent")
	}
	if !bytes.Equal(first.Data, second.Data) {
		t.Fatal("canonical output changed on the second migration")
	}
}

func TestMigrateRejectsUndefinedSchemaVersions(t *testing.T) {
	for _, version := range []string{"0", "2"} {
		t.Run(version, func(t *testing.T) {
			input := strings.Replace(base, "schemaVersion: 1", "schemaVersion: "+version, 1)
			_, err := Migrate([]byte(input))
			if err == nil || !strings.Contains(err.Error(), "cannot migrate schema version "+version) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestMigrateKeepsExtensionData(t *testing.T) {
	result, err := Migrate([]byte("# project configuration\n" + base))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(result.Data, []byte("vendor.example:")) || !bytes.Contains(result.Data, []byte("anything: true")) {
		t.Fatalf("extension data missing from migration:\n%s", result.Data)
	}
	if !bytes.Contains(result.Data, []byte("# project configuration")) {
		t.Fatalf("comment missing from migration:\n%s", result.Data)
	}
}
