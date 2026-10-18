package localdev

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/config"
	"go.yaml.in/yaml/v4"
)

func TestPlanUsesReleaseSpecificPinnedServices(t *testing.T) {
	plan, err := Plan(config.BuildSpec{
		Application: config.Application{Version: "2.4.6-p15"},
		Build:       config.Build{PHP: "8.2", Composer: config.Composer{Version: "2.2.26+"}, Extensions: []string{"redis", "intl"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Database.Family != "mariadb" || plan.Database.Version != "10.11" || !strings.Contains(plan.Database.Image, "sha256:") {
		t.Fatalf("database plan = %#v", plan.Database)
	}
	if plan.Search.Version != "2" || plan.Cache.Version != "8.1" || plan.Queue.Version != "4.2" {
		t.Fatalf("service plan = %#v", plan)
	}
	if plan.Database.Connection.Host != "database" || plan.Database.Connection.Port != 3306 || plan.Database.Connection.PasswordEnv != "MAGENTO_DC_DB__CONNECTION__DEFAULT__PASSWORD" {
		t.Fatalf("database connection = %#v", plan.Database.Connection)
	}
	if len(plan.Database.Credentials) != 2 || plan.Database.Credentials[1].Environment != "MAGENTO_DC_DB__CONNECTION__DEFAULT__PASSWORD" {
		t.Fatalf("database credentials = %#v", plan.Database.Credentials)
	}
	if plan.Queue.Connection.ManagementPort != 15672 || plan.Queue.Connection.PasswordEnv != "MAGENTO_DC_QUEUE__AMQP__PASSWORD" {
		t.Fatalf("queue connection = %#v", plan.Queue.Connection)
	}
	if strings.Index(strings.Join(plan.Extensions, ","), "intl") > strings.Index(strings.Join(plan.Extensions, ","), "redis") {
		t.Fatalf("extensions are not deterministic: %#v", plan.Extensions)
	}
}

func TestPlanSupportsArtemisNginxAndVarnishContracts(t *testing.T) {
	plan, err := Plan(config.BuildSpec{
		Application: config.Application{Version: "2.4.9"},
		Build:       config.Build{PHP: "8.5", Composer: config.Composer{Version: "2.10"}},
		Local: config.LocalRuntime{
			Queue:     config.LocalService{Family: "artemis", Version: "2"},
			WebServer: config.LocalService{Family: "nginx", Version: "1.30"},
			WebCache:  config.LocalService{Family: "varnish", Version: "8"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.AppImage != "magelift/php-nginx:8.5-local" || plan.WebServer.Family != "nginx" {
		t.Fatalf("web runtime = %#v app=%q", plan.WebServer, plan.AppImage)
	}
	if plan.Queue.Family != "artemis" || plan.Queue.Connection.Scheme != "stomp" || plan.Queue.Connection.Port != 61613 {
		t.Fatalf("queue = %#v", plan.Queue)
	}
	if plan.WebCache.Family != "varnish" || !strings.Contains(plan.WebCache.Image, "sha256:") {
		t.Fatalf("web cache = %#v", plan.WebCache)
	}
	compose := ComposeTemplateFor(plan)
	for _, value := range []string{
		"apache/activemq-artemis:2.38.0-alpine@sha256:",
		"MAGENTO_DC_QUEUE__DEFAULT_CONNECTION: \"stomp\"",
		"MAGENTO_DC_QUEUE__STOMP__PORT: \"61613\"",
		"varnish:8@sha256:",
		"VARNISH_BACKEND_HOST: app",
		"php-fpm --daemonize && exec nginx",
		"MAGELIFT_LOCAL_APP_HTTP_PORT:-8081",
	} {
		if !strings.Contains(compose, value) {
			t.Fatalf("generated Compose lacks %q", value)
		}
	}
	if strings.Contains(compose, "__MAGELIFT_") {
		t.Fatal("generated Compose contains an unresolved template marker")
	}
	var document map[string]any
	if err := yaml.Unmarshal([]byte(compose), &document); err != nil {
		t.Fatalf("generated Compose is not valid YAML: %v", err)
	}
}

func TestPlanFollowsRegisteredWebRuntime(t *testing.T) {
	for _, test := range []struct {
		webRuntime string
		family     string
		image      string
		command    string
	}{
		{webRuntime: "nginx-fpm", family: "nginx", image: "magelift/php-nginx:8.5-local", command: "php-fpm --daemonize && exec nginx"},
		{webRuntime: "frankenphp-classic", family: "frankenphp-classic", image: "magelift/frankenphp-classic:8.5-local", command: `command: ["frankenphp", "run"]`},
		{webRuntime: "php-apache", family: "php-apache", image: "magelift/php-apache:8.5-local", command: "php-fpm --daemonize && exec apache2ctl -D FOREGROUND"},
	} {
		test := test
		t.Run(test.webRuntime, func(t *testing.T) {
			plan, err := Plan(config.BuildSpec{
				Application: config.Application{Version: "2.4.9", WebRuntime: test.webRuntime},
				Build:       config.Build{PHP: "8.5", Composer: config.Composer{Version: "2.10"}},
				Local:       config.LocalRuntime{WebServer: config.LocalService{Family: test.family}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if plan.WebServer.Family != test.family || plan.AppImage != test.image {
				t.Fatalf("%s plan = %#v app=%q", test.webRuntime, plan.WebServer, plan.AppImage)
			}
			if compose := ComposeTemplateFor(plan); !strings.Contains(compose, test.image) || !strings.Contains(compose, test.command) {
				t.Fatalf("%s Compose = %s", test.webRuntime, compose)
			}
		})
	}
}

func TestPlanRejectsContradictoryOrUnregisteredWebRuntime(t *testing.T) {
	base := config.BuildSpec{
		Application: config.Application{Version: "2.4.9", WebRuntime: "php-apache"},
		Build:       config.Build{PHP: "8.5", Composer: config.Composer{Version: "2.10"}},
		Local:       config.LocalRuntime{WebServer: config.LocalService{Family: "nginx"}},
	}
	if _, err := Plan(base); err == nil || !strings.Contains(err.Error(), "contradicts application.webRuntime") {
		t.Fatalf("contradictory web server error = %v", err)
	}
	base.Application.WebRuntime = "frankenphp-worker"
	base.Local.WebServer = config.LocalService{}
	if _, err := Plan(base); err == nil || !strings.Contains(err.Error(), `plugin "frankenphp-worker" is not registered`) {
		t.Fatalf("unregistered web runtime error = %v", err)
	}
}

func TestPlanRequiresAnExplicitCompatibilityExceptionForRedis(t *testing.T) {
	base := config.BuildSpec{
		Application: config.Application{Version: "2.4.9"},
		Build:       config.Build{PHP: "8.5", Composer: config.Composer{Version: "2.10"}},
		Local:       config.LocalRuntime{Cache: config.LocalService{Family: "redis", Version: "7.2"}},
	}
	if _, err := Plan(base); err == nil || !strings.Contains(err.Error(), "compatibility.allowUnsupported") {
		t.Fatalf("Redis without exception error = %v", err)
	}
	base.Compatibility.Status = config.CompatibilityUnsupportedAllowed
	plan, err := Plan(base)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Cache.Family != "redis" || plan.Cache.Version != "7.2" || !strings.Contains(plan.Cache.Healthcheck, "redis-cli") {
		t.Fatalf("Redis plan = %#v", plan.Cache)
	}
	if len(plan.Warnings) == 0 || !strings.Contains(plan.Warnings[0], "Redis 7.2") {
		t.Fatalf("Redis warnings = %#v", plan.Warnings)
	}
}

func TestPlanNormalizesProviderEmailTransport(t *testing.T) {
	plan, err := Plan(config.BuildSpec{
		Application: config.Application{Version: "2.4.9"},
		Build:       config.Build{PHP: "8.5", Composer: config.Composer{Version: "2.10"}},
		Local:       config.LocalRuntime{Email: config.LocalEmailSettings{Mode: "ses", Host: "email-smtp.eu-west-3.amazonaws.com", Port: 587, Username: "ses-smtp-user", CredentialEnv: "SES_SMTP_PASSWORD", From: "shop@example.test"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Email.Host != "email-smtp.eu-west-3.amazonaws.com" || plan.Email.Port != 587 || plan.Email.Username != "ses-smtp-user" {
		t.Fatalf("email = %#v", plan.Email)
	}
	compose := ComposeTemplateFor(plan)
	for _, value := range []string{
		`MAGELIFT_LOCAL_EMAIL_HOST: "email-smtp.eu-west-3.amazonaws.com"`,
		`MAGELIFT_LOCAL_EMAIL_USERNAME: "ses-smtp-user"`,
		`MAGELIFT_LOCAL_EMAIL_CREDENTIAL_ENV: "SES_SMTP_PASSWORD"`,
		`MAGELIFT_LOCAL_EMAIL_DISABLE: "0"`,
		`MAGELIFT_LOCAL_EMAIL_AUTH: "LOGIN"`,
		`MAGELIFT_LOCAL_EMAIL_SSL: "tls"`,
	} {
		if !strings.Contains(compose, value) {
			t.Fatalf("generated Compose lacks %q", value)
		}
	}
}

func TestPlanMailpitWiresSMTPAndComposeService(t *testing.T) {
	plan, err := Plan(config.BuildSpec{
		Application: config.Application{Version: "2.4.9"},
		Build:       config.Build{PHP: "8.5", Composer: config.Composer{Version: "2.10"}},
		Local:       config.LocalRuntime{Email: config.LocalEmailSettings{Mode: "mailpit"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Email.Mode != "mailpit" || plan.Email.Host != "mailpit" || plan.Email.Port != 1025 {
		t.Fatalf("email = %#v", plan.Email)
	}
	if len(plan.Warnings) == 0 || !strings.Contains(plan.Warnings[0], "not cloud delivery") {
		t.Fatalf("warnings = %#v", plan.Warnings)
	}
	compose := ComposeTemplateFor(plan)
	for _, value := range []string{
		"axllent/mailpit:v1.30.7@sha256:",
		"/mailpit",
		"readyz",
		`MAGELIFT_LOCAL_EMAIL_HOST: "mailpit"`,
		`MAGELIFT_LOCAL_EMAIL_PORT: "1025"`,
		`MAGELIFT_LOCAL_EMAIL_DISABLE: "0"`,
		`MAGELIFT_LOCAL_EMAIL_AUTH: "NONE"`,
	} {
		if !strings.Contains(compose, value) {
			t.Fatalf("generated Compose lacks %q", value)
		}
	}
	if strings.Contains(compose, "__MAGELIFT_") {
		t.Fatal("generated Compose contains an unresolved template marker")
	}
	var document map[string]any
	if err := yaml.Unmarshal([]byte(compose), &document); err != nil {
		t.Fatalf("compose yaml: %v", err)
	}
}

func TestLocalRowsMatchTheSharedCompatibilityCatalog(t *testing.T) {
	for release, row := range localReleaseCatalog {
		t.Run(release, func(t *testing.T) {
			if len(row.PHPBranches) == 0 {
				t.Fatal("local row has no PHP branches")
			}
			build := config.BuildSpec{
				Application: config.Application{Version: row.Release},
				Build:       config.Build{PHP: row.PHPBranches[0], Composer: config.Composer{Version: row.Composer}},
			}
			if err := validateSharedCompatibility(build, row); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPlanRejectsUnverifiedLocalServiceChoice(t *testing.T) {
	_, err := Plan(config.BuildSpec{
		Application: config.Application{Version: "2.4.9"},
		Build:       config.Build{PHP: "8.5", Composer: config.Composer{Version: "2.10"}},
		Local:       config.LocalRuntime{Queue: config.LocalService{Family: "artemis", Version: "3"}},
	})
	if err == nil || !strings.Contains(err.Error(), "nearest supported choices") {
		t.Fatalf("error = %v", err)
	}
}

func TestPlanRejectsUnverifiedLocalExtension(t *testing.T) {
	_, err := Plan(config.BuildSpec{
		Application: config.Application{Version: "2.4.9"},
		Build:       config.Build{PHP: "8.5", Composer: config.Composer{Version: "2.10"}, Extensions: []string{"xdebug"}},
	})
	if err == nil || !strings.Contains(err.Error(), "no verified app-image contract") {
		t.Fatalf("error = %v", err)
	}
}

func TestComposeTemplateForWritesMagentoRuntimeOverlays(t *testing.T) {
	plan, err := Plan(config.BuildSpec{
		Application: config.Application{
			Version: "2.4.9",
			Magento: config.MagentoRuntime{FrontName: "backend", CookieDomain: ".shop.test"},
		},
		Build: config.Build{PHP: "8.5", Composer: config.Composer{Version: "2.10"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	compose := ComposeTemplateFor(plan)
	if !strings.Contains(compose, `MAGENTO_DC_BACKEND__FRONTNAME: "backend"`) {
		t.Fatalf("compose missing frontName overlay: %s", compose)
	}
	if !strings.Contains(compose, `CONFIG__DEFAULT__WEB__COOKIE__COOKIE_DOMAIN: ".shop.test"`) {
		t.Fatalf("compose missing cookie overlay: %s", compose)
	}
}

func TestComposeTemplateForCarriesRuntimeInputs(t *testing.T) {
	plan, err := Plan(config.BuildSpec{
		Application: config.Application{Version: "2.4.9"},
		Build:       config.Build{PHP: "8.5", Composer: config.Composer{Version: "2.10"}, Extensions: []string{"intl", "redis"}},
		Local: config.LocalRuntime{
			PHPSettings: map[string]string{"memory_limit": "1G", "max_execution_time": "180"},
			Email:       config.LocalEmailSettings{Mode: "smtp", Host: "mail.example.test", Port: 2525, From: "shop@example.test", CredentialEnv: "SMTP_PASSWORD"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	compose := ComposeTemplateFor(plan)
	for _, value := range []string{
		`MAGELIFT_LOCAL_EXTENSIONS: "intl,redis"`,
		`MAGELIFT_LOCAL_PHP_SETTINGS: "max_execution_time=180;memory_limit=1G"`,
		`MAGELIFT_LOCAL_EMAIL_MODE: "smtp"`,
		`MAGELIFT_LOCAL_EMAIL_PORT: "2525"`,
		`MAGELIFT_LOCAL_EMAIL_CREDENTIAL_ENV: "SMTP_PASSWORD"`,
		`MAGENTO_DC_DB__CONNECTION__DEFAULT__HOST: "database"`,
		`MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_SERVER_HOSTNAME: "search"`,
		`CONFIG__DEFAULT__CATALOG__SEARCH__OPENSEARCH_SERVER_HOSTNAME: "search"`,
		`CONFIG__DEFAULT__CATALOG__SEARCH__ENGINE: "opensearch"`,
		`MAGENTO_DC_QUEUE__AMQP__PASSWORD: "magento"`,
		`MAGENTO_DC__OVERRIDE: "{`,
	} {
		if !strings.Contains(compose, value) {
			t.Fatalf("Compose template lacks %q", value)
		}
	}
	if strings.Contains(compose, "MAGENTO_DB_HOST") || strings.Contains(compose, "MAGENTO_SEARCH_HOST") {
		t.Fatal("Compose template still contains the legacy Magento environment names")
	}
	var document map[string]any
	if err := yaml.Unmarshal([]byte(compose), &document); err != nil {
		t.Fatalf("generated Compose is not valid YAML: %v", err)
	}
	var override map[string]any
	if err := json.Unmarshal([]byte(localDeploymentConfigOverride(plan)), &override); err != nil {
		t.Fatalf("generated Magento override is not valid JSON: %v", err)
	}
	if got := override["db"].(map[string]any)["connection"].(map[string]any)["default"].(map[string]any)["host"]; got != "database" {
		t.Fatalf("database override host = %#v", got)
	}
	elasticsuite := override["system"].(map[string]any)["default"].(map[string]any)["smile_elasticsuite_core_base_settings"].(map[string]any)["es_client"].(map[string]any)
	if elasticsuite["servers"] != "search:9200" || elasticsuite["enable_https_mode"] != "0" || elasticsuite["enable_http_auth"] != "0" {
		t.Fatalf("Elasticsuite override = %#v", elasticsuite)
	}
	smtp := override["system"].(map[string]any)["default"].(map[string]any)["smtp"].(map[string]any)
	if smtp["host"] != "#env(MAGELIFT_LOCAL_EMAIL_HOST)" || smtp["password"] != "#env(MAGELIFT_LOCAL_EMAIL_PASSWORD)" || smtp["auth"] != "#env(MAGELIFT_LOCAL_EMAIL_AUTH, \"NONE\")" {
		t.Fatalf("SMTP override = %#v", smtp)
	}
	transEmail := override["system"].(map[string]any)["default"].(map[string]any)["trans_email"].(map[string]any)["ident_general"].(map[string]any)
	if transEmail["email"] != "#env(MAGELIFT_LOCAL_EMAIL_FROM)" {
		t.Fatalf("sender override = %#v", transEmail)
	}
}

func TestPHPIniForSortsAndRejectsUnsafeValues(t *testing.T) {
	plan := RuntimePlan{PHPSettings: map[string]string{
		"memory_limit":       "1G",
		"max_execution_time": "180",
	}}
	got, err := PHPIniFor(plan)
	if err != nil {
		t.Fatal(err)
	}
	if got != "max_execution_time = 180\nmemory_limit = 1G\n" {
		t.Fatalf("php ini = %q", got)
	}
	for _, settings := range []map[string]string{
		{"memory_limit": "${SECRET}"},
		{"memory_limit": "1G#comment"},
		{"Memory_Limit": "1G"},
	} {
		if _, err := PHPIniFor(RuntimePlan{PHPSettings: settings}); err == nil {
			t.Fatalf("expected unsafe PHP settings to fail: %#v", settings)
		}
	}
}

func TestPlanForRecordsAuroraMySQLSubstituteAndNginx(t *testing.T) {
	plan, err := PlanFor(config.BuildSpec{
		Application: config.Application{Version: "2.4.9", WebRuntime: "nginx-fpm"},
		Build:       config.Build{PHP: "8.5", Composer: config.Composer{Version: "2.10"}},
	}, CloudHints{
		Environment:     "staging",
		Provider:        "aws",
		Preset:          "standard",
		DatabaseEngine:  "aurora-mysql",
		DatabaseVersion: "8.4",
		SearchMode:      "serverless",
		QueueMode:       "amazon-mq",
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Database.Family != "mysql" || plan.Database.Version != "8.4" {
		t.Fatalf("database = %#v", plan.Database)
	}
	if plan.WebServer.Family != "nginx" {
		t.Fatalf("web server = %#v", plan.WebServer)
	}
	got := map[string]string{}
	for _, substitute := range plan.Substitutes {
		got[substitute.Cloud] = substitute.Local
	}
	if got["Aurora MySQL"] != "mysql 8.4" || got["OpenSearch Serverless"] != "opensearch 3" || got["Amazon MQ"] != "rabbitmq 4.2" {
		t.Fatalf("substitutes = %#v", plan.Substitutes)
	}
	if len(plan.Warnings) == 0 || !strings.Contains(strings.Join(plan.Warnings, "\n"), CloudOnlyNote) {
		t.Fatalf("warnings = %#v", plan.Warnings)
	}
	compose := ComposeTemplateFor(plan)
	if strings.Contains(compose, "cloudfront") || strings.Contains(compose, "CloudFront") {
		t.Fatal("local Compose emulates CloudFront")
	}
}

func TestPlanForSplitsCacheAndSessionOnDurablePresets(t *testing.T) {
	plan, err := PlanFor(config.BuildSpec{
		Application: config.Application{Version: "2.4.9"},
		Build:       config.Build{PHP: "8.5", Composer: config.Composer{Version: "2.10"}},
	}, CloudHints{Provider: "aws", Preset: "standard", IsolateCacheAndSession: true})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Session.Family == "" || plan.Session.Connection.Host != "session" {
		t.Fatalf("session = %#v", plan.Session)
	}
	compose := ComposeTemplateFor(plan)
	for _, value := range []string{
		"  session:",
		`MAGENTO_DC_SESSION__SAVE: "redis"`,
		`MAGENTO_DC_SESSION__REDIS_HOST: "session"`,
		"condition: service_healthy",
	} {
		if !strings.Contains(compose, value) {
			t.Fatalf("compose lacks %q", value)
		}
	}
	if strings.Contains(compose, "__MAGELIFT_") {
		t.Fatal("generated Compose contains an unresolved template marker")
	}
	var document map[string]any
	if err := yaml.Unmarshal([]byte(compose), &document); err != nil {
		t.Fatalf("compose yaml: %v", err)
	}
}

func TestPlanForRecordsMemorystoreSubstitute(t *testing.T) {
	plan, err := PlanFor(config.BuildSpec{
		Application: config.Application{Version: "2.4.9"},
		Build:       config.Build{PHP: "8.5", Composer: config.Composer{Version: "2.10"}},
	}, CloudHints{Provider: "gcp", CacheProduct: "memorystore", DatabaseEngine: "cloudsql-mysql"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, substitute := range plan.Substitutes {
		if substitute.Cloud == "Memorystore" && strings.HasPrefix(substitute.Local, "valkey ") {
			found = true
		}
	}
	if !found {
		t.Fatalf("substitutes = %#v", plan.Substitutes)
	}
}

func TestHintsFromConfigDefaultsStandardQueueToECSRabbitMQ(t *testing.T) {
	hints := HintsFromConfig(config.Config{
		Defaults: config.Defaults{Preset: "standard"},
		Target:   config.Target{Provider: "aws"},
	})
	if hints.QueueMode != "ecs-rabbitmq" {
		t.Fatalf("standard AWS queue hint = %q, want ecs-rabbitmq", hints.QueueMode)
	}
}

func TestPlanForRecordsECSRabbitMQSubstitute(t *testing.T) {
	plan, err := PlanFor(config.BuildSpec{
		Application: config.Application{Version: "2.4.9", WebRuntime: "nginx-fpm"},
		Build:       config.Build{PHP: "8.5", Composer: config.Composer{Version: "2.10"}},
	}, CloudHints{
		Provider:  "aws",
		Preset:    "standard",
		QueueMode: "ecs-rabbitmq",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, substitute := range plan.Substitutes {
		got[substitute.Cloud] = substitute.Local
	}
	if got["ECS RabbitMQ"] != "rabbitmq 4.2" {
		t.Fatalf("substitutes = %#v", plan.Substitutes)
	}
}
