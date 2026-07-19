package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	awsprovider "github.com/pulumi/pulumi-aws/sdk/v7/go/aws"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/cloudfront"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/s3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const ComponentToken = "magelift:aws:MediaStorage"

var bucketNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)
var kmsARNPattern = regexp.MustCompile(`^arn:(?:aws|aws-us-gov|aws-cn):kms:[a-z0-9-]+:[0-9]{12}:key/[A-Za-z0-9-]+$`)
var certificateARNPattern = regexp.MustCompile(`^arn:(?:aws|aws-us-gov|aws-cn):acm:[a-z0-9-]+:[0-9]{12}:certificate/[A-Za-z0-9-]+$`)
var domainPattern = regexp.MustCompile(`^(?:[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+[A-Za-z]{2,63}$`)

type Args struct {
	Region                         string
	BucketName                     string
	KMSKeyARN                      string
	NoncurrentVersionRetentionDays int
	AbortMultipartUploadDays       int
	Domain                         string
	CertificateARN                 string
	RegionalProvider               *awsprovider.Provider
	GlobalProvider                 *awsprovider.Provider
	Tags                           map[string]string
}

type Component struct {
	pulumi.ResourceState
	BucketName      pulumi.StringOutput `pulumi:"bucketName"`
	BucketARN       pulumi.StringOutput `pulumi:"bucketArn"`
	DistributionID  pulumi.IDOutput     `pulumi:"distributionId"`
	DistributionURL pulumi.StringOutput `pulumi:"distributionUrl"`
}

