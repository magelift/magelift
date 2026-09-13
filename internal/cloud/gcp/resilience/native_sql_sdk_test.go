package resilience

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/api/option"
	sqladmin "google.golang.org/api/sqladmin/v1beta4"
)

func TestCloudSQLSDKCreateBackupUsesLegacyBackupRunsInsert(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/sql/v1beta4/projects/demo/instances/orders/backupRuns" {
			t.Fatalf("request = %s %s, want POST /sql/v1beta4/projects/demo/instances/orders/backupRuns", request.Method, request.URL.Path)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		var backup sqladmin.BackupRun
		if err := json.Unmarshal(body, &backup); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if backup.Description != "description" {
			t.Fatalf("backup request = %#v", backup)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"name":"projects/demo/operations/backup-1","status":"PENDING"}`))
	}))
	defer server.Close()

	service, err := sqladmin.NewService(context.Background(), option.WithEndpoint(server.URL+"/"), option.WithoutAuthentication(), option.WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	api := cloudSQLSDK{service: service}
	operation, err := api.CreateBackup(context.Background(), "demo", "orders", "description", 3)
	if err != nil {
		t.Fatal(err)
	}
	if operation.Name != "projects/demo/operations/backup-1" || operation.Status != "pending" {
		t.Fatalf("operation = %#v", operation)
	}
}
