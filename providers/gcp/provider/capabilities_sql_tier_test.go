package gcpprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/option"
	sqladmin "google.golang.org/api/sqladmin/v1beta4"
)

func TestCloudSQLTierUsesRegionalDedicatedSeriesForCustomShapes(t *testing.T) {
	for _, tt := range []struct {
		name, tier, catalogTier, catalogRegion string
		want                                   bool
	}{
		{"custom absent from catalog", "db-custom-2-7680", "db-n1-standard-2", "us-central1", true},
		{"one CPU custom", "db-custom-1-3840", "db-n1-standard-1", "us-central1", true},
		{"high memory dedicated series", "db-custom-4-16384", "db-n1-highmem-4", "us-central1", true},
		{"exact custom catalog entry", "db-custom-2-7680", "db-custom-2-7680", "us-central1", true},
		{"dedicated series in another region", "db-custom-2-7680", "db-n1-standard-2", "europe-west1", false},
		{"Plus does not establish dedicated availability", "db-custom-2-7680", "db-perf-optimized-N-2", "us-central1", false},
		{"shared core does not establish dedicated availability", "db-custom-2-7680", "db-f1-micro", "us-central1", false},
		{"odd CPU", "db-custom-3-7680", "db-n1-standard-2", "us-central1", false},
		{"unaligned memory", "db-custom-2-7700", "db-n1-standard-2", "us-central1", false},
		{"insufficient memory", "db-custom-2-2048", "db-n1-standard-2", "us-central1", false},
		{"unsupported custom series", "db-custom-N4-2-7680", "db-n1-standard-2", "us-central1", false},
		{"Plus exact tier preserved", "db-perf-optimized-N-2", "db-perf-optimized-N-2", "us-central1", true},
		{"C4A exact tier preserved", "db-c4a-highmem-2", "db-c4a-highmem-2", "us-central1", true},
		{"Plus missing exact tier", "db-perf-optimized-N-4", "db-perf-optimized-N-2", "us-central1", false},
		{"Plus region mismatch", "db-perf-optimized-N-2", "db-perf-optimized-N-2", "europe-west1", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/projects/shop-test/tiers") {
					t.Errorf("unexpected capability request: %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected request", http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(&sqladmin.TiersListResponse{Items: []*sqladmin.Tier{
					nil, {Tier: tt.catalogTier, Region: []string{tt.catalogRegion}},
				}})
			}))
			defer server.Close()
			sql, err := sqladmin.NewService(context.Background(), option.WithHTTPClient(server.Client()), option.WithEndpoint(server.URL+"/"))
			if err != nil {
				t.Fatal(err)
			}
			client := &SDKCapabilityClient{sql: sql}
			got, err := client.CloudSQLTier(context.Background(), "shop-test", "us-central1", tt.tier)
			if err != nil || got != tt.want {
				t.Fatalf("CloudSQLTier() = %v, %v; want %v, nil", got, err, tt.want)
			}
			if got := calls.Load(); got != 1 {
				t.Fatalf("catalog reads = %d, want 1", got)
			}
		})
	}
}

func TestCloudSQLCustomTierFailsClosedOnCatalogReadError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "catalog unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	sql, err := sqladmin.NewService(context.Background(), option.WithHTTPClient(server.Client()), option.WithEndpoint(server.URL+"/"))
	if err != nil {
		t.Fatal(err)
	}
	client := &SDKCapabilityClient{sql: sql}
	available, err := client.CloudSQLTier(context.Background(), "shop-test", "us-central1", "db-custom-2-7680")
	if err == nil || available {
		t.Fatalf("failed catalog read returned available=%v, err=%v", available, err)
	}
}
