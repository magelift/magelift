package gcpprovider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/api/option"
	serviceusage "google.golang.org/api/serviceusage/v1"
)

func TestValkeyRESTClientListsCurrentValkeyEndpointAndPaginates(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Valkey request method = %s, want GET", r.Method)
		}
		requests = append(requests, r.URL.RequestURI())
		if r.URL.Path != "/v1/projects/shop-prod/locations/europe-west1/instances" {
			t.Errorf("Valkey request path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("pageToken") == "next" {
			_, _ = fmt.Fprint(w, `{"instances":[]}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"instances":[],"nextPageToken":"next"}`)
	}))
	defer server.Close()

	client := NewValkeyRESTClient(server.Client(), server.URL)
	if err := client.ListInstances(context.Background(), "shop-prod", "europe-west1"); err != nil {
		t.Fatalf("ListInstances() error = %v", err)
	}
	if len(requests) != 2 {
		t.Fatalf("request count = %d, want 2", len(requests))
	}
}

func TestValkeyRESTClientFailsClosedForUnreachableLocations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Valkey request method = %s, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"unreachable":["europe-west1"]}`)
	}))
	defer server.Close()

	client := NewValkeyRESTClient(server.Client(), server.URL)
	if err := client.ListInstances(context.Background(), "shop-prod", "europe-west1"); err == nil {
		t.Fatal("ListInstances() error = nil, want unreachable-location error")
	}
}

func TestValkeyRESTClientRejectsRepeatedPageToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"nextPageToken":"same"}`)
	}))
	defer server.Close()

	client := NewValkeyRESTClient(server.Client(), server.URL)
	if err := client.ListInstances(context.Background(), "shop-prod", "europe-west1"); err == nil {
		t.Fatal("ListInstances() error = nil, want repeated-page-token error")
	}
}

func TestValkeyRESTClientRejectsResourcePathInjection(t *testing.T) {
	client := NewValkeyRESTClient(http.DefaultClient, "https://memorystore.googleapis.com")
	if err := client.ListInstances(context.Background(), "shop/prod", "europe-west1"); err == nil {
		t.Fatal("ListInstances() error = nil, want invalid path segment error")
	}
}

func TestSDKCapabilityClientUsesProjectNumberForBatchedServiceUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Service Usage request method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/v1/projects/249/services:batchGet" {
			t.Errorf("Service Usage request path = %q, want project-number batchGet path", r.URL.Path)
		}
		names := r.URL.Query()["names"]
		if len(names) != 2 || names[0] != "projects/249/services/compute.googleapis.com" || names[1] != "projects/249/services/container.googleapis.com" {
			t.Errorf("Service Usage names = %#v, want project-number service names", names)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"services":[{"name":"projects/249/services/compute.googleapis.com","state":"ENABLED"},{"name":"projects/249/services/container.googleapis.com","state":"DISABLED"}]}`)
	}))
	defer server.Close()

	services, err := serviceusage.NewService(context.Background(), option.WithHTTPClient(server.Client()), option.WithEndpoint(server.URL))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	client := &SDKCapabilityClient{services: services}
	result, err := client.EnabledServices(context.Background(), ProjectIdentity{ID: "shop-prod", Number: 249}, []string{"compute.googleapis.com", "container.googleapis.com"})
	if err != nil {
		t.Fatalf("EnabledServices() error = %v", err)
	}
	if !result["compute.googleapis.com"] || result["container.googleapis.com"] {
		t.Fatalf("EnabledServices() = %#v, want compute enabled and container disabled", result)
	}
}
