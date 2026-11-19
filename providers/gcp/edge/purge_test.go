package edge

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
)

func TestComputePurgeScopesInvalidationToTheGKEStorefront(t *testing.T) {
	const (
		host   = "shop.example.com"
		marker = "magelift/shop/staging"
		mapURL = "/compute/v1/projects/project-1/global/urlMaps/serving"
	)
	for _, tc := range []struct {
		name  string
		hosts []string
		want  bool
	}{
		{"storefront", []string{host}, true},
		{"different storefront", []string{"other.example.com"}, false},
		{"shared map", []string{host, "other.example.com"}, false},
		{"wildcard", []string{"*.example.com"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var invalidations atomic.Int32
			mux := http.NewServeMux()
			mux.HandleFunc("GET "+mapURL, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(compute.UrlMap{Name: "serving", HostRules: []*compute.HostRule{{Hosts: tc.hosts}}}); err != nil {
					t.Error(err)
				}
			})
			mux.HandleFunc("POST "+mapURL+"/invalidateCache", func(w http.ResponseWriter, r *http.Request) {
				invalidations.Add(1)
				var rule compute.CacheInvalidationRule
				if err := json.NewDecoder(r.Body).Decode(&rule); err != nil || rule.Host != host || rule.Path != "/*" {
					t.Errorf("invalid cache invalidation rule %+v, error %v", rule, err)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"name":"invalidate-1"}`))
			})
			mux.HandleFunc("GET /compute/v1/projects/project-1/global/operations/invalidate-1", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"name":"invalidate-1","status":"DONE"}`))
			})
			server := httptest.NewServer(mux)
			t.Cleanup(server.Close)
			service, err := compute.NewService(t.Context(), option.WithEndpoint(server.URL+"/compute/v1/"), option.WithoutAuthentication(), option.WithHTTPClient(server.Client()))
			if err != nil {
				t.Fatal(err)
			}
			api, err := newComputeGoogleEdgeAPI(service, "project-1")
			if err != nil {
				t.Fatal(err)
			}
			result, err := api.Purge(t.Context(), GoogleEdgeApplyRequest{
				Project: "project-1", OwnershipMarker: marker, URLMapName: "serving", Hostname: host, Domains: []string{host},
				GKEIngressName: "shop", GKEIngressNamespace: "staging", GKEIngressOwner: marker,
			})
			if tc.want {
				if err != nil || result.Invalidation != "invalidate-1" || invalidations.Load() != 1 {
					t.Fatalf("invalidation = %q, requests = %d, error = %v", result.Invalidation, invalidations.Load(), err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "refusing to purge") || invalidations.Load() != 0 {
				t.Fatalf("expected rejection before invalidation, requests = %d, error = %v", invalidations.Load(), err)
			}
		})
	}
}
