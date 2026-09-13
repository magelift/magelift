package resilience

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ovh/go-ovh/ovh"
)

func TestDecodeOVHIdentityListAcceptsIDsAndObjects(t *testing.T) {
	t.Parallel()
	ids, err := decodeOVHIdentityList([]byte(`["a","b"]`))
	if err != nil || len(ids) != 2 || ids[0] != "a" || ids[1] != "b" {
		t.Fatalf("id list = %#v err=%v", ids, err)
	}
	objects, err := decodeOVHIdentityList([]byte(`[{"id":"c"},{"id":"d"}]`))
	if err != nil || len(objects) != 2 || objects[0] != "c" || objects[1] != "d" {
		t.Fatalf("object list = %#v err=%v", objects, err)
	}
	empty, err := decodeOVHIdentityList([]byte(`null`))
	if err != nil || empty != nil {
		t.Fatalf("null list = %#v err=%v", empty, err)
	}
}

func TestOVHDatabaseInstanceMapsRegionAndRetentionFromProviderShape(t *testing.T) {
	t.Parallel()
	api := &ovhDatabaseSDK{}
	pitr := time.Date(2026, 8, 13, 11, 0, 0, 0, time.UTC)
	instance, err := api.instance(ovhDatabaseService{
		ID: "svc", Description: "owned", Engine: "mysql", Version: "8.4",
		Status: "READY", Plan: "essential", NodeNumber: 1,
		Nodes:   []ovhDatabaseNode{{ID: "node-1", Region: "GRA", Flavor: "db1-4"}},
		Backups: ovhDatabaseServiceBackup{PITR: &pitr},
	}, "", "mysql")
	if err != nil {
		t.Fatal(err)
	}
	if instance.Region != "GRA" || instance.Flavor != "db1-4" || instance.BackupRetentionDays != 2 || !instance.Encrypted || !instance.PITRAvailableFrom.Equal(pitr) || instance.DeletionProtection {
		t.Fatalf("instance = %#v", instance)
	}
}

func TestOVHDatabaseBackupKeepsPlanRetentionWhenAPIOmitsRetentionDays(t *testing.T) {
	t.Parallel()
	api := &ovhDatabaseSDK{}
	backup, err := api.backup(ovhDatabaseBackup{
		ID: "backup", CreatedAt: time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC),
		Status: "READY", Engine: "mysql", Plan: "essential", Region: "GRA",
	}, "svc", "mysql")
	if err != nil {
		t.Fatal(err)
	}
	if backup.RetentionDays != 2 || !backup.Encrypted || len(backup.Regions) != 1 || backup.Regions[0] != "GRA" {
		t.Fatalf("backup = %#v", backup)
	}
}

func TestDecodeOVHRegionNamesAcceptsStringsAndObjects(t *testing.T) {
	t.Parallel()
	names, err := decodeOVHRegionNames([]byte(`["GRA","SBG"]`))
	if err != nil || len(names) != 2 || names[0] != "GRA" || names[1] != "SBG" {
		t.Fatalf("string regions = %#v err=%v", names, err)
	}
	objects, err := decodeOVHRegionNames([]byte(`[{"name":"GRA"}]`))
	if err != nil || len(objects) != 1 || objects[0] != "GRA" {
		t.Fatalf("object regions = %#v err=%v", objects, err)
	}
}
func TestOVHDatabasePlanRetentionDays(t *testing.T) {
	t.Parallel()
	if got := ovhDatabasePlanRetentionDays("essential"); got != 2 {
		t.Fatalf("essential = %d", got)
	}
	if got := ovhDatabasePlanRetentionDays("production"); got != 14 {
		t.Fatalf("production = %d", got)
	}
	if got := ovhDatabasePlanRetentionDays("unknown"); got != 0 {
		t.Fatalf("unknown = %d, want fail-closed 0", got)
	}
}

func TestOVHDatabaseRestoreSendsIPRestrictions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/cloud/project/project/database/mysql" {
			http.NotFound(response, request)
			return
		}
		var body ovhDatabaseServiceCreation
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		if len(body.IPRestrictions) != 1 || body.IPRestrictions[0].IP != "198.51.100.7/32" {
			http.Error(response, "missing IP restriction", http.StatusBadRequest)
			return
		}
		_, _ = response.Write([]byte(`{"id":"restore","engine":"mysql","version":"8.4","region":"GRA","status":"CREATING","plan":"essential","flavor":"db1-4","nodeNumber":1}`))
	}))
	defer server.Close()
	client, err := ovh.NewAccessTokenClient(server.URL, "test-token")
	if err != nil {
		t.Fatal(err)
	}
	api := newOVHDatabaseSDK(client, NativeAPIConfig{DatabaseProjectID: "project", DatabaseEngine: "mysql"})
	instance, err := api.CreateInstanceFromBackup(context.Background(), DatabaseRestoreRequest{
		InstanceID: "source", Engine: "mysql", Description: "restore", Region: "GRA", Plan: "essential", Flavor: "db1-4", Version: "8.4", NodeCount: 1, BackupID: "backup",
		IPRestrictions: []string{"198.51.100.7/32"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if instance.ID != "restore" {
		t.Fatalf("instance = %#v", instance)
	}
}

func TestOVHDatabaseRestoreSendsPointInTime(t *testing.T) {
	point := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/cloud/project/project/database/mysql" {
			http.NotFound(response, request)
			return
		}
		var body ovhDatabaseServiceCreation
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		if body.ForkFrom.BackupID != "" || body.ForkFrom.PointInTime == nil || !body.ForkFrom.PointInTime.Equal(point) {
			http.Error(response, "invalid point-in-time fork", http.StatusBadRequest)
			return
		}
		_, _ = response.Write([]byte(`{"id":"restore","engine":"mysql","version":"8.4","region":"GRA","status":"CREATING","plan":"essential","flavor":"db1-4","nodeNumber":1}`))
	}))
	defer server.Close()
	client, err := ovh.NewAccessTokenClient(server.URL, "test-token")
	if err != nil {
		t.Fatal(err)
	}
	api := newOVHDatabaseSDK(client, NativeAPIConfig{DatabaseProjectID: "project", DatabaseEngine: "mysql"})
	instance, err := api.CreateInstanceFromBackup(context.Background(), DatabaseRestoreRequest{
		InstanceID: "source", Engine: "mysql", Description: "restore", Region: "GRA", Plan: "essential", Flavor: "db1-4", Version: "8.4", NodeCount: 1,
		PointInTime: &point,
	})
	if err != nil {
		t.Fatal(err)
	}
	if instance.ID != "restore" {
		t.Fatalf("instance = %#v", instance)
	}
}
