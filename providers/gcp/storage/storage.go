// Package storage provisions Magento media object storage on GCS.
//
// Asset-delivery design: Magento's AwsS3 remote-storage driver talks to
// GCS through S3 interop (HMAC keys below); storefronts fetch media URLs
// directly, so the bucket is world-readable. The bucket is single
// purpose: every object lives under media/ (driver prefix plus transfer
// scoping), nothing else is ever written here, and nothing written here
// is secret. Anonymous readers cannot use IAM conditions, so the public
// grant is unconditional by necessity; writes stay HMAC-gated to the
// media service account. Catalog images are public by nature (any
// visitor's browser fetches them); paid downloadable content is out of
// alpha scope and needs signed-URL or split delivery when it arrives.
// The HMAC secret never leaves Pulumi state and the workload Secret; it
// is not a stack output.
package storage

import (
	"errors"
	"fmt"
	"strings"

	"github.com/pulumi/pulumi-gcp/sdk/v9/go/gcp/serviceaccount"
	"github.com/pulumi/pulumi-gcp/sdk/v9/go/gcp/storage"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const TypeToken = "magelift:gcp:MediaStorage"

// MediaPrefix scopes Magento objects. Contract with the PHP lifecycle
// remote-storage writer: both sides use media/.
const MediaPrefix = "media/"

type Args struct {
	Project    string
	Location   string
	BucketName string
	Labels     map[string]string
}

type Component struct {
	pulumi.ResourceState
	BucketName   pulumi.StringOutput
	MediaURL     pulumi.StringOutput
	HmacAccessID pulumi.StringOutput
	HmacSecret   pulumi.StringOutput
	MediaAccount pulumi.StringOutput
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
		// Inherited (not enforced) so the world-readable grant below
		// applies. An org policy forcing prevention fails closed here.
		PublicAccessPrevention: pulumi.String("inherited"),
		ForceDestroy:           pulumi.Bool(true),
		Versioning:             &storage.BucketVersioningArgs{Enabled: pulumi.Bool(true)},
		Labels:                 pulumi.ToStringMap(args.Labels),
	}, parent)
	if err != nil {
		return nil, fmt.Errorf("create media GCS bucket: %w", err)
	}
	account, err := serviceaccount.NewAccount(ctx, name+"-media-sa", &serviceaccount.AccountArgs{
		Project:     pulumi.String(args.Project),
		AccountId:   pulumi.String(saID(name)),
		DisplayName: pulumi.String("MageLift media storage (" + name + ")"),
	}, parent)
	if err != nil {
		return nil, fmt.Errorf("create media service account: %w", err)
	}
	hmac, err := storage.NewHmacKey(ctx, name+"-media-hmac", &storage.HmacKeyArgs{
		Project:             pulumi.String(args.Project),
		ServiceAccountEmail: account.Email,
		State:               pulumi.String("ACTIVE"),
	}, parent, pulumi.DependsOn([]pulumi.Resource{account}))
	if err != nil {
		return nil, fmt.Errorf("create media HMAC key: %w", err)
	}
	if _, err := storage.NewBucketIAMMember(ctx, name+"-media-writer", &storage.BucketIAMMemberArgs{
		Bucket: bucket.Name,
		Role:   pulumi.String("roles/storage.objectAdmin"),
		Member: account.Email.ApplyT(func(email string) string {
			return "serviceAccount:" + email
		}).(pulumi.StringOutput),
	}, parent, pulumi.DependsOn([]pulumi.Resource{bucket, account})); err != nil {
		return nil, fmt.Errorf("grant media bucket access: %w", err)
	}
	// Unconditional by necessity: IAM rejects conditions on allUsers
	// bindings, and anonymous storefront readers have no other
	// principal. The bucket stays single-purpose instead.
	if _, err := storage.NewBucketIAMMember(ctx, name+"-media-public", &storage.BucketIAMMemberArgs{
		Bucket: bucket.Name,
		Role:   pulumi.String("roles/storage.objectViewer"),
		Member: pulumi.String("allUsers"),
	}, parent, pulumi.DependsOn([]pulumi.Resource{bucket})); err != nil {
		return nil, fmt.Errorf("grant media public reads: %w", err)
	}
	component.BucketName = bucket.Name
	component.MediaURL = bucket.Name.ApplyT(func(n string) string {
		return fmt.Sprintf("https://storage.googleapis.com/%s/%s", n, MediaPrefix)
	}).(pulumi.StringOutput)
	component.HmacAccessID = hmac.AccessId
	component.HmacSecret = hmac.Secret
	component.MediaAccount = account.Email
	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
		"bucketName": component.BucketName, "mediaURL": component.MediaURL,
		"hmacAccessId": component.HmacAccessID, "mediaAccount": component.MediaAccount,
	}); err != nil {
		return nil, err
	}
	return component, nil
}

// saID derives a valid service account ID from the stack name.
func saID(name string) string {
	cleaned := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			return r
		case r >= 'A' && r <= 'Z':
			return r - 'A' + 'a'
		default:
			return '-'
		}
	}, name)
	cleaned = strings.Trim(cleaned, "-")
	if len(cleaned) > 30 {
		cleaned = cleaned[:30]
		cleaned = strings.TrimRight(cleaned, "-")
	}
	if cleaned == "" {
		cleaned = "magelift-media"
	}
	if cleaned[0] >= '0' && cleaned[0] <= '9' {
		cleaned = "m-" + cleaned
		if len(cleaned) > 30 {
			cleaned = cleaned[:30]
		}
	}
	return cleaned
}
