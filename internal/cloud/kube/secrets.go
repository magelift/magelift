package kube

import (
	"errors"
	"fmt"
	"strings"

	"github.com/magelift/magelift/internal/platform"
	pulumicorev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	pulumimetav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	clientcorev1 "k8s.io/api/core/v1"
)

const (
	databaseCredentialsSecretSuffix      = "-db-credentials"
	databaseAdminCredentialsSecretSuffix = "-db-admin-credentials"
	encryptionKeySecretSuffix            = "-encryption-key"
	queuePasswordSecretSuffix            = "-queue-password"
	smtpPasswordSecretSuffix             = "-smtp-password"
	mediaHmacSecretSuffix                = "-media-hmac"
)

// DatabaseCredentialsSecretName returns the stable Kubernetes Secret name used
// by Magento workloads and migration Jobs in a runtime namespace.
func DatabaseCredentialsSecretName(runtimeName string) string {
	return runtimeName + databaseCredentialsSecretSuffix
}

// DatabaseAdminCredentialsSecretName returns the stable Kubernetes Secret
// name used by the short-lived database privilege grant Job.
func DatabaseAdminCredentialsSecretName(runtimeName string) string {
	return runtimeName + databaseAdminCredentialsSecretSuffix
}

// EncryptionKeySecretName returns the stable Kubernetes Secret name used by
// Magento workloads and migration Jobs for the encryption key.
func EncryptionKeySecretName(runtimeName string) string {
	return runtimeName + encryptionKeySecretSuffix
}

// QueuePasswordSecretName returns the stable Kubernetes Secret name used by
// RabbitMQ workloads and migration Jobs in a runtime namespace.
func QueuePasswordSecretName(runtimeName string) string {
	return runtimeName + queuePasswordSecretSuffix
}

// SmtpPasswordSecretName returns the stable Kubernetes Secret name used by
// Magento SMTP relay workloads in a runtime namespace.
func SmtpPasswordSecretName(runtimeName string) string {
	return runtimeName + smtpPasswordSecretSuffix
}

// MediaHmacSecretName returns the stable Kubernetes Secret name holding the
// media storage HMAC secret for Magento remote-storage workloads.
func MediaHmacSecretName(runtimeName string) string {
	return runtimeName + mediaHmacSecretSuffix
}

// NewDatabaseCredentialsSecret keeps the database password in Kubernetes
// Secret data. Workloads consume it through SecretKeyRef so it never becomes a
// literal environment value in a rendered Pod or Job manifest.
func NewDatabaseCredentialsSecret(
	ctx *pulumi.Context,
	runtimeName string,
	username string,
	password pulumi.StringInput,
	opts ...pulumi.ResourceOption,
) (*pulumicorev1.Secret, error) {
	if ctx == nil {
		return nil, errors.New("Pulumi context is required")
	}
	if strings.TrimSpace(runtimeName) == "" {
		return nil, errors.New("runtime name is required")
	}
	if strings.TrimSpace(username) == "" {
		return nil, errors.New("database username is required")
	}
	if password == nil {
		return nil, errors.New("database password is required")
	}
	return newCredentialsSecret(ctx, runtimeName+"-db-credentials", DatabaseCredentialsSecretName(runtimeName), username, password, opts...)
}

// NewDatabaseAdminCredentialsSecret keeps the database admin credential in a
// separate Kubernetes Secret consumed only by the privilege grant Job.
func NewDatabaseAdminCredentialsSecret(
	ctx *pulumi.Context,
	runtimeName string,
	username string,
	password pulumi.StringInput,
	opts ...pulumi.ResourceOption,
) (*pulumicorev1.Secret, error) {
	if ctx == nil {
		return nil, errors.New("Pulumi context is required")
	}
	if strings.TrimSpace(runtimeName) == "" {
		return nil, errors.New("runtime name is required")
	}
	if strings.TrimSpace(username) == "" {
		return nil, errors.New("database admin username is required")
	}
	if password == nil {
		return nil, errors.New("database admin password is required")
	}
	return newCredentialsSecret(ctx, runtimeName+"-db-admin-credentials", DatabaseAdminCredentialsSecretName(runtimeName), username, password, opts...)
}