func New(ctx *pulumi.Context, name string, args Args, opts ...pulumi.ResourceOption) (*Component, error) {
	if err := validate(name, args); err != nil {
		return nil, err
	}
	component := &Component{}
	if err := ctx.RegisterComponentResource(ComponentToken, name, component, opts...); err != nil {
		return nil, err
	}
	child := pulumi.Parent(component)
	regionalChild := []pulumi.ResourceOption{child}
	if args.RegionalProvider != nil {
		regionalChild = append(regionalChild, pulumi.Provider(args.RegionalProvider))
	}
	globalChild := []pulumi.ResourceOption{child}
	if args.GlobalProvider != nil {
		globalChild = append(globalChild, pulumi.Provider(args.GlobalProvider))
	}
	tags := pulumi.ToStringMap(tags(args.Tags, name))
	bucket, err := s3.NewBucket(ctx, name+"-bucket", &s3.BucketArgs{
		Bucket: pulumi.String(args.BucketName), Region: pulumi.String(args.Region), ForceDestroy: pulumi.Bool(false), Tags: tags,
	}, regionalChild...)
	if err != nil {
		return nil, fmt.Errorf("create media bucket: %w", err)
	}
	if _, err := s3.NewBucketOwnershipControls(ctx, name+"-ownership", &s3.BucketOwnershipControlsArgs{
		Bucket: bucket.ID(), Region: pulumi.String(args.Region), Rule: &s3.BucketOwnershipControlsRuleArgs{ObjectOwnership: pulumi.String("BucketOwnerEnforced")},
	}, regionalChild...); err != nil {
		return nil, fmt.Errorf("configure media bucket ownership: %w", err)
	}
	if _, err := s3.NewBucketPublicAccessBlock(ctx, name+"-public-access", &s3.BucketPublicAccessBlockArgs{
		Bucket: bucket.ID(), Region: pulumi.String(args.Region), BlockPublicAcls: pulumi.Bool(true), BlockPublicPolicy: pulumi.Bool(true), IgnorePublicAcls: pulumi.Bool(true), RestrictPublicBuckets: pulumi.Bool(true),
	}, regionalChild...); err != nil {
		return nil, fmt.Errorf("configure media bucket public access: %w", err)
	}
	if _, err := s3.NewBucketVersioning(ctx, name+"-versioning", &s3.BucketVersioningArgs{
		Bucket: bucket.ID(), Region: pulumi.String(args.Region), VersioningConfiguration: &s3.BucketVersioningVersioningConfigurationArgs{Status: pulumi.String("Enabled")},
	}, regionalChild...); err != nil {
		return nil, fmt.Errorf("enable media bucket versioning: %w", err)
	}
	if _, err := s3.NewBucketServerSideEncryptionConfiguration(ctx, name+"-encryption", &s3.BucketServerSideEncryptionConfigurationArgs{
		Bucket: bucket.ID(), Region: pulumi.String(args.Region), Rules: s3.BucketServerSideEncryptionConfigurationRuleArray{
			s3.BucketServerSideEncryptionConfigurationRuleArgs{
				ApplyServerSideEncryptionByDefault: &s3.BucketServerSideEncryptionConfigurationRuleApplyServerSideEncryptionByDefaultArgs{SseAlgorithm: pulumi.String("aws:kms"), KmsMasterKeyId: pulumi.String(args.KMSKeyARN)},
				BucketKeyEnabled:                   pulumi.Bool(true), BlockedEncryptionTypes: pulumi.StringArray{pulumi.String("SSE-C")},
			},
		},
	}, regionalChild...); err != nil {
		return nil, fmt.Errorf("configure media bucket encryption: %w", err)
	}
	if _, err := s3.NewBucketLifecycleConfiguration(ctx, name+"-lifecycle", &s3.BucketLifecycleConfigurationArgs{
		Bucket: bucket.ID(), Region: pulumi.String(args.Region), Rules: s3.BucketLifecycleConfigurationRuleArray{
			s3.BucketLifecycleConfigurationRuleArgs{
				Id: pulumi.String("media-retention"), Status: pulumi.String("Enabled"),
				AbortIncompleteMultipartUpload: &s3.BucketLifecycleConfigurationRuleAbortIncompleteMultipartUploadArgs{DaysAfterInitiation: pulumi.Int(args.AbortMultipartUploadDays)},
				NoncurrentVersionExpiration:    &s3.BucketLifecycleConfigurationRuleNoncurrentVersionExpirationArgs{NoncurrentDays: pulumi.Int(args.NoncurrentVersionRetentionDays)},
			},
		},
	}, regionalChild...); err != nil {
		return nil, fmt.Errorf("configure media bucket lifecycle: %w", err)
	}
	oac, err := cloudfront.NewOriginAccessControl(ctx, name+"-oac", &cloudfront.OriginAccessControlArgs{
		Name: pulumi.String(name + "-media"), Description: pulumi.String("MageLift private media origin"), OriginAccessControlOriginType: pulumi.String("s3"), SigningBehavior: pulumi.String("always"), SigningProtocol: pulumi.String("sigv4"),
	}, globalChild...)
	if err != nil {
		return nil, fmt.Errorf("create media origin access control: %w", err)
	}
	viewerCertificate := &cloudfront.DistributionViewerCertificateArgs{CloudfrontDefaultCertificate: pulumi.Bool(true), MinimumProtocolVersion: pulumi.String("TLSv1.2_2021")}
	distributionArgs := &cloudfront.DistributionArgs{
		Aliases: aliasValues(args.Domain), Comment: pulumi.String("MageLift private media"), Enabled: pulumi.Bool(true), HttpVersion: pulumi.String("http2and3"), IsIpv6Enabled: pulumi.Bool(true), PriceClass: pulumi.String("PriceClass_100"),
		Origins: cloudfront.DistributionOriginArray{cloudfront.DistributionOriginArgs{DomainName: bucket.BucketRegionalDomainName, OriginId: pulumi.String("media-s3"), OriginAccessControlId: oac.ID()}},
		DefaultCacheBehavior: &cloudfront.DistributionDefaultCacheBehaviorArgs{
			AllowedMethods: pulumi.StringArray{pulumi.String("GET"), pulumi.String("HEAD")}, CachedMethods: pulumi.StringArray{pulumi.String("GET"), pulumi.String("HEAD")},
			CachePolicyId: pulumi.String("658327ea-f89d-4fab-a63d-7e88639e58f6"), Compress: pulumi.Bool(true), TargetOriginId: pulumi.String("media-s3"), ViewerProtocolPolicy: pulumi.String("redirect-to-https"),
		},
		Restrictions:      &cloudfront.DistributionRestrictionsArgs{GeoRestriction: &cloudfront.DistributionRestrictionsGeoRestrictionArgs{RestrictionType: pulumi.String("none")}},
		ViewerCertificate: viewerCertificate, WaitForDeployment: pulumi.Bool(false), Tags: tags,
	}
	if args.CertificateARN != "" {
		distributionArgs.ViewerCertificate = &cloudfront.DistributionViewerCertificateArgs{AcmCertificateArn: pulumi.String(args.CertificateARN), MinimumProtocolVersion: pulumi.String("TLSv1.2_2021"), SslSupportMethod: pulumi.String("sni-only")}
	}
	distribution, err := cloudfront.NewDistribution(ctx, name+"-cdn", distributionArgs, globalChild...)
	if err != nil {
		return nil, fmt.Errorf("create media distribution: %w", err)
	}
	policy := pulumi.All(bucket.Arn, distribution.Arn).ApplyT(func(values []interface{}) (string, error) {
		bucketARN, _ := values[0].(string)
		distributionARN, _ := values[1].(string)
		return bucketPolicy(bucketARN, distributionARN)
	}).(pulumi.StringOutput)
	if _, err := s3.NewBucketPolicy(ctx, name+"-policy", &s3.BucketPolicyArgs{Bucket: bucket.ID(), Region: pulumi.String(args.Region), Policy: policy}, regionalChild...); err != nil {
		return nil, fmt.Errorf("restrict media bucket to CloudFront: %w", err)
	}
	component.BucketName, component.BucketARN = bucket.Bucket, bucket.Arn
	component.DistributionID, component.DistributionURL = distribution.ID(), pulumi.Sprintf("https://%s", distribution.DomainName)
	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{"bucketName": component.BucketName, "bucketArn": component.BucketARN, "distributionId": component.DistributionID, "distributionUrl": component.DistributionURL}); err != nil {
		return nil, err
	}
	return component, nil
}

