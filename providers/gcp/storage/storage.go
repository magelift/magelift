// Package storage provisions Magento media object storage on GCS.
package storage

import (
	"errors"
	"fmt"
	"strings"

	"github.com/pulumi/pulumi-gcp/sdk/v9/go/gcp/storage"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const TypeToken = "magelift:gcp:MediaStorage"

type Args struct {
	Project    string
	Location   string
	BucketName string
	Labels     map[string]string
}

type Component struct {
	pulumi.ResourceState
	BucketName pulumi.StringOutput
	MediaURL   pulumi.StringOutput
}

func New(ctx *pulumi.Context, name string, args Args, opts ...pulumi.ResourceOption) (*Component, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("storage name is required")
	}
	if strings.TrimSpace(args.Project) == "" || strings.TrimSpace(args.Location) == "" {
		return nil, errors.New("GCP project and location are required")
	}
	bucketName := args.BucketName
	if bucketName == "" {
		bucketName = name + "-media"
	}
	if len(bucketName) > 63 {
		bucketName = bucketName[:63]
		bucketName = strings.TrimRight(bucketName, "-")
	}
	component := &Component{}
	if err := ctx.RegisterComponentResourceV2(TypeToken, name, pulumi.Map{
		"project": pulumi.String(args.Project),
	}, component, opts...); err != nil {
		return nil, err
	}
	parent := pulumi.Parent(component)

	bucket, err := storage.NewBucket(ctx, name, &storage.BucketArgs{
		Project:                  pulumi.String(args.Project),
		Name:                     pulumi.String(bucketName),
		Location:                 pulumi.String(args.Location),
		UniformBucketLevelAccess: pulumi.Bool(true),
		PublicAccessPrevention:   pulumi.String("enforced"),
		ForceDestroy:             pulumi.Bool(true),
		Versioning:               &storage.BucketVersioningArgs{Enabled: pulumi.Bool(true)},
		Labels:                   pulumi.ToStringMap(args.Labels),
	}, parent)
	if err != nil {
		return nil, fmt.Errorf("create media GCS bucket: %w", err)
	}
	component.BucketName = bucket.Name
	// Magento media base URL via authenticated GCS JSON API path until Cloud CDN
	// custom domain is configured (escape hatch / existing cert later).
	component.MediaURL = bucket.Name.ApplyT(func(n string) string {
		return fmt.Sprintf("https://storage.googleapis.com/%s", n)
	}).(pulumi.StringOutput)
	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
		"bucketName": component.BucketName, "mediaURL": component.MediaURL,
	}); err != nil {
		return nil, err
	}
	return component, nil
}