// NewMagentoEncryptionKeySecret stores the provider-resolved Magento
// encryption key in a namespace-local Secret. The value remains a Pulumi
// secret and is exposed to containers only through a SecretKeyRef.
func NewMagentoEncryptionKeySecret(
	ctx *pulumi.Context,
	resourceName string,
	key pulumi.StringInput,
	opts ...pulumi.ResourceOption,
) (*pulumicorev1.Secret, error) {
	return NewNamedMagentoEncryptionKeySecret(ctx, resourceName, EncryptionKeySecretName(resourceName), key, opts...)
}

// NewNamedMagentoEncryptionKeySecret is the provider-neutral primitive for a
// namespace-local Magento encryption key Secret. Providers that do not yet
// have a Secrets API can use it with a generated Pulumi secret while retaining
// the configured Kubernetes Secret name.
func NewNamedMagentoEncryptionKeySecret(
	ctx *pulumi.Context,
	resourceName string,
	secretName string,
	key pulumi.StringInput,
	opts ...pulumi.ResourceOption,
) (*pulumicorev1.Secret, error) {
	if ctx == nil {
		return nil, errors.New("Pulumi context is required")
	}
	if strings.TrimSpace(resourceName) == "" {
		return nil, errors.New("resource name is required")
	}
	if strings.TrimSpace(secretName) == "" {
		return nil, errors.New("Magento encryption key Secret name is required")
	}
	if key == nil {
		return nil, fmt.Errorf("Magento encryption key is required")
	}
	return pulumicorev1.NewSecret(ctx, resourceName+"-encryption-key", &pulumicorev1.SecretArgs{
		Metadata: &pulumimetav1.ObjectMetaArgs{
			Name:        pulumi.String(secretName),
			Annotations: SkipAwaitAnnotations(),
		},
		StringData: pulumi.StringMap{"key": key},
		Type:       pulumi.String("Opaque"),
	}, opts...)
}

// NewQueuePasswordSecret keeps the RabbitMQ password in Kubernetes Secret
// data. Workloads consume it through SecretKeyRef so it never becomes a
// literal environment value in a rendered Pod or Job manifest.
func NewQueuePasswordSecret(
	ctx *pulumi.Context,
	runtimeName string,
	password pulumi.StringInput,
	opts ...pulumi.ResourceOption,
) (*pulumicorev1.Secret, error) {
	if ctx == nil {
		return nil, errors.New("Pulumi context is required")
	}
	if strings.TrimSpace(runtimeName) == "" {
		return nil, errors.New("runtime name is required")
	}
	if password == nil {
		return nil, errors.New("queue password is required")
	}
	return pulumicorev1.NewSecret(ctx, runtimeName+"-queue-password", &pulumicorev1.SecretArgs{
		Metadata: &pulumimetav1.ObjectMetaArgs{
			Name:        pulumi.String(QueuePasswordSecretName(runtimeName)),
			Annotations: SkipAwaitAnnotations(),
		},
		StringData: pulumi.StringMap{"password": password},
		Type:       pulumi.String("Opaque"),
	}, opts...)
}

func newCredentialsSecret(
	ctx *pulumi.Context,
	resourceName string,
	secretName string,
	username string,
	password pulumi.StringInput,
	opts ...pulumi.ResourceOption,
) (*pulumicorev1.Secret, error) {
	return pulumicorev1.NewSecret(ctx, resourceName, &pulumicorev1.SecretArgs{
		Metadata: &pulumimetav1.ObjectMetaArgs{
			Name:        pulumi.String(secretName),
			Annotations: SkipAwaitAnnotations(),
		},
		StringData: pulumi.StringMap{
			"username": pulumi.String(username),
			"password": password,
		},
		Type: pulumi.String("Opaque"),
	}, opts...)
}

// AppendDatabaseCredentialEnv adds SecretKeyRef bindings for Magento's
// database username and password variables to an environment output.
func AppendDatabaseCredentialEnv(env pulumicorev1.EnvVarArrayOutput, secretName string) pulumicorev1.EnvVarArrayOutput {
	if strings.TrimSpace(secretName) == "" {
		return env
	}
	return env.ApplyT(func(values []pulumicorev1.EnvVar) []pulumicorev1.EnvVar {
		result := append([]pulumicorev1.EnvVar(nil), values...)
		for _, field := range []struct {
			name string
			key  string
		}{
			{platform.EnvMagentoDBUser, "username"},
			{platform.EnvMagentoDBPass, "password"},
		} {
			secret := secretName
			result = append(result, pulumicorev1.EnvVar{
				Name: field.name,
				ValueFrom: &pulumicorev1.EnvVarSource{
					SecretKeyRef: &pulumicorev1.SecretKeySelector{Name: &secret, Key: field.key},
				},
			})
		}
		return result
	}).(pulumicorev1.EnvVarArrayOutput)
}

