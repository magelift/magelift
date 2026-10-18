package bootstrap

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/googleapi"
	"google.golang.org/api/iam/v1"
	"google.golang.org/api/option"
)

func TestRetryWIFCallRetriesTransientBackendError(t *testing.T) {
	attempts := 0
	value, err := retryWIFCall(context.Background(), 0, func() (string, error) {
		attempts++
		if attempts < 3 {
			return "", &googleapi.Error{Code: 503, Message: "Policy checks are unavailable."}
		}
		return "ready", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if value != "ready" || attempts != 3 {
		t.Fatalf("value=%q attempts=%d", value, attempts)
	}
}

func TestRetryWIFCallDoesNotRetryPermanentError(t *testing.T) {
	attempts := 0
	_, err := retryWIFCall(context.Background(), 0, func() (string, error) {
		attempts++
		return "", &googleapi.Error{Code: 403, Message: "permission denied"}
	})
	if err == nil || attempts != 1 {
		t.Fatalf("err=%v attempts=%d", err, attempts)
	}
}

func TestMergeUnconditionalWorkloadIdentityMemberPreservesConditionalBinding(t *testing.T) {
	policy := &iam.Policy{
		Version: 3,
		Etag:    "etag-1",
		Bindings: []*iam.Binding{{
			Role:    workloadIdentityUserRole,
			Members: []string{"user:admin@example.com"},
			Condition: &iam.Expr{
				Title:      "time bound",
				Expression: "request.time < timestamp('2026-01-01T00:00:00Z')",
			},
		}},
	}
	updated, changed := mergeUnconditionalWorkloadIdentityMember(policy, "principalSet://iam.googleapis.com/projects/1/locations/global/workloadIdentityPools/pool/attribute.environment/build")
	if !changed {
		t.Fatal("expected member add")
	}
	if len(updated.Bindings) != 2 {
		t.Fatalf("bindings=%#v", updated.Bindings)
	}
	if updated.Bindings[0].Condition == nil || updated.Bindings[0].Members[0] != "user:admin@example.com" {
		t.Fatalf("conditional binding changed: %#v", updated.Bindings[0])
	}
}

func TestIAMClientGetPolicyRequestsVersion3(t *testing.T) {
	t.Parallel()
	const resource = "projects/example/serviceAccounts/ci@example.iam.gserviceaccount.com"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, ":getIamPolicy") {
			if r.URL.Query().Get("options.requestedPolicyVersion") != "3" {
				t.Fatalf("requestedPolicyVersion=%q", r.URL.Query().Get("options.requestedPolicyVersion"))
			}
			writeJSON(w, map[string]any{
				"version": float64(3),
				"etag":    "abc",
				"auditConfigs": []map[string]any{{
					"service":         "allServices",
					"auditLogConfigs": []map[string]any{{"logType": "ADMIN_READ"}},
				}},
				"bindings": []map[string]any{{
					"role":    workloadIdentityUserRole,
					"members": []string{"user:admin@example.com"},
					"condition": map[string]any{
						"expression": "request.time < timestamp('2026-01-01T00:00:00Z')",
						"title":      "keep",
					},
				}},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	service, err := iam.NewService(context.Background(), option.WithEndpoint(srv.URL), option.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}
	client := iamWIFClient{iam: service}
	policy, err := client.GetServiceAccountIAMPolicy(context.Background(), resource)
	if err != nil {
		t.Fatal(err)
	}
	if policy.Version != 3 || policy.Etag != "abc" || len(policy.AuditConfigs) != 1 {
		t.Fatalf("policy=%#v", policy)
	}
	if policy.Bindings[0].Condition == nil || policy.Bindings[0].Condition.Title != "keep" {
		t.Fatalf("binding=%#v", policy.Bindings[0])
	}
}

func TestIAMClientSetPolicyPreservesFullDocument(t *testing.T) {
	t.Parallel()
	const resource = "projects/example/serviceAccounts/ci@example.iam.gserviceaccount.com"
	var setBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, ":setIamPolicy") {
			setBody, _ = io.ReadAll(r.Body)
			writeJSON(w, map[string]any{"version": float64(3), "etag": "new"})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	service, err := iam.NewService(context.Background(), option.WithEndpoint(srv.URL), option.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}
	client := iamWIFClient{iam: service}
	policy := &iam.Policy{
		Version: 3,
		Etag:    "abc",
		AuditConfigs: []*iam.AuditConfig{{
			Service:         "allServices",
			AuditLogConfigs: []*iam.AuditLogConfig{{LogType: "ADMIN_READ"}},
		}},
		Bindings: []*iam.Binding{{
			Role:    workloadIdentityUserRole,
			Members: []string{"user:admin@example.com"},
			Condition: &iam.Expr{
				Title:      "keep",
				Expression: "request.time < timestamp('2026-01-01T00:00:00Z')",
			},
		}},
	}
	if err := client.SetServiceAccountIAMPolicy(context.Background(), resource, policy); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(setBody), `"auditConfigs"`) || !strings.Contains(string(setBody), `"condition"`) {
		t.Fatalf("set body dropped fields: %s", string(setBody))
	}
	if !strings.Contains(string(setBody), `"etag":"abc"`) {
		t.Fatalf("etag missing from set body: %s", string(setBody))
	}
}

func writeJSON(w http.ResponseWriter, payload map[string]any) {
	body, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
