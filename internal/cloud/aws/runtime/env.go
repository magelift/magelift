package runtime

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/magelift/magelift/internal/platform"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func appendDatabaseSecret(secrets []SecretReference, databaseARN string) []SecretReference {
	if strings.TrimSpace(databaseARN) == "" {
		return append([]SecretReference(nil), secrets...)
	}
	result := append([]SecretReference(nil), secrets...)
	result = append(result, SecretReference{Name: "MAGELIFT_DATABASE_CREDENTIALS", ARN: databaseARN})
	// RDS/Aurora ManageMasterUserPassword secrets only contain username and
	// password. Host, port, and dbname come from capability environment vars.
	for _, field := range []struct {
		name string
		key  string
	}{
		{platform.EnvMagentoDBUser, "username"},
		{platform.EnvMagentoDBPass, "password"},
	} {
		result = append(result, SecretReference{Name: field.name, ARN: databaseARN, JSONKey: field.key})
	}
	return result
}

func appendEncryptionSecret(secrets []SecretReference, encryptionARN string) []SecretReference {
	result := append([]SecretReference(nil), secrets...)
	for _, secret := range result {
		if secret.Name == platform.EnvMagentoCryptKey {
			return result
		}
	}
	return append(result, SecretReference{Name: platform.EnvMagentoCryptKey, ARN: encryptionARN})
}

func deploymentCommand() []string {
	return platform.MagentoMigrationShell()
}

func capabilityEnvironment(args Args) pulumi.Output {
	if args.Capabilities == nil {
		return pulumi.ToOutput(containerEnvFromBindings(platform.CoreEnvBindings(platform.CapabilityEndpoints{
			ApplicationMode:    args.ApplicationMode,
			ApplicationVersion: args.ApplicationVersion,
			WebRuntime:         args.WebRuntime,
			Magento:            args.Magento,
		})))
	}
	capabilities := args.Capabilities
	return pulumi.All(
		capabilities.DatabaseWriterEndpoint, capabilities.DatabaseSecretARN, capabilities.CacheEndpoint, capabilities.SessionEndpoint,
		capabilities.SearchEndpoint, capabilities.QueueMode, capabilities.QueueEndpoint, capabilities.QueueUsername, capabilities.MediaBucket,
	).ApplyT(func(values []interface{}) []containerEnvironment {
		queueMode := values[5].(string)
		queueEndpoint := values[6].(string)
		searchEndpoint := values[4].(string)
		queueConnection := "db"
		if queueMode == "rabbitmq" {
			queueConnection = "amqp"
		}
		queueHost, queuePort, queueSSL := amqpSettings(queueEndpoint)
		session := values[3].(string)
		if session == "" {
			session = values[2].(string)
		}
		environment := containerEnvFromBindings(platform.CoreEnvBindings(platform.CapabilityEndpoints{
			ApplicationMode:    args.ApplicationMode,
			ApplicationVersion: args.ApplicationVersion,
			WebRuntime:         args.WebRuntime,
			DatabaseWriter:     values[0].(string),
			DatabaseName:       capabilities.DatabaseName,
			CacheEndpoint:      values[2].(string),
			SessionEndpoint:    session,
			SearchEndpoint:     magentoSearchEndpoint(args, searchEndpoint),
			Magento:            args.Magento,
		}))
		// MAGELIFT_SEARCH_ENDPOINT stays the AWS endpoint. Magento may talk to
		// a local SigV4 proxy when the target is AOSS.
		environment = upsertContainerEnv(environment, platform.EnvSearchEndpoint, searchEndpoint)
		// AWS adapter-local: secret ARNs, queue AMQP, media until those ports land.
		environment = append(environment,
			containerEnvironment{Name: "MAGELIFT_DATABASE_SECRET_ARN", Value: values[1].(string)},
			containerEnvironment{Name: "MAGELIFT_QUEUE_MODE", Value: queueMode},
			containerEnvironment{Name: "MAGELIFT_QUEUE_ENDPOINT", Value: queueEndpoint},
			containerEnvironment{Name: "MAGELIFT_MEDIA_BUCKET", Value: values[8].(string)},
			containerEnvironment{Name: "MAGENTO_DC_QUEUE__DEFAULT_CONNECTION", Value: queueConnection},
			containerEnvironment{Name: "MAGENTO_DC_QUEUE__AMQP__HOST", Value: queueHost},
			containerEnvironment{Name: "MAGENTO_DC_QUEUE__AMQP__PORT", Value: queuePort},
			containerEnvironment{Name: "MAGENTO_DC_QUEUE__AMQP__SSL", Value: queueSSL},
			containerEnvironment{Name: "MAGENTO_DC_QUEUE__AMQP__USERNAME", Value: values[7].(string)},
		)
		return environment
	})
}

func containerEnvFromBindings(bindings []platform.EnvBinding) []containerEnvironment {
	environment := make([]containerEnvironment, 0, len(bindings))
	for _, binding := range bindings {
		environment = append(environment, containerEnvironment{Name: binding.Name, Value: binding.Value})
	}
	return environment
}

func magentoSearchEndpoint(args Args, awsEndpoint string) string {
	if args.SearchProxyImage == "" {
		return awsEndpoint
	}
	return "http://127.0.0.1:" + strconv.Itoa(searchProxyPort)
}

func upsertContainerEnv(environment []containerEnvironment, name, value string) []containerEnvironment {
	for index := range environment {
		if environment[index].Name == name {
			environment[index].Value = value
			return environment
		}
	}
	return append(environment, containerEnvironment{Name: name, Value: value})
}

func amqpSettings(endpoint string) (string, string, string) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Hostname() == "" {
		return "", "", ""
	}
	port := parsed.Port()
	if port == "" {
		if parsed.Scheme == "amqps" {
			port = "5671"
		} else {
			port = "5672"
		}
	}
	ssl := "0"
	if parsed.Scheme == "amqps" {
		ssl = "1"
	}
	return parsed.Hostname(), port, ssl
}
