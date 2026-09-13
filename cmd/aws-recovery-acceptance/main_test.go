package main

import "testing"

func TestParseObjectReferenceAcceptsSupportedSchemes(t *testing.T) {
	for _, reference := range []string{"s3://bucket/fixture/media", "aws-s3://bucket/fixture/media"} {
		bucket, prefix, err := parseObjectReference(reference)
		if err != nil || bucket != "bucket" || prefix != "fixture/media" {
			t.Fatalf("parseObjectReference(%q) = bucket=%q prefix=%q err=%v", reference, bucket, prefix, err)
		}
	}
}

func TestParseObjectReferenceRejectsUnsafeValues(t *testing.T) {
	for _, reference := range []string{"s3://bucket/", "s3://bucket/media?x=1", "s3://bucket/media#fragment", "s3://bucket/media%00bad"} {
		if _, _, err := parseObjectReference(reference); err == nil {
			t.Fatalf("parseObjectReference(%q) accepted unsafe reference", reference)
		}
	}
}

func TestValidatePartRejectsControlData(t *testing.T) {
	if err := validatePart("fixture\nvalue", "fixture"); err == nil {
		t.Fatal("validatePart accepted control data")
	}
}
