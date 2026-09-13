package kube

import (
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/platform"
)

func TestMigrationJobBindsRabbitMQCredentials(t *testing.T) {
	job := migrationJob("magelift-migrate-test", CandidateRequest{
		Cluster:                 "shop-gke",
		ImageDigest:             "ghcr.io/magelift/magento@sha256:" + strings.Repeat("a", 64),
		DatabaseWriter:          "db.internal",
		DatabaseName:            "magento",
		DatabaseSecretName:      "shop-db-credentials",
		EncryptionKeySecretName: "shop-encryption-key",
		QueuePasswordSecretName: "shop-queue-password",
		CacheEndpoint:           "cache.internal",
		SearchEndpoint:          "search.internal",
		QueueMode:               "rabbitmq",
		QueueHost:               "rabbitmq",
		QueueUsername:           "magento",
	})

	sawPassword := false
	sawUsername := false
	for _, env := range job.Spec.Template.Spec.Containers[0].Env {
		if env.Name == platform.EnvMagentoQueueUsername {
			if env.Value != "magento" || env.ValueFrom != nil {
				t.Fatalf("queue username binding = %#v", env)
			}
			sawUsername = true
			continue
		}
		if env.Name != platform.EnvMagentoQueuePassword {
			continue
		}
		if env.Value != "" || env.ValueFrom == nil || env.ValueFrom.SecretKeyRef == nil {
			t.Fatalf("queue password env contains a literal or no SecretKeyRef: %#v", env)
		}
		ref := env.ValueFrom.SecretKeyRef
		if ref.Name != "shop-queue-password" || ref.Key != "password" {
			t.Fatalf("queue password SecretKeyRef = %#v", ref)
		}
		sawPassword = true
	}
	if !sawUsername {
		t.Fatalf("migration Job is missing %s", platform.EnvMagentoQueueUsername)
	}
	if !sawPassword {
		t.Fatalf("migration Job is missing %s", platform.EnvMagentoQueuePassword)
	}
	for _, env := range job.Spec.Template.Spec.Containers[0].Env {
		if env.Name == platform.EnvMagentoSearchHost && env.Value == "search.internal" {
			return
		}
	}
	t.Fatalf("migration Job is missing %s", platform.EnvMagentoSearchHost)
}

func TestValidateCandidateRequestRequiresRabbitMQSecret(t *testing.T) {
	err := validateCandidateRequest(CandidateRequest{
		Cluster:                 "shop-gke",
		ImageDigest:             "ghcr.io/magelift/magento@sha256:" + strings.Repeat("a", 64),
		DatabaseWriter:          "db.internal",
		DatabaseName:            "magento",
		CacheEndpoint:           "cache.internal",
		QueueMode:               "rabbitmq",
		QueueHost:               "rabbitmq",
		EncryptionKeySecretName: "shop-encryption-key",
	})
	if err == nil || !strings.Contains(err.Error(), "queue password Secret") {
		t.Fatalf("expected queue password Secret validation error, got %v", err)
	}
}
