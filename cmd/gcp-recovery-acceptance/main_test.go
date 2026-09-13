package main

import (
	"testing"
)

func TestParseObjectReferenceAcceptsSupportedSchemes(t *testing.T) {
	for _, scheme := range []string{"gs", "gcs", "gcp-storage"} {
		bucket, prefix, err := parseObjectReference(scheme + "://bucket/fixture/a%20b")
		if err != nil || bucket != "bucket" || prefix != "fixture/a b" {
			t.Fatalf("scheme=%s bucket=%q prefix=%q err=%v", scheme, bucket, prefix, err)
		}
	}
}

func TestParseObjectReferenceRejectsUnsafeValues(t *testing.T) {
	for _, reference := range []string{
		"gs://bucket/",
		"gs://bucket/a?token=secret",
		"https://bucket/a",
		"gs:///a",
	} {
		if _, _, err := parseObjectReference(reference); err == nil {
			t.Fatalf("parseObjectReference(%q) accepted invalid input", reference)
		}
	}
}

func TestParseObjectDataClass(t *testing.T) {
	class, err := parseObjectDataClass("infrastructure-state")
	if err != nil || class != "infrastructure-state" {
		t.Fatalf("class = %q err=%v", class, err)
	}
	class, err = parseObjectDataClass("audit-evidence")
	if err != nil || class != "audit-evidence" {
		t.Fatalf("audit class = %q err=%v", class, err)
	}
	class, err = parseObjectDataClass("")
	if err != nil || class != "media" {
		t.Fatalf("default class = %q err=%v", class, err)
	}
	if _, err := parseObjectDataClass("database"); err == nil {
		t.Fatal("unknown data class was accepted")
	}
}

func TestObjectSourcePrefix(t *testing.T) {
	if got := objectSourcePrefix("infrastructure-state", "fx"); got != "state/fx" {
		t.Fatalf("state prefix = %q", got)
	}
	if got := objectSourcePrefix("audit-evidence", "fx"); got != "audit/fx" {
		t.Fatalf("audit prefix = %q", got)
	}
	if got := objectSourcePrefix("media", "fx"); got != "fixture/fx" {
		t.Fatalf("media prefix = %q", got)
	}
}
