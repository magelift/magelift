package main

import (
	"context"
	"io"
	"reflect"
	"strings"
	"testing"

	gcpresilience "github.com/magelift/magelift/providers/gcp/resilience"
)

func TestRunRejectsMissingInstanceBeforeAPI(t *testing.T) {
	err := run(context.Background(), []string{"--project=demo"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "instance must be a single Cloud SQL name") {
		t.Fatalf("missing instance error = %v", err)
	}
}

func TestRunRejectsWildcardInstanceBeforeAPI(t *testing.T) {
	err := run(context.Background(), []string{"--project=demo", "--instance=shop/*"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "instance must be a single Cloud SQL name") {
		t.Fatalf("wildcard instance error = %v", err)
	}
}

func TestUniqueBackupTypesDedupesAndSorts(t *testing.T) {
	got := uniqueBackupTypes([]gcpresilience.CloudSQLBackup{
		{Type: "ON_DEMAND"},
		{Type: "FINAL"},
		{Type: "FINAL"},
		{Type: ""},
	})
	want := []string{"FINAL", "ON_DEMAND", "unspecified"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("types = %#v, want %#v", got, want)
	}
}
