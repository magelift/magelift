package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	artifactregistry "google.golang.org/api/artifactregistry/v1"
	"google.golang.org/api/option"
	serviceusage "google.golang.org/api/serviceusage/v1"
)

func TestServiceEnablementHTTPFixture(t *testing.T) {
	t.Parallel()
	var enableCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/v1/projects/"):
			if strings.HasSuffix(r.URL.Path, ":batchGet") {
				_, _ = fmt.Fprint(w, `{"services":[{"name":"projects/249/services/compute.googleapis.com","state":"DISABLED"}]}`)
				return
			}
			_, _ = fmt.Fprint(w, `{"projectNumber":"249"}`)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, ":enable"):
			enableCalls.Add(1)
			_, _ = fmt.Fprint(w, `{"name":"operations/op-enable","done":true}`)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	services, err := serviceusage.NewService(context.Background(), option.WithHTTPClient(server.Client()), option.WithEndpoint(server.URL+"/"))
	if err != nil {
		t.Fatal(err)
	}
	client := &sdkServiceEnablement{services: services, crm: nil}
	enabled, err := client.EnabledServices(context.Background(), 249, []string{"compute.googleapis.com"})
	if err != nil {
		t.Fatal(err)
	}
	if enabled["compute.googleapis.com"] {
		t.Fatal("expected compute disabled in fixture")
	}
	if err := client.EnableService(context.Background(), 249, "compute.googleapis.com"); err != nil {
		t.Fatal(err)
	}
	if enableCalls.Load() != 1 {
		t.Fatalf("enable calls = %d", enableCalls.Load())
	}
}

func TestArtifactRegistryCreateHTTPFixture(t *testing.T) {
	t.Parallel()
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/repositories"):
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			_, _ = fmt.Fprint(w, `{"name":"operations/create-repo","done":true}`)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/repositories/magelift-shop-preview-images"):
			_, _ = fmt.Fprint(w, `{"name":"projects/p/locations/europe-west1/repositories/magelift-shop-preview-images","format":"DOCKER","labels":{"magelift-purpose":"application-images"}}`)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := artifactregistry.NewService(context.Background(), option.WithHTTPClient(server.Client()), option.WithEndpoint(server.URL+"/"))
	if err != nil {
		t.Fatal(err)
	}
	registry := &sdkArtifactRegistry{client: client}
	meta, err := registry.CreateRepository(context.Background(), "p", "europe-west1", "magelift-shop-preview-images", map[string]string{
		"magelift-managed-by": "magelift", "magelift-project": "shop",
		"magelift-environment": "preview", "magelift-purpose": purposeApplicationImages,
	})
	if err != nil {
		t.Fatal(err)
	}
	if meta.Format != "DOCKER" {
		t.Fatalf("format = %q", meta.Format)
	}
	if body["format"] != "DOCKER" {
		t.Fatalf("create payload format = %#v", body["format"])
	}
	labels, _ := body["labels"].(map[string]any)
	if labels["magelift-purpose"] != purposeApplicationImages {
		t.Fatalf("labels = %#v", body["labels"])
	}
}

func TestServiceUsageListReturnsActionableErrorWhenAPIDisabled(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, ":batchGet") {
			w.WriteHeader(http.StatusForbidden)
			_, _ = fmt.Fprint(w, `{"error":{"code":403,"message":"Service Usage API has not been used in project","errors":[{"reason":"SERVICE_DISABLED"}]}}`)
			return
		}
		t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	services, err := serviceusage.NewService(context.Background(), option.WithHTTPClient(server.Client()), option.WithEndpoint(server.URL+"/"))
	if err != nil {
		t.Fatal(err)
	}
	client := &sdkServiceEnablement{services: services, crm: nil}
	_, err = client.EnabledServices(context.Background(), 249, []string{"compute.googleapis.com"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "serviceusage.googleapis.com") {
		t.Fatalf("expected actionable serviceusage hint, got %v", err)
	}
}

func TestSecretAddVersionDoesNotLeakCanaryInStageError(t *testing.T) {
	t.Parallel()
	secrets := &fakeSecrets{addErrOnce: true}
	if err := secrets.AddVersion(context.Background(), "p", "s", []byte(syntheticPassphraseCanary)); err == nil {
		t.Fatal("expected stage error")
	}
	if strings.Contains(errAddSecretVersionStage.Error(), syntheticPassphraseCanary) {
		t.Fatal("stage error must not include payload")
	}
}

func TestCheckpointReaderRequestsArchivedGenerationsOverHTTP(t *testing.T) {
	var versions, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		versions = r.URL.Query().Get("versions")
		path = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"items":[{"name":".pulumi/stacks/shop/preview.json","generation":"2","timeDeleted":"2026-01-01T00:00:00Z"}]}`)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	reader, err := NewGCSCheckpointReader(ctx, option.WithEndpoint(server.URL+"/storage/v1/"), option.WithHTTPClient(server.Client()), option.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	prior, err := reader.HasPriorCheckpointData(ctx, "magelift-state-test", "shop", "preview")
	if err != nil {
		t.Fatal(err)
	}
	if !prior || versions != "true" || !strings.HasSuffix(path, "/b/magelift-state-test/o") {
		t.Fatalf("prior=%v versions=%q path=%q", prior, versions, path)
	}
}
