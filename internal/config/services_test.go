package config

import (
	"strings"
	"testing"
)

func TestServiceBindingsOnlyProjectTheirOwnRole(t *testing.T) {
	value := map[string]any{
		"target":   map[string]any{"provider": "aws", "aws": map[string]any{"catalog": map[string]any{"databaseEngine": "rds-mysql"}}},
		"services": map[string]any{"cache": map[string]any{"family": "mysql", "version": "8.4"}},
	}
	if err := resolveServiceBindings(value, map[string]Provenance{}); err != nil {
		t.Fatal(err)
	}
	if got := nestedString(value, "target", "aws", "catalog", "databaseEngine"); got != "rds-mysql" {
		t.Fatalf("cache altered database engine: %q", got)
	}
	if got := nestedString(value, "target", "aws", "catalog", "versions", "mysql"); got != "" {
		t.Fatalf("cache altered database version: %q", got)
	}
	cfg := Config{Target: Target{Provider: "aws", Runtime: "ecs-fargate"}, Services: map[string]ServiceBinding{"cache": {Family: "mysql"}}}
	if err := cfg.CheckServiceCapabilities(); err == nil || !strings.Contains(err.Error(), "service adapter") {
		t.Fatalf("capability error = %v", err)
	}
}

func TestExternalServiceCapabilitiesNeverIgnoreCredentialsOrPorts(t *testing.T) {
	secret := "aws-secrets-manager://arn:aws:secretsmanager:eu-west-3:123456789012:secret:service"
	tests := []struct {
		name    string
		role    string
		binding ServiceBinding
		want    string
	}{
		{"database", "database", ServiceBinding{Family: "mysql", Ownership: "external", Endpoint: "db.example", Secret: secret}, ""},
		{"cache", "cache", ServiceBinding{Family: "valkey", Ownership: "external", Endpoint: "cache.example", Secret: secret}, ""},
		{"queue", "queue", ServiceBinding{Family: "rabbitmq", Ownership: "external", Endpoint: "amqps://queue.example:5671", Secret: secret, Username: "shop"}, ""},
		{"search auth", "search", ServiceBinding{Family: "opensearch", Ownership: "external", Endpoint: "https://search.example", Secret: secret}, "authenticated"},
		{"database port", "database", ServiceBinding{Family: "mysql", Ownership: "external", Endpoint: "db.example:3307", Secret: secret}, "port"},
		{"cache missing secret", "cache", ServiceBinding{Family: "valkey", Ownership: "external", Endpoint: "cache.example"}, "Secrets Manager"},
		{"queue missing username", "queue", ServiceBinding{Family: "rabbitmq", Ownership: "external", Endpoint: "queue.example", Secret: secret}, "username"},
		{"gcp invalid secret", "database", ServiceBinding{Family: "mysql", Ownership: "external", Endpoint: "db.example", Secret: secret, Username: "shop"}, "Secret Manager"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, runtime := "aws", "ecs-fargate"
			if tt.name == "gcp invalid secret" {
				provider, runtime = "gcp", "gke-autopilot"
			}
			cfg := Config{Target: Target{Provider: provider, Runtime: runtime}, Services: map[string]ServiceBinding{tt.role: tt.binding}}
			err := cfg.CheckServiceCapabilities()
			if tt.want == "" && err != nil {
				t.Fatal(err)
			}
			if tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestServiceAddressHonorsExplicitPort(t *testing.T) {
	tests := []struct {
		binding ServiceBinding
		want    string
	}{
		{ServiceBinding{Endpoint: "search.example", Port: 9201}, "search.example:9201"},
		{ServiceBinding{Endpoint: "https://search.example", Port: 9201}, "https://search.example:9201"},
		{ServiceBinding{Endpoint: "amqps://queue.example:5671"}, "amqps://queue.example:5671"},
	}
	for _, tt := range tests {
		if got := tt.binding.Address(); got != tt.want {
			t.Errorf("Address() = %q, want %q", got, tt.want)
		}
	}
}

func TestGCPServiceVersionsKeepExplicitChoices(t *testing.T) {
	for _, test := range []struct{ database, cache, wantSQL, wantCache string }{
		{"8", "8", "MYSQL_8_0", "VALKEY_8_0"},
		{"8.0.35", "8.1", "MYSQL_8_0", "VALKEY_8_1"},
		{"8.4.3", "9.0.1", "MYSQL_8_4", "VALKEY_9_0"},
	} {
		value := map[string]any{
			"target": map[string]any{"provider": "gcp"},
			"services": map[string]any{
				"database": map[string]any{"family": "mysql", "version": test.database},
				"cache":    map[string]any{"family": "valkey", "version": test.cache},
			},
		}
		if err := resolveServiceBindings(value, map[string]Provenance{}); err != nil {
			t.Fatal(err)
		}
		if got := nestedString(value, "target", "gcp", "cloudSqlDatabaseVersion"); got != test.wantSQL {
			t.Errorf("Cloud SQL version = %q, want %q", got, test.wantSQL)
		}
		if got := nestedString(value, "target", "gcp", "memorystoreEngineVersion"); got != test.wantCache {
			t.Errorf("Memorystore version = %q, want %q", got, test.wantCache)
		}
		if got := nestedString(value, "local", "cache", "version"); got != test.cache {
			t.Errorf("local cache version = %q, want %q", got, test.cache)
		}
	}
}

func TestGCPRedisOnlySupportsSelfHostedOrAttached(t *testing.T) {
	for _, role := range []string{"cache", "session"} {
		for _, tc := range []struct {
			ownership string
			accepted  bool
		}{
			{"self-hosted", true}, {"managed", false}, {"", false},
		} {
			cfg := Config{Target: Target{Provider: "gcp", Runtime: "gke-autopilot"}, Services: map[string]ServiceBinding{role: {Family: "redis", Ownership: tc.ownership, Version: "7.2"}}}
			err := cfg.CheckServiceCapabilities()
			if (err == nil) != tc.accepted {
				t.Errorf("%s ownership %q: %v", role, tc.ownership, err)
			}
		}
	}
}
