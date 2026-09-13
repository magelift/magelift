package config

import (
	"strings"
	"testing"
)

func TestLocalRuntimeChoicesLoadAndResolve(t *testing.T) {
	input := strings.Replace(base, "target: {", "local:\n  database: {family: mysql, version: \"8.4\"}\n  webServer: {family: nginx, version: \"1.30\"}\n  webCache: {family: varnish, version: \"8\"}\n  phpSettings: {memory_limit: 1G}\n  email: {mode: smtp, host: mail.example.test, port: 2525, username: smtp-user, from: shop@example.test}\ntarget: {", 1)
	input = strings.Replace(input, "  shared:\n    build:\n      staticContent: {strategy: standard}\n", "  shared: {}\n", 1)
	input = strings.Replace(input, "    build:\n      staticContent: {locales: [de_DE], strategy: null}\n", "", 1)
	file, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	spec, err := file.ResolveBuild()
	if err != nil {
		t.Fatal(err)
	}
	if spec.Local.Database.Family != "mysql" || spec.Local.WebServer.Family != "nginx" || spec.Local.WebCache.Version != "8" || spec.Local.Email.Port != 2525 || spec.Local.Email.Username != "smtp-user" || spec.Local.PHPSettings["memory_limit"] != "1G" {
		t.Fatalf("local runtime = %#v", spec.Local)
	}
}

func TestLocalRuntimeRejectsComposeInterpolationCharacters(t *testing.T) {
	input := strings.Replace(base, "target: {", "local:\n  phpSettings: {memory_limit: \"${SECRET}\"}\ntarget: {", 1)
	input = strings.Replace(input, "  shared:\n    build:\n      staticContent: {strategy: standard}\n", "  shared: {}\n", 1)
	input = strings.Replace(input, "    build:\n      staticContent: {locales: [de_DE], strategy: null}\n", "", 1)
	file, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.ResolveBuild(); err == nil || !strings.Contains(err.Error(), "forbidden character") {
		t.Fatalf("error = %v", err)
	}
}

func TestLocalRuntimeRejectsSMTPWithoutEndpoint(t *testing.T) {
	input := strings.Replace(base, "target: {", "local:\n  email: {mode: smtp}\ntarget: {", 1)
	input = strings.Replace(input, "  shared:\n    build:\n      staticContent: {strategy: standard}\n", "  shared: {}\n", 1)
	input = strings.Replace(input, "    build:\n      staticContent: {locales: [de_DE], strategy: null}\n", "", 1)
	file, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.ResolveBuild(); err == nil || !strings.Contains(err.Error(), "local.email.smtp requires host and port") {
		t.Fatalf("error = %v", err)
	}
}
