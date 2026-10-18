package storage

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type mocks struct {
	mu        sync.Mutex
	resources []pulumi.MockResourceArgs
}

func (m *mocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	m.resources = append(m.resources, args)
	m.mu.Unlock()
	state := args.Inputs.Copy()
	switch args.TypeToken {
	case "aws:s3/bucket:Bucket":
		state["arn"] = resource.NewStringProperty("arn:aws:s3:::shop-media")
		state["bucket"] = resource.NewStringProperty("shop-media")
		state["bucketRegionalDomainName"] = resource.NewStringProperty("shop-media.s3.eu-west-3.amazonaws.com")
	case "aws:cloudfront/distribution:Distribution":
		state["arn"] = resource.NewStringProperty("arn:aws:cloudfront::123456789012:distribution/EDFDVBD6EXAMPLE")
		state["domainName"] = resource.NewStringProperty("d111111abcdef8.cloudfront.net")
	}
	return args.Name + "-id", state, nil
}

func (*mocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) { return args.Args, nil }

func TestMediaStorageGraphIsPrivateVersionedAndEncrypted(t *testing.T) {
	m := deploy(t, validArgs())
	if m.count("aws:s3/bucket:Bucket") != 1 || m.count("aws:cloudfront/distribution:Distribution") != 1 {
		t.Fatalf("media graph does not contain one bucket and distribution: %#v", m.resources)
	}
	block := m.one(t, "aws:s3/bucketPublicAccessBlock:BucketPublicAccessBlock").Inputs
	for _, key := range []string{"blockPublicAcls", "blockPublicPolicy", "ignorePublicAcls", "restrictPublicBuckets"} {
		if !block[resource.PropertyKey(key)].BoolValue() {
			t.Fatalf("%s is not enabled", key)
		}
	}
	encryption := m.one(t, "aws:s3/bucketServerSideEncryptionConfiguration:BucketServerSideEncryptionConfiguration").Inputs
	encoded, err := json.Marshal(encryption.Mappable())
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"aws:kms", "bucketKeyEnabled", "SSE-C"} {
		if !strings.Contains(string(encoded), required) {
			t.Fatalf("encryption policy lacks %s: %s", required, encoded)
		}
	}
	policy := m.one(t, "aws:s3/bucketPolicy:BucketPolicy").Inputs[resource.PropertyKey("policy")].StringValue()
	for _, required := range []string{"cloudfront.amazonaws.com", "s3:GetObject", "AWS:SourceArn"} {
		if !strings.Contains(policy, required) {
			t.Fatalf("bucket policy lacks %s: %s", required, policy)
		}
	}
}

func TestRejectsUnsafeStorageInputsBeforeRegistration(t *testing.T) {
	tests := []Args{
		func() Args { args := validArgs(); args.BucketName = "UpperCase"; return args }(),
		func() Args { args := validArgs(); args.KMSKeyARN = "alias/aws/s3"; return args }(),
		func() Args { args := validArgs(); args.NoncurrentVersionRetentionDays = 0; return args }(),
		func() Args { args := validArgs(); args.Domain = "media.example.com"; return args }(),
	}
	for index, args := range tests {
		m := &mocks{}
		err := pulumi.RunErr(func(ctx *pulumi.Context) error { _, err := New(ctx, "shop", args); return err }, pulumi.WithMocks("magelift", "test", m))
		if err == nil {
			t.Fatalf("case %d was accepted", index)
		}
		if len(m.resources) != 0 {
			t.Fatalf("case %d registered resources", index)
		}
	}
}

func validArgs() Args {
	return Args{
		Region: "eu-west-3", BucketName: "shop-media", KMSKeyARN: "arn:aws:kms:eu-west-3:123456789012:key/00000000-0000-0000-0000-000000000000",
		NoncurrentVersionRetentionDays: 30, AbortMultipartUploadDays: 7, Tags: map[string]string{"magelift:managed-by": "magelift"},
	}
}

func deploy(t *testing.T, args Args) *mocks {
	t.Helper()
	m := &mocks{}
	if err := pulumi.RunErr(func(ctx *pulumi.Context) error { _, err := New(ctx, "shop", args); return err }, pulumi.WithMocks("magelift", "test", m)); err != nil {
		t.Fatal(err)
	}
	return m
}

func (m *mocks) count(token string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, resource := range m.resources {
		if resource.TypeToken == token {
			count++
		}
	}
	return count
}

func (m *mocks) one(t *testing.T, token string) pulumi.MockResourceArgs {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, resource := range m.resources {
		if resource.TypeToken == token {
			return resource
		}
	}
	t.Fatalf("resource %s not found", token)
	return pulumi.MockResourceArgs{}
}