func validate(name string, args Args) error {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(args.Region) == "" || !bucketNamePattern.MatchString(args.BucketName) || !kmsARNPattern.MatchString(args.KMSKeyARN) {
		return errors.New("media storage requires a valid name, region, bucket name, and customer-managed KMS key ARN")
	}
	if args.NoncurrentVersionRetentionDays < 1 || args.NoncurrentVersionRetentionDays > 3650 || args.AbortMultipartUploadDays < 1 || args.AbortMultipartUploadDays > 30 {
		return errors.New("media lifecycle retention values are outside the supported bounds")
	}
	if args.Domain != "" && args.CertificateARN == "" {
		return errors.New("a media domain requires an ACM certificate ARN")
	}
	if args.Domain != "" && !domainPattern.MatchString(args.Domain) {
		return errors.New("media domain is invalid")
	}
	if args.CertificateARN != "" && !certificateARNPattern.MatchString(args.CertificateARN) {
		return errors.New("media certificate must be an ACM ARN")
	}
	return nil
}

func aliasValues(domain string) pulumi.StringArrayInput {
	if domain == "" {
		return pulumi.StringArray{}
	}
	return pulumi.StringArray{pulumi.String(domain)}
}

func bucketPolicy(bucketARN, distributionARN string) (string, error) {
	document := struct {
		Version   string        `json:"Version"`
		Statement []policyEntry `json:"Statement"`
	}{Version: "2012-10-17", Statement: []policyEntry{{Effect: "Allow", Principal: map[string]string{"Service": "cloudfront.amazonaws.com"}, Action: []string{"s3:GetObject"}, Resource: bucketARN + "/*", Condition: map[string]map[string]string{"StringEquals": {"AWS:SourceArn": distributionARN}}}}}
	encoded, err := json.Marshal(document)
	return string(encoded), err
}

type policyEntry struct {
	Effect    string                       `json:"Effect"`
	Principal map[string]string            `json:"Principal"`
	Action    []string                     `json:"Action"`
	Resource  string                       `json:"Resource"`
	Condition map[string]map[string]string `json:"Condition"`
}

func tags(input map[string]string, component string) map[string]string {
	result := make(map[string]string, len(input)+2)
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		result[key] = input[key]
	}
	result["Name"] = component + "-media"
	result["magelift:component"] = component
	result["magelift:role"] = "object-storage"
	return result
}