// AppendEncryptionKeyEnv adds the Magento encryption-key SecretKeyRef to a
// runtime environment output.
func AppendEncryptionKeyEnv(env pulumicorev1.EnvVarArrayOutput, secretName string) pulumicorev1.EnvVarArrayOutput {
	if strings.TrimSpace(secretName) == "" {
		return env
	}
	return env.ApplyT(func(values []pulumicorev1.EnvVar) []pulumicorev1.EnvVar {
		result := append([]pulumicorev1.EnvVar(nil), values...)
		secret := secretName
		result = append(result, pulumicorev1.EnvVar{
			Name: platform.EnvMagentoCryptKey,
			ValueFrom: &pulumicorev1.EnvVarSource{
				SecretKeyRef: &pulumicorev1.SecretKeySelector{Name: &secret, Key: "key"},
			},
		})
		return result
	}).(pulumicorev1.EnvVarArrayOutput)
}

// NewSmtpPasswordSecret keeps the SMTP relay password in Kubernetes.
func NewSmtpPasswordSecret(
	ctx *pulumi.Context,
	runtimeName string,
	password pulumi.StringInput,
	opts ...pulumi.ResourceOption,
) (*pulumicorev1.Secret, error) {
	if ctx == nil {
		return nil, errors.New("Pulumi context is required")
	}
	if strings.TrimSpace(runtimeName) == "" {
		return nil, errors.New("runtime name is required")
	}
	if password == nil {
		return nil, errors.New("SMTP password is required")
	}
	return pulumicorev1.NewSecret(ctx, runtimeName+"-smtp-password", &pulumicorev1.SecretArgs{
		Metadata: &pulumimetav1.ObjectMetaArgs{
			Name:        pulumi.String(SmtpPasswordSecretName(runtimeName)),
			Annotations: SkipAwaitAnnotations(),
		},
		StringData: pulumi.StringMap{"password": password},
		Type:       pulumi.String("Opaque"),
	}, opts...)
}

// AppendQueuePasswordEnv adds the RabbitMQ password SecretKeyRef to a runtime
// environment output.
func AppendQueuePasswordEnv(env pulumicorev1.EnvVarArrayOutput, secretName string) pulumicorev1.EnvVarArrayOutput {
	if strings.TrimSpace(secretName) == "" {
		return env
	}
	return env.ApplyT(func(values []pulumicorev1.EnvVar) []pulumicorev1.EnvVar {
		result := append([]pulumicorev1.EnvVar(nil), values...)
		secret := secretName
		result = append(result, pulumicorev1.EnvVar{
			Name: platform.EnvMagentoQueuePassword,
			ValueFrom: &pulumicorev1.EnvVarSource{
				SecretKeyRef: &pulumicorev1.SecretKeySelector{Name: &secret, Key: "password"},
			},
		})
		return result
	}).(pulumicorev1.EnvVarArrayOutput)
}

// DatabaseCredentialEnvVars returns the client-go equivalent of the runtime
// SecretKeyRef bindings for short-lived migration Jobs.
func DatabaseCredentialEnvVars(secretName string) []clientcorev1.EnvVar {
	if strings.TrimSpace(secretName) == "" {
		return nil
	}
	result := make([]clientcorev1.EnvVar, 0, 2)
	for _, field := range []struct {
		name string
		key  string
	}{
		{platform.EnvMagentoDBUser, "username"},
		{platform.EnvMagentoDBPass, "password"},
	} {
		secret := secretName
		result = append(result, clientcorev1.EnvVar{
			Name: field.name,
			ValueFrom: &clientcorev1.EnvVarSource{
				SecretKeyRef: &clientcorev1.SecretKeySelector{
					LocalObjectReference: clientcorev1.LocalObjectReference{Name: secret},
					Key:                  field.key,
				},
			},
		})
	}
	return result
}

