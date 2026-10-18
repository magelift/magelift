package kube

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestMigrationJobDoesNotRetry(t *testing.T) {
	job := migrationJob("magelift-migrate-test", CandidateRequest{
		Cluster:                 "shop-gke",
		ImageDigest:             "registry.example.invalid/shop@sha256:" + strings.Repeat("a", 64),
		DatabaseWriter:          "db.internal",
		DatabaseName:            "magento",
		DatabaseSecretName:      "shop-db-credentials",
		EncryptionKeySecretName: "shop-encryption-key",
		CacheEndpoint:           "cache.internal",
		SearchEndpoint:          "search.internal",
		QueueMode:               "database",
	})
	if job.Spec.BackoffLimit == nil || *job.Spec.BackoffLimit != 0 {
		t.Fatalf("migration must require an explicit retry: backoffLimit=%v", job.Spec.BackoffLimit)
	}
	if job.Spec.Template.Spec.RestartPolicy != corev1.RestartPolicyNever {
		t.Fatalf("migration container must not restart: policy=%q", job.Spec.Template.Spec.RestartPolicy)
	}
}
