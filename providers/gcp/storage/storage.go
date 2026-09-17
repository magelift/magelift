// Package storage provisions Magento media object storage on GCS.
//
// Asset-delivery design: Magento's AwsS3 remote-storage driver talks to
// GCS through S3 interop (HMAC keys below). The bucket stays fully
// private (public access prevention enforced): Magento routes both
// MEDIA and VAR_IMPORT_EXPORT through the remote driver, so import and
// export files share the bucket and must never be world-readable.
// Storefronts fetch media through the application instead: image URLs
// stay app-relative (base_media_url default), nginx falls back to
// get.php, and get.php materializes from remote storage through the
// Synchronizer. Only the media service account reads and writes here.
// The bucket uses fine-grained access because the driver sets private
// object ACLs on every write, which uniform buckets reject; IAM grants
// no reads to anyone but the service account. The HMAC secret never
// leaves Pulumi state and the workload Secret; it is not a stack output.
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

// MediaPrefix is the Magento MEDIA subtree inside remote storage. It is
// pinned to DirectoryList::MEDIA's URL path ("media"), not to the
// remote_storage root prefix (which stays empty so each directory URI
// lands at the bucket root). Magento appends the URI itself: a
// media-relative catalog/product/a.jpg lives at media/catalog/a.jpg,
// while VAR_IMPORT_EXPORT lands at the sibling import_export/ subtree
// the CLI never touches. Refs: DirectoryList::getDefaultConfig,
// RemoteStorage\Filesystem::getDirectoryWrite,
// AwsS3Factory::createConfigured (magento/magento2 2.4.9).
const MediaPrefix = "media/"

// MediaObjectKey maps a pub/media-relative path to its remote object
// key. Relative paths use slash separators, as the CLI transfer layer
// enforces.
func MediaObjectKey(relative string) string {
	return MediaPrefix + relative
}

type Args struct {
	Project    string
	Location   string
	BucketName string
	Labels     map[string]string
}

type Component struct {
	pulumi.ResourceState
	BucketName   pulumi.StringOutput
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
		Project:  pulumi.String(args.Project),
		Name:     pulumi.String(bucketName),
		Location: pulumi.String(args.Location),
		// Fine-grained: the AwsS3 driver sets private object ACLs on
		// every write and uniform buckets reject ACL operations.
		UniformBucketLevelAccess: pulumi.Bool(false),
		// Fully private delivery is through the application; enforce
		// prevention so no future binding can expose the bucket.
		PublicAccessPrevention: pulumi.String("enforced"),
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
	component.BucketName = bucket.Name
	component.HmacAccessID = hmac.AccessId
	component.HmacSecret = hmac.Secret
	component.MediaAccount = account.Email
	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
		"bucketName":   component.BucketName,
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
