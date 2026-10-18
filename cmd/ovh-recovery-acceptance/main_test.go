package main

import "testing"

func TestResolveEndpointUsesRegionalDefault(t *testing.T) {
	got, err := resolveEndpoint("", "GRA")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://s3.gra.io.cloud.ovh.net" {
		t.Fatalf("regional endpoint = %q", got)
	}

	got, err = resolveEndpoint("https://s3.example.invalid/", "gra")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://s3.example.invalid" {
		t.Fatalf("explicit endpoint = %q", got)
	}
}

func TestResolveEndpointRejectsUnsafeOverrides(t *testing.T) {
	for _, endpoint := range []string{
		"http://s3.example.invalid",
		"https://s3.example.invalid/path",
		"https://s3.example.invalid?token=secret",
		"https://s3.example.invalid#fragment",
	} {
		if _, err := resolveEndpoint(endpoint, "gra"); err == nil {
			t.Fatalf("resolveEndpoint(%q) accepted an unsafe endpoint", endpoint)
		}
	}
}

func TestParseObjectReferenceRejectsQueryAndEmptyPrefix(t *testing.T) {
	if bucket, prefix, err := parseObjectReference("ovh-object://bucket/a/b"); err != nil || bucket != "bucket" || prefix != "a/b" {
		t.Fatalf("parsed reference = bucket=%q prefix=%q err=%v", bucket, prefix, err)
	}
	for _, reference := range []string{
		"ovh-object://bucket/a?token=secret",
		"ovh-object://bucket/",
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
