package main

import (
	"testing"
)

func TestResolveEndpointUsesProfileBeforeRegionalDefault(t *testing.T) {
	got, err := resolveEndpoint("", "https://s3.example.invalid/", "fr-par")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://s3.example.invalid" {
		t.Fatalf("endpoint = %q", got)
	}

	got, err = resolveEndpoint("", "", "fr-par")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://s3.fr-par.scw.cloud" {
		t.Fatalf("regional endpoint = %q", got)
	}
}

func TestResolveEndpointRejectsUnsafeOverrides(t *testing.T) {
	for _, endpoint := range []string{
		"http://s3.example.invalid",
		"https://s3.example.invalid/path",
		"https://s3.example.invalid?token=secret",
		"https://s3.example.invalid#fragment",
	} {
		if _, err := resolveEndpoint(endpoint, "", "fr-par"); err == nil {
			t.Fatalf("resolveEndpoint(%q) accepted an unsafe endpoint", endpoint)
		}
	}
}

func TestParseObjectReferenceRejectsQueryAndEmptyPrefix(t *testing.T) {
	if bucket, prefix, err := parseObjectReference("scaleway-object://bucket/a/b"); err != nil || bucket != "bucket" || prefix != "a/b" {
		t.Fatalf("parsed reference = bucket=%q prefix=%q err=%v", bucket, prefix, err)
	}
	for _, reference := range []string{
		"scaleway-object://bucket/a?token=secret",
		"scaleway-object://bucket/",
		"https://bucket/a",
	} {
		if _, _, err := parseObjectReference(reference); err == nil {
			t.Fatalf("parseObjectReference(%q) accepted an invalid reference", reference)
		}
	}
}

func TestValidatePartRejectsMultilineIdentity(t *testing.T) {
	if err := validatePart("fixture\nowned", "fixture"); err == nil {
		t.Fatal("validatePart accepted a multiline identity")
	}
	if err := validatePart("fixture", "fixture"); err != nil {
		t.Fatal(err)
	}
}
