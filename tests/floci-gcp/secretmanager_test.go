//go:build floci_gcp

package flocigcp_test

import (
	"context"
	"testing"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
)

func TestSecretWriteAndAccessAgainstFlociGCP(t *testing.T) {
	host := requireFlociGCP(t)
	t.Setenv("SECRET_MANAGER_EMULATOR_HOST", host)
	t.Setenv("GOOGLE_CLOUD_PROJECT", "floci-local")

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	client, err := secretmanager.NewClient(ctx, flociGCPClientOptions(host)...)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	const project = "floci-local"
	const secretID = "magelift-floci-gcp-secret"
	parent := "projects/" + project
	created, err := client.CreateSecret(ctx, &secretmanagerpb.CreateSecretRequest{
		Parent:   parent,
		SecretId: secretID,
		Secret: &secretmanagerpb.Secret{
			Replication: &secretmanagerpb.Replication{
				Replication: &secretmanagerpb.Replication_Automatic_{
					Automatic: &secretmanagerpb.Replication_Automatic{},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("create secret: %v", err)
	}
	defer func() {
		_ = client.DeleteSecret(context.Background(), &secretmanagerpb.DeleteSecretRequest{Name: created.Name})
	}()

	if _, err := client.AddSecretVersion(ctx, &secretmanagerpb.AddSecretVersionRequest{
		Parent: created.Name,
		Payload: &secretmanagerpb.SecretPayload{
			Data: []byte("composer-auth-v1"),
		},
	}); err != nil {
		t.Fatalf("add secret version: %v", err)
	}

	got, err := client.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
		Name: created.Name + "/versions/latest",
	})
	if err != nil {
		t.Fatalf("access secret version: %v", err)
	}
	if string(got.GetPayload().GetData()) != "composer-auth-v1" {
		t.Fatalf("got %q, want composer-auth-v1", got.GetPayload().GetData())
	}
}