// EncryptionKeyEnvVars returns the client-go equivalent of the runtime
// SecretKeyRef binding for candidate migration Jobs.
func EncryptionKeyEnvVars(secretName string) []clientcorev1.EnvVar {
	if strings.TrimSpace(secretName) == "" {
		return nil
	}
	secret := secretName
	return []clientcorev1.EnvVar{{
		Name: platform.EnvMagentoCryptKey,
		ValueFrom: &clientcorev1.EnvVarSource{
			SecretKeyRef: &clientcorev1.SecretKeySelector{
				LocalObjectReference: clientcorev1.LocalObjectReference{Name: secret},
				Key:                  "key",
			},
		},
	}}
}

// QueuePasswordEnvVars returns the client-go equivalent of the runtime
// SecretKeyRef binding for short-lived migration Jobs.
func QueuePasswordEnvVars(secretName string) []clientcorev1.EnvVar {
	if strings.TrimSpace(secretName) == "" {
		return nil
	}
	secret := secretName
	return []clientcorev1.EnvVar{{
		Name: platform.EnvMagentoQueuePassword,
		ValueFrom: &clientcorev1.EnvVarSource{
			SecretKeyRef: &clientcorev1.SecretKeySelector{
				LocalObjectReference: clientcorev1.LocalObjectReference{Name: secret},
				Key:                  "password",
			},
		},
	}}
}

// AppendSmtpPasswordEnv adds the SMTP relay password SecretKeyRef to a
// runtime environment output.
func AppendSmtpPasswordEnv(env pulumicorev1.EnvVarArrayOutput, secretName string) pulumicorev1.EnvVarArrayOutput {
	if strings.TrimSpace(secretName) == "" {
		return env
	}
	return env.ApplyT(func(values []pulumicorev1.EnvVar) []pulumicorev1.EnvVar {
		result := append([]pulumicorev1.EnvVar(nil), values...)
		secret := secretName
		result = append(result, pulumicorev1.EnvVar{
			Name: "CONFIG__DEFAULT__SYSTEM__SMTP__PASSWORD",
			ValueFrom: &pulumicorev1.EnvVarSource{
				SecretKeyRef: &pulumicorev1.SecretKeySelector{Name: &secret, Key: "password"},
			},
		})
		return result
	}).(pulumicorev1.EnvVarArrayOutput)
}

// NewMediaHmacSecret keeps the media storage HMAC secret in Kubernetes.
func NewMediaHmacSecret(
	ctx *pulumi.Context,
	runtimeName string,
	secret pulumi.StringInput,
	opts ...pulumi.ResourceOption,
) (*pulumicorev1.Secret, error) {
	if ctx == nil {
		return nil, errors.New("Pulumi context is required")
	}
	if strings.TrimSpace(runtimeName) == "" {
		return nil, errors.New("runtime name is required")
	}
	if secret == nil {
		return nil, errors.New("media HMAC secret is required")
	}
	return pulumicorev1.NewSecret(ctx, runtimeName+"-media-hmac", &pulumicorev1.SecretArgs{
		Metadata: &pulumimetav1.ObjectMetaArgs{
			Name:        pulumi.String(MediaHmacSecretName(runtimeName)),
			Annotations: SkipAwaitAnnotations(),
		},
		StringData: pulumi.StringMap{"secret": secret},
		Type:       pulumi.String("Opaque"),
	}, opts...)
}

// AppendMediaHmacSecretEnv adds the media HMAC secret SecretKeyRef to a
// runtime environment output for the PHP lifecycle remote-storage writer.
func AppendMediaHmacSecretEnv(env pulumicorev1.EnvVarArrayOutput, secretName string) pulumicorev1.EnvVarArrayOutput {
	if strings.TrimSpace(secretName) == "" {
		return env
	}
	return env.ApplyT(func(values []pulumicorev1.EnvVar) []pulumicorev1.EnvVar {
		result := append([]pulumicorev1.EnvVar(nil), values...)
		secret := secretName
		result = append(result, pulumicorev1.EnvVar{
			Name: "MAGELIFT_MEDIA_S3_SECRET",
			ValueFrom: &pulumicorev1.EnvVarSource{
				SecretKeyRef: &pulumicorev1.SecretKeySelector{Name: &secret, Key: "secret"},
			},
		})
		return result
	}).(pulumicorev1.EnvVarArrayOutput)
}
